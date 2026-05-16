# Phase23 distributed attribution guard 境界整理実装仕様作成プロンプト

Goal:
  RenCrow のリファクタリング Phase23 として、DistributedOrchestrator の attribution guard 生成を distributed attribution guard 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase23: distributed attribution guard 境界整理

目的:
  - `withAttributionGuard` / `buildAttributionGuardedMessage` を分散 attribution guard 専用 collaborator へ分離する。
  - CentralMemory unified view、IdleChat 除外、既存 guard 文面、task metadata 継承を維持する。
  - route dispatcher、transport executor、Code execution には踏み込まない。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/Phase19_完了判定.md
  6. docs/refactor/Phase22_完了判定.md
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
  - docs/refactor/Phase23_distributed_attribution_guard境界整理実装仕様.md

文書に必ず含める内容:
  1. Phase23 の目的
  2. 対象範囲
  3. 対象外
  4. 現在の attribution guard 構造
  5. 提案する `distributedAttributionGuard`
  6. collaborator 契約
  7. 実装手順
  8. 検証手順
  9. リスク
  10. 完了条件

必ず明記する既存契約:
  - targetAgent 空、Code route、既に guard がある user message では変更しない。
  - memory unified view は 120 件見る。
  - session ID が違う、content 空、IdleChat message、idle- session は除外する。
  - selfLines / otherLines はそれぞれ最大 3 件。
  - self / other が空の場合は `なし` を入れる。
  - guard 文面と `【ユーザー依頼】` section を維持する。
  - Task の JobID / Channel / ChatID / ForcedRoute / Route を維持する。

検証手順:
  - baseline / after:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw`
  - attribution focused:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase23|TestDistributedOrchestrator_AttributionGuardOnUserChat'`
  - diff:
    `git diff --check`
    `git diff --stat`

実行手順:
  1. 参照文書と対象コードを読む。
  2. attribution guard の除外条件、memory source、guard 文面、task metadata 継承を棚卸しする。
  3. `distributedAttributionGuard` の契約を書く。
  4. docs/refactor/Phase23_distributed_attribution_guard境界整理実装仕様.md を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
