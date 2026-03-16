# ProcessHeartbeat 設計問題の調査

**調査日時**: 2026-02-28 14:07:24 JST
**調査者**: Claude Sonnet 4.5
**重要度**: 🚨 高（即座に対応が必要）

---

## 📋 問題の概要

**ProcessHeartbeat が Chat ルート固定になっており、Worker/Coder LLM の健全性確認ができない設計上の問題。**

ユーザーが介入しない長時間タスク（OPS/RESEARCH/PLAN/ANALYZE/CODE等）の実行中に、担当LLMの異常を検出できず、タスクが無限に停止したままになる可能性がある。

---

## 🔍 発見の経緯

### 質問の流れ

1. **ユーザー質問1**: 「どのシステムもHeartbeatを持っていますか？」
   - Chat, Worker, Coder1, Coder2, Coder3 の5つのLLMシステムそれぞれにHeartbeat機能があるか確認要求

2. **調査結果**:
   - Ollama監視: Chat/Worker (Ollamaプロバイダー使用時のみ)
   - ProcessHeartbeat: Chat固定

3. **ユーザー質問2**: 「ProcessHeartbeat が本システムの１つの特徴だと思うのですが、Chatだけ対応していて、ユーザーが介入しない作業を続けられますか？」
   - **重要な設計上の問題点を指摘**

---

## 🔴 問題の詳細

### 現在の実装

**ProcessHeartbeat** (`pkg/agent/loop.go:323-337`):
```go
func (al *AgentLoop) ProcessHeartbeat(ctx context.Context, content, channel, chatID string) (string, error) {
    return al.runAgentLoop(ctx, processOptions{
        SessionKey:      "heartbeat",
        Channel:         channel,
        ChatID:          chatID,
        UserMessage:     content,
        DefaultResponse: "I've completed processing but have no response to give.",
        EnableSummary:   false,
        SendResponse:    false,
        NoHistory:       true, // Don't load session history for heartbeat
        Route:           RouteChat,  // ← Chat固定！問題の原因
        MaxLoops:        al.maxIterations,
        MaxMillis:       al.loopMaxMillis,
    })
}
```

**LLM解決ロジック** (`pkg/agent/loop.go:868-896`):
```go
func (al *AgentLoop) resolveRouteLLMWithTask(route, taskText string) (string, string) {
    // ...
    switch strings.ToUpper(strings.TrimSpace(route)) {
    case RouteCode1:
        return resolveCoder1()
    case RouteCode2:
        return resolveCoder2()
    case RouteCode3:
        return resolveCoder3()
    case RouteCode:
        selected := selectCoderRoute(taskText)
        // ...
        return resolveCoder2()
    case RouteChat:
        return chooseProvider(..., llmCfg.ChatProvider), chooseModel(..., llmCfg.ChatModel)
    default:  // ← OPS, RESEARCH, PLAN, ANALYZE はここ
        workerProvider := chooseProvider(defaultProvider, llmCfg.WorkerProvider)
        workerModel := chooseModel(defaultModel, llmCfg.WorkerModel)
        if strings.TrimSpace(llmCfg.WorkerProvider) == "" {
            workerProvider = chooseProvider(workerProvider, llmCfg.ChatProvider)
        }
        if strings.TrimSpace(llmCfg.WorkerModel) == "" {
            workerModel = chooseModel(workerModel, llmCfg.ChatModel)
        }
        return workerProvider, workerModel  // ← Worker LLM
    }
}
```

---

## 📊 影響範囲

### 各ルートとLLMの対応

| ルート | 担当LLM | Heartbeat確認 | 長時間タスク | 影響度 |
|--------|---------|--------------|-------------|--------|
| **CHAT** | Chat (Mio) | ✅ 対応 | - | - |
| **OPS** | Worker (Shiro) | ❌ **未対応** | ✅ あり | 🚨 高 |
| **RESEARCH** | Worker (Shiro) | ❌ **未対応** | ✅ あり | 🚨 高 |
| **PLAN** | Worker (Shiro) | ❌ **未対応** | ✅ あり | 🚨 高 |
| **ANALYZE** | Worker (Shiro) | ❌ **未対応** | ✅ あり | 🚨 高 |
| **CODE1** | Coder1 (Aka) | ❌ **未対応** | ✅ あり | ⚠️ 中 |
| **CODE2** | Coder2 (Ao) | ❌ **未対応** | ✅ あり | ⚠️ 中 |
| **CODE3** | Coder3 (Claude) | ❌ **未対応** | ✅ あり | ⚠️ 中 |

### 問題のシナリオ

