package sandbox

import "testing"

func TestEvaluatePromotionRequestApprovesOnlyCompleteRequest(t *testing.T) {
	decision := EvaluatePromotionRequest(PromotionRequest{
		PromotionID:         "prom_1",
		SandboxID:           "sbx_1",
		TargetPath:          "docs/example.md",
		DiffPath:            "sandbox/sbx_1/diff/changes.patch",
		Reason:              "仕様に基づく文書更新",
		TestResultPath:      "sandbox/sbx_1/reports/test.txt",
		RollbackPlanPath:    "sandbox/sbx_1/reports/rollback.md",
		HumanApprovalStatus: ApprovalGranted,
	})

	if decision.Status != GateStatusApproved {
		t.Fatalf("status = %s reason=%s missing=%v", decision.Status, decision.Reason, decision.MissingRequirements)
	}
}

func TestEvaluatePromotionRequestRejectsMissingChecklist(t *testing.T) {
	decision := EvaluatePromotionRequest(PromotionRequest{
		PromotionID: "prom_1",
		SandboxID:   "sbx_1",
		TargetPath:  "docs/example.md",
		Reason:      "仕様に基づく文書更新",
	})

	if decision.Status != GateStatusNeedsMoreTest {
		t.Fatalf("status = %s", decision.Status)
	}
	for _, want := range []string{"diff_path", "test_result_path", "rollback_plan_path", "human_approval"} {
		if !contains(decision.MissingRequirements, want) {
			t.Fatalf("missing requirements %v does not contain %s", decision.MissingRequirements, want)
		}
	}
}

func TestEvaluatePromotionRequestRejectsHumanRejectedRequest(t *testing.T) {
	decision := EvaluatePromotionRequest(PromotionRequest{
		PromotionID:         "prom_1",
		SandboxID:           "sbx_1",
		TargetPath:          "docs/example.md",
		DiffPath:            "sandbox/sbx_1/diff/changes.patch",
		Reason:              "仕様に基づく文書更新",
		TestResultPath:      "sandbox/sbx_1/reports/test.txt",
		RollbackPlanPath:    "sandbox/sbx_1/reports/rollback.md",
		HumanApprovalStatus: ApprovalRejected,
	})

	if decision.Status != GateStatusRejected {
		t.Fatalf("status = %s", decision.Status)
	}
}
