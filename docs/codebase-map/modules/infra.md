---
generated_at: 2026-02-28T08:31:12Z
run_id: run_20260228_170007
phase: 1
step: "1-7"
profile: picoclaw_multiLLM
artifact: module
module_group_id: infra
---

# インフラ層（ログ・設定）

## 概要

PicoClaw の観測性（Observability）と設定管理を担う基盤層。構造化ログによるトレーサビリティの確保、環境変数・JSON ファイルによる柔軟な設定管理、API キー等の安全な取り扱いを提供する。

## 関連ドキュメント

- **プロファイル**: `codebase-analysis-profile.yaml`
- **外部資料**: `docs/codebase-map/refs_mapping.md` (infra セクション)
- **実装仕様**:
  - `docs/01_正本仕様/実装仕様.md` (7章「ログ」)
  - `docs/02_v2統合分割仕様/実装仕様_v2_07_ログ.md` (イベント種別)
  - `docs/02_v2統合分割仕様/実装仕様_v2_10_設定閾値.md` (設定値)
- **運用ガイド**:
  - `docs/06_実装ガイド進行管理/20260220_監視実装検証レポート.md`
  - `docs/06_実装ガイド進行管理/20260220_常駐監視運用手順.md`
  - `docs/06_実装ガイド進行管理/20260224_ヘルスチェック強化とテスト追加.md`

---

## 役割と責務

### 主要な責務

#### **pkg/logger**: 構造化ログの一元管理
- **5段階ログレベル**: DEBUG / INFO / WARN / ERROR / FATAL
- **構造化出力**: JSON 形式でのログ永続化（観測・監査用）
- **標準出力**: 人間可読な整形ログ（開発・デバッグ用）
- **コンポーネント識別**: `component` タグによるログ発生源の明示
- **フィールド拡張**: `fields` による任意メタデータの付与
- **承認フロー専用ヘルパー**: `LogApprovalRequested`, `LogApprovalGranted`, `LogApprovalDenied`, `LogApprovalAutoApproved`
- **Coder 専用ヘルパー**: `LogCoderPlanGenerated`

#### **pkg/config**: 設定の読み込みとバリデーション
- **デフォルト設定**: `DefaultConfig()` による安全なフォールバック
- **JSON + 環境変数**: 設定ファイルと環境変数の階層的マージ（環境変数が優先）
- **型安全な設定構造**: `Config` 構造体による静的型付け
- **柔軟な型変換**: `FlexibleStringSlice` による JSON 数値・文字列混在の許容
- **パス展開**: チルダ (`~`) 展開による HOME ディレクトリ対応
- **並行アクセス制御**: `sync.RWMutex` による設定読み取り/更新の保護

### 対外インターフェース

#### **logger パッケージの公開関数**

**基本ログ出力（5レベル × 4バリエーション = 20関数）**:
- `Debug(message)`, `DebugC(component, message)`, `DebugF(message, fields)`, `DebugCF(component, message, fields)`
- `Info*`, `Warn*`, `Error*`, `Fatal*` も同様のバリエーション

**承認フロー専用ログ**:
- `LogApprovalRequested(jobID, plan, patch, risk)`: 承認要求の記録
- `LogApprovalGranted(jobID, approver)`: 承認許可の記録
- `LogApprovalDenied(jobID, approver)`: 承認拒否の記録
- `LogApprovalAutoApproved(jobID, reason)`: 自動承認の記録

**Coder 専用ログ**:
- `LogCoderPlanGenerated(jobID, plan)`: plan/patch 生成の記録

**ログ制御**:
- `SetLevel(level)`: ログレベルの動的変更
- `GetLevel()`: 現在のログレベル取得
- `EnableFileLogging(filePath)`: ファイル出力の有効化（JSON 形式）
- `DisableFileLogging()`: ファイル出力の無効化

#### **config パッケージの公開関数**

- `DefaultConfig()`: デフォルト設定の生成
- `LoadConfig(path)`: JSON ファイル + 環境変数からの設定読み込み
- `SaveConfig(path, cfg)`: 設定の JSON ファイル保存（権限 0600）
- `Config.WorkspacePath()`: workspace パスの取得（チルダ展開済み）
- `Config.GetAPIKey()`: 優先順位に従った API キー取得（OpenRouter > Anthropic > OpenAI > ...）
- `Config.GetAPIBase()`: API Base URL の取得

### 内部構造

