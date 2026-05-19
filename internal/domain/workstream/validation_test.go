package workstream

import "testing"

func TestValidateGoalRequiresSuccessCriteriaAndVerification(t *testing.T) {
	goal := Goal{
		GoalID:       "goal_1",
		WorkstreamID: "ws_1",
		Title:        "LPを作る",
	}
	if err := ValidateGoal(goal); err == nil {
		t.Fatal("expected missing success criteria to fail")
	}
	goal.SuccessCriteria = []string{"CTAがある"}
	if err := ValidateGoal(goal); err == nil {
		t.Fatal("expected missing verification to fail")
	}
	goal.Verification = []string{"Viewerで確認する"}
	if err := ValidateGoal(goal); err != nil {
		t.Fatalf("ValidateGoal failed: %v", err)
	}
}

func TestValidateWorkstreamRequiresIdentityAndStatus(t *testing.T) {
	item := Workstream{}
	if err := ValidateWorkstream(item); err == nil {
		t.Fatal("expected missing workstream_id to fail")
	}
	item.WorkstreamID = "ws_1"
	if err := ValidateWorkstream(item); err == nil {
		t.Fatal("expected missing name to fail")
	}
	item.Name = "収益化"
	if err := ValidateWorkstream(item); err == nil {
		t.Fatal("expected missing status to fail")
	}
	item.Status = StatusActive
	if err := ValidateWorkstream(item); err != nil {
		t.Fatalf("ValidateWorkstream failed: %v", err)
	}
}

func TestValidateArtifactRequiresContractFields(t *testing.T) {
	item := Artifact{}
	if err := ValidateArtifact(item); err == nil {
		t.Fatal("expected missing artifact_id to fail")
	}
	item.ArtifactID = "art_1"
	if err := ValidateArtifact(item); err == nil {
		t.Fatal("expected missing workstream_id to fail")
	}
	item.WorkstreamID = "ws_1"
	if err := ValidateArtifact(item); err == nil {
		t.Fatal("expected missing type to fail")
	}
	item.Type = "markdown"
	if err := ValidateArtifact(item); err == nil {
		t.Fatal("expected missing status to fail")
	}
	item.Status = "draft"
	if err := ValidateArtifact(item); err != nil {
		t.Fatalf("ValidateArtifact failed: %v", err)
	}
}

func TestValidateArtifactAnnotationRequiresComment(t *testing.T) {
	item := ArtifactAnnotation{}
	if err := ValidateArtifactAnnotation(item); err == nil {
		t.Fatal("expected missing annotation_id to fail")
	}
	item.AnnotationID = "ann_1"
	if err := ValidateArtifactAnnotation(item); err == nil {
		t.Fatal("expected missing artifact_id to fail")
	}
	item.ArtifactID = "art_1"
	if err := ValidateArtifactAnnotation(item); err == nil {
		t.Fatal("expected missing comment to fail")
	}
	item.Comment = "見出しが抽象的"
	if err := ValidateArtifactAnnotation(item); err == nil {
		t.Fatal("expected missing status to fail")
	}
	item.Status = "open"
	if err := ValidateArtifactAnnotation(item); err != nil {
		t.Fatalf("ValidateArtifactAnnotation failed: %v", err)
	}
}

func TestValidateSteeringItemRequiresInstruction(t *testing.T) {
	item := SteeringItem{}
	if err := ValidateSteeringItem(item); err == nil {
		t.Fatal("expected missing steering_id to fail")
	}
	item.SteeringID = "stq_1"
	if err := ValidateSteeringItem(item); err == nil {
		t.Fatal("expected missing workstream_id to fail")
	}
	item.WorkstreamID = "ws_1"
	if err := ValidateSteeringItem(item); err == nil {
		t.Fatal("expected missing instruction to fail")
	}
	item.Instruction = "CTAを直す"
	if err := ValidateSteeringItem(item); err == nil {
		t.Fatal("expected missing status to fail")
	}
	item.Status = "pending"
	if err := ValidateSteeringItem(item); err != nil {
		t.Fatalf("ValidateSteeringItem failed: %v", err)
	}
}

func TestValidateHeartbeatScheduleRequiresDraftTaskContract(t *testing.T) {
	item := HeartbeatSchedule{}
	if err := ValidateHeartbeatSchedule(item); err == nil {
		t.Fatal("expected missing heartbeat_id to fail")
	}
	item.HeartbeatID = "hb_1"
	if err := ValidateHeartbeatSchedule(item); err == nil {
		t.Fatal("expected missing workstream_id to fail")
	}
	item.WorkstreamID = "ws_1"
	if err := ValidateHeartbeatSchedule(item); err == nil {
		t.Fatal("expected missing schedule_text to fail")
	}
	item.ScheduleText = "daily 08:00"
	if err := ValidateHeartbeatSchedule(item); err == nil {
		t.Fatal("expected missing task to fail")
	}
	item.Task = "draft report only"
	if err := ValidateHeartbeatSchedule(item); err == nil {
		t.Fatal("expected missing status to fail")
	}
	item.Status = StatusActive
	if err := ValidateHeartbeatSchedule(item); err != nil {
		t.Fatalf("ValidateHeartbeatSchedule failed: %v", err)
	}
}

func TestValidateVaultUpdateLogRequiresReviewStatus(t *testing.T) {
	item := VaultUpdateLog{}
	if err := ValidateVaultUpdateLog(item); err == nil {
		t.Fatal("expected missing update_id to fail")
	}
	item.UpdateID = "upd_1"
	if err := ValidateVaultUpdateLog(item); err == nil {
		t.Fatal("expected missing workstream_id to fail")
	}
	item.WorkstreamID = "ws_1"
	if err := ValidateVaultUpdateLog(item); err == nil {
		t.Fatal("expected missing file_path to fail")
	}
	item.FilePath = "vault/workstreams/ws_1/STATUS.md"
	if err := ValidateVaultUpdateLog(item); err == nil {
		t.Fatal("expected missing review_status to fail")
	}
	item.ReviewStatus = "pending"
	if err := ValidateVaultUpdateLog(item); err != nil {
		t.Fatalf("ValidateVaultUpdateLog failed: %v", err)
	}
}

func TestValidateVaultUpdateReviewAllowsOnlyTerminalReviewStatus(t *testing.T) {
	item := VaultUpdateLog{
		UpdateID:     "upd_1",
		WorkstreamID: "ws_1",
		FilePath:     "vault/workstreams/ws_1/STATUS.md",
		ReviewStatus: VaultReviewPending,
	}
	if err := ValidateVaultUpdateReview(item); err == nil {
		t.Fatal("expected pending review status to fail")
	}
	item.ReviewStatus = VaultReviewApproved
	if err := ValidateVaultUpdateReview(item); err != nil {
		t.Fatalf("ValidateVaultUpdateReview approved failed: %v", err)
	}
	item.ReviewStatus = VaultReviewRejected
	if err := ValidateVaultUpdateReview(item); err != nil {
		t.Fatalf("ValidateVaultUpdateReview rejected failed: %v", err)
	}
}
