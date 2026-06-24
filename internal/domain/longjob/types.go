package longjob

import (
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusBlocked   Status = "blocked"
	StatusCompleted Status = "completed"
	StatusCanceled  Status = "canceled"
	StatusFailed    Status = "failed"
)

type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepBlocked   StepStatus = "blocked"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

type Job struct {
	ID            string            `json:"id"`
	Kind          string            `json:"kind"`
	Goal          string            `json:"goal"`
	Status        Status            `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	StartedAt     *time.Time        `json:"started_at,omitempty"`
	FinishedAt    *time.Time        `json:"finished_at,omitempty"`
	ResumePoint   string            `json:"resume_point,omitempty"`
	Params        map[string]string `json:"params,omitempty"`
	Plan          []Step            `json:"plan"`
	SharedContext []ContextEntry    `json:"shared_context,omitempty"`
	Artifacts     []Artifact        `json:"artifacts,omitempty"`
	ReviewGates   []ReviewGate      `json:"review_gates,omitempty"`
}

type Step struct {
	ID          string     `json:"id"`
	Role        string     `json:"role"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      StepStatus `json:"status"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Summary     string     `json:"summary,omitempty"`
	ArtifactIDs []string   `json:"artifact_ids,omitempty"`
}

type ContextEntry struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Artifact struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Path      string    `json:"path"`
	Summary   string    `json:"summary,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ReviewGate struct {
	ID        string     `json:"id"`
	Reviewer  string     `json:"reviewer"`
	Status    StepStatus `json:"status"`
	Summary   string     `json:"summary,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (j Job) Validate() error {
	if strings.TrimSpace(j.ID) == "" {
		return fmt.Errorf("job id is required")
	}
	if strings.TrimSpace(j.Kind) == "" {
		return fmt.Errorf("job kind is required")
	}
	if strings.TrimSpace(j.Goal) == "" {
		return fmt.Errorf("job goal is required")
	}
	if j.Status == "" {
		return fmt.Errorf("job status is required")
	}
	if len(j.Plan) == 0 {
		return fmt.Errorf("job plan is required")
	}
	return nil
}

func (j Job) NextStepIndex() int {
	for i, step := range j.Plan {
		if step.Status == StepRunning || step.Status == StepBlocked || step.Status == StepFailed {
			return i
		}
	}
	for i, step := range j.Plan {
		if step.Status == StepPending {
			return i
		}
	}
	return -1
}

func (j Job) CompletedStepCount() int {
	count := 0
	for _, step := range j.Plan {
		if step.Status == StepCompleted || step.Status == StepSkipped {
			count++
		}
	}
	return count
}

func (j Job) IsTerminal() bool {
	return j.Status == StatusCompleted || j.Status == StatusCanceled || j.Status == StatusFailed
}
