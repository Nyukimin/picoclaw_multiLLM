package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func TestExecuteProposal_Success_JSONPatch(t *testing.T) {
	// テスト用ワークスペース作成
	tmpDir := t.TempDir()

	cfg := config.WorkerConfig{
		AutoCommit:           false,
		CommitMessagePrefix:  "[Test]",
		CommandTimeout:       10,
		GitTimeout:           10,
		StopOnError:          false,
		Workspace:            tmpDir,
		ProtectedPatterns:    []string{".env*"},
		ActionOnProtected:    "error",
		ShowExecutionSummary: false,
	}

	service := NewWorkerExecutionService(cfg)

	// JSON形式のPatch
	jsonPatch := `[
		{
			"type": "file_edit",
			"action": "create",
			"target": "` + filepath.Join(tmpDir, "test.txt") + `",
			"content": "Hello, World!"
		}
	]`

	p := proposal.NewProposal("Test plan", jsonPatch, "Low risk", "Low cost")
	jobID := task.NewJobID()

	result, err := service.ExecuteProposal(context.Background(), jobID, p)
	if err != nil {
		t.Fatalf("ExecuteProposal failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success, but got failure")
	}

	if result.ExecutedCmds != 1 {
		t.Errorf("Expected 1 executed command, got %d", result.ExecutedCmds)
	}

	if result.FailedCmds != 0 {
		t.Errorf("Expected 0 failed commands, got %d", result.FailedCmds)
	}

	// ファイルが作成されたか確認
	content, err := os.ReadFile(filepath.Join(tmpDir, "test.txt"))
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	if string(content) != "Hello, World!" {
		t.Errorf("Expected 'Hello, World!', got '%s'", string(content))
	}
}

func TestExecuteProposal_Success_MarkdownPatch(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.WorkerConfig{
		AutoCommit:        false,
		CommandTimeout:    10,
		GitTimeout:        10,
		StopOnError:       false,
		Workspace:         tmpDir,
		ProtectedPatterns: []string{".env*"},
		ActionOnProtected: "error",
	}

	service := NewWorkerExecutionService(cfg)

	// Markdown形式のPatch
	testFilePath := filepath.Join(tmpDir, "hello.go")
	markdownPatch := "```go:" + testFilePath + "\npackage main\n\nfunc main() {\n\tprintln(\"Hello\")\n}\n```"

	p := proposal.NewProposal("Test plan", markdownPatch, "Low risk", "Low cost")
	jobID := task.NewJobID()

	result, err := service.ExecuteProposal(context.Background(), jobID, p)
	if err != nil {
		t.Fatalf("ExecuteProposal failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success, but got failure")
	}

	// ファイルが作成されたか確認
	content, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	expected := "package main\n\nfunc main() {\n\tprintln(\"Hello\")\n}\n"
	if string(content) != expected {
		t.Errorf("Expected '%s', got '%s'", expected, string(content))
	}
}

func TestExecuteProposal_ParseError(t *testing.T) {
	cfg := config.WorkerConfig{
		Workspace: t.TempDir(),
	}

	service := NewWorkerExecutionService(cfg)

	// 不正なPatch
	invalidPatch := "this is not valid JSON or Markdown"
	p := proposal.NewProposal("Test", invalidPatch, "", "")
	jobID := task.NewJobID()

	_, err := service.ExecuteProposal(context.Background(), jobID, p)
	if err == nil {
		t.Error("Expected parse error, but got nil")
	}
}

