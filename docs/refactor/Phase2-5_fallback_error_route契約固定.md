# Phase2-5 fallback / error route 契約固定

## 目的

Phase 2-5 では、fallback、unknown route、proposal invalid、agent missing を正常系として扱わない契約を固定する。

Phase 2 の route chain 明確化では、失敗を成功 response に丸めず、どの段階で止まったかを error と event / report から追えることを優先する。

## 対象範囲

- `executeTask` の unknown route error。
- `executeRouteDirect` の unsupported autonomous route error。
- `executeWildRoute` の wild agent missing error。
- `executeCodeViaShiro` から `CodeExecutor` へ渡る proposal invalid error。
- contract test による Worker 未到達確認。

## 対象外

- routing classifier の fallback route 選択。
- Coder / Worker の内部実行方式。
- autonomous executor の repair policy。
- LLM provider の retry / fallback。
- Viewer JS / CSS の error 表示。
- STT / TTS provider の error handling。

## 守る契約

- unknown route は success response を返さない。
- unsupported autonomous route は success response を返さない。
- wild agent がない WILD route は error とする。
- invalid proposal は WorkerExecutionService へ渡さない。
- fallback 文字列は retry prompt の欠損補完に限定し、route 成功扱いの根拠にしない。
- error が返った route は `ProcessMessage` で `task execution failed` として上位へ伝える。

## 実装方針

- 既存の error path を変更しない。
- 契約テストで unknown route が `agent.response` を出さず error になることを固定する。
- 既存の invalid proposal contract test を維持する。
- fallback を正常系 response に変換する helper は追加しない。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

## 完了条件

- fallback / unknown route / invalid proposal が成功扱いされないことが文書化されている。
- unknown route の error 契約がテストで固定されている。
- invalid proposal が WorkerExecutionService に到達しない契約テストが成功している。
- Phase 2 の対象パッケージテストが成功している。
