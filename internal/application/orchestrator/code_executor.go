package orchestrator

import (
	"context"
	"fmt"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// CodeExecutor はコード生成タスクの実行を担当
type CodeExecutor interface {
	ExecuteCode(ctx context.Context, req CodeExecutionRequest) (CodeExecutionResponse, error)
}

// CodeExecutionRequest はコード実行リクエスト
type CodeExecutionRequest struct {
	Task      task.Task
	Route     routing.Route
	SessionID string
	Channel   string
	ChatID    string
	JobID     string
}

// CodeExecutionResponse はコード実行レスポンス
type CodeExecutionResponse struct {
	Response string
	Handled  bool // Proposal経由で処理された場合true
}

// codeTarget はコーダー選択の結果
type codeTarget struct {
	name         string
	coder        CoderAgent
	systemPrompt string
	release      func() // CoderStatus解放用（オプション）
}

// DefaultCodeExecutor は標準的なCodeExecutor実装
type DefaultCodeExecutor struct {
	coder1          CoderAgent
	coder2          CoderAgent
	coder3          CoderAgent
	workerExecution service.WorkerExecutionService
	eventEmitter    func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string)
}

// NewDefaultCodeExecutor は新しいDefaultCodeExecutorを作成
func NewDefaultCodeExecutor(
	coder1, coder2, coder3 CoderAgent,
	workerExecution service.WorkerExecutionService,
	eventEmitter func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string),
) *DefaultCodeExecutor {
	return &DefaultCodeExecutor{
		coder1:          coder1,
		coder2:          coder2,
		coder3:          coder3,
		workerExecution: workerExecution,
		eventEmitter:    eventEmitter,
	}
}

// ExecuteCode はコード生成タスクを実行
func (e *DefaultCodeExecutor) ExecuteCode(ctx context.Context, req CodeExecutionRequest) (CodeExecutionResponse, error) {
	target, err := e.selectCoderForRoute(req.Route)
	if err != nil {
		return CodeExecutionResponse{}, err
	}

	log.Printf("[CodeExecutor] code handoff route=%s target=%s job=%s", req.Route, target.name, req.JobID)

	e.emit("agent.start", "mio", "shiro", "コードタスクをShiro経由で実行", req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
	e.emit("agent.start", "shiro", target.name, req.Task.UserMessage(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)

	// CODE3明示ルートはProposal生成が可能ならWorkerで即時実行
	if req.Route == routing.RouteCODE3 && e.workerExecution != nil {
		if resp, handled, err := e.tryExecuteProposalPath(ctx, req, target); handled {
			return resp, err
		}
	}

	return e.executeCoderGeneratePath(ctx, req, target)
}

// selectCoderForRoute はルートに応じてCoderを選択
func (e *DefaultCodeExecutor) selectCoderForRoute(route routing.Route) (codeTarget, error) {
	if name, prompt, ok := explicitCodeRouteTarget(route); ok {
		coder := e.coderByName(name)
		if coder == nil {
			return codeTarget{}, fmt.Errorf("%s route requested but no %s available", route, name)
		}
		log.Printf("[CodeExecutor] coder selected route=%s target=%s mode=explicit", route, name)
		return codeTarget{name: name, coder: coder, systemPrompt: prompt}, nil
	}

	switch route {
	case routing.RouteCODE:
		// 汎用CODEルート: coder1→coder2→coder3の順でフォールバック
		type coderEntry struct {
			name  string
			coder CoderAgent
		}
		chain := []coderEntry{
			{name: "coder1", coder: e.coder1},
			{name: "coder2", coder: e.coder2},
			{name: "coder3", coder: e.coder3},
		}
		for _, c := range chain {
			if c.coder == nil {
				log.Printf("[CodeExecutor] coder skip route=%s target=%s reason=unavailable", route, c.name)
				continue
			}
			log.Printf("[CodeExecutor] coder selected route=%s target=%s mode=auto", route, c.name)
			return codeTarget{
				name:         c.name,
				coder:        c.coder,
				systemPrompt: "You are a code generation assistant.",
			}, nil
		}
		return codeTarget{}, fmt.Errorf("CODE route requested but all coders are unavailable")
	default:
		return codeTarget{}, fmt.Errorf("unknown code route: %s", route)
	}
}

// coderByName は名前からCoderAgentを取得
func (e *DefaultCodeExecutor) coderByName(name string) CoderAgent {
	switch name {
	case "coder1":
		return e.coder1
	case "coder2":
		return e.coder2
	case "coder3":
		return e.coder3
	default:
		return nil
	}
}

// tryExecuteProposalPath はProposal生成→Worker実行パスを試行
func (e *DefaultCodeExecutor) tryExecuteProposalPath(
	ctx context.Context,
	req CodeExecutionRequest,
	target codeTarget,
) (CodeExecutionResponse, bool, error) {
	coderWithProposal, ok := target.coder.(CoderAgentWithProposal)
	if !ok {
		return CodeExecutionResponse{}, false, nil
	}

	p, err := coderWithProposal.GenerateProposal(ctx, req.Task)
	if err != nil {
		e.emit("agent.response", target.name, "shiro", "エラー: "+err.Error(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
		return CodeExecutionResponse{}, true, fmt.Errorf("%s proposal generation failed: %w", target.name, err)
	}

	if p == nil || !p.IsValid() {
		e.emit("agent.response", target.name, "shiro", "無効な Proposal が返されました", req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
		return CodeExecutionResponse{}, true, fmt.Errorf("%s proposal generation failed: invalid proposal", target.name)
	}

	e.emit("agent.response", target.name, "shiro", "## Plan\n"+p.Plan(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
	e.emit("agent.start", "shiro", "mio", "Patch を実行中...", req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)

	result, err := e.workerExecution.ExecuteProposal(ctx, req.Task.JobID(), p)
	if err != nil {
		e.emit("agent.response", "shiro", "mio", "実行失敗: "+err.Error(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
		return CodeExecutionResponse{}, true, fmt.Errorf("worker execution failed: %w", err)
	}

	formatted := formatExecutionResult(p, result)
	e.emit("agent.response", "shiro", "mio", formatted, req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)

	return CodeExecutionResponse{Response: formatted, Handled: true}, true, nil
}

// executeCoderGeneratePath は通常のGenerate実行パス
func (e *DefaultCodeExecutor) executeCoderGeneratePath(
	ctx context.Context,
	req CodeExecutionRequest,
	target codeTarget,
) (CodeExecutionResponse, error) {
	resp, err := target.coder.Generate(ctx, req.Task, target.systemPrompt)
	if err != nil {
		e.emit("agent.response", target.name, "shiro", "エラー: "+err.Error(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
		return CodeExecutionResponse{}, err
	}

	e.emit("agent.response", target.name, "shiro", truncate(resp, 500), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
	e.emit("agent.response", "shiro", "mio", truncate(resp, 500), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)

	return CodeExecutionResponse{Response: resp, Handled: false}, nil
}

func (e *DefaultCodeExecutor) emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {
	if e.eventEmitter != nil {
		e.eventEmitter(eventType, from, to, content, route, jobID, sessionID, channel, chatID)
	}
}

// explicitCodeRouteTarget はCODE1/CODE2/CODE3の明示的ルートを判定
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
