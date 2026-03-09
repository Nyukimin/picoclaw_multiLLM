package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/patch"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// ProcessMessageRequest はメッセージ処理リクエスト
type ProcessMessageRequest struct {
	SessionID   string
	Channel     string
	ChatID      string
	UserMessage string
}

// ProcessMessageResponse はメッセージ処理レスポンス
type ProcessMessageResponse struct {
	Response   string
	Route      routing.Route
	Confidence float64
	JobID      string
}

// SessionRepository はセッション永続化のインターフェース
type SessionRepository interface {
	Save(ctx context.Context, sess *session.Session) error
	Load(ctx context.Context, id string) (*session.Session, error)
	Exists(ctx context.Context, id string) (bool, error)
	Delete(ctx context.Context, id string) error
}

// MioAgent はルーティング・会話を担当
type MioAgent interface {
	DecideAction(ctx context.Context, t task.Task) (routing.Decision, error)
	Chat(ctx context.Context, t task.Task) (string, error)
	HandleChatCommand(ctx context.Context, sessionID string, message string) (agent.ChatCommandResult, error)
}

// ShiroAgent は実行を担当
type ShiroAgent interface {
	Execute(ctx context.Context, t task.Task) (string, error)
}

// CoderAgent はコード生成を担当
type CoderAgent interface {
	Generate(ctx context.Context, t task.Task, systemPrompt string) (string, error)
}

// CoderAgentWithProposal はProposal生成機能を持つCoderAgent
type CoderAgentWithProposal interface {
	CoderAgent
	GenerateProposal(ctx context.Context, t task.Task) (*proposal.Proposal, error)
}

// MessageOrchestrator はメッセージ処理を統括
type MessageOrchestrator struct {
	sessionRepo     SessionRepository
	mio             MioAgent
	shiro           ShiroAgent
	coder1          CoderAgent // DeepSeek
	coder2          CoderAgent // OpenAI
	coder3          CoderAgent // Claude
	workerExecution service.WorkerExecutionService
	coderStatus     *CoderStatus
	listener        EventListener
	idleNotifier    IdleNotifier
}

// NewMessageOrchestrator は新しいMessageOrchestratorを作成
func NewMessageOrchestrator(
	sessionRepo SessionRepository,
	mio MioAgent,
	shiro ShiroAgent,
	coder1 CoderAgent,
	coder2 CoderAgent,
	coder3 CoderAgent,
	workerExecution service.WorkerExecutionService,
) *MessageOrchestrator {
	return &MessageOrchestrator{
		sessionRepo:     sessionRepo,
		mio:             mio,
		shiro:           shiro,
		coder1:          coder1,
		coder2:          coder2,
		coder3:          coder3,
		workerExecution: workerExecution,
		coderStatus:     NewCoderStatus(),
	}
}

// SetEventListener sets an optional listener for monitoring events.
func (o *MessageOrchestrator) SetEventListener(l EventListener) {
	o.listener = l
}

// SetIdleNotifier sets an optional notifier used to control idle chat.
func (o *MessageOrchestrator) SetIdleNotifier(n IdleNotifier) {
	o.idleNotifier = n
}

func (o *MessageOrchestrator) emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {
	if o.listener == nil {
		log.Printf("[MessageOrch] emit SKIPPED: no listener (eventType=%s from=%s to=%s)", eventType, from, to)
		return
	}
	log.Printf("[MessageOrch] emit: eventType=%s from=%s to=%s route=%s jobID=%s", eventType, from, to, route, jobID)
	o.listener.OnEvent(NewEvent(eventType, from, to, content, route, jobID, sessionID, channel, chatID))
}

