---
generated_at: 2026-02-28T18:00:00+09:00
run_id: run_20260228_170007
phase: 1
step: "1-6"
profile: picoclaw_multiLLM
artifact: module
module_group_id: mcp
---

# MCP 統合

## 概要

外部 MCP (Model Context Protocol) サーバーへの HTTP 接続を管理し、Chrome DevTools Protocol 経由のブラウザ操作を提供するモジュール。Win11 環境で稼働する mcp-chrome-bridge との通信により、承認フローを経由したブラウザ自動化を実現する。

## 関連ドキュメント

- プロファイル: `codebase-analysis-profile.yaml`
- 外部資料: `docs/codebase-map/refs_mapping.md` (mcp セクション)
- MCP Chrome 統合手順: `docs/06_実装ガイド進行管理/20260225_MCP_Chrome統合手順.md`
- Windows 11 セットアップ: `docs/06_実装ガイド進行管理/win11_manual_setup.md`, `docs/06_実装ガイド進行管理/README_Win11_Setup.md`
- Coder3 承認フロー: `docs/06_実装ガイド進行管理/20260224_Coder3承認フロー実装プラン.md`
- メモリ: `.serena/memories/mcp_chrome_setup_progress.md`

---

## 役割と責務

### 主要な責務

1. **MCP サーバー接続管理**: HTTP API 経由で外部 MCP サーバー（mcp-chrome-bridge）への接続、ヘルスチェック、タイムアウト・リトライ処理。
2. **Chrome 操作ラッパー**: ブラウザ操作（navigate, click, screenshot, get_text）を Go の型安全な API として提供。
3. **ツール列挙**: MCP サーバーが提供するツール一覧の取得（tools/list）と、汎用的なツール呼び出し（tools/call）。
4. **承認フロー統合**: すべての Chrome 操作は `job_id` と承認フローを経由して実行される前提で設計（※現在は Agent Loop で初期化のみ、実行ロジックは未統合）。

### 対外インターフェース（公開 API）

**pkg/mcp パッケージの公開型・関数**:

- **`type Client`**: MCP クライアントの構造体（HTTP クライアントと base URL を保持）
- **`func NewClient(baseURL string) *Client`**: クライアントの生成（タイムアウト 30 秒）
- **`func (c *Client) Ping(ctx context.Context) error`**: ヘルスチェック（GET /health）
- **`func (c *Client) ListTools(ctx context.Context) (*ToolListResponse, error)`**: ツール一覧の取得（POST /mcp, method: "tools/list"）
- **`func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolCallResponse, error)`**: 汎用ツール呼び出し（POST /mcp, method: "tools/call"）

**Chrome 操作専用ラッパー**（chrome_tools.go）:

- **`func (c *Client) ChromeNavigate(ctx context.Context, url string) (string, error)`**: 指定 URL に移動
- **`func (c *Client) ChromeClick(ctx context.Context, selector string) (string, error)`**: 指定セレクタの要素をクリック
- **`func (c *Client) ChromeScreenshot(ctx context.Context) (string, error)`**: ページのスクリーンショットを Base64 で取得
- **`func (c *Client) ChromeGetText(ctx context.Context, selector string) (string, error)`**: 指定セレクタの要素のテキストを取得

**型定義**（types.go）:

- **`MCPRequest`**: MCP サーバーへのリクエスト（Method, Params）
- **`MCPResponse`**: MCP サーバーからのレスポンス（Result, Error）
- **`Tool`**: ツール定義（Name, Description, InputSchema）
- **`ToolListResponse`**: tools/list のレスポンス（Tools []Tool）
- **`ToolCallResponse`**: tools/call のレスポンス（Content []map[string]interface{}）

### 内部構造（非公開）

- **`call(ctx, req MCPRequest) (*MCPResponse, error)`**: HTTP POST /mcp へのリクエスト送信とレスポンスパース（内部メソッド）
- Chrome 操作関数は `CallTool` を呼び出すラッパーとして実装（ツール名と引数をマッピング）
- エラーハンドリング: HTTP エラー、MCP エラー（MCPError）、レスポンスパースエラーを区別して返す

