# Phase34 モジュール分割完了監査

## 目的

この文書は、Phase28 から Phase33 までの分割結果を監査し、現時点で追加分割すべき production module が残っているかを判定するための完了記録である。

判断基準は、行数だけではなく、仕様変更時に主担当ファイルを説明できるか、複数の仕様変更理由が 1 ファイルに混ざっているか、差し替え可能な境界が見えるかである。

## 完了対象

Phase28 から Phase33 では、次の責務集中を分割した。

| Phase | 対象 | 結果 |
| --- | --- | --- |
| Phase28 | `cmd/picoclaw/runtime_dependencies.go` の `buildDependencies()` | runtime wiring を capability、LLM provider、tool runtime、conversation、glossary、agent、session、viewer、IdleChat、orchestrator、channel、heartbeat に分割 |
| Phase29 | forecast session、viewer monitor、VectorDB、RealConversationManager、DuckDB | IdleChat forecast、Viewer monitor、conversation persistence を責務別ファイルへ分割 |
| Phase30 | config、ToolRunner | config types/defaults/validation/local LLM helper、ToolRunner registration/wrapper/shell/file/web search を分割 |
| Phase31 | IdleChat sanitize/topic/response/loop 補助処理 | raw/view/topic/retry/loop/similarity/attribution の補助責務を分割 |
| Phase32 | MioAgent、`picoclaw-agent` | Mio options/web search/persona/attribution/helper と agent process handler/runtime/message loop を分割 |
| Phase33 | TTS / LLM provider | Irodori、RenCrow TTS bridge、OpenAI、Ollama を provider 本体、URL、audio、params、retry、stream、message、thinking bridge、model ready へ分割 |

## 検証結果

各 Phase で baseline / after の対象テストを実行し、Phase33 終了時点で次を確認した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
GOCACHE=/tmp/picoclaw-gocache go test ./...
GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e
git diff --check
curl -fsS http://127.0.0.1:18790/health
```

確認結果:

- `go test ./cmd/picoclaw`: 成功
- `go test ./...`: 成功
- `go test -tags=e2e ./test/e2e`: 成功
- `git diff --check`: 成功
- live health: `status: ok`

## 残存 production file 監査

Phase33 後に残っている大きめの production file を確認した。

| ファイル | 行数目安 | 判定 | 理由 |
| --- | ---: | --- | --- |
| `internal/adapter/config/config_types.go` | 478 | 追加分割不要 | config type 定義だけで実行ロジックを持たない。仕様変更時は該当 config type を編集する。 |
| `internal/infrastructure/persistence/conversation/vectordb_kb.go` | 443 | 追加分割不要 | VectorDB の KB API に閉じている。thread memory は `vectordb_thread_memory.go` へ分離済み。 |
| `internal/infrastructure/transport/ssh.go` | 437 | 現時点では追加分割不要 | SSH transport の dial/connect/send/receive/reconnect に閉じている。remote transport contract を変える時の主担当として説明できる。 |
| `internal/application/idlechat/forecast_trend_sources.go` | 434 | 追加分割不要 | forecast trend source fetch / rank に閉じている。forecast session runner や stock からは分離済み。 |
| `internal/infrastructure/tts/sbv2_provider.go` | 393 | 現時点では追加分割不要 | SBV2 provider 1 つの実装に閉じている。editor API と provider contract の境界は同一 provider 内の詳細である。 |
| `internal/adapter/config/config_defaults.go` | 381 | 追加分割不要 | default 設定の単一責務である。型定義と validation からは分離済み。 |
| `internal/domain/conversation/recall_pack.go` | 375 | 追加分割不要 | recall pack budgeting / prompt conversion の単一 domain contract に閉じている。 |
| `internal/infrastructure/llm/providers/claude/provider.go` | 374 | 現時点では追加分割不要 | Claude provider 1 つの実装に閉じている。OpenAI / Ollama のような追加 stream/reasoning/model-ready 分岐はない。 |
| `cmd/picoclaw/stt_runtime_websocket.go` | 361 | 現時点では追加分割不要 | STT websocket runtime route/proxy/provider handling に閉じている。STT provider 本体や Viewer 表示とは分離されている。 |
| `internal/domain/agent/coder.go` | 358 | 現時点では追加分割不要 | Coder の proposal generation と proposal extraction contract に閉じている。破壊的操作は直接実行しない境界を維持している。 |

上記は行数だけなら大きいが、現時点では「仕様変更時に主担当ファイルを説明できる」状態であり、Phase28 から Phase33 の目的だった未分化な集中点とは異なる。

## 仕様と実装箇所の対応

Phase28 から Phase33 の結果、主要な仕様変更時の主担当は次のように説明できる。

| 仕様変更領域 | 主担当ファイル群 |
| --- | --- |
| composition root / dependency wiring | `cmd/picoclaw/runtime_*.go` |
| HTTP route registration | `cmd/picoclaw/routes.go` |
| config type/default/validation | `internal/adapter/config/config_types.go`, `config_defaults.go`, `config_validation.go` |
| ToolRunner registration / shell / file / web search | `internal/infrastructure/tools/runner_*.go` |
| IdleChat forecast | `internal/application/idlechat/forecast_*.go` |
| IdleChat sanitize / topic / response / loop | `internal/application/idlechat/orchestrator_sanitize_*.go`, `topic_generator_*.go`, `orchestrator_response_*.go`, `orchestrator_loop_*.go` |
| Mio web search / persona / attribution | `internal/domain/agent/mio_*.go` |
| standalone agent process | `cmd/picoclaw-agent/*.go` |
| Viewer monitor | `internal/adapter/viewer/monitor_*.go` |
| VectorDB thread memory / KB | `internal/infrastructure/persistence/conversation/vectordb_*.go` |
| RealConversationManager recall / thread / archive / KB | `internal/infrastructure/persistence/conversation/real_manager_*.go` |
| DuckDB archive / thread / export | `internal/infrastructure/persistence/conversation/duckdb_*.go` |
| TTS provider / bridge | `internal/infrastructure/tts/irodori_*.go`, `rencrow_tts_*.go`, `sbv2_provider.go` |
| LLM provider | `internal/infrastructure/llm/providers/openai/*.go`, `ollama/*.go`, provider package files |

## 維持された方針

次の方針は維持している。

- モジュール化と疎結合を最重要方針にする。
- 単にファイルを分けるだけではなく、仕様変更理由で分ける。
- fallback を正常系として扱わない。
- Viewer 表示、音声、口パク、ログを混同しない。
- repo example と live runtime config を混同しない。
- archive 文書を一次参照にしない。
- Chat / Worker / Coder の責務境界を崩さない。
- public interface、handler contract、DTO、SSE event、provider contract、DB schema を変更しない。

## 完了判定

Phase28 で対象になった `buildDependencies()` 分割から始まり、Phase29 から Phase33 で残存していた大型かつ未分化な production module は責務単位へ分割した。

Phase33 後の残存 production file は、行数は残るものの、型定義、単一 provider、単一 store API、単一 transport、単一 domain contract として説明できる。現時点で追加分割すべき未分化モジュールは残っていない。

今後、仕様追加により上記の残存ファイルが複数の変更理由を持つようになった場合は、その時点で新しい Phase として仕様化してから分割する。
