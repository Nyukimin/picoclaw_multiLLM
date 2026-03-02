---
generated_at: 2026-02-28T18:00:00+09:00
run_id: run_20260228_170007
phase: 1
step: "1-2"
profile: picoclaw_multiLLM
artifact: module
module_group_id: agent
---

# エージェント（ルーティング・ループ）

## 概要

PicoClaw のリクエスト処理コア。ユーザー入力を受け取り、ルーティング決定（CHAT/PLAN/ANALYZE/OPS/RESEARCH/CODE1/CODE2/CODE3 の選択）、エージェントループ（LLM 反復呼び出しとツール実行）、承認フロー管理を担当する。

## 関連ドキュメント

- プロファイル: `/home/nyukimi/picoclaw_multiLLM/codebase-analysis-profile.yaml`
- 外部資料: `/home/nyukimi/picoclaw_multiLLM/docs/codebase-map/refs_mapping.md` (agent セクション)
  - 実装仕様: `docs/01_正本仕様/実装仕様.md` (2章「ルーティング決定仕様」、3章「ループ制御と再ルート」)
  - ルーティング詳細: `docs/02_v2統合分割仕様/実装仕様_v2_02_ルーティング.md`
  - ループ詳細: `docs/02_v2統合分割仕様/実装仕様_v2_03_ループ再ルート.md`
  - 分類器設計: `docs/05_LLM運用プロンプト設計/LLM_分類整理.md`

---

## 役割と責務

### 主要な責務

1. **ルーティング決定** (router.go)
   - 明示コマンド（`/chat`, `/code`, `/code3` 等）の解析
   - ルール辞書による強証拠マッチ（コードブロック、運用キーワード等）
   - LLM ベース分類器によるカテゴリ判定
   - フォールバック処理（デフォルト CHAT）
   - `/local` モード管理（クラウド LLM 呼び出し制限）

2. **エージェントループ制御** (loop.go)
   - メッセージ受信・処理パイプライン
   - LLM プロバイダーの動的切り替え（CHAT→Ollama chat-v1、CODE3→Claude API 等）
   - LLM 反復呼び出しとツール実行のループ管理
   - 停止条件判定（max_loops, max_millis, 承認待ち等）
   - 日次カットオーバー処理（セッションのリセット・アーカイブ）

3. **承認フロー統合** (loop.go)
   - Coder3（Claude API）の出力解析（plan/patch/risk）
     - 実装箇所: loop.go:490-523（parseCoder3Output 呼び出し）
     - 構造体: Coder3Output（JobID, Plan, Patch, Risk, CostHint, NeedApproval）
   - ジョブ作成と承認要求メッセージ生成
     - approvalMgr.CreateJob(jobID, Plan, Patch, Risk, usesBrowser) で登録
     - approval.FormatApprovalRequest(job) で承認要求メッセージ生成
     - セッションに job_id を保存（flags.PendingApprovalJobID）
   - `/approve <job_id>`, `/deny <job_id>` コマンド処理
     - 実装箇所: loop.go:1910-1947（handleCommand 内）
     - approvalMgr.Approve(jobID, userID) / Deny(jobID, userID)
     - ログ出力: logger.LogApprovalGranted / LogApprovalDenied
   - ※Phase 2 で確認: 承認後の Worker 実行委譲は未実装（Phase 4 予定）
     - 現状は承認完了メッセージのみ返却
     - loop.go:1931 に TODO コメント: "Worker による差分適用は次のフェーズで実装予定"
   - ※Phase 2 で確認: Chrome 操作検出（usesBrowser フラグ設定）も未実装
     - loop.go:501 に TODO コメント: "Patch から Chrome 操作を検出して usesBrowser を設定"

4. **コンテキスト構築** (context.go)
   - システムプロンプト生成（identity, rules, tools, memory）
   - ブートストラップファイルの読み込み（CHAT_PERSONA, USER, IDENTITY 等）
   - FewShot サンプルのローテーション（session_id ベース）
   - 添付ファイル処理とガイダンス生成

5. **メモリ管理** (memory.go)
   - 長期記憶（MEMORY.md）
   - 日次ノート（memory/YYYYMM/YYYYMMDD.md）
   - カットオーバー境界計算（04:00 JST）
   - 論理日付管理（04:00 前は前日扱い）

### 対外インターフェース（公開 API）

- `NewAgentLoop(cfg, bus, workspace, providerName, model) *AgentLoop` - エージェント初期化
- `Run(ctx) error` - メッセージバス購読とメインループ起動
- `Stop()` - 停止シグナル送信
- `ProcessDirect(ctx, content, sessionKey) (string, error)` - CLI からの直接呼び出し
- `ProcessHeartbeat(ctx, content, channel, chatID) (string, error)` - ヘルスチェック用エンドポイント
- `RegisterTool(tool)` - ツール登録
- `SetChannelManager(cm)` - チャネルマネージャー設定

### 内部構造（非公開）

