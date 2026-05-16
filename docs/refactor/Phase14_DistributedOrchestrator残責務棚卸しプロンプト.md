# Phase14 DistributedOrchestrator 残責務棚卸しプロンプト

```md
Goal:
  RenCrow のリファクタリング Phase14 として、DistributedOrchestrator の残責務を棚卸しし、分散実行側の分割計画を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase14: DistributedOrchestrator 残責務棚卸し

目的:
  - `DistributedOrchestrator` に集中している session、routing、event、TTS、route dispatch、autonomous execution、transport、evidence の責務を棚卸しする。
  - MessageOrchestrator で分離した collaborator 方針をそのまま共有せず、分散実行固有の境界を整理する。
  - 次に実装する Phase15 の最小対象を決める。

必ず参照するもの:
  1. docs/refactor/Phase13_MessageOrchestrator残責務棚卸し.md
  2. docs/refactor/Phase8_完了判定.md
  3. docs/refactor/Phase9_完了判定.md
  4. docs/refactor/Phase10_完了判定.md
  5. docs/refactor/Phase11_完了判定.md
  6. docs/refactor/Phase12_完了判定.md
  7. docs/refactor/リファクタリング指針.md
  8. docs/refactor/段階移行計画.md
  9. docs/codebase-map/modules/application.md
  10. docs/codebase-map/modules/潜在バグ一覧.md
  11. internal/application/orchestrator/distributed_orchestrator.go
  12. internal/application/orchestrator/distributed_orchestrator_test.go
  13. internal/domain/transport/
  14. internal/infrastructure/transport/

作成する文書:
  - docs/refactor/Phase14_DistributedOrchestrator残責務棚卸し.md

制約:
  - この作業では文書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は対象外として触らない。

文書に必ず含める内容:
  1. Phase14 の目的
  2. DistributedOrchestrator の現在責務
  3. MessageOrchestrator と共通する責務
  4. DistributedOrchestrator 固有の責務
  5. 分けてよい候補
     - distributed session lifecycle
     - distributed event port
     - distributed TTS lifecycle
     - distributed route dispatcher
     - distributed autonomous coordinator
     - distributed transport executor
     - distributed evidence reporter
  6. まだ分けない方がよい候補
  7. Phase15 のおすすめ
  8. 検証方針
  9. 完了条件

実行手順:
  1. Phase13 と DistributedOrchestrator 周辺コードを読む。
  2. 責務を棚卸しする。
  3. Phase15 のおすすめを決める。
  4. `docs/refactor/Phase14_DistributedOrchestrator残責務棚卸し.md` を作成する。
  5. コード変更は行わない。
```
