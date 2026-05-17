# IdleChat 仕様

## 目的

IdleChat は、ユーザー操作がない時間にエージェント同士が自律的に会話する機能である。

Viewer と TTS を通じてリアルタイム表示・読み上げを行うが、raw response、view data、audio trigger は別契約として扱う。

## モード

| モード | 目的 | 主担当 |
| --- | --- | --- |
| normal idle | 通常の自律雑談 | `internal/application/idlechat/orchestrator*.go` |
| forecast | 未来展望・トピック探索 | `internal/application/idlechat/forecast_*.go` |
| story | 物語・昔話の読み上げ | `internal/application/idlechat/story_mode*.go` |

Viewer は IdleChat の Live Timeline、Summary Review、History を表示する。これらは raw response そのものではなく、表示用 state と診断情報の投影である。

## raw / view / audio 境界

| 種別 | 役割 |
| --- | --- |
| raw response | LLM が返した素の応答。診断・監査用。 |
| view data | Viewer 表示用に整形された本文。会話注入に使うのは view data。 |
| audio trigger | TTS と口パクを動かすための trigger。本文表示の唯一の根拠にしない。 |

fallback は成功扱いしない。空応答、invalid response、generation error は失敗または回復経路として扱い、Viewer / log に隠さない。

## STT との境界

STT input は通常 chat に流す。IdleChat に直接流さない。

ユーザー入力が来た場合、IdleChat は中断または状態更新の対象になるが、音声入力そのものを IdleChat 会話として扱わない。

## 主な実装箇所

| 領域 | 主担当 |
| --- | --- |
| orchestrator lifecycle | `internal/application/idlechat/orchestrator.go`, `orchestrator_constructor.go`, `orchestrator_modes.go` |
| response generation | `internal/application/idlechat/orchestrator_response_*.go` |
| sanitize / invalid response | `internal/application/idlechat/orchestrator_sanitize_*.go` |
| loop detection | `internal/application/idlechat/orchestrator_loop_*.go` |
| topic generation | `internal/application/idlechat/topic_generator_*.go`, `orchestrator_topics.go` |
| forecast | `internal/application/idlechat/forecast_*.go` |
| story | `internal/application/idlechat/story_mode*.go` |
| quality review | `internal/application/idlechat/quality_review.go` |
| Viewer handlers | `cmd/picoclaw/runtime_idlechat_handlers.go`, `internal/adapter/viewer/*idlechat*` |
| TTS bridge | `cmd/picoclaw/idlechat_tts*.go`, `internal/infrastructure/tts/rencrow_tts_*.go` |

## raw response 診断

IdleChat では、編集後の view data と LLM の素の raw response を分ける。

- raw response は空応答、invalid response、generation error、provider 出力異常の診断に使う。
- view data は Viewer 表示と会話注入に使う。
- Summary Review / History は、表示状態、境界、終了状態を追うための観測面である。
- fallback は成功扱いにしない。fallback に落ちた場合は失敗経路として記録する。

## event 契約

IdleChat は Viewer / TTS / log に event を出す。

- Viewer 向け event は表示用 state を更新する。
- TTS event は音声再生と口パクを起動する。
- log は診断・追跡のために残す。

`idlechat.viewer` を TTS 用に使わない。`idlechat.tts` を Viewer 表示本文として扱わない。

## 検証

確認対象:

- start / stop / status が動く。
- normal / forecast / story の開始条件が維持される。
- raw response と view data が分離される。
- audio trigger が本文表示と混ざらない。
- fallback が成功扱いされない。
- invalid response / generation error が隠れない。
- TTS 完了待ちと break が成立する。

主な確認:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/idlechat
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e
```
