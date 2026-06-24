package persona

import (
	"strings"
	"testing"
	"time"
)

func TestValidateDiscomfortLog(t *testing.T) {
	now := time.Date(2026, 5, 20, 7, 20, 0, 0, time.UTC)
	item := DiscomfortLog{
		EventID:     "evt_persona_discomfort_1",
		CharacterID: "mio",
		Discomfort:  "作業中なのに雑談へ広げすぎた",
		Status:      "candidate",
		CreatedAt:   now,
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
	now := time.Date(2026, 5, 20, 7, 20, 0, 0, time.UTC)
	item := TriggerLog{
		EventID:     "evt_trigger_1",
		CharacterID: "kuro",
		TriggerID:   "danger_destructive",
		Activated:   true,
		Confidence:  0.8,
		CreatedAt:   now,
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
	now := time.Date(2026, 5, 20, 7, 20, 0, 0, time.UTC)
	item := ObservationLog{
		EventID:         "evt_observation_1",
		ObserverID:      "lumina",
		TargetID:        "ren",
		ObservationType: "daily",
		Summary:         "sensitive observation candidate",
		Sensitivity:     "health",
		ReviewStatus:    "pending",
		CreatedAt:       now,
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
	now := time.Date(2026, 5, 20, 7, 20, 0, 0, time.UTC)
	item := MetaProfileUpdate{
		UpdateID:        "meta_upd_1",
		ObserverID:      "lumina",
		TargetID:        "ren",
		Section:         "Risk Signs",
		ProposedContent: "疲労時は判断を急がない方がよい",
		Sensitivity:     "health",
		ReviewStatus:    "pending",
		CreatedAt:       now,
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
	now := time.Date(2026, 5, 20, 7, 20, 0, 0, time.UTC)
	item := InterfaceSession{
		SessionID:     "persona_session_1",
		CharacterID:   "mio",
		InterfaceType: "web",
		SessionKey:    "web:viewer-session",
		CreatedAt:     now,
	}
	if err := ValidateInterfaceSession(item); err != nil {
		t.Fatalf("ValidateInterfaceSession() error = %v", err)
	}
	item.SessionKey = ""
	if err := ValidateInterfaceSession(item); err == nil {
		t.Fatal("expected missing session_key to fail")
	}
}

func TestValidatePersonaRejectsMissingCreatedAt(t *testing.T) {
	cases := []struct {
		name string
		run  func() error
	}{
		{
			name: "discomfort",
			run: func() error {
				return ValidateDiscomfortLog(DiscomfortLog{EventID: "evt_1", CharacterID: "mio", Discomfort: "違和感", Status: "candidate"})
			},
		},
		{
			name: "trigger",
			run: func() error {
				return ValidateTriggerLog(TriggerLog{EventID: "evt_1", CharacterID: "kuro", TriggerID: "danger", Confidence: 0.8})
			},
		},
		{
			name: "canonical",
			run: func() error {
				return ValidateCanonicalResponseLog(CanonicalResponseLog{EventID: "evt_1", CharacterID: "kuro", ResponseID: "block_destructive"})
			},
		},
		{
			name: "observation",
			run: func() error {
				return ValidateObservationLog(ObservationLog{EventID: "evt_1", ObserverID: "lumina", TargetID: "ren", ObservationType: "daily", Sensitivity: "normal", ReviewStatus: "pending"})
			},
		},
		{
			name: "meta profile",
			run: func() error {
				return ValidateMetaProfileUpdate(MetaProfileUpdate{UpdateID: "meta_1", ObserverID: "lumina", TargetID: "ren", Section: "Risk Signs", ProposedContent: "疲労時は判断を急がない", Sensitivity: "health", ReviewStatus: "pending"})
			},
		},
		{
			name: "interface session",
			run: func() error {
				return ValidateInterfaceSession(InterfaceSession{SessionID: "session_1", CharacterID: "mio", InterfaceType: "web", SessionKey: "web:viewer"})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("expected created_at error")
			}
			if !strings.Contains(err.Error(), "created_at") {
				t.Fatalf("expected created_at error, got %v", err)
			}
		})
	}
}
