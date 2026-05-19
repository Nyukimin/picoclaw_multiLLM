package rencrowclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSuperAgentStatus(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		if r.Method != http.MethodGet {
			t.Fatalf("method=%s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(SuperAgentStatus{
			AgentRuns: []AgentRun{{RunID: "run_1", AgentType: "LeadAgent", Status: "completed"}},
		})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.SuperAgentStatus(context.Background(), 5)
	if err != nil {
		t.Fatalf("SuperAgentStatus() error = %v", err)
	}
	if gotPath != "/viewer/superagent?limit=5" {
		t.Fatalf("path=%s", gotPath)
	}
	if len(status.AgentRuns) != 1 || status.AgentRuns[0].RunID != "run_1" {
		t.Fatalf("status=%#v", status)
	}
}

func TestCreateAgentRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/superagent/runs" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var item AgentRun
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			t.Fatal(err)
		}
		if item.RunID != "run_1" || item.AgentType != "LeadAgent" {
			t.Fatalf("payload=%#v", item)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = client.CreateAgentRun(context.Background(), AgentRun{
		RunID:     "run_1",
		AgentType: "LeadAgent",
		Status:    "running",
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
}

func TestRunCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/ai-workflow/commands/run" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req CommandRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.CommandName != "review-architecture" || req.Input != "target docs" {
			t.Fatalf("payload=%#v", req)
		}
		_ = json.NewEncoder(w).Encode(CommandRunResponse{EventID: "evt_1", CommandName: req.CommandName, Status: "recorded"})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.RunCommand(context.Background(), CommandRunRequest{CommandName: "review-architecture", Input: "target docs"})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if resp.EventID != "evt_1" || resp.Status != "recorded" {
		t.Fatalf("response=%#v", resp)
	}
}

func TestPauseAndResumeRun(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var req RunStateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.RunID != "run_1" {
			t.Fatalf("payload=%#v", req)
		}
		status := "paused"
		if r.URL.Path == "/viewer/superagent/runs/resume" {
			status = "running"
		}
		_ = json.NewEncoder(w).Encode(RunStateResponse{RunID: req.RunID, Status: status, EventID: "evt_" + status})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := client.PauseRun(context.Background(), "run_1", "pause")
	if err != nil {
		t.Fatalf("PauseRun() error = %v", err)
	}
	resumed, err := client.ResumeRun(context.Background(), "run_1", "resume")
	if err != nil {
		t.Fatalf("ResumeRun() error = %v", err)
	}
	if paused.Status != "paused" || resumed.Status != "running" {
		t.Fatalf("statuses paused=%#v resumed=%#v", paused, resumed)
	}
	if len(paths) != 2 || paths[0] != "/viewer/superagent/runs/pause" || paths[1] != "/viewer/superagent/runs/resume" {
		t.Fatalf("paths=%#v", paths)
	}
}

func TestCreateWorkstreamArtifact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/workstreams/artifacts" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req WorkstreamArtifact
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ArtifactID != "art_1" || req.WorkstreamID != "ws_1" || req.Type != "markdown" {
			t.Fatalf("payload=%#v", req)
		}
		_ = json.NewEncoder(w).Encode(WorkstreamArtifactResponse{Artifact: req})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.CreateWorkstreamArtifact(context.Background(), WorkstreamArtifact{
		ArtifactID:   "art_1",
		WorkstreamID: "ws_1",
		Type:         "markdown",
		FilePath:     "docs/example.md",
		Status:       "draft",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateWorkstreamArtifact() error = %v", err)
	}
	if resp.Artifact.ArtifactID != "art_1" || resp.Artifact.WorkstreamID != "ws_1" {
		t.Fatalf("response=%#v", resp)
	}
}

func TestReviewWorkstreamVaultUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/workstreams/vault-updates/review" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req WorkstreamVaultUpdate
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.UpdateID != "upd_1" || req.WorkstreamID != "ws_1" || req.ReviewStatus != "approved" {
			t.Fatalf("payload=%#v", req)
		}
		_ = json.NewEncoder(w).Encode(WorkstreamVaultUpdateResponse{VaultUpdate: req})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.ReviewWorkstreamVaultUpdate(context.Background(), WorkstreamVaultUpdate{
		UpdateID:     "upd_1",
		WorkstreamID: "ws_1",
		FilePath:     "vault/workstreams/ws_1/STATUS.md",
		ReviewStatus: "approved",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ReviewWorkstreamVaultUpdate() error = %v", err)
	}
	if resp.VaultUpdate.UpdateID != "upd_1" || resp.VaultUpdate.ReviewStatus != "approved" {
		t.Fatalf("response=%#v", resp)
	}
}

func TestEvaluateRevenueHumanDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/revenue/human-decision-gate" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req RevenueHumanDecision
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.DecisionID != "dec_1" || req.DecisionType != "high_ticket_offer" || req.ApprovalStatus != "" {
			t.Fatalf("payload=%#v", req)
		}
		_ = json.NewEncoder(w).Encode(RevenueHumanDecisionResponse{
			Decision: req,
			Record: RevenueHumanDecisionRecord{
				DecisionID:       req.DecisionID,
				DecisionType:     req.DecisionType,
				ApprovalStatus:   "pending",
				GateStatus:       "needs_review",
				RequiresApproval: true,
			},
			Result: RevenueHumanDecisionResult{
				Status:           "needs_review",
				RequiresApproval: true,
			},
		})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.EvaluateRevenueHumanDecision(context.Background(), RevenueHumanDecision{
		DecisionID:   "dec_1",
		DecisionType: "high_ticket_offer",
		Description:  "高単価 offer を案内する",
	})
	if err != nil {
		t.Fatalf("EvaluateRevenueHumanDecision() error = %v", err)
	}
	if resp.Result.Status != "needs_review" || !resp.Result.RequiresApproval || resp.Record.DecisionID != "dec_1" {
		t.Fatalf("response=%#v", resp)
	}
}

func TestReviewRevenueHumanDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/revenue/human-decision-gate/review" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req RevenueHumanDecisionReview
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.DecisionID != "dec_1" || req.ApprovalStatus != "approved" {
			t.Fatalf("payload=%#v", req)
		}
		_ = json.NewEncoder(w).Encode(RevenueHumanDecisionResponse{
			Record: RevenueHumanDecisionRecord{
				DecisionID:       req.DecisionID,
				DecisionType:     "external_publish",
				ApprovalStatus:   "approved",
				GateStatus:       "approved",
				RequiresApproval: true,
			},
			Result: RevenueHumanDecisionResult{
				Status:           "approved",
				RequiresApproval: true,
			},
		})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.ReviewRevenueHumanDecision(context.Background(), RevenueHumanDecisionReview{
		DecisionID:     "dec_1",
		ApprovalStatus: "approved",
	})
	if err != nil {
		t.Fatalf("ReviewRevenueHumanDecision() error = %v", err)
	}
	if resp.Result.Status != "approved" || resp.Record.ApprovalStatus != "approved" {
		t.Fatalf("response=%#v", resp)
	}
}

func TestCreateRevenueDailyRoutineReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/revenue/daily-routine" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req RevenueDailyRoutineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ReportID != "daily_1" || req.WorkstreamID != "ws_revenue" || req.Limit != 20 {
			t.Fatalf("payload=%#v", req)
		}
		_ = json.NewEncoder(w).Encode(RevenueDailyRoutineResponse{
			Report: RevenueDailyRoutineReport{
				ReportID:            req.ReportID,
				WorkstreamID:        req.WorkstreamID,
				Date:                "2026-05-18",
				Status:              "draft_report",
				ExternalSendApplied: false,
				MarketResearch:      1,
			},
			ExternalActionsApplied:                  false,
			HumanApprovalRequiredForExternalActions: true,
		})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.CreateRevenueDailyRoutineReport(context.Background(), RevenueDailyRoutineRequest{
		ReportID:     "daily_1",
		WorkstreamID: "ws_revenue",
		Date:         "2026-05-18",
		Limit:        20,
	})
	if err != nil {
		t.Fatalf("CreateRevenueDailyRoutineReport() error = %v", err)
	}
	if resp.Report.Status != "draft_report" || resp.Report.ExternalSendApplied || resp.ExternalActionsApplied || !resp.HumanApprovalRequiredForExternalActions {
		t.Fatalf("response=%#v", resp)
	}
}

