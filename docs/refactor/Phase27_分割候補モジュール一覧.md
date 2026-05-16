# Phase27 分割候補モジュール一覧

## 目的

この文書は、Phase26 後の RenCrow に残っている分割候補を整理するための一覧である。

分割の目的は、単にファイルを小さくすることではない。仕様変更時に主に触る実装箇所を説明できるようにし、モジュール化と疎結合を維持することである。

## 分割判断の基準

次のいずれかに当てはまる場合、分割候補として扱う。

- 1 つのファイルが複数の仕様変更理由を持っている。
- 仕様変更時に触る主担当ファイルを 1 対 1 に説明しにくい。
- `cmd/` 配下に provider、handler、usecase、persistence の詳細が残っている。
- helper / service / manager 的に責務が集まり始めている。
- Viewer 表示、音声、口パク、ログ、runtime config の契約が混ざる危険がある。

## 最優先

### `cmd/picoclaw/runtime_dependencies.go`

現在も最大の集中点である。

残っている主な責務:

- Viewer wiring
- store setup
- provider setup
- orchestrator setup
- channel setup
- heartbeat
- tool registry
- runtime dependency assembly

分割候補:

- `runtime_viewer.go`
- `runtime_stores.go`
- `runtime_providers.go`
- `runtime_orchestrators.go`
- `runtime_channels.go`
- `runtime_heartbeat.go`
- `runtime_tools.go`

注意:

- Application 層の usecase 本体へ踏み込まない。
- composition root としての依存注入に留める。
- route、handler 本体、provider 実装、runtime config の意味を変えない。

## 高優先

### `cmd/picoclaw/llm_runtime_factory.go`

LLM provider assembly、alias 解決、timeout、warmup がまとまっている。

分割候補:

- provider factory 呼び出し
- local alias 解決
- timeout 解決
- model 解決
- warmup

注意:

- provider 固有仕様を Application / Domain へ漏らさない。
- repo example と live runtime config を混同しない。
- fallback を正常系として扱わない。

### `cmd/picoclaw/tts_runtime_factory.go`

TTS provider selection と command spec がまとまっている。

分割候補:

- Irodori provider setup
- SBV2 provider setup
- fallback synthesizer setup
- command spec assembly

注意:

- TTS chunk を Viewer 表示本文の根拠にしない。
- 音声、口パク、ログの契約を混同しない。

### `cmd/picoclaw/idlechat_tts.go`

IdleChat と TTS の接続点である。

分割候補:

- IdleChat TTS trigger
- pending session 管理
- topic gate
- public / internal session mapping

注意:

- IdleChat raw response、view data、audio trigger を混同しない。
- fallback は成功扱いしない。

## 中優先

### `cmd/picoclaw/cli_operations.go`

CLI 操作が大きくまとまっている。

分割候補:

- channel registry
- ollama ops
- logs
- source registry
- knowledge
- evidence

注意:

- CLI command ごとに入力、出力、副作用、ログを分ける。
- runtime service の挙動変更と混ぜない。

### `internal/application/orchestrator/distributed_orchestrator.go`

Phase26 で runtime / retry / display helper は分離済みだが、root file はまだ大きい。

残してよい責務:

- constructor
- public setters
- `ProcessMessage`
- thin delegation

分割候補:

- constructor assembly
- public setter group
- process message flow

注意:

- Phase15 から Phase23 の分散実行契約を維持する。
- local / ssh / mailbox / direct / distributed transport の error contract を変えない。

### `internal/application/service/worker_execution_service.go`

Phase26 で file / shell / git / error / summary helper は分離済みだが、実行 orchestration 本体はまだ太めである。

分割候補:

- proposal command parsing
- sequential execution
- parallel execution
- command dispatch
- pre / post auto commit orchestration

注意:

- `WorkerExecutionService` interface は維持する。
- Coder は plan / patch 生成、Worker が実行主体という境界を崩さない。
- protected file、workspace、failure classification の拒否契約を優先して検証する。

## 必要に応じて

### `internal/adapter/viewer/handler.go`

Viewer の送信、runtime config、page handler の境界確認対象である。

分割候補:

- page handler
- runtime config handler
- send handler
- attachment send handler

注意:

- DOM 要素の存在だけで成功扱いしない。
- Viewer 表示本文、SSE event、event log、history、audio trigger を分けて確認する。

### `internal/application/idlechat/orchestrator.go`

IdleChat の中心実装である。

分割候補:

- manual mode
- forecast mode
- story mode
- raw / view response handling
- TTS coordination
- history / topic handling

