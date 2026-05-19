package browsertrace

import "testing"

func TestValidateAPICandidateRejectsWriteMethods(t *testing.T) {
	item := APICandidate{
		CandidateID:          "api_cand_1",
		TraceRunID:           "trace_1",
		Method:               "GET",
		ObservedURL:          "https://example.com/api/items",
		ContainsPersonalData: "unknown",
		RiskLevel:            "low",
		Status:               "candidate",
		Confidence:           0.8,
	}
	if err := ValidateAPICandidate(item); err != nil {
		t.Fatalf("ValidateAPICandidate() error = %v", err)
	}
	item.Method = "DELETE"
	if err := ValidateAPICandidate(item); err == nil {
		t.Fatal("expected DELETE candidate to fail")
	}
}

func TestValidateAPICandidateSchema(t *testing.T) {
	item := APICandidateSchema{
		SchemaID:    "schema_1",
		CandidateID: "api_cand_1",
		SchemaType:  "response",
		SchemaJSON:  `{"type":"object"}`,
		SampleCount: 1,
	}
	if err := ValidateAPICandidateSchema(item); err != nil {
		t.Fatalf("ValidateAPICandidateSchema() error = %v", err)
	}
	item.SchemaJSON = ""
	if err := ValidateAPICandidateSchema(item); err == nil {
		t.Fatal("expected missing schema_json to fail")
	}
}

func TestValidateAPICandidateValidationResultRequiresIssueCode(t *testing.T) {
	err := ValidateAPICandidateValidationResult(APICandidateValidationResult{
		ValidationID: "api_val_1",
		CandidateID:  "api_cand_1",
		TraceRunID:   "trace_1",
		Status:       "needs_review",
		Issues: []APIValidationIssue{{
			Message: "terms review is required",
		}},
	})
	if err == nil {
		t.Fatal("expected missing issue code to fail")
	}
}

func TestValidateAPIArtifactRequiresContent(t *testing.T) {
	err := ValidateAPIArtifact(APIArtifact{
		ArtifactID: "art_1",
		TraceRunID: "trace_1",
		Type:       "observed_openapi",
		Title:      "Observed OpenAPI",
		Status:     "generated",
	})
	if err == nil {
		t.Fatal("expected missing content to fail")
	}
}
