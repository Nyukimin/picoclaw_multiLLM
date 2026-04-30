package viewer

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	domainexecution "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
)

const (
	monitorOfflineAfter = 120 * time.Second
	monitorMaxLogs      = 2000
	monitorMaxJobEvents = 200
)

var monitorAgents = []string{"mio", "shiro", "coder1", "coder2", "coder3", "coder4"}

type MonitorStore struct {
	mu       sync.RWMutex
	logs     []orchestrator.OrchestratorEvent
	agents   map[string]AgentSnapshot
	jobs     map[string]*JobSnapshot
	evidence EvidenceLister
	archive  EventLogReader
}

type StatusSnapshot struct {
	UpdatedAt string            `json:"updated_at"`
	Chat      ComponentSnapshot `json:"chat"`
	Worker    ComponentSnapshot `json:"worker"`
	Coders    CodersSnapshot    `json:"coders"`
}

type ComponentSnapshot struct {
	Status    string `json:"status"`
	AgentID   string `json:"agent_id,omitempty"`
	JobID     string `json:"job_id,omitempty"`
	Route     string `json:"route,omitempty"`
	LastEvent string `json:"last_event,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Preview   string `json:"preview,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type CodersSnapshot struct {
	Status    string          `json:"status"`
	UpdatedAt string          `json:"updated_at,omitempty"`
	Items     []AgentSnapshot `json:"items"`
}

type AgentSnapshot struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	State      string `json:"state"`
	Route      string `json:"route,omitempty"`
	JobID      string `json:"job_id,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	LastEvent  string `json:"last_event,omitempty"`
	Preview    string `json:"preview,omitempty"`
	Reason     string `json:"reason,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
	EventCount int    `json:"event_count,omitempty"`
}

type JobSnapshot struct {
	JobID           string                           `json:"job_id"`
	Route           string                           `json:"route,omitempty"`
	Phase           string                           `json:"phase"`
	Owner           string                           `json:"owner,omitempty"`
	Status          string                           `json:"status"`
	SessionID       string                           `json:"session_id,omitempty"`
	Channel         string                           `json:"channel,omitempty"`
	ChatID          string                           `json:"chat_id,omitempty"`
	StartedAt       string                           `json:"started_at,omitempty"`
	UpdatedAt       string                           `json:"updated_at,omitempty"`
	Summary         string                           `json:"summary,omitempty"`
	FailureKind     string                           `json:"failure_kind,omitempty"`
	FailureReason   string                           `json:"failure_reason,omitempty"`
	FinalUserReport string                           `json:"final_user_report,omitempty"`
	MioReported     bool                             `json:"mio_reported"`
	Events          []orchestrator.OrchestratorEvent `json:"events,omitempty"`
}

type JobFilter struct {
	Route     string
	Status    string
	Owner     string
	SessionID string
	ChatID    string
	Limit     int
}

type LogFilter struct {
	Type      string
	Agent     string
	Route     string
	JobID     string
	SessionID string
	ChatID    string
	Limit     int
}

type AgentDetail struct {
	Agent      AgentSnapshot                    `json:"agent"`
	ActiveJobs []JobSnapshot                    `json:"active_jobs"`
	Events     []orchestrator.OrchestratorEvent `json:"events"`
}

type AuditSummary struct {
	StoredLogs int            `json:"stored_logs"`
	ByType     map[string]int `json:"by_type"`
	ByAgent    map[string]int `json:"by_agent"`
	ByRoute    map[string]int `json:"by_route"`
}

type JobDetail struct {
	Item     JobSnapshot                      `json:"item"`
	Evidence *domainexecution.ExecutionReport `json:"evidence,omitempty"`
}

func NewMonitorStore(evidence EvidenceLister, archive EventLogReader) *MonitorStore {
	s := &MonitorStore{
		agents:   make(map[string]AgentSnapshot, len(monitorAgents)),
		jobs:     make(map[string]*JobSnapshot),
		evidence: evidence,
		archive:  archive,
	}
	for _, id := range monitorAgents {
		s.agents[id] = AgentSnapshot{
			ID:    id,
			Role:  agentRole(id),
			State: "offline",
		}
	}
	return s
}