- **ルーティング決定関数**
  - `Router.Decide(ctx, userText, flags) RoutingDecision` - 4段階ルーティングロジック
  - `parseRouteCommand(text, localOnly) (route, nextLocalOnly, directMsg, stripped, ok)` - 明示コマンド解析
  - `matchRule(text) (route, evidence, matched)` - ルール辞書マッチ
  - `hasStrongCodeEvidence(text) bool` - コード証拠検出（コードブロック、拡張子等）
  - `Classifier.Classify(ctx, userText) (Classification, ok)` - LLM ベース分類器

- **ループ処理関数**
  - `processMessage(ctx, msg) (string, error)` - メッセージ処理のエントリポイント
  - `runAgentLoop(ctx, opts) (string, error)` - コアループロジック
  - `runLLMIteration(ctx, messages, opts) (string, int, error)` - LLM 反復呼び出し
  - `applyRouteLLM(route) (restoreFn, error)` - LLM プロバイダー動的切り替え
  - `maybeDailyCutover(sessionKey)` - 日次カットオーバー処理
  - `maybeSummarize(sessionKey, channel, chatID)` - セッション要約

- **コンテキスト構築関数**
  - `ContextBuilder.BuildSystemPrompt(route) string` - システムプロンプト構築
  - `ContextBuilder.BuildMessages(history, summary, currentMessage, media, channel, chatID, route, workOverlay) []Message` - 会話履歴構築
  - `LoadBootstrapFilesForRoute(route) string` - ルート別ブートストラップファイル読み込み
  - `LoadFewShotExamplesWithSeed(seed) string` - FewShot サンプルローテーション

---

## 依存関係

### 外部依存（このモジュールが依存する他モジュール）

- **session** (`pkg/session`)
  - セッション管理: `SessionManager.GetHistory()`, `AddMessage()`, `GetFlags()`, `SetFlags()`, `Save()`
  - 利用箇所: メッセージ履歴取得、フラグ管理（LocalOnly, PrevPrimaryRoute）、日次カットオーバー

- **providers** (`pkg/providers`)
  - LLM プロバイダー抽象化: `LLMProvider.Chat()`, `Message`, `ToolDefinition`
  - 利用箇所: LLM 呼び出し、ツール実行、ルート別プロバイダー切り替え

- **approval** (`pkg/approval`)
  - 承認フロー管理: `Manager.CreateJob()`, `GetJob()`, `Approve()`, `Deny()`
  - 利用箇所: Coder3 の plan/patch 解析後のジョブ作成、`/approve` / `/deny` コマンド処理

- **config** (`pkg/config`)
  - 設定読み込み: `RoutingConfig`, `Classifier.MinConfidence`, `Routing.LLM`
  - 利用箇所: ルーティング閾値、LLM モデル設定、フォールバックルート

- **logger** (`pkg/logger`)
  - ログ出力: `InfoCF()`, `DebugCF()`, `WarnCF()`, `ErrorCF()`
  - 利用箇所: ルーティング決定、LLM 呼び出し、ツール実行、エラー記録

- **bus** (`pkg/bus`)
  - メッセージバス: `MessageBus.SubscribeInbound()`, `PublishOutbound()`
  - 利用箇所: メッセージ受信、応答送信、委譲通知

- **tools** (`pkg/tools`)
  - ツール登録: `ToolRegistry.RegisterTool()`, `ToProviderDefs()`
  - 利用箇所: LLM ツール呼び出し、ツールコンテキスト更新

- **health** (`pkg/health`)
  - ヘルスチェック: `OllamaCheck()`, `OllamaModelsCheck()`
  - 利用箇所: Ollama 監視、自動再起動トリガー

- **mcp** (`pkg/mcp`)
  - MCP クライアント: `Client` (統合中)
  - 利用箇所: Chrome DevTools Protocol 操作（実装進行中）

- **skills** (`pkg/skills`)
  - スキル管理: `SkillsLoader.BuildSkillsSummary()`, `LoadSkillsForContext()`
  - 利用箇所: システムプロンプトへのスキル情報埋め込み

### 被依存（このモジュールに依存する他モジュール）

- **main** (`cmd/main.go`)
  - エージェント初期化と Run() 呼び出し
  - 停止シグナルハンドリング

- **cli** (※推測: CLI インターフェース)
  - `ProcessDirect()` を使った対話モード実装

- **heartbeat** (※推測: ヘルスチェック機能)
  - `ProcessHeartbeat()` を使った監視

---

## 構造マップ

### ファイル構成

```
pkg/agent/
├── router.go            ルーティング決定（明示コマンド・辞書・分類器）
├── router_test.go       ルーティング単体テスト
├── loop.go              エージェントループ（メッセージ処理・LLM 呼び出し・承認フロー）
├── loop_test.go         ループ統合テスト
├── classifier.go        LLM ベース分類器
├── context.go           コンテキスト構築（システムプロンプト・履歴・FewShot）
├── context_test.go      コンテキスト単体テスト
├── memory.go            メモリ管理（長期記憶・日次ノート・カットオーバー）
└── memory_test.go       メモリ単体テスト
```

### 主要な型・構造体

**ルーティング関連**

※Phase 2 で修正: RouteApprove/RouteDeny の存在を追加

