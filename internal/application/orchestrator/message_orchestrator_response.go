package orchestrator

import (
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

type messageResponseAssembler struct{}

func (messageResponseAssembler) Build(response string, decision routing.Decision, jobID task.JobID) ProcessMessageResponse {
	return ProcessMessageResponse{
		Response:   response,
		Route:      decision.Route,
		Confidence: decision.Confidence,
		JobID:      jobID.String(),
	}
}

func (messageResponseAssembler) BuildChatCommand(response string) ProcessMessageResponse {
	return ProcessMessageResponse{
		Response:   response,
		Route:      routing.RouteCHAT,
		Confidence: 1.0,
		JobID:      task.NewJobID().String(),
	}
}
