# 実装仕様: RenCrow VTuber連携 v0.1

**更新日**: 2026-03-19  
**ステータス**: 現行実装ベース  
**対象**: `feature/rencrow` の VTuber / VTube Studio 連携  

---

## 1. 概要

RenCrow の VTuber 連携は、会話や IdleChat で生成された発話テキストから感情状態を組み立て、character 単位で VTube Studio 互換 WebSocket endpoint へ `emotion_tick` を送る最小実装である。

現行実装は次の責務に限定される。

1. character ごとの VTS 接続先設定
2. 発話内容からの感情値生成
3. `emotion_tick` の WebSocket 送信
4. character ごとの接続再利用

OBS 合成、YouTube 配信、VTS 側 plugin 実装、Live2D モデル構成は本仕様の対象外である。

---

## 2. 構成要素

### 2.1 起動時配線

**ファイル**: `cmd/picoclaw/main.go`  
**ファイル**: `cmd/picoclaw/vtuber_bridge.go`

- `buildVTuberBridge(cfg)` が `cfg.VTuber.Enabled` を見て bridge を構築する
- 有効時は `MessageOrchestrator` と `DistributedOrchestrator` の両方へ `SetVTuberBridge(...)` で注入される
- character 設定が 0 件なら warning を出して bridge は無効化される

### 2.2 Bridge 実装

**ファイル**: `internal/application/orchestrator/vtuber_bridge.go`  
**ファイル**: `internal/infrastructure/vtuber/client_bridge.go`

公開 I/F:

```go
type VTuberBridge interface {
    PublishEmotion(ctx context.Context, req VTuberEmotionRequest) error
}
```

`ClientBridge` は現行の具象実装であり、character ごとに runtime connection を保持する。

### 2.3 設定

**ファイル**: `internal/adapter/config/config.go`

```go
type VTuberConfig struct {
    Enabled        bool
    TickIntervalMS int
    ConnectTimeout int
    WriteTimeout   int
    Characters     map[string]VTuberCharacterConfig
}

type VTuberCharacterConfig struct {
    AudioOutput   string
    VTSHost       string
    VTSPort       int
    ExpressionMap map[string]string
}
```

デフォルト値:

- `tick_interval_ms = 100`
- `connect_timeout_ms = 3000`
- `write_timeout_ms = 2000`

検証条件:

- `vtuber.enabled=true` の場合、character は 1 件以上必須
- 各 character に `audio_output`, `vts_host`, `vts_port`, `expression_map` が必要
- `tick_interval_ms` は `50..100`

---

## 3. データフロー

### 3.1 発火点

VTuber push は orchestrator 側で TTS payload と同じ入力から組み立てる。

関連ファイル:

- `internal/application/orchestrator/vtuber_support.go`
- `internal/application/orchestrator/vtuber_stream.go`
- `internal/application/orchestrator/message_orchestrator.go`
- `internal/application/orchestrator/distributed_orchestrator.go`
- `cmd/picoclaw/idlechat_tts.go`

大まかな流れ:

1. `agent.response` などの発話イベントが発生
2. `buildTTSPayload(...)` で発話テキストと感情状態を生成
3. `buildVTuberRequest(...)` で `VTuberEmotionRequest` を構築
4. `pushVTuber(...)` が bridge を通して送信

### 3.2 Character 選定

`VTuberEmotionRequest` は character 単位の 1 tick を表す。

主な項目:

- `CharacterID`
- `SessionID`
- `Speaking`
- `Valence`
- `Arousal`
- `Intensity`
- `EmotionLabel`
- `Expression`

character ID は設定読み込み時に小文字化・trim される。送信時も `PublishEmotion()` 側で同様に正規化される。

---

## 4. WebSocket 送信仕様

### 4.1 接続

`ClientBridge` は character ごとに `runtimeConn` を持ち、初回送信時に WebSocket 接続を確立する。

- scheme: `ws`
- host: `vts_host:vts_port`
- path: `/`
- origin: `http://localhost/`

接続確立後は runtime に再利用し、送信失敗時のみ close して次回再接続する。

### 4.2 送信 payload

VTS 側へ送る JSON は現行実装では次の形である。

```json
{
  "type": "emotion_tick",
  "character": "mio",
  "timestamp_ms": 1710000000000,
  "payload": {
    "speaking": 1,
    "valence": 0.42,
    "arousal": 0.31,
    "intensity": 0.55,
    "emotion_label": "happy",
    "expression": "smile",
    "audio_output": "CABLE Input"
  }
}
```

現行実装のルール:

- `speaking` は bool ではなく `0/1`
- `expression` は request 側指定が空なら `ExpressionMap[emotion_label]` を使う
- `audio_output` は character 設定に値がある場合のみ payload に含める

### 4.3 エラー処理

- `character_id` 空は error
- 未設定 character は error
- connect timeout / write timeout は error
- 送信失敗時はその character の connection を破棄し、次回再接続させる

上位 orchestrator 側では degraded warning として扱い、会話や TTS 自体は継続する。

---

## 5. Audio Router との関係

VTuber 連携は音声ファイルや PCM を直接送るのではなく、感情 tick と `audio_output` 指定を送るだけである。  
実際の TTS 音声配信は別系統の `TTSBridge` / `audio-router` が担当する。

つまり現行責務分離は次の通り。

- `VTuberBridge`: 表情・感情・話中状態
- `TTSBridge` / `audio-router`: 音声チャンク配信

Viewer には `GET /audio-router/events` SSE があり、音声ルータ側イベントのみ観測できる。VTuber 側専用 API は現状ない。

---

## 6. 実装到達点

2026-03-19 時点で確認できる到達点:

- `vtuber.enabled=true` で bridge を起動できる
- character ごとに host / port / expression map を持てる
- `MessageOrchestrator` と `DistributedOrchestrator` の双方から VTuber push が走る
- IdleChat の TTS 連携経路でも VTuber push が利用される
- VTS 障害が会話全体を止めない degraded 動作になっている

---

## 7. 既知の制約

1. VTube Studio の公式 API 契約そのものを実装しているわけではなく、`emotion_tick` を受けるローカル WebSocket endpoint を前提にしている。
2. `tick_interval_ms` は設定に存在するが、現行 bridge 実装では周期送信ループを持たず、発話イベント駆動で push する。
3. OBS / NDI / YouTube 連携はコードベースの正本仕様ではなく、運用構想レベルに留まる。
4. VTuber 側の状態確認用 HTTP API や Viewer タブは現状存在しない。
5. character ごとの音声経路分離は設定で表現されるが、最終的なデバイス制御は audio-router / 外部環境依存である。

---

## 8. 確認観点

実装確認時は次を見ればよい。

- 起動ログに `VTuber bridge enabled (characters=N)` が出る
- `vtuber.enabled=true` で config validation を通る
- 発話時に VTuber push failure が degraded warning としてのみ現れる
- character 未設定や接続断時でも本体会話が止まらない

以上をもって、VTuber 連携は Draft 構想ではなく「最小実装が本体に統合済み」の現行機能として扱う。
