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
		return o.executeCodeFallbackChain(ctx, t)

	case routing.RouteCODE1:
		if o.coder1 == nil {
			return "", fmt.Errorf("CODE1 route requested but no coder1 available")
		}
		o.emit("agent.start", "mio", "coder1", "仕様設計を依頼", "CODE1", jid, sessionID, channel, chatID)
		resp, err := o.coder1.Generate(ctx, t, "You are a specification design assistant.")
		if err == nil {
			o.emit("agent.response", "coder1", "mio", truncate(resp, 500), "CODE1", jid, sessionID, channel, chatID)
		}
		return resp, err

	case routing.RouteCODE2:
		if o.coder2 == nil {
			return "", fmt.Errorf("CODE2 route requested but no coder2 available")
		}
		o.emit("agent.start", "mio", "coder2", "実装を依頼", "CODE2", jid, sessionID, channel, chatID)
		resp, err := o.coder2.Generate(ctx, t, "You are an implementation assistant.")
		if err == nil {
			o.emit("agent.response", "coder2", "mio", truncate(resp, 500), "CODE2", jid, sessionID, channel, chatID)
		}
		return resp, err

	case routing.RouteCODE3:
		if o.coder3 == nil {
			return "", fmt.Errorf("CODE3 route requested but no coder3 available")
		}

		// Coder3がProposal生成をサポートするか確認
		if coderWithProposal, ok := o.coder3.(CoderAgentWithProposal); ok {
			o.emit("agent.start", "mio", "shiro", "Coder3にコード生成を依頼します", "CODE3", jid, sessionID, channel, chatID)
			o.emit("agent.start", "shiro", "coder3", t.UserMessage(), "CODE3", jid, sessionID, channel, chatID)

			// Proposal生成 → Worker即時実行
			p, err := coderWithProposal.GenerateProposal(ctx, t)
			if err != nil {
				o.emit("agent.response", "coder3", "shiro", "エラー: "+err.Error(), "CODE3", jid, sessionID, channel, chatID)
				return "", fmt.Errorf("coder3 proposal generation failed: %w", err)
			}

			if p == nil || !p.IsValid() {
				o.emit("agent.response", "coder3", "shiro", "無効な Proposal が返されました", "CODE3", jid, sessionID, channel, chatID)
				return "", fmt.Errorf("coder3 generated invalid proposal")
			}

			o.emit("agent.response", "coder3", "shiro", "## Plan\n"+p.Plan(), "CODE3", jid, sessionID, channel, chatID)
			o.emit("agent.start", "shiro", "mio", "Patch を実行中...", "CODE3", jid, sessionID, channel, chatID)

			// Worker即時実行（核心機能）
			result, err := o.workerExecution.ExecuteProposal(ctx, t.JobID(), p)
			if err != nil {
				o.emit("agent.response", "shiro", "mio", "実行失敗: "+err.Error(), "CODE3", jid, sessionID, channel, chatID)
				return "", fmt.Errorf("worker execution failed: %w", err)
			}

			// 結果をフォーマット
			formatted := o.formatExecutionResult(p, result)
			o.emit("agent.response", "shiro", "mio", formatted, "CODE3", jid, sessionID, channel, chatID)
			return formatted, nil
		}

		// フォールバック：Proposal非対応の場合は通常のGenerate()を使用
		o.emit("agent.start", "mio", "coder3", "コードレビューを依頼", "CODE3", jid, sessionID, channel, chatID)
		resp, err := o.coder3.Generate(ctx, t, "You are a high-quality code review and reasoning assistant.")
		if err == nil {
			o.emit("agent.response", "coder3", "mio", truncate(resp, 500), "CODE3", jid, sessionID, channel, chatID)
		}
		return resp, err

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

// executeCodeFallbackChain はCODEルートのフォールバックチェーン実行
// Coder1 → Coder2 → Coder3 の順に利用可能なCoderを試行する
func (o *MessageOrchestrator) executeCodeFallbackChain(ctx context.Context, t task.Task) (string, error) {
	type coderEntry struct {
		name  string
		coder CoderAgent
	}
	chain := []coderEntry{
		{"coder1", o.coder1},
		{"coder2", o.coder2},
		{"coder3", o.coder3},
	}

	for _, c := range chain {
		if c.coder == nil {
			continue
		}
		if !o.coderStatus.Acquire(c.name) {
			continue // ビジー → 次へ
		}
		defer o.coderStatus.Release(c.name)
		return c.coder.Generate(ctx, t, "You are a code generation assistant.")
	}

	return "", fmt.Errorf("CODE route requested but all coders are busy or unavailable")
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
