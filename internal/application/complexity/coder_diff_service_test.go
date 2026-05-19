package complexity

import (
	"context"
	"strings"
	"testing"

	domaincomplexity "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/complexity"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

type stubCoderDiffGenerator struct {
	response string
	requests []task.Task
}

func (s *stubCoderDiffGenerator) Generate(_ context.Context, t task.Task, _ string) (string, error) {
	s.requests = append(s.requests, t)
	return s.response, nil
}

func TestCoderDiffServiceGenerateConcreteDiffExtractsAndValidatesReviewOnlyDiff(t *testing.T) {
	hotspot := domaincomplexity.Hotspot{
		HotspotID:   "hot_1",
		ScanID:      "scan_1",
		FilePath:    "internal/application/example.go",
		HotspotType: "repeated_lookup",
		RiskLevel:   "medium",
		Summary:     "repeated lookup",
	}
	diff := `diff --git a/internal/application/example.go b/internal/application/example.go
--- a/internal/application/example.go
+++ b/internal/application/example.go
@@ -1 +1 @@
-old
+new`
	coder := &stubCoderDiffGenerator{response: "```diff\n" + diff + "\n```"}
	result, err := NewCoderDiffService(coder).GenerateConcreteDiff(context.Background(), CoderDiffRequest{
		Hotspot:      hotspot,
		WorkstreamID: "ws_1",
		JobID:        "job_1",
	})
	if err != nil {
		t.Fatalf("GenerateConcreteDiff failed: %v", err)
	}
	if result.JobID != "job_1" || result.ConcreteDiff != diff {
		t.Fatalf("unexpected result=%#v", result)
	}
	if len(coder.requests) != 1 {
		t.Fatalf("coder requests=%d", len(coder.requests))
	}
	if !strings.Contains(coder.requests[0].UserMessage(), "Do not apply it") {
		t.Fatalf("prompt missing review-only boundary:\n%s", coder.requests[0].UserMessage())
	}
}

func TestExtractUnifiedDiffRejectsNonDiffOutput(t *testing.T) {
	if _, err := ExtractUnifiedDiff("I cannot safely change this."); err == nil {
		t.Fatal("expected non-diff output to be rejected")
	}
}
