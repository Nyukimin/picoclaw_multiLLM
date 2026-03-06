package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ToolRunner はツール実行の実装
type ToolRunner struct {
	tools  map[string]ToolFunc
	config ToolRunnerConfig
}

// ToolRunnerConfig はToolRunnerの設定
type ToolRunnerConfig struct {
	GoogleAPIKey         string
	GoogleSearchEngineID string
	HTTPClient           *http.Client           // テスト用注入（nilの場合はデフォルトを使用）
	Subagents            map[string]SubagentFunc // サブエージェントマップ（nil許容）
}

// ToolFunc はツール実行関数の型
type ToolFunc func(ctx context.Context, args map[string]interface{}) (string, error)

// NewToolRunner は新しいToolRunnerを作成
func NewToolRunner(config ToolRunnerConfig) *ToolRunner {
	runner := &ToolRunner{
		tools:  make(map[string]ToolFunc),
		config: config,
	}

	// ツール登録
	runner.registerTools()

	return runner
}

// registerTools は利用可能なツールを登録
func (r *ToolRunner) registerTools() {
	r.tools["shell"] = r.executeShell
	r.tools["file_read"] = r.executeFileRead
	r.tools["file_write"] = r.executeFileWrite
	r.tools["file_list"] = r.executeFileList
	r.tools["web_search"] = r.executeWebSearch
	if len(r.config.Subagents) > 0 {
		r.tools["subagent"] = r.executeSubagent
	}
}

// Execute はツールを実行
func (r *ToolRunner) Execute(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	toolFunc, exists := r.tools[toolName]
	if !exists {
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}

	return toolFunc(ctx, args)
}

// List は利用可能なツール一覧を返す
func (r *ToolRunner) List(ctx context.Context) ([]string, error) {
	tools := make([]string, 0, len(r.tools))
	for name := range r.tools {
		tools = append(tools, name)
	}
	return tools, nil
}

// executeShell はシェルコマンドを実行
func (r *ToolRunner) executeShell(ctx context.Context, args map[string]interface{}) (string, error) {
	command, ok := args["command"].(string)
	if !ok {
		return "", fmt.Errorf("'command' argument is required and must be a string")
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// executeFileRead はファイルを読み込む
func (r *ToolRunner) executeFileRead(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("'path' argument is required and must be a string")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}

// executeFileWrite はファイルに書き込む
func (r *ToolRunner) executeFileWrite(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("'path' argument is required and must be a string")
	}

	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("'content' argument is required and must be a string")
	}

	// ディレクトリが存在しない場合は作成
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// executeFileList はディレクトリ内のファイル一覧を取得
func (r *ToolRunner) executeFileList(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("'path' argument is required and must be a string")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	var result strings.Builder
	for i, entry := range entries {
		if entry.IsDir() {
			result.WriteString(fmt.Sprintf("%s/\n", entry.Name()))
		} else {
			result.WriteString(fmt.Sprintf("%s\n", entry.Name()))
		}
		if i >= 1000 {
			result.WriteString("... (truncated, too many entries)\n")
			break
		}
	}

	return result.String(), nil
}

// executeWebSearch はWeb検索を実行（Google Custom Search JSON API使用）
func (r *ToolRunner) executeWebSearch(ctx context.Context, args map[string]interface{}) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("'query' argument is required and must be a string")
	}

	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// 設定チェック
	if r.config.GoogleAPIKey == "" || r.config.GoogleSearchEngineID == "" {
		return "", fmt.Errorf("Google Search API not configured")
	}

	// Google Custom Search JSON API
	apiURL := fmt.Sprintf("https://www.googleapis.com/customsearch/v1?key=%s&cx=%s&q=%s",
		r.config.GoogleAPIKey,
		r.config.GoogleSearchEngineID,
		url.QueryEscape(query))

	// HTTPクライアント（注入がなければデフォルトを使用）
	client := r.config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("search API returned status %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// JSON解析
	var result GoogleSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// 結果フォーマット
	return formatGoogleSearchResult(result), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GoogleSearchResponse はGoogle Custom Search JSON APIレスポンス
type GoogleSearchResponse struct {
	Items []GoogleSearchItem `json:"items"`
	SearchInformation struct {
		TotalResults string `json:"totalResults"`
	} `json:"searchInformation"`
}

// GoogleSearchItem は検索結果アイテム
type GoogleSearchItem struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

// formatGoogleSearchResult は検索結果を整形
func formatGoogleSearchResult(result GoogleSearchResponse) string {
	var output strings.Builder

	if len(result.Items) == 0 {
		return "検索結果が見つかりませんでした。"
	}

	output.WriteString("🔍 検索結果:\n\n")

	// 最大5件の検索結果を表示
	maxResults := 5
	if len(result.Items) < maxResults {
		maxResults = len(result.Items)
	}

	for i := 0; i < maxResults; i++ {
		item := result.Items[i]
		output.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.Title))
		output.WriteString(fmt.Sprintf("   %s\n", item.Snippet))
		output.WriteString(fmt.Sprintf("   %s\n\n", item.Link))
	}

	return output.String()
}