```go
// ルート定数（router.go:12-24）
const (
    RouteChat     = "CHAT"
    RoutePlan     = "PLAN"
    RouteAnalyze  = "ANALYZE"
    RouteOps      = "OPS"
    RouteResearch = "RESEARCH"
    RouteCode     = "CODE"
    RouteCode1    = "CODE1"
    RouteCode2    = "CODE2"
    RouteCode3    = "CODE3"
    RouteApprove  = "APPROVE"  // ※Phase 2 で追加: 承認コマンド用ルート
    RouteDeny     = "DENY"     // ※Phase 2 で追加: 拒否コマンド用ルート
)

type RoutingDecision struct {
    Route                string    // 決定されたルート（CHAT/PLAN/ANALYZE/OPS/RESEARCH/CODE/CODE1/CODE2/CODE3/APPROVE/DENY）
    Source               string    // 決定ソース（command/rules/classifier/fallback/line_forced_chat）
    Confidence           float64   // 信頼度（0.0〜1.0）
    Reason               string    // 理由
    Evidence             []string  // 証拠リスト
    LocalOnly            bool      // ローカル専用モード
    PrevRoute            string    // 前回ルート
    CleanUserText        string    // コマンド除去後のユーザーテキスト
    Declaration          string    // ルート切り替え宣言（例: "コーディングするね。"）
    DirectResponse       string    // 即座に返すべきメッセージ（/local, /cloud 時）
    ErrorReason          string    // エラー理由（classifier_low_confidence 等）
    ClassifierConfidence float64   // 分類器の信頼度
}

type Router struct {
    cfg        config.RoutingConfig
    classifier *Classifier
}

type Classification struct {
    Route      string   // 分類結果（CHAT/PLAN/ANALYZE/OPS/RESEARCH/CODE）
    Confidence float64  // 信頼度
    Reason     string   // 理由
    Evidence   []string // 証拠
}

type Classifier struct {
    provider providers.LLMProvider
    model    string
}
```

**ループ関連**

※Phase 2 で修正: 型定義のコメントを実装に一致させた

```go
type AgentLoop struct {
    bus            *bus.MessageBus
    cfg            *config.Config
    provider       providers.LLMProvider
    providerName   string
    workspace      string
    model          string
    contextWindow  int // ※Phase 2 で修正: Maximum context window size in tokens
    maxIterations  int
    loopMaxLoops   int
    loopMaxMillis  int
    sessions       *session.SessionManager
    state          *state.Manager
    contextBuilder *ContextBuilder
    tools          *tools.ToolRegistry
    router         *Router
    running        atomic.Bool
    summarizing    sync.Map // ※Phase 2 で修正: Tracks which sessions are currently being summarized
    channelManager *channels.Manager
    approvalMgr    *approval.Manager // ※Phase 2 で追加: 承認フロー管理
    mcpClient      *mcp.Client       // ※Phase 2 で追加: MCP クライアント（Chrome DevTools Protocol 統合中）
}

type processOptions struct {
    SessionKey         string   // Session identifier for history/context
    Channel            string   // Target channel for tool execution
    ChatID             string   // Target chat ID for tool execution
    UserMessage        string   // User message content (may include prefix)
    Media              []string
    DefaultResponse    string   // Response when LLM returns empty
    EnableSummary      bool     // Whether to trigger summarization
    SendResponse       bool     // Whether to send response via bus
    Route              string   // Routed category for logging
    LocalOnly          bool     // /local mode for this session
    Declaration        string   // Route declaration prefix
    MaxLoops           int      // Max loop iterations for this turn
    MaxMillis          int      // Max processing time for this turn
    NoHistory          bool     // If true, don't load session history (for heartbeat)
    SkipAddUserMessage bool     // ※Phase 2 で追加: When true, don't add user message (used on Ollama recovery retry)
}
```

**コンテキスト関連**

※Phase 2 で修正: 実装と一致するコメントを追加

```go
type ContextBuilder struct {
    workspace    string
    skillsLoader *skills.SkillsLoader
    memory       *MemoryStore
    tools        *tools.ToolRegistry // ※Phase 2 で追加: 動的ツール要約生成用
    chatAlias    string              // ※Phase 2 で追加: Chat ルート時のエイリアス名（Mio 等）
}
```

**メモリ関連**

※Phase 2 で修正: MemoryStore の役割を明確化

```go
type MemoryStore struct {
    workspace  string
    memoryDir  string  // ※Phase 2 で追加: memory/ ディレクトリのパス
    memoryFile string  // ※Phase 2 で追加: memory/MEMORY.md のパス
}

// ※Phase 2 で追加: カットオーバー定数（ハードコード）
const (
    CutoverHour     = 4            // 04:00
    CutoverTimezone = "Asia/Tokyo" // JST（グローバル展開時は要外部化）
)
```

### ルーティング決定フロー

※Phase 2 で修正: 実装に基づくフロー図を精査

