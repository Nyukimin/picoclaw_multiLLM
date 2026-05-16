# Phase13 MessageOrchestrator 残責務棚卸しプロンプト

```md
Goal:
  RenCrow のリファクタリング Phase13 として、Phase8-12 で collaborator 化した MessageOrchestrator の残責務を棚卸しし、次の大きなリファクタリング対象を判断するための文書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase13: MessageOrchestrator 残責務棚卸し

目的:
  - `MessageOrchestrator` が top-level orchestration として十分に薄くなったか確認する。
  - Phase8-12 で分離した collaborator の責務境界を一覧化する。
  - `MessageOrchestrator` に残すべき責務と、次に移すべき候補を区別する。
  - Phase14 以降のおすすめ対象を提案する。

必ず参照するもの:
  1. docs/refactor/Phase8_完了判定.md
  2. docs/refactor/Phase9_完了判定.md
  3. docs/refactor/Phase10_完了判定.md
  4. docs/refactor/Phase11_完了判定.md
  5. docs/refactor/Phase12_完了判定.md
  6. docs/refactor/リファクタリング指針.md
  7. docs/refactor/段階移行計画.md
  8. docs/codebase-map/modules/application.md
  9. internal/application/orchestrator/message_orchestrator.go
  10. internal/application/orchestrator/message_orchestrator*.go

作成する文書:
  - docs/refactor/Phase13_MessageOrchestrator残責務棚卸し.md

制約:
  - この作業では文書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は対象外として触らない。

文書に必ず含める内容:
  1. Phase13 の目的
  2. Phase8-12 で分離した collaborator 一覧
  3. `MessageOrchestrator` に残っている責務
  4. 残してよい責務
  5. これ以上分けない方がよい責務
  6. 次 Phase 候補
     - distributed orchestrator 整理
     - CodeExecutor / WorkerExecutionService 連携再確認
     - Viewer / IdleChat 境界確認
     - README / 実装仕様更新前の総合検証
  7. おすすめ Phase14
  8. 検証済みコマンド
  9. 完了条件

実行手順:
  1. Phase8-12 完了判定を読む。
  2. 現在の MessageOrchestrator 周辺コードを確認する。
  3. 残責務を棚卸しする。
  4. `docs/refactor/Phase13_MessageOrchestrator残責務棚卸し.md` を作成する。
  5. コード変更は行わない。
  6. 最後に作成ファイル、判断、次のおすすめを報告する。
```
