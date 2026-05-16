# Phase16 distributed TTS lifecycle 境界整理実装仕様作成プロンプト

Goal:
  RenCrow のリファクタリング Phase16 として、DistributedOrchestrator の TTS / VTuber lifecycle を collaborator 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase16: distributed TTS lifecycle 境界整理

目的:
  - `DistributedOrchestrator` に残る TTS session start / end / stream hook / final push を、分散実行専用の collaborator へ分離する。
  - 音声、口パク、Viewer 表示本文、event log、execution log を混同しない。
  - TTS / VTuber degraded log を route execution success と混同しない。
  - Phase15 で整理した event / evidence 境界を維持する。
  - MessageOrchestrator の `messageTTSLifecycle` と安易に共通化せず、分散側の既存契約を固定する。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/フォルダ構成方針.md
  6. docs/refactor/段階移行計画.md
  7. docs/refactor/検証方針.md
  8. docs/refactor/Phase10_TTS_lifecycle境界整理実装仕様.md
  9. docs/refactor/Phase14_DistributedOrchestrator残責務棚卸し.md
  10. docs/refactor/Phase15_distributed_event_evidence境界整理実装仕様.md
  11. docs/refactor/Phase15_完了判定.md
  12. docs/codebase-map/アーキテクチャ総合.md
  13. docs/codebase-map/結合ポイントマップ.md
  14. docs/codebase-map/ユースケース逆引き.md
  15. docs/codebase-map/modules/*.md
  16. docs/codebase-map/modules/潜在バグ一覧.md
  17. internal/application/orchestrator/distributed_orchestrator.go
  18. internal/application/orchestrator/message_orchestrator_tts_lifecycle.go
  19. internal/application/orchestrator/tts_support.go
  20. internal/application/orchestrator/vtuber_stream.go
  21. internal/application/orchestrator/distributed_orchestrator_test.go

docs/codebase-map/ の使い方:
  - 実装前の一次解析資料として、対象周辺の責務、結合点、ユースケース、潜在バグを確認する。
  - 正本仕様ではない。
  - 矛盾がある場合は `docs/01_正本仕様/実装仕様.md` と現在コードを優先する。
  - docs/archive/ は一次参照にしない。

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語を含める。
  - 実装仕様の正本は docs/01_正本仕様/実装仕様.md のままとする。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は今回の対象外として触らない。
  - handler、DTO、SSE event、Viewer JS/CSS、IdleChat 契約、STT/TTS provider、LLM provider、runtime config の挙動は変更しない前提にする。

作成する文書:
  - docs/refactor/Phase16_distributed_TTS_lifecycle境界整理実装仕様.md

文書に必ず含める内容:

1. Phase16 の目的
   - DistributedOrchestrator の TTS / VTuber lifecycle だけを整理すること。
   - route dispatch、transport executor、autonomous coordinator には踏み込まないこと。
   - 分散側の start failure 契約を維持すること。

2. 対象範囲
   - `DistributedOrchestrator.ProcessMessage` の TTS start / end。
   - `DistributedOrchestrator.withStreamHooks`
   - `DistributedOrchestrator.pushTTS`
   - `SetTTSBridge`
   - `SetVTuberBridge`
   - distributed TTS focused tests。

3. 対象外
   - route dispatcher 分割。
   - transport executor 分割。
   - autonomous executor 分割。
   - event / evidence 境界の追加変更。
   - provider 実装変更。
   - Viewer / IdleChat / STT / runtime config。

4. 現在の TTS lifecycle 構造
   - start: route decision 後、route execution 前。
   - start error: `[DistributedOrch] TTS start degraded:` を出し、`ttsSessionID` を空にする。
   - end: route execution 成功後、session save 前。
   - stream hook: previous callback、`agent.thinking` event、TTS stream forwarder、VTuber stream forwarder を接続する。
   - push: final response を TTS / VTuber に push する。

5. 提案する collaborator
   - `distributedTTSLifecycle`
   - private struct とし、初期段階では interface 化しない。
   - MessageOrchestrator の `messageTTSLifecycle` と共通化しない理由を書く。

6. collaborator 契約
   以下を明記する:
   - 入力
   - 出力
   - 副作用
   - 永続化
   - ログ
   - エラー契約
   - 変更してはいけない既存挙動

7. 実装手順
   - baseline test を実行する。
   - `distributedTTSLifecycle` を追加する。
   - `DistributedOrchestrator` に field を追加する。
   - constructor で組み立てる。
   - `SetTTSBridge` / `SetVTuberBridge` で最新 bridge を反映する。
   - `ProcessMessage` の start / end を collaborator へ委譲する。
   - `withStreamHooks` / `pushTTS` を collaborator へ委譲する。
   - gofmt を実行する。
   - focused test と全体 test を実行する。
   - `docs/refactor/Phase16_完了判定.md` を作成する。

8. 検証手順
   - baseline / after:
     `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw`
   - TTS focused:
     `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase16|TestDistributedOrchestrator_TTSBridge_|TestTTSStreamForwarder'`
   - 差分確認:
     `git diff --check`
     `git diff --stat`

9. リスク
   - start failure 時に `ttsSessionID` を空にしなくなる。
   - EndSession を start failure 後に呼んでしまう。
   - previous stream callback を落とす。
   - `agent.thinking` event を落とす。
   - TTS chunk を Viewer 表示本文の根拠にしてしまう。
   - TTS / VTuber provider 挙動変更に踏み込む。
   - MessageOrchestrator 側と無理に共通化する。

10. 完了条件
   - 実装仕様書が docs/refactor/ に作成されている。
   - current lifecycle、分離単位、契約、実装手順、検証手順、リスクが書かれている。
   - コード変更は行っていない。
   - ユーザーが次に実装してよいか判断できる。

実行手順:
  1. 参照文書と対象コードを読む。
  2. DistributedOrchestrator の TTS / VTuber lifecycle を棚卸しする。
  3. `distributedTTSLifecycle` の契約を書く。
  4. docs/refactor/Phase16_distributed_TTS_lifecycle境界整理実装仕様.md を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