func (s *MonitorStore) OnEvent(ev orchestrator.OrchestratorEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logs = append(s.logs, ev)
	if len(s.logs) > monitorMaxLogs {
		s.logs = s.logs[len(s.logs)-monitorMaxLogs:]
	}

	s.reduceAgents(ev)
	s.reduceJobs(ev)
}

func (s *MonitorStore) Status() StatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	chat := s.agentSnapshotLocked("mio", now)
	worker := s.agentSnapshotLocked("shiro", now)
	coders := s.coderSnapshotsLocked(now)
	updatedAtValues := []string{chat.UpdatedAt, worker.UpdatedAt}
	for _, coder := range coders {
		updatedAtValues = append(updatedAtValues, coder.UpdatedAt)
	}
	return StatusSnapshot{
		UpdatedAt: latestUpdatedAt(updatedAtValues...),
		Chat:      componentFromAgent(chat),
		Worker:    componentFromAgent(worker),
		Coders: CodersSnapshot{
			Status:    summarizeCoderState(coders),
			UpdatedAt: latestUpdatedAt(agentUpdatedAtValues(coders)...),
			Items:     coders,
		},
	}
}

func (s *MonitorStore) SetAgentUnavailable(id, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	s.patchAgent(id, AgentSnapshot{
		State:     "unavailable",
		LastEvent: "agent.unavailable",
		Preview:   shortText(reason, 120),
		Reason:    shortText(reason, 160),
		UpdatedAt: now,
	})
}

func (s *MonitorStore) Agents() []AgentSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	items := make([]AgentSnapshot, 0, len(monitorAgents))
	for _, id := range monitorAgents {
		items = append(items, s.agentSnapshotLocked(id, now))
	}
	return items
}

func (s *MonitorStore) coderSnapshotsLocked(now time.Time) []AgentSnapshot {
	items := make([]AgentSnapshot, 0, len(monitorAgents))
	for _, id := range monitorAgents {
		if strings.HasPrefix(id, "coder") {
			items = append(items, s.agentSnapshotLocked(id, now))
		}
	}
	return items
}

func (s *MonitorStore) AgentDetail(ctx context.Context, id string, limit int) (AgentDetail, bool) {
	s.mu.RLock()
	now := time.Now()
	_, ok := s.agents[id]
	if !ok {
		s.mu.RUnlock()
		return AgentDetail{}, false
	}
	agent := s.agentSnapshotLocked(id, now)
	jobs := make([]JobSnapshot, 0, 4)
	for _, job := range s.jobs {
		if strings.EqualFold(job.Owner, id) || (agent.JobID != "" && strings.EqualFold(job.JobID, agent.JobID)) {
			jobs = append(jobs, *job)
		}
	}
	s.mu.RUnlock()

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].UpdatedAt > jobs[j].UpdatedAt
	})
	if limit > 0 && len(jobs) > limit {
		jobs = jobs[:limit]
	}

	events, err := s.ArchivedLogs(ctx, LogFilter{Agent: id, Limit: limit})
	if err != nil || len(events) == 0 {
		events = s.Logs(LogFilter{Agent: id, Limit: limit})
	}
	return AgentDetail{Agent: agent, ActiveJobs: jobs, Events: events}, true
}

func (s *MonitorStore) Jobs(filter JobFilter) []JobSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]JobSnapshot, 0, len(s.jobs))
	for _, job := range s.jobs {
		if filter.Route != "" && !strings.EqualFold(job.Route, filter.Route) {
			continue
		}
		if filter.Status != "" && !strings.EqualFold(job.Status, filter.Status) {
			continue
		}
		if filter.Owner != "" && !strings.EqualFold(job.Owner, filter.Owner) {
			continue
		}
		if filter.SessionID != "" && !strings.EqualFold(job.SessionID, filter.SessionID) {
			continue
		}
		if filter.ChatID != "" && !strings.EqualFold(job.ChatID, filter.ChatID) {
			continue
		}
		items = append(items, *job)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items
}

func (s *MonitorStore) Logs(filter LogFilter) []orchestrator.OrchestratorEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]orchestrator.OrchestratorEvent, 0, len(s.logs))
	for i := len(s.logs) - 1; i >= 0; i-- {
		ev := s.logs[i]
		if !matchesLogFilter(ev, filter) {
			continue
		}
		items = append(items, ev)
		if filter.Limit > 0 && len(items) >= filter.Limit {
			break
		}
	}
	return items
}

