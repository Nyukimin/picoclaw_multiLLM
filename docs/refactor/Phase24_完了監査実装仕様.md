# Phase24 完了監査実装仕様

## Phase 24 の目的

Phase 24 は、これまでの段階的リファクタリングを新しい構造変更へ進める Phase ではなく、完了監査の Phase とする。

目的は次の通り。

- Phase ごとのプロンプト、実装仕様、実装、検証、Push の流れが成立していることを確認する。
- `cmd/picoclaw`、`MessageOrchestrator`、`CodeExecutor`、`DistributedOrchestrator` の責務分割が、モジュール化と疎結合の方針に沿っていることを確認する。
- 通常テストと E2E テストの実行条件を明文化する。
- 実装後の構成を README に反映する。
- 完了判定に必要な最小修正だけを行い、追加の大規模リファクタを開始しない。

## 対象範囲

- `docs/refactor/` の Phase 文書
- `README.md`
- `config/config.yaml.example`
- `test/e2e/*.go`
- 検証コマンド
  - `GOCACHE=/tmp/picoclaw-gocache go test ./...`
  - `GOCACHE=/tmp/picoclaw-gocache go test -tags=e2e ./test/e2e`
  - `git diff --check`

## 対象外

- 新しい機能分割の開始
- handler 本体の変更
- DTO の変更
- SSE event の変更
- Viewer JS / CSS の変更
- IdleChat raw/view/audio 契約の変更
- STT / TTS provider の挙動変更
- LLM provider の挙動変更
- runtime config の意味変更
- 未追跡の `tests/` 配下の変更

## 現在の責務分割

### cmd/picoclaw

- `main.go`
  - CLI entrypoint
  - config load
  - top-level runtime assembly
  - server startup
  - registration / factory / wiring の呼び出し
- `routes.go`
  - HTTP route registration
  - route path と handler 接続の対応
- `runtime_dependencies.go`
  - runtime dependency assembly
- `runtime_options.go`
  - debug / display / runtime option assembly
- `health_runtime.go`
  - health / runtime helper

### MessageOrchestrator

- `message_orchestrator.go`
  - top-level orchestration
- `message_orchestrator_routes.go`
  - route dispatch
- `message_orchestrator_routing.go`
  - routing decision 周辺
- `message_orchestrator_response.go`
  - response assembly
- `message_orchestrator_session.go`
  - session lifecycle
- `message_orchestrator_task.go`
  - task context builder
- `message_orchestrator_tts_lifecycle.go`
  - TTS lifecycle
- `message_orchestrator_events.go`
  - event emitter port
- `message_orchestrator_idle.go`
  - IdleChat entry
- `message_orchestrator_autonomous.go`
  - autonomous route handling

### CodeExecutor

- `code_executor.go`
  - CodeExecutor の entry と主要構造
- `code_executor_selection.go`
  - coder selection
- `code_executor_proposal.go`
  - proposal path
- `code_executor_generate.go`
  - generate path
- `code_executor_events.go`
  - event helper
- `code_executor_response.go`
  - response helper

### DistributedOrchestrator

- `distributed_orchestrator.go`
  - top-level orchestration と薄い委譲
- `distributed_orchestrator_events.go`
  - event emission
- `distributed_orchestrator_evidence.go`
  - execution report / evidence
- `distributed_orchestrator_tts_lifecycle.go`
  - TTS lifecycle
- `distributed_orchestrator_session.go`
  - distributed session lifecycle
- `distributed_orchestrator_autonomous.go`
  - autonomous coordinator
- `distributed_orchestrator_routes.go`
  - route dispatcher
- `distributed_orchestrator_transport.go`
  - transport executor
- `distributed_orchestrator_code.go`
  - code execution
- `distributed_orchestrator_coder_selection.go`
  - coder selection
- `distributed_orchestrator_attribution.go`
  - attribution guard

## Phase24 で行う最小修正

### config example