---

## 依存関係

### 外部依存（このモジュールが依存する他モジュール）

**標準ライブラリのみ**:
- `context`: タイムアウト・キャンセル管理
- `net/http`: HTTP クライアント
- `encoding/json`: JSON シリアライズ・デシリアライズ
- `time`: タイムアウト設定（デフォルト 30 秒）

**PicoClaw パッケージへの依存なし**:
- このモジュールは純粋な MCP クライアントとして、他の PicoClaw モジュールに依存しない
- ロガーや設定も直接参照せず、Agent Loop 側で初期化時に設定を読み取る

### 被依存（このモジュールに依存する他モジュール）

**現在の統合状況**:
- **`pkg/agent`** (loop.go): MCP クライアントを初期化し、`mcpClient` フィールドに保持
  - 初期化条件: `cfg.MCP.Chrome.Enabled == true`
  - 使用状況: **現在は初期化のみで、実際の呼び出しロジックは未実装**

**将来の統合計画**（※推測、実装仕様未記載）:
- **`pkg/providers/anthropic.go`** (Coder3): Chrome 操作を含む plan/patch の生成時に `uses_browser: true` フラグを設定（未実装）
- **`pkg/agent/loop.go`** (Worker 実行): 承認済み job_id の patch から Chrome 操作コマンドをパースし、`mcpClient` 経由で実行（未実装）

**設定での統合**:
- **`pkg/config/config.go`**: `MCPConfig` と `MCPChromeConfig` 構造体を定義
  - `MCP.Chrome.Enabled`: MCP 機能の有効化
  - `MCP.Chrome.BaseURL`: mcp-chrome-bridge の URL（デフォルト: `http://100.83.235.65:12306`）
  - `MCP.Chrome.TimeoutSec`: タイムアウト（秒）

**承認フローでの統合**:
- **`pkg/approval/manager.go`**: `Job.UsesBrowser` フィールドでブラウザ操作を追跡
- **`pkg/approval/message.go`**: 承認要求メッセージに "⚠️ **この操作はブラウザ操作を含みます**" 警告を追加

---

## 構造マップ

### ファイル構成

```
pkg/mcp/
├── types.go           # MCP プロトコルの型定義（Request/Response/Tool/Error）
├── client.go          # MCP クライアント本体（HTTP 通信、tools/list, tools/call, Ping）
├── chrome_tools.go    # Chrome 操作ラッパー（Navigate, Click, Screenshot, GetText）
└── client_test.go     # ユニットテスト（Ping, ListTools）
```

### MCP サーバー接続フロー

```
┌─────────────────────────────────────────────────────────────────┐
│ PicoClaw (Linux)                                                │
│   pkg/agent/loop.go                                             │
│     └─ mcpClient = mcp.NewClient(cfg.MCP.Chrome.BaseURL)       │
│         ↓                                                       │
│   pkg/mcp/client.go                                             │
│     └─ Client.Ping(ctx) → GET http://100.83.235.65:12306/health│
│     └─ Client.ListTools(ctx) → POST /mcp {"method":"tools/list"}│
│     └─ Client.CallTool(ctx, "chrome_navigate", {url: "..."})   │
│         ↓                                                       │
└─────────────────────────────────────────────────────────────────┘
         │ HTTP (JSON-RPC 風プロトコル)
         ↓
┌─────────────────────────────────────────────────────────────────┐
│ Win11 (100.83.235.65:12306)                                     │
│   mcp-chrome-bridge                                             │
│     └─ POST /mcp → Native Messaging → Chrome 拡張機能          │
│         ↓                                                       │
│   Chrome 拡張機能 (MCP Chrome)                                  │
│     └─ Chrome DevTools Protocol (CDP) 経由でブラウザ操作       │
└─────────────────────────────────────────────────────────────────┘
```

### Tool 定義とルーティング

**MCP プロトコルのツール呼び出しパターン**:

1. **ツール一覧取得**:
   ```json
   Request: {"method": "tools/list", "params": {}}
   Response: {
     "result": {
       "tools": [
         {"name": "chrome_navigate", "description": "Navigate to URL"},
         {"name": "chrome_click", "description": "Click element"},
         {"name": "chrome_screenshot", "description": "Take screenshot"},
         {"name": "chrome_get_text", "description": "Get text from element"}
       ]
     }
   }
   ```

2. **ツール実行**:
   ```json
   Request: {
     "method": "tools/call",
     "params": {
       "name": "chrome_navigate",
       "arguments": {"url": "https://example.com"}
     }
   }
   Response: {
     "result": {
       "content": [{"text": "Navigated to https://example.com"}]
     }
   }
   ```

**Go API のラッパー設計**:

- `CallTool(ctx, name, args)` → 汎用的なツール呼び出し
- `ChromeNavigate(ctx, url)` → `CallTool(ctx, "chrome_navigate", {"url": url})` のラッパー
- `ChromeClick(ctx, selector)` → `CallTool(ctx, "chrome_click", {"selector": selector})` のラッパー
- レスポンスパース: `Content[0]["text"]` または `Content[0]["data"]` から結果を抽出

### 承認フローとの統合（計画）

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. Coder3 (Claude API) が plan/patch を生成                    │
│    └─ plan: "Google で検索して結果を取得"                       │
│    └─ patch: "chrome_navigate: https://google.com?q=..."       │
│    └─ uses_browser: true                                        │
│         ↓                                                       │
│ 2. Chat が承認要求を送信（job_id 付き）                        │
│    └─ approval.CreateJob(jobID, plan, patch, risk, usesBrowser)│
│    └─ "⚠️ **この操作はブラウザ操作を含みます**"                │
│         ↓                                                       │
│ 3. 人間が承認（/approve <job_id>）                            │
│    └─ approval.Approve(jobID, approver)                         │
│         ↓                                                       │
│ 4. Worker が承認済みジョブを実行                               │
│    └─ job, _ := approvalMgr.GetJob(jobID)                      │
│    └─ if job.UsesBrowser && mcpClient != nil {                 │
│          parseChromeCommands(job.Patch) → mcpClient 呼び出し   │
│       }                                                         │
└─────────────────────────────────────────────────────────────────┘
```

**※注意**: 上記フローの Step 4 は未実装。現在は MCP クライアントの初期化のみ完了。

---

## 落とし穴・注意点

### 設計上の制約

1. **外部サーバー依存**: Win11 環境の mcp-chrome-bridge が稼働していなければ使用不可（エラー時は Ping でスキップ）。
2. **タイムアウト固定**（※Phase 2 で確認: L23-24）: HTTP クライアントのタイムアウトは 30 秒でハードコード（`Timeout: 30 * time.Second`）、config.TimeoutSec は参照されていない。
3. **レスポンスパースの脆弱性**（※Phase 2 で確認: chrome_tools.go L17-25, L39-47, L58-67, L79-87）: `Content[0]["text"]` や `Content[0]["data"]` の存在を前提としており、フォーマットが異なると panic のリスク（現在は型アサーション失敗時に `fmt.Errorf("invalid response format")` で返す）。
   - ※Phase 2 で追加: 各 Chrome 操作関数（Navigate, Click, GetText, Screenshot）は同一のパターンでエラーハンドリング
   - ※Phase 2 で追加: `len(resp.Content) == 0` で空レスポンスを検出（L17-19 等）
4. **リトライなし**: HTTP リクエスト失敗時に自動リトライは実装されていない（Agent Loop 側で実装が必要）。

### 既知の問題・リスク

1. **未統合**: Agent Loop で `mcpClient` を初期化するが、実際にブラウザ操作を実行するロジックは未実装。
   - Coder3 が `uses_browser: true` を設定するロジックなし
   - Worker が patch から Chrome コマンドをパースする `parseChromeCommands` 関数なし
   - 実行結果のログ記録なし
   - ※Phase 2 で確認: client.go と chrome_tools.go は完全に実装されており、Phase 5-B（クライアント実装）は完了
   - ※Phase 2 で確認: Phase 5-C（Agent Loop 統合）は未着手

2. **承認フローの未検証**: `Job.UsesBrowser` フラグは実装済みだが、実際にブラウザ操作が承認フローを通過するエンドツーエンドテストが存在しない。

3. **Auto-Approve の対象外**: 統合手順書では「Auto-Approve は Chrome 操作を対象外とする」とあるが、`pkg/approval` には Auto-Approve のロジックが見当たらない（※推測: 未実装または別モジュール）。

4. **Win11 環境の可用性**: mcp-chrome-bridge が停止すると MCP 機能全体が使用不可になる。フォールバック機能なし。

5. **セキュリティ**: Chrome 操作は破壊的操作と同等のリスクがあるが、承認フローの実装が不完全なため、実運用前に慎重な検証が必須。

### 変更時の注意事項

1. **タイムアウトの設定反映**: `config.MCP.Chrome.TimeoutSec` を `Client` の HTTP タイムアウトに反映させる場合、`NewClient` のシグネチャ変更が必要（現在は引数で受け取っていない）。

2. **ツール名の変更**: mcp-chrome-bridge 側のツール名（`chrome_navigate` 等）が変更された場合、chrome_tools.go の各関数も同期して修正が必要。

3. **MCP プロトコル仕様の追従**: MCP プロトコルが更新された場合、`types.go` の型定義と `client.go` のパースロジックを見直す必要がある。

4. **エラーハンドリング強化**: 現在は単純な `fmt.Errorf` でエラーを返しているが、将来的にはリトライ可能エラー（ネットワーク一時障害）と致命的エラー（認証失敗）を区別する設計が望ましい。

5. **ログ統合**: 現在は `pkg/logger` を使用していないため、MCP 接続エラーやツール実行結果が構造化ログに記録されない。Agent Loop 側でラップしてログを記録する必要がある。

6. **テストの実行条件**: `client_test.go` のテストは Win11 環境が稼働していない場合 Skip される。CI/CD パイプラインでは環境変数 `MCP_TEST_URL` を設定するか、モックサーバーを用意する必要がある。

---

## Phase 2 検証結果

### 検証日時
2026-02-28

### 検証内容
- `pkg/mcp/client.go` (138 行) との突合せ完了
- `pkg/mcp/chrome_tools.go` (90 行) との突合せ完了

### 発見された差異・追加事項
1. **タイムアウトのハードコード確認**（※Phase 2 で追加）:
   - L23-24 で `Timeout: 30 * time.Second` を確認
   - config.TimeoutSec は参照されていない（設定変更が効かない制約）

2. **レスポンスパースのエラーハンドリング詳細を追記**（※Phase 2 で追加）:
   - Chrome 操作関数（Navigate, Click, GetText, Screenshot）はすべて同一パターンでエラーハンドリング
   - `len(resp.Content) == 0` で空レスポンスを検出
   - 型アサーション失敗時は `fmt.Errorf("invalid response format")` を返す

3. **Phase 5-B 完了を確認**（※Phase 2 で追加）:
   - client.go と chrome_tools.go は完全に実装されている
   - Phase 5-C（Agent Loop 統合）は未着手であることを確認

### 構造マップの正確性検証
- ✅ ファイル構成: 正確（types.go, client.go, chrome_tools.go を確認）
- ✅ MCP サーバー接続フロー: 正確（HTTP POST /mcp, GET /health を確認）
- ✅ Tool 定義とルーティング: 正確（tools/list, tools/call を確認）

### 落とし穴の網羅性検証
- ✅ タイムアウト固定: 実コードで確認、L23-24 でハードコード
- ✅ レスポンスパースの脆弱性: 実コードで確認、エラーハンドリングの詳細を追記
- ✅ 未統合: 実コードで確認、Phase 5-B 完了・Phase 5-C 未着手を追記

### 設計書との乖離
- なし（実装仕様 `docs/06_実装ガイド進行管理/20260225_MCP_Chrome統合手順.md` と整合）

---

**最終更新**: 2026-02-28 (Phase 2 検証完了)
**作成者**: Claude Sonnet 4.5
**ステータス**: Phase 5-B 完了（クライアント実装済み）、Phase 5-C 未着手（Agent Loop 統合未完了）