func (s *MonitorStore) ArchivedLogs(ctx context.Context, filter LogFilter) ([]orchestrator.OrchestratorEvent, error) {
	if s.archive == nil {
		return nil, nil
	}
	return s.archive.Query(ctx, filter)
}

func (s *MonitorStore) JobDetail(ctx context.Context, jobID string) (JobDetail, bool) {
	s.mu.RLock()
	job, ok := s.jobs[jobID]
	if !ok {
		s.mu.RUnlock()
		return JobDetail{}, false
	}
	item := *job
	s.mu.RUnlock()

	if events, err := s.ArchivedLogs(ctx, LogFilter{JobID: jobID, Limit: monitorMaxJobEvents}); err == nil && len(events) > 0 {
		item.Events = events
	}

	var evidence *domainexecution.ExecutionReport
	if s.evidence != nil {
		if ev, err := s.evidence.GetByJobID(ctx, jobID); err == nil {
			evidence = &ev
		}
	}
	return JobDetail{Item: item, Evidence: evidence}, true
}

func (s *MonitorStore) Summary() AuditSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := AuditSummary{
		StoredLogs: len(s.logs),
		ByType:     map[string]int{},
		ByAgent:    map[string]int{},
		ByRoute:    map[string]int{},
	}
	for _, ev := range s.logs {
		if ev.Type != "" {
			out.ByType[ev.Type]++
		}
		if ev.From != "" {
			out.ByAgent[strings.ToLower(ev.From)]++
		}
		if ev.Route != "" {
			out.ByRoute[strings.ToUpper(ev.Route)]++
		}
	}
	return out
}

func (s *MonitorStore) agentSnapshotLocked(id string, now time.Time) AgentSnapshot {
	agent := s.agents[id]
	if agent.UpdatedAt == "" {
		return agent
	}
	if agent.State == "unavailable" {
		return agent
	}
	ts, err := time.Parse(time.RFC3339, agent.UpdatedAt)
	if err == nil && now.Sub(ts) > monitorOfflineAfter {
		agent.State = "offline"
	}
	return agent
}

func (s *MonitorStore) reduceAgents(ev orchestrator.OrchestratorEvent) {
	ts := ev.Timestamp
	route := ev.Route
	jid := ev.JobID

	if ev.Type == "agent.unavailable" {
		s.patchAgent(strings.ToLower(strings.TrimSpace(ev.From)), AgentSnapshot{
			State:     "unavailable",
			LastEvent: ev.Type,
			Preview:   shortText(ev.Content, 80),
			Reason:    shortText(ev.Content, 160),
			UpdatedAt: ts,
		})
		return
	}

	if ev.Type == "message.received" || ev.Type == "routing.decision" {
		s.patchAgent("mio", AgentSnapshot{
			State:     "running",
			Route:     route,
			JobID:     jid,
			SessionID: ev.SessionID,
			LastEvent: ev.Type,
			Preview:   shortText(ev.Content, 80),
			UpdatedAt: ts,
		})
		return
	}

	from := strings.ToLower(strings.TrimSpace(ev.From))
	to := strings.ToLower(strings.TrimSpace(ev.To))
	if isMonitorAgent(from) {
		state := "running"
		switch ev.Type {
		case "agent.thinking", "agent.waiting":
			state = "thinking"
		case "agent.response":
			lower := strings.ToLower(ev.Content)
			if strings.Contains(lower, "error") || strings.Contains(lower, "失敗") {
				state = "error"
			} else {
				state = "idle"
			}
		case "agent.error", "mailbox.error":
			state = "error"
		}
		s.patchAgent(from, AgentSnapshot{
			State:     state,
			Route:     route,
			JobID:     jid,
			SessionID: ev.SessionID,
			LastEvent: ev.Type,
			Preview:   shortText(ev.Content, 80),
			Reason:    "",
			UpdatedAt: ts,
		})
	}
	if (ev.Type == "agent.start" || ev.Type == "agent.dispatch" || ev.Type == "mailbox.sent") && isMonitorAgent(to) {
		s.patchAgent(to, AgentSnapshot{
			State:     "running",
			Route:     route,
			JobID:     jid,
			SessionID: ev.SessionID,
			LastEvent: ev.Type,
			Preview:   shortText(ev.Content, 80),
			Reason:    "",
			UpdatedAt: ts,
		})
	}
	if ev.Type == "agent.response" && to == "mio" {
		s.patchAgent("mio", AgentSnapshot{
			State:     "idle",
			Route:     route,
			JobID:     jid,
			SessionID: ev.SessionID,
			LastEvent: ev.Type,
			Preview:   shortText(ev.Content, 80),
			Reason:    "",
			UpdatedAt: ts,
		})
	}
}

