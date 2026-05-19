package rencrowclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func New(baseURL string, opts ...Option) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

type SuperAgentStatus struct {
	AgentRuns       []AgentRun       `json:"agent_runs"`
	SubagentTasks   []SubagentTask   `json:"subagent_tasks"`
	ContextPacks    []ContextPack    `json:"context_packs"`
	MessageChannels []MessageChannel `json:"message_channels"`
	TraceEvents     []TraceEvent     `json:"trace_events"`
}

type AgentRun struct {
	RunID        string    `json:"run_id"`
	WorkstreamID string    `json:"workstream_id,omitempty"`
	ParentRunID  string    `json:"parent_run_id,omitempty"`
	AgentType    string    `json:"agent_type"`
	Goal         string    `json:"goal,omitempty"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"started_at"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
	Summary      string    `json:"summary,omitempty"`
}

type SubagentTask struct {
	SubagentID           string    `json:"subagent_id"`
	ParentRunID          string    `json:"parent_run_id"`
	AgentType            string    `json:"agent_type"`
	Task                 string    `json:"task"`
	Scope                []string  `json:"scope"`
	Tools                []string  `json:"tools,omitempty"`
	TerminationCondition string    `json:"termination_condition"`
	OutputPath           string    `json:"output_path,omitempty"`
	Status               string    `json:"status"`
	CreatedAt            time.Time `json:"created_at"`
	CompletedAt          time.Time `json:"completed_at,omitempty"`
}

type ContextPack struct {
	ContextPackID   string    `json:"context_pack_id"`
	RunID           string    `json:"run_id"`
	WorkstreamID    string    `json:"workstream_id,omitempty"`
	Summary         string    `json:"summary"`
	IncludedSources []string  `json:"included_sources,omitempty"`
	TokenEstimate   int       `json:"token_estimate,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type MessageChannel struct {
	ChannelID      string    `json:"channel_id"`
	ChannelType    string    `json:"channel_type"`
	Name           string    `json:"name,omitempty"`
	AuthScope      string    `json:"auth_scope,omitempty"`
	AllowedActions []string  `json:"allowed_actions,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type TraceEvent struct {
	EventID        string    `json:"event_id"`
	ParentEventID  string    `json:"parent_event_id,omitempty"`
	RunID          string    `json:"run_id,omitempty"`
	EventType      string    `json:"event_type"`
	Actor          string    `json:"actor,omitempty"`
	PayloadSummary string    `json:"payload_summary,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type CommandRunRequest struct {
	CommandName  string `json:"command_name"`
	WorkstreamID string `json:"workstream_id,omitempty"`
	Agent        string `json:"agent,omitempty"`
	Input        string `json:"input,omitempty"`
}

type CommandRunResponse struct {
	EventID      string `json:"event_id"`
	SkillEventID string `json:"skill_event_id,omitempty"`
	CommandName  string `json:"command_name"`
	Status       string `json:"status"`
}

type RunStateRequest struct {
	RunID  string `json:"run_id"`
	Reason string `json:"reason,omitempty"`
}

type RunStateResponse struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	EventID string `json:"event_id"`
}

type ExternalControlRequest struct {
	Actor         string `json:"actor"`
	ChannelID     string `json:"channel_id"`
	Action        string `json:"action"`
	HumanApproved bool   `json:"human_approved"`
}

type ExternalControlDecision struct {
	Status           string   `json:"status"`
	RequiresApproval bool     `json:"requires_approval"`
	Reasons          []string `json:"reasons,omitempty"`
}

type ExternalControlResponse struct {
	Request  ExternalControlRequest  `json:"request"`
	Decision ExternalControlDecision `json:"decision"`
}