`config/config.yaml.example` は E2E の repo example として参照される可能性があるため、YAML として parse できる必要がある。

`audio_router.device_map` の中に `shiro` キーが重複し、片方が `vtuber.characters.shiro` 相当の内容になっていたため、`shiro` の VTuber character 設定を `vtuber.characters` に戻す。

これは live runtime config の意味変更ではなく、repo example の構造不備修正である。

### E2E helper

`test/e2e` は `//go:build e2e` 付きであり、通常の `go test ./...` には含まれない。

E2E helper は次を満たす必要がある。

- `PICOCLAW_CONFIG` が指定されていればそれを優先する。
- 未指定の場合は tracked repo example まで探索できる。
- 外部 API key が未設定の場合は、既存方針通り該当ケースを skip する。
- Ollama 実体が到達不能な場合は、該当 Ollama 実機 E2E を環境未準備として skip する。
- Viewer model switch の httptest ベース E2E は外部依存なしで実行する。

skip は runtime fallback 成功として扱わない。外部依存が準備されていないことを明示するためのテスト制御とする。

## 入力

- Phase 1 から Phase 23 までの文書と実装差分
- `README.md`
- `config/config.yaml.example`
- `test/e2e/*.go`
- 現在コードの主要ファイル構成
- 検証コマンド出力

## 出力

- `docs/refactor/Phase24_完了監査実装仕様プロンプト.md`
- `docs/refactor/Phase24_完了監査実装仕様.md`
- `docs/refactor/Phase24_最終完了判定.md`
- 更新された `README.md`
- parse 可能な `config/config.yaml.example`
- tagged E2E を実行できる `test/e2e` helper

## 副作用

- docs と README が現在の構成に追従する。
- repo example config が YAML として正しく parse できるようになる。
- E2E の外部依存未準備時の失敗が skip に切り分けられる。

## 永続化

Phase24 では runtime data、DB、session、logs の永続化仕様を変更しない。

## ログ

E2E 実行時に Viewer handler の test log が出る場合がある。これは handler 挙動の変更ではなく、既存 handler を httptest で通した結果である。

## エラー契約

- config file が存在しない場合は、E2E helper が候補パスを表示して失敗する。
- YAML parse error は config example の構造不備として扱う。
- 外部 API key 未設定は該当 API E2E の skip とする。
- Ollama 到達不能は該当 Ollama E2E の skip とする。
- httptest で完結する E2E は skip せず失敗を不具合として扱う。

## 変更してはいけない既存挙動

- Chat / Worker / Coder の責務境界
- fallback を正常系にしない方針
- Viewer 表示、音声、口パク、ログを混同しない方針
- repo example と live runtime config を混同しない方針
- handler 本体、DTO、SSE event、IdleChat 契約、STT/TTS provider、LLM provider の挙動

## 検証手順

1. 通常テスト

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./...
```

2. E2E テスト

```bash
GOCACHE=/tmp/picoclaw-gocache go test -tags=e2e ./test/e2e
```

3. E2E 詳細確認

```bash
GOCACHE=/tmp/picoclaw-gocache go test -v -tags=e2e ./test/e2e
```

4. 差分確認

```bash
git diff --check
git diff --stat
```

## リスク

- E2E skip を実装成功と誤読するリスク。
- repo example config と live runtime config を混同するリスク。
- README の構成説明が再び古くなるリスク。
- Phase 文書が多いため、個別 Phase の詳細と最終完了判定の役割を混同するリスク。

## 完了条件

- Phase24 の prompt / 実装仕様 / 最終完了判定が存在する。
- README が現在構成と検証コマンドに合っている。
- `config/config.yaml.example` が YAML として parse できる。
- `GOCACHE=/tmp/picoclaw-gocache go test ./...` が成功する。
- `GOCACHE=/tmp/picoclaw-gocache go test -tags=e2e ./test/e2e` が成功する。
- `git diff --check` が成功する。
- 未追跡の `tests/` を触っていない。
- 変更が commit / Push されている。
