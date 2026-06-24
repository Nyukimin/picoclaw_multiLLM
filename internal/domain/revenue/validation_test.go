package revenue

import (
	"strings"
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
	now := time.Date(2026, 5, 20, 7, 30, 0, 0, time.UTC)
	if err := ValidateMarketResearchItem(MarketResearchItem{ItemID: "mkt_1", SourcePlatform: "note", CreatedAt: now}); err != nil {
		t.Fatalf("market research should be valid: %v", err)
	}
	if err := ValidateSNSPostMetric(SNSPostMetric{PostID: "post_1", Platform: "x", Impressions: 1, CreatedAt: now}); err != nil {
		t.Fatalf("sns metric should be valid: %v", err)
	}
	if err := ValidateRevenueEvent(RevenueEvent{EventID: "rev_1", EventType: "purchase", Amount: 980, CreatedAt: now}); err != nil {
		t.Fatalf("revenue event should be valid: %v", err)
	}
	if err := ValidateDailyRoutineReport(DailyRoutineReport{ReportID: "daily_1", Date: "2026-05-18", Status: "draft_report", CreatedAt: now}); err != nil {
		t.Fatalf("daily routine report should be valid: %v", err)
	}
}

func TestValidateRevenueRejectsMissingCreatedAt(t *testing.T) {
	now := time.Date(2026, 5, 20, 7, 30, 0, 0, time.UTC)
	cases := []struct {
		name string
		err  error
	}{
		{name: "market research", err: ValidateMarketResearchItem(MarketResearchItem{ItemID: "mkt_1", SourcePlatform: "note"})},
		{name: "sns post metric", err: ValidateSNSPostMetric(SNSPostMetric{PostID: "post_1", Platform: "x"})},
		{name: "product", err: ValidateProduct(Product{ProductID: "prod_1", ProductName: "商品設計シート", Status: "draft"})},
		{name: "customer voice", err: ValidateCustomerVoice(CustomerVoice{VoiceID: "voice_1", RawText: "よかった", PermissionStatus: "unknown"})},
		{name: "revenue event", err: ValidateRevenueEvent(RevenueEvent{EventID: "rev_1", EventType: "purchase"})},
		{name: "daily routine", err: ValidateDailyRoutineReport(DailyRoutineReport{ReportID: "daily_1", Date: "2026-05-20", Status: "draft_report"})},
		{name: "channel draft", err: ValidateChannelDraft(ChannelDraft{DraftID: "draft_1", Channel: "email", Body: "下書き本文", ApprovalStatus: "pending"})},
		{name: "external send apply", err: ValidateExternalSendApplyRecord(ExternalSendApplyRecord{ApplyID: "apply_1", DraftID: "draft_1", DecisionID: "dec_1", Channel: "email", ApprovalStatus: "approved", HumanApproved: true, ApplyStatus: "blocked", SendResult: "not_sent", FailureReason: "external channel adapter is not configured"})},
		{name: "human decision", err: ValidateHumanDecisionGateRecord(HumanDecisionGateRecord{DecisionID: "dec_1", DecisionType: "external_publish", ApprovalStatus: "pending", GateStatus: "needs_review"})},
		{name: "product updated_at optional", err: ValidateProduct(Product{ProductID: "prod_1", ProductName: "商品設計シート", Status: "draft", CreatedAt: now})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "product updated_at optional" {
				if tc.err != nil {
					t.Fatalf("expected valid product without updated_at: %v", tc.err)
				}
				return
			}
			if tc.err == nil || !strings.Contains(tc.err.Error(), "created_at") {
				t.Fatalf("err=%v, want created_at", tc.err)
			}
		})
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
	now := time.Date(2026, 5, 20, 7, 30, 0, 0, time.UTC)
	record := BuildHumanDecisionGateRecord(HumanDecisionGateRequest{
		DecisionID:   "dec_1",
		DecisionType: "high_ticket_offer",
		Description:  "30万円の導入支援を案内する",
		CreatedAt:    now,
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

func TestValidateExternalSendApplyRecordRequiresApprovedHumanDecision(t *testing.T) {
	now := time.Date(2026, 5, 20, 7, 30, 0, 0, time.UTC)
	record := ExternalSendApplyRecord{
		ApplyID:             "apply_1",
		DraftID:             "draft_1",
		DecisionID:          "dec_1",
		Channel:             "email",
		ApprovalStatus:      "approved",
		HumanApproved:       true,
		ApplyStatus:         "blocked",
		SendResult:          "not_sent",
		FailureReason:       "external channel adapter is not configured",
		ExternalSendApplied: false,
		CreatedAt:           now,
	}
	if err := ValidateExternalSendApplyRecord(record); err != nil {
		t.Fatalf("record should be valid: %v", err)
	}

	record.HumanApproved = false
	if err := ValidateExternalSendApplyRecord(record); err == nil {
		t.Fatal("expected missing human approval to fail")
	}
	record.HumanApproved = true
	record.ApprovalStatus = "pending"
	if err := ValidateExternalSendApplyRecord(record); err == nil {
		t.Fatal("expected unapproved decision to fail")
	}
	record.ApprovalStatus = "approved"
	record.ExternalSendApplied = true
	if err := ValidateExternalSendApplyRecord(record); err == nil {
		t.Fatal("expected externally applied non-sent record to fail")
	}
}

func TestValidateExternalSendApplyRecordRequiresSentStateForSuccessfulSend(t *testing.T) {
	now := time.Date(2026, 5, 20, 7, 30, 0, 0, time.UTC)
	record := ExternalSendApplyRecord{
		ApplyID:             "apply_1",
		DraftID:             "draft_1",
		DecisionID:          "dec_1",
		Channel:             "email",
		ApprovalStatus:      "approved",
		HumanApproved:       true,
		ApplyStatus:         "sent",
		SendResult:          "sent",
		ExternalSendApplied: false,
		PostSendVerified:    true,
		PostSendEvidence:    "delivery id msg_1 observed",
		CreatedAt:           now,
	}
	if err := ValidateExternalSendApplyRecord(record); err == nil {
		t.Fatal("expected sent status without external_send_applied to fail")
	}
	record.ExternalSendApplied = true
	record.PostSendVerified = false
	if err := ValidateExternalSendApplyRecord(record); err == nil {
		t.Fatal("expected sent status without post_send_verified to fail")
	}
	record.PostSendVerified = true
	record.PostSendEvidence = ""
	if err := ValidateExternalSendApplyRecord(record); err == nil {
		t.Fatal("expected sent status without post_send_evidence to fail")
	}
	record.PostSendEvidence = "delivery id msg_1 observed and status=delivered"
	if err := ValidateExternalSendApplyRecord(record); err != nil {
		t.Fatalf("record should be valid: %v", err)
	}
}

func TestValidateExternalSendApplyRecordRejectsVerificationWithoutSentStatus(t *testing.T) {
	record := ExternalSendApplyRecord{
		ApplyID:             "apply_1",
		DraftID:             "draft_1",
		DecisionID:          "dec_1",
		Channel:             "email",
		ApprovalStatus:      "approved",
		HumanApproved:       true,
		ApplyStatus:         "blocked",
		SendResult:          "not_sent",
		FailureReason:       "external channel adapter is not configured",
		ExternalSendApplied: false,
		PostSendVerified:    true,
	}
	if err := ValidateExternalSendApplyRecord(record); err == nil {
		t.Fatal("expected post_send_verified without sent status to fail")
	}
}

func TestValidateExternalSendApplyRecordRejectsSentResultWithoutSentStatus(t *testing.T) {
	record := ExternalSendApplyRecord{
		ApplyID:             "apply_1",
		DraftID:             "draft_1",
		DecisionID:          "dec_1",
		Channel:             "email",
		ApprovalStatus:      "approved",
		HumanApproved:       true,
		ApplyStatus:         "blocked",
		SendResult:          "sent",
		FailureReason:       "external channel adapter is not configured",
		ExternalSendApplied: false,
	}
	if err := ValidateExternalSendApplyRecord(record); err == nil || !strings.Contains(err.Error(), "send_result=sent") {
		t.Fatalf("err=%v", err)
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