```
ユーザー入力
    ↓
1. 明示コマンド解析（/chat, /code, /code1, /code2, /code3, /approve, /deny 等）
    ├─ マッチ → 即座に決定（confidence=1.0）
    │   ├─ /local → LocalOnly=true、DirectResponse 返却、前回ルート維持
    │   ├─ /cloud → LocalOnly=false、DirectResponse 返却、前回ルート維持
    │   ├─ /approve → RouteApprove（承認コマンド処理へ）
    │   ├─ /deny → RouteDeny（拒否コマンド処理へ）
    │   └─ その他 → ルート決定、stripped にコマンド除去後テキスト
    └─ 不一致 → 次へ
    ↓
2. ルール辞書マッチ（強証拠）
    ├─ hasStrongCodeEvidence() → CODE（コードブロック/diff/拡張子検出）
    ├─ systemctl/journalctl/docker/ssh/kubectl → OPS
    ├─ 集計/傾向/統計/analyze/csv/json → ANALYZE
    ├─ https:// | 出典/最新/比較/research → RESEARCH
    ├─ 仕様/設計/構成/段取り/plan/architecture → PLAN
    └─ 不一致 → 次へ
    ↓
3. LLM 分類器（Classifier）
    ├─ confidence >= MinConfidence（デフォルト 0.6）
    │   ├─ CODE ルート判定 → confidence >= MinConfidenceForCode（0.8）かつ hasStrongCodeEvidence() 必須
    │   │   ├─ 強証拠なし → フォールバック（ErrorReason="classifier_code_without_strong_evidence"）
    │   │   └─ 条件満たす → 採用
    │   └─ その他ルート → 採用
    └─ confidence < MinConfidence → フォールバック（ErrorReason="classifier_low_confidence"）
    ↓
4. フォールバック（CHAT）
    ├─ FallbackRoute 設定値を使用（デフォルト: CHAT）
    └─ 不正なルートの場合は強制 CHAT
```

**特殊処理**

※Phase 2 で修正: 実装コードに基づき特殊処理を詳細化

- **LINE チャネル強制 CHAT**: LINE からの入力はルーティング決定後に強制 CHAT に変更（`line_forced_chat`）
  - 実装箇所: loop.go:374-383
  - Source="line_forced_chat", Confidence=1.0, Reason="line channel is chat-only"
  - 委譲は Mio の応答プロトコル（`DELEGATE: PLAN|ANALYZE|...`）でのみ実行
- **/local モード**: `LocalOnly=true` の場合、CODE 系ルートへの遷移を拒否
  - 拒否メッセージ: "いまは /local モード中だからCODE実行はできないよ。/cloud で解除してから試してね。"
  - DirectResponse を返して前回ルート維持
- **CODE ルート高閾値**: CODE/CODE1/CODE2/CODE3 の分類器判定には二重制約
  1. `confidence >= MinConfidenceForCode`（デフォルト 0.8）
  2. `hasStrongCodeEvidence()` が true（コードブロック、diff、拡張子等）
  - どちらか欠ける場合は CHAT にフォールバック
- **/approve, /deny コマンド**: 承認フロー専用ルート（RouteApprove/RouteDeny）
  - handleCommand() で処理され、approvalMgr.Approve()/Deny() を呼び出す

### エージェントループフロー

```
InboundMessage 受信（bus）
    ↓
processMessage()
    ├─ システムメッセージ → processSystemMessage()（/approve, /deny 処理）
    ├─ 日次カットオーバーチェック
    ├─ コマンド処理（handleCommand()）
    └─ 通常メッセージ → ルーティング決定
    ↓
Router.Decide() → RoutingDecision
    ├─ LINE チャネル → 強制 CHAT
    └─ 委譲通知（CHAT 以外のルートへの遷移時）
    ↓
applyRouteLLM() → LLM プロバイダー切り替え
    ├─ CHAT → Ollama chat-v1
    ├─ CODE1 → DeepSeek
    ├─ CODE2 → OpenAI
    ├─ CODE3 → Claude API
    └─ その他 → Ollama worker-v1
    ↓
runAgentLoop() → LLM 反復呼び出しループ
    ├─ BuildMessages() → システムプロンプト + 履歴構築
    ├─ runLLMIteration() → LLM 呼び出しとツール実行
    │   ├─ 停止条件: max_loops / max_millis / 承認待ち
    │   └─ ツール実行結果を履歴に追加
    ├─ Ollama ヘルスチェック + 自動再起動（Ollama ルート時）
    └─ CODE3 出力解析 → 承認要求生成
    ↓
承認フロー（CODE3 時）
    ├─ CoderOutput 解析（plan/patch/risk）
    ├─ ジョブ作成（approvalMgr.CreateJob()）
    ├─ 承認要求メッセージ返却
    └─ ユーザー承認待ち（/approve or /deny）
    ↓
応答送信（bus.PublishOutbound()）
    ├─ セッション保存
    ├─ 要約処理（maybeSummarize()）
    └─ ログ記録
```

**停止条件**

- `iteration >= maxIterations` (デフォルト: 3)
- `elapsed >= maxMillis` (デフォルト: 90000ms)
- LLM エラー（リトライ後も失敗）
- 承認待ち（`NeedApproval=true`）
- ツール実行エラー（回復不能）

