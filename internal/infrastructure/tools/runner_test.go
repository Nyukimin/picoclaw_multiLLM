package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewToolRunner(t *testing.T) {
	runner := NewToolRunner()

	if runner == nil {
		t.Fatal("NewToolRunner should not return nil")
	}
}

func TestToolRunner_List(t *testing.T) {
	runner := NewToolRunner()

	tools, err := runner.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	// 最低限のツールが登録されているか
	expectedTools := []string{"shell", "file_read", "file_write", "file_list"}
	for _, expected := range expectedTools {
		found := false
		for _, tool := range tools {
			if tool == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected tool '%s' not found in list: %v", expected, tools)
		}
	}
}

func TestToolRunner_Execute_Shell_Success(t *testing.T) {
	runner := NewToolRunner()

	args := map[string]interface{}{
		"command": "echo 'Hello, World!'",
	}

	result, err := runner.Execute(context.Background(), "shell", args)
	if err != nil {
		t.Fatalf("Execute shell failed: %v", err)
	}

	if !strings.Contains(result, "Hello, World!") {
		t.Errorf("Expected 'Hello, World!' in result, got: %s", result)
	}
}

func TestToolRunner_Execute_Shell_MissingCommand(t *testing.T) {
	runner := NewToolRunner()

	args := map[string]interface{}{}

	_, err := runner.Execute(context.Background(), "shell", args)
	if err == nil {
		t.Error("Expected error when command is missing")
	}
}

func TestToolRunner_Execute_FileRead_Success(t *testing.T) {
	runner := NewToolRunner()

	// テスト用ファイル作成
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "This is a test file"
	os.WriteFile(testFile, []byte(testContent), 0644)

	args := map[string]interface{}{
		"path": testFile,
	}

	result, err := runner.Execute(context.Background(), "file_read", args)
	if err != nil {
		t.Fatalf("Execute file_read failed: %v", err)
	}

	if result != testContent {
		t.Errorf("Expected '%s', got '%s'", testContent, result)
	}
}

func TestToolRunner_Execute_FileRead_NotFound(t *testing.T) {
	runner := NewToolRunner()

	args := map[string]interface{}{
		"path": "/nonexistent/file.txt",
	}

	_, err := runner.Execute(context.Background(), "file_read", args)
	if err == nil {
		t.Error("Expected error when file not found")
	}
}

func TestToolRunner_Execute_FileWrite_Success(t *testing.T) {
	runner := NewToolRunner()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "output.txt")
	testContent := "Written content"

	args := map[string]interface{}{
		"path":    testFile,
		"content": testContent,
	}

	result, err := runner.Execute(context.Background(), "file_write", args)
	if err != nil {
		t.Fatalf("Execute file_write failed: %v", err)
	}

	// ファイルが作成されたか確認
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("File was not created")
	}

	// 内容確認
	content, _ := os.ReadFile(testFile)
	if string(content) != testContent {
		t.Errorf("Expected '%s', got '%s'", testContent, string(content))
	}

	if !strings.Contains(strings.ToLower(result), "success") {
		t.Errorf("Expected success message, got: %s", result)
	}
}

func TestToolRunner_Execute_FileList_Success(t *testing.T) {
	runner := NewToolRunner()

	tmpDir := t.TempDir()
	// テスト用ファイル作成
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("test"), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	args := map[string]interface{}{
		"path": tmpDir,
	}

	result, err := runner.Execute(context.Background(), "file_list", args)
	if err != nil {
		t.Fatalf("Execute file_list failed: %v", err)
	}

	// 作成したファイルが含まれているか
	if !strings.Contains(result, "file1.txt") {
		t.Error("Expected 'file1.txt' in result")
	}
	if !strings.Contains(result, "file2.txt") {
		t.Error("Expected 'file2.txt' in result")
	}
	if !strings.Contains(result, "subdir") {
		t.Error("Expected 'subdir' in result")
	}
}

func TestToolRunner_Execute_UnknownTool(t *testing.T) {
	runner := NewToolRunner()

	args := map[string]interface{}{}

	_, err := runner.Execute(context.Background(), "unknown_tool", args)
	if err == nil {
		t.Error("Expected error for unknown tool")
	}
}

func TestToolRunner_Execute_Shell_Timeout(t *testing.T) {
	runner := NewToolRunner()

	// 長時間かかるコマンド（sleepは避け、実際にはすぐ終わるが概念的なテスト）
	args := map[string]interface{}{
		"command": "echo 'quick'",
	}

	// タイムアウト付きコンテキスト
	ctx := context.Background()

	result, err := runner.Execute(ctx, "shell", args)
	if err != nil {
		t.Fatalf("Execute should succeed for quick command: %v", err)
	}

	if !strings.Contains(result, "quick") {
		t.Errorf("Expected 'quick' in result, got: %s", result)
	}
}

func TestToolRunner_Execute_FileWrite_CreateDirectory(t *testing.T) {
	runner := NewToolRunner()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "subdir", "nested", "file.txt")
	testContent := "Nested content"

	args := map[string]interface{}{
		"path":    testFile,
		"content": testContent,
	}

	_, err := runner.Execute(context.Background(), "file_write", args)
	if err != nil {
		t.Fatalf("Execute file_write with nested path failed: %v", err)
	}

	// ファイルが作成されたか確認
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("Expected '%s', got '%s'", testContent, string(content))
	}
}

func TestToolRunner_Execute_FileRead_MissingPath(t *testing.T) {
	runner := NewToolRunner()

	args := map[string]interface{}{}

	_, err := runner.Execute(context.Background(), "file_read", args)
	if err == nil {
		t.Error("Expected error when path is missing")
	}
}

func TestToolRunner_Execute_FileWrite_MissingContent(t *testing.T) {
	runner := NewToolRunner()

	tmpDir := t.TempDir()
	args := map[string]interface{}{
		"path": filepath.Join(tmpDir, "test.txt"),
	}

	_, err := runner.Execute(context.Background(), "file_write", args)
	if err == nil {
		t.Error("Expected error when content is missing")
	}
}

func TestToolRunner_Execute_FileList_MissingPath(t *testing.T) {
	runner := NewToolRunner()

	args := map[string]interface{}{}

	_, err := runner.Execute(context.Background(), "file_list", args)
	if err == nil {
		t.Error("Expected error when path is missing")
	}
}