type WorkstreamArtifact struct {
	ArtifactID   string    `json:"artifact_id"`
	WorkstreamID string    `json:"workstream_id"`
	Type         string    `json:"artifact_type"`
	FilePath     string    `json:"file_path,omitempty"`
	Title        string    `json:"title,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

type WorkstreamArtifactResponse struct {
	Artifact WorkstreamArtifact `json:"artifact"`
}

type WorkstreamVaultUpdate struct {
	UpdateID          string    `json:"update_id"`
	WorkstreamID      string    `json:"workstream_id"`
	FilePath          string    `json:"file_path"`
	UpdateType        string    `json:"update_type,omitempty"`
	ProposedContent   string    `json:"proposed_content,omitempty"`
	ContentHashBefore string    `json:"content_hash_before,omitempty"`
	ContentHashAfter  string    `json:"content_hash_after,omitempty"`
	ReviewStatus      string    `json:"review_status"`
	CreatedAt         time.Time `json:"created_at"`
}

type WorkstreamVaultUpdateResponse struct {
	VaultUpdate WorkstreamVaultUpdate `json:"vault_update"`
	Applied     bool                  `json:"applied,omitempty"`
	AppliedPath string                `json:"applied_path,omitempty"`
}

type RevenueHumanDecision struct {
	DecisionID     string    `json:"decision_id,omitempty"`
	DecisionType   string    `json:"decision_type"`
	SubjectID      string    `json:"subject_id,omitempty"`
	Description    string    `json:"description,omitempty"`
	ApprovalStatus string    `json:"approval_status,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
}

type RevenueHumanDecisionResult struct {
	Status           string   `json:"status"`
	RequiresApproval bool     `json:"requires_approval"`
	Reasons          []string `json:"reasons,omitempty"`
}

