package persona

import "testing"

func TestValidateDiscomfortLog(t *testing.T) {
	item := DiscomfortLog{
		EventID:     "evt_persona_discomfort_1",
		CharacterID: "mio",
		Discomfort:  "作業中なのに雑談へ広げすぎた",
		Status:      "candidate",
	}
	if err := ValidateDiscomfortLog(item); err != nil {
		t.Fatalf("ValidateDiscomfortLog() error = %v", err)
	}
	item.Discomfort = ""
	if err := ValidateDiscomfortLog(item); err == nil {
		t.Fatal("expected missing discomfort to fail")
	}
}

func TestValidateTriggerLogConfidence(t *testing.T) {
	item := TriggerLog{
		EventID:     "evt_trigger_1",
		CharacterID: "kuro",
		TriggerID:   "danger_destructive",
		Activated:   true,
		Confidence:  0.8,
	}
	if err := ValidateTriggerLog(item); err != nil {
		t.Fatalf("ValidateTriggerLog() error = %v", err)
	}
	item.Confidence = 1.2
	if err := ValidateTriggerLog(item); err == nil {
		t.Fatal("expected confidence > 1 to fail")
	}
}

func TestValidateObservationLogRequiresReviewForSensitive(t *testing.T) {
	item := ObservationLog{
		EventID:         "evt_observation_1",
		ObserverID:      "lumina",
		TargetID:        "ren",
		ObservationType: "daily",
		Summary:         "sensitive observation candidate",
		Sensitivity:     "health",
		ReviewStatus:    "pending",
	}
	if err := ValidateObservationLog(item); err != nil {
		t.Fatalf("ValidateObservationLog() error = %v", err)
	}
	item.ReviewStatus = "approved"
	if err := ValidateObservationLog(item); err == nil {
		t.Fatal("expected sensitive auto-approved observation to fail")
	}
}

func TestValidateMetaProfileUpdateReviewRequiresTerminalStatus(t *testing.T) {
	item := MetaProfileUpdate{
		UpdateID:        "meta_upd_1",
		ObserverID:      "lumina",
		TargetID:        "ren",
		Section:         "Risk Signs",
		ProposedContent: "疲労時は判断を急がない方がよい",
		Sensitivity:     "health",
		ReviewStatus:    "pending",
	}
	if err := ValidateMetaProfileUpdate(item); err != nil {
		t.Fatalf("ValidateMetaProfileUpdate() error = %v", err)
	}
	if err := ValidateMetaProfileUpdateReview(item); err == nil {
		t.Fatal("expected pending meta update review to fail")
	}
	item.ReviewStatus = "approved"
	if err := ValidateMetaProfileUpdateReview(item); err != nil {
		t.Fatalf("ValidateMetaProfileUpdateReview() error = %v", err)
	}
}

func TestValidateInterfaceSession(t *testing.T) {
	item := InterfaceSession{
		SessionID:     "persona_session_1",
		CharacterID:   "mio",
		InterfaceType: "web",
		SessionKey:    "web:viewer-session",
	}
	if err := ValidateInterfaceSession(item); err != nil {
		t.Fatalf("ValidateInterfaceSession() error = %v", err)
	}
	item.SessionKey = ""
	if err := ValidateInterfaceSession(item); err == nil {
		t.Fatal("expected missing session_key to fail")
	}
}
