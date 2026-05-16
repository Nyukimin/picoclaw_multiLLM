# Phase18 distributed autonomous coordinator 境界整理実装仕様作成プロンプト

Goal:
  RenCrow のリファクタリング Phase18 として、DistributedOrchestrator の autonomous execution request assembly / observe / execute / verify callback を collaborator 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase18: distributed autonomous coordinator 境界整理

目的:
  - `executeAutonomousDistributed` に残る `autonomousapp.RunExecutor` request assembly を、分散実行専用 collaborator へ分離する。
  - retry message、verify contract、execution steps、failure kind の既存契約を維持する。
  - route direct execution は引き続き `DistributedOrchestrator.executeDistributedDirect` に委譲し、route dispatcher 分割には踏み込まない。
  - Phase15 event / evidence、Phase16 TTS lifecycle、Phase17 session lifecycle を維持する。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/フォルダ構成方針.md
  6. docs/refactor/段階移行計画.md
  7. docs/refactor/検証方針.md
  8. docs/refactor/Phase8_MessageOrchestrator_collaborator化実装仕様.md
  9. docs/refactor/Phase14_DistributedOrchestrator残責務棚卸し.md
  10. docs/refactor/Phase17_完了判定.md
  11. docs/codebase-map/アーキテクチャ総合.md
  12. docs/codebase-map/結合ポイントマップ.md
  13. docs/codebase-map/ユースケース逆引き.md
  14. docs/codebase-map/modules/*.md
  15. docs/codebase-map/modules/潜在バグ一覧.md
  16. internal/application/orchestrator/distributed_orchestrator.go
  17. internal/application/orchestrator/message_orchestrator_autonomous.go
  18. internal/application/autonomous/executor.go
  19. internal/application/orchestrator/distributed_orchestrator_test.go

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語を含める。
  - 実装仕様の正本は docs/01_正本仕様/実装仕様.md のままとする。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は今回の対象外として触らない。

作成する文書:
  - docs/refactor/Phase18_distributed_autonomous_coordinator境界整理実装仕様.md

文書に必ず含める内容:
  1. Phase18 の目的
  2. 対象範囲
  3. 対象外
  4. 現在の autonomous execution 構造
  5. 提案する `distributedAutonomousCoordinator`
  6. collaborator 契約
     - 入力
     - 出力
     - 副作用
     - 永続化
     - ログ
     - エラー契約
     - 変更してはいけない既存挙動
  7. 実装手順
  8. 検証手順
  9. リスク
  10. 完了条件

必ず明記する既存契約:
  - `contractapp.NormalizeRequestWithRoute` を維持する。
  - `autonomousapp.RunExecutor` を維持する。
  - `ReportStore: o.reporter` 相当を維持する。
  - `MaxRepair: o.maxRepairOrDefault()` 相当を維持する。
  - observe stage は `entry.stage` event を emit する。
  - attempt > 0 のときだけ `buildExecutorRetryMessage` を使う。
  - execute は `executeDistributedDirect` 相当へ委譲する。
  - verify は `verifyByContract` を使う。
  - RunExecutor が error を返した場合でも `result.Response` を返す。
  - route dispatch / transport executor は変更しない。

検証手順:
  - baseline / after:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw`
  - autonomous focused:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase18|TestDistributedOrchestrator_.*(CODE|OPS|Retry|Evidence)|TestPhase8AutonomousExecutionCoordinatorUsesUpdatedReportStore'`
  - diff:
    `git diff --check`
    `git diff --stat`

実行手順:
  1. 参照文書と対象コードを読む。
  2. `executeAutonomousDistributed` の callback 契約を棚卸しする。
  3. `distributedAutonomousCoordinator` の契約を書く。
  4. docs/refactor/Phase18_distributed_autonomous_coordinator境界整理実装仕様.md を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
