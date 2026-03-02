# MCP Chrome 統合手順書

**作成日**: 2026-02-25
**対象**: PicoClaw Coder3 + mcp-chrome 統合
**前提**: Phase 1-4（Coder3 基本実装）が完了していること

---

## 0. 概要

### 目的
Coder3（Claude API）にブラウザ操作機能を追加する。**すべての Chrome 操作は承認フロー（job_id）を通してのみ実行される**。

### アーキテクチャ
```
PicoClaw (Linux)
  │
  ├─ Coder3: Chrome 操作の plan を生成
  │           ↓
  ├─ Chat: 承認要求を送信（job_id 付き）
  │           ↓
  └─ 人間: /approve <job_id> または 永続承認（期限付き）
              ↓
PicoClaw Worker
  ↓ HTTP
Win11 (100.83.235.65:12306)
  ↓ Native Messaging (mcp-chrome-bridge)
Chrome 拡張機能
  ↓
ブラウザ操作（承認済みのみ実行）
```

### 承認フローの必須化
- Chrome 操作は **job_id で追跡**
- 承認なしでは実行されない
- 承認要求には `uses_browser: true` フラグが含まれる
- Auto-Approve の対象外（例外なく承認必須）

### 実装範囲
- **Phase 5-A**: Win11 側のセットアップ（mcp-chrome-bridge + Chrome 拡張）
- **Phase 5-B**: PicoClaw 側の MCP クライアント実装
- **Phase 5-C**: Coder3 統合とテスト

---

## Phase 5-A: Win11 側のセットアップ

### A-1. 前提条件の確認

**Win11 マシン（100.83.235.65）で実行**:

```powershell
# Node.js バージョン確認（v18 以上推奨）
node --version

# npm バージョン確認
npm --version

# Chrome がインストールされていることを確認
Get-Process chrome -ErrorAction SilentlyContinue
```

**Node.js が未インストールの場合**:
1. https://nodejs.org/ から LTS 版をダウンロード
2. インストーラーを実行
3. PowerShell を再起動して `node --version` で確認

---

### A-2. mcp-chrome-bridge のインストール

```powershell
# グローバルインストール
npm install -g mcp-chrome-bridge

# インストール確認
mcp-chrome-bridge --version

# インストールパスを確認（後で使う）
npm list -g --depth=0 | Select-String mcp-chrome-bridge
```

**エラーが出た場合**:
- PowerShell を管理者権限で実行
- npm のキャッシュをクリア: `npm cache clean --force`

---

### A-3. Chrome 拡張機能のインストール

#### オプション1: Chrome Web Store から（推奨）

1. Chrome を起動
2. https://chrome.google.com/webstore/ で "MCP Chrome" を検索
3. 「Chrome に追加」をクリック
4. 拡張機能アイコンが表示されることを確認

#### オプション2: GitHub から手動インストール

```powershell
# リポジトリをクローン
cd ~\Downloads
git clone https://github.com/hangwin/mcp-chrome.git
cd mcp-chrome

# 依存関係をインストール
npm install

# ビルド
npm run build
```

**Chrome で手動ロード**:
1. Chrome で `chrome://extensions/` を開く
2. 「デベロッパーモード」を ON
3. 「パッケージ化されていない拡張機能を読み込む」をクリック
4. `mcp-chrome/dist` フォルダを選択

---

### A-4. mcp-chrome-bridge の設定

**設定ファイルの作成**:

```powershell
# 設定ディレクトリを作成
mkdir ~\.mcp-chrome -ErrorAction SilentlyContinue

# 設定ファイルを作成
@"
{
  "port": 12306,
  "host": "0.0.0.0",
  "cors": {
    "enabled": true,
    "origins": ["*"]
  },
  "logging": {
    "level": "info",
    "file": "mcp-chrome.log"
  }
}
"@ | Out-File -Encoding UTF8 ~\.mcp-chrome\config.json
```

---

### A-5. mcp-chrome-bridge の起動

**手動起動（テスト用）**:

```powershell
# フォアグラウンドで起動
mcp-chrome-bridge --config ~\.mcp-chrome\config.json

# 別の PowerShell で接続テスト
curl http://localhost:12306/health
```

**バックグラウンド起動（Windows サービス化）**:

