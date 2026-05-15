# Phase2-2 route dispatch 関数分割

## 目的

Phase 2-2 では、`MessageOrchestrator` の route dispatch を route ごとの private function に分ける。

Chat / Worker / Coder の差を隠す汎用 handler は作らず、各 route の実行先を関数名で追える状態にする。

## 対象範囲

- `executeTask`
- `executeRouteDirect`
- route-specific private function

## 対象外

- `ProcessMessage` の手順整理。
- `DefaultCodeExecutor` の Coder 選択。
- `WorkerExecutionService` の内部実行方式。
- Viewer JS / CSS。
- STT / TTS provider。

## 守る契約

- CHAT は `MioAgent.Chat`。
- OPS は `ShiroAgent.Execute`。
- CODE / CODE1 / CODE2 / CODE3 / CODE4 は `executeCodeViaShiro` から `CodeExecutor.ExecuteCode`。
- PLAN / RESEARCH は `MioAgent.Chat`。
- ANALYZE は Heavy があれば `HeavyAgent.Generate`、なければ `MioAgent.Chat`。
- WILD は `WildAgent.Generate`。Wild agent がなければ error。
- unknown route / unsupported autonomous route は error。
- `agent.start` / `agent.response` / TTS push / stream hook の契約を変えない。

## 実装方針

- `executeTask` は CHAT と autonomous route への入口に限定する。
- `executeRouteDirect` は switch と route-specific function call のみに近づける。
- route-specific function は `executeChatRoute`、`executeOPSRoute`、`executeCodeRoute`、`executePlanRoute`、`executeAnalyzeRoute`、`executeResearchRoute`、`executeWildRoute` とする。
- 共通化は最小限にし、Chat / Worker / Coder の差を隠さない。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

## 完了条件

- route ごとの実行先が private function 名で追える。
- Coder proposal と Worker execution の境界が維持されている。
- fallback / unknown route が成功扱いされていない。
- Phase 2-0 の契約テストが成功している。