#### **logger.LogEntry 構造**
```go
type LogEntry struct {
    Level     string                 // DEBUG/INFO/WARN/ERROR/FATAL
    Timestamp string                 // RFC3339 UTC
    Component string                 // 発生源コンポーネント（router, classifier, approval, coder 等）
    Message   string                 // ログメッセージ
    Fields    map[string]interface{} // 任意の構造化データ
    Caller    string                 // 呼び出し元（ファイル:行番号 (関数名)）
}
```

#### **config.Config 主要構造**
```go
type Config struct {
    Agents    AgentsConfig    // エージェント設定（workspace, model, temperature 等）
    Channels  ChannelsConfig  // チャネル設定（LINE, Slack, Telegram 等）
    Providers ProvidersConfig // LLM プロバイダー設定（API キー、エンドポイント）
    Gateway   GatewayConfig   // Gateway サーバー設定
    Watchdog  WatchdogConfig  // 監視・自動再起動設定
    Tools     ToolsConfig     // ツール（検索、Cron）設定
    Routing   RoutingConfig   // ルーティング設定（分類器閾値、フォールバック）
    Loop      LoopConfig      // ループ制御設定（最大ループ数、再ルート許可）
    Heartbeat HeartbeatConfig // ハートビート設定
    Devices   DevicesConfig   // デバイス監視設定
    MCP       MCPConfig       // MCP 統合設定（Chrome DevTools 等）
}
```

---

## 依存関係

### 外部依存

#### **logger パッケージ**
- **標準ライブラリのみ**: `encoding/json`, `fmt`, `log`, `os`, `runtime`, `strings`, `sync`, `time`
- **外部依存なし**: 純粋な Go 標準機能のみで実装

#### **config パッケージ**
- **`github.com/caarlos0/env/v11`**: 環境変数のパースとマージ
- **標準ライブラリ**: `encoding/json`, `fmt`, `os`, `path/filepath`, `sync`

### 被依存（全モジュールから参照される基盤層）

**logger パッケージは全モジュールで使用**（約 10 パッケージ以上）:
- `pkg/agent` (router, loop, classifier, context)
- `pkg/channels` (line, slack, telegram, discord, feishu, onebot 等)
- `pkg/providers` (http_provider, codex_provider)
- `pkg/heartbeat`, `pkg/devices`, `pkg/voice`, `pkg/tools`, `pkg/session`

**config パッケージは起動時に読み込まれる**:
- `cmd/picoclaw/main.go`: 起動時に `LoadConfig` でグローバル設定を読み込み
- `pkg/migrate`: 設定マイグレーション（旧設定から新設定への移行）
- 各モジュールは設定インスタンスを参照（DI パターン）

**※特記事項**: logger パッケージは他パッケージに依存しない設計（循環参照の回避）。

---

## 構造マップ

### ファイル構成

```
pkg/
├── logger/
│   ├── logger.go       # 構造化ログ実装（284行）
│   └── logger_test.go  # ログテスト
└── config/
    ├── config.go       # 設定管理実装（599行）
    └── config_test.go  # 設定テスト（デフォルト値検証）
```

### ログイベント種別（仕様定義）

**ルーティング・分類器**:
- `router.decision`: ルーティング決定
- `classifier.error`: 分類器エラー
- `route.override`: ルート上書き

**ワーカー実行**:
- `worker.success`: Worker 実行成功
- `worker.fail`: Worker 実行失敗

**ループ制御**:
- `loop.stop`: ループ停止
- `final.route`: 最終ルート確定

**承認フロー（logger に専用関数あり）**:
- `approval.requested`: 承認要求
- `approval.granted`: 承認許可
- `approval.denied`: 承認拒否
- `approval.auto_approved`: 自動承認

**Coder**:
- `coder.plan_generated`: plan/patch 生成

※推測: 実際のログ出力時には `logger.InfoCF("approval", "approval.requested", fields)` のように `component` と `message` で記録。

### マスキング機構

**現状**: API キー等の自動マスキング機構は **未実装**。

**事実確認**（※Phase 2 で再確認）:
- `logger.go` (283 行) には `maskSensitive|MaskSecret|mask` 等のマスキング処理なし
- L119-124 でファイル書き込み時に JSON を直接書き込み、フィールドのフィルタリングなし
- L153-159 で `formatFields` は `fmt.Sprintf("%s=%v", k, v)` で単純出力、マスキングなし

**仕様上の要求**:
- `CLAUDE.md` 3.4.3節「API キー管理」: 環境変数からの取得、平文保存禁止
- `docs/01_正本仕様/実装仕様.md` 6章「セキュリティ」: 承認フロー、安全な取り扱い（詳細は未確認）

