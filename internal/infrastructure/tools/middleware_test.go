package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestWithTimeout_Normal(t *testing.T) {
	fn := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "ok", nil
	}
	wrapped := withTimeout(fn, 5*time.Second)
	result, err := wrapped(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want %q", result, "ok")
	}
}

func TestWithTimeout_Exceeded(t *testing.T) {
	fn := func(ctx context.Context, args map[string]interface{}) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "should not reach", nil
		}
	}
	wrapped := withTimeout(fn, 50*time.Millisecond)
	_, err := wrapped(context.Background(), nil)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestWithPathValidation_Valid(t *testing.T) {
	fn := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "ok", nil
	}
	wrapped := withPathValidation(fn, "path")
	result, err := wrapped(context.Background(), map[string]interface{}{"path": "/safe/path"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want %q", result, "ok")
	}
}

func TestWithPathValidation_Traversal(t *testing.T) {
	fn := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "should not reach", nil
	}
	wrapped := withPathValidation(fn, "path")
	_, err := wrapped(context.Background(), map[string]interface{}{"path": "../etc/passwd"})
	if err == nil {
		t.Error("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "VALIDATION_FAILED") {
		t.Errorf("error should contain VALIDATION_FAILED, got: %v", err)
	}
}

func TestWithPathValidation_ControlChars(t *testing.T) {
	fn := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "should not reach", nil
	}
	wrapped := withPathValidation(fn, "path")
	_, err := wrapped(context.Background(), map[string]interface{}{"path": "/path/\x00evil"})
	if err == nil {
		t.Error("expected error for control chars")
	}
}

func TestWithPathValidation_Empty(t *testing.T) {
	fn := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "should not reach", nil
	}
	wrapped := withPathValidation(fn, "path")
	_, err := wrapped(context.Background(), map[string]interface{}{"path": ""})
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestWithPathValidation_MissingArg(t *testing.T) {
	fn := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "should not reach", nil
	}
	wrapped := withPathValidation(fn, "path")
	_, err := wrapped(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing arg")
	}
}

func TestWithStringValidation_Valid(t *testing.T) {
	fn := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "ok", nil
	}
	wrapped := withStringValidation(fn, "query", 100)
	result, err := wrapped(context.Background(), map[string]interface{}{"query": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want %q", result, "ok")
	}
}

func TestWithStringValidation_TooLong(t *testing.T) {
	fn := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "should not reach", nil
	}
	wrapped := withStringValidation(fn, "query", 5)
	_, err := wrapped(context.Background(), map[string]interface{}{"query": "toolong"})
	if err == nil {
		t.Error("expected error for too-long input")
	}
}

func TestWithStringValidation_Empty(t *testing.T) {
	fn := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "should not reach", nil
	}
	wrapped := withStringValidation(fn, "query", 100)
	_, err := wrapped(context.Background(), map[string]interface{}{"query": ""})
	if err == nil {
		t.Error("expected error for empty string")
	}
}