// ProcessMessage はメッセージを処理
func (o *MessageOrchestrator) ProcessMessage(ctx context.Context, req ProcessMessageRequest) (ProcessMessageResponse, error) {
	log.Printf("[MessageOrch] ProcessMessage START: sessionID=%s channel=%s chatID=%s message=%q",
		req.SessionID, req.Channel, req.ChatID, req.UserMessage)

	if o.idleNotifier != nil {
		o.idleNotifier.NotifyActivity()
		o.idleNotifier.SetChatBusy(true)
		defer o.idleNotifier.SetChatBusy(false)
	}

	// 1. セッションをロードまたは作成
	sess, err := o.loadOrCreateSession(ctx, req.SessionID, req.Channel, req.ChatID)
	if err != nil {
		log.Printf("[MessageOrch] ProcessMessage ERROR: failed to load or create session: %v", err)
		return ProcessMessageResponse{}, fmt.Errorf("failed to load or create session: %w", err)
	}
	log.Printf("[MessageOrch] Session loaded/created: %s", sess.ID())

	// Event: ユーザーメッセージ受信
	o.emit("message.received", "user", "mio", req.UserMessage, "", "", req.SessionID, req.Channel, req.ChatID)

	// 2. チャットコマンドのチェック（ルーティング前）
	cmdResult, err := o.mio.HandleChatCommand(ctx, req.ChatID, req.UserMessage)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("chat command failed: %w", err)
	}
	if cmdResult.Handled {
		o.emit("agent.response", "mio", "user", cmdResult.Response, "CHAT", "", req.SessionID, req.Channel, req.ChatID)
		return ProcessMessageResponse{
			Response:   cmdResult.Response,
			Route:      routing.RouteCHAT,
			Confidence: 1.0,
			JobID:      task.NewJobID().String(),
		}, nil
	}

	// 3. タスクを作成
	jobID := task.NewJobID()
	t := task.NewTask(jobID, req.UserMessage, req.Channel, req.ChatID)

	// 4. ルーティング決定
	decision, err := o.mio.DecideAction(ctx, t)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("routing decision failed: %w", err)
	}

	// Event: ルーティング決定
	o.emit("routing.decision", "mio", "",
		fmt.Sprintf("confidence %.0f%%", decision.Confidence*100),
		string(decision.Route), jobID.String(), req.SessionID, req.Channel, req.ChatID)

	// タスクにルートを設定
	t = t.WithRoute(decision.Route)

	workerMarkedBusy := false
	if o.idleNotifier != nil && decision.Route != routing.RouteCHAT {
		o.idleNotifier.SetWorkerBusy(true)
		workerMarkedBusy = true
	}
	if workerMarkedBusy {
		defer o.idleNotifier.SetWorkerBusy(false)
	}

	// 4. ルートに応じて実行
	response, err := o.executeTask(ctx, t, decision.Route, req.SessionID, req.Channel, req.ChatID)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("task execution failed: %w", err)
	}

	// 5. タスクを履歴に追加
	sess.AddTask(t)

	// 6. セッションを保存
	if err := o.sessionRepo.Save(ctx, sess); err != nil {
		log.Printf("[MessageOrch] ProcessMessage ERROR: failed to save session: %v", err)
		return ProcessMessageResponse{}, fmt.Errorf("failed to save session: %w", err)
	}

	log.Printf("[MessageOrch] ProcessMessage COMPLETE: jobID=%s route=%s response_len=%d",
		jobID.String(), decision.Route, len(response))

	return ProcessMessageResponse{
		Response:   response,
		Route:      decision.Route,
		Confidence: decision.Confidence,
		JobID:      jobID.String(),
	}, nil
}

// loadOrCreateSession はセッションをロードまたは作成
func (o *MessageOrchestrator) loadOrCreateSession(ctx context.Context, id, channel, chatID string) (*session.Session, error) {
	sess, err := o.sessionRepo.Load(ctx, id)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			// 新規セッション作成
			return session.NewSession(id, channel, chatID), nil
		}
		return nil, err
	}
	return sess, nil
}