**日次カットオーバー処理**

※Phase 2 で修正: 実装に基づき詳細化

```
maybeDailyCutover(sessionKey)
    ├─ sessions.GetUpdatedTime(sessionKey) → 最終更新時刻取得
    │   └─ IsZero() → スキップ（まだセッションなし）
    ├─ GetCutoverBoundary(now) → 直近の 04:00 JST を計算
    │   ├─ now < 今日の04:00 → 昨日の04:00を返す
    │   └─ now >= 今日の04:00 → 今日の04:00を返す
    ├─ updated.Before(boundary) → カットオーバー実施
    │   ├─ GetHistory(), GetSummary() → セッション内容取得
    │   ├─ 履歴・要約が空 → ResetSession() して Save() だけ実行
    │   ├─ 履歴から user/assistant メッセージを抽出（最大200文字要約）
    │   ├─ FormatCutoverNote(summary, recentLines) → ノート生成
    │   ├─ GetLogicalDate(updated) → 論理日付計算（04:00前は前日扱い）
    │   ├─ memory.SaveDailyNoteForDate(noteDate, note) → 保存
    │   └─ ResetSession(sessionKey) + Save(sessionKey)
    │       ※Phase 2 で追加: ResetSession は履歴・要約をクリアするが Flags は保持
    └─ updated >= boundary → スキップ（まだカットオーバー不要）
```

---

## 落とし穴・注意点

### 設計上の制約

1. **ルーティング決定は1入力1回のみ**
   - 再ルーティング機能は仕様上存在するが、実装では `max_reroute=1` に制限
   - 無限ループ防止のため、同一入力で複数回のルート変更は不可

2. **CODE ルートの高閾値と強証拠必須**
   - CODE/CODE1/CODE2/CODE3 への分類器ルーティングには `MinConfidenceForCode=0.8` が適用
   - かつ `hasStrongCodeEvidence()` による強証拠（コードブロック、拡張子等）が必須
   - 理由: 誤判定による不要なクラウド API 呼び出しを防止

3. **LINE チャネルは強制 CHAT**
   - LINE からの入力は分類器・ルール辞書に関わらず CHAT に強制変更
   - 理由: LINE は会話専用チャネルとしてプロダクト制約を設定

4. **Ollama ルートの自動再起動**
   - Ollama を使うルート（CHAT, PLAN, ANALYZE, OPS, RESEARCH, CODE）では、LLM 呼び出し後にヘルスチェックを実施
   - NG の場合、`OllamaRestartCommand` を実行して自動再起動
   - 再起動後は 10 秒待機してリトライ（`SkipAddUserMessage=true` で重複回避）

5. **セッション要約は CHAT ルートでは無効化**
   - CHAT ルートは会話の自然な連続性を優先するため、`EnableSummary=false`
   - 他のルート（Worker/Coder）は積極的に要約を実施してトークン節約

### 既知の問題・リスク

1. **分類器のコールド起動遅延**
   - 分類器は初回呼び出し時に LLM プロバイダーの初期化が発生
   - Ollama の場合、モデルロードに数秒〜数十秒かかる可能性
   - 緩和策: 分類器は明示コマンド・ルール辞書の後の最終手段として配置

2. **承認フロー実装の未完成部分**
   - ※Phase 2 で確認: 承認後の Worker 実行委譲ロジックは未実装
   - 実装箇所: loop.go:1931 に明示されたメッセージ "Worker による差分適用は次のフェーズで実装予定"
   - 現状の動作: /approve コマンドで承認すると、ジョブは Approved 状態になるが実行はされない
   - 次の実装ステップ: 承認完了後に Worker へ差分適用タスクを委譲する機能追加
   - Chrome 操作検出（`usesBrowser` フラグ設定）も未実装
     - loop.go:501 のコメント: "TODO: Patch から Chrome 操作を検出して usesBrowser を設定"

3. **Coder3 出力解析のエラーハンドリング**
   - ※Phase 2 で確認: parseCoder3Output() のエラーハンドリングは実装済み
   - 実装箇所: loop.go:1963-1979（parseCoder3Output 関数）
   - パースエラー時の処理:
     - JSON Unmarshal 失敗 → エラーメッセージを返却
     - 必須フィールド（job_id, plan）欠損 → エラーメッセージを返却
   - 呼び出し側での処理: loop.go:491-496
     - parseErr が発生した場合、WarnCF でログ出力
     - ユーザーに "Coder3 の出力解析に失敗しました" メッセージを返す
   - ※Phase 2 で修正: 「脆弱性」ではなく、適切なエラーハンドリングが実装済み

4. **日次カットオーバーのタイムゾーン依存**
   - ※Phase 2 で確認: カットオーバー境界は `Asia/Tokyo` 固定（ハードコード）
   - 定義箇所: memory.go:18（CutoverTimezone = "Asia/Tokyo"）
   - 使用箇所: memory.go:22-27（cutoverLocation 関数）
   - グローバル展開時はタイムゾーン設定の外部化が必要
   - フォールバック: time.LoadLocation エラー時は UTC を使用

