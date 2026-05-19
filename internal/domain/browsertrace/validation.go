package browsertrace

import (
	"errors"
	"strings"
)

var deniedWriteMethods = map[string]bool{
	"PUT":    true,
	"PATCH":  true,
	"DELETE": true,
}

func ValidateTraceRun(item TraceRun) error {
	if strings.TrimSpace(item.TraceRunID) == "" {
		return errors.New("trace_run_id is required")
	}
	if strings.TrimSpace(item.TracePath) == "" {
		return errors.New("trace_path is required")
	}
	return nil
}

func ValidateAPICandidate(item APICandidate) error {
	if strings.TrimSpace(item.CandidateID) == "" {
		return errors.New("candidate_id is required")
	}
	if strings.TrimSpace(item.TraceRunID) == "" {
		return errors.New("trace_run_id is required")
	}
	method := strings.ToUpper(strings.TrimSpace(item.Method))
	if method == "" {
		return errors.New("method is required")
	}
	if deniedWriteMethods[method] {
		return errors.New("write method is not allowed for api discovery")
	}
	if strings.TrimSpace(item.ObservedURL) == "" {
		return errors.New("observed_url is required")
	}
	if strings.TrimSpace(item.Status) == "" {
		return errors.New("status is required")
	}
	if strings.TrimSpace(item.ContainsPersonalData) == "" {
		return errors.New("contains_personal_data is required")
	}
	if item.Confidence < 0 || item.Confidence > 1 {
		return errors.New("confidence must be between 0 and 1")
	}
	return nil
}

func ValidateAPICandidateSchema(item APICandidateSchema) error {
	if strings.TrimSpace(item.SchemaID) == "" {
		return errors.New("schema_id is required")
	}
	if strings.TrimSpace(item.CandidateID) == "" {
		return errors.New("candidate_id is required")
	}
	if strings.TrimSpace(item.SchemaType) == "" {
		return errors.New("schema_type is required")
	}
	if strings.TrimSpace(item.SchemaJSON) == "" {
		return errors.New("schema_json is required")
	}
	if item.SampleCount < 0 {
		return errors.New("sample_count must be >= 0")
	}
	return nil
}

func ValidateAPICandidateValidationResult(item APICandidateValidationResult) error {
	if strings.TrimSpace(item.ValidationID) == "" {
		return errors.New("validation_id is required")
	}
	if strings.TrimSpace(item.CandidateID) == "" {
		return errors.New("candidate_id is required")
	}
	if strings.TrimSpace(item.TraceRunID) == "" {
		return errors.New("trace_run_id is required")
	}
	if strings.TrimSpace(item.Status) == "" {
		return errors.New("status is required")
	}
	for _, issue := range item.Issues {
		if strings.TrimSpace(issue.Code) == "" {
			return errors.New("validation issue code is required")
		}
		if strings.TrimSpace(issue.Message) == "" {
			return errors.New("validation issue message is required")
		}
	}
	return nil
}

func ValidateAPICoverageReport(item APICoverageReport) error {
	if strings.TrimSpace(item.ReportID) == "" {
		return errors.New("report_id is required")
	}
	if strings.TrimSpace(item.TraceRunID) == "" {
		return errors.New("trace_run_id is required")
	}
	return nil
}

func ValidateAPIArtifact(item APIArtifact) error {
	if strings.TrimSpace(item.ArtifactID) == "" {
		return errors.New("artifact_id is required")
	}
	if strings.TrimSpace(item.TraceRunID) == "" {
		return errors.New("trace_run_id is required")
	}
	if strings.TrimSpace(item.Type) == "" {
		return errors.New("artifact_type is required")
	}
	if strings.TrimSpace(item.Title) == "" {
		return errors.New("title is required")
	}
	if strings.TrimSpace(item.Status) == "" {
		return errors.New("status is required")
	}
	if strings.TrimSpace(item.Content) == "" {
		return errors.New("content is required")
	}
	return nil
}
