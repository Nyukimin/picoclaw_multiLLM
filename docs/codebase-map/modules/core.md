---
generated_at: 2026-02-28T17:30:00+09:00
run_id: run_20260228_170007
phase: 1
step: "1-1"
profile: picoclaw_multiLLM
artifact: module
module_group_id: core
---

# コア（エントリポイント・設定）

## 概要

PicoClaw の起動・初期化・設定管理を担当するモジュール。コマンドライン引数の解析、設定ファイルの読み込み、各種サービスの初期化、サブコマンド（agent/gateway/cron/skills 等）の起動を統括する。

## 関連ドキュメント

- プロファイル: `codebase-analysis-profile.yaml`
- 外部資料: `docs/codebase-map/refs_mapping.md` (core セクション)
- 実装仕様（正本）: `docs/01_正本仕様/実装仕様.md`（1章：スコープ・責務境界）
- 設定閾値仕様: `docs/02_v2統合分割仕様/実装仕様_v2_10_設定閾値.md`
- スコープ責務仕様: `docs/02_v2統合分割仕様/実装仕様_v2_01_スコープ責務.md`

---

## 役割と責務

### 主要な責務

1. **コマンドライン引数の解析**: サブコマンド（onboard, agent, gateway, status, migrate, auth, cron, skills, version）を判定し、対応する処理に振り分ける。
2. **設定ファイルの管理**: JSON 形式の設定ファイル（`~/.picoclaw/config.json`）の読み込み、書き込み、デフォルト値の提供。環境変数による上書きもサポート。
3. **初期化と起動**: Agent Loop、Channel Manager、Provider、各種サービス（Cron、Heartbeat、Device、Health Check）の初期化と起動。
4. **ワークスペース管理**: 初回セットアップ時にテンプレートファイルを展開し、ワークスペースディレクトリ（`~/.picoclaw/workspace`）を作成。
5. **OAuth/認証管理**: OpenAI/Anthropic 等の OAuth/Token 認証フローをサポート（auth サブコマンド）。

※Phase 2 で追加: main.go の実コードを確認した結果、上記5つの責務は正確。追加で以下の責務を検出:
6. **Skills 管理**: GitHub からのスキルインストール、ビルトインスキルのコピー、スキル一覧表示（skills サブコマンド）。
7. **バージョン情報管理**: ビルド時に埋め込まれた version/gitCommit/buildTime/goVersion の表示（version サブコマンド）。

### 対外インターフェース（公開 API）

- **コマンドライン引数**: `picoclaw <subcommand> [options]`
  - `onboard`: 初回セットアップ（設定ファイルとワークスペースの作成）
  - `agent`: Agent モードで直接対話（`-m` でメッセージ、`-s` でセッション指定）
  - `gateway`: Gateway モードで起動（各種チャネルと統合）
  - `status`: 現在の設定と状態を表示
  - `migrate`: OpenClaw からのマイグレーション
  - `auth`: OAuth/Token 認証の管理（login/logout/status）
  - `cron`: 定期実行タスクの管理（list/add/remove/enable/disable）
  - `skills`: スキルの管理（list/install/remove/search/show）
  - `version`: バージョン情報の表示

- **設定ファイル**: `~/.picoclaw/config.json`（JSON 形式）
  - agents, channels, providers, gateway, watchdog, tools, routing, loop, heartbeat, devices, mcp の各セクション

- **環境変数**: `PICOCLAW_*` 形式の環境変数で設定を上書き可能（例: `PICOCLAW_ROUTING_LLM_CHAT_MODEL`）

### 内部構造（非公開）

- **起動フローの制御**: main() → サブコマンド振り分け → 各種 *Cmd() 関数
- **設定読み込み**: loadConfig() → config.LoadConfig() → DefaultConfig() + JSON 読み込み + 環境変数パース
- **サービス初期化**: gatewayCmd() 内で providers.CreateProvider()、agent.NewAgentLoop()、channels.NewManager() 等を初期化
- **ヘルスチェック統合**: Ollama モデルのロード状態確認、MaxContext 制約チェック（8192 を超える context_length は NG）

