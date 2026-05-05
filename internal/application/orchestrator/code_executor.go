package orchestrator

import (
	"context"
	"fmt"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
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
	name          string
	coder         CoderAgent
	systemPrompt  string
	release       func()        // CoderStatus解放用（オプション）
	degradedRoute routing.Route // 品質縮退が発生した場合の実際のルート（空 = 縮退なし）
}

// DefaultCodeExecutor は標準的なCodeExecutor実装
type DefaultCodeExecutor struct {
	coder1          CoderAgent
	coder2          CoderAgent
	coder3          CoderAgent
	coder4          CoderAgent // v4.1: 4th coder slot
	workerExecution service.WorkerExecutionService
	coderStatus     *CoderStatus // optional: coder busy state management
	eventEmitter    func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string)
	coderCaps       []capability.CoderCapability // Phase 3: nil = 静的チェーン（後方互換）
}

// NewDefaultCodeExecutor は新しいDefaultCodeExecutorを作成
func NewDefaultCodeExecutor(
	coder1, coder2, coder3, coder4 CoderAgent,
	workerExecution service.WorkerExecutionService,
	coderStatus *CoderStatus,
	eventEmitter func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string),
) *DefaultCodeExecutor {
	return &DefaultCodeExecutor{
		coder1:          coder1,
		coder2:          coder2,
		coder3:          coder3,
		coder4:          coder4,
		workerExecution: workerExecution,
		coderStatus:     coderStatus,
		eventEmitter:    eventEmitter,
	}
}

// WithCapabilities は動的コーダー選択に使う能力情報を設定する（Phase 3）
func (e *DefaultCodeExecutor) WithCapabilities(caps []capability.CoderCapability) *DefaultCodeExecutor {
	e.coderCaps = caps
	return e
}

