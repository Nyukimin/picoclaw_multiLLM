package orchestrator

import (
	"fmt"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func (o *MessageOrchestrator) buildTaskForRequest(req ProcessMessageRequest) (task.Task, task.JobID, string) {
	jobID := task.NewJobID()
	t := task.NewTask(jobID, req.UserMessage, req.Channel, req.ChatID).WithAttachments(req.Attachments)
	if len(req.Attachments) > 0 {
		o.emit("viewer.attachment.received", "viewer", "mio",
			fmt.Sprintf("%d attachment(s)", len(req.Attachments)),
			"", jobID.String(), req.SessionID, req.Channel, req.ChatID)
	}
	ttsSessionID := ""
	if o.ttsBridge != nil {
		ttsSessionID = fmt.Sprintf("%s-%s", req.SessionID, jobID.String())
	}
	return t, jobID, ttsSessionID
}
