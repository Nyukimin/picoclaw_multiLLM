package execution

import (
	"context"
	"fmt"
	"sync"
	"time"

	domain "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
)

// PolicyEvaluator は実行ポリシー判定I/F
type PolicyEvaluator interface {
	Evaluate(action domain.Action) domain.PolicyDecision
}

// ToolExecutor は実ツール実行I/F
type ToolExecutor interface {
	ExecuteV2(ctx context.Context, toolName string, args map[string]any) (*tool.ToolResponse, error)
}

// Service は RequestToolExecution / ApproveExecution を提供する
// ask 判定時は in-memory キューに保持する（再起動永続化は今後拡張）
type Service struct {
	policy      PolicyEvaluator
	executor    ToolExecutor
	repo        domain.Repository
	approvalTTL time.Duration
	now         func() time.Time

	mu      sync.Mutex
	pending map[string]domain.Action
}

// Result は実行結果
type Result struct {
	Record   domain.Record
	Response *tool.ToolResponse
}

func NewService(policy PolicyEvaluator, executor ToolExecutor, repo domain.Repository, approvalTTL time.Duration) *Service {
	if approvalTTL <= 0 {
		approvalTTL = 10 * time.Minute
	}
	if repo == nil {
		repo = &noopRepository{}
	}
	return &Service{
		policy:      policy,
		executor:    executor,
		repo:        repo,
		approvalTTL: approvalTTL,
		now:         time.Now,
		pending:     make(map[string]domain.Action),
	}
}

func (s *Service) RequestToolExecution(ctx context.Context, action domain.Action) (*Result, error) {
	started := s.now().UTC()
	decision := s.policy.Evaluate(action)

	rec := domain.Record{
		JobID:       action.JobID,
		ActionID:    action.ActionID,
		Tool:        action.Tool,
		RequestedBy: action.RequestedBy,
		Arguments:   action.Arguments,
		EventType:   eventTypeFromDecision(decision.Decision),
		Decision:    decision.Decision,
		Reason:      decision.Reason,
		TraceID:     action.JobID + ":" + action.ActionID,
		StartedAt:   started,
	}

	switch decision.Decision {
	case domain.DecisionDeny:
		now := s.now().UTC()
		rec.Status = domain.StatusDenied
		rec.FinishedAt = &now
		if err := s.repo.Create(ctx, rec); err != nil {
			return nil, err
		}
		return &Result{Record: rec}, nil
	case domain.DecisionAsk:
		rec.Status = domain.StatusWaitingApproval
		if err := s.repo.Create(ctx, rec); err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.pending[pendingKey(action.JobID, action.ActionID)] = action
		s.mu.Unlock()
		return &Result{Record: rec}, nil
	default:
		rec.Status = domain.StatusRunning
		if err := s.repo.Create(ctx, rec); err != nil {
			return nil, err
		}
		resp, err := s.executor.ExecuteV2(ctx, action.Tool, action.Arguments)
		if err != nil {
			failed, uerr := s.repo.UpdateStatus(ctx, action.JobID, action.ActionID, domain.StatusFailed, err.Error())
			if uerr != nil {
				return nil, uerr
			}
			return &Result{Record: failed}, nil
		}
		if resp != nil && resp.Error != nil {
			failed, uerr := s.repo.UpdateStatus(ctx, action.JobID, action.ActionID, domain.StatusFailed, resp.Error.Message)
			if uerr != nil {
				return nil, uerr
			}
			return &Result{Record: failed, Response: resp}, nil
		}
		success, err := s.repo.UpdateStatus(ctx, action.JobID, action.ActionID, domain.StatusSucceeded, "")
		if err != nil {
			return nil, err
		}
		return &Result{Record: success, Response: resp}, nil
	}
}

func (s *Service) ApproveExecution(ctx context.Context, jobID, actionID string) (*Result, error) {
	key := pendingKey(jobID, actionID)
	s.mu.Lock()
	action, ok := s.pending[key]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("pending action not found: %s", key)
	}

	requestedAt := action.RequestedAt
	if requestedAt.IsZero() {
		requestedAt = s.now().UTC()
	}
	if s.now().After(requestedAt.Add(s.approvalTTL)) {
		denied, err := s.repo.UpdateStatus(ctx, jobID, actionID, domain.StatusDenied, "approval ttl exceeded")
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		delete(s.pending, key)
		s.mu.Unlock()
		return &Result{Record: denied}, nil
	}

	running, err := s.repo.UpdateStatus(ctx, jobID, actionID, domain.StatusRunning, "")
	if err != nil {
		return nil, err
	}

	resp, execErr := s.executor.ExecuteV2(ctx, action.Tool, action.Arguments)
	s.mu.Lock()
	delete(s.pending, key)
	s.mu.Unlock()
	if execErr != nil {
		failed, err := s.repo.UpdateStatus(ctx, jobID, actionID, domain.StatusFailed, execErr.Error())
		if err != nil {
			return nil, err
		}
		return &Result{Record: failed}, nil
	}
	if resp != nil && resp.Error != nil {
		failed, err := s.repo.UpdateStatus(ctx, jobID, actionID, domain.StatusFailed, resp.Error.Message)
		if err != nil {
			return nil, err
		}
		return &Result{Record: failed, Response: resp}, nil
	}
	_ = running
	success, err := s.repo.UpdateStatus(ctx, jobID, actionID, domain.StatusSucceeded, "")
	if err != nil {
		return nil, err
	}
	return &Result{Record: success, Response: resp}, nil
}

func pendingKey(jobID, actionID string) string {
	return jobID + "::" + actionID
}

func eventTypeFromDecision(d domain.Decision) string {
	if d == domain.DecisionDeny {
		return "security.violation"
	}
	return "security.decision"
}

type noopRepository struct{}

func (n *noopRepository) Create(context.Context, domain.Record) error {
	return nil
}

func (n *noopRepository) UpdateStatus(_ context.Context, jobID, actionID string, status domain.Status, errMsg string) (domain.Record, error) {
	now := time.Now().UTC()
	rec := domain.Record{JobID: jobID, ActionID: actionID, Status: status, Error: errMsg, StartedAt: now}
	if status.IsTerminal() {
		rec.FinishedAt = &now
	}
	return rec, nil
}

func (n *noopRepository) Get(context.Context, string, string) (domain.Record, error) {
	return domain.Record{}, fmt.Errorf("record not found")
}

func (n *noopRepository) ListPendingApprovals(context.Context, int) ([]domain.Record, error) {
	return nil, nil
}

func (n *noopRepository) CountByStatus(context.Context) (map[domain.Status]int, error) {
	return map[domain.Status]int{}, nil
}
