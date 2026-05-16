# Phase23 完了判定

## Phase の目的

Phase23 は `DistributedOrchestrator` に残っていた `withAttributionGuard` / `buildAttributionGuardedMessage` を、分散 attribution guard 専用の `distributedAttributionGuard` へ分離する。

目的は構造整理であり、guard 文面、CentralMemory、route dispatcher、transport executor、Code execution、event / evidence、TTS lifecycle、session lifecycle、autonomous coordinator、provider、Viewer、IdleChat、runtime config の挙動は変更しない。

## 実装した境界

- `distributedAttributionGuard`
  - 入力: task、target agent、session ID
  - 出力: task
  - 副作用: なし。CentralMemory は読み取りだけ
  - 永続化: なし
  - ログ: なし
  - エラー契約: error は返さず、guard に使える memory がない場合は元 task を返す

## 維持した既存挙動

- targetAgent 空、Code route、既に guard がある user message では変更しない。
- memory unified view は 120 件見る。
- session ID が違う、content 空、IdleChat message、idle- session は除外する。
- selfLines / otherLines はそれぞれ最大 3 件。
- self / other が空の場合は `なし` を入れる。
- guard 文面と `【ユーザー依頼】` section を維持する。
- Task の JobID / Channel / ChatID / ForcedRoute / Route を維持する。

## 変更ファイル

- `internal/application/orchestrator/distributed_orchestrator.go`
- `internal/application/orchestrator/distributed_orchestrator_attribution.go`
- `internal/application/orchestrator/distributed_orchestrator_phase23_attribution_test.go`
- `docs/refactor/Phase23_完了判定.md`

## 検証

Phase23 の最終確認では次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase23|TestDistributedOrchestrator_AttributionGuardOnUserChat'
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

## 完了条件

- distributed attribution guard の詳細が `distributedAttributionGuard` へ分離されている。
- `DistributedOrchestrator` 本体は attribution guard の構築と委譲だけを持つ。
- metadata preservation、skip 条件、IdleChat 除外がテストで固定されている。
- Phase23 の検証コマンドが成功している。
- Phase23 の文書と実装差分が Push 済みである。

## 次の候補

次は Phase24 として、`DistributedOrchestrator` に残る pure helper / wrapper / top-level composition の棚卸しを行い、Phase1 全体の完了判定に進めるか確認する。
