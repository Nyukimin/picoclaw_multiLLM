package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLLMOpsTokenEnvFileAtLoadsToken(t *testing.T) {
	t.Setenv(llmOpsTokenEnvName, "")
	path := filepath.Join(t.TempDir(), "llm_ops.env")
	if err := os.WriteFile(path, []byte("# local secret\nLLM_OPS_TOKEN=abc123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, loadedPath, err := loadLLMOpsTokenEnvFileAt(path)
	if err != nil {
		t.Fatalf("loadLLMOpsTokenEnvFileAt returned error: %v", err)
	}
	if !loaded {
		t.Fatal("expected token to be loaded")
	}
	if loadedPath != path {
		t.Fatalf("expected path %q, got %q", path, loadedPath)
	}
	if got := os.Getenv(llmOpsTokenEnvName); got != "abc123" {
		t.Fatalf("expected token env to be set, got %q", got)
	}
}

func TestLoadLLMOpsTokenEnvFileAtDoesNotOverrideExistingEnv(t *testing.T) {
	t.Setenv(llmOpsTokenEnvName, "existing")
	path := filepath.Join(t.TempDir(), "llm_ops.env")
	if err := os.WriteFile(path, []byte("LLM_OPS_TOKEN=file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, _, err := loadLLMOpsTokenEnvFileAt(path)
	if err != nil {
		t.Fatalf("loadLLMOpsTokenEnvFileAt returned error: %v", err)
	}
	if loaded {
		t.Fatal("expected existing env to win")
	}
	if got := os.Getenv(llmOpsTokenEnvName); got != "existing" {
		t.Fatalf("expected existing token, got %q", got)
	}
}

func TestLoadLLMOpsTokenEnvFileAtAcceptsExportAndQuotes(t *testing.T) {
	t.Setenv(llmOpsTokenEnvName, "")
	path := filepath.Join(t.TempDir(), "llm_ops.env")
	if err := os.WriteFile(path, []byte("export LLM_OPS_TOKEN=\"quoted-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, _, err := loadLLMOpsTokenEnvFileAt(path)
	if err != nil {
		t.Fatalf("loadLLMOpsTokenEnvFileAt returned error: %v", err)
	}
	if !loaded {
		t.Fatal("expected token to be loaded")
	}
	if got := os.Getenv(llmOpsTokenEnvName); got != "quoted-token" {
		t.Fatalf("expected quoted token to be unwrapped, got %q", got)
	}
}