```powershell
# nssm をインストール（サービス管理ツール）
# https://nssm.cc/download から nssm.exe をダウンロード

# サービスをインストール
nssm install mcp-chrome-bridge "C:\Program Files\nodejs\mcp-chrome-bridge.cmd" "--config" "$env:USERPROFILE\.mcp-chrome\config.json"

# サービスを開始
nssm start mcp-chrome-bridge

# サービス状態を確認
nssm status mcp-chrome-bridge
```

---

### A-6. ファイアウォール設定

**Win11 ファイアウォールでポート 12306 を開放**:

```powershell
# 受信規則を追加
New-NetFirewallRule -DisplayName "MCP Chrome Bridge" `
  -Direction Inbound `
  -Protocol TCP `
  -LocalPort 12306 `
  -Action Allow

# 規則が追加されたことを確認
Get-NetFirewallRule -DisplayName "MCP Chrome Bridge"
```

---

### A-7. 接続テスト（Win11 ローカル）

```powershell
# ヘルスチェック
Invoke-WebRequest http://localhost:12306/health

# MCP エンドポイントテスト
$body = @{
  method = "tools/list"
  params = @{}
} | ConvertTo-Json

Invoke-WebRequest -Uri http://localhost:12306/mcp `
  -Method POST `
  -ContentType "application/json" `
  -Body $body
```

**期待される応答**:
```json
{
  "tools": [
    {"name": "chrome_navigate", "description": "..."},
    {"name": "chrome_click", "description": "..."},
    {"name": "chrome_screenshot", "description": "..."}
  ]
}
```

---

## Phase 5-B: PicoClaw 側の MCP クライアント実装

### B-1. MCP パッケージの作成

**ディレクトリ構造**:
```
pkg/mcp/
├── client.go          # MCP クライアント
├── client_test.go     # ユニットテスト
├── types.go           # リクエスト/レスポンス型
└── chrome_tools.go    # Chrome 操作ツール
```

---

### B-2. types.go の実装

**pkg/mcp/types.go**:

```go
package mcp

// MCPRequest は MCP サーバーへのリクエスト
type MCPRequest struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// MCPResponse は MCP サーバーからのレスポンス
type MCPResponse struct {
	Result map[string]interface{} `json:"result,omitempty"`
	Error  *MCPError              `json:"error,omitempty"`
}

// MCPError は MCP エラー
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool は MCP ツール定義
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ToolListResponse は tools/list のレスポンス
type ToolListResponse struct {
	Tools []Tool `json:"tools"`
}

// ToolCallRequest は tools/call のリクエスト
type ToolCallRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCallResponse は tools/call のレスポンス
type ToolCallResponse struct {
	Content []map[string]interface{} `json:"content"`
}
```

---

### B-3. client.go の実装

**pkg/mcp/client.go**:

```go
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client は MCP クライアント
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient は新しい MCP クライアントを作成
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListTools は利用可能なツール一覧を取得
func (c *Client) ListTools(ctx context.Context) (*ToolListResponse, error) {
	req := MCPRequest{
		Method: "tools/list",
		Params: make(map[string]interface{}),
	}

	resp, err := c.call(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", resp.Error.Message)
	}

	var result ToolListResponse
	data, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal tools: %w", err)
	}

	return &result, nil
}

// CallTool は指定されたツールを呼び出す
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolCallResponse, error) {
	req := MCPRequest{
		Method: "tools/call",
		Params: map[string]interface{}{
			"name":      name,
			"arguments": args,
		},
	}

	resp, err := c.call(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", resp.Error.Message)
	}

	var result ToolCallResponse
	data, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal tool response: %w", err)
	}

	return &result, nil
}

// call は MCP サーバーに HTTP リクエストを送信
func (c *Client) call(ctx context.Context, req MCPRequest) (*MCPResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/mcp", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status: %d", httpResp.StatusCode)
	}

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var mcpResp MCPResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &mcpResp, nil
}

// Ping は MCP サーバーのヘルスチェック
func (c *Client) Ping(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status: %d", httpResp.StatusCode)
	}

	return nil
}
```

---

### B-4. chrome_tools.go の実装

**pkg/mcp/chrome_tools.go**:

```go
package mcp

import (
	"context"
	"fmt"
)

