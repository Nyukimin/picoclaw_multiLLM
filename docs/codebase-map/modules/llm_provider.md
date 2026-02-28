---
generated_at: 2026-02-28T17:15:00+09:00
run_id: run_20260228_170007
phase: 1
step: "1-3"
profile: picoclaw_multiLLM
artifact: module
module_group_id: llm_provider
---

# LLM プロバイダー

## 概要

複数の LLM プロバイダー（Ollama, Claude API, OpenAI, DeepSeek 等）への統一インターフェースを提供し、認証、リトライ、タイムアウト、ストリーム処理、ヘルスチェックを管理するモジュール。PicoClaw の役割分離（Chat/Worker/Coder）を支える基盤レイヤー。

## 関連ドキュメント

- プロファイル: `/home/nyukimi/picoclaw_multiLLM/codebase-analysis-profile.yaml`
- 外部資料: `docs/codebase-map/refs_mapping.md` (llm_provider セクション)
  - `docs/05_LLM運用プロンプト設計/LLM_Ollama常駐管理.md` (Ollama 常駐化・MaxContext 制約)
  - `docs/05_LLM運用プロンプト設計/Coder3_Claude_API仕様.md` (Claude API 統合・承認フロー)
  - `docs/05_LLM運用プロンプト設計/LLM_deepseek運用仕様.md` (DeepSeek キャッシュ設計)
  - `docs/05_LLM運用プロンプト設計/LLM_Worker_Spec_v1_0.md` (Worker 仕様)
  - `docs/05_LLM運用プロンプト設計/CHAT_PERSONA設計.md` (Chat ペルソナ設計)

---

## 役割と責務

### 主要な責務

1. **LLM 統一インターフェース**: `LLMProvider` インターフェースによる複数プロバイダーの抽象化
2. **プロバイダー選択・初期化**: 設定（`config.Config`）とモデル名から適切なプロバイダーを自動選択
3. **認証管理**: API キー、OAuth トークン、CLI ベース認証（codex-cli, claude-cli）の統合
4. **リトライ・タイムアウト**: HTTP リクエストのタイムアウト（120秒）、タイムアウトエラー検出、画像ペイロード監査
5. **ストリーム処理**: Codex（OpenAI Responses API）のストリーム受信対応
6. **ヘルスチェック**: Ollama の生存確認、モデルロード状態監視、MaxContext 制約チェック（pkg/health 連携）

### 対外インターフェース（公開 API）

#### 統一インターフェース（`types.go`）

- **`LLMProvider` インターフェース**:
  - `Chat(ctx, messages, tools, model, options) -> (*LLMResponse, error)`: LLM 呼び出しの共通 API
  - `GetDefaultModel() -> string`: デフォルトモデル名の取得
- **共通型定義**:
  - `Message`: role, content, media, tool_calls, tool_call_id を保持
  - `LLMResponse`: content, tool_calls, finish_reason, usage を保持
  - `ToolCall` / `ToolDefinition`: Function Calling 対応

#### プロバイダー生成（`http_provider.go`）

- **`CreateProvider(cfg *config.Config) -> (LLMProvider, error)`**: 設定とモデル名から適切なプロバイダーを自動選択・生成
  - 明示プロバイダー指定（`cfg.Agents.Defaults.Provider`）を優先
  - モデル名から推論（`moonshot/kimi-k2.5`, `claude-*`, `gpt-*` など）
  - フォールバック: OpenRouter をデフォルト使用
  - ※Phase 2 で確認: L521-749 で実装、3 段階フォールバック（明示指定→モデル名推論→OpenRouter）

#### 個別プロバイダー