**※推測**: API キーは環境変数で管理されログに出力されない設計を前提としており、積極的なマスキングは不要と判断された可能性。

### 設定構造（主要項目）

#### **ルーティング設定** (`Routing`)
```go
Classifier:
  - Enabled: true                    // 分類器の有効化
  - MinConfidence: 0.6               // 一般判定の最低信頼度
  - MinConfidenceForCode: 0.8        // CODE 判定の最低信頼度
FallbackRoute: "CHAT"                // 分類失敗時のフォールバック
LLM:
  - ChatAlias: "Mio", ChatProvider: "ollama", ChatModel: "chat-v1:latest"
  - WorkerAlias: "Shiro", WorkerProvider: "ollama", WorkerModel: "worker-v1:latest"
  - CoderAlias: "Aka", CoderProvider: "deepseek"
  - Coder2Alias: "Ao", Coder2Provider: "openai"
  - Coder3Alias: "Claude", Coder3Provider: "anthropic", Coder3Model: "claude-sonnet-4.5"
```

#### **ループ設定** (`Loop`)
```go
MaxLoops: 3                          // 最大ループ数
MaxMillis: 25000                     // タイムアウト（ミリ秒）
AllowAutoRerouteOnce: true           // 自動再ルートの1回許可
AllowChatProposeRerouteOnce: true    // Chat 提案再ルートの1回許可
```

#### **監視設定** (`Watchdog`)
```go
Enabled: false                       // デフォルト無効
IntervalSec: 60                      // 監視間隔（秒）
HealthURL: "http://127.0.0.1:18790/health"
OllamaModelsURL: "http://100.83.207.6:11434/v1/models"
RestartWindowSec: 600                // 再起動窓（10分）
RestartMaxCount: 3                   // 窓内最大再起動回数
```

#### **環境変数の優先順位**
1. **環境変数**（最優先）: `PICOCLAW_*` プレフィックスで各設定を上書き可能
   - 例: `PICOCLAW_ROUTING_LLM_CHAT_PROVIDER=ollama`
2. **JSON ファイル**: 指定パスの設定ファイル
3. **デフォルト値**: `DefaultConfig()` による安全なフォールバック

※環境変数パースは `github.com/caarlos0/env/v11` を使用。

---

## 落とし穴・注意点

### 設計上の制約

#### **ログの並行安全性**
- **シングルトンパターン**: `logger` は `init()` で一度だけ生成（`sync.Once`）
- **ファイル書き込み**: `mu.Lock()` による排他制御（並行書き込み時の破損防止）
- **ログレベル変更**: `SetLevel()` は `mu.Lock()` で保護されており、実行中の動的変更が可能
- **落とし穴**: `FATAL` レベルは `os.Exit(1)` で即座に終了するため、defer や cleanup 処理が実行されない

#### **設定の並行アクセス**
- **RWMutex による保護**: 読み取りは `RLock()`, 書き込みは `Lock()` で保護
- **落とし穴**: `LoadConfig()` 自体はロックを取らないため、起動時の1回のみ使用を想定（実行中の再読み込みは考慮されていない）
- **SaveConfig()**: `RLock()` で設定を読み取ってから JSON 書き込み（権限 0600 で安全性確保）

#### **Caller 情報の深度**（※Phase 2 で確認: L112-117）
- **`runtime.Caller(2)`**: ログヘルパー関数を2段階遡って実際の呼び出し元を取得
- **落とし穴**: ログヘルパーをさらにラップすると Caller 情報がずれる（深度調整が必要）
- ※Phase 2 で確認: L112-117 で実装、`runtime.FuncForPC(pc)` でファイル名・行番号・関数名を取得

### 既知の問題・リスク

#### **マスキング機構の不在**
- **現状**: API キー、トークン等の自動マスキングは未実装
- **リスク**: 誤ってログに API キーを含む fields を渡すと平文で記録される
- **緩和策**: 環境変数からの取得を前提とし、設定ファイル・ログへの出力を避ける運用
- **※推測**: 仕様上は環境変数管理を前提としており、積極的なマスキングは後回しにされている可能性

#### **ファイルログのローテーション不在**
- **現状**: `EnableFileLogging()` は append モードでファイルに追記し続ける
- **リスク**: 長時間稼働でログファイルが肥大化
- **緩和策**: 外部ツール（logrotate 等）によるローテーション、または定期的な `DisableFileLogging()` + 再 `Enable`

