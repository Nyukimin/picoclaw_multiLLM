package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
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
	}
}

// ProcessMessage はメッセージを処理
func (o *MessageOrchestrator) ProcessMessage(ctx context.Context, req ProcessMessageRequest) (ProcessMessageResponse, error) {
	// 1. セッションをロードまたは作成
	sess, err := o.loadOrCreateSession(ctx, req.SessionID, req.Channel, req.ChatID)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("failed to load or create session: %w", err)
	}

	// 2. タスクを作成
	jobID := task.NewJobID()
	t := task.NewTask(jobID, req.UserMessage, req.Channel, req.ChatID)

	// 3. ルーティング決定
	decision, err := o.mio.DecideAction(ctx, t)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("routing decision failed: %w", err)
	}

	// タスクにルートを設定
	t = t.WithRoute(decision.Route)

	// 4. ルートに応じて実行
	response, err := o.executeTask(ctx, t, decision.Route)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("task execution failed: %w", err)
	}

	// 5. タスクを履歴に追加
	sess.AddTask(t)

	// 6. セッションを保存
	if err := o.sessionRepo.Save(ctx, sess); err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("failed to save session: %w", err)
	}

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
func (o *MessageOrchestrator) executeTask(ctx context.Context, t task.Task, route routing.Route) (string, error) {
	switch route {
	case routing.RouteCHAT:
		return o.mio.Chat(ctx, t)

	case routing.RouteOPS:
		return o.shiro.Execute(ctx, t)

	case routing.RouteCODE:
		if o.coder1 != nil {
			return o.coder1.Generate(ctx, t, "You are a code generation assistant.")
		}
		return "", fmt.Errorf("CODE route requested but no coder1 available")

	case routing.RouteCODE1:
		if o.coder1 != nil {
			return o.coder1.Generate(ctx, t, "You are a specification design assistant.")
		}
		return "", fmt.Errorf("CODE1 route requested but no coder1 available")

	case routing.RouteCODE2:
		if o.coder2 != nil {
			return o.coder2.Generate(ctx, t, "You are an implementation assistant.")
		}
		return "", fmt.Errorf("CODE2 route requested but no coder2 available")

	case routing.RouteCODE3:
		if o.coder3 == nil {
			return "", fmt.Errorf("CODE3 route requested but no coder3 available")
		}

		// Coder3がProposal生成をサポートするか確認
		if coderWithProposal, ok := o.coder3.(CoderAgentWithProposal); ok {
			// Proposal生成 → Worker即時実行
			p, err := coderWithProposal.GenerateProposal(ctx, t)
			if err != nil {
				return "", fmt.Errorf("coder3 proposal generation failed: %w", err)
			}

			if p == nil || !p.IsValid() {
				return "", fmt.Errorf("coder3 generated invalid proposal")
			}

			// Worker即時実行（核心機能）
			result, err := o.workerExecution.ExecuteProposal(ctx, t.JobID(), p)
			if err != nil {
				return "", fmt.Errorf("worker execution failed: %w", err)
			}

			// 結果をフォーマット
			return o.formatExecutionResult(p, result), nil
		}

		// フォールバック：Proposal非対応の場合は通常のGenerate()を使用
		return o.coder3.Generate(ctx, t, "You are a high-quality code review and reasoning assistant.")

	case routing.RoutePLAN:
		// PLAN は現時点では CHAT として処理（将来的に専用エージェント追加可能）
		return o.mio.Chat(ctx, t)

	case routing.RouteANALYZE:
		// ANALYZE は現時点では CHAT として処理
		return o.mio.Chat(ctx, t)

	case routing.RouteRESEARCH:
		// RESEARCH は現時点では CHAT として処理
		return o.mio.Chat(ctx, t)

	default:
		return "", fmt.Errorf("unknown route: %s", route)
	}
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