- **`NewHTTPProvider(apiKey, apiBase, proxy)`**: OpenAI 互換 API 向け汎用プロバイダー（Ollama, Groq, Moonshot, DeepSeek, OpenRouter など）
- **`NewClaudeProvider(token)` / `NewClaudeProviderWithTokenSource(token, tokenSource)`**: Anthropic Claude API 専用（Coder3）
- **`NewCodexProvider(token, accountID)` / `NewCodexProviderWithTokenSource(...)`**: OpenAI Codex (ChatGPT backend) 専用
- **`NewClaudeCliProvider(workspace)`**: claude CLI サブプロセス実行プロバイダー
- **`NewCodexCliProvider(workspace)`**: codex CLI サブプロセス実行プロバイダー
- **`NewGitHubCopilotProvider(uri, connectMode, model)`**: GitHub Copilot SDK 統合

### 内部構造（非公開）

#### 認証トークン管理

- `createClaudeTokenSource()`: `pkg/auth.GetCredential("anthropic")` から OAuth トークン取得
- `createCodexTokenSource()`: `pkg/auth.GetCredential("openai")` から OAuth トークン取得、リフレッシュ処理内蔵
- `CreateCodexCliTokenSource()`: codex CLI のクレデンシャル取得（`codex_cli_credentials.go`）

#### HTTP リクエスト構築・パース

- `buildHTTPMessagesWithAudit(messages)`: OpenAI 形式メッセージ構築、画像ペイロード監査
- `toImageURLPayload(media)`: ローカルファイルを Base64 データ URI 化、サイズ制限（5MB）適用
- `buildClaudeParams(messages, tools, model, options)`: Claude API 形式パラメータ構築（system, multimodal 対応）
- `buildCodexParams(messages, tools, model, options)`: Codex Responses API 形式パラメータ構築

#### エラーハンドリング・監視

- `isTimeoutError(err)`: タイムアウトエラー判定（`context.DeadlineExceeded`, `net.Error.Timeout()`）
- `enrichAuditAfterTimeout(entries)`: タイムアウト後の画像ファイル状態再確認（削除検出用）
- ログ出力（`pkg/logger`）:
  - `provider.http`: リクエスト送信前、応答受信後、エラー時の詳細ログ
  - `provider.codex`: Codex API エラー詳細（status_code, api_type, hint）

#### Ollama 固有処理（`http_provider.go`）

- `isOllamaEndpoint(apiBase)`: `:11434` 検出で Ollama 判定（L358-361, `localhost:11434`, `127.0.0.1:11434` も検出）
- Ollama 向けオプション自動設定（L103-108）:
  - `keep_alive: -1` で Chat/Worker モデルを永続化
  - `num_ctx: 8192` で MaxContext 制約を適用（131072 等の大きすぎる context によるクラッシュ防止）
  - ※Phase 2 で確認: `options` として `num_ctx: 8192` を明示的に設定

---

## 依存関係

### 外部依存（このモジュールが依存する他モジュール）

| 依存先 | 用途 | 重要度 |
|--------|------|--------|
| `pkg/config` | プロバイダー設定（API キー、API Base、デフォルトモデル）取得 | 必須 |
| `pkg/auth` | OAuth トークン取得・リフレッシュ（`GetCredential`, `SetCredential`, `RefreshAccessToken`） | OAuth 使用時必須 |
| `pkg/logger` | 構造化ログ出力（`InfoCF`, `WarnCF`, `ErrorCF`） | 必須（観察性） |
| `github.com/anthropics/anthropic-sdk-go` | Claude API SDK | Claude 使用時必須 |
| `github.com/openai/openai-go/v3` | OpenAI SDK (Codex Responses API) | Codex 使用時必須 |
| `github.com/github/copilot-sdk/go` | GitHub Copilot SDK | Copilot 使用時必須 |

### 被依存（このモジュールに依存する他モジュール）

| 依存元 | 用途 | 結合度 |
|--------|------|--------|
| `pkg/agent/loop.go` | `CreateProvider` でプロバイダーを生成し、LLM 呼び出し | 強 |
| `pkg/health/checks.go` | Ollama ヘルスチェック（`OllamaCheck`, `OllamaModelsCheck`）※直接依存ではなく HTTP 経由 | 弱 |