func (s *MonitorStore) reduceJobs(ev orchestrator.OrchestratorEvent) {
	jid := strings.TrimSpace(ev.JobID)
	if jid == "" {
		return
	}
	job := s.jobs[jid]
	if job == nil {
		job = &JobSnapshot{
			JobID:     jid,
			Route:     valueOr(ev.Route, "-"),
			Phase:     "received",
			Owner:     "mio",
			Status:    "running",
			SessionID: ev.SessionID,
			Channel:   ev.Channel,
			ChatID:    ev.ChatID,
			StartedAt: ev.Timestamp,
			UpdatedAt: ev.Timestamp,
		}
		s.jobs[jid] = job
	}
	job.UpdatedAt = ev.Timestamp
	if ev.Route != "" {
		job.Route = ev.Route
	}
	if ev.SessionID != "" {
		job.SessionID = ev.SessionID
	}
	if ev.Channel != "" {
		job.Channel = ev.Channel
	}
	if ev.ChatID != "" {
		job.ChatID = ev.ChatID
	}
	if ev.Content != "" {
		job.Summary = shortText(ev.Content, 160)
	}
	job.Phase, job.Owner = classifyJobPhase(ev, job)
	if ev.Type == "worker.classified_failure" || ev.Type == "agent.error" || ev.Type == "mailbox.error" {
		raw := strings.TrimSpace(ev.Content)
		if idx := strings.Index(raw, ":"); idx >= 0 {
			job.FailureKind = strings.TrimSpace(raw[:idx])
			job.FailureReason = strings.TrimSpace(raw[idx+1:])
		} else {
			job.FailureReason = raw
		}
		job.Status = "error"
	}
	if clearsJobFailure(ev) {
		job.FailureKind = ""
		job.FailureReason = ""
		if job.Status == "error" {
			job.Status = "running"
		}
	}
	if ev.Type == "agent.response" {
		if strings.EqualFold(ev.From, "mio") && strings.EqualFold(ev.To, "user") {
			job.FinalUserReport = ev.Content
			job.MioReported = true
			if responseLooksLikeFailure(ev.Content) {
				job.Status = "error"
			} else {
				job.FailureKind = ""
				job.FailureReason = ""
				job.Status = "done"
			}
		} else if job.Status != "error" {
			job.Status = "running"
		}
	}
	job.Events = append(job.Events, ev)
	if len(job.Events) > monitorMaxJobEvents {
		job.Events = job.Events[len(job.Events)-monitorMaxJobEvents:]
	}
}

func clearsJobFailure(ev orchestrator.OrchestratorEvent) bool {
	from := strings.ToLower(strings.TrimSpace(ev.From))
	to := strings.ToLower(strings.TrimSpace(ev.To))
	switch ev.Type {
	case "mailbox.received":
		return strings.Contains(strings.ToLower(ev.Content), "type=result")
	case "agent.response":
		if from == "mio" && to == "user" {
			return !responseLooksLikeFailure(ev.Content)
		}
		return (strings.HasPrefix(from, "coder") && to == "shiro") || (from == "shiro" && to == "mio")
	default:
		return false
	}
}

func responseLooksLikeFailure(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "失敗: 0") || strings.Contains(lower, "failures: 0") || strings.Contains(lower, "failed: 0") {
		return false
	}
	return strings.Contains(lower, "error") || strings.Contains(lower, "失敗")
}

