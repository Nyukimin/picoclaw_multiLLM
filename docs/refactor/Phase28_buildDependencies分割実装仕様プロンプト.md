# Phase28 buildDependencies 分割実装仕様プロンプト

```text
Goal:
  RenCrow のリファクタリング Phase28 として、cmd/picoclaw/runtime_dependencies.go の buildDependencies() 本体を責務単位へ分割するための実装仕様書を作成してください。

Repository:
  /home/nyukimi/picoclaw_multiLLM

作成する文書:
  docs/refactor/Phase28_buildDependencies分割実装仕様.md

目的:
  - buildDependencies() に残っている runtime wiring の集中を解消する。
  - composition root としての責務は維持しつつ、store setup、provider setup、conversation runtime、agent setup、Viewer handler wiring、channel handler wiring、orchestrator wiring、heartbeat wiring を分ける。
  - 挙動変更なしで、仕様変更時に主に触る実装箇所を説明できる状態に近づける。
  - モジュール化と疎結合を最重要方針として、Application / Adapter / Infrastructure の usecase 本体へ踏み込まない。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/10_新仕様/モジュール構成仕様.md
  5. docs/refactor/リファクタリング指針.md
  6. docs/refactor/Phase26_システムリファクタリング方針.md
  7. docs/refactor/Phase27_分割候補モジュール一覧.md
  8. cmd/picoclaw/runtime_dependencies.go
  9. cmd/picoclaw/runtime_*.go
  10. cmd/picoclaw/*_test.go
  11. test/e2e/phase25_*_test.go

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語にする。
  - 正本仕様は docs/01_正本仕様/実装仕様.md のままとする。
  - TODO / TBD の仮置きは残さない。
  - handler 本体、DTO、SSE event、IdleChat 契約、Viewer 表示契約、STT/TTS provider 挙動、LLM provider 挙動は変更しない前提にする。

文書に必ず含める内容:

  1. Phase28 の目的
     - buildDependencies() の runtime wiring 集中を解消すること
     - composition root としての責務は維持すること
     - 挙動変更なしで分割すること

  2. 対象範囲
     - cmd/picoclaw/runtime_dependencies.go の buildDependencies()
     - buildDependencies() から呼ばれる既存 runtime helper
     - Dependencies struct へ詰める handler / service / store / runtime の組み立て

  3. 対象外
     - route 登録変更
     - handler 本体変更
     - provider 実装変更
     - runtime config の意味変更
     - Viewer JS / CSS 変更
     - IdleChat 契約変更
     - STT / TTS provider 挙動変更
     - LLM provider 挙動変更
     - MessageOrchestrator / DistributedOrchestrator の usecase 本体変更
     - L1SQLiteStore の persistence 契約変更

  4. 現在の buildDependencies() の責務分類
     - tool registry
     - capability probe
     - LLM providers
     - MCP / ToolRunner
     - conversation runtime / L1 SQLite
     - glossary
     - agents
     - session / memory store
     - WorkerExecutionService
     - Viewer handlers
     - IdleChat runtime
     - MessageOrchestrator / DistributedOrchestrator
     - channel handlers
     - heartbeat

  5. 提案する分割関数
     - buildRuntimeToolRegistry
     - buildCapabilityRuntime
     - buildLLMRuntimeProviders
     - buildToolRuntime
     - buildConversationRuntime
     - buildGlossaryRuntime
     - buildAgentRuntime
     - buildSessionRuntime
     - buildViewerRuntimeHandlers
     - buildIdleChatRuntime
     - buildOrchestratorRuntime
     - buildChannelRuntimeHandlers
     - buildHeartbeatRuntime

  6. 各関数の契約
     各関数について以下を書く:
     - 入力
     - 出力
     - 副作用
     - 永続化
     - ログ
     - エラー契約
     - 変更してはいけない既存挙動

  7. 実装手順
     - baseline test を実行する
     - 小さい責務から関数化する
     - 初期化順を変えない
     - nil check を変えない
     - handler 契約を変えない
     - store path / runtime config の意味を変えない
     - LLM Ops start gate の生成順を変えない
     - Viewer 表示、音声、口パク、ログを混同しない
     - fallback を正常系として扱わない
     - gofmt / goimports を実行する
     - after test を実行する

  8. 検証手順
     - baseline:
       GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
     - after:
       GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
       GOCACHE=/tmp/picoclaw-gocache go test ./...
       GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e
     - 差分確認:
       git diff --check
       git diff --stat
     - live health が使える場合:
       curl -fsS http://127.0.0.1:18790/health
     - live/browser E2E が使える場合:
       PICOCLAW_LIVE_E2E=1 GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e -run TestE2E_Phase25LiveRuntimeHealthAndViewerConfig -v
       PICOCLAW_BROWSER_E2E=1 PICOCLAW_LIVE_BASE_URL=http://127.0.0.1:18790 GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e -run TestE2E_Phase25BrowserViewerSessionContract -v

  9. リスク
     - 初期化順を変える
     - nil dependency を変える
     - Viewer handler 登録漏れ
     - IdleChat 起動条件の変化
     - LLM Ops start gate の変化
     - L1SQLite / Source Registry / Memory の保存先変化
     - ToolRunner / PolicyEngine の共有境界を崩す
     - Chat / Worker / Coder の責務境界を崩す
     - fallback を正常系として扱ってしまう
     - repo example と live runtime config を混同する

  10. 完了条件
      - 実装仕様書が docs/refactor/ に作成されている
      - buildDependencies() の責務分類がある
      - 分割関数案と契約がある
      - 検証手順がある
      - コード変更は行っていない
      - ユーザーが次に「実装してよいか」を判断できる

実行手順:
  1. 参照文書を読む。
  2. cmd/picoclaw/runtime_dependencies.go の buildDependencies() を確認する。
  3. buildDependencies() の現在責務を分類する。
  4. 分割関数案と各関数の契約を書く。
  5. docs/refactor/Phase28_buildDependencies分割実装仕様.md を作成する。
  6. コード変更は行わない。
  7. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