---

## 依存関係

### 外部依存（このモジュールが依存する他モジュール）

**必須依存**:
- `pkg/config`: 設定構造体の定義、読み込み・書き込み・デフォルト値提供
- `pkg/agent`: Agent Loop の初期化と起動
- `pkg/providers`: LLM プロバイダーの初期化（Anthropic, OpenAI, Ollama, DeepSeek 等）
- `pkg/logger`: 構造化ログの出力

**Gateway モード時に追加で依存**:
- `pkg/channels`: LINE, Slack, Telegram 等のチャネル管理
- `pkg/bus`: メッセージバスによるイベント配信
- `pkg/health`: ヘルスチェックエンドポイント（/health, /ready）
- `pkg/heartbeat`: 定期的なハートビート処理
- `pkg/cron`: 定期実行タスクの管理
- `pkg/devices`: デバイスイベント監視（USB 等）
- `pkg/voice`: 音声文字起こし（Groq Transcriber）
- `pkg/state`: ステート管理（デバイスイベント用）

**その他のサブコマンドで依存**:
- `pkg/migrate`: OpenClaw からのマイグレーション
- `pkg/auth`: OAuth/Token 認証
- `pkg/skills`: スキルの読み込み・インストール
- `pkg/tools`: Cron ツールの登録

**外部ライブラリ**:
- `github.com/chzyer/readline`: インタラクティブモードのコマンド履歴
- `github.com/caarlos0/env/v11`: 環境変数パース

### 被依存（このモジュールに依存する他モジュール）

- **直接の被依存なし**: エントリポイントのため、他モジュールから依存されない。

---

## 構造マップ

### ファイル構成

```
cmd/picoclaw/
  main.go              # エントリポイント、サブコマンド振り分け、起動処理

config/
  config.example.json  # 設定ファイルのサンプル
  watchdog.env.example # Watchdog 用の環境変数サンプル（未使用？）

pkg/config/
  config.go            # 設定構造体、読み込み・書き込み関数
```

### 主要な型・構造体

**`pkg/config/config.go`**:
- `Config`: 全体設定のルート構造体
  - `Agents`: エージェントのデフォルト設定（workspace, model, max_tokens 等）
  - `Channels`: 各種チャネル（LINE, Slack, Telegram, Discord, Feishu, DingTalk, WhatsApp, MaixCam, QQ, OneBot）の設定
  - `Providers`: LLM プロバイダー（Anthropic, OpenAI, Ollama, DeepSeek, Groq, Zhipu, Gemini, VLLM, Nvidia, Moonshot, ShengSuanYun, GitHubCopilot）の API Key/Base URL
  - `Gateway`: ゲートウェイのホスト・ポート設定
  - `Watchdog`: 監視・自動再起動の設定
  - `Tools`: Web 検索（Brave, DuckDuckGo, Perplexity）、Cron の設定
  - `Routing`: ルーティング決定の設定（分類器、LLM 割り当て）
  - `Loop`: ループ制御の設定（最大ループ回数、タイムアウト、再ルーティング許可）
  - `Heartbeat`: ハートビートの有効化・間隔設定
  - `Devices`: デバイスイベント監視の設定
  - `MCP`: MCP（Chrome DevTools Protocol）統合の設定
  - `mu`: sync.RWMutex（設定読み書き時のロック）※Phase 2 で追加

※Phase 2 で修正: Channels に Discord, Feishu, DingTalk, WhatsApp, MaixCam, QQ, OneBot を追加。Providers に DeepSeek, Groq, Zhipu, Gemini, VLLM, Nvidia, Moonshot, ShengSuanYun, GitHubCopilot を追加。Config に mu フィールドを追加。

- `FlexibleStringSlice`: JSON で数値と文字列の混在を許容する `[]string` 型（allow_from フィールド用）

