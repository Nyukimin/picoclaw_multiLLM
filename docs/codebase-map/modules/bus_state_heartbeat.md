---
run_id: run_20260619_000000
generated_at: 2026-06-19
phase: phase2
module_group: bus,state,heartbeat
---

# pkg/bus, pkg/state, pkg/heartbeat 解析

## pkg/bus — MessageBus

### 構造

```go
// pkg/bus/bus.go
type MessageBus struct {
    mu       sync.RWMutex
    inbound  chan InboundMessage  // バッファ 100
    outbound chan OutboundMessage // バッファ 100
}
```

### メッセージ型

```go
// pkg/bus/types.go
type InboundMessage struct {
    Channel    string
    SenderID   string
    ChatID     string
    Content    string
    Media      []byte
    SessionKey string
    Metadata   map[string]string
}

type OutboundMessage struct {
    Channel  string
    ChatID   string
    Content  string
    Metadata map[string]string
}
```

### 既知バグ: RLock 保持中のチャネル送信

```go
func (b *MessageBus) PublishInbound(msg InboundMessage) error {
    b.mu.RLock()         // RLock 取得
    defer b.mu.RUnlock()
    b.inbound <- msg     // チャネルが満杯(100件)ならここでブロック
    // RLock を保持したままブロック → 他の goroutine が Lock() で待つとデッドロック
    return nil
}
```

チャネルが満杯（100件のバックログ）で AgentLoop の消費が追いつかない場合、
`PublishInbound` は `b.mu.RLock()` を保持したままブロックする。
この状態で別 goroutine が `b.mu.Lock()` を試みると全体がデッドロックする。

**緩和**: バッファ 100 で通常ありえない。AgentLoop が詰まると発現リスクあり。

### ConsumeInbound / SubscribeOutbound

```go
func (b *MessageBus) ConsumeInbound() <-chan InboundMessage  { return b.inbound }
func (b *MessageBus) SubscribeOutbound() <-chan OutboundMessage { return b.outbound }
```

AgentLoop は `ConsumeInbound()` から select ループでメッセージを受信。

---

## pkg/state — 最終チャネル記録

```go
type Manager struct {
    mu      sync.Mutex
    data    map[string]string // chatID -> channel
    path    string
}

func (m *Manager) Set(chatID, channel string) error
func (m *Manager) Get(chatID string) (string, bool)
```

- 原子的永続化: `os.CreateTemp` → write → `Sync` → `Close` → `os.Rename`
- 起動時に JSON ファイルから復元
- LINE チャネルで「最後に使ったチャネル」を追跡するために使用

---

## pkg/heartbeat — HeartbeatService

### 構造

```go
// pkg/heartbeat/service.go
type HeartbeatService struct {
    mu          sync.Mutex
    enabled     bool          // 外部から読み取りあり（ロック外）
    interval    time.Duration // min 5分, default 30分
    stopChan    chan struct{}
    workspacePath string
}
```

### 動作

1. `Start()`: goroutine 起動 → 1秒後に初回 heartbeat（`time.AfterFunc`）
2. `executeHeartbeat()`: `workspacePath/HEARTBEAT.md` を読み込み
   - ファイル未存在時はデフォルトテンプレートを生成
   - 内容を LLM に渡して状態報告（現時点では Ollama 等）
3. `Stop()`: `stopChan` を close してゴルーチンを停止

### 既知バグ: enabled のロック不整合

```go
func (hs *HeartbeatService) executeHeartbeat() {
    if !hs.enabled { // ← mu.RLock() なしで読み取り（競合条件）
        return
    }
    hs.mu.Lock()
    defer hs.mu.Unlock()
    if hs.stopChan == nil { // ← mu.Lock() ありで確認
        return
    }
    // ...実行...
}
```

`hs.enabled` の読み取りがロックなし。`hs.stopChan != nil` の確認はロックあり。
この不整合により、`enabled` の変更が別 goroutine から見えない可能性がある（Go メモリモデル違反）。

### HeartbeatService vs HeartbeatCollectorModule

| | HeartbeatService (`pkg/heartbeat`) | HeartbeatCollectorModule (`pkg/modules/worker`) |
|--|--|--|
| 役割 | 外部への定期報告（Ollama等へ） | 各エージェントの状態を in-memory で収集 |
| 場所 | pkg/ 層（旧アーキ） | modules/ 層（新アーキ） |
| 状態 | 実装済み・動作 | 実装済みだがデータ供給なし |

---

## 関連

- [アーキテクチャ総合.md](../アーキテクチャ総合.md)
- [潜在バグ一覧.md](../潜在バグ一覧.md)
