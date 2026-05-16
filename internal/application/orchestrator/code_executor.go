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
	e.emitDegradedRouteNotice(req, target)
	e.emitCodeHandoffStart(req, target)

	// CODE3明示ルート、または動的選択でCODE3品質へ縮退したルートは、
	// Proposal生成が可能ならWorkerで即時実行する。
	if shouldUseProposalPath(req.Route, target) && e.workerExecution != nil {
		if resp, handled, err := e.tryExecuteProposalPath(ctx, req, target); handled {
			return resp, err
		}
	}

	return e.executeCoderGeneratePath(ctx, req, target)
}

func (e *DefaultCodeExecutor) emitDegradedRouteNotice(req CodeExecutionRequest, target codeTarget) {
	if target.degradedRoute == "" || req.Route == routing.RouteCODE {
		return
	}
	msg := fmt.Sprintf("⚠️ %s は利用不可のため %s 品質で代替実行します", req.Route, target.degradedRoute)
	e.emit("agent.notice", "shiro", "mio", msg, req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
	log.Printf("[CodeExecutor] quality degraded route=%s degraded=%s target=%s", req.Route, target.degradedRoute, target.name)
}

func (e *DefaultCodeExecutor) emitCodeHandoffStart(req CodeExecutionRequest, target codeTarget) {
	e.emit("agent.start", "mio", "shiro", "コードタスクをShiro経由で実行", req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
	e.emit("agent.start", "shiro", target.name, req.Task.UserMessage(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
}

// executeCoderGeneratePath は通常のGenerate実行パス
func (e *DefaultCodeExecutor) executeCoderGeneratePath(
	ctx context.Context,
	req CodeExecutionRequest,
	target codeTarget,
) (CodeExecutionResponse, error) {
	resp, err := target.coder.Generate(ctx, req.Task, target.systemPrompt)
	if err != nil {
		e.emitCoderGenerateError(req, target, err)
		return CodeExecutionResponse{}, err
	}

	e.emitCoderGenerateResponse(req, target, resp)

	return buildCoderGenerateResponse(resp), nil
}

func buildProposalHandledResponse(response string) CodeExecutionResponse {
	return CodeExecutionResponse{Response: response, Handled: true}
}

func buildCoderGenerateResponse(response string) CodeExecutionResponse {
	return CodeExecutionResponse{Response: response, Handled: false}
}

func (e *DefaultCodeExecutor) emitCoderGenerateError(req CodeExecutionRequest, target codeTarget, err error) {
	e.emit("agent.response", target.name, "shiro", "エラー: "+err.Error(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
}

func (e *DefaultCodeExecutor) emitCoderGenerateResponse(req CodeExecutionRequest, target codeTarget, response string) {
	content := truncate(response, 500)
	e.emit("agent.response", target.name, "shiro", content, req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
	e.emit("agent.response", "shiro", "mio", content, req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
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
