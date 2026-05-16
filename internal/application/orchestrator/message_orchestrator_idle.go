package orchestrator

import "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"

func (o *MessageOrchestrator) beginChatBusy() func() {
	if o.idleNotifier == nil {
		return func() {}
	}
	o.idleNotifier.NotifyActivity()
	o.idleNotifier.SetChatBusy(true)
	return func() {
		o.idleNotifier.SetChatBusy(false)
	}
}

func (o *MessageOrchestrator) beginWorkerBusy(route routing.Route) func() {
	if o.idleNotifier == nil || route == routing.RouteCHAT {
		return func() {}
	}
	o.idleNotifier.SetWorkerBusy(true)
	return func() {
		o.idleNotifier.SetWorkerBusy(false)
	}
}
