# Phase20 distributed transport executor 境界整理実装仕様作成プロンプト

Goal:
  RenCrow のリファクタリング Phase20 として、DistributedOrchestrator の mailbox / SSH / local router execution を distributed transport executor 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase20: distributed transport executor 境界整理

目的:
  - `executeToAgent` / `executeToAgentViaMailbox` / `executeViaLocal` / `executeViaSSH` を、分散 transport 専用 collaborator へ分離する。
  - mailbox event、timeout、CentralMemory record、SSH/local 分岐、error message の既存契約を維持する。
  - route dispatcher、Code route retry、node selection、coder config、attribution guard 本体には踏み込まない。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/Phase14_DistributedOrchestrator残責務棚卸し.md
  6. docs/refactor/Phase19_完了判定.md
  7. docs/codebase-map/アーキテクチャ総合.md
  8. docs/codebase-map/結合ポイントマップ.md
  9. docs/codebase-map/ユースケース逆引き.md
  10. docs/codebase-map/modules/*.md
  11. docs/codebase-map/modules/潜在バグ一覧.md
  12. internal/application/orchestrator/distributed_orchestrator.go
  13. internal/application/orchestrator/distributed_orchestrator_routes.go
  14. internal/application/orchestrator/distributed_orchestrator_test.go

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語を含める。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は今回の対象外として触らない。

作成する文書:
  - docs/refactor/Phase20_distributed_transport_executor境界整理実装仕様.md

文書に必ず含める内容:
  1. Phase20 の目的
  2. 対象範囲
  3. 対象外
  4. 現在の transport execution 構造
  5. 提案する `distributedTransportExecutor`
  6. collaborator 契約
  7. 実装手順
  8. 検証手順
  9. リスク
  10. 完了条件

必ず明記する既存契約:
  - `executeToAgent` は `receiveOnAgent = msg.From` で mailbox 実行に委譲する。
  - SSH transport があれば SSH 経路を使い、なければ local router 経路を使う。
  - `mailbox.sent` / `mailbox.waiting` / `mailbox.received` / `mailbox.error` / `agent.error` event を維持する。
  - SSH / local とも受信 result を CentralMemory に記録する。
  - SSH / local とも MessageTypeError は `agent <from> returned error: <content>` として返す。
  - local receive transport は receiveOnAgent、なければ mio に fallback する。両方なければ error。
  - timeout は `distributedWaitTimeout` 相当を使う。
  - fallback を正常系として扱わない。

検証手順:
  - baseline / after:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw`
  - transport focused:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase20|TestDistributedWaitTimeout|TestDistributedOrchestrator_.*(Retry|CODE|LocalRoute)'`
  - diff:
    `git diff --check`
    `git diff --stat`

実行手順:
  1. 参照文書と対象コードを読む。
  2. transport execution の SSH/local 分岐、timeout、event、memory record、error 契約を棚卸しする。
  3. `distributedTransportExecutor` の契約を書く。
  4. docs/refactor/Phase20_distributed_transport_executor境界整理実装仕様.md を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