```
【シナリオ】長時間のRESEARCH タスク実行中

1. ユーザーが /research コマンド実行
   ↓
2. Worker LLM (Shiro/ollama/worker-v1) が調査開始
   ↓
3. 30分間実行中...
   ↓
4. Worker LLM がクラッシュ/フリーズ/無限ループ
   ↓
5. 定期Heartbeat実行 (60分間隔)
   ↓
6. ProcessHeartbeat は Chat LLM のみ確認
   ↓
7. ✗ Worker の異常を検出できない
   ↓
8. ✗ タスクが無限に停止したまま
   ↓
9. ✗ ユーザーは異常に気づかない
```

---

## ⚠️ 具体的な問題点

### 1. Worker LLMがHeartbeat確認されない

**Worker LLM** は以下のルートを担当：
- **OPS**（運用操作）- システム管理、デプロイ、設定変更等
- **RESEARCH**（調査）- 情報収集、文献調査、データ分析等
- **PLAN**（計画策定）- プロジェクト計画、タスク分解等
- **ANALYZE**（分析）- ログ分析、パフォーマンス分析等

これらは**ユーザー介入なしで長時間実行される可能性が高い**タスクですが、**Heartbeatで健全性確認できません**。

### 2. Coder LLMもHeartbeat確認されない

**Coder1/2/3** も以下の長時間タスクを実行：
- **CODE1**: 設計・仕様策定（時間がかかる）
- **CODE2**: 実装（複雑なコードは長時間）
- **CODE3**: 高品質コーディング（Claude APIによる推論は時間がかかる）

これらもHeartbeat確認がありません。

### 3. Ollama監視との違い

| 項目 | Ollama監視 | ProcessHeartbeat |
|------|-----------|-----------------|
| **対象** | Ollamaプロバイダー使用時のみ | 全LLM |
| **タイミング** | LLM呼び出し前（即座） | 定期的（設定間隔） |
| **確認方法** | サーバー/モデルチェック | 軽いタスク送信 |
| **カバー範囲** | Chat/Worker (Ollama使用時) | **Chatのみ** ❌ |
| **検出内容** | サーバー死活、モデルロード状態 | LLM応答能力 |

**Ollama監視の限界**:
- Ollamaプロバイダー以外（DeepSeek, OpenAI, Anthropic）は監視対象外
- LLM呼び出し前のチェックのため、実行中の異常は検出できない

---

## 🔍 根本原因の分析

### 設計上の見落とし

1. **ProcessHeartbeat の設計時**:
   - Heartbeat = Chat との対話を想定
   - 他のLLMルートの存在を考慮していなかった

2. **ルーティング設計時**:
   - Worker/Coder ルートの追加
   - Heartbeat との連携を考慮しなかった

3. **CLAUDE.md の記載不整合**:
   - L95-101: 責務の分離で「Chat/Worker/Coder」の3役割を定義
   - ProcessHeartbeat の動作仕様が明記されていない

---

## 💡 改善提案

### オプション1: 全ルートのHeartbeat対応（最も堅牢）

**実装方針**: 全ルートをHeartbeat確認できるようにする

**修正箇所**:
- `pkg/agent/loop.go:323-337` - ProcessHeartbeat
- `cmd/picoclaw/agent.go` - Heartbeatサービス

**修正内容**:
```go
// 現在
func (al *AgentLoop) ProcessHeartbeat(ctx context.Context, content, channel, chatID string) (string, error) {
    return al.runAgentLoop(ctx, processOptions{
        Route: RouteChat,  // ← Chat固定
        // ...
    })
}

// 改善案
func (al *AgentLoop) ProcessHeartbeat(ctx context.Context, route, content, channel, chatID string) (string, error) {
    if route == "" {
        route = RouteChat // デフォルト
    }
    return al.runAgentLoop(ctx, processOptions{
        Route: route,  // ← 可変に
        // ...
    })
}
```

**設定拡張** (`pkg/config/config.go`):
```go
type HeartbeatConfig struct {
    Enabled  bool     `json:"enabled"`
    Interval int      `json:"interval"`
    Routes   []string `json:"routes"`  // ← 追加
}
```

**設定例** (`config.example.json`):
```json
"heartbeat": {
  "enabled": true,
  "interval": 60,
  "routes": ["CHAT", "OPS", "RESEARCH", "CODE1", "CODE2", "CODE3"]
}
```

**メリット**:
- 全LLMの健全性を確認できる
- 柔軟な設定が可能

**デメリット**:
- 実装が複雑
- Heartbeat実行時間が増加（ルート数分）

---

### オプション2: ラウンドロビン方式（バランス型）

**実装方針**: 定期Heartbeatで複数ルートを順番に確認

