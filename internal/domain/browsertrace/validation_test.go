package browsertrace

import (
	"strings"
	"testing"
	"time"
)

func TestValidateAPICandidateRejectsWriteMethods(t *testing.T) {
	now := fixedBrowserTraceValidationTime()
	item := APICandidate{
		CandidateID:          "api_cand_1",
		TraceRunID:           "trace_1",
		Method:               "GET",
		ObservedURL:          "https://example.com/api/items",
		ContainsPersonalData: "unknown",
		RiskLevel:            "low",
		Status:               "candidate",
		Confidence:           0.8,
		CreatedAt:            now,
	}
	if err := ValidateAPICandidate(item); err != nil {
		t.Fatalf("ValidateAPICandidate() error = %v", err)
	}
	item.Method = "DELETE"
	if err := ValidateAPICandidate(item); err == nil {
		t.Fatal("expected DELETE candidate to fail")
	}
	item.Method = "GET"
	item.Status = "promoted"
	if err := ValidateAPICandidate(item); err == nil {
		t.Fatal("expected unknown candidate status to fail")
	}
}

func TestValidateAPICandidateSchema(t *testing.T) {
	now := fixedBrowserTraceValidationTime()
	item := APICandidateSchema{
		SchemaID:    "schema_1",
		CandidateID: "api_cand_1",
		SchemaType:  "response",
		SchemaJSON:  `{"type":"object"}`,
		SampleCount: 1,
		CreatedAt:   now,
	}
	if err := ValidateAPICandidateSchema(item); err != nil {
		t.Fatalf("ValidateAPICandidateSchema() error = %v", err)
	}
	item.SchemaJSON = ""
	if err := ValidateAPICandidateSchema(item); err == nil {
		t.Fatal("expected missing schema_json to fail")
	}
	item.SchemaJSON = `{"type":`
	if err := ValidateAPICandidateSchema(item); err == nil {
		t.Fatal("expected invalid schema_json to fail")
	}
	item.SchemaJSON = `{"type":"object"}`
	item.Confidence = 1.1
	if err := ValidateAPICandidateSchema(item); err == nil {
		t.Fatal("expected invalid confidence to fail")
	}
}

func TestValidateAPICandidateValidationResultRequiresIssueCode(t *testing.T) {
	now := fixedBrowserTraceValidationTime()
	err := ValidateAPICandidateValidationResult(APICandidateValidationResult{
		ValidationID: "api_val_1",
		CandidateID:  "api_cand_1",
		TraceRunID:   "trace_1",
		Status:       "needs_review",
		CreatedAt:    now,
		Issues: []APIValidationIssue{{
			Message: "terms review is required",
		}},
	})
	if err == nil {
		t.Fatal("expected missing issue code to fail")
	}
	err = ValidateAPICandidateValidationResult(APICandidateValidationResult{
		ValidationID: "api_val_1",
		CandidateID:  "api_cand_1",
		TraceRunID:   "trace_1",
		Status:       "approved",
		CreatedAt:    now,
		Issues: []APIValidationIssue{{
			Code:    "terms_review_required",
			Message: "terms review is required",
		}},
	})
	if err == nil {
		t.Fatal("expected unknown validation status to fail")
	}
}

