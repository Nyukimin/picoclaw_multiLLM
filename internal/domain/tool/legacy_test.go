package tool

import (
	"context"
	"fmt"
	"testing"
)

// mockRunnerV2 は RunnerV2 のモック実装
type mockRunnerV2 struct {
	executeResult *ToolResponse
	executeErr    error
	listResult    []ToolMetadata
	listErr       error
}

func (m *mockRunnerV2) ExecuteV2(_ context.Context, _ string, _ map[string]any) (*ToolResponse, error) {
	return m.executeResult, m.executeErr
}

func (m *mockRunnerV2) ListTools(_ context.Context) ([]ToolMetadata, error) {
	return m.listResult, m.listErr
}

func TestLegacyRunner_Execute_Success(t *testing.T) {
	mock := &mockRunnerV2{
		executeResult: NewSuccess("hello world"),
	}
	legacy := NewLegacyRunner(mock)

	result, err := legacy.Execute(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("result = %q, want %q", result, "hello world")
	}
}

func TestLegacyRunner_Execute_V2Error(t *testing.T) {
	mock := &mockRunnerV2{
		executeResult: NewError(ErrNotFound, "file not found", nil),
	}
	legacy := NewLegacyRunner(mock)

	_, err := legacy.Execute(context.Background(), "test", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	toolErr, ok := err.(*ToolError)
	if !ok {
		t.Fatalf("expected *ToolError, got %T", err)
	}
	if toolErr.Code != ErrNotFound {
		t.Errorf("code = %s, want NOT_FOUND", toolErr.Code)
	}
}

func TestLegacyRunner_Execute_TransportError(t *testing.T) {
	mock := &mockRunnerV2{
		executeErr: fmt.Errorf("connection failed"),
	}
	legacy := NewLegacyRunner(mock)

	_, err := legacy.Execute(context.Background(), "test", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "connection failed" {
		t.Errorf("error = %q, want %q", err.Error(), "connection failed")
	}
}

func TestLegacyRunner_List(t *testing.T) {
	mock := &mockRunnerV2{
		listResult: []ToolMetadata{
			{ToolID: "shell", Version: "1.0.0"},
			{ToolID: "file_read", Version: "1.0.0"},
		},
	}
	legacy := NewLegacyRunner(mock)

	names, err := legacy.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("len = %d, want 2", len(names))
	}
	if names[0] != "shell" || names[1] != "file_read" {
		t.Errorf("names = %v, want [shell file_read]", names)
	}
}

func TestLegacyRunner_List_Error(t *testing.T) {
	mock := &mockRunnerV2{
		listErr: fmt.Errorf("list failed"),
	}
	legacy := NewLegacyRunner(mock)

	_, err := legacy.List(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}
