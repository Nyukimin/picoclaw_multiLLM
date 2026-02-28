package mcp

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestClient_Ping(t *testing.T) {
	// テスト用の MCP サーバーが起動していることを前提
	// 環境変数 MCP_TEST_URL がない場合はスキップ
	baseURL := os.Getenv("MCP_TEST_URL")
	if baseURL == "" {
		baseURL = "http://100.83.235.65:12306"
	}

	client := NewClient(baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Ping(ctx)
	if err != nil {
		t.Skipf("MCP server not available: %v", err)
	}
}

func TestClient_ListTools(t *testing.T) {
	baseURL := os.Getenv("MCP_TEST_URL")
	if baseURL == "" {
		baseURL = "http://100.83.235.65:12306"
	}

	client := NewClient(baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.ListTools(ctx)
	if err != nil {
		t.Skipf("MCP server not available: %v", err)
	}

	if len(resp.Tools) == 0 {
		t.Error("Expected at least one tool")
	}

	t.Logf("Available tools: %d", len(resp.Tools))
	for _, tool := range resp.Tools {
		t.Logf("  - %s: %s", tool.Name, tool.Description)
	}
}