---

## 構造マップ

### ファイル構成

```
pkg/providers/
├── types.go                          # 共通インターフェース・型定義
├── http_provider.go                  # OpenAI 互換 API 向け汎用プロバイダー + CreateProvider
├── claude_provider.go                # Claude API 専用プロバイダー
├── codex_provider.go                 # OpenAI Codex (ChatGPT backend) 専用
├── claude_cli_provider.go            # claude CLI サブプロセス実行
├── codex_cli_provider.go             # codex CLI サブプロセス実行
├── codex_cli_credentials.go          # codex CLI クレデンシャル取得
├── github_copilot_provider.go        # GitHub Copilot SDK 統合
├── tool_call_extract.go              # Claude CLI 出力からの tool_calls 抽出
└── *_test.go                         # ユニットテスト群
```

### プロバイダー別実装

| プロバイダー | 実装クラス | 対応モデル例 | 認証方式 | 備考 |
|-------------|-----------|-------------|---------|------|
| **Ollama** | `HTTPProvider` | `chat-v1`, `worker-v1`, `qwen2.5:14b` | なし | ローカル推論、MaxContext 制約、keep_alive 永続化 |
| **Claude API** | `ClaudeProvider` | `claude-sonnet-4-5-20250929` | API キーまたは OAuth | Coder3 専用、multimodal 対応 |
| **OpenAI** | `HTTPProvider` | `gpt-4`, `gpt-5.2` | API キーまたは OAuth | OpenAI 互換 API として統一処理 |
| **Codex (ChatGPT backend)** | `CodexProvider` | `gpt-5.2` | OAuth + Account ID | ストリーム処理、account_id ヘッダ必須 |
| **DeepSeek** | `HTTPProvider` | `deepseek-chat`, `deepseek-reasoner` | API キー | KV キャッシュ設計（prefix 固定化） |
| **Groq** | `HTTPProvider` | `groq/*` | API キー | OpenAI 互換 API |
| **Moonshot (Kimi)** | `HTTPProvider` | `moonshot/kimi-k2.5` | API キー | Temperature=1 固定 |
| **OpenRouter** | `HTTPProvider` | `openrouter/*`, `anthropic/*`, `meta-llama/*` | API キー | フォールバック先 |
| **claude CLI** | `ClaudeCliProvider` | `claude-code` | CLI 経由 | サブプロセス実行、`--no-chrome`, `--dangerously-skip-permissions` |
| **codex CLI** | `CodexCliProvider` | `gpt-5.2` | CLI 経由 | サブプロセス実行 |
| **GitHub Copilot** | `GitHubCopilotProvider` | `gpt-4.1` | SDK 経由 | gRPC または stdio 接続 |

### 共通インターフェース（`LLMProvider`）

- **設計方針**: すべてのプロバイダーが `Chat(...)` と `GetDefaultModel()` を実装
- **ストリーム対応**: `CodexProvider` のみストリーム受信実装（Responses API）
- **Tool Calling 対応**: `HTTPProvider`, `ClaudeProvider`, `CodexProvider` が `tools` パラメータを受け入れ、`tool_calls` を返却
- **メディア対応**: `HTTPProvider` と `ClaudeProvider` が multimodal (画像) をサポート（Base64 データ URI 化）

### ヘルスチェック機構

※ `pkg/health/checks.go` に実装、providers モジュールは HTTP エンドポイント提供のみ

- **`OllamaCheck(baseURL, timeout)`**: Ollama API 生存確認（GET `/api/tags`）
- **`OllamaModelsCheck(baseURL, timeout, required[]ModelRequirement)`**:
  - `/api/ps` でロード済みモデル一覧を取得
  - `ModelRequirement` で MinContext/MaxContext 制約をチェック（例: MaxContext=8192 で 131072 は NG）
  - 常駐化モデル（chat-v1, worker-v1）の存在確認

---

## 落とし穴・注意点