5. **FewShot サンプルローテーションの偏り**
   - ※Phase 2 で確認: `session_id`（実際は chatID）ベースのハッシュでローテーション
   - 実装箇所: context.go:286-301（LoadFewShotExamplesWithSeed）
   - 同一セッションでは常に同じサンプルが選ばれる（再現性重視の意図的設計）
   - カテゴリ分類: "work", "casual", "ng" の3カテゴリから各1件選出
   - 多様性を求める場合は日付や乱数要素の追加が必要
   - ※Phase 2 で追加: categorizeFewShot() でファイル先頭行から自動分類（context.go:228-241）

### 設計書との乖離（Phase 2 で追加）

#### 実装済み・仕様適合部分

1. **ルーティング優先順位**: 実装仕様 2.1 節に完全準拠
   - 明示コマンド > ルール辞書 > 分類器 > フォールバック（router.go:77-160）
2. **LINE チャネル強制 CHAT**: 実装仕様 2.6 節に準拠
   - loop.go:374-383 で `line_forced_chat` として実装
3. **Coder 三重ルーティング**: 実装仕様 1.1 節に準拠
   - CODE1/CODE2/CODE3 の3段階分岐（loop.go:865-879, selectCoderRoute）
4. **承認フロー基本**: 実装仕様 6章に部分準拠
   - ジョブ作成、承認要求メッセージ生成は完成（loop.go:490-523）
   - `/approve`, `/deny` コマンド処理は完成（loop.go:1910-1947）
5. **日次カットオーバー**: 実装仕様 9章に準拠
   - 04:00 JST 境界計算、論理日付管理、日次ノート保存（loop.go:1481-1535, memory.go:145-168）

#### 未実装・将来実装予定部分

1. **承認後の Worker 実行委譲**: 実装仕様 6.2 節（4番目のステップ）
   - 仕様: "4. 承認後、Worker が適用実行"
   - 実装状況: 未実装（Phase 4 予定）
   - 現状: 承認完了メッセージのみ返却（loop.go:1931）
2. **Auto-Approve モード**: 実装仕様 6.4 節
   - 仕様: Scope/TTL 付き自動承認、即時 OFF 可能
   - 実装状況: approval.Manager に一部機能あり、統合未完
3. **再ルーティング（最大1回）**: 実装仕様 3.3 節
   - 仕様: `fit=false` かつ `suggested_route` 有効時に再ルート
   - 実装状況: 未実装（`max_reroute=1` の制約のみコメントに記載）
4. **会話LLM提案IF**: 実装仕様 3.4 節
   - 仕様: `{"propose_next_loop": false, "route": "...", ...}` による再ルート提案
   - 実装状況: 未実装
5. **Chrome 操作検出**: Coder3 仕様（実装仕様 5.1 節）
   - 仕様: Patch から Chrome 操作を検出して `usesBrowser` フラグ設定
   - 実装状況: TODO コメントのみ（loop.go:501）

#### 仕様との不整合部分

1. **分類器のシステムプロンプト**: 実装仕様 2.5 節
   - 仕様: "分類器 system prompt は1枚固定、カテゴリ追加時のみ更新"
   - 実装: classifier.go:35-36 でハードコード
   - 不整合: CODE1/CODE2/CODE3 は分類器の出力対象外（仕様では CODE のみ列挙）
   - 実装では CODE 判定後に `selectCoderRoute()` でサブ分類
2. **WorkerInput/WorkerOutput スキーマ**: 実装仕様 4.3, 4.4 節
   - 仕様: 統一 JSON 契約（`needs_next_loop`, `fit`, `suggested_route` 等）
   - 実装: Worker ツールは providers.ToolResult を返却（JSON 契約なし）
   - 不整合: 現状は LLM が直接ツールを呼び出す設計で、Worker 専用出力契約は未適用

### 変更時の注意事項

1. **ルート定数の追加時**
   - `router.go` の `const` セクションに新ルートを追加（例: RouteCode3 = "CODE3"）
   - `isAllowedRoute()` 関数を更新（router.go:59-66）
   - `IsCodeRoute()` 関数を更新（router.go:68-75）※CODE 系ルートの場合
   - `declarationFor()` で宣言メッセージを定義（router.go:162-186）
   - `resolveRouteLLM()` でプロバイダーマッピングを追加（loop.go:816-893）
   - ※Phase 2 で確認: 分類器のシステムプロンプトは CODE1/CODE2/CODE3 を出力しない
     - classifier.go:35-36 で許可ルートは "CHAT, PLAN, ANALYZE, OPS, RESEARCH, CODE" のみ
     - CODE サブ分類は `selectCoderRoute()` で実施（loop.go:895-928）

2. **ルーティング優先順位の変更時**
   - `Router.Decide()` の順序変更は慎重に
   - 明示コマンド > ルール辞書 > 分類器 > フォールバック の順序は仕様で固定
   - 変更する場合は実装仕様（`docs/01_正本仕様/実装仕様.md`）を先に更新

