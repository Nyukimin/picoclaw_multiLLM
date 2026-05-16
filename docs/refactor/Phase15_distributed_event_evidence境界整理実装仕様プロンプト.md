# Phase15 distributed event / evidence 境界整理実装仕様作成プロンプト

```md
Goal:
  RenCrow のリファクタリング Phase15 として、DistributedOrchestrator の event emission と evidence report 保存を collaborator 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase15: distributed event / evidence 境界整理

目的:
  - `DistributedOrchestrator.emit` / `emitNote` / `emitProgress` を distributed event port として整理する。
  - `saveExecutionReport` と distributed evidence helper を evidence reporter として整理する。
  - Viewer event、transport log、execution report、response text を混同しない。
  - route dispatch / TTS lifecycle / transport executor には踏み込まない。

必ず参照するもの:
  1. docs/refactor/Phase14_DistributedOrchestrator残責務棚卸し.md
  2. docs/refactor/Phase11_完了判定.md
  3. docs/refactor/Phase13_MessageOrchestrator残責務棚卸し.md
  4. internal/application/orchestrator/distributed_orchestrator.go
  5. internal/application/orchestrator/distributed_orchestrator_test.go
  6. internal/domain/execution/report.go
  7. internal/domain/transport/

作成する文書:
  - docs/refactor/Phase15_distributed_event_evidence境界整理実装仕様.md

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は対象外として触らない。

文書に必ず含める内容:
  1. Phase15 の目的
  2. 対象範囲
  3. 対象外
  4. 現在の event / evidence 構造
  5. 提案する collaborator
     - `distributedEventPort`
     - `distributedEvidenceReporter`
  6. 各 collaborator の契約
  7. 実装手順
  8. テスト方針
  9. リスク
  10. 完了条件

実行手順:
  1. 参照文書と distributed orchestrator 周辺テストを読む。
  2. event / evidence の境界を棚卸しする。
  3. `docs/refactor/Phase15_distributed_event_evidence境界整理実装仕様.md` を作成する。
  4. コード変更は行わない。
```