func (s *MonitorStore) patchAgent(id string, patch AgentSnapshot) {
	cur, ok := s.agents[id]
	if !ok {
		cur = AgentSnapshot{ID: id, Role: agentRole(id), State: "offline"}
	}
	if patch.State != "" {
		cur.State = patch.State
	}
	if patch.Route != "" {
		cur.Route = patch.Route
	}
	if patch.JobID != "" {
		cur.JobID = patch.JobID
	}
	if patch.SessionID != "" {
		cur.SessionID = patch.SessionID
	}
	if patch.LastEvent != "" {
		cur.LastEvent = patch.LastEvent
	}
	if patch.Preview != "" {
		cur.Preview = patch.Preview
	}
	if patch.Reason != "" || cur.State != patch.State {
		cur.Reason = patch.Reason
	}
	if patch.UpdatedAt != "" {
		cur.UpdatedAt = patch.UpdatedAt
	}
	cur.EventCount++
	s.agents[id] = cur
}

func classifyJobPhase(ev orchestrator.OrchestratorEvent, current *JobSnapshot) (string, string) {
	from := strings.ToLower(strings.TrimSpace(ev.From))
	to := strings.ToLower(strings.TrimSpace(ev.To))
	content := ev.Content
	switch ev.Type {
	case "message.received":
		return "received", "mio"
	case "routing.decision":
		return "routing", valueOr(current.Owner, "mio")
	case "agent.dispatch":
		return "delegating", valueOr(to, current.Owner)
	case "agent.thinking":
		return "chatting", valueOr(from, current.Owner)
	case "agent.waiting":
		return "waiting", valueOr(from, current.Owner)
	case "worker.retry_request":
		return "retrying", valueOr(to, "coder1")
	case "worker.classified_failure", "agent.error", "mailbox.error":
		return "error", valueOr(from, current.Owner)
	case "mailbox.sent":
		return "queued", valueOr(to, current.Owner)
	case "mailbox.received":
		return "processing", valueOr(from, current.Owner)
	case "agent.start":
		if to == "shiro" {
			if strings.Contains(content, "Worker実行") || strings.Contains(content, "Patch") || strings.Contains(content, "整形") {
				return "worker_verifying", "shiro"
			}
			return "delegated_to_worker", "shiro"
		}
		if strings.HasPrefix(to, "coder") {
			return "delegated_to_coder", to
		}
		if to == "mio" {
			return "reporting", "mio"
		}
	case "agent.response":
		if from == "mio" && to == "user" {
			if responseLooksLikeFailure(content) {
				return "error", "mio"
			}
			return "done", "mio"
		}
		if from == "shiro" && to == "mio" {
			return "reporting", "mio"
		}
		if strings.HasPrefix(from, "coder") && to == "shiro" {
			return "worker_verifying", "shiro"
		}
	}
	return valueOr(current.Phase, "received"), valueOr(current.Owner, "-")
}

func summarizeCoderState(items []AgentSnapshot) string {
	status := "idle"
	for _, item := range items {
		switch item.State {
		case "error":
			return "error"
		case "unavailable":
			if status != "running" {
				status = "degraded"
			}
		case "thinking", "running":
			status = "running"
		case "offline":
			if status == "idle" {
				status = "offline"
			}
		}
	}
	return status
}

func componentFromAgent(agent AgentSnapshot) ComponentSnapshot {
	return ComponentSnapshot{
		Status:    agent.State,
		AgentID:   agent.ID,
		JobID:     agent.JobID,
		Route:     agent.Route,
		LastEvent: agent.LastEvent,
		UpdatedAt: agent.UpdatedAt,
		Preview:   agent.Preview,
		Reason:    agent.Reason,
	}
}

func latestUpdatedAt(values ...string) string {
	best := ""
	for _, v := range values {
		if v > best {
			best = v
		}
	}
	return best
}

func agentUpdatedAtValues(items []AgentSnapshot) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, item.UpdatedAt)
	}
	return values
}

func shortText(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 || len(s) <= limit {
		return s
	}
	if limit <= 3 {
		return s[:limit]
	}
	return s[:limit-3] + "..."
}

func isMonitorAgent(id string) bool {
	for _, item := range monitorAgents {
		if item == id {
			return true
		}
	}
	return false
}

func agentRole(id string) string {
	switch id {
	case "mio":
		return "chat"
	case "shiro":
		return "worker"
	default:
		if strings.HasPrefix(id, "coder") {
			return "coder"
		}
		return "agent"
	}
}

func valueOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (d JobDetail) MarshalJSON() ([]byte, error) {
	type alias JobDetail
	if d.Item.Events == nil {
		d.Item.Events = []orchestrator.OrchestratorEvent{}
	}
	return json.Marshal(alias(d))
}