**実装内容**:
```go
type HeartbeatService struct {
    // ...
    routes      []string
    routeIndex  int
}

func (h *HeartbeatService) nextRoute() string {
    route := h.routes[h.routeIndex]
    h.routeIndex = (h.routeIndex + 1) % len(h.routes)
    return route
}

func (h *HeartbeatService) tick() {
    route := h.nextRoute()
    response, err := h.agentLoop.ProcessHeartbeat(ctx, route, prompt, channel, chatID)
    // ...
}
```

**設定例**:
```json
"heartbeat": {
  "enabled": true,
  "interval": 60,
  "round_robin": true,
  "routes": ["CHAT", "OPS", "CODE1", "CODE2", "CODE3"]
}
```

**動作例** (interval: 60分):
```
t=0:   CHAT確認
t=60:  OPS確認
t=120: CODE1確認
t=180: CODE2確認
t=240: CODE3確認
t=300: CHAT確認（ループ）
```

**メリット**:
- 全LLMを確認できる
- 1回の実行時間は短い（1ルートのみ）
- 検出の遅延は許容範囲（最大5時間）

**デメリット**:
- 異常検出までの時間が長い可能性

---

### オプション3: Chat + Worker のみ（最小限）

**実装方針**: 主要な長時間タスクをカバー

**実装内容**:
```go
type HeartbeatConfig struct {
    Enabled      bool `json:"enabled"`
    Interval     int  `json:"interval"`
    CheckChat    bool `json:"check_chat"`    // デフォルト: true
    CheckWorker  bool `json:"check_worker"`  // デフォルト: true
    CheckCoder   bool `json:"check_coder"`   // デフォルト: false
}
```

**設定例**:
```json
"heartbeat": {
  "enabled": true,
  "interval": 60,
  "check_chat": true,
  "check_worker": true,
  "check_coder": false
}
```

**メリット**:
- 実装が簡単
- 主要な長時間タスク（OPS/RESEARCH）をカバー
- Coderは短期タスクが多いため後回し可

**デメリット**:
- Coder LLMは未確認

---

## 🎯 推奨アクション

### 優先度評価

| オプション | 実装難易度 | カバー範囲 | 即効性 | 推奨度 |
|-----------|----------|----------|--------|--------|
| **オプション1** | 高 | 全LLM | 中 | ⭐⭐⭐ |
| **オプション2** | 中 | 全LLM | 低 | ⭐⭐⭐⭐ |
| **オプション3** | 低 | Chat/Worker | 高 | ⭐⭐⭐⭐⭐ |

### 推奨: **オプション3（Chat + Worker）→ オプション2（段階的拡張）**

**理由**:
1. **即座に対応すべき問題**: Worker LLMが長時間タスク実行中に異常検出できない
2. **最小限の修正**: オプション3で迅速に主要問題を解決
3. **段階的拡張**: 将来的にオプション2でCoder対応

**実装ステップ**:

**Phase 1**: Chat + Worker Heartbeat（即座に実装）
```
1. HeartbeatConfig 拡張
2. ProcessHeartbeat にルートパラメータ追加
3. HeartbeatService でChat/Workerを順番に確認
4. テスト・検証
```

**Phase 2**: 全ルートのラウンドロビン（将来拡張）
```
1. routes配列を設定から読み込み
2. ラウンドロビンロジック実装
3. テスト・検証
```

---

## 📝 実装タスク（Phase 1）

### Task 1: HeartbeatConfig 拡張

**ファイル**: `pkg/config/config.go`

```go
type HeartbeatConfig struct {
    Enabled      bool `json:"enabled" env:"PICOCLAW_HEARTBEAT_ENABLED"`
    Interval     int  `json:"interval" env:"PICOCLAW_HEARTBEAT_INTERVAL"` // minutes, min 5
    CheckChat    bool `json:"check_chat" env:"PICOCLAW_HEARTBEAT_CHECK_CHAT"`       // デフォルト: true
    CheckWorker  bool `json:"check_worker" env:"PICOCLAW_HEARTBEAT_CHECK_WORKER"`   // デフォルト: true
}

// DefaultConfig に追加
Heartbeat: HeartbeatConfig{
    Enabled:     false,
    Interval:    60,
    CheckChat:   true,
    CheckWorker: true,
},
```

### Task 2: ProcessHeartbeat 拡張

**ファイル**: `pkg/agent/loop.go`

```go
// 関数シグネチャを変更
func (al *AgentLoop) ProcessHeartbeat(ctx context.Context, route, content, channel, chatID string) (string, error) {
    if route == "" {
        route = RouteChat // デフォルト
    }
    return al.runAgentLoop(ctx, processOptions{
        SessionKey:      "heartbeat",
        Channel:         channel,
        ChatID:          chatID,
        UserMessage:     content,
        DefaultResponse: "I've completed processing but have no response to give.",
        EnableSummary:   false,
        SendResponse:    false,
        NoHistory:       true,
        Route:           route,  // ← 可変に
        MaxLoops:        al.maxIterations,
        MaxMillis:       al.loopMaxMillis,
    })
}
```

