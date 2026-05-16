package orchestrator

import (
	"context"
	"fmt"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// executeTask はルートに応じてタスクを実行
func (o *MessageOrchestrator) executeTask(ctx context.Context, t task.Task, route routing.Route, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	if route != routing.RouteCHAT {
		return o.executeAutonomousTask(ctx, t, route, sessionID, channel, chatID, ttsSessionID)
	}

	return o.executeChatRoute(ctx, t, sessionID, channel, chatID, ttsSessionID)
}

func (o *MessageOrchestrator) executeChatRoute(ctx context.Context, t task.Task, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	jid := t.JobID().String()
	o.emit("agent.start", "mio", "user", "考え中...", "CHAT", jid, sessionID, channel, chatID)
	streamCtx, ttsStream := o.withStreamHooks(ctx, routing.RouteCHAT, jid, sessionID, channel, chatID, ttsSessionID)
	resp, err := o.mio.Chat(streamCtx, t)
	if err == nil {
		o.emit("agent.response", "mio", "user", resp, "CHAT", jid, sessionID, channel, chatID)
		ttsStream.Finalize(ctx, resp)
	}
	return resp, err
}

func (o *MessageOrchestrator) executeRouteDirect(ctx context.Context, t task.Task, route routing.Route, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	switch route {
	case routing.RouteOPS:
		return o.executeOPSRoute(ctx, t, sessionID, channel, chatID, ttsSessionID)
	case routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3, routing.RouteCODE4:
		return o.executeCodeRoute(ctx, t, route, sessionID, channel, chatID, ttsSessionID)
	case routing.RouteWILD:
		return o.executeWildRoute(ctx, t, sessionID, channel, chatID, ttsSessionID)
	case routing.RoutePLAN:
		return o.executePlanRoute(ctx, t, sessionID, channel, chatID, ttsSessionID)
	case routing.RouteANALYZE:
		return o.executeAnalyzeRoute(ctx, t, sessionID, channel, chatID, ttsSessionID)
	case routing.RouteRESEARCH:
		return o.executeResearchRoute(ctx, t, sessionID, channel, chatID, ttsSessionID)
	default:
		return "", fmt.Errorf("unsupported autonomous route: %s", route)
	}
}

func (o *MessageOrchestrator) executeOPSRoute(ctx context.Context, t task.Task, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	jid := t.JobID().String()
	o.emit("agent.start", "mio", "shiro", "タスクを実行依頼", "OPS", jid, sessionID, channel, chatID)
	resp, err := o.shiro.Execute(ctx, t)
	if err == nil {
		o.emit("agent.response", "shiro", "mio", resp, "OPS", jid, sessionID, channel, chatID)
		o.pushTTS(ctx, ttsSessionID, routing.RouteOPS, "agent.response", resp)
	}
	return resp, err
}

func (o *MessageOrchestrator) executeCodeRoute(ctx context.Context, t task.Task, route routing.Route, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	resp, err := o.executeCodeViaShiro(ctx, t, route, sessionID, channel, chatID)
	if err == nil {
		o.pushTTS(ctx, ttsSessionID, route, "agent.response", resp)
	}
	return resp, err
}

func (o *MessageOrchestrator) executeWildRoute(ctx context.Context, t task.Task, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	if o.wild == nil {
		return "", fmt.Errorf("no wild agent available")
	}
	jid := t.JobID().String()
	o.emit("agent.start", "mio", "wild", "創作中...", "WILD", jid, sessionID, channel, chatID)
	streamCtx, ttsStream := o.withStreamHooks(ctx, routing.RouteWILD, jid, sessionID, channel, chatID, ttsSessionID)
	resp, err := o.wild.Generate(streamCtx, t)
	if err == nil {
		o.emit("agent.response", "wild", "mio", resp, "WILD", jid, sessionID, channel, chatID)
		ttsStream.Finalize(ctx, resp)
	}
	return resp, err
}

func (o *MessageOrchestrator) executePlanRoute(ctx context.Context, t task.Task, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	jid := t.JobID().String()
	o.emit("agent.start", "mio", "user", "計画を検討中...", "PLAN", jid, sessionID, channel, chatID)
	planCtx, ttsStream := o.withStreamHooks(ctx, routing.RoutePLAN, jid, sessionID, channel, chatID, ttsSessionID)
	resp, err := o.mio.Chat(planCtx, t)
	if err == nil {
		o.emit("agent.response", "mio", "user", resp, "PLAN", jid, sessionID, channel, chatID)
		ttsStream.Finalize(ctx, resp)
	}
	return resp, err
}

func (o *MessageOrchestrator) executeAnalyzeRoute(ctx context.Context, t task.Task, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	jid := t.JobID().String()
	if o.heavy == nil {
		o.emit("agent.start", "mio", "user", "分析中...", "ANALYZE", jid, sessionID, channel, chatID)
		analyzeCtx, ttsStream := o.withStreamHooks(ctx, routing.RouteANALYZE, jid, sessionID, channel, chatID, ttsSessionID)
		resp, err := o.mio.Chat(analyzeCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "ANALYZE", jid, sessionID, channel, chatID)
			ttsStream.Finalize(ctx, resp)
		}
		return resp, err
	}
	o.emit("agent.start", "mio", "heavy", "分析中...", "ANALYZE", jid, sessionID, channel, chatID)
	analyzeCtx, ttsStream := o.withStreamHooks(ctx, routing.RouteANALYZE, jid, sessionID, channel, chatID, ttsSessionID)
	resp, err := o.heavy.Generate(analyzeCtx, t)
	if err == nil {
		o.emit("agent.response", "heavy", "mio", resp, "ANALYZE", jid, sessionID, channel, chatID)
		ttsStream.Finalize(ctx, resp)
	}
	return resp, err
}

func (o *MessageOrchestrator) executeResearchRoute(ctx context.Context, t task.Task, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	jid := t.JobID().String()
	o.emit("agent.start", "mio", "user", "調査中...", "RESEARCH", jid, sessionID, channel, chatID)
	researchCtx, ttsStream := o.withStreamHooks(ctx, routing.RouteRESEARCH, jid, sessionID, channel, chatID, ttsSessionID)
	resp, err := o.mio.Chat(researchCtx, t)
	if err == nil {
		o.emit("agent.response", "mio", "user", resp, "RESEARCH", jid, sessionID, channel, chatID)
		ttsStream.Finalize(ctx, resp)
	}
	return resp, err
}

func (o *MessageOrchestrator) executeCodeViaShiro(
	ctx context.Context,
	t task.Task,
	route routing.Route,
	sessionID, channel, chatID string,
) (string, error) {
	// Phase 1リファクタリング: CodeExecutorに委譲
	req := CodeExecutionRequest{
		Task:      t,
		Route:     route,
		SessionID: sessionID,
		Channel:   channel,
		ChatID:    chatID,
		JobID:     t.JobID().String(),
	}
	resp, err := o.codeExecutor.ExecuteCode(ctx, req)
	return resp.Response, err
}
