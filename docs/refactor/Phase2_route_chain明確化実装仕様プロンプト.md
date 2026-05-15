# Phase2 route chain 明確化実装仕様プロンプト

```text
Goal:
RenCrow のリファクタリング Phase 2「Chat / Worker / Coder route chain の明確化」を最後まで実装するための実装仕様書を作成してください。

Repository:
- /home/nyukimi/picoclaw_multiLLM

作成する文書:
- docs/refactor/Phase2_route_chain明確化実装仕様.md

目的:
- MessageOrchestrator を中心に、Chat / Worker / Coder の route 判断、dispatch、response assembly の流れを明確にする。
- Chat / Worker / Coder の責務境界をコード上で追いやすくする。
- Coder proposal 生成と Worker 実行の境界を明文化する。
- fallback を正常系として扱わず、エラーまたは安全側の経路として明確に扱う。
- Viewer event、session、report、TTS hook を落とさず、表示・音声・口パク・ログを混同しない。
- モジュール化と疎結合を最重要方針として、route ごとの入力、出力、event、error contract を整理する。

現在の前提:
- Phase 1「cmd/picoclaw の composition root 整理」は完了済み。
- Phase 1 完了判定は docs/refactor/Phase1_完了判定.md に記録済み。
- cmd/picoclaw/main.go は composition root として薄くなっている。
- 未追跡の tests/ は今回の対象外として触らない。
- この作業では実装仕様書だけを作成し、コード変更はしない。

必ず参照するもの:
1. AGENTS.md
2. CLAUDE.md
3. docs/01_正本仕様/実装仕様.md
4. docs/refactor/リファクタリング指針.md
5. docs/refactor/フォルダ構成方針.md
6. docs/refactor/段階移行計画.md
7. docs/refactor/検証方針.md
8. docs/refactor/Phase1_完了判定.md
9. docs/codebase-map/アーキテクチャ総合.md
10. docs/codebase-map/結合ポイントマップ.md
11. docs/codebase-map/ユースケース逆引き.md
12. docs/codebase-map/modules/application.md
13. docs/codebase-map/modules/domain.md
14. docs/codebase-map/modules/adapter.md
15. docs/codebase-map/modules/infrastructure.md
16. docs/codebase-map/modules/潜在バグ一覧.md
17. internal/application/orchestrator/message_orchestrator.go
18. internal/application/service/worker_execution_service.go
19. internal/domain/routing または routing 関連 domain
20. internal/domain/task
21. internal/domain/proposal
22. internal/domain/execution
23. internal/adapter/viewer 関連 handler / event
24. 関連する *_test.go

docs/codebase-map/ の使い方:
- 実装前の一次解析資料として使う。
- MessageOrchestrator、WorkerExecutionService、Viewer event、LLM provider、ToolRunner / PolicyEngine などの結合点を確認する。
- ただし正本仕様ではない。
- 実装判断で矛盾がある場合は docs/01_正本仕様/実装仕様.md を優先する。
- codebase-map と現在コードが違う場合は、現在コードを確認し、仕様書に差分リスクとして記録する。
- docs/archive/ は一次参照にしない。

制約:
- この作業では実装仕様書だけを作成する。
- コード変更はしない。
- docs/refactor/ 配下の Markdown 追加だけにする。
- ファイル名は日本語にする。
- 実装仕様の正本は docs/01_正本仕様/実装仕様.md のままとする。
- TODO / TBD の仮置きは残さない。
- 未追跡の tests/ は今回の対象外として触らない。
- Coder に実行責務を戻さない。
- Worker の安全境界を薄めない。
- fallback を正常成功として扱わない。
- Viewer 表示、音声、口パク、ログを混同しない。
- route 分岐を共通化しすぎて Chat / Worker / Coder の差を隠さない。
- いきなり大規模実装を始める内容にしない。段階実装できる仕様書にする。

文書に必ず含める内容:

1. Phase 2 の目的
   - Chat / Worker / Coder route chain を明確にすること。
   - route 判断、dispatch、response assembly を読みやすくすること。
   - Coder proposal 生成と Worker 実行の境界を守ること。
   - fallback を正常系にしないこと。

2. Phase 2 の対象範囲
   - MessageOrchestrator.ProcessMessage。
   - route 判断。
   - route dispatch。
   - Chat response assembly。
   - Worker task execution handoff。
   - Coder proposal generation handoff。
   - Viewer event emission。
   - session / report / TTS hook との接続。
   - route ごとの入力、出力、event、error contract。

3. 対象外
   - WorkerExecutionService の内部実行方式の変更。
   - ToolRunner / PolicyEngine の実装変更。
   - LLM provider の挙動変更。
   - Viewer JS / CSS の変更。
   - STT / TTS provider の変更。
   - IdleChat 契約変更。
   - persistence schema 変更。
   - route 名や外部 API 契約の意味変更。
   - fallback を成功表示にする変更。

4. 現在コードの把握
   - MessageOrchestrator.ProcessMessage の現在の流れ。
   - route 判定に使われる入力。
   - Chat / Worker / Coder / ANALYZE / OPS などの route 分岐。
   - Viewer event が発火する場所。
   - TTS hook が呼ばれる場所。
   - execution report が作られる場所。
   - session ID / job ID が作られる場所。
   - fallback または安全側 route が使われる場所。

5. Phase 2 の分割案
   - Phase 2-0: 現状 route chain の契約固定。
   - Phase 2-1: MessageOrchestrator.ProcessMessage の内部ステップ命名。
   - Phase 2-2: route ごとの dispatch 関数分割。
   - Phase 2-3: response assembly の分離。
   - Phase 2-4: Viewer event / report / TTS hook の契約確認。
   - Phase 2-5: fallback / error route の扱い固定。
   - Phase 2-6: Phase 2 完了判定。

6. 各小 Phase の仕様
   各小 Phase について以下を書く:
   - 目的。
   - 対象範囲。
   - 対象外。
   - 入力。
   - 出力。
   - 副作用。
   - 永続化。
   - ログ。
   - event 契約。
   - error 契約。
   - 変更してはいけない既存挙動。
   - 実装手順。
   - 検証手順。
   - 完了条件。

7. Chat / Worker / Coder の責務境界
   - Chat:
     - ユーザー対話。
     - route 判断。
     - 結果返却。
     - 実行詳細や破壊的操作を抱え込まない。
   - Worker:
     - 実行。
     - file edit / command / test / git など。
     - Coder が生成した plan / patch の適用。
     - 実行結果とログの記録。
   - Coder:
     - 設計。
     - plan / patch 生成。
     - 破壊的操作を直接実行しない。

8. route ごとの契約
   少なくとも以下について書く:
   - CHAT
   - PLAN
   - ANALYZE
   - OPS
   - RESEARCH
   - CODE
   - CODE1
   - CODE2
   - CODE3
   - safety fallback / unknown route

   各 route について:
   - 入力。
   - 出力。
   - dispatch 先。
   - Viewer event。
   - session / job ID。
   - report。
   - TTS hook。
   - error handling。
   - fallback handling。

9. fallback / error 方針
   - fallback を正常成功として扱わない。
   - route fallback は安全側経路であり、成功表示の代替ではない。
   - invalid route、empty response、provider error、worker error、coder proposal error を区別する。
   - Viewer には成功・失敗・保留・安全側遷移が区別できる event / log を残す。

10. モジュール化と疎結合の方針
    - 単に関数を分けるだけではモジュール化ではない。
    - route 判断、dispatch、response assembly、event emission、reporting を責務単位で分ける。
    - 共通化は意味のある契約がある場合だけ行う。
    - 「似ているからまとめる」だけの共通化は禁止する。
    - 巨大 service / manager / helper / util を新設しない。
    - interface、contract、event、DTO、adapter の境界を明確にする。
    - Chat / Worker / Coder の差を隠す抽象化を避ける。

11. テスト方針
    - まず既存テストを確認する。
    - route ごとの unit test / integration test を優先する。
    - ProcessMessage の分岐を変更する場合は、route 別に入力と期待結果を固定する。
    - Coder proposal と Worker execution の境界が崩れていないことを検証する。
    - fallback / error route が成功扱いになっていないことを検証する。
    - Viewer event / session / report / TTS hook が落ちていないことを検証する。
    - Viewer が関係する場合は最低 1 セッションを追う確認を入れる。

12. 検証手順
    原則:
    - GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
    - git diff --check
    - git diff --stat

    route / Viewer event に触った場合:
    - 関連 event 名を rg で確認する。
    - 必要に応じて Viewer で最低 1 セッション確認する。

    runtime config に触った場合:
    - ~/.picoclaw/config.yaml を確認する。
    - http://127.0.0.1:18790/health を確認する。

13. リスク
    - route 分岐を共通化しすぎて責務差が見えなくなる。
    - Coder に実行責務が戻る。
    - Worker の安全境界が薄くなる。
    - fallback が成功扱いになる。
    - Viewer event / report / TTS hook が落ちる。
    - session ID / job ID の単位が混ざる。
    - route 名と実行責務がずれる。
    - テストだけ通って実セッションが壊れる。

14. Phase 2 の最終完了条件
    - MessageOrchestrator の route chain が読みやすくなっている。
    - Chat / Worker / Coder の責務境界がコード上で追える。
    - route ごとの入力、出力、event、error contract が文書化されている。
    - Coder proposal と Worker execution の境界が崩れていない。
    - fallback が正常成功として扱われていない。
    - Viewer event / session / report / TTS hook が維持されている。
    - 対象テストが成功している。
    - 必要な場合、Viewer で最低 1 セッション確認している。
    - 各小 Phase の文書と実装差分が Push 済み。
    - ユーザーが Phase 3「Worker execution 安全境界の分離」に進むか判断できる。

実行手順:
1. 参照文書を読む。
2. docs/codebase-map/ で結合点と潜在バグを確認する。
3. MessageOrchestrator と WorkerExecutionService 周辺を読む。
4. 現在の route chain を整理する。
5. Phase 2 を小 Phase に分ける。
6. 各小 Phase の仕様、検証、完了条件を書く。
7. docs/refactor/Phase2_route_chain明確化実装仕様.md を作成する。
8. コード変更は行わない。
9. TODO / TBD を残さない。
10. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
