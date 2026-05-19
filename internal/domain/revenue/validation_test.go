package revenue

import (
	"testing"
	"time"
)

func TestValidateProductRejectsSuccessGuarantee(t *testing.T) {
	err := ValidateProduct(Product{
		ProductID:   "prod_1",
		ProductName: "AI副業テンプレ",
		Promise:     "誰でも必ず稼げる",
		Status:      "draft",
	})
	if err == nil {
		t.Fatal("expected prohibited revenue claim to fail")
	}
}

func TestValidateCustomerVoiceRequiresPermissionForMarketing(t *testing.T) {
	err := ValidateCustomerVoice(CustomerVoice{
		VoiceID:            "voice_1",
		RawText:            "ここがわからない",
		UsableForMarketing: true,
		PermissionStatus:   "unknown",
	})
	if err == nil {
		t.Fatal("expected missing permission to fail")
	}
}

func TestValidateRevenueRecords(t *testing.T) {
	if err := ValidateMarketResearchItem(MarketResearchItem{ItemID: "mkt_1", SourcePlatform: "note"}); err != nil {
		t.Fatalf("market research should be valid: %v", err)
	}
	if err := ValidateSNSPostMetric(SNSPostMetric{PostID: "post_1", Platform: "x", Impressions: 1}); err != nil {
		t.Fatalf("sns metric should be valid: %v", err)
	}
	if err := ValidateRevenueEvent(RevenueEvent{EventID: "rev_1", EventType: "purchase", Amount: 980}); err != nil {
		t.Fatalf("revenue event should be valid: %v", err)
	}
	if err := ValidateDailyRoutineReport(DailyRoutineReport{ReportID: "daily_1", Date: "2026-05-18", Status: "draft_report"}); err != nil {
		t.Fatalf("daily routine report should be valid: %v", err)
	}
}

func TestEvaluateHumanDecisionGateRequiresApprovalForHighTicketOffer(t *testing.T) {
	result := EvaluateHumanDecisionGate(HumanDecisionGateRequest{
		DecisionType: "high_ticket_offer",
		Description:  "30万円の導入支援を案内する",
	})

	if result.Status != "needs_review" || !result.RequiresApproval {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestEvaluateHumanDecisionGateBlocksRejectedApproval(t *testing.T) {
	result := EvaluateHumanDecisionGate(HumanDecisionGateRequest{
		DecisionType:   "customer_voice_publication",
		Description:    "購入者の声を販売ページへ掲載する",
		ApprovalStatus: "rejected",
	})

	if result.Status != "blocked" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestEvaluateHumanDecisionGateAllowsApprovedDecision(t *testing.T) {
	result := EvaluateHumanDecisionGate(HumanDecisionGateRequest{
		DecisionType:   "product_price",
		Description:    "低単価商品の価格を980円にする",
		ApprovalStatus: "approved",
	})

	if result.Status != "approved" || !result.RequiresApproval {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestEvaluateHumanDecisionGateBlocksProhibitedClaim(t *testing.T) {
	result := EvaluateHumanDecisionGate(HumanDecisionGateRequest{
		DecisionType: "external_publish",
		Description:  "誰でも必ず稼げると投稿する",
	})

	if result.Status != "blocked" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestBuildHumanDecisionGateRecordDefaultsPendingApproval(t *testing.T) {
	record := BuildHumanDecisionGateRecord(HumanDecisionGateRequest{
		DecisionID:   "dec_1",
		DecisionType: "high_ticket_offer",
		Description:  "30万円の導入支援を案内する",
	})

	if record.ApprovalStatus != "pending" || record.GateStatus != "needs_review" || !record.RequiresApproval {
		t.Fatalf("unexpected record: %#v", record)
	}
	if err := ValidateHumanDecisionGateRecord(record); err != nil {
		t.Fatalf("record should be valid: %v", err)
	}
}

func TestValidateHumanDecisionGateRecordRejectsInvalidApprovalStatus(t *testing.T) {
	err := ValidateHumanDecisionGateRecord(HumanDecisionGateRecord{
		DecisionID:     "dec_1",
		DecisionType:   "external_publish",
		ApprovalStatus: "granted",
		GateStatus:     "approved",
	})
	if err == nil {
		t.Fatal("expected invalid approval_status to fail")
	}
}

func TestBuildDailyRoutineReportIsDraftOnly(t *testing.T) {
	report := BuildDailyRoutineReport(DailyRoutineInput{
		ReportID:       "daily_1",
		WorkstreamID:   "ws_revenue",
		Date:           "2026-05-18",
		Now:            time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
		MarketResearch: []MarketResearchItem{{ItemID: "mkt_1", SourcePlatform: "note"}},
		SNSPosts:       []SNSPostMetric{{PostID: "post_1", Platform: "x"}},
		Products:       []Product{{ProductID: "prod_1", ProductName: "商品設計シート", Status: "draft"}},
		CustomerVoices: []CustomerVoice{{VoiceID: "voice_1", RawText: "ここがわからない", PermissionStatus: "unknown"}},
		RevenueEvents: []RevenueEvent{
			{EventID: "rev_1", EventType: "purchase", Amount: 980, CustomerID: "cust_1"},
			{EventID: "rev_2", EventType: "purchase", Amount: 1980, CustomerID: "cust_1"},
		},
		Decisions: []HumanDecisionGateRecord{{DecisionID: "dec_1", DecisionType: "external_publish", ApprovalStatus: "pending", GateStatus: "needs_review"}},
	})

	if report.Status != "draft_report" || report.ExternalSendApplied {
		t.Fatalf("expected draft-only report: %#v", report)
	}
	if report.PaidCustomers != 1 || report.PendingDecisions != 1 {
		t.Fatalf("unexpected counts: %#v", report)
	}
	if err := ValidateDailyRoutineReport(report); err != nil {
		t.Fatalf("report should be valid: %v", err)
	}
}

func TestValidateDailyRoutineReportRejectsExternalSend(t *testing.T) {
	err := ValidateDailyRoutineReport(DailyRoutineReport{
		ReportID:            "daily_1",
		Date:                "2026-05-18",
		Status:              "draft_report",
		ExternalSendApplied: true,
	})
	if err == nil {
		t.Fatal("expected external send report to fail")
	}
}
