package security

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	domainexecution "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
	execrepo "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/execution"
)

type fakeRunner struct {
	metas []tool.ToolMetadata
}

func (f *fakeRunner) ExecuteV2(_ context.Context, _ string, _ map[string]any) (*tool.ToolResponse, error) {
	return tool.NewSuccess("ok"), nil
}

func (f *fakeRunner) ListTools(_ context.Context) ([]tool.ToolMetadata, error) {
	return f.metas, nil
}

func TestPolicyRunner_DenyBlockedCommand(t *testing.T) {
	repo, err := execrepo.NewJSONLRepository(filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil {
		t.Fatalf("repo init failed: %v", err)
	}

	inner := &fakeRunner{metas: []tool.ToolMetadata{{ToolID: "shell", RequiresApproval: true}}}
	engine := NewPolicyEngine(PolicyConfig{ApprovalMode: "never", DenyCommands: []string{"rm -rf"}})
	runner, err := NewPolicyRunner(inner, engine, repo, "test", 10*time.Minute)
	if err != nil {
		t.Fatalf("NewPolicyRunner failed: %v", err)
	}

	resp, err := runner.ExecuteV2(context.Background(), "shell", map[string]any{"command": "rm -rf /tmp/x"})
	if err != nil {
		t.Fatalf("ExecuteV2 returned err: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != tool.ErrPermissionDenied {
		t.Fatalf("expected permission denied, got %+v", resp)
	}

	counts, err := repo.CountByStatus(context.Background())
	if err != nil {
		t.Fatalf("CountByStatus failed: %v", err)
	}
	if counts[domainexecution.StatusDenied] == 0 {
		t.Fatalf("expected denied count > 0, got %v", counts)
	}
}
