# Viewer 仕様

## 目的

Viewer は RenCrow の操作面と観測面である。

Viewer は単なる静的 UI ではない。HTTP API、SSE event、event log、history、monitor、Memory / Source Registry、IdleChat、STT/TTS の状態を投影する。

## 境界

Viewer では次を混同しない。

- 表示本文
- SSE event
- event log
- history
- audio trigger
- lipsync trigger
- runtime config 表示
- debug / monitor 情報

音声 chunk は本文表示の唯一の根拠ではない。音声 chunk は音声再生と口パク trigger のための契約である。

## 主な実装箇所

| 領域 | 主担当 |
| --- | --- |
| Viewer handler | `internal/adapter/viewer/*_handler.go` |
| Viewer send | `internal/adapter/viewer/handler_send.go` |
| static page/assets | `internal/adapter/viewer/viewer.html`, `internal/adapter/viewer/assets/` |
| SSE hub | `internal/adapter/viewer/hub.go` |
| event log | `internal/adapter/viewer/event_log_store.go`, `event_log_gc.go` |
| monitor | `internal/adapter/viewer/monitor_*.go` |
| runtime config | `internal/adapter/viewer/debug_system_handler.go`, `cmd/picoclaw/routes.go`, `cmd/picoclaw/runtime_*.go` |
| LLM Ops | `internal/adapter/viewer/llm_ops_handler.go` |
| Source Registry | `internal/adapter/viewer/source_registry_handler.go` |
| Memory API | `internal/adapter/viewer/memory_*_handler.go` |

## route / API

代表的な route:

- `/viewer`
- `/viewer/assets/`
- `/viewer/runtime-config`
- `/viewer/send`
- `/viewer/status`
- `/viewer/jobs`
- `/viewer/logs`
- `/viewer/audit/summary`
- `/viewer/evidence/*`
- `/viewer/memory/*`
- `/viewer/source-registry`
- `/viewer/idlechat/*`
- `/viewer/tts/audio`
- `/viewer/llm-ops/*`

route 登録は `cmd/picoclaw/routes.go` が担当する。handler 本体は `internal/adapter/viewer` に置く。

## runtime config

Viewer runtime config は表示と操作のための投影である。

次を混同しない。

- repo example config
- live `~/.picoclaw/config.yaml`
- process 起動時に解決された runtime config
- Viewer に返す runtime config

live runtime を判断する場合は、repo example ではなく `/health`、`/viewer/runtime-config`、実設定、fresh log を確認する。

## IdleChat と Viewer

IdleChat event は Viewer に表示されるが、IdleChat の raw response、view data、audio trigger は別契約である。

- raw response: LLM の素の出力、診断用に保持する。
- view data: Viewer 表示用に整形された本文。
- audio trigger: TTS / lipsync 起動用。

## 検証

Viewer 変更では DOM 存在だけで完了扱いしない。

最低 1 session で確認する。

- Viewer が開く。
- 入力できる。
- `/viewer/send` が成功する。
- route と response が対応する。
- SSE event が届く。
- event log / history に残る。
- error / invalid response が隠れない。
- TTS / lipsync trigger が本文表示と混ざらない。

主な確認:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/adapter/viewer
GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e
```

live 確認:

```bash
curl -fsS http://127.0.0.1:18790/health
curl -fsS http://127.0.0.1:18790/viewer/runtime-config
```