※Phase 2 で追加: 以下の構造体も定義されている:
- `AgentsConfig` / `AgentDefaults`
- `ChannelsConfig` と各種チャネル設定（WhatsAppConfig, TelegramConfig, FeishuConfig, DiscordConfig, MaixCamConfig, QQConfig, DingTalkConfig, SlackConfig, LINEConfig, OneBotConfig）
- `HeartbeatConfig`, `DevicesConfig`, `MCPConfig`, `MCPChromeConfig`
- `ProvidersConfig`, `ProviderConfig`（AuthMethod, ConnectMode フィールドを持つ）
- `GatewayConfig`, `WatchdogConfig`
- `BraveConfig`, `DuckDuckGoConfig`, `PerplexityConfig`, `WebToolsConfig`, `CronToolsConfig`, `ToolsConfig`
- `RoutingConfig`, `RoutingClassifierConfig`, `RouteLLMConfig`（Coder/Coder2/Coder3 のalias/provider/model を持つ）
- `LoopConfig`

### 主要な関数・メソッド

**`cmd/picoclaw/main.go`**:
- `main()`: コマンドライン引数を解析し、サブコマンドに振り分け
- `onboard()`: 初回セットアップ（config.json とワークスペースを作成）
- `agentCmd()`: Agent モードで起動（対話型または `-m` で単発実行）
- `gatewayCmd()`: Gateway モードで起動（チャネル統合、ヘルスチェック、各種サービス起動）
- `statusCmd()`: 設定と API Key の状態を表示
- `authCmd()`: OAuth/Token 認証の管理（login/logout/status）
- `cronCmd()`: 定期実行タスクの管理（list/add/remove/enable/disable）
- `skillsCmd()`: スキルの管理（list/install/remove/install-builtin/list-builtin/search/show）※Phase 2 で修正
- `migrateCmd()`: OpenClaw からのマイグレーション

※Phase 2 で追加: 以下のサブコマンド用ヘルパー関数も定義されている:
- `printHelp()`, `printVersion()`, `formatVersion()`, `formatBuildInfo()`
- `migrateHelp()`, `authHelp()`, `cronHelp()`, `skillsHelp()`
- `authLoginCmd()`, `authLoginOpenAI()`, `authLoginPasteToken()`, `authLogoutCmd()`, `authStatusCmd()`
- `cronListCmd()`, `cronAddCmd()`, `cronRemoveCmd()`, `cronEnableCmd()`
- `skillsListCmd()`, `skillsInstallCmd()`, `skillsRemoveCmd()`, `skillsInstallBuiltinCmd()`, `skillsListBuiltinCmd()`, `skillsSearchCmd()`, `skillsShowCmd()`
- `interactiveMode()`, `simpleInteractiveMode()`: Agent 対話モード（readline による履歴管理）
- `copyDirectory()`, `copyEmbeddedToTarget()`, `createWorkspaceTemplates()`: ワークスペーステンプレートの展開
- `getConfigPath()`: 設定ファイルパスを返す（`~/.picoclaw/config.json`）
- `setupCronTool()`: Cron サービスと Tool を初期化
- `loadConfig()`: 設定ファイルを読み込む

**`pkg/config/config.go`**:
- `DefaultConfig()`: デフォルト設定を返す
- `LoadConfig(path)`: JSON ファイルから設定を読み込み、環境変数で上書き
- `SaveConfig(path, cfg)`: 設定を JSON ファイルに保存（RWMutex でロック）※Phase 2 で修正
- `WorkspacePath()`: ワークスペースパスを返す（`~` を展開）（RWMutex でロック）※Phase 2 で修正
- `GetAPIKey()`: 優先順位付きで API Key を返す（OpenRouter > Anthropic > OpenAI > Gemini > Zhipu > Groq > VLLM > ShengSuanYun）※Phase 2 で修正
- `GetAPIBase()`: API Base URL を返す（OpenRouter > Zhipu > VLLM 優先）※Phase 2 で修正
- `expandHome(path)`: `~` 始まりのパスをホームディレクトリに展開（内部関数）※Phase 2 で追加

### 起動フロー

