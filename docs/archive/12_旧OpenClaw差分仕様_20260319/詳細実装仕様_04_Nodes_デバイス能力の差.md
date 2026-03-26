# 詳細実装仕様 04: Nodes/デバイス能力の差

**更新日**: 2026-03-19  
**ステータス**: 現行実装ベース  
**親仕様**: `docs/実装仕様_OpenClaw移植_v1.md`

---

## 1. 概要

RenCrow は OpenClaw のような汎用ノードスケジューラではないが、分散実行と coder 選定のための最小 capability model をすでに持っている。  
現行実装の中心は次の 3 要素である。

1. `Capability` / `TaskRequirement`
2. `NodeSelector`
3. `local | ssh` transport 構成

---

## 2. 能力モデル

### 2.1 Capability

**ファイル**: `internal/domain/node/capability.go`

```go
type Capability struct {
    NodeID       string
    CPUCores     int
    MemoryMB     int
    HasGPU       bool
    HasAudioOut  bool
    HasBrowser   bool
    NetworkClass string
    Labels       map[string]string
}
```

### 2.2 TaskRequirement

```go
type TaskRequirement struct {
    NeedsGPU      bool
    NeedsAudioOut bool
    NeedsBrowser  bool
    MaxLatencyMs  int
}
```

`Matches(cap)` は現行では次だけを見る。

- GPU 要件
- audio out 要件
- browser 要件

CPU / memory / latency は保持するが、選定ロジックでは未使用である。

---

## 3. NodeSelector の現行挙動

**ファイル**: `internal/application/orchestrator/node_selector.go`

`NodeSelector.Select(candidates, caps, req)` の流れ:

1. candidate ごとに capability map を引く
2. `req.Matches(cap)` を満たす node のみ残す
3. 一致候補を sort
4. `coder3` がいれば優先
5. それ以外は先頭を返す

補足:

- candidate が空なら空文字
- capability が未登録なら候補から除外
- 負荷分散や動的スコアリングはない

### 3.1 Requirement 推定

`inferTaskRequirement(msg)` は user message から簡易推定する。

- `tts | audio | voice` -> `NeedsAudioOut=true`
- `browser | chrome | canvas` -> `NeedsBrowser=true`
- `gpu | cuda` -> `NeedsGPU=true`

この推定は `RouteCODE` の coder 選定でのみ利用される。

---

## 4. 分散実行との接続

### 4.1 DistributedOrchestrator

**ファイル**: `internal/application/orchestrator/distributed_orchestrator.go`

`DistributedOrchestrator` は以下を持つ。

- `nodeCaps map[string]domainnode.Capability`
- `nodeSelector *NodeSelector`

`SetNodeCapabilities(caps)` で capability map を注入できる。

`routeToCoderForMessage(route, userMessage)` の現行挙動:

1. `RouteCODE` かつ `nodeSelector` と `nodeCaps` がある場合のみ capability 選定を使う
2. 接続済み coder 候補を列挙
3. `inferTaskRequirement(userMessage)` を作る
4. `NodeSelector.Select(...)` を実行
5. 選べなければ従来の fallback chain に戻る

### 4.2 Fallback chain

capability 選定が使えない場合:

- `CODE` は `coder1 -> coder2 -> coder3` の接続順 fallback
- `CODE1/2/3` は明示 target を優先

つまり現行実装は「能力ベース選定 + 静的 fallback」の二段構えである。

---

## 5. Transport 構成

### 5.1 Transport I/F

**ファイル**: `internal/domain/transport/transport.go`

```go
type Transport interface {
    Send(ctx context.Context, msg Message) error
    Receive(ctx context.Context) (Message, error)
    Close() error
    IsHealthy() bool
}
```

実装:

- `LocalTransport`
- `SSHTransport`

### 5.2 TransportFactory

**ファイル**: `internal/infrastructure/transport/factory.go`

`DistributedConfig.Transports` から agent ごとの transport を構築する。

- `type=local` -> `LocalTransport`
- `type=ssh` -> `SSHTransport`

SSH では次が必要:

- `remote_host`
- `remote_user`
- `ssh_key_path`

必要に応じて:

- `remote_agent_path`
- `remote_config_path`

### 5.3 現行のデバイス能力の位置づけ

audio / browser / gpu の能力は、汎用 probe ではなく capability map と設定運用で表現される。  
特に audio 系は `audio_router`、browser 系は `chrome` / `canvas` キーワード、remote 実行系は SSH transport と結びつく。

---

## 6. OpenClaw 観点との差分

現行到達点:

- node capability の型がある
- task requirement の型がある
- coder 選定に capability matching が入っている
- local / ssh の複数 transport がある

未到達:

1. capability heartbeat はない
2. health と capability を統合した node registry はない
3. audio/browser/gpu の自動 probe はない
4. 一般 task 全体の scheduler はなく、主に coder 選定に限定される
5. 負荷分散・フェイルオーバー最適化は限定的

---

## 7. 確認観点

- `Capability` / `TaskRequirement` が domain に存在する
- `RouteCODE` で capability select が走る
- `coder3` 優先ルールがある
- `TransportFactory` が `local` と `ssh` を構築する

以上をもって、RenCrow の nodes/capability は「未実装」ではなく、coder 選定に限った最小能力モデルとして実装済みである。
