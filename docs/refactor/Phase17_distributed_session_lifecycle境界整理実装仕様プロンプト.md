# Phase17 distributed session lifecycle 境界整理実装仕様作成プロンプト

Goal:
  RenCrow のリファクタリング Phase17 として、DistributedOrchestrator の session load / create / save を collaborator 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase17: distributed session lifecycle 境界整理

目的:
  - `DistributedOrchestrator` に残る session load / create / task add / save を、分散実行専用の collaborator へ分離する。
  - session 永続化境界を `SessionRepository` へ閉じる。
  - `ProcessMessage` の top-level orchestration を薄くする。
  - Phase15 event / evidence 境界、Phase16 TTS lifecycle 境界を維持する。
  - MessageOrchestrator の `messageSessionLifecycle` と安易に共通化せず、分散側の既存契約を固定する。

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
  10. docs/refactor/Phase15_完了判定.md
  11. docs/refactor/Phase16_完了判定.md
  12. docs/codebase-map/アーキテクチャ総合.md
  13. docs/codebase-map/結合ポイントマップ.md
  14. docs/codebase-map/ユースケース逆引き.md
  15. docs/codebase-map/modules/*.md
  16. docs/codebase-map/modules/潜在バグ一覧.md
  17. internal/application/orchestrator/distributed_orchestrator.go
  18. internal/application/orchestrator/message_orchestrator_session.go
  19. internal/application/orchestrator/distributed_orchestrator_test.go

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
  - docs/refactor/Phase17_distributed_session_lifecycle境界整理実装仕様.md

文書に必ず含める内容:

1. Phase17 の目的
   - DistributedOrchestrator の session lifecycle だけを整理すること。
   - route dispatch、transport executor、TTS lifecycle、event/evidence には踏み込まないこと。

2. 対象範囲
   - `DistributedOrchestrator.ProcessMessage` の session load / create / task add / save。
   - `DistributedOrchestrator.loadOrCreateSession`
   - `SessionRepository`
   - distributed session focused tests。

3. 対象外
   - session policy の仕様変更。
   - MessageOrchestrator 側の session lifecycle 変更。
   - route dispatch / transport / TTS / event / evidence。
   - provider / Viewer / IdleChat / runtime config。

4. 現在の session lifecycle 構造
   - load error は種類を問わず新規 session にする既存挙動。
   - route execution 成功後だけ `sess.AddTask(t)` と `sessionRepo.Save` を行う。
   - save error は `[DistributedOrch] ProcessMessage ERROR: failed to save session:` としてログに残し、`failed to save session` error を返す。

5. 提案する collaborator
   - `distributedSessionLifecycle`
   - private struct とし、初期段階では interface 化しない。
   - MessageOrchestrator の `messageSessionLifecycle` と共通化しない理由を書く。

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
   - `distributedSessionLifecycle` を追加する。
   - `DistributedOrchestrator` に field を追加する。
   - constructor で組み立てる。
   - `ProcessMessage` の load / save を collaborator へ委譲する。
   - `loadOrCreateSession` を移すか委譲にする。
   - gofmt を実行する。
   - focused test と全体 test を実行する。
   - `docs/refactor/Phase17_完了判定.md` を作成する。

8. 検証手順
   - baseline / after:
     `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw`
   - session focused:
     `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase17|TestDistributedOrchestrator_ProcessMessage_(LocalRoute|SavesEvidenceOnSuccess|SavesEvidenceOnChatError)'`
   - 差分確認:
     `git diff --check`
     `git diff --stat`

9. リスク
   - load error 全般を新規 session にする既存挙動を、ErrSessionNotFound 限定へ変えてしまう。
   - route execution 失敗時に session save してしまう。
   - save error のログ / error wrapping を変えてしまう。
   - MessageOrchestrator 側と無理に共通化する。

10. 完了条件
   - 実装仕様書が docs/refactor/ に作成されている。
   - current lifecycle、分離単位、契約、実装手順、検証手順、リスクが書かれている。
   - コード変更は行っていない。
   - ユーザーが次に実装してよいか判断できる。

実行手順:
  1. 参照文書と対象コードを読む。
  2. DistributedOrchestrator の session lifecycle を棚卸しする。
  3. `distributedSessionLifecycle` の契約を書く。
  4. docs/refactor/Phase17_distributed_session_lifecycle境界整理実装仕様.md を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
