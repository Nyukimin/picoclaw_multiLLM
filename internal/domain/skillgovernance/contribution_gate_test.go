package skillgovernance

import (
	"testing"
	"time"
)

func TestEvaluateContributionGateBlocksMissingRequiredChecks(t *testing.T) {
	decision := EvaluateContributionGate(ContributionGateLog{
		Repo:                "example/repo",
		ProblemStatement:    "real bug",
		ExistingPRsChecked:  true,
		RealProblemVerified: false,
		CoreChangeVerified:  true,
		DiffHumanApproved:   false,
		TestResult:          "go test ./...",
	})
	if decision.Status != GateStatusBlocked || decision.CanContribute {
		t.Fatalf("decision=%#v", decision)
	}
	if len(decision.StopReasons) != 2 {
		t.Fatalf("reasons=%#v", decision.StopReasons)
	}
}

func TestEvaluateContributionGatePassesWhenAllChecksArePresent(t *testing.T) {
	decision := EvaluateContributionGate(ContributionGateLog{
		Repo:                "example/repo",
		ProblemStatement:    "real bug",
		ExistingPRsChecked:  true,
		RealProblemVerified: true,
		CoreChangeVerified:  true,
		DiffHumanApproved:   true,
		TestResult:          "go test ./...",
	})
	if decision.Status != GateStatusPassed || !decision.CanContribute {
		t.Fatalf("decision=%#v", decision)
	}
	if len(decision.StopReasons) != 0 {
		t.Fatalf("reasons=%#v", decision.StopReasons)
	}
}

func TestNewContributionGateLogSetsStatusAndTimestamp(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	log, decision, err := NewContributionGateLog("evt_contrib_1", ContributionGateLog{
		Repo:                "example/repo",
		ProblemStatement:    "real bug",
		ExistingPRsChecked:  true,
		RealProblemVerified: true,
		CoreChangeVerified:  true,
		DiffHumanApproved:   true,
		TestResult:          "go test ./...",
	}, now)
	if err != nil {
		t.Fatalf("NewContributionGateLog failed: %v", err)
	}
	if log.EventID != "evt_contrib_1" || log.GateStatus != GateStatusPassed || !log.CreatedAt.Equal(now) {
		t.Fatalf("log=%#v", log)
	}
	if decision.Status != GateStatusPassed {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestNewContributionGateLogRequiresEventID(t *testing.T) {
	_, _, err := NewContributionGateLog("", ContributionGateLog{}, time.Now())
	if err == nil {
		t.Fatal("expected missing event_id to fail")
	}
}