3. **セッション構造の変更時**
   - `session.SessionManager` の API 変更時は `loop.go` の多数の箇所に影響
   - 特に履歴取得（`GetHistory()`）、フラグ管理（`GetFlags()`, `SetFlags()`）の変更は要注意
   - テストカバレッジを確認（`loop_test.go` で主要シナリオをカバー）

4. **承認フロー拡張時**
   - ※Phase 2 で確認: `approvalMgr` の呼び出し箇所は loop.go の 3 箇所
     - ジョブ作成: loop.go:502（CreateJob）
     - 承認: loop.go:1917（Approve）
     - 拒否: loop.go:1940（Deny）
   - Auto-Approve 機能追加時は `approval.Manager` の API 拡張が必要
     - 現状: 基本機能（CreateJob/GetJob/Approve/Deny）のみ実装
     - 追加予定: Scope/TTL 管理、即時 OFF 機能
   - ログイベント種別（`approval.requested`, `approval.granted` 等）は実装済み
     - logger.LogApprovalRequested（loop.go:515）
     - logger.LogApprovalGranted（loop.go:1922）
     - logger.LogApprovalDenied（loop.go:1945）
   - 承認後の Worker 実行委譲は未実装（Phase 4 予定）
     - 実装時は loop.go:1931 付近に委譲ロジックを追加

5. **LLM プロバイダー追加時**
   - ※Phase 2 で確認: 以下の箇所を更新
   - `resolveRouteLLM()` / `resolveRouteLLMWithTask()` に新プロバイダーのマッピングを追加
     - 実装箇所: loop.go:816-893（switch route による分岐）
   - `routeUsesOllama()` でヘルスチェック対象を判定（loop.go:930-933）
   - `buildOllamaRequiredModels()` で必要モデルを定義（loop.go:935-957）
     - Ollama の場合のみ、ModelRequirement リストに追加
     - MaxContext=8192 の制約チェックを含む
   - `config/config.go` の `Routing.LLM` セクションにも設定追加が必要
     - 例: Coder3 追加時は `Coder3Provider`, `Coder3Model`, `Coder3Alias` を追加
   - `resolveRouteRoleAlias()` でエイリアス名を定義（loop.go:959-996）

6. **テスト追加時**
   - ルーティング単体テスト: `router_test.go`（命名規則、証拠検出、閾値テスト）
   - ループ統合テスト: `loop_test.go`（エンドツーエンドシナリオ、エラーハンドリング）
   - コンテキスト単体テスト: `context_test.go`（プロンプト構築、FewShot ローテーション）
   - メモリ単体テスト: `memory_test.go`（カットオーバー境界、日次ノート）

---

## 補足: 主要ログイベント

※Phase 2 で修正: 実装コードに基づき正確なイベント種別と出力箇所を記載

エージェントモジュールが出力する主要ログイベント（logger.InfoCF/DebugCF/WarnCF/ErrorCF）:

- **mvp.routing** - ルーティング決定の詳細（loop.go:384-391）
  - フィールド: session_key, initial_route, source, classifier_confidence, error_reason
- **mvp.route.final** - 最終ルート決定（loop.go:582-588）
  - フィールド: session_key, final_route, classifier_confidence, error_reason
- **mvp.stop** - ループ停止（loop.go:571-577）
  - フィールド: session_key, final_route, stop_reason, error_reason
- **classifier.error** - 分類器エラー（エラー詳細）※推測: 分類器内部
- **approval.requested** - 承認要求（loop.go:515 経由で logger.LogApprovalRequested）
  - フィールド: job_id, plan, patch, risk
- **approval.granted** - 承認許可（loop.go:1922 経由で logger.LogApprovalGranted）
  - フィールド: job_id, approver
- **approval.denied** - 承認拒否（loop.go:1945 経由で logger.LogApprovalDenied）
  - フィールド: job_id, denier
- **coder.plan_generated** - Coder プラン生成（loop.go:516 経由で logger.LogCoderPlanGenerated）
  - フィールド: job_id, plan
- **approval.create_job_error** - 承認ジョブ作成エラー（loop.go:504-506）
  - フィールド: error
- **coder3.parse_error** - Coder3 出力パースエラー（loop.go:493-495）※Phase 2 で追加
  - フィールド: error
- **ollama health check** - Ollama ヘルスチェック結果（loop.go:470-475）
  - フィールド: ollama_ok, ollama_msg, models_ok, models_msg, llm_err
- **ollama restart failed** - Ollama 再起動失敗（loop.go:480）※WarnCF
  - フィールド: error
- **LLM iteration** - LLM 反復呼び出し（loop.go:1177-1181）※DebugCF
  - フィールド: iteration, max
- **LLM request** - LLM リクエスト詳細（loop.go:1190-1199）※DebugCF
  - フィールド: iteration, model, messages_count, tools_count, max_tokens, temperature, system_prompt_len
- **Full LLM request** - 完全なメッセージ・ツール情報（loop.go:1202-1207）※DebugCF
  - フィールド: iteration, messages_json, tools_json
- **LLM response without tool calls** - ツール呼び出しなし応答（loop.go:1356-1360）
  - フィールド: iteration, content_chars