func TestExecuteFileEdit_Create(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.WorkerConfig{
		Workspace:         tmpDir,
		ProtectedPatterns: []string{},
		ActionOnProtected: "error",
	}

	service := &workerExecutionService{config: cfg}

	testFile := filepath.Join(tmpDir, "create_test.txt")
	jsonPatch := `[{"type": "file_edit", "action": "create", "target": "` + testFile + `", "content": "Created"}]`

	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, err := service.ExecuteProposal(context.Background(), jobID, p)
	if err != nil {
		t.Fatalf("ExecuteProposal failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}

	// ファイル確認
	content, _ := os.ReadFile(testFile)
	if string(content) != "Created" {
		t.Errorf("Expected 'Created', got '%s'", string(content))
	}
}

func TestExecuteFileEdit_Update(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "update_test.txt")

	// 既存ファイル作成
	os.WriteFile(testFile, []byte("Original"), 0644)

	cfg := config.WorkerConfig{
		Workspace:         tmpDir,
		ProtectedPatterns: []string{},
	}

	service := &workerExecutionService{config: cfg}

	jsonPatch := `[{"type": "file_edit", "action": "update", "target": "` + testFile + `", "content": "Updated"}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	if !result.Success {
		t.Error("Expected success")
	}

	content, _ := os.ReadFile(testFile)
	if string(content) != "Updated" {
		t.Errorf("Expected 'Updated', got '%s'", string(content))
	}
}

func TestExecuteFileEdit_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "delete_test.txt")

	// 既存ファイル作成
	os.WriteFile(testFile, []byte("ToDelete"), 0644)

	cfg := config.WorkerConfig{
		Workspace: tmpDir,
	}

	service := &workerExecutionService{config: cfg}

	jsonPatch := `[{"type": "file_edit", "action": "delete", "target": "` + testFile + `"}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	if !result.Success {
		t.Error("Expected success")
	}

	// ファイルが削除されたか確認
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("File should have been deleted")
	}
}

func TestExecuteFileEdit_Append(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "append_test.txt")

	// 既存ファイル作成
	os.WriteFile(testFile, []byte("Line1\n"), 0644)

	cfg := config.WorkerConfig{
		Workspace: tmpDir,
	}

	service := &workerExecutionService{config: cfg}

	jsonPatch := `[{"type": "file_edit", "action": "append", "target": "` + testFile + `", "content": "Line2\n"}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	if !result.Success {
		t.Error("Expected success")
	}

	content, _ := os.ReadFile(testFile)
	expected := "Line1\nLine2\n"
	if string(content) != expected {
		t.Errorf("Expected '%s', got '%s'", expected, string(content))
	}
}

func TestExecuteFileEdit_Mkdir(t *testing.T) {
	tmpDir := t.TempDir()
	newDir := filepath.Join(tmpDir, "newdir")

	cfg := config.WorkerConfig{
		Workspace: tmpDir,
	}

	service := &workerExecutionService{config: cfg}

	jsonPatch := `[{"type": "file_edit", "action": "mkdir", "target": "` + newDir + `"}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	if !result.Success {
		t.Error("Expected success")
	}

	// ディレクトリが作成されたか確認
	if info, err := os.Stat(newDir); err != nil || !info.IsDir() {
		t.Error("Directory should have been created")
	}
}

func TestExecuteShellCommand_Success(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.WorkerConfig{
		Workspace:      tmpDir,
		CommandTimeout: 10,
	}

	service := &workerExecutionService{config: cfg}

	jsonPatch := `[{"type": "shell_command", "action": "run", "target": "echo 'test'"}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	if !result.Success {
		t.Error("Expected success")
	}

	if len(result.Results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(result.Results))
	}

	if !result.Results[0].Success {
		t.Error("Shell command should succeed")
	}
}

func TestProtectedFile_Error(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.WorkerConfig{
		Workspace:         tmpDir,
		ProtectedPatterns: []string{".env*"},
		ActionOnProtected: "error",
	}

	service := &workerExecutionService{config: cfg}

	// 保護ファイルへの書き込み試行
	envFile := filepath.Join(tmpDir, ".env")
	jsonPatch := `[{"type": "file_edit", "action": "create", "target": "` + envFile + `", "content": "SECRET=xxx"}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	// 失敗すべき
	if result.Success {
		t.Error("Expected failure for protected file")
	}

	if result.FailedCmds != 1 {
		t.Errorf("Expected 1 failed command, got %d", result.FailedCmds)
	}
}

func TestWorkspaceRestriction_Error(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.WorkerConfig{
		Workspace: tmpDir,
	}

	service := &workerExecutionService{config: cfg}

	// workspace外への書き込み試行
	outsideFile := "/tmp/outside.txt"
	jsonPatch := `[{"type": "file_edit", "action": "create", "target": "` + outsideFile + `", "content": "Bad"}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	// 失敗すべき
	if result.Success {
		t.Error("Expected failure for outside workspace")
	}

	if result.FailedCmds != 1 {
		t.Errorf("Expected 1 failed command, got %d", result.FailedCmds)
	}
}

func TestExecuteFileEdit_Rename(t *testing.T) {
	tmpDir := t.TempDir()
	oldFile := filepath.Join(tmpDir, "old.txt")
	newFile := filepath.Join(tmpDir, "new.txt")

	// 既存ファイル作成
	os.WriteFile(oldFile, []byte("Content"), 0644)

	cfg := config.WorkerConfig{
		Workspace: tmpDir,
	}

	service := &workerExecutionService{config: cfg}

	jsonPatch := `[{
		"type": "file_edit",
		"action": "rename",
		"target": "` + oldFile + `",
		"metadata": {"new_name": "` + newFile + `"}
	}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	if !result.Success {
		t.Error("Expected success")
	}

	// 新しいファイルが存在し、古いファイルが消えているか確認
	if _, err := os.Stat(newFile); err != nil {
		t.Error("New file should exist")
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("Old file should not exist")
	}
}

func TestExecuteFileEdit_Copy(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "source.txt")
	destFile := filepath.Join(tmpDir, "dest.txt")

	// ソースファイル作成
	os.WriteFile(srcFile, []byte("Source content"), 0644)

	cfg := config.WorkerConfig{
		Workspace: tmpDir,
	}

	service := &workerExecutionService{config: cfg}

	jsonPatch := `[{
		"type": "file_edit",
		"action": "copy",
		"target": "` + srcFile + `",
		"metadata": {"destination": "` + destFile + `"}
	}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	if !result.Success {
		t.Error("Expected success")
	}

	// 両方のファイルが存在するか確認
	content, err := os.ReadFile(destFile)
	if err != nil || string(content) != "Source content" {
		t.Error("Destination file should have same content as source")
	}

	// ソースファイルも残っているはず
	if _, err := os.Stat(srcFile); err != nil {
		t.Error("Source file should still exist")
	}
}

func TestExecuteShellCommand_WithEnv(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.WorkerConfig{
		Workspace:      tmpDir,
		CommandTimeout: 10,
	}

	service := &workerExecutionService{config: cfg}

	jsonPatch := `[{
		"type": "shell_command",
		"action": "run",
		"target": "echo $TEST_VAR",
		"metadata": {"env": "TEST_VAR=HelloFromEnv"}
	}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	if !result.Success {
		t.Error("Expected success")
	}

	if len(result.Results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(result.Results))
	}

	// 環境変数が設定されて実行されたか確認（出力にHelloFromEnvが含まれる）
	if !strings.Contains(result.Results[0].Output, "HelloFromEnv") {
		t.Errorf("Expected output to contain 'HelloFromEnv', got: %s", result.Results[0].Output)
	}
}

func TestShowExecutionSummary(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.WorkerConfig{
		Workspace:            tmpDir,
		ShowExecutionSummary: true,
		CommandTimeout:       10,
	}

	service := NewWorkerExecutionService(cfg)

	jsonPatch := `[
		{"type": "file_edit", "action": "create", "target": "` + tmpDir + `/test1.txt", "content": "A"},
		{"type": "shell_command", "action": "run", "target": "echo test"}
	]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	// サマリが表示される（標準出力）
	result, err := service.ExecuteProposal(context.Background(), jobID, p)
	if err != nil {
		t.Fatalf("ExecuteProposal failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}

	// サマリが含まれているか（resultのSummaryフィールド）
	if result.Summary == "" {
		t.Error("Summary should not be empty when ShowExecutionSummary is true")
	}
}

func TestProtectedFile_Skip(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.WorkerConfig{
		Workspace:         tmpDir,
		ProtectedPatterns: []string{".env*"},
		ActionOnProtected: "skip", // skip mode
	}

	service := &workerExecutionService{config: cfg}

	envFile := filepath.Join(tmpDir, ".env")
	jsonPatch := `[{"type": "file_edit", "action": "create", "target": "` + envFile + `", "content": "SECRET=xxx"}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	// skipモードなので成功する（ただしファイルは作成されない）
	if !result.Success {
		t.Error("Expected success in skip mode")
	}

	// ファイルは作成されないはず
	if _, err := os.Stat(envFile); !os.IsNotExist(err) {
		t.Error("Protected file should not be created in skip mode")
	}
}

func TestProtectedFile_Log(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.WorkerConfig{
		Workspace:         tmpDir,
		ProtectedPatterns: []string{".env*"},
		ActionOnProtected: "log", // log mode
	}

	service := &workerExecutionService{config: cfg}

	envFile := filepath.Join(tmpDir, ".env")
	jsonPatch := `[{"type": "file_edit", "action": "create", "target": "` + envFile + `", "content": "SECRET=xxx"}]`
	p := proposal.NewProposal("", jsonPatch, "", "")
	jobID := task.NewJobID()

	result, _ := service.ExecuteProposal(context.Background(), jobID, p)

	// logモードなので成功する（警告ログが出る）
	if !result.Success {
		t.Error("Expected success in log mode")
	}

	// ファイルは作成される
	if _, err := os.Stat(envFile); err != nil {
		t.Error("File should be created in log mode")
	}
}

func TestStopOnError_vs_ContinueOnError(t *testing.T) {
	tmpDir := t.TempDir()

	// StopOnError=trueのテスト
	t.Run("StopOnError=true", func(t *testing.T) {
		cfg := config.WorkerConfig{
			Workspace:   tmpDir,
			StopOnError: true,
		}

		service := &workerExecutionService{config: cfg}

		// 最初は成功、2番目は失敗、3番目は実行されないはず
		file1 := filepath.Join(tmpDir, "file1.txt")
		file2 := filepath.Join(tmpDir, "file2.txt")

		jsonPatch := `[
			{"type": "file_edit", "action": "create", "target": "` + file1 + `", "content": "OK"},
			{"type": "file_edit", "action": "delete", "target": "/nonexistent/file.txt"},
			{"type": "file_edit", "action": "create", "target": "` + file2 + `", "content": "Should not execute"}
		]`

		p := proposal.NewProposal("", jsonPatch, "", "")
		jobID := task.NewJobID()

		result, _ := service.ExecuteProposal(context.Background(), jobID, p)

		if result.ExecutedCmds != 2 {
			t.Errorf("Expected 2 executed commands (stopped on error), got %d", result.ExecutedCmds)
		}

		if result.FailedCmds != 1 {
			t.Errorf("Expected 1 failed command, got %d", result.FailedCmds)
		}

		// file2は作成されないはず
		if _, err := os.Stat(file2); !os.IsNotExist(err) {
			t.Error("file2 should not have been created (execution stopped)")
		}
	})

	// StopOnError=falseのテスト
	t.Run("StopOnError=false", func(t *testing.T) {
		cfg := config.WorkerConfig{
			Workspace:   tmpDir,
			StopOnError: false,
		}

		service := &workerExecutionService{config: cfg}

		file3 := filepath.Join(tmpDir, "file3.txt")
		file4 := filepath.Join(tmpDir, "file4.txt")

		jsonPatch := `[
			{"type": "file_edit", "action": "create", "target": "` + file3 + `", "content": "OK"},
			{"type": "file_edit", "action": "delete", "target": "/nonexistent/file.txt"},
			{"type": "file_edit", "action": "create", "target": "` + file4 + `", "content": "Should execute"}
		]`

		p := proposal.NewProposal("", jsonPatch, "", "")
		jobID := task.NewJobID()

		result, _ := service.ExecuteProposal(context.Background(), jobID, p)

		if result.ExecutedCmds != 3 {
			t.Errorf("Expected 3 executed commands (continue on error), got %d", result.ExecutedCmds)
		}

		if result.FailedCmds != 1 {
			t.Errorf("Expected 1 failed command, got %d", result.FailedCmds)
		}

		// file4は作成されるはず
		if _, err := os.Stat(file4); err != nil {
			t.Error("file4 should have been created (execution continued)")
		}
	})
}
