package orchestrator

import (
	"fmt"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

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

func (e *DefaultCodeExecutor) emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {
	if e.eventEmitter != nil {
		e.eventEmitter(eventType, from, to, content, route, jobID, sessionID, channel, chatID)
	}
}

// SetEventEmitter はイベント発火関数を設定
func (e *DefaultCodeExecutor) SetEventEmitter(emitter func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string)) {
	e.eventEmitter = emitter
}