#### onboard（初回セットアップ）
1. `getConfigPath()` で設定ファイルのパス取得（`~/.picoclaw/config.json`）
2. 既存設定がある場合は上書き確認
3. `config.DefaultConfig()` でデフォルト設定を生成
4. `config.SaveConfig()` で設定ファイルに書き込み
5. `createWorkspaceTemplates()` でワークスペースにテンプレートを展開
6. 次のステップ（API Key 設定、agent 起動）を案内

#### agent（Agent モード）
1. `loadConfig()` で設定を読み込み
2. `providers.CreateProvider(cfg)` で LLM プロバイダーを初期化
3. `bus.NewMessageBus()` でメッセージバスを作成
4. `agent.NewAgentLoop(cfg, msgBus, provider)` で Agent Loop を初期化
5. `-m` オプションがあれば単発実行、なければインタラクティブモード
6. インタラクティブモードでは readline でコマンド履歴機能を提供

#### gateway（Gateway モード）
※Phase 2 で詳細化: 以下の起動フローは `gatewayCmd()` (L520-729) の実装と一致することを確認。

1. `loadConfig()` で設定を読み込み（L531）
2. `providers.CreateProvider(cfg)` で LLM プロバイダーを初期化（L537）
3. `bus.NewMessageBus()` でメッセージバスを作成（L543）
4. `agent.NewAgentLoop(cfg, msgBus, provider)` で Agent Loop を初期化（L544）
5. Agent 起動情報を表示（L547-562）※Phase 2 で追加
6. `setupCronTool()` で Cron サービスと Tool を登録（L566）
7. `heartbeat.NewHeartbeatService()` でハートビートサービスを初期化（L568-590）
   - ハートビートハンドラを設定（L574-590）※Phase 2 で追加
8. `channels.NewManager(cfg, msgBus)` でチャネルマネージャーを初期化（L592）
9. `agentLoop.SetChannelManager(channelManager)` で Agent Loop にチャネルマネージャーを注入（L599）※Phase 2 で追加
10. `voice.NewGroqTranscriber()` で音声文字起こしを初期化（Groq API Key がある場合、L602-626）
    - Telegram/Discord/Slack チャネルに transcriber を注入（L608-625）※Phase 2 で追加
11. 有効なチャネルを表示（L628-633）※Phase 2 で追加
12. `devices.NewService()` でデバイスイベント監視を初期化（L651-661）
13. `health.NewServer()` でヘルスチェックサーバーを初期化（L667）
14. Ollama が設定されている場合、ヘルスチェックに以下を追加（L669-705）:
    - `health.OllamaCheck()`: Ollama サーバーの生存確認（30秒ごと、L671-672）
    - `health.OllamaModelsCheck()`: 必要なモデルのロード状態確認、MaxContext 制約チェック（8192 を超える context_length は NG、L674-701）
    - 対象モデル: Chat/Worker/Coder/Coder2 で provider が "ollama" のもののみ（Coder3 は対象外）※Phase 2 で追加
15. 各サービスを起動（L641-665）:
    - Cron サービスを起動（L641-644）
    - Heartbeat サービスを起動（L646-649）
    - Device サービスを起動（L657-661）
    - チャネルマネージャーを起動（L663-665）
16. ヘルスチェックサーバーを goroutine で起動（L707-712）
17. `agentLoop.Run(ctx)` で Agent Loop を goroutine で起動（L714）
18. シグナル（Ctrl+C）を待機し、受信時にすべてのサービスを停止（L716-728）:
    - context をキャンセル（L721）
    - ヘルスチェックサーバーを停止（L722）
    - Device/Heartbeat/Cron サービスを停止（L723-725）
    - Agent Loop を停止（L726）
    - チャネルマネージャーを停止（L727）

---

## 落とし穴・注意点

### 設計上の制約