### 設計上の制約

1. **プロバイダー選択ロジックの複雑性**:
   - `CreateProvider` は明示指定→モデル名推論→フォールバックの3段階で分岐
   - モデル名プレフィックス（`moonshot/`, `groq/`, `ollama/`）の strip 処理が散在
   - ※推測: 設定ミスや新プロバイダー追加時にフォールバック先（OpenRouter）へ流れるリスクあり

2. **Ollama の MaxContext 制約**:
   - Ollama は num_ctx を指定しないと大きすぎる context window（131072 等）でロードしようとし、マルチモーダル処理でクラッシュ・タイムアウトの原因となる
   - `http_provider.go` L103-107 で `num_ctx: 8192` を強制設定
   - `pkg/health/checks.go` の `OllamaModelsCheck` で MaxContext 制約違反を検出
   - **※重要**: モデルロード時に num_ctx を指定しても、既にロード済みの場合は効かない（warmup 時に設定が必要）

3. **keep_alive による常駐化**:
   - `keep_alive: -1` で Chat/Worker モデルを永続化（リクエスト毎のロード待ち時間削減）
   - Windows 環境で Ollama サービスが再起動すると常駐状態がリセットされる（warmup スクリプト再実行が必要）

4. **認証方式の多様性**:
   - API キー（環境変数 / 設定ファイル）
   - OAuth トークン（`pkg/auth` 経由、リフレッシュ処理あり）
   - CLI ベース認証（claude CLI, codex CLI）
   - ※推測: 認証失敗時のエラーメッセージが統一されていない（プロバイダー毎に異なる）

### 既知の問題・リスク

1. **タイムアウトとログの非対称性** (L149-176, `http_provider.go`):
   - HTTP リクエストが 120 秒でタイムアウトする（L54, `Timeout: 120 * time.Second`）が、Ollama 側は処理を継続している可能性あり
   - タイムアウト時に画像ファイルが削除されていないかを `enrichAuditAfterTimeout` (L387-408) で再確認
   - ログに `note: "Ollama may have responded but PicoClaw timed out before reading"` を記録（L167）
   - `isTimeoutError` (L411-423) で `context.DeadlineExceeded`, `net.Error.Timeout()`, エラー文字列の 3 通りを検出
   - **※Phase 2 で確認**: タイムアウトエラー判定の実装は堅牢（3 段階チェック）

2. **Codex の account_id 要件** (L73-79, `codex_provider.go`):
   - Codex backend は account_id ヘッダがないと 400 エラーを返す
   - OAuth トークンリフレッシュ時に account_id が空になるリスクあり（L356-358 で補完処理）
   - ログで `hint: "verify account id header and model compatibility for codex backend"` を出力

3. **モデル名のフォールバック** (L143-183, `codex_provider.go`):
   - Codex backend は非 OpenAI モデル（`claude-*`, `glm-*` など）をサポートしない
   - `resolveCodexModel` で不適合モデルを `gpt-5.2` にフォールバック
   - ログで `reason` を記録するが、ユーザー通知はない（Chat レイヤーで判断）

4. **画像ペイロードのサイズ制限**:
   - Base64 エンコード後のデータ URI が 5MB を超えると黙って除外（`toImageURLPayload` L334-336, `maxInlineImageBytes = 5 * 1024 * 1024` を L50 で定義）
   - 除外された画像は `imagePayloadAudit` で `drop_reason: "file_too_large"` を記録（L335）
   - ※Phase 2 で確認: サイズ制限は定数で明示されており、仕様通り実装されている
   - ※リスク: ユーザーがマルチモーダル処理を期待しているのに画像が送信されず、結果が不正確になる可能性

5. **Claude CLI の tool_calls 抽出**:
   - Claude CLI は JSON 出力に tool_calls を直接含めない
   - `tool_call_extract.go` でテキストから JSON ブロックを検出・パース
   - ※推測: 複雑な tool_calls や不正な JSON 形式でパース失敗のリスクあり