func TestSandboxStatus(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		if r.Method != http.MethodGet {
			t.Fatalf("method=%s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(SandboxStatus{
			Sandboxes: []SandboxRecord{{SandboxID: "sbx_1", Type: "code", Status: "active"}},
			Decisions: []PromotionGateDecision{{
				Status: "needs_review",
				Reason: "missing approval",
			}},
		})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.SandboxStatus(context.Background(), 10)
	if err != nil {
		t.Fatalf("SandboxStatus() error = %v", err)
	}
	if gotPath != "/viewer/sandbox?limit=10" {
		t.Fatalf("path=%s", gotPath)
	}
	if len(status.Sandboxes) != 1 || status.Sandboxes[0].SandboxID != "sbx_1" {
		t.Fatalf("status=%#v", status)
	}
	if len(status.Decisions) != 1 || status.Decisions[0].Status != "needs_review" {
		t.Fatalf("decisions=%#v", status.Decisions)
	}
}

func TestCreatePromotionRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/sandbox/promotions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req PromotionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.PromotionID != "promo_1" || req.SandboxID != "sbx_1" || req.HumanApprovalStatus != "granted" {
			t.Fatalf("payload=%#v", req)
		}
		_ = json.NewEncoder(w).Encode(PromotionRequestResponse{
			Promotion: req,
			Decision:  PromotionGateDecision{Status: "approve", Reason: "ok"},
			GateLog:   PromotionGateLog{EventID: "evt_1", PromotionID: req.PromotionID, GateStatus: "approve"},
		})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.CreatePromotionRequest(context.Background(), PromotionRequest{
		PromotionID:         "promo_1",
		SandboxID:           "sbx_1",
		TargetPath:          "internal/example.go",
		DiffPath:            "sandbox/diff.patch",
		TestResultPath:      "sandbox/test.log",
		Reason:              "verified patch",
		RollbackPlanPath:    "sandbox/rollback.md",
		HumanApprovalStatus: "granted",
		CreatedAt:           time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreatePromotionRequest() error = %v", err)
	}
	if resp.Promotion.PromotionID != "promo_1" || resp.Decision.Status != "approve" || resp.GateLog.EventID != "evt_1" {
		t.Fatalf("response=%#v", resp)
	}
}

func TestApplyPromotion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/sandbox/promotions/apply" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req PromotionApplyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Promotion.PromotionID != "promo_1" || !req.HumanApproved || req.PostApplyVerificationPath == "" {
			t.Fatalf("payload=%#v", req)
		}
		_ = json.NewEncoder(w).Encode(PromotionApplyResponse{
			Decision:                      PromotionGateDecision{Status: "promotion_applied", Reason: "recorded"},
			DiffApplyResult:               &PromotionDiffApplyResult{Status: "applied", AppliedFiles: []string{"internal/example.go"}},
			GateLog:                       PromotionGateLog{EventID: "evt_apply_1", PromotionID: req.Promotion.PromotionID, GateStatus: "promotion_applied"},
			PostApplyVerificationArtifact: SandboxArtifact{ArtifactID: "art_post_apply_1", SandboxID: req.Promotion.SandboxID, Type: "post_apply_verification"},
		})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.ApplyPromotion(context.Background(), PromotionApplyRequest{
		Promotion: PromotionRequest{
			PromotionID:         "promo_1",
			SandboxID:           "sbx_1",
			TargetPath:          "internal/example.go",
			DiffPath:            "sandbox/diff.patch",
			TestResultPath:      "sandbox/test.log",
			Reason:              "verified patch",
			RollbackPlanPath:    "sandbox/rollback.md",
			HumanApprovalStatus: "granted",
			CreatedAt:           time.Now().UTC(),
		},
		AppliedBy:                 "Worker",
		PostApplyVerificationPath: "sandbox/post-apply.log",
		HumanApproved:             true,
	})
	if err != nil {
		t.Fatalf("ApplyPromotion() error = %v", err)
	}
	if resp.Decision.Status != "promotion_applied" || resp.GateLog.EventID != "evt_apply_1" {
		t.Fatalf("response=%#v", resp)
	}
	if resp.DiffApplyResult == nil || resp.DiffApplyResult.Status != "applied" || len(resp.DiffApplyResult.AppliedFiles) != 1 {
		t.Fatalf("diff apply response=%#v", resp.DiffApplyResult)
	}
}

