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

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
)

// ToolFuncV2 は構造化レスポンスを返すツール実行関数の型
type ToolFuncV2 func(ctx context.Context, args map[string]interface{}) (*tool.ToolResponse, error)

// ToolRunner はツール実行の実装（V1 + V2 対応）
type ToolRunner struct {
	tools    map[string]ToolFunc
	toolsV2  map[string]ToolFuncV2
	metadata map[string]tool.ToolMetadata
	config   ToolRunnerConfig
}

// ToolRunnerConfig はToolRunnerの設定
type ToolRunnerConfig struct {
	GoogleAPIKey         string
	GoogleSearchEngineID string
	HTTPClient           *http.Client           // テスト用注入（nilの場合はデフォルトを使用）
	Subagents            map[string]SubagentFunc // サブエージェントマップ（nil許容）
	AllowedShellCommands []string               // 許可コマンドプレフィックス（空=全許可）
}

// ToolFunc はツール実行関数の型
type ToolFunc func(ctx context.Context, args map[string]interface{}) (string, error)

// NewToolRunner は新しいToolRunnerを作成
func NewToolRunner(config ToolRunnerConfig) *ToolRunner {
	runner := &ToolRunner{
		tools:    make(map[string]ToolFunc),
		toolsV2:  make(map[string]ToolFuncV2),
		metadata: make(map[string]tool.ToolMetadata),
		config:   config,
	}

	// ツール登録
	runner.registerTools()

	return runner
}

// registerTools は利用可能なツールを登録（ミドルウェアで安全レール適用）
func (r *ToolRunner) registerTools() {
	// V1 ツール登録（既存互換）
	r.tools["shell"] = withTimeout(
		withStringValidation(r.executeShell, "command", 10000),
		30*time.Second,
	)
	r.tools["file_read"] = withTimeout(
		withPathValidation(r.executeFileRead, "path"),
		10*time.Second,
	)
	r.tools["file_write"] = withTimeout(
		withPathValidation(r.executeFileWrite, "path"),
		10*time.Second,
	)
	r.tools["file_list"] = withTimeout(
		withPathValidation(r.executeFileList, "path"),
		10*time.Second,
	)
	r.tools["web_search"] = withTimeout(
		withRetry(
			withStringValidation(r.executeWebSearch, "query", 500),
			DefaultRetryConfig,
		),
		15*time.Second,
	)
	if len(r.config.Subagents) > 0 {
		r.tools["subagent"] = withTimeout(r.executeSubagent, 30*time.Second)
	}

	// V2 ツール登録（V1 → ToolResponse 変換ラッパー）
	r.toolsV2["shell"] = v2Wrap(r.tools["shell"])
	r.toolsV2["file_read"] = v2Wrap(r.tools["file_read"])
	r.toolsV2["file_write"] = v2Wrap(r.tools["file_write"])
	r.toolsV2["file_list"] = v2Wrap(r.tools["file_list"])
	r.toolsV2["web_search"] = v2Wrap(r.tools["web_search"])
	if len(r.config.Subagents) > 0 {
		r.toolsV2["subagent"] = v2Wrap(r.tools["subagent"])
	}

	// メタデータ登録
	r.metadata["shell"] = tool.ToolMetadata{
		ToolID: "shell", Version: "1.0.0", Category: "mutation",
		RequiresApproval: true, DryRun: true,
	}
	r.metadata["file_read"] = tool.ToolMetadata{
		ToolID: "file_read", Version: "1.0.0", Category: "query",
	}
	r.metadata["file_write"] = tool.ToolMetadata{
		ToolID: "file_write", Version: "1.0.0", Category: "mutation",
		RequiresApproval: true, DryRun: true,
	}
	r.metadata["file_list"] = tool.ToolMetadata{
		ToolID: "file_list", Version: "1.0.0", Category: "query",
	}
	r.metadata["web_search"] = tool.ToolMetadata{
		ToolID: "web_search", Version: "1.0.0", Category: "query",
	}
	if len(r.config.Subagents) > 0 {
		r.metadata["subagent"] = tool.ToolMetadata{
			ToolID: "subagent", Version: "1.0.0", Category: "query",
		}
	}
}

// v2Wrap は V1 ToolFunc を V2 ToolFuncV2 に変換する
// V1 エラーが *ToolError を含む場合、そのコードを維持する
func v2Wrap(fn ToolFunc) ToolFuncV2 {
	return func(ctx context.Context, args map[string]interface{}) (*tool.ToolResponse, error) {
		result, err := fn(ctx, args)
		if err != nil {
			return classifyV1Error(err), nil
		}
		return tool.NewSuccess(result), nil
	}
}