注意:

- raw response、view data、audio trigger を混同しない。
- STT input は通常 chat のみに流し、IdleChat に流さない。

### `internal/infrastructure/persistence/conversation/l1_sqlite_store.go`

Memory / Source Registry / conversation persistence の集中点である。

分割候補:

- conversation event store
- memory state transition
- source registry staging
- validation / promote
- search cache
- query helpers

注意:

- observed、candidate、validated、promoted の状態遷移を飛ばさない。
- Source Registry を無審査で正式 memory へ昇格しない。

## Phase27 実施結果

Phase27 では、上記の分割候補を対象に、挙動変更を入れず top-level declaration の配置を責務単位へ移した。

実施した分割:

- `cmd/picoclaw/runtime_dependencies.go`
  - `runtime_background_jobs.go`
  - `runtime_event_relay.go`
  - `runtime_coders.go`
  - `runtime_heartbeat.go`
  - `runtime_distributed_mode.go`
  - `runtime_subagent_tools.go`
- `cmd/picoclaw/llm_runtime_factory.go`
  - `llm_conversation_runtime.go`
  - `llm_local_alias.go`
  - `llm_runtime_warmup.go`
- `cmd/picoclaw/tts_runtime_factory.go`
  - `tts_runtime_irodori.go`
  - `tts_runtime_options.go`
- `cmd/picoclaw/idlechat_tts.go`
  - `idlechat_tts_text.go`
  - `idlechat_tts_queue.go`
  - `idlechat_tts_pending.go`
  - `idlechat_tts_voice.go`
- `cmd/picoclaw/cli_operations.go`
  - `cli_channels.go`
  - `cli_gateway_ollama.go`
  - `cli_logs.go`
  - `cli_evidence.go`
  - `cli_source_registry.go`
  - `cli_knowledge.go`
- `internal/application/orchestrator/distributed_orchestrator.go`
  - `distributed_orchestrator_constructor.go`
  - `distributed_orchestrator_settings.go`
  - `distributed_orchestrator_ports.go`
  - `distributed_orchestrator_process.go`
  - `distributed_orchestrator_execution.go`
- `internal/application/service/worker_execution_service.go`
  - `worker_execution_lifecycle.go`
  - `worker_execution_modes.go`
  - `worker_execution_dispatch.go`
- `internal/adapter/viewer/handler.go`
  - `handler_assets.go`
  - `handler_sse.go`
  - `handler_send.go`
- `internal/application/idlechat/orchestrator.go`
  - `orchestrator_constructor.go`
  - `orchestrator_modes.go`
  - `orchestrator_monitor.go`
  - `orchestrator_topics.go`
  - `orchestrator_summary.go`
  - `orchestrator_response_generation.go`
  - `orchestrator_loop_detection.go`
  - `orchestrator_prompts.go`
  - `orchestrator_sanitize.go`
  - `orchestrator_timeline.go`
- `internal/infrastructure/persistence/conversation/l1_sqlite_store.go`
  - `l1_sqlite_types.go`
  - `l1_sqlite_schema.go`
  - `l1_sqlite_messages.go`
  - `l1_sqlite_search_cache.go`
  - `l1_sqlite_events.go`
  - `l1_sqlite_source_registry.go`
  - `l1_sqlite_staging.go`
  - `l1_sqlite_staging_validation.go`
  - `l1_sqlite_promotions.go`
  - `l1_sqlite_news_digest.go`
  - `l1_sqlite_knowledge.go`
  - `l1_sqlite_validation.go`
  - `l1_sqlite_meta_helpers.go`
  - `l1_sqlite_scan.go`

Phase27 の完了条件:

- 分割候補一覧に挙げた対象が、責務単位のファイルへ分割されている。
- handler 本体、DTO、SSE event、IdleChat 契約、Viewer 表示契約、STT/TTS provider 挙動、LLM provider 挙動は変更しない。
- fallback を正常系として扱わない方針を維持する。
- Viewer 表示、音声、口パク、ログを混同しない方針を維持する。
- `go test ./...` と Phase25 E2E で回帰を確認する。

Phase27 後に残す判断:

- `cmd/picoclaw/runtime_dependencies.go` はまだ composition root として大きいが、Phase27 では関数移動に留める。
- `buildDependencies` 本体をさらに builder 化する場合は、次 Phase で store setup、provider setup、viewer setup、agent setup の順に分ける。
- 次 Phase では、単なるファイル分割ではなく、仕様変更時の主担当ファイルを 1 対 1 で説明できるかを基準にする。
