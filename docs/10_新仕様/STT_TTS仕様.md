# STT / TTS 仕様

## 目的

STT / TTS は Viewer と会話体験に接続する audio 境界である。

STT は通常 chat への入力経路、TTS は応答や IdleChat の音声出力経路として扱う。Viewer 表示本文、音声 chunk、口パク trigger、ログを混同しない。

## STT

### 入力経路

```text
Browser mic
  -> Viewer STT client
  -> /stt-ws or /ws
  -> STT gateway / provider
  -> final text
  -> normal chat input
```

STT input は通常 chat に流す。IdleChat に直接流さない。

### 主な実装箇所

| 領域 | 主担当 |
| --- | --- |
| STT runtime factory | `cmd/picoclaw/stt_runtime_factory.go` |
| STT runtime config | `cmd/picoclaw/stt_runtime_config.go` |
| STT WebSocket | `cmd/picoclaw/stt_runtime_websocket.go` |
| STT HTTP / audio | `cmd/picoclaw/stt_runtime_http.go`, `cmd/picoclaw/stt_runtime_audio.go` |
| STT provider | `internal/infrastructure/stt` |
| route registration | `cmd/picoclaw/routes.go` |

### 注意

- `/stt-ws` と `/ws` は互換 endpoint として扱う。
- trailing silence がないと final text に進まない場合がある。
- gateway 未設定時の fallback は品質低下を伴う回復経路であり、正常系として扱わない。

## TTS

### 出力経路

```text
response / IdleChat event
  -> TTS bridge
  -> provider
  -> audio bytes / media URL
  -> audio router / Viewer playback
  -> lipsync trigger
```

音声 chunk は本文表示の唯一の根拠ではない。

### 主な実装箇所

| 領域 | 主担当 |
| --- | --- |
| TTS runtime factory | `cmd/picoclaw/tts_runtime_factory.go` |
| IdleChat TTS queue / pending / voice | `cmd/picoclaw/idlechat_tts*.go` |
| RenCrow TTS bridge | `internal/infrastructure/tts/rencrow_tts_*.go` |
| Irodori provider | `internal/infrastructure/tts/irodori_*.go` |
| SBV2 provider | `internal/infrastructure/tts/sbv2_provider.go` |
| audio router | `internal/infrastructure/audiorouter` |
| Viewer audio route | `internal/adapter/viewer/audio_router_sse.go`, `/viewer/tts/audio` |
| lipsync / VTuber | `internal/infrastructure/vtuber`, `cmd/picoclaw/tts_vtuber_lipsync.go`, `cmd/picoclaw/vtuber_bridge.go` |

## 口パク trigger

口パクは音声出力や TTS event に同期する演出である。本文表示やログを口パクの根拠にしない。

TTS が失敗した場合、Viewer 表示が成功していても音声・口パクは成功扱いしない。

## 検証

STT:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/stt ./cmd/picoclaw
```

TTS:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/tts ./internal/infrastructure/audiorouter ./cmd/picoclaw
```

live / browser では次を確認する。

- mic permission
- WebSocket 接続
- final text
- normal chat 入力への接続
- TTS provider response
- browser playback
- lipsync trigger
- log と表示本文の分離