### Task 3: HeartbeatService 修正

**ファイル**: `cmd/picoclaw/agent.go` または `pkg/heartbeat/service.go`

```go
type HeartbeatService struct {
    // ...
    checkChat   bool
    checkWorker bool
    routeIndex  int
}

func (h *HeartbeatService) tick() {
    routes := h.getActiveRoutes()
    if len(routes) == 0 {
        return
    }

    route := routes[h.routeIndex]
    h.routeIndex = (h.routeIndex + 1) % len(routes)

    response, err := h.agentLoop.ProcessHeartbeat(ctx, route, prompt, channel, chatID)
    // ...
}

func (h *HeartbeatService) getActiveRoutes() []string {
    var routes []string
    if h.checkChat {
        routes = append(routes, "CHAT")
    }
    if h.checkWorker {
        routes = append(routes, "OPS")  // Workerが担当する代表ルート
    }
    return routes
}
```

### Task 4: テスト追加

**ファイル**: `pkg/agent/loop_test.go`

```go
func TestProcessHeartbeat_MultipleRoutes(t *testing.T) {
    tests := []struct {
        name  string
        route string
        want  string
    }{
        {
            name:  "Chat route",
            route: RouteChat,
            want:  "ok",
        },
        {
            name:  "Ops route (Worker)",
            route: RouteOps,
            want:  "ok",
        },
        {
            name:  "Empty route (default Chat)",
            route: "",
            want:  "ok",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // テストロジック
        })
    }
}
```

---

## 📚 関連ファイル

### 実装ファイル
- `pkg/agent/loop.go:323-337` - ProcessHeartbeat実装
- `pkg/agent/loop.go:868-896` - ルート解決ロジック
- `pkg/config/config.go:164-167` - HeartbeatConfig
- `cmd/picoclaw/agent.go:568-585` - HeartbeatService

### ドキュメント
- `CLAUDE.md:95-125` - 責務の分離とルーティング
- `docs/01_正本仕様/実装仕様.md` - 実装仕様（要更新）

### テスト
- `pkg/agent/loop_test.go` - ユニットテスト追加必要

---

## 🔄 更新すべきドキュメント

### CLAUDE.md

```markdown

```

**追加すべき内容**:
```markdown
#### 3.3.2 Heartbeat機能

- **ProcessHeartbeat**: 定期的なLLM健全性確認
- **対象ルート**: CHAT, OPS（設定により変更可能）
- **確認間隔**: デフォルト60分（最小5分）
- **動作**: セッション履歴なしの軽いタスクを送信して応答を確認
```

---

## ✅ 検証計画

### ユニットテスト
```bash
go test ./pkg/agent/... -run TestProcessHeartbeat
```

### 統合テスト
```bash
# Chat Heartbeat
curl -X POST http://localhost:8080/heartbeat -d '{"route":"CHAT"}'

# Worker Heartbeat
curl -X POST http://localhost:8080/heartbeat -d '{"route":"OPS"}'
```

### E2Eテスト
1. Heartbeat有効化（設定）
2. 長時間OPSタスク実行
3. Heartbeatログ確認
4. Worker異常時のHeartbeat失敗確認

---

## 📅 実装スケジュール（推奨）

| フェーズ | 内容 | 期間 | 優先度 |
|---------|------|------|--------|
| **Phase 1** | Chat + Worker Heartbeat | 1-2日 | 🚨 高 |
| **Phase 2** | ラウンドロビン拡張 | 3-5日 | ⚠️ 中 |
| **Phase 3** | ドキュメント更新 | 1日 | 📝 中 |

---

## 🎯 まとめ

### 問題の本質
ProcessHeartbeat が Chat ルート固定により、Worker/Coder LLM の長時間タスク実行時の健全性を保証できない**重大な設計上の問題**。

### 影響
- ✗ Worker LLM（OPS/RESEARCH/PLAN/ANALYZE）の異常検出不可
- ✗ Coder LLM（CODE1/CODE2/CODE3）の異常検出不可
- ✗ ユーザー介入なしの自動作業の継続性が保証できない

### 推奨アクション
**Phase 1**: Chat + Worker Heartbeat（即座に実装）
**Phase 2**: 全ルートのラウンドロビン（段階的拡張）

### 期待される効果
- ✓ 主要LLM（Chat/Worker）の健全性確認
- ✓ 長時間タスクの安定性向上
- ✓ 無人運用の信頼性向上

---