// executeTask はルートに応じてタスクを実行
func (o *MessageOrchestrator) executeTask(ctx context.Context, t task.Task, route routing.Route, sessionID, channel, chatID string) (string, error) {
	jid := t.JobID().String()

	switch route {
	case routing.RouteCHAT:
		o.emit("agent.start", "mio", "user", "考え中...", "CHAT", jid, sessionID, channel, chatID)
		streamCtx := llm.ContextWithStreamCallback(ctx, func(token string) {
			o.emit("agent.thinking", "mio", "user", token, "CHAT", jid, sessionID, channel, chatID)
		})
		resp, err := o.mio.Chat(streamCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "CHAT", jid, sessionID, channel, chatID)
		}
		return resp, err

	case routing.RouteOPS:
		o.emit("agent.start", "mio", "shiro", "タスクを実行依頼", "OPS", jid, sessionID, channel, chatID)
		resp, err := o.shiro.Execute(ctx, t)
		if err == nil {
			o.emit("agent.response", "shiro", "mio", resp, "OPS", jid, sessionID, channel, chatID)
		}
		return resp, err

	case routing.RouteCODE:
		return o.executeCodeViaShiro(ctx, t, route, sessionID, channel, chatID)

	case routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3:
		return o.executeCodeViaShiro(ctx, t, route, sessionID, channel, chatID)

	case routing.RoutePLAN:
		o.emit("agent.start", "mio", "user", "計画を検討中...", "PLAN", jid, sessionID, channel, chatID)
		planCtx := llm.ContextWithStreamCallback(ctx, func(token string) {
			o.emit("agent.thinking", "mio", "user", token, "PLAN", jid, sessionID, channel, chatID)
		})
		resp, err := o.mio.Chat(planCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "PLAN", jid, sessionID, channel, chatID)
		}
		return resp, err

	case routing.RouteANALYZE:
		o.emit("agent.start", "mio", "user", "分析中...", "ANALYZE", jid, sessionID, channel, chatID)
		analyzeCtx := llm.ContextWithStreamCallback(ctx, func(token string) {
			o.emit("agent.thinking", "mio", "user", token, "ANALYZE", jid, sessionID, channel, chatID)
		})
		resp, err := o.mio.Chat(analyzeCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "ANALYZE", jid, sessionID, channel, chatID)
		}
		return resp, err

	case routing.RouteRESEARCH:
		o.emit("agent.start", "mio", "user", "調査中...", "RESEARCH", jid, sessionID, channel, chatID)
		researchCtx := llm.ContextWithStreamCallback(ctx, func(token string) {
			o.emit("agent.thinking", "mio", "user", token, "RESEARCH", jid, sessionID, channel, chatID)
		})
		resp, err := o.mio.Chat(researchCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "RESEARCH", jid, sessionID, channel, chatID)
		}
		return resp, err

	default:
		return "", fmt.Errorf("unknown route: %s", route)
	}
}

type codeTarget struct {
	name         string
	coder        CoderAgent
	systemPrompt string
	release      func()
}

func (o *MessageOrchestrator) coderByName(name string) CoderAgent {
	switch name {
	case "coder1":
		return o.coder1
	case "coder2":
		return o.coder2
	case "coder3":
		return o.coder3
	default:
		return nil
	}
}

func explicitCodeRouteTarget(route routing.Route) (name, prompt string, ok bool) {
	switch route {
	case routing.RouteCODE1:
		return "coder1", "You are a specification design assistant.", true
	case routing.RouteCODE2:
		return "coder2", "You are an implementation assistant.", true
	case routing.RouteCODE3:
		return "coder3", "You are a high-quality code review and reasoning assistant.", true
	default:
		return "", "", false
	}
}

func (o *MessageOrchestrator) selectCoderForRoute(route routing.Route) (codeTarget, error) {
	if name, prompt, ok := explicitCodeRouteTarget(route); ok {
		coder := o.coderByName(name)
		if coder == nil {
			return codeTarget{}, fmt.Errorf("%s route requested but no %s available", route, name)
		}
		return codeTarget{name: name, coder: coder, systemPrompt: prompt}, nil
	}

	switch route {
	case routing.RouteCODE:
		type coderEntry struct {
			name  string
			coder CoderAgent
		}
		chain := []coderEntry{
			{name: "coder1", coder: o.coder1},
			{name: "coder2", coder: o.coder2},
			{name: "coder3", coder: o.coder3},
		}
		for _, c := range chain {
			if c.coder == nil {
				continue
			}
			if !o.coderStatus.Acquire(c.name) {
				continue
			}
			coderName := c.name
			return codeTarget{
				name:         coderName,
				coder:        c.coder,
				systemPrompt: "You are a code generation assistant.",
				release: func() {
					o.coderStatus.Release(coderName)
				},
			}, nil
		}
		return codeTarget{}, fmt.Errorf("CODE route requested but all coders are busy or unavailable")
	default:
		return codeTarget{}, fmt.Errorf("unknown code route: %s", route)
	}
}

func (o *MessageOrchestrator) executeCodeViaShiro(
	ctx context.Context,
	t task.Task,
	route routing.Route,
	sessionID, channel, chatID string,
) (string, error) {
	jid := t.JobID().String()
	target, err := o.selectCoderForRoute(route)
	if err != nil {
		return "", err
	}
	if target.release != nil {
		defer target.release()
	}

	o.emit("agent.start", "mio", "shiro", "コードタスクをShiro経由で実行", route.String(), jid, sessionID, channel, chatID)
	o.emit("agent.start", "shiro", target.name, t.UserMessage(), route.String(), jid, sessionID, channel, chatID)

	// CODE3 明示ルートは Proposal 生成が可能なら Worker で即時実行する。
	if route == routing.RouteCODE3 && o.workerExecution != nil {
		if resp, handled, err := o.tryExecuteProposalPath(ctx, t, route, target, sessionID, channel, chatID, jid); handled {
			return resp, err
		}
	}

	return o.executeCoderGeneratePath(ctx, t, route, target, sessionID, channel, chatID, jid)
}