### 変更時の注意事項

1. **新プロバイダー追加時**:
   - `CreateProvider` の switch 文と fallback ロジックの両方に追加が必要
   - `LLMProvider` インターフェースを実装
   - モデル名プレフィックスの strip 処理を確認（L78-84, `http_provider.go`）
   - デフォルトモデルを `GetDefaultModel()` で定義

2. **認証方式変更時**:
   - `pkg/auth` の変更が providers モジュールに波及する可能性
   - OAuth トークンリフレッシュの失敗ハンドリングを確認（`createCodexTokenSource` L350-363）

3. **Ollama 運用変更時**:
   - MaxContext 制約を変更する場合、`http_provider.go` L106 と `pkg/health/checks.go` の両方を更新
   - warmup スクリプトで num_ctx を指定（再起動後の常駐化）

4. **ログ出力の変更**:
   - `pkg/logger` のインターフェース変更時、providers モジュール全体の InfoCF/WarnCF/ErrorCF 呼び出しを確認
   - 画像ペイロード監査ログ（`imageAuditLogEntries`）は運用監視で重要（削除しない）

5. **ストリーム処理の拡張**:
   - 現在 `CodexProvider` のみストリーム対応
   - 他プロバイダーでストリーム実装時、`LLMProvider` インターフェースの拡張が必要（互換性リスク）

---

## 既知の調査記録との整合性

- `docs/06_実装ガイド進行管理/20260224_Coder3統合仕様反映.md`: Coder3（Claude API）統合の実装ガイドと整合
- `docs/06_実装ガイド進行管理/20260224_ヘルスチェック強化とテスト追加.md`: Ollama ヘルスチェック強化（MaxContext 制約）と整合
- `docs/05_LLM運用プロンプト設計/LLM_Ollama常駐管理.md`: keep_alive 永続化、MaxContext 制約の運用仕様と整合

---

## Phase 2 検証結果

### 検証日時
2026-02-28

### 検証内容
- `pkg/providers/http_provider.go` (749 行) との突合せ完了
- `pkg/providers/types.go` (59 行) との突合せ完了

### 発見された差異・追加事項
1. **タイムアウト実装の詳細を追記**（※Phase 2 で追加）:
   - `isTimeoutError` は 3 段階チェック（`context.DeadlineExceeded`, `net.Error.Timeout()`, エラー文字列）を実装
   - HTTP クライアントのタイムアウトは 120 秒（L54）

2. **Ollama 判定ロジックの詳細を追記**（※Phase 2 で追加）:
   - `isOllamaEndpoint` は `:11434` だけでなく `localhost:11434`, `127.0.0.1:11434` も検出（L358-361）

3. **画像ペイロードサイズ制限の定数を追記**（※Phase 2 で追加）:
   - `maxInlineImageBytes = 5 * 1024 * 1024` を L50 で定義

4. **CreateProvider の実装範囲を追記**（※Phase 2 で追加）:
   - L521-749 で実装、3 段階フォールバック（明示指定→モデル名推論→OpenRouter）

### 構造マップの正確性検証
- ✅ ファイル構成: 正確（types.go, http_provider.go 等を確認）
- ✅ プロバイダー別実装: 正確（HTTPProvider, ClaudeProvider 等を確認）
- ✅ 共通インターフェース: 正確（LLMProvider インターフェースを確認）

### 落とし穴の網羅性検証
- ✅ タイムアウトとログの非対称性: 実コードで確認、詳細を追記
- ✅ 画像ペイロードのサイズ制限: 実コードで確認、定数を追記
- ✅ Ollama の MaxContext 制約: 実コードで確認、options 設定を追記

### 設計書との乖離
- なし（実装仕様 `docs/01_正本仕様/実装仕様.md` と整合）

---

**最終更新**: 2026-02-28 (Phase 2 検証完了)
**解析担当**: Claude Sonnet 4.5 (codebase-analysis)