1. **設定ファイルの場所**: `~/.picoclaw/config.json` 固定（環境変数 `PICOCLAW_CONFIG_PATH` による上書きは未サポート）
2. **ワークスペースパスの展開**: `~` 始まりのパスは `expandHome()` で展開されるが、環境変数参照（`$HOME` 等）は未サポート
3. **API Key の優先順位**: `GetAPIKey()` は OpenRouter → Anthropic → OpenAI → Gemini → Zhipu → Groq → VLLM → ShengSuanYun の順で検索するため、複数設定時の挙動に注意（L541-565）※Phase 2 で修正
4. **環境変数の命名**: `PICOCLAW_*` 形式で、構造体のフィールド名とマッピングされる（例: `PICOCLAW_ROUTING_LLM_CHAT_MODEL`）
5. **JSON 設定と環境変数の優先順位**: 環境変数が JSON 設定を上書きする（`env.Parse(cfg)` が後実行、L508）※Phase 2 で追加
6. **サブコマンドの引数パース**: 各サブコマンドは独自の引数パースロジックを持つ（例: `agentCmd()` では `-m`/`--message`, `-s`/`--session`, `--debug`）。標準の flag パッケージや CLI ライブラリを使用していないため、拡張性に制限がある。※Phase 2 で追加
7. **Cron ジョブの保存先**: Cron ジョブは `<workspace>/cron/jobs.json` に保存される（L1031, L1068）。ワークスペースをクリーンアップする際は注意が必要。※Phase 2 で追加
8. **OAuth ストアの場所**: OAuth/Token 認証情報は `pkg/auth` パッケージで管理されるが、保存先は core モジュールでは制御しない（別モジュールの責務）。※Phase 2 で追加

### 既知の問題・リスク

1. **設定ファイルの後方互換性**: 旧キー（`code_provider/code_model`）は許容されるが、新規設定では使用しないこと
2. **Ollama の MaxContext 制約**: ヘルスチェックで `MaxContext=8192` を超える context_length のモデルを検出した場合、NG として扱う（`pkg/health/checks.go`）。設定で 8192 を超える context_length のモデルを指定しないこと。
3. **環境変数のバリデーション不足**: 環境変数で不正な値（例: 範囲外の confidence 値）を設定しても、実行時エラーになるまで検出されない（※推測）
4. **Workspace テンプレートの展開エラー**: `createWorkspaceTemplates()` でエラーが発生しても、エラーメッセージを出力するだけで処理は継続する（L297-300）
5. **OAuth/Token 認証の状態管理**: `auth.LoadStore()` でストアの読み込みに失敗した場合、エラーは返すが、部分的に破損した場合の挙動は不明（※推測）

※Phase 2 で追加:
6. **embed.FS の依存**: ワークスペーステンプレート（`workspace/`）は `//go:embed` でバイナリに埋め込まれる。ビルド時に `go generate` を実行して `workspace/` をコピーする必要がある（L43）。ビルド環境で `workspace/` が欠損している場合、埋め込みが失敗する。
7. **スキルインストールのタイムアウト**: `skillsInstallCmd()` と `skillsSearchCmd()` は 30 秒のタイムアウトを設定しているが、GitHub API のレート制限や大きなリポジトリのクローン時には不足する可能性がある（L1313, L1426）。
8. **ビルドイン スキルのハードコード**: `skillsInstallBuiltinCmd()` は特定のスキル名（weather, news, stock, calculator）をハードコードしている（L1341-1346）。実際のビルトインスキルディレクトリと不一致の場合、エラーになる。
9. **config.json のパス固定**: 設定ファイルのパスは `~/.picoclaw/config.json` に固定されており、環境変数 `PICOCLAW_CONFIG_PATH` による上書きは未サポート（L1025-1027）。複数プロファイルの運用には不便。
10. **Readline のフォールバック**: `interactiveMode()` で readline の初期化に失敗した場合、`simpleInteractiveMode()` にフォールバックするが、コマンド履歴機能は失われる（L445-449）。エラーメッセージは出るが、ユーザーは履歴機能がないことに気づかない可能性がある。
11. **チャネルマネージャーへの Transcriber 注入**: `gatewayCmd()` では、Groq transcriber を Telegram/Discord/Slack チャネルに個別に注入している（L608-625）。新しいチャネルを追加した際に、この注入処理を忘れると、音声文字起こし機能が動作しない。※Phase 2 で追加
12. **Ollama モデルチェックの対象外**: Coder3（Claude API）は Ollama モデルチェックの対象外（L674-701）。Coder3 の provider に誤って "ollama" を設定しても、ヘルスチェックで検出されない。※Phase 2 で追加
13. **Gateway 起動時の出力形式**: `gatewayCmd()` では、各サービスの起動状態を標準出力に `✓` マークで表示しているが、ログファイルには記録されない。運用監視時にログだけを見ている場合、起動状態が追跡できない。※Phase 2 で追加
14. **Heartbeat ハンドラの channel/chatID フォールバック**: Heartbeat サービスのハンドラ（L574-590）では、channel/chatID が空の場合に "cli"/"direct" にフォールバックするが、実際の Heartbeat 設定（workspace 内の heartbeat ファイル）に channel/chatID が記録されていない場合、通知先が不明確になる。※Phase 2 で追加
15. **embed.FS のビルド時依存**: `//go:embed workspace` による埋め込みは、ビルド時に `workspace/` ディレクトリが存在することを前提とする。CI/CD 環境でビルドする際に、`go generate` を実行し忘れると、埋め込みに失敗する。※Phase 2 で追加

