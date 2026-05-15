# Phase2 完了判定

## 対象

Phase 2 は `MessageOrchestrator` の Chat / Worker / Coder route chain を明確化する Phase とする。

対象は `ProcessMessage`、route decision、route dispatch、response assembly、Viewer event、execution report、TTS / VTuber hook、fallback / error route 契約である。

## 実施した Phase

- Phase 2-0: route chain 契約固定。
- Phase 2-1: `ProcessMessage` 内部ステップ命名。
- Phase 2-2: route dispatch 関数分割。
- Phase 2-3: response assembly 分離。
- Phase 2-4: Viewer event / report / TTS hook 契約確認。
- Phase 2-5: fallback / error route 契約固定。
- Phase 2-6: 完了判定。

## 作成した文書

- `docs/refactor/Phase2_route_chain明確化実装仕様.md`
- `docs/refactor/Phase2-0_route_chain契約固定.md`
- `docs/refactor/Phase2-1_ProcessMessage内部ステップ命名.md`
- `docs/refactor/Phase2-2_route_dispatch関数分割.md`
- `docs/refactor/Phase2-3_response_assembly分離.md`
- `docs/refactor/Phase2-4_event_report_tts契約確認.md`
- `docs/refactor/Phase2-5_fallback_error_route契約固定.md`
- `docs/refactor/Phase2_完了判定.md`

## 実装で変更したファイル

- `internal/application/orchestrator/message_orchestrator.go`
- `internal/application/orchestrator/message_orchestrator_route_chain_contract_test.go`

## 完了状態

`MessageOrchestrator.ProcessMessage` は次の手順として読める状態になった。

1. session load。
2. `message.received` event。
3. pre-routing chat command。
4. task / job / TTS session ID assembly。
5. route decision。
6. TTS session start。
7. route execution。
8. TTS session end。
9. session save。
10. response assembly。

route dispatch は route-specific private function に分離した。

- `executeChatRoute`
- `executeOPSRoute`
- `executeCodeRoute`
- `executePlanRoute`
- `executeAnalyzeRoute`
- `executeResearchRoute`
- `executeWildRoute`

response assembly は副作用のない helper に分離した。

- `buildProcessMessageResponse`
- `buildChatCommandResponse`

## 守られている境界

- Chat route は `MioAgent.Chat`。
- OPS route は `ShiroAgent.Execute`。
- CODE / CODE1 / CODE2 / CODE3 / CODE4 route は `executeCodeViaShiro` から `CodeExecutor.ExecuteCode`。
- Coder proposal は `CodeExecutor` 内で検証し、invalid proposal は WorkerExecutionService に渡さない。
- PLAN / RESEARCH route は `MioAgent.Chat`。
- ANALYZE route は Heavy があれば `HeavyAgent.Generate`、なければ `MioAgent.Chat`。
- WILD route は `WildAgent.Generate`。Wild agent がなければ error。
- Viewer event、execution report、TTS / VTuber hook は別の責務として扱う。
- fallback / unknown route / invalid proposal を正常系 response にすり替えない。

## 検証

Phase 2 の完了確認では次を実行する。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
git status --short
```

`git status --short` では、今回の対象外である未追跡 `tests/` を除き、未コミット差分がないことを確認する。

## 完了条件の判定

- route chain の主要手順が private function 名で追える。
- Chat / Worker / Coder の責務境界を崩していない。
- WorkerExecutionService、ToolRunner、PolicyEngine、LLM provider、STT / TTS provider、Viewer JS / CSS は変更していない。
- handler、DTO、SSE event、IdleChat 契約は変更していない。
- fallback を正常系として扱っていない。
- Viewer 表示、音声、口パク、ログを混同していない。
- 対象パッケージのテストが成功している。
- Phase 2 の文書と実装差分は commit / push 済みである。

上記を満たすため、Phase 2 は完了と判定する。

## Phase 3 前の確認事項

Phase 3 に進む前に、次の対象をどれにするか決める。

- WorkerExecutionService と ToolRunner / PolicyEngine の境界整理。
- CodeExecutor と Coder selection の責務整理。
- Viewer adapter 側の event 表示契約整理。
- Memory / Source Registry の adapter 境界整理。
