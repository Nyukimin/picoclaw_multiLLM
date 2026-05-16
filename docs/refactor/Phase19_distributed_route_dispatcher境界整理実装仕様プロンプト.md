# Phase19 distributed route dispatcher 境界整理実装仕様作成プロンプト

Goal:
  RenCrow のリファクタリング Phase19 として、DistributedOrchestrator の route dispatch 分岐を distributed route dispatcher 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase19: distributed route dispatcher 境界整理

目的:
  - `executeDistributed` / `executeDistributedDirect` に残る route 分岐を、分散実行専用 collaborator へ分離する。
  - remote / local agent dispatch の接続点を明確にする。
  - transport executor、Code route retry、node selection、attribution guard 本体には踏み込まない。
  - Phase15-18 で分けた event / evidence / TTS / session / autonomous coordinator 境界を維持する。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/Phase9_route_dispatcher境界整理実装仕様.md
  6. docs/refactor/Phase14_DistributedOrchestrator残責務棚卸し.md
  7. docs/refactor/Phase18_完了判定.md
  8. docs/codebase-map/アーキテクチャ総合.md
  9. docs/codebase-map/結合ポイントマップ.md
  10. docs/codebase-map/ユースケース逆引き.md
  11. docs/codebase-map/modules/*.md
  12. docs/codebase-map/modules/潜在バグ一覧.md
  13. internal/application/orchestrator/distributed_orchestrator.go
  14. internal/application/orchestrator/message_orchestrator_routes.go
  15. internal/application/orchestrator/distributed_orchestrator_test.go

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語を含める。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は今回の対象外として触らない。

作成する文書:
  - docs/refactor/Phase19_distributed_route_dispatcher境界整理実装仕様.md

文書に必ず含める内容:
  1. Phase19 の目的
  2. 対象範囲
  3. 対象外
  4. 現在の route dispatch 構造
  5. 提案する `distributedRouteDispatcher`
  6. collaborator 契約
  7. 実装手順
  8. 検証手順
  9. リスク
  10. 完了条件

必ず明記する既存契約:
  - CHAT は autonomous coordinator を通さず direct execution に入る。
  - 非 CHAT は autonomous coordinator を通る。
  - CODE route は `executeCodeViaShiro` 相当へ委譲し、成功時に user response event / note / TTS push を行う。
  - WILD / ANALYZE heavy は stream hook と finalize を維持する。
  - local CHAT は `withAttributionGuard`、CentralMemory record、Mio.Chat、response record、event / note / TTS finalize を維持する。
  - remote agent route は `routeToAgent`、`withAttributionGuard`、transport message context、memory record、`executeToAgent`、response event / note / TTS push を維持する。
  - transport executor 本体と Code route retry は変更しない。

検証手順:
  - baseline / after:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw`
  - route focused:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase19|TestDistributedOrchestrator_.*(LocalRoute|CODE|OPS|Retry|TTSBridge)'`
  - diff:
    `git diff --check`
    `git diff --stat`

実行手順:
  1. 参照文書と対象コードを読む。
  2. `executeDistributed` / `executeDistributedDirect` の route 分岐と依存を棚卸しする。
  3. `distributedRouteDispatcher` の契約を書く。
  4. docs/refactor/Phase19_distributed_route_dispatcher境界整理実装仕様.md を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