// ExecuteCode はコード生成タスクを実行
func (e *DefaultCodeExecutor) ExecuteCode(ctx context.Context, req CodeExecutionRequest) (CodeExecutionResponse, error) {
	target, err := e.selectCoderForRoute(req.Route)
	if err != nil {
		return CodeExecutionResponse{}, err
	}
	// CoderStatusのrelease処理
	if target.release != nil {
		defer target.release()
	}

	log.Printf("[CodeExecutor] code handoff route=%s target=%s job=%s", req.Route, target.name, req.JobID)

	// 明示ルートで品質縮退が発生した場合にユーザー通知
	if target.degradedRoute != "" && req.Route != routing.RouteCODE {
		msg := fmt.Sprintf("⚠️ %s は利用不可のため %s 品質で代替実行します", req.Route, target.degradedRoute)
		e.emit("agent.notice", "shiro", "mio", msg, req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
		log.Printf("[CodeExecutor] quality degraded route=%s degraded=%s target=%s", req.Route, target.degradedRoute, target.name)
	}

	e.emit("agent.start", "mio", "shiro", "コードタスクをShiro経由で実行", req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
	e.emit("agent.start", "shiro", target.name, req.Task.UserMessage(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)

	// CODE3明示ルート、または動的選択でCODE3品質へ縮退したルートは、
	// Proposal生成が可能ならWorkerで即時実行する。
	if shouldUseProposalPath(req.Route, target) && e.workerExecution != nil {
		if resp, handled, err := e.tryExecuteProposalPath(ctx, req, target); handled {
			return resp, err
		}
	}

	return e.executeCoderGeneratePath(ctx, req, target)
}

func shouldUseProposalPath(route routing.Route, target codeTarget) bool {
	switch route {
	case routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3, routing.RouteCODE4:
		return true
	}
	return target.degradedRoute == routing.RouteCODE3
}

// selectCoderForRoute はルートに応じてCoderを選択
func (e *DefaultCodeExecutor) selectCoderForRoute(route routing.Route) (codeTarget, error) {
	// Phase 3: 動的選択（coderCaps が設定されている場合）
	if e.coderCaps != nil {
		chosen, degraded, err := capability.SelectCoder(e.coderCaps, route)
		if err != nil {
			return codeTarget{}, fmt.Errorf("%s route: %w", route, err)
		}
		coder := e.coderByName(chosen)
		if coder == nil {
			return codeTarget{}, fmt.Errorf("%s route: selected coder %s is not initialized", route, chosen)
		}
		log.Printf("[CodeExecutor] coder selected route=%s target=%s mode=dynamic degraded=%s", route, chosen, degraded)
		return codeTarget{
			name:          chosen,
			coder:         coder,
			systemPrompt:  systemPromptForRoute(route),
			degradedRoute: degraded,
		}, nil
	}

	// 後方互換: 静的チェーン（coderCaps が nil の場合）
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
		// 汎用CODEルート: coder1→coder2→coder3→coder4の順でフォールバック
		type coderEntry struct {
			name  string
			coder CoderAgent
		}
		chain := []coderEntry{
			{name: "coder1", coder: e.coder1},
			{name: "coder2", coder: e.coder2},
			{name: "coder3", coder: e.coder3},
			{name: "coder4", coder: e.coder4},
		}
		for _, c := range chain {
			if c.coder == nil {
				log.Printf("[CodeExecutor] coder skip route=%s target=%s reason=unavailable", route, c.name)
				continue
			}
			// CoderStatusがあれば、busy checkを行う
			if e.coderStatus != nil {
				if !e.coderStatus.Acquire(c.name) {
					log.Printf("[CodeExecutor] coder skip route=%s target=%s reason=busy", route, c.name)
					continue
				}
				// Acquire成功時はreleaseを設定
				coderName := c.name
				log.Printf("[CodeExecutor] coder selected route=%s target=%s mode=auto", route, coderName)
				return codeTarget{
					name:         coderName,
					coder:        c.coder,
					systemPrompt: "You are a code generation assistant.",
					release: func() {
						e.coderStatus.Release(coderName)
					},
				}, nil
			}
			// CoderStatusがない場合は単純に選択
			log.Printf("[CodeExecutor] coder selected route=%s target=%s mode=auto", route, c.name)
			return codeTarget{
				name:         c.name,
				coder:        c.coder,
				systemPrompt: "You are a code generation assistant.",
			}, nil
		}
		if e.coderStatus != nil {
			return codeTarget{}, fmt.Errorf("CODE route requested but all coders are busy or unavailable")
		}
		return codeTarget{}, fmt.Errorf("CODE route requested but all coders are unavailable")
	default:
		return codeTarget{}, fmt.Errorf("unknown code route: %s", route)
	}
}

// systemPromptForRoute はルートに対応するシステムプロンプトを返す
func systemPromptForRoute(route routing.Route) string {
	switch route {
	case routing.RouteCODE1:
		return "You are a specification design assistant."
	case routing.RouteCODE2:
		return "You are an implementation assistant."
	case routing.RouteCODE3:
		return "You are a high-quality code review and reasoning assistant."
	case routing.RouteCODE4:
		return "You are a fast prototyping and experimental coding assistant."
	default:
		return "You are a code generation assistant."
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
	case "coder4":
		return e.coder4
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

// SetEventEmitter はイベント発火関数を設定
func (e *DefaultCodeExecutor) SetEventEmitter(emitter func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string)) {
	e.eventEmitter = emitter
}

// explicitCodeRouteTarget はCODE1/CODE2/CODE3/CODE4の明示的ルートを判定
func explicitCodeRouteTarget(route routing.Route) (name, prompt string, ok bool) {
	switch route {
	case routing.RouteCODE1:
		return "coder1", "You are a specification design assistant.", true
	case routing.RouteCODE2:
		return "coder2", "You are an implementation assistant.", true
	case routing.RouteCODE3:
		return "coder3", "You are a high-quality code review and reasoning assistant.", true
	case routing.RouteCODE4:
		return "coder4", "You are a fast prototyping and experimental coding assistant.", true
	default:
		return "", "", false
	}
}