func TestRollbackPromotion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/sandbox/promotions/rollback" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req PromotionApplyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Promotion.PromotionID != "promo_1" || !req.HumanApproved || req.PostApplyVerificationPath == "" {
			t.Fatalf("payload=%#v", req)
		}
		_ = json.NewEncoder(w).Encode(PromotionRollbackResponse{
			Decision:         PromotionGateDecision{Status: "rollback_executed", Reason: "rolled back"},
			RollbackResult:   PromotionDiffApplyResult{Status: "rolled_back", AppliedFiles: []string{"internal/example.go"}},
			RollbackArtifact: SandboxArtifact{ArtifactID: "art_rollback_1", SandboxID: req.Promotion.SandboxID, Type: "rollback_execution"},
			GateLog:          PromotionGateLog{EventID: "evt_rollback_1", PromotionID: req.Promotion.PromotionID, GateStatus: "rollback_executed"},
		})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.RollbackPromotion(context.Background(), PromotionApplyRequest{
		Promotion: PromotionRequest{
			PromotionID:         "promo_1",
			SandboxID:           "sbx_1",
			TargetPath:          "internal/example.go",
			DiffPath:            "sandbox/diff.patch",
			TestResultPath:      "sandbox/test.log",
			Reason:              "verified patch",
			RollbackPlanPath:    "sandbox/rollback.md",
			HumanApprovalStatus: "granted",
			CreatedAt:           time.Now().UTC(),
		},
		AppliedBy:                 "Worker",
		PostApplyVerificationPath: "sandbox/post-rollback.log",
		HumanApproved:             true,
	})
	if err != nil {
		t.Fatalf("RollbackPromotion() error = %v", err)
	}
	if resp.Decision.Status != "rollback_executed" || resp.GateLog.EventID != "evt_rollback_1" {
		t.Fatalf("response=%#v", resp)
	}
	if resp.RollbackResult.Status != "rolled_back" || len(resp.RollbackResult.AppliedFiles) != 1 {
		t.Fatalf("rollback response=%#v", resp.RollbackResult)
	}
}

func TestSubmitPromotionWorkflowCreatesRequestButDoesNotApplyWithoutApproval(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/sandbox/promotions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req PromotionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(PromotionRequestResponse{
			Promotion: req,
			Decision:  PromotionGateDecision{Status: "approve", Reason: "ok"},
			GateLog:   PromotionGateLog{EventID: "evt_1", PromotionID: req.PromotionID, GateStatus: "approve"},
		})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.SubmitPromotionWorkflow(context.Background(), PromotionWorkflowRequest{
		Promotion: PromotionRequest{
			PromotionID:         "promo_1",
			SandboxID:           "sbx_1",
			TargetPath:          "internal/example.go",
			DiffPath:            "sandbox/diff.patch",
			TestResultPath:      "sandbox/test.log",
			Reason:              "verified patch",
			RollbackPlanPath:    "sandbox/rollback.md",
			HumanApprovalStatus: "granted",
			CreatedAt:           time.Now().UTC(),
		},
		ApplyAfterApproval:        true,
		PostApplyVerificationPath: "sandbox/post-apply.log",
		HumanApproved:             false,
	})
	if err != nil {
		t.Fatalf("SubmitPromotionWorkflow() error = %v", err)
	}
	if resp.Applied || resp.ApplyResponse != nil || resp.SkippedReason != "human approval is required before apply" {
		t.Fatalf("workflow response=%#v", resp)
	}
	if len(paths) != 1 || paths[0] != "/viewer/sandbox/promotions" {
		t.Fatalf("paths=%#v", paths)
	}
}

func TestSubmitPromotionWorkflowStopsWhenExternalControlPolicyBlocks(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/ai-workflow/external-control/check" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ExternalControlResponse{
			Decision: ExternalControlDecision{Status: "needs_approval", RequiresApproval: true},
		})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.SubmitPromotionWorkflow(context.Background(), PromotionWorkflowRequest{
		Promotion: PromotionRequest{
			PromotionID:         "promo_1",
			SandboxID:           "sbx_1",
			TargetPath:          "internal/example.go",
			DiffPath:            "sandbox/diff.patch",
			TestResultPath:      "sandbox/test.log",
			Reason:              "verified patch",
			RollbackPlanPath:    "sandbox/rollback.md",
			HumanApprovalStatus: "granted",
			CreatedAt:           time.Now().UTC(),
		},
		ApplyAfterApproval: true,
		HumanApproved:      true,
		ExternalControl: &ExternalControlRequest{
			Actor:     "Worker",
			ChannelID: "viewer",
			Action:    "promotion_apply",
		},
	})
	if err != nil {
		t.Fatalf("SubmitPromotionWorkflow() error = %v", err)
	}
	if resp.Applied || resp.SkippedReason != "external control policy did not allow action" {
		t.Fatalf("workflow response=%#v", resp)
	}
	if len(paths) != 1 || paths[0] != "/viewer/ai-workflow/external-control/check" {
		t.Fatalf("paths=%#v", paths)
	}
}