// classifyV1Error は V1 エラーを適切な ErrorCode に分類する
func classifyV1Error(err error) *tool.ToolResponse {
	// ToolError がそのまま返された場合（ミドルウェアからのバリデーションエラー等）
	if te, ok := err.(*tool.ToolError); ok {
		return tool.NewError(te.Code, te.Message, te.Details)
	}

	// context.DeadlineExceeded → TIMEOUT
	if err == context.DeadlineExceeded {
		return tool.NewError(tool.ErrTimeout, err.Error(), nil)
	}

	// エラーメッセージによる分類
	msg := err.Error()
	switch {
	case strings.Contains(msg, "VALIDATION_FAILED"):
		return tool.NewError(tool.ErrValidationFailed, msg, nil)
	case strings.Contains(msg, "not found") || strings.Contains(msg, "no such file"):
		return tool.NewError(tool.ErrNotFound, msg, nil)
	case strings.Contains(msg, "permission denied"):
		return tool.NewError(tool.ErrPermissionDenied, msg, nil)
	case strings.Contains(msg, "timed out") || strings.Contains(msg, "deadline exceeded"):
		return tool.NewError(tool.ErrTimeout, msg, nil)
	default:
		return tool.NewError(tool.ErrInternalError, msg, nil)
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

// ExecuteV2 はツールを実行して構造化レスポンスを返す
func (r *ToolRunner) ExecuteV2(ctx context.Context, toolName string, args map[string]any) (*tool.ToolResponse, error) {
	v2Func, exists := r.toolsV2[toolName]
	if !exists {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
	return v2Func(ctx, args)
}

// ListTools はツールのメタデータ一覧を返す
func (r *ToolRunner) ListTools(ctx context.Context) ([]tool.ToolMetadata, error) {
	metas := make([]tool.ToolMetadata, 0, len(r.metadata))
	for _, m := range r.metadata {
		metas = append(metas, m)
	}
	return metas, nil
}

// executeShell はシェルコマンドを実行
func (r *ToolRunner) executeShell(ctx context.Context, args map[string]interface{}) (string, error) {
	command, ok := args["command"].(string)
	if !ok {
		return "", fmt.Errorf("'command' argument is required and must be a string")
	}

	// 許可コマンドリストチェック
	if len(r.config.AllowedShellCommands) > 0 {
		if !r.isShellCommandAllowed(command) {
			return "", &tool.ToolError{
				Code:    tool.ErrPermissionDenied,
				Message: "command not in allowed list",
				Details: map[string]any{"command": command},
			}
		}
	}

	// dry-run: コマンド表示のみ（実行しない）
	mode, _ := args["mode"].(string)
	if mode == "plan" {
		return fmt.Sprintf("[DRY-RUN] shell\ncommand: %s\naction: would execute via sh -c", command), nil
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// isShellCommandAllowed は許可コマンドリストに含まれるか判定する
func (r *ToolRunner) isShellCommandAllowed(command string) bool {
	trimmed := strings.TrimSpace(command)
	for _, prefix := range r.config.AllowedShellCommands {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// executeFileRead はファイルを読み込む（limit + offset 行制限対応）
func (r *ToolRunner) executeFileRead(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("'path' argument is required and must be a string")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// limit/offset が指定されている場合、行単位でスライス
	if _, hasLimit := args["limit"]; hasLimit {
		lines := strings.Split(string(content), "\n")
		total := len(lines)
		limit := intArg(args, "limit", 100)
		offset := intArg(args, "offset", 0)

		if limit > 10000 {
			limit = 10000
		}
		if offset < 0 {
			offset = 0
		}

		start := offset
		if start > total {
			start = total
		}
		end := start + limit
		if end > total {
			end = total
		}

		result := strings.Join(lines[start:end], "\n")
		if end < total {
			result += fmt.Sprintf("\n--- showing lines %d-%d of %d ---", start+1, end, total)
		}
		return result, nil
	}

	return string(content), nil
}

// executeFileWrite はファイルに書き込む（mode=plan で dry-run 対応）
func (r *ToolRunner) executeFileWrite(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("'path' argument is required and must be a string")
	}

	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("'content' argument is required and must be a string")
	}

	// dry-run モード: ファイル存在チェック + プレビューのみ
	mode, _ := args["mode"].(string)
	if mode == "plan" {
		return r.fileWriteDryRun(path, content), nil
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

// fileWriteDryRun はファイル書き込みの dry-run を実行
func (r *ToolRunner) fileWriteDryRun(path, content string) string {
	var result strings.Builder
	result.WriteString("[DRY-RUN] file_write\n")
	result.WriteString(fmt.Sprintf("path: %s\n", path))
	result.WriteString(fmt.Sprintf("content_size: %d bytes\n", len(content)))

	if info, err := os.Stat(path); err == nil {
		result.WriteString(fmt.Sprintf("exists: true (current size: %d bytes)\n", info.Size()))
		result.WriteString("action: overwrite\n")
	} else {
		result.WriteString("exists: false\n")
		result.WriteString("action: create\n")
	}

	// プレビュー（最大5行）
	lines := strings.SplitN(content, "\n", 6)
	if len(lines) > 5 {
		lines = lines[:5]
		result.WriteString("preview (first 5 lines):\n")
	} else {
		result.WriteString("preview:\n")
	}
	for _, line := range lines {
		if len(line) > 120 {
			line = line[:120] + "..."
		}
		result.WriteString("  " + line + "\n")
	}

	return result.String()
}

// executeFileList はディレクトリ内のファイル一覧を取得（limit + offset 対応）
func (r *ToolRunner) executeFileList(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("'path' argument is required and must be a string")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	// ページングパラメータ（デフォルト: limit=100, offset=0）
	limit := intArg(args, "limit", 100)
	offset := intArg(args, "offset", 0)

	// 上限制約
	if limit > 1000 {
		limit = 1000
	}
	if limit < 1 {
		limit = 1
	}
	if offset < 0 {
		offset = 0
	}

	total := len(entries)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	var result strings.Builder
	for _, entry := range entries[start:end] {
		if entry.IsDir() {
			fmt.Fprintf(&result, "%s/\n", entry.Name())
		} else {
			fmt.Fprintf(&result, "%s\n", entry.Name())
		}
	}

	// ページング情報
	if end < total {
		fmt.Fprintf(&result, "--- showing %d-%d of %d (next offset: %d) ---\n", start+1, end, total, end)
	}

	return result.String(), nil
}

// intArg は args から int 値を取得する（float64 からの変換対応、JSON 由来）
func intArg(args map[string]interface{}, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return defaultVal
	}
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