### 変更時の注意事項

1. **設定構造体の追加・変更**: `pkg/config/config.go` の `Config` 構造体にフィールドを追加する場合、以下を忘れずに:
   - `DefaultConfig()` でデフォルト値を設定
   - 環境変数タグ（`env:"PICOCLAW_*"`）を追加
   - `config.example.json` にサンプル値を追加
   - 既存の設定ファイルとの後方互換性を確保（フィールド追加は OK、削除・リネームは注意）

2. **サブコマンドの追加**: `main()` の switch 文に新しい case を追加し、対応する `*Cmd()` 関数を実装。ヘルプメッセージ（`printHelp()`）も更新すること。

3. **Gateway 起動時のサービス初期化順序**: サービス間の依存関係に注意。特に Agent Loop は他のサービス（Cron, Heartbeat）から呼び出されるため、先に初期化すること。

4. **ヘルスチェックの追加**: `health.NewServer()` に新しいチェックを追加する場合、`RunPeriodicCheck()` で定期実行を登録。Ollama モデルチェックのように、設定に応じて動的にチェックを追加する場合は、Gateway 起動時の処理（L669-702）を参考にすること。

5. **ルーティング設定の拡張**: Coder3 のように新しい LLM 割り当てを追加する場合:
   - `RouteLLMConfig` に `*Alias`, `*Provider`, `*Model` フィールドを追加
   - `DefaultConfig()` でデフォルト値を設定（空文字列でも OK）
   - `config.example.json` にサンプルを追加
   - `pkg/agent/router.go` のルーティングロジックに対応を追加（別モジュールの責務）

6. **JSON 互換性のテスト**: 設定構造体を変更した場合、旧バージョンの `config.json` が読み込めることを確認すること。`FlexibleStringSlice` のように、互換性のための型を追加することも検討。

※Phase 2 で追加:
7. **環境変数タグのテンプレート**: `ProviderConfig` の環境変数タグは `{{.Name}}` テンプレートを使用している（例: `env:"PICOCLAW_PROVIDERS_{{.Name}}_API_KEY"`）。実際には `caarlos0/env` ライブラリがこのタンプレートを展開するのではなく、各プロバイダー固有のタグが必要。現状の実装では、環境変数による上書きが正しく動作しない可能性がある。※要検証
8. **RWMutex の使用**: `Config` 構造体に `mu sync.RWMutex` が追加されているが、すべてのメソッドでロックが使用されているわけではない。`DefaultConfig()` や `LoadConfig()` では mu が初期化されていない状態で返されるため、後続の `WorkspacePath()` や `GetAPIKey()` でのロック取得は機能するが、`SaveConfig()` の前に `LoadConfig()` で返された Config を別スレッドから読み書きする場合は注意が必要。※推測
9. **Ollama 再起動コマンドの未使用**: `ProvidersConfig.OllamaRestartCommand` フィールドは定義されているが、main.go 内では使用されていない。実際の再起動処理は `pkg/health/` や別のパッケージで実装されている可能性がある。※要確認
10. **デバッグフラグの統一性**: `agentCmd()` と `gatewayCmd()` では `--debug` / `-d` フラグでログレベルを DEBUG に変更できるが、他のサブコマンドでは未対応。デバッグ時の利便性のため、全サブコマンドで統一することを検討。

