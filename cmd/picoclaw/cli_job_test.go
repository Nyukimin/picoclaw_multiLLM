package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	longjobapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/longjob"
)

func testJobDeps(t *testing.T) (jobCLIDeps, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errOut bytes.Buffer
	store := longjobapp.NewFileStore(t.TempDir())
	svc := longjobapp.NewService(store, func() time.Time {
		return time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC)
	})
	return jobCLIDeps{service: svc, out: &out, errOut: &errOut}, &out, &errOut
}

func TestRunJobCommand_StartStatusResumeJSON(t *testing.T) {
	deps, out, errOut := testJobDeps(t)
	code := runJobCommand([]string{"start", "stock-learn", "--universe", "jp-liquid", "--json"}, deps)
	if code != 0 {
		t.Fatalf("start code=%d err=%s", code, errOut.String())
	}
	var payload struct {
		ID     string            `json:"id"`
		Kind   string            `json:"kind"`
		Status string            `json:"status"`
		Params map[string]string `json:"params"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid start json: %v", err)
	}
	if payload.ID == "" || payload.Kind != "stock-learn" || payload.Params["universe"] != "jp-liquid" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	out.Reset()
	errOut.Reset()
	code = runJobCommand([]string{"status", payload.ID}, deps)
	if code != 0 {
		t.Fatalf("status code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "next: define-scope") {
		t.Fatalf("status missing next step: %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	code = runJobCommand([]string{"resume", payload.ID}, deps)
	if code != 0 {
		t.Fatalf("resume code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "RenCrow Long Running Job Resume") || !strings.Contains(out.String(), "artifact:") {
		t.Fatalf("resume output missing prompt/artifact: %s", out.String())
	}
}

func TestRunJobCommand_CompleteStepAndReport(t *testing.T) {
	deps, out, errOut := testJobDeps(t)
	code := runJobCommand([]string{"start", "stock-learn", "--json"}, deps)
	if code != 0 {
		t.Fatalf("start code=%d err=%s", code, errOut.String())
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid start json: %v", err)
	}
	out.Reset()
	errOut.Reset()
	code = runJobCommand([]string{"complete-step", payload.ID, "--step", "define-scope", "--summary", "条件固定"}, deps)
	if code != 0 {
		t.Fatalf("complete-step code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "next: snapshot-data") {
		t.Fatalf("complete-step missing next step: %s", out.String())
	}
	out.Reset()
	errOut.Reset()
	code = runJobCommand([]string{"report", payload.ID}, deps)
	if code != 0 {
		t.Fatalf("report code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "RenCrow Long Running Job Report") || !strings.Contains(out.String(), "条件固定") {
		t.Fatalf("unexpected report: %s", out.String())
	}
}
