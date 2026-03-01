package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/config"
)

func TestParsePatch_JSONFormat(t *testing.T) {
	tests := []struct {
		name      string
		patch     string
		wantCount int
		wantErr   bool
	}{
		{
			name: "valid JSON patch",
			patch: `[
				{"type": "file_edit", "action": "update", "target": "test.go", "content": "code"}
			]`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "empty JSON array",
			patch:     `[]`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:    "invalid JSON",
			patch:   `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands, err := parsePatch(tt.patch)
			if tt.wantErr && err == nil {
				t.Errorf("Expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.wantErr && len(commands) != tt.wantCount {
				t.Errorf("Expected %d commands, got %d", tt.wantCount, len(commands))
			}
		})
	}
}

func TestPatchCommand_JSONSerialization(t *testing.T) {
	tests := []struct {
		name string
		cmd  PatchCommand
	}{
		{
			name: "file_edit command",
			cmd: PatchCommand{
				Type:    "file_edit",
				Action:  "update",
				Target:  "pkg/agent/loop.go",
				Content: "package agent\n",
			},
		},
		{
			name: "shell_command",
			cmd: PatchCommand{
				Type:   "shell_command",
				Action: "run",
				Target: "go test ./pkg/...",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.cmd)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var decoded PatchCommand
			err = json.Unmarshal(data, &decoded)
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}
			if decoded.Type != tt.cmd.Type {
				t.Errorf("Type mismatch: got %s, want %s", decoded.Type, tt.cmd.Type)
			}
		})
	}
}

func TestExecuteFileEdit_CreateUpdate(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		content    string
		wantSubstr string
	}{
		{
			name:       "create new file",
			action:     "create",
			content:    "Hello World",
			wantSubstr: "written successfully",
		},
		{
			name:       "update existing file",
			action:     "update",
			content:    "Updated content",
			wantSubstr: "written successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			testFile := filepath.Join(tmpDir, "test.txt")

			cmd := PatchCommand{
				Type:    "file_edit",
				Action:  tt.action,
				Target:  testFile,
				Content: tt.content,
			}

			al := &AgentLoop{workspace: tmpDir}
			output, err := al.executeFileEdit(context.Background(), cmd)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !strings.Contains(output, tt.wantSubstr) {
				t.Errorf("Expected %q in output, got %q", tt.wantSubstr, output)
			}

			content, _ := os.ReadFile(testFile)
			if string(content) != tt.content {
				t.Errorf("File content mismatch: got %q, want %q", string(content), tt.content)
			}
		})
	}
}

func TestExecuteFileEdit_DeleteAppend(t *testing.T) {
	tests := []struct {
		name      string
		action    string
		content   string
		checkFunc func(t *testing.T, testFile string)
	}{
		{
			name:   "delete file",
			action: "delete",
			checkFunc: func(t *testing.T, testFile string) {
				if _, err := os.Stat(testFile); !os.IsNotExist(err) {
					t.Errorf("File should be deleted")
				}
			},
		},
		{
			name:    "append to file",
			action:  "append",
			content: "\nAppended",
			checkFunc: func(t *testing.T, testFile string) {
				content, _ := os.ReadFile(testFile)
				if !strings.Contains(string(content), "Appended") {
					t.Errorf("Appended text not found")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			testFile := filepath.Join(tmpDir, "test.txt")
			os.WriteFile(testFile, []byte("Original"), 0644)

			cmd := PatchCommand{
				Type:    "file_edit",
				Action:  tt.action,
				Target:  testFile,
				Content: tt.content,
			}

			al := &AgentLoop{workspace: tmpDir}
			_, err := al.executeFileEdit(context.Background(), cmd)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, testFile)
			}
		})
	}
}

func TestExecuteShellCommand(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantErr    bool
		wantSubstr string
	}{
		{
			name:       "successful echo",
			target:     "echo 'hello'",
			wantSubstr: "hello",
		},
		{
			name:    "failed command",
			target:  "ls /nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			al := &AgentLoop{workspace: tmpDir}

			cmd := PatchCommand{
				Type:   "shell_command",
				Action: "run",
				Target: tt.target,
			}

			output, err := al.executeShellCommand(context.Background(), cmd)

			if tt.wantErr && err == nil {
				t.Errorf("Expected error")
			}
			if !tt.wantErr && !strings.Contains(output, tt.wantSubstr) {
				t.Errorf("Expected %q in output, got %q", tt.wantSubstr, output)
			}
		})
	}
}

func TestExecuteCommand_Dispatcher(t *testing.T) {
	tests := []struct {
		name    string
		cmdType string
		action  string
		target  string
		wantErr bool
	}{
		{
			name:    "dispatch to file_edit",
			cmdType: "file_edit",
			action:  "create",
			target:  "test.txt",
		},
		{
			name:    "dispatch to shell_command",
			cmdType: "shell_command",
			action:  "run",
			target:  "echo test",
		},
		{
			name:    "unknown type",
			cmdType: "unknown",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cmd := PatchCommand{
				Type:   tt.cmdType,
				Action: tt.action,
				Target: tt.target,
			}

			if tt.cmdType == "file_edit" {
				cmd.Target = filepath.Join(tmpDir, tt.target)
				cmd.Content = "test"
			}

			al := &AgentLoop{workspace: tmpDir}
			_, err := al.executeCommand(context.Background(), cmd)

			if tt.wantErr && err == nil {
				t.Errorf("Expected error")
			}
		})
	}
}

func TestExecuteFileEdit_SecurityViolation(t *testing.T) {
	tmpDir := t.TempDir()
	al := &AgentLoop{workspace: tmpDir}

	cmd := PatchCommand{
		Type:   "file_edit",
		Action: "create",
		Target: "/etc/passwd",
	}

	_, err := al.executeFileEdit(context.Background(), cmd)
	if err == nil {
		t.Errorf("Expected security error")
	}
	if !strings.Contains(err.Error(), "outside workspace") {
		t.Errorf("Expected workspace security error, got: %v", err)
	}
}

func TestPatchExecutionResult_JSONSerialization(t *testing.T) {
	result := PatchExecutionResult{
		Success:      true,
		ExecutedCmds: 2,
		FailedCmds:   0,
		Summary:      "test summary",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded PatchExecutionResult
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if decoded.ExecutedCmds != 2 {
		t.Errorf("ExecutedCmds mismatch: got %d, want 2", decoded.ExecutedCmds)
	}
	if decoded.Summary != "test summary" {
		t.Errorf("Summary mismatch: got %q, want %q", decoded.Summary, "test summary")
	}
}

func TestExecuteWorkerPatch_Integration(t *testing.T) {
	tests := []struct {
		name             string
		patch            string
		wantSuccess      bool
		wantExecutedCmds int
	}{
		{
			name: "successful multi-command",
			patch: `[
				{"type": "file_edit", "action": "create", "target": "test1.txt", "content": "Hello"},
				{"type": "file_edit", "action": "create", "target": "test2.txt", "content": "World"}
			]`,
			wantSuccess:      true,
			wantExecutedCmds: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			patchWithPaths := strings.ReplaceAll(tt.patch, "test1.txt", filepath.Join(tmpDir, "test1.txt"))
			patchWithPaths = strings.ReplaceAll(patchWithPaths, "test2.txt", filepath.Join(tmpDir, "test2.txt"))

			al := &AgentLoop{
				workspace: tmpDir,
				cfg: &config.Config{
					Worker: config.WorkerConfig{AutoCommit: false},
				},
			}

			result, err := al.executeWorkerPatch(context.Background(), patchWithPaths, "test-session")

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result.Success != tt.wantSuccess {
				t.Errorf("Success mismatch: got %v, want %v", result.Success, tt.wantSuccess)
			}
			if result.ExecutedCmds != tt.wantExecutedCmds {
				t.Errorf("ExecutedCmds: got %d, want %d", result.ExecutedCmds, tt.wantExecutedCmds)
			}
		})
	}
}

func TestParsePatch_MarkdownFormat(t *testing.T) {
	tests := []struct {
		name       string
		patch      string
		wantCount  int
		wantType   string
		wantTarget string
	}{
		{
			name:       "Markdown file edit with go syntax",
			patch:      "```go:pkg/agent/test.go\npackage agent\n\nfunc Test() {}\n```",
			wantCount:  1,
			wantType:   "file_edit",
			wantTarget: "pkg/agent/test.go",
		},
		{
			name:       "Markdown shell command with bash",
			patch:      "```bash\ngo test ./pkg/...\n```",
			wantCount:  1,
			wantType:   "shell_command",
			wantTarget: "go test ./pkg/...",
		},
		{
			name:       "Markdown shell command with sh",
			patch:      "```sh\necho hello\n```",
			wantCount:  1,
			wantType:   "shell_command",
			wantTarget: "echo hello",
		},
		{
			name: "Mixed markdown with multiple blocks",
			patch: `## Changes

` + "```go:pkg/agent/loop.go\n" + `package agent
` + "```\n\n" + `Run tests:
` + "```bash\n" + `go test
` + "```",
			wantCount: 2,
		},
		{
			name:      "Markdown with no code blocks",
			patch:     "Just some text\nNo code here",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands, err := parsePatch(tt.patch)

			if tt.wantCount == 0 {
				if err == nil {
					t.Errorf("Expected error for patch with no code blocks")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if len(commands) != tt.wantCount {
				t.Errorf("Count mismatch: got %d, want %d", len(commands), tt.wantCount)
			}
			if tt.wantType != "" && commands[0].Type != tt.wantType {
				t.Errorf("Type mismatch: got %s, want %s", commands[0].Type, tt.wantType)
			}
			if tt.wantTarget != "" && commands[0].Target != tt.wantTarget {
				t.Errorf("Target mismatch: got %s, want %s", commands[0].Target, tt.wantTarget)
			}
		})
	}
}