---

## 補足: 主要設定項目のデフォルト値

※Phase 2 で検証: 以下のデフォルト値は `pkg/config/config.go` の `DefaultConfig()` と一致することを確認。

**エージェント設定**:
- `workspace`: `~/.picoclaw/workspace`
- `restrict_to_workspace`: `true`
- `model`: `glm-4.7`（※ 実際は routing.llm で LLM ごとに指定）
- `max_tokens`: 8192
- `temperature`: 0.7
- `max_tool_iterations`: 20

**ルーティング設定**:
- `classifier.enabled`: `true`
- `classifier.min_confidence`: 0.6
- `classifier.min_confidence_for_code`: 0.8
- `fallback_route`: `CHAT`
- `llm.chat_alias`: `Mio`
- `llm.worker_alias`: `Shiro`
- `llm.coder_alias`: `Aka`（Coder1）
- `llm.coder2_alias`: `""`（空文字列、未設定）※Phase 2 で修正
- `llm.coder3_alias`: `Claude`（Coder3）
- `llm.*_provider`: `""`（すべて空文字列、未設定）※Phase 2 で追加
- `llm.*_model`: `""`（すべて空文字列、未設定）※Phase 2 で追加

**ループ制御設定**:
- `max_loops`: 3
- `max_millis`: 25000（25 秒）
- `allow_auto_reroute_once`: `true`
- `allow_chat_propose_reroute_once`: `true`

**ゲートウェイ設定**:
- `host`: `0.0.0.0`
- `port`: 18790

**ハートビート設定**:
- `enabled`: `true`
- `interval`: 30（分）

**Cron 設定**:
- `exec_timeout_minutes`: 5

**MCP 設定**:
- `chrome.enabled`: `false`
- `chrome.base_url`: `http://100.83.235.65:12306`
- `chrome.timeout_sec`: 30

**DuckDuckGo Web 検索設定** ※Phase 2 で追加:
- `tools.web.duckduckgo.enabled`: `true`（デフォルトで有効）
- `tools.web.duckduckgo.max_results`: 5

---

## Phase 2 検証結果: 設計書（実装仕様.md）との乖離

### 乖離なし（一致）
- 1章「スコープ・責務境界」の1.1節「役割命名と実体」: `RouteLLMConfig` の coder/coder2/coder3 フィールドは実装仕様通り。
- 10章「設定値と閾値」の設定値: `routing.classifier.min_confidence`, `loop.max_loops`, `loop.max_millis` 等はデフォルト値と一致。
- Ollama の `keep_alive: -1` 設定: 実装仕様に記載されているが、実際の送信処理は `pkg/providers/` で実装される（core モジュールの責務外）。

### 軽微な乖離（仕様に影響なし）
- **coder2_alias のデフォルト値**: 実装仕様では `Ao` だが、実コードでは空文字列（L455）。config.example.json では `Ao` が設定されている（L157）。デフォルトで未設定にすることで、Coder2 を使用しない運用も可能にしている。※仕様に影響なし、運用上の柔軟性向上
- **loop.max_millis のデフォルト値**: 実装仕様では `90000`（90秒）だが、実コードでは `25000`（25秒）（L471）。これは実装仕様の誤記と思われる（仕様書の10章では 25000 と記載）。※要確認

