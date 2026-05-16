# Phase21 distributed code execution 境界整理実装仕様作成プロンプト

Goal:
  RenCrow のリファクタリング Phase21 として、DistributedOrchestrator の Code route execution / coder selection / coder retry を distributed code execution 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase21: distributed code execution 境界整理

目的:
  - `executeCodeViaShiro` に集中している coder selection、Coder 依頼、Worker 実行、retry instruction 生成を分散 Code 専用 collaborator へ分離する。
  - coder config、proposal payload、worker retryable failure、event / note、CentralMemory record の既存契約を維持する。
  - node selection helper と transport executor 本体には踏み込まない。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/Phase19_完了判定.md
  6. docs/refactor/Phase20_完了判定.md
  7. docs/codebase-map/アーキテクチャ総合.md
  8. docs/codebase-map/結合ポイントマップ.md
  9. docs/codebase-map/ユースケース逆引き.md
  10. docs/codebase-map/modules/*.md
  11. docs/codebase-map/modules/潜在バグ一覧.md
  12. internal/application/orchestrator/distributed_orchestrator.go
  13. internal/application/orchestrator/distributed_orchestrator_routes.go
  14. internal/application/orchestrator/distributed_orchestrator_transport.go
  15. internal/application/orchestrator/distributed_orchestrator_test.go

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語を含める。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は今回の対象外として触らない。

作成する文書:
  - docs/refactor/Phase21_distributed_code_execution境界整理実装仕様.md

文書に必ず含める内容:
  1. Phase21 の目的
  2. 対象範囲
  3. 対象外
  4. 現在の Code route execution 構造
  5. 提案する `distributedCodeExecutionCoordinator`
  6. collaborator 契約
  7. 実装手順
  8. 検証手順
  9. リスク
  10. 完了条件

必ず明記する既存契約:
  - coder が見つからない場合は `no coder mapped for route %s` を返す。
  - attempt 0 は通常依頼、attempt > 0 は `worker.retry_request` と retry note を出す。
  - coder message context に `route` / `retry_attempt` / `channel` / `chat_id` を入れる。
  - coder config があれば `coder_config` を入れる。
  - coder message / shiro task / exec message を CentralMemory に記録する。
  - coder mailbox は receiveOnAgent `mio` を使う。
  - coderResult.Proposal nil の場合は Shiro 整形へ回す。
  - Proposal がある場合は Shiro Worker 実行へ回す。
  - retryable failure は `buildCoderRetryInstruction` を使って次 attempt に進む。
  - retry budget exhausted error を維持する。

検証手順:
  - baseline / after:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw`
  - code focused:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase21|TestDistributedOrchestrator_.*(CODE|Retry)|TestDistributedExecutionErrorClassification'`
  - diff:
    `git diff --check`
    `git diff --stat`

実行手順:
  1. 参照文書と対象コードを読む。
  2. Code route execution の coder selection、message context、retry、proposal / worker path を棚卸しする。
  3. `distributedCodeExecutionCoordinator` の契約を書く。
  4. docs/refactor/Phase21_distributed_code_execution境界整理実装仕様.md を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