func TestValidateAPICandidateValidationResultRequiresStatusPassedIssueConsistency(t *testing.T) {
	now := fixedBrowserTraceValidationTime()
	validated := APICandidateValidationResult{
		ValidationID: "api_val_1",
		CandidateID:  "api_cand_1",
		TraceRunID:   "trace_1",
		Passed:       true,
		Status:       "validated",
		CreatedAt:    now,
	}
	if err := ValidateAPICandidateValidationResult(validated); err != nil {
		t.Fatalf("ValidateAPICandidateValidationResult() error = %v", err)
	}
	needsReview := APICandidateValidationResult{
		ValidationID: "api_val_2",
		CandidateID:  "api_cand_1",
		TraceRunID:   "trace_1",
		Passed:       false,
		Status:       "needs_review",
		CreatedAt:    now,
		Issues: []APIValidationIssue{{
			Code:    "terms_review_required",
			Message: "terms review is required",
		}},
	}
	if err := ValidateAPICandidateValidationResult(needsReview); err != nil {
		t.Fatalf("ValidateAPICandidateValidationResult() error = %v", err)
	}

	tests := []struct {
		name string
		item APICandidateValidationResult
	}{
		{name: "validated without passed", item: APICandidateValidationResult{ValidationID: "api_val_3", CandidateID: "api_cand_1", TraceRunID: "trace_1", Status: "validated", CreatedAt: now}},
		{name: "validated with issues", item: APICandidateValidationResult{ValidationID: "api_val_4", CandidateID: "api_cand_1", TraceRunID: "trace_1", Passed: true, Status: "validated", CreatedAt: now, Issues: []APIValidationIssue{{Code: "terms", Message: "terms issue"}}}},
		{name: "needs review with passed", item: APICandidateValidationResult{ValidationID: "api_val_5", CandidateID: "api_cand_1", TraceRunID: "trace_1", Passed: true, Status: "needs_review", CreatedAt: now, Issues: []APIValidationIssue{{Code: "terms", Message: "terms issue"}}}},
		{name: "needs review without issues", item: APICandidateValidationResult{ValidationID: "api_val_6", CandidateID: "api_cand_1", TraceRunID: "trace_1", Status: "needs_review", CreatedAt: now}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateAPICandidateValidationResult(tt.item); err == nil {
				t.Fatal("expected invalid validation state to fail")
			}
		})
	}
}

func TestValidateBrowserTraceAPIRejectsMissingCreatedAt(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "trace run",
			err: ValidateTraceRun(TraceRun{
				TraceRunID: "trace_1",
				TracePath:  "traces/trace_1.json",
			}),
		},
		{
			name: "candidate",
			err: ValidateAPICandidate(APICandidate{
				CandidateID:          "api_cand_1",
				TraceRunID:           "trace_1",
				Method:               "GET",
				ObservedURL:          "https://example.com/api/items",
				ContainsPersonalData: "unknown",
				Status:               "candidate",
			}),
		},
		{
			name: "schema",
			err: ValidateAPICandidateSchema(APICandidateSchema{
				SchemaID:    "schema_1",
				CandidateID: "api_cand_1",
				SchemaType:  "response",
				SchemaJSON:  `{"type":"object"}`,
			}),
		},
		{
			name: "validation",
			err: ValidateAPICandidateValidationResult(APICandidateValidationResult{
				ValidationID: "api_val_1",
				CandidateID:  "api_cand_1",
				TraceRunID:   "trace_1",
				Passed:       true,
				Status:       "validated",
			}),
		},
		{
			name: "coverage",
			err: ValidateAPICoverageReport(APICoverageReport{
				ReportID:   "coverage_1",
				TraceRunID: "trace_1",
			}),
		},
		{
			name: "artifact",
			err: ValidateAPIArtifact(APIArtifact{
				ArtifactID: "art_1",
				TraceRunID: "trace_1",
				Type:       "observed_openapi",
				Title:      "Observed OpenAPI",
				Status:     "generated",
				Content:    "openapi: 3.1.0",
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil || !strings.Contains(tt.err.Error(), "created_at") {
				t.Fatalf("validation error = %v, want created_at", tt.err)
			}
		})
	}
}

func TestValidateAPIArtifactRequiresContent(t *testing.T) {
	now := fixedBrowserTraceValidationTime()
	err := ValidateAPIArtifact(APIArtifact{
		ArtifactID: "art_1",
		TraceRunID: "trace_1",
		Type:       "observed_openapi",
		Title:      "Observed OpenAPI",
		Status:     "generated",
		CreatedAt:  now,
	})
	if err == nil {
		t.Fatal("expected missing content to fail")
	}
	err = ValidateAPIArtifact(APIArtifact{
		ArtifactID: "art_1",
		TraceRunID: "trace_1",
		Type:       "observed_openapi",
		Title:      "Observed OpenAPI",
		Status:     "promoted",
		Content:    "openapi: 3.1.0",
		CreatedAt:  now,
	})
	if err == nil {
		t.Fatal("expected unknown artifact status to fail")
	}
}

func fixedBrowserTraceValidationTime() time.Time {
	return time.Date(2026, 5, 20, 6, 40, 0, 0, time.UTC)
}