### 設計書に記載のない実装（拡張）
- **Watchdog 設定**: 実装仕様には記載がないが、`WatchdogConfig` が実装されている（L213-233）。外部監視プロセス用の設定と思われる。
- **DuckDuckGo のデフォルト有効化**: 実装仕様には Web 検索の設定が記載されているが、DuckDuckGo がデフォルトで有効（L431）であることは明示されていない。
- **Devices 設定**: 実装仕様には記載がないが、`DevicesConfig` が実装されている（L168-171）。USB デバイスイベント監視の設定。
- **GitHubCopilot プロバイダー**: 実装仕様には記載がないが、`ProvidersConfig` に `GitHubCopilot` フィールドが追加されている（L196）。※実験的機能の可能性

---

## 参考: 起動例

```bash
# 初回セットアップ
picoclaw onboard

# Agent モードで対話
picoclaw agent

# Agent モードで単発実行
picoclaw agent -m "今日の天気は？"

# Gateway モードで起動
picoclaw gateway

# デバッグモード有効化
picoclaw agent --debug
picoclaw gateway --debug

# ステータス確認
picoclaw status

# バージョン確認
picoclaw version

# Cron ジョブ管理 ※Phase 2 で追加
picoclaw cron list
picoclaw cron add -n "daily-backup" -m "バックアップを実行" -e 86400

# Skills 管理 ※Phase 2 で追加
picoclaw skills list
picoclaw skills install sipeed/picoclaw-skills/weather
picoclaw skills install-builtin

# 認証管理 ※Phase 2 で追加
picoclaw auth login --provider openai
picoclaw auth status
picoclaw auth logout --provider openai
```

---

## Phase 2 検証サマリ

### 検証実施日
2026-02-28

### 検証対象
- cmd/picoclaw/main.go（1467 行）
- pkg/config/config.go（599 行）
- config/config.example.json（190 行）
- docs/01_正本仕様/実装仕様.md（1章: スコープ・責務境界、10章: 設定値と閾値）

### Phase 1 ドキュメントとの突合せ結果

**修正箇所**:
- 主要な責務に Skills 管理とバージョン情報管理を追加
- 主要な型・構造体に Channel/Provider の拡張（Discord, Feishu, DeepSeek, Groq 等）を反映
- 主要な関数・メソッドにサブコマンドヘルパー関数を追加
- デフォルト値の誤記を修正（coder2_alias は空文字列）
- API Key 優先順位を最新の実装に合わせて修正

**追加箇所**:
- 既知の問題・リスクに 10 項目の新たな落とし穴を追加
- 変更時の注意事項に 4 項目の実装詳細を追加
- 設計上の制約に 3 項目の制約を追加
- gateway 起動フローの詳細化（行番号付き）
- 設計書との乖離分析を追加

**検出された重要な落とし穴**:
1. embed.FS のビルド時依存（`go generate` 必須）
2. スキルインストールのタイムアウト不足（30秒固定）
3. ビルトインスキルのハードコード（拡張性の制限）
4. 環境変数タグのテンプレート問題（要検証）
5. RWMutex の初期化タイミング（要注意）
6. Ollama モデルチェックの対象外（Coder3）
7. チャネルマネージャーへの Transcriber 注入忘れのリスク
8. Gateway 起動時の標準出力のみ（ログファイル未記録）

**設計書との乖離**:
- 軽微な乖離: coder2_alias のデフォルト値（空文字列 vs "Ao"）
- 軽微な乖離: loop.max_millis の記載不一致（要確認）
- 拡張実装: Watchdog, Devices, GitHubCopilot（仕様に記載なし）

### 次のアクション
- [ ] 環境変数タグのテンプレート問題を検証（実際の env ライブラリの挙動を確認）
- [ ] loop.max_millis のデフォルト値を仕様書と突合せ（90000 vs 25000）
- [ ] Ollama 再起動コマンドの使用箇所を調査（pkg/health/ 等）
- [ ] Gateway 起動時のログ出力を追加（運用監視のため）

---

**Phase 2 検証完了**: core モジュールの Phase 2 ボトムアップ検証を完了しました。