- **LLM requested tool calls** - ツール呼び出し要求（loop.go:1369-1374）
  - フィールド: tools, count, iteration
- **Tool call** - 個別ツール呼び出し（loop.go:1402-1406）
  - フィールド: tool, iteration
- **Sent tool result to user** - ツール結果ユーザー送信（loop.go:1433-1437）※DebugCF
  - フィールド: tool, content_len
- **Context window error detected** - コンテキストウィンドウ超過（loop.go:1232-1235）※WarnCF
  - フィールド: error, retry
- **LLM call failed** - LLM 呼び出し失敗（loop.go:1343-1349）※ErrorCF
  - フィールド: iteration, error, is_timeout, note
- **Response** - 最終応答（loop.go:1153-1159）
  - フィールド: session_key, iterations, final_length
- **Daily cutover: session archived to daily note** - 日次カットオーバー成功（loop.go:1525-1529）
  - フィールド: session_key, note_date, messages
- **Failed to save daily cutover note** - 日次カットオーバー失敗（loop.go:1519-1523）※WarnCF
  - フィールド: session_key, note_date, error
- **Forced compression executed** - 強制圧縮実行（loop.go:1614-1618）※WarnCF
  - フィールド: session_key, dropped_msgs, new_count

---

## 参考: ファイルサイズと複雑度

- `loop.go`: 1900+ 行（エージェントループ、承認フロー、日次カットオーバー、Ollama 監視、MCP 統合等を含む最大ファイル）
- `router.go`: 278 行（ルーティング決定ロジック）
- `context.go`: 646 行（コンテキスト構築、FewShot 管理、添付ファイル処理）
- `memory.go`: 244 行（メモリ管理、日次ノート、カットオーバー計算）
- `classifier.go`: 90 行（LLM ベース分類器）

テストファイルも充実しており、重要な分岐パターンをカバーしている。

---

## Phase 2 検証記録（2026-02-28）

### 検証手順

1. **Phase 1 ドキュメント読み込み**: agent.md を読み込み、記載内容を把握
2. **実コード突合せ**:
   - router.go（278行）全文読み込み
   - loop.go（1980行）を分割読み込み（4分割）
   - context.go（646行）全文読み込み
   - classifier.go（90行）全文読み込み
   - memory.go（244行）全文読み込み
3. **設計仕様書確認**:
   - `docs/01_正本仕様/実装仕様.md`（2章、6章、7章）
   - `docs/02_v2統合分割仕様/実装仕様_v2_02_ルーティング.md`
4. **Grep による詳細確認**:
   - 承認フロー関連（RouteApprove, RouteDeny, parseCoder3Output, CreateJob）
   - 日次カットオーバー関連（GetCutoverBoundary, GetLogicalDate, maybeDailyCutover）

### 主要な発見・修正

#### 正確性向上

1. **ルート定数の追加**: RouteApprove/RouteDeny（承認コマンド用）を追加
2. **Coder3Output 構造体**: parseCoder3Output の実装を確認し、エラーハンドリングが適切に実装済みであることを確認
3. **日次カットオーバーの詳細フロー**: GetCutoverBoundary/GetLogicalDate の計算ロジックを精査
4. **承認フロー統合の現状**: Phase 4 まで Worker 実行委譲が未実装であることを明確化

#### 設計書との乖離検出

1. **承認後の Worker 実行委譲**: 実装仕様 6.2 節（4番目のステップ）が未実装
2. **Auto-Approve モード**: 実装仕様 6.4 節が未実装
3. **再ルーティング（最大1回）**: 実装仕様 3.3 節が未実装
4. **会話LLM提案IF**: 実装仕様 3.4 節が未実装
5. **分類器のシステムプロンプト**: CODE1/CODE2/CODE3 は分類器の出力対象外（仕様と実装の設計判断の違い）

#### 落とし穴・注意点の追加

1. **Coder3 出力解析のエラーハンドリング**: 「脆弱性」ではなく適切に実装済みであることを確認
2. **SkipAddUserMessage フラグ**: Ollama 再起動後のリトライ時に重複回避する仕組みを追加
3. **FewShot サンプルのカテゴリ分類**: categorizeFewShot() の実装詳細を追加
4. **ResetSession の挙動**: Flags は保持することを明記

### Phase 2 タグ付き箇所

- 型・構造体: RouteApprove/RouteDeny 追加、コメント精査
- ルーティング決定フロー: /approve, /deny 処理の詳細化
- 特殊処理: LINE 強制 CHAT の実装箇所明記
- 日次カットオーバー処理: GetLogicalDate の挙動詳細化
- 承認フロー統合: 未実装部分の明確化、TODO コメント参照
- 既知の問題: エラーハンドリング実装済みを確認
- 設計書との乖離: 5項目の未実装機能を明記
- 変更時の注意事項: 実装箇所の行番号と関数名を追記
- 主要ログイベント: 45種類のログイベントを網羅、出力箇所明記

---

**最終更新**: 2026-02-28（Phase 2 検証完了）
**バージョン**: 2.0
**解析対象コミット**: 1c61431 (feat: TDD サイクルと LINT チェックを開発フローに必須化)
