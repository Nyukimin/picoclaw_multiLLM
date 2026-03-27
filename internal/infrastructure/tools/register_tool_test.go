package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tools"
)

func newRegisterToolRunner(t *testing.T, registry *mockToolRegistry, autoApprove bool, callback func(name, desc string, trusted bool)) (*tools.ToolRunner, string) {
	t.Helper()
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := tools.ToolRunnerConfig{
		ToolRegistry:     registry,
		WorkspaceDir:     dir,
		AutoApproveShiro: autoApprove,
		OnToolRegistered: callback,
		DisableWebSearch: true,
	}
	return tools.NewToolRunner(cfg), dir
}

func TestRegisterTool_InvalidName_ReturnsError(t *testing.T) {
	reg := &mockToolRegistry{entries: map[string]capability.ToolEntry{}}
	runner, _ := newRegisterToolRunner(t, reg, false, nil)

	resp, err := runner.ExecuteV2(context.Background(), "register_tool", map[string]any{
		"name":        "bad name!", // invalid
		"description": "a tool",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsError() {
		t.Error("expected error response for invalid name")
	}
}

func TestRegisterTool_MissingScript_ReturnsError(t *testing.T) {
	reg := &mockToolRegistry{entries: map[string]capability.ToolEntry{}}
	runner, _ := newRegisterToolRunner(t, reg, false, nil)

	resp, err := runner.ExecuteV2(context.Background(), "register_tool", map[string]any{
		"name":        "nonexistent_tool",
		"description": "a tool",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsError() {
		t.Error("expected error response for missing script")
	}
}

func TestRegisterTool_AutoApprove_TrustedTrue(t *testing.T) {
	reg := &mockToolRegistry{entries: map[string]capability.ToolEntry{}}
	runner, dir := newRegisterToolRunner(t, reg, true, nil)

	// スクリプト作成
	scriptPath := filepath.Join(dir, "tools", "my_tool.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho ok"), 0755); err != nil {
		t.Fatal(err)
	}

	resp, err := runner.ExecuteV2(context.Background(), "register_tool", map[string]any{
		"name":        "my_tool",
		"description": "a test tool",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError() {
		t.Fatalf("unexpected tool error: %s", resp.String())
	}

	entry, getErr := reg.Get(context.Background(), "my_tool")
	if getErr != nil {
		t.Fatalf("tool not found in registry: %v", getErr)
	}
	if !entry.Trusted {
		t.Error("expected trusted=true with auto_approve=true")
	}
}

func TestRegisterTool_ManualApprove_TrustedFalse(t *testing.T) {
	reg := &mockToolRegistry{entries: map[string]capability.ToolEntry{}}
	runner, dir := newRegisterToolRunner(t, reg, false, nil)

	scriptPath := filepath.Join(dir, "tools", "my_tool.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho ok"), 0755); err != nil {
		t.Fatal(err)
	}

	resp, err := runner.ExecuteV2(context.Background(), "register_tool", map[string]any{
		"name":        "my_tool",
		"description": "a test tool",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError() {
		t.Fatalf("unexpected tool error: %s", resp.String())
	}

	entry, getErr := reg.Get(context.Background(), "my_tool")
	if getErr != nil {
		t.Fatalf("tool not found in registry: %v", getErr)
	}
	if entry.Trusted {
		t.Error("expected trusted=false with auto_approve=false")
	}
}

func TestRegisterTool_Callback_Invoked(t *testing.T) {
	reg := &mockToolRegistry{entries: map[string]capability.ToolEntry{}}
	var gotName, gotDesc string
	var gotTrusted bool
	callback := func(name, desc string, trusted bool) {
		gotName = name
		gotDesc = desc
		gotTrusted = trusted
	}
	runner, dir := newRegisterToolRunner(t, reg, true, callback)

	scriptPath := filepath.Join(dir, "tools", "cb_tool.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho ok"), 0755); err != nil {
		t.Fatal(err)
	}

	_, _ = runner.ExecuteV2(context.Background(), "register_tool", map[string]any{
		"name":        "cb_tool",
		"description": "callback test",
	})

	if gotName != "cb_tool" {
		t.Errorf("expected callback name 'cb_tool', got %q", gotName)
	}
	if gotDesc != "callback test" {
		t.Errorf("expected callback desc 'callback test', got %q", gotDesc)
	}
	if !gotTrusted {
		t.Error("expected callback trusted=true")
	}
}