type RevenueHumanDecisionRecord struct {
	DecisionID       string    `json:"decision_id"`
	DecisionType     string    `json:"decision_type"`
	SubjectID        string    `json:"subject_id,omitempty"`
	Description      string    `json:"description,omitempty"`
	ApprovalStatus   string    `json:"approval_status"`
	GateStatus       string    `json:"gate_status"`
	RequiresApproval bool      `json:"requires_approval"`
	Reasons          []string  `json:"reasons,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type RevenueHumanDecisionResponse struct {
	Decision RevenueHumanDecision       `json:"decision"`
	Record   RevenueHumanDecisionRecord `json:"record"`
	Result   RevenueHumanDecisionResult `json:"result"`
}

type RevenueHumanDecisionReview struct {
	DecisionID     string `json:"decision_id"`
	ApprovalStatus string `json:"approval_status"`
}

type RevenueDailyRoutineRequest struct {
	ReportID     string `json:"report_id,omitempty"`
	WorkstreamID string `json:"workstream_id,omitempty"`
	Date         string `json:"date,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

type RevenueDailyRoutineReport struct {
	ReportID            string    `json:"report_id"`
	WorkstreamID        string    `json:"workstream_id,omitempty"`
	Date                string    `json:"date"`
	Summary             string    `json:"summary,omitempty"`
	MarketResearch      int       `json:"market_research_count"`
	SNSPosts            int       `json:"sns_post_count"`
	Products            int       `json:"product_count"`
	CustomerVoices      int       `json:"customer_voice_count"`
	RevenueEvents       int       `json:"revenue_event_count"`
	PaidCustomers       int       `json:"paid_customer_count"`
	PendingDecisions    int       `json:"pending_decision_count"`
	SuggestedActions    []string  `json:"suggested_actions,omitempty"`
	Status              string    `json:"status"`
	ExternalSendApplied bool      `json:"external_send_applied"`
	CreatedAt           time.Time `json:"created_at"`
}

type RevenueDailyRoutineResponse struct {
	Report                                  RevenueDailyRoutineReport `json:"daily_routine_report"`
	ExternalActionsApplied                  bool                      `json:"external_actions_applied"`
	HumanApprovalRequiredForExternalActions bool                      `json:"human_approval_required_for_external_actions"`
}

type SandboxStatus struct {
	Sandboxes  []SandboxRecord         `json:"sandboxes"`
	Artifacts  []SandboxArtifact       `json:"artifacts"`
	Promotions []PromotionRequest      `json:"promotions"`
	Decisions  []PromotionGateDecision `json:"decisions"`
	GateLogs   []PromotionGateLog      `json:"gate_logs"`
}

type SandboxRecord struct {
	SandboxID    string    `json:"sandbox_id"`
	WorkstreamID string    `json:"workstream_id,omitempty"`
	GoalID       string    `json:"goal_id,omitempty"`
	Type         string    `json:"type"`
	Path         string    `json:"path"`
	BaseRef      string    `json:"base_ref,omitempty"`
	CreatedBy    string    `json:"created_by,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	ClosedAt     time.Time `json:"closed_at,omitempty"`
}

type SandboxArtifact struct {
	ArtifactID string    `json:"artifact_id"`
	SandboxID  string    `json:"sandbox_id"`
	Type       string    `json:"artifact_type"`
	FilePath   string    `json:"file_path"`
	Title      string    `json:"title,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type PromotionRequest struct {
	PromotionID               string    `json:"promotion_id"`
	SandboxID                 string    `json:"sandbox_id"`
	WorkstreamID              string    `json:"workstream_id,omitempty"`
	GoalID                    string    `json:"goal_id,omitempty"`
	RequestedBy               string    `json:"requested_by,omitempty"`
	TargetPath                string    `json:"target_path"`
	DiffPath                  string    `json:"diff_path"`
	TestResultPath            string    `json:"test_result_path"`
	RiskLevel                 string    `json:"risk_level,omitempty"`
	Reason                    string    `json:"reason"`
	RollbackPlanPath          string    `json:"rollback_plan_path"`
	PostApplyVerificationPath string    `json:"post_apply_verification_path,omitempty"`
	HumanApprovalStatus       string    `json:"human_approval_status"`
	CreatedAt                 time.Time `json:"created_at"`
}

type PromotionGateDecision struct {
	Status              string   `json:"status"`
	Reason              string   `json:"reason"`
	MissingRequirements []string `json:"missing_requirements,omitempty"`
}

type PromotionGateLog struct {
	EventID               string    `json:"event_id"`
	PromotionID           string    `json:"promotion_id"`
	GateStatus            string    `json:"gate_status"`
	Reason                string    `json:"reason"`
	HumanApprovalStatus   string    `json:"human_approval_status"`
	PostApplyVerification string    `json:"post_apply_verification,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
}

type PromotionRequestResponse struct {
	Promotion                     PromotionRequest      `json:"promotion"`
	Decision                      PromotionGateDecision `json:"decision"`
	GateLog                       PromotionGateLog      `json:"gate_log"`
	RollbackArtifact              *SandboxArtifact      `json:"rollback_artifact,omitempty"`
	PostApplyVerificationArtifact *SandboxArtifact      `json:"post_apply_verification_artifact,omitempty"`
}

type PromotionApplyRequest struct {
	Promotion                    PromotionRequest `json:"promotion"`
	AppliedBy                    string           `json:"applied_by,omitempty"`
	ApplyTarget                  string           `json:"apply_target,omitempty"`
	PostApplyVerificationPath    string           `json:"post_apply_verification_path"`
	PostApplyVerificationCommand string           `json:"post_apply_verification_command,omitempty"`
	HumanApproved                bool             `json:"human_approved"`
}

type PromotionDiffApplyResult struct {
	DiffPath     string   `json:"diff_path"`
	ApplyRoot    string   `json:"apply_root"`
	AppliedFiles []string `json:"applied_files"`
	Status       string   `json:"status"`
}

type PromotionApplyResponse struct {
	Decision                      PromotionGateDecision     `json:"decision"`
	DiffApplyResult               *PromotionDiffApplyResult `json:"diff_apply_result,omitempty"`
	GateLog                       PromotionGateLog          `json:"gate_log"`
	PostApplyVerificationArtifact SandboxArtifact           `json:"post_apply_verification_artifact"`
}

type PromotionRollbackResponse struct {
	Decision         PromotionGateDecision    `json:"decision"`
	RollbackResult   PromotionDiffApplyResult `json:"rollback_result"`
	RollbackArtifact SandboxArtifact          `json:"rollback_artifact"`
	GateLog          PromotionGateLog         `json:"gate_log"`
}

type PromotionWorkflowRequest struct {
	Promotion                    PromotionRequest        `json:"promotion"`
	ApplyAfterApproval           bool                    `json:"apply_after_approval,omitempty"`
	AppliedBy                    string                  `json:"applied_by,omitempty"`
	ApplyTarget                  string                  `json:"apply_target,omitempty"`
	PostApplyVerificationPath    string                  `json:"post_apply_verification_path,omitempty"`
	PostApplyVerificationCommand string                  `json:"post_apply_verification_command,omitempty"`
	HumanApproved                bool                    `json:"human_approved,omitempty"`
	ExternalControl              *ExternalControlRequest `json:"external_control,omitempty"`
}

type PromotionWorkflowResponse struct {
	PromotionResponse PromotionRequestResponse `json:"promotion_response"`
	ApplyResponse     *PromotionApplyResponse  `json:"apply_response,omitempty"`
	Applied           bool                     `json:"applied"`
	SkippedReason     string                   `json:"skipped_reason,omitempty"`
}

func (c *Client) SuperAgentStatus(ctx context.Context, limit int) (SuperAgentStatus, error) {
	path := "/viewer/superagent"
	if limit > 0 {
		path = fmt.Sprintf("%s?limit=%d", path, limit)
	}
	var out SuperAgentStatus
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return SuperAgentStatus{}, err
	}
	return out, nil
}

func (c *Client) CreateAgentRun(ctx context.Context, item AgentRun) error {
	return c.do(ctx, http.MethodPost, "/viewer/superagent/runs", item, nil)
}

func (c *Client) CreateTraceEvent(ctx context.Context, item TraceEvent) error {
	return c.do(ctx, http.MethodPost, "/viewer/superagent/trace-events", item, nil)
}

func (c *Client) PauseRun(ctx context.Context, runID string, reason string) (RunStateResponse, error) {
	var out RunStateResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/superagent/runs/pause", RunStateRequest{RunID: runID, Reason: reason}, &out); err != nil {
		return RunStateResponse{}, err
	}
	return out, nil
}

func (c *Client) ResumeRun(ctx context.Context, runID string, reason string) (RunStateResponse, error) {
	var out RunStateResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/superagent/runs/resume", RunStateRequest{RunID: runID, Reason: reason}, &out); err != nil {
		return RunStateResponse{}, err
	}
	return out, nil
}

func (c *Client) CheckExternalControl(ctx context.Context, req ExternalControlRequest) (ExternalControlResponse, error) {
	var out ExternalControlResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/ai-workflow/external-control/check", req, &out); err != nil {
		return ExternalControlResponse{}, err
	}
	return out, nil
}

func (c *Client) RunCommand(ctx context.Context, req CommandRunRequest) (CommandRunResponse, error) {
	var out CommandRunResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/ai-workflow/commands/run", req, &out); err != nil {
		return CommandRunResponse{}, err
	}
	return out, nil
}

func (c *Client) CreateWorkstreamArtifact(ctx context.Context, item WorkstreamArtifact) (WorkstreamArtifactResponse, error) {
	var out WorkstreamArtifactResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/workstreams/artifacts", item, &out); err != nil {
		return WorkstreamArtifactResponse{}, err
	}
	return out, nil
}

func (c *Client) ReviewWorkstreamVaultUpdate(ctx context.Context, item WorkstreamVaultUpdate) (WorkstreamVaultUpdateResponse, error) {
	var out WorkstreamVaultUpdateResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/workstreams/vault-updates/review", item, &out); err != nil {
		return WorkstreamVaultUpdateResponse{}, err
	}
	return out, nil
}

func (c *Client) EvaluateRevenueHumanDecision(ctx context.Context, item RevenueHumanDecision) (RevenueHumanDecisionResponse, error) {
	var out RevenueHumanDecisionResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/revenue/human-decision-gate", item, &out); err != nil {
		return RevenueHumanDecisionResponse{}, err
	}
	return out, nil
}

func (c *Client) ReviewRevenueHumanDecision(ctx context.Context, item RevenueHumanDecisionReview) (RevenueHumanDecisionResponse, error) {
	var out RevenueHumanDecisionResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/revenue/human-decision-gate/review", item, &out); err != nil {
		return RevenueHumanDecisionResponse{}, err
	}
	return out, nil
}

func (c *Client) CreateRevenueDailyRoutineReport(ctx context.Context, item RevenueDailyRoutineRequest) (RevenueDailyRoutineResponse, error) {
	var out RevenueDailyRoutineResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/revenue/daily-routine", item, &out); err != nil {
		return RevenueDailyRoutineResponse{}, err
	}
	return out, nil
}

func (c *Client) SandboxStatus(ctx context.Context, limit int) (SandboxStatus, error) {
	path := "/viewer/sandbox"
	if limit > 0 {
		path = fmt.Sprintf("%s?limit=%d", path, limit)
	}
	var out SandboxStatus
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return SandboxStatus{}, err
	}
	return out, nil
}

func (c *Client) CreatePromotionRequest(ctx context.Context, req PromotionRequest) (PromotionRequestResponse, error) {
	var out PromotionRequestResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/sandbox/promotions", req, &out); err != nil {
		return PromotionRequestResponse{}, err
	}
	return out, nil
}

func (c *Client) ApplyPromotion(ctx context.Context, req PromotionApplyRequest) (PromotionApplyResponse, error) {
	var out PromotionApplyResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/sandbox/promotions/apply", req, &out); err != nil {
		return PromotionApplyResponse{}, err
	}
	return out, nil
}

func (c *Client) RollbackPromotion(ctx context.Context, req PromotionApplyRequest) (PromotionRollbackResponse, error) {
	var out PromotionRollbackResponse
	if err := c.do(ctx, http.MethodPost, "/viewer/sandbox/promotions/rollback", req, &out); err != nil {
		return PromotionRollbackResponse{}, err
	}
	return out, nil
}

func (c *Client) SubmitPromotionWorkflow(ctx context.Context, req PromotionWorkflowRequest) (PromotionWorkflowResponse, error) {
	if req.ExternalControl != nil {
		controlResp, err := c.CheckExternalControl(ctx, *req.ExternalControl)
		if err != nil {
			return PromotionWorkflowResponse{}, err
		}
		if controlResp.Decision.Status != "allowed" {
			return PromotionWorkflowResponse{SkippedReason: "external control policy did not allow action"}, nil
		}
	}
	promotionResp, err := c.CreatePromotionRequest(ctx, req.Promotion)
	if err != nil {
		return PromotionWorkflowResponse{}, err
	}
	resp := PromotionWorkflowResponse{PromotionResponse: promotionResp}
	if promotionResp.Decision.Status != "approve" {
		resp.SkippedReason = "promotion gate did not approve"
		return resp, nil
	}
	if !req.ApplyAfterApproval {
		resp.SkippedReason = "apply_after_approval is false"
		return resp, nil
	}
	if !req.HumanApproved || strings.TrimSpace(req.Promotion.HumanApprovalStatus) != "granted" {
		resp.SkippedReason = "human approval is required before apply"
		return resp, nil
	}
	if strings.TrimSpace(req.PostApplyVerificationPath) == "" {
		resp.SkippedReason = "post_apply_verification_path is required before apply"
		return resp, nil
	}
	applyResp, err := c.ApplyPromotion(ctx, PromotionApplyRequest{
		Promotion:                    promotionResp.Promotion,
		AppliedBy:                    req.AppliedBy,
		ApplyTarget:                  req.ApplyTarget,
		PostApplyVerificationPath:    req.PostApplyVerificationPath,
		PostApplyVerificationCommand: req.PostApplyVerificationCommand,
		HumanApproved:                req.HumanApproved,
	})
	if err != nil {
		return PromotionWorkflowResponse{}, err
	}
	resp.ApplyResponse = &applyResp
	resp.Applied = true
	return resp, nil
}

func (c *Client) do(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("rencrow API %s %s failed: status=%d body=%s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