func (o *MessageOrchestrator) tryExecuteProposalPath(
	ctx context.Context,
	t task.Task,
	route routing.Route,
	target codeTarget,
	sessionID, channel, chatID, jid string,
) (string, bool, error) {
	coderWithProposal, ok := target.coder.(CoderAgentWithProposal)
	if !ok {
		return "", false, nil
	}

	p, err := coderWithProposal.GenerateProposal(ctx, t)
	if err != nil {
		o.emit("agent.response", target.name, "shiro", "エラー: "+err.Error(), route.String(), jid, sessionID, channel, chatID)
		return "", true, fmt.Errorf("%s proposal generation failed: %w", target.name, err)
	}
	if p == nil || !p.IsValid() {
		o.emit("agent.response", target.name, "shiro", "無効な Proposal が返されました", route.String(), jid, sessionID, channel, chatID)
		return "", true, fmt.Errorf("%s generated invalid proposal", target.name)
	}

	o.emit("agent.response", target.name, "shiro", "## Plan\n"+p.Plan(), route.String(), jid, sessionID, channel, chatID)
	o.emit("agent.start", "shiro", "mio", "Patch を実行中...", route.String(), jid, sessionID, channel, chatID)

	result, err := o.workerExecution.ExecuteProposal(ctx, t.JobID(), p)
	if err != nil {
		o.emit("agent.response", "shiro", "mio", "実行失敗: "+err.Error(), route.String(), jid, sessionID, channel, chatID)
		return "", true, fmt.Errorf("worker execution failed: %w", err)
	}

	formatted := o.formatExecutionResult(p, result)
	o.emit("agent.response", "shiro", "mio", formatted, route.String(), jid, sessionID, channel, chatID)
	return formatted, true, nil
}

func (o *MessageOrchestrator) executeCoderGeneratePath(
	ctx context.Context,
	t task.Task,
	route routing.Route,
	target codeTarget,
	sessionID, channel, chatID, jid string,
) (string, error) {
	resp, err := target.coder.Generate(ctx, t, target.systemPrompt)
	if err != nil {
		o.emit("agent.response", target.name, "shiro", "エラー: "+err.Error(), route.String(), jid, sessionID, channel, chatID)
		return "", err
	}
	o.emit("agent.response", target.name, "shiro", truncate(resp, 500), route.String(), jid, sessionID, channel, chatID)
	o.emit("agent.response", "shiro", "mio", truncate(resp, 500), route.String(), jid, sessionID, channel, chatID)
	return resp, nil
}

// truncate はビュワー表示用に長いテキストを切り詰める
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// 行単位で切り詰め
	lines := strings.SplitN(s, "\n", -1)
	var b strings.Builder
	for _, line := range lines {
		if b.Len()+len(line)+1 > maxLen {
			b.WriteString("\n... (truncated)")
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

// formatExecutionResult はProposalとPatchExecutionResultを整形
func (o *MessageOrchestrator) formatExecutionResult(
	p *proposal.Proposal,
	result *patch.PatchExecutionResult,
) string {
	// 成功/失敗の絵文字
	statusEmoji := "✅"
	if !result.Success {
		statusEmoji = "⚠️"
	}

	// Gitコミット行
	gitCommitLine := ""
	if result.GitCommit != "" && result.GitCommit != "no-changes" {
		shortHash := result.GitCommit
		if len(shortHash) > 8 {
			shortHash = shortHash[:8]
		}
		gitCommitLine = fmt.Sprintf("\n- **Git Commit**: `%s`", shortHash)
	}

	// コマンド結果詳細
	commandDetails := ""
	for i, cmdResult := range result.Results {
		status := "✅"
		if !cmdResult.Success {
			status = "❌"
		}
		commandDetails += fmt.Sprintf("\n%d. %s `%s` %s",
			i+1, status, cmdResult.Command.Action, cmdResult.Command.Target)
		if cmdResult.Error != "" {
			commandDetails += fmt.Sprintf("\n   Error: %s", cmdResult.Error)
		}
	}

	return fmt.Sprintf(`## Plan
%s

## Execution Result
- **Status**: %s
- **Executed**: %d commands
- **Failed**: %d commands
- **Success Rate**: %.1f%%%s

### Command Results%s

## Risk
%s
`,
		p.Plan(),
		statusEmoji,
		result.ExecutedCmds,
		result.FailedCmds,
		result.SuccessRate()*100,
		gitCommitLine,
		commandDetails,
		p.Risk(),
	)
}
