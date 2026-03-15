# 詳細実装仕様 04: Nodes/デバイス能力の差

**作成日**: 2026-03-09  
**ステータス**: Draft  
**親仕様**: `docs/実装仕様_OpenClaw移植_v1.md`

---

## 0. OpenClaw原典の理解

- Nodes: <https://docs.openclaw.ai/nodes>
- Platforms: <https://docs.openclaw.ai/platforms>

OpenClawは実行ノードを能力単位で扱い、ノード選定をタスク要件から決定する。

---

## 1. 現状差分

- RenCrow現状: Local/SSH Transportはあるが、能力モデルが薄くノード選定が静的。
- 差分の本質: Node Capability Model とスケジューリング戦略不足。

---

## 2. 実装対象

1. `NodeCapability` モデル
2. ノードヘルス + 能力ハートビート
3. タスク要求に応じたノード選定器
4. デバイス能力（audio/browser/gpu）を明示管理

---

## 3. 契約仕様

```go
type NodeCapability struct {
    NodeID       string
    CPUCores     int
    MemoryMB     int
    HasGPU       bool
    HasAudioOut  bool
    HasBrowser   bool
    NetworkClass string // offline|limited|full
    Labels       map[string]string
}
```

```go
type TaskRequirement struct {
    NeedsGPU      bool
    NeedsAudioOut bool
    NeedsBrowser  bool
    MaxLatencyMs  int
}
```

---

## 4. 配置

- `internal/domain/node/capability.go`
- `internal/application/orchestrator/node_selector.go`
- `internal/infrastructure/transport/capability_probe.go`
- `internal/adapter/http/node_admin_handler.go`

---

## 5. TDD計画

1. ノード能力シリアライズテスト
2. 要件未充足ノードの除外テスト
3. 同能力ノード間の負荷分散テスト
4. ノードダウン時フェイルオーバーテスト
5. TTS要件（audio必須）で不適格ノードが選ばれないテスト

受け入れ基準:
- タスク要件に合致しないノード選定が0件
- ノード離脱時の再配置がSLO内で完了

---

## 6. 未決事項

1. GPU能力をベンダー依存で管理するか抽象化するか
2. ブラウザ能力の健全性判定を何秒周期で実施するか
