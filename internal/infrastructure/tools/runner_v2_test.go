package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
)

func TestToolRunner_ExecuteV2_Shell_Success(t *testing.T) {
	runner := NewToolRunner(ToolRunnerConfig{})

	resp, err := runner.ExecuteV2(context.Background(), "shell", map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError() {
		t.Fatalf("expected success, got error: %s", resp.Error.Message)
	}
	if resp.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should be set")
	}
	if resp.String() == "" {
		t.Error("result should not be empty")
	}
}

func TestToolRunner_ExecuteV2_Shell_ValidationError(t *testing.T) {
	runner := NewToolRunner(ToolRunnerConfig{})

	// Empty command should fail validation
	resp, err := runner.ExecuteV2(context.Background(), "shell", map[string]any{
		"command": "",
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !resp.IsError() {
		t.Fatal("expected error for empty command")
	}
	if resp.Error.Code != tool.ErrInternalError {
		t.Logf("error code = %s (validation errors are wrapped as internal errors in V1→V2 bridge)", resp.Error.Code)
	}
}

func TestToolRunner_ExecuteV2_FileRead_Success(t *testing.T) {
	runner := NewToolRunner(ToolRunnerConfig{})

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello v2"), 0644)

	resp, err := runner.ExecuteV2(context.Background(), "file_read", map[string]any{
		"path": testFile,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError() {
		t.Fatalf("expected success, got error: %s", resp.Error.Message)
	}
	if resp.String() != "hello v2" {
		t.Errorf("result = %q, want %q", resp.String(), "hello v2")
	}
}

func TestToolRunner_ExecuteV2_PathTraversal(t *testing.T) {
	runner := NewToolRunner(ToolRunnerConfig{})

	resp, err := runner.ExecuteV2(context.Background(), "file_read", map[string]any{
		"path": "../../../etc/passwd",
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !resp.IsError() {
		t.Fatal("expected error for path traversal")
	}
}

func TestToolRunner_ExecuteV2_UnknownTool(t *testing.T) {
	runner := NewToolRunner(ToolRunnerConfig{})

	_, err := runner.ExecuteV2(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestToolRunner_ExecuteV2_JSON(t *testing.T) {
	runner := NewToolRunner(ToolRunnerConfig{})

	resp, err := runner.ExecuteV2(context.Background(), "shell", map[string]any{
		"command": "echo ok",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, err := resp.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := parsed["result"]; !ok {
		t.Error("JSON should contain 'result' field")
	}
	if _, ok := parsed["generated_at"]; !ok {
		t.Error("JSON should contain 'generated_at' field")
	}
}

func TestToolRunner_ListTools(t *testing.T) {
	runner := NewToolRunner(ToolRunnerConfig{})

	metas, err := runner.ListTools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(metas) < 5 {
		t.Errorf("expected at least 5 tools, got %d", len(metas))
	}

	// Check metadata fields
	found := map[string]bool{}
	for _, m := range metas {
		found[m.ToolID] = true
		if m.Version == "" {
			t.Errorf("tool %s has empty version", m.ToolID)
		}
		if m.Category == "" {
			t.Errorf("tool %s has empty category", m.ToolID)
		}
	}

	for _, name := range []string{"shell", "file_read", "file_write", "file_list", "web_search"} {
		if !found[name] {
			t.Errorf("expected tool %s in metadata list", name)
		}
	}
}

func TestToolRunner_ListTools_MutationCategory(t *testing.T) {
	runner := NewToolRunner(ToolRunnerConfig{})

	metas, err := runner.ListTools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, m := range metas {
		switch m.ToolID {
		case "shell", "file_write":
			if m.Category != "mutation" {
				t.Errorf("%s category = %q, want %q", m.ToolID, m.Category, "mutation")
			}
			if !m.RequiresApproval {
				t.Errorf("%s should require approval", m.ToolID)
			}
		case "file_read", "file_list", "web_search":
			if m.Category != "query" {
				t.Errorf("%s category = %q, want %q", m.ToolID, m.Category, "query")
			}
		}
	}
}

func TestToolRunner_V2_SatisfiesRunnerV2Interface(t *testing.T) {
	runner := NewToolRunner(ToolRunnerConfig{})

	// Compile-time check: ToolRunner implements RunnerV2
	var _ tool.RunnerV2 = runner
}