#### **設定ホットリロードの未サポート**
- **現状**: `LoadConfig()` は起動時のみ想定（実行中の再読み込み機能なし）
- **リスク**: 設定変更には再起動が必要
- **緩和策**: 設計上の制約として受け入れ、重要な変更は計画的に再起動

#### **FlexibleStringSlice の型変換**
- **目的**: JSON で `allow_from: ["123", 456]` のような混在を許容
- **落とし穴**: 数値は `fmt.Sprintf("%.0f", val)` で文字列化されるため、浮動小数点数の場合に精度が失われる可能性（実用上は問題なし）

### 変更時の注意事項

#### **ログヘルパー関数の追加**（※Phase 2 で確認: L241-283）
- **承認フロー専用**: `LogApprovalRequested` 等は `InfoCF("approval", "approval.requested", fields)` をラップ
  - `LogApprovalRequested` (L243-251): component="approval", message="approval.requested"
  - `LogApprovalGranted` (L253-259): component="approval", message="approval.granted"
  - `LogApprovalDenied` (L261-267): component="approval", message="approval.denied"
  - `LogApprovalAutoApproved` (L269-275): component="approval", message="approval.auto_approved"
  - `LogCoderPlanGenerated` (L277-283): component="coder", message="coder.plan_generated"
- **命名規則**: `Log<Domain><Action>` の形式（例: `LogCoderPlanGenerated`）
- **注意**: component と message を固定して呼び出すことで、ログイベント種別の一貫性を保つ

#### **設定構造の拡張**
- **後方互換性**: 旧キー（例: `code_provider`）は残したまま新キー（`coder_provider`）を追加
- **環境変数タグ**: `env:"PICOCLAW_<SECTION>_<KEY>"` の命名規則を厳守
- **デフォルト値**: `DefaultConfig()` に必ず安全なフォールバックを設定

#### **ログレベルの動的変更**
- **デバッグモード**: `cmd/picoclaw/main.go` では `--debug` フラグで `logger.SetLevel(logger.DEBUG)` を設定
- **注意**: ログレベル変更は即座に反映されるが、過去のログには影響しない

#### **環境変数の優先順位**（※Phase 2 で確認: L493-519）
- **`env.Parse(cfg)`**: JSON 読み込み後に環境変数でオーバーライド（L512）
- **注意**: 環境変数が設定されている場合、JSON ファイルの値は無視される（意図しない上書きに注意）
- ※Phase 2 で確認: LoadConfig は L493-519 で実装、JSON 読み込み→環境変数パース→デフォルト値マージの順序

---

## Phase 2 検証結果

### 検証日時
2026-02-28

### 検証内容
- `pkg/logger/logger.go` (283 行) との突合せ完了
- `pkg/config/config.go` (598 行) との突合せ完了

### 発見された差異・追加事項
1. **マスキング機構の不在を再確認**（※Phase 2 で追加）:
   - logger.go にマスキング処理なし（L119-124, L153-159 で確認）
   - フィールドは `fmt.Sprintf("%s=%v", k, v)` で単純出力

2. **Caller 情報取得の実装詳細を追記**（※Phase 2 で追加）:
   - L112-117 で `runtime.Caller(2)` を使用
   - `runtime.FuncForPC(pc)` でファイル名・行番号・関数名を取得

3. **ログヘルパー関数の実装詳細を追記**（※Phase 2 で追加）:
   - L241-283 で 5 つのヘルパー関数を実装
   - 各関数は component と message を固定して InfoCF を呼び出し

4. **環境変数パースの実装詳細を追記**（※Phase 2 で追加）:
   - LoadConfig は L493-519 で実装
   - JSON 読み込み→環境変数パース（L512）→デフォルト値マージの順序

### 構造マップの正確性検証
- ✅ ファイル構成: 正確（logger.go (283 行), config.go (598 行) を確認）
- ✅ ログイベント種別: 正確（approval.requested 等のイベント種別を確認）
- ✅ 設定構造: 正確（Config 構造体、デフォルト値を確認）

### 落とし穴の網羅性検証
- ✅ マスキング機構の不在: 実コードで再確認、実装箇所なし
- ✅ Caller 情報の深度: 実コードで確認、L112-117 で実装
- ✅ ログヘルパー関数: 実コードで確認、L241-283 で実装

### 設計書との乖離
- なし（実装仕様 `docs/01_正本仕様/実装仕様.md` 7章「ログ」と整合）

---

**最終更新**: 2026-02-28 (Phase 2 検証完了)
**解析担当**: Claude Sonnet 4.5 (codebase-analysis)