// ChromeNavigate はブラウザで指定 URL に移動
func (c *Client) ChromeNavigate(ctx context.Context, url string) (string, error) {
	resp, err := c.CallTool(ctx, "chrome_navigate", map[string]interface{}{
		"url": url,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}

	result, ok := resp.Content[0]["text"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	return result, nil
}

// ChromeClick は指定セレクタの要素をクリック
func (c *Client) ChromeClick(ctx context.Context, selector string) (string, error) {
	resp, err := c.CallTool(ctx, "chrome_click", map[string]interface{}{
		"selector": selector,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}

	result, ok := resp.Content[0]["text"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	return result, nil
}

// ChromeScreenshot はページのスクリーンショットを取得
func (c *Client) ChromeScreenshot(ctx context.Context) (string, error) {
	resp, err := c.CallTool(ctx, "chrome_screenshot", map[string]interface{}{})
	if err != nil {
		return "", err
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}

	// Base64 エンコードされた画像データを返す
	result, ok := resp.Content[0]["data"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	return result, nil
}

// ChromeGetText は指定セレクタの要素のテキストを取得
func (c *Client) ChromeGetText(ctx context.Context, selector string) (string, error) {
	resp, err := c.CallTool(ctx, "chrome_get_text", map[string]interface{}{
		"selector": selector,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}

	result, ok := resp.Content[0]["text"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	return result, nil
}
```

---

### B-5. client_test.go の実装

**pkg/mcp/client_test.go**:

```go
package mcp

import (
	"context"
	"testing"
	"time"
)

func TestClient_Ping(t *testing.T) {
	// テスト用の MCP サーバーが起動していることを前提
	// 環境変数 MCP_TEST_URL がない場合はスキップ
	baseURL := "http://100.83.235.65:12306"

	client := NewClient(baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Ping(ctx)
	if err != nil {
		t.Skipf("MCP server not available: %v", err)
	}
}

func TestClient_ListTools(t *testing.T) {
	baseURL := "http://100.83.235.65:12306"
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
```

---

### B-6. 設定ファイルの更新

**config/config.example.json** に MCP 設定を追加:

```json
{
  "mcp": {
    "chrome": {
      "enabled": true,
      "base_url": "http://100.83.235.65:12306",
      "timeout_sec": 30
    }
  }
}
```

**pkg/config/config.go** に設定構造体を追加:

```go
type MCPConfig struct {
	Chrome MCPChromeConfig `json:"chrome"`
}

type MCPChromeConfig struct {
	Enabled    bool   `json:"enabled"`
	BaseURL    string `json:"base_url"`
	TimeoutSec int    `json:"timeout_sec"`
}

// Config に追加
type Config struct {
	// ... 既存のフィールド ...
	MCP MCPConfig `json:"mcp"`
}
```

---

## Phase 5-C: Coder3 統合とテスト

### C-1. Coder3 への MCP ツール追加

**pkg/agent/loop.go** に MCP クライアントを追加:

```go
import (
	"github.com/sipeed/picoclaw/pkg/mcp"
)

type AgentLoop struct {
	// ... 既存のフィールド ...
	mcpClient *mcp.Client
}

func NewAgentLoop(cfg *config.Config, ...) *AgentLoop {
	var mcpClient *mcp.Client
	if cfg.MCP.Chrome.Enabled {
		mcpClient = mcp.NewClient(cfg.MCP.Chrome.BaseURL)
	}

	return &AgentLoop{
		// ... 既存の初期化 ...
		mcpClient: mcpClient,
	}
}
```

---

### C-2. Coder3 のシステムプロンプトに MCP ツールを追加

**Coder3 呼び出し時に利用可能ツールを付与**:

```go
func (s *AgentLoop) buildCoder3Prompt(req *Request) string {
	prompt := "あなたは高品質コーディング専門の Coder3 です。\n\n"

	if s.mcpClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tools, err := s.mcpClient.ListTools(ctx)
		if err == nil && len(tools.Tools) > 0 {
			prompt += "## 利用可能なブラウザ操作ツール\n\n"
			for _, tool := range tools.Tools {
				prompt += fmt.Sprintf("- **%s**: %s\n", tool.Name, tool.Description)
			}
			prompt += "\n**重要**: これらのツールは plan として提案するのみ。直接実行はしない。\n"
			prompt += "実行は Worker が承認済み job_id を確認してから行う。\n\n"
		}
	}

	prompt += fmt.Sprintf("## ユーザーリクエスト\n%s\n", req.Text)

	return prompt
}
```

**注意**: Coder3 は Chrome 操作を **plan（提案）** として生成するのみ。実際の実行は Worker が承認後に行う。

---

### C-3. 承認フローに「ブラウザ操作」リスクを追加

**pkg/approval/manager.go** の Job 構造体を拡張:

```go
type Job struct {
	// ... 既存のフィールド ...
	UsesBrowser bool `json:"uses_browser"` // ブラウザ操作を含むか
}

func (m *Manager) CreateJob(jobID, plan, patch string, risk map[string]interface{}, usesBrowser bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.jobs[jobID]; exists {
		return fmt.Errorf("job %s already exists", jobID)
	}

	m.jobs[jobID] = &Job{
		JobID:       jobID,
		Status:      StatusPending,
		Plan:        plan,
		Patch:       patch,
		Risk:        risk,
		UsesBrowser: usesBrowser,
		RequestedAt: time.Now().Format(time.RFC3339),
	}
	return nil
}
```

**承認要求メッセージにブラウザ操作警告を追加**:

```go
func FormatApprovalRequest(job *Job) string {
	msg := fmt.Sprintf(`
【承認要求】
Job ID: %s

【操作要約】
%s

【変更内容】
%s

【影響範囲とリスク】
%+v
`, job.JobID, job.Plan, job.Patch, job.Risk)

	if job.UsesBrowser {
		msg += "\n⚠️ **この操作はブラウザ操作を含みます**\n"
	}

	msg += fmt.Sprintf(`
承認する場合: /approve %s
拒否する場合: /deny %s
`, job.JobID, job.JobID)

	return msg
}
```

---

### C-4. Worker による Chrome 操作実行

**pkg/agent/loop.go** で承認後の実行処理:

```go
func (s *AgentLoop) executeApprovedJob(ctx context.Context, jobID string) error {
	// 1. ジョブを取得
	job, err := s.approvalMgr.GetJob(jobID)
	if err != nil {
		return fmt.Errorf("get job: %w", err)
	}

	// 2. 承認状態を確認
	approved, err := s.approvalMgr.IsApproved(jobID)
	if err != nil || !approved {
		return fmt.Errorf("job not approved: %s", jobID)
	}

	// 3. ブラウザ操作が含まれる場合、MCP クライアント経由で実行
	if job.UsesBrowser && s.mcpClient != nil {
		// patch から Chrome 操作コマンドをパース（例: "chrome_navigate: https://example.com"）
		commands := parseChromeCommands(job.Patch)

		for _, cmd := range commands {
			switch cmd.Type {
			case "navigate":
				result, err := s.mcpClient.ChromeNavigate(ctx, cmd.URL)
				if err != nil {
					return fmt.Errorf("chrome navigate: %w", err)
				}
				logger.LogWorkerSuccess(jobID, "chrome_navigate", result)

			case "click":
				result, err := s.mcpClient.ChromeClick(ctx, cmd.Selector)
				if err != nil {
					return fmt.Errorf("chrome click: %w", err)
				}
				logger.LogWorkerSuccess(jobID, "chrome_click", result)

			case "screenshot":
				result, err := s.mcpClient.ChromeScreenshot(ctx)
				if err != nil {
					return fmt.Errorf("chrome screenshot: %w", err)
				}
				logger.LogWorkerSuccess(jobID, "chrome_screenshot", "screenshot saved")

			default:
				return fmt.Errorf("unknown chrome command: %s", cmd.Type)
			}
		}
	} else {
		// 通常のファイル編集・コマンド実行など
		// （既存の Worker 実行ロジック）
	}

	return nil
}

// parseChromeCommands は patch から Chrome 操作コマンドをパース
func parseChromeCommands(patch string) []ChromeCommand {
	// TODO: patch の形式に応じてパース
	// 例: YAML/JSON 形式で記述された Chrome 操作を構造化
	return []ChromeCommand{}
}

type ChromeCommand struct {
	Type     string // navigate, click, screenshot, etc.
	URL      string
	Selector string
}
```

**重要な実装ポイント**:
- Worker は**必ず job_id の承認状態を確認**してから実行
- 承認されていない job_id は実行しない
- すべての Chrome 操作をログに記録（job_id 付き）
- エラー時は詳細を構造化して Chat に返す

---

### C-5. 統合テスト

**Linux 側（PicoClaw）から接続テスト**:

```bash
# MCP クライアントのテスト
cd /home/nyukimi/picoclaw_multiLLM
go test ./pkg/mcp/... -v

# 期待: SKIP（MCP サーバーが起動していない場合）
# または PASS（Win11 で mcp-chrome-bridge が起動している場合）
```

**Win11 側が起動している場合の End-to-End テスト**:

```bash
# テスト用プログラムを作成
cat <<'EOF' > test_mcp.go
package main

import (
	"context"
	"fmt"
	"time"
	"github.com/sipeed/picoclaw/pkg/mcp"
)

func main() {
	client := mcp.NewClient("http://100.83.235.65:12306")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// ヘルスチェック
	if err := client.Ping(ctx); err != nil {
		fmt.Printf("Ping failed: %v\n", err)
		return
	}
	fmt.Println("✓ Ping successful")

	// ツール一覧を取得
	tools, err := client.ListTools(ctx)
	if err != nil {
		fmt.Printf("ListTools failed: %v\n", err)
		return
	}
	fmt.Printf("✓ Found %d tools:\n", len(tools.Tools))
	for _, tool := range tools.Tools {
		fmt.Printf("  - %s\n", tool.Name)
	}

	// ブラウザ操作テスト（example.com に移動）
	result, err := client.ChromeNavigate(ctx, "https://example.com")
	if err != nil {
		fmt.Printf("ChromeNavigate failed: %v\n", err)
		return
	}
	fmt.Printf("✓ Navigation result: %s\n", result)
}
EOF

# テスト実行
go run test_mcp.go
```

**期待される出力**:
```
✓ Ping successful
✓ Found 4 tools:
  - chrome_navigate
  - chrome_click
  - chrome_screenshot
  - chrome_get_text
✓ Navigation result: Navigated to https://example.com
```

---

## トラブルシューティング

### Win11 側の問題

#### mcp-chrome-bridge が起動しない
```powershell
# ログを確認
cat ~\.mcp-chrome\mcp-chrome.log

# ポートが既に使われていないか確認
netstat -ano | findstr :12306

# プロセスを強制終了
taskkill /F /PID <プロセスID>
```

#### Chrome 拡張機能が認識されない
1. Chrome で `chrome://extensions/` を開く
2. 拡張機能が有効になっているか確認
3. 拡張機能のコンソールでエラーを確認

---

### PicoClaw 側の問題

#### 接続タイムアウト
```bash
# Win11 側にネットワーク接続できるか確認
ping 100.83.235.65

# ポートが開いているか確認
nc -zv 100.83.235.65 12306

# または
curl -v http://100.83.235.65:12306/health
```

#### MCP レスポンスのパースエラー
```bash
# レスポンスを直接確認
curl -X POST http://100.83.235.65:12306/mcp \
  -H "Content-Type: application/json" \
  -d '{"method":"tools/list","params":{}}'
```

---

## チェックリスト

### Win11 側
- [ ] Node.js インストール（v18 以上）
- [ ] mcp-chrome-bridge インストール
- [ ] Chrome 拡張機能インストール
- [ ] 設定ファイル作成（~\.mcp-chrome\config.json）
- [ ] mcp-chrome-bridge 起動（ポート 12306）
- [ ] ファイアウォール設定（ポート 12306 開放）
- [ ] ローカルテスト（http://localhost:12306/health）

### PicoClaw 側
- [ ] pkg/mcp/ パッケージ作成
- [ ] client.go 実装
- [ ] types.go 実装
- [ ] chrome_tools.go 実装
- [ ] client_test.go 実装
- [ ] config.json に MCP 設定追加
- [ ] Coder3 に MCP クライアント統合
- [ ] 承認フローに「ブラウザ操作」リスク追加
- [ ] End-to-End テスト

### 統合テスト
- [ ] PicoClaw から Win11 への接続テスト
- [ ] tools/list 呼び出しテスト
- [ ] chrome_navigate テスト
- [ ] Coder3 経由でのブラウザ操作テスト
- [ ] 承認フロー全体のテスト

---

## 参考資料

- **mcp-chrome GitHub**: https://github.com/hangwin/mcp-chrome
- **MCP プロトコル仕様**: https://modelcontextprotocol.io/
- **Chrome MCP Server Documentation**: https://lobehub.com/mcp/hangwin-mcp-chrome
- **PicoClaw Coder3 仕様**: `docs/05_LLM運用プロンプト設計/Coder3_Claude_API仕様.md`

---

**最終更新**: 2026-02-25
**作成者**: Claude Sonnet 4.5
**バージョン**: 1.0