func TestSubmitPromotionWorkflowDoesNotApplyWhenGateDoesNotApprove(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost || r.URL.Path != "/viewer/sandbox/promotions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req PromotionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(PromotionRequestResponse{
			Promotion: req,
			Decision:  PromotionGateDecision{Status: "needs_more_tests", Reason: "missing test"},
			GateLog:   PromotionGateLog{EventID: "evt_1", PromotionID: req.PromotionID, GateStatus: "needs_more_tests"},
		})
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.SubmitPromotionWorkflow(context.Background(), PromotionWorkflowRequest{
		Promotion: PromotionRequest{
			PromotionID:         "promo_1",
			SandboxID:           "sbx_1",
			TargetPath:          "internal/example.go",
			DiffPath:            "sandbox/diff.patch",
			Reason:              "verified patch",
			RollbackPlanPath:    "sandbox/rollback.md",
			HumanApprovalStatus: "granted",
			CreatedAt:           time.Now().UTC(),
		},
		ApplyAfterApproval:        true,
		PostApplyVerificationPath: "sandbox/post-apply.log",
		HumanApproved:             true,
	})
	if err != nil {
		t.Fatalf("SubmitPromotionWorkflow() error = %v", err)
	}
	if resp.Applied || resp.SkippedReason != "promotion gate did not approve" {
		t.Fatalf("workflow response=%#v", resp)
	}
	if len(paths) != 1 || paths[0] != "/viewer/sandbox/promotions" {
		t.Fatalf("paths=%#v", paths)
	}
}

func TestSubmitPromotionWorkflowAppliesOnlyAfterGateAndApproval(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/viewer/sandbox/promotions":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			var req PromotionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(PromotionRequestResponse{
				Promotion: req,
				Decision:  PromotionGateDecision{Status: "approve", Reason: "ok"},
				GateLog:   PromotionGateLog{EventID: "evt_1", PromotionID: req.PromotionID, GateStatus: "approve"},
			})
		case "/viewer/sandbox/promotions/apply":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			var req PromotionApplyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.Promotion.PromotionID != "promo_1" || !req.HumanApproved || req.PostApplyVerificationPath != "sandbox/post-apply.log" {
				t.Fatalf("apply payload=%#v", req)
			}
			_ = json.NewEncoder(w).Encode(PromotionApplyResponse{
				Decision:                      PromotionGateDecision{Status: "promotion_applied", Reason: "recorded"},
				DiffApplyResult:               &PromotionDiffApplyResult{Status: "applied", AppliedFiles: []string{"internal/example.go"}},
				GateLog:                       PromotionGateLog{EventID: "evt_apply_1", PromotionID: req.Promotion.PromotionID, GateStatus: "promotion_applied"},
				PostApplyVerificationArtifact: SandboxArtifact{ArtifactID: "art_post_apply_1", SandboxID: req.Promotion.SandboxID, Type: "post_apply_verification"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.SubmitPromotionWorkflow(context.Background(), PromotionWorkflowRequest{
		Promotion: PromotionRequest{
			PromotionID:         "promo_1",
			SandboxID:           "sbx_1",
			TargetPath:          "internal/example.go",
			DiffPath:            "sandbox/diff.patch",
			TestResultPath:      "sandbox/test.log",
			Reason:              "verified patch",
			RollbackPlanPath:    "sandbox/rollback.md",
			HumanApprovalStatus: "granted",
			CreatedAt:           time.Now().UTC(),
		},
		ApplyAfterApproval:        true,
		AppliedBy:                 "Worker",
		PostApplyVerificationPath: "sandbox/post-apply.log",
		HumanApproved:             true,
	})
	if err != nil {
		t.Fatalf("SubmitPromotionWorkflow() error = %v", err)
	}
	if !resp.Applied || resp.ApplyResponse == nil || resp.ApplyResponse.Decision.Status != "promotion_applied" {
		t.Fatalf("workflow response=%#v", resp)
	}
	if len(paths) != 2 || paths[0] != "/viewer/sandbox/promotions" || paths[1] != "/viewer/sandbox/promotions/apply" {
		t.Fatalf("paths=%#v", paths)
	}
}

func TestAPIErrorIncludesStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no store", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SuperAgentStatus(context.Background(), 0)
	if err == nil {
		t.Fatal("expected API error")
	}
}
