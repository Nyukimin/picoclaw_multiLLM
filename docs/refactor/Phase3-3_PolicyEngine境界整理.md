# Phase3-3 PolicyEngine 境界整理

## 目的

Phase 3-3 では、PolicyEngine を policy decision の純粋な境界として整理する。

PolicyEngine は `execution.Action` を受け取り、`execution.PolicyDecision` を返す。tool 実行の副作用、Worker patch 実行、Coder proposal の valid / invalid 判定は持たない。

## 対象範囲

- `internal/infrastructure/security/policy_engine.go`
- `internal/infrastructure/security/policy_runner.go`
- `internal/domain/execution/`
- policy 関連 tests

## 対象外

- policy rule の意味変更。
- deny / allow の新規ルール追加。
- ToolRunner の実処理変更。
- WorkerExecutionService の protected file check 変更。
- Viewer JS / CSS。
- runtime config の意味変更。

## 現在の責務

PolicyEngine:

- shell deny command を判定する。
- workspace 外 file_write を判定する。
- network scope を判定する。
- allow / deny、reason、matched rule を返す。

PolicyRunner:

- inner `RunnerV2` の tool metadata を確認する。
- `execution.Action` を作る。
- `execution.Service.RequestToolExecution` を通じて policy と実行を接続する。
- denied action を permission error response に変換する。

## 提案する分離単位

- `evaluateShellPolicy`
- `evaluateWorkspacePolicy`
- `evaluateNetworkPolicy`
- `allowDecision`

Phase 3-3 では policy の意味を変えず、`Evaluate` の判定順を読みやすくするだけに留める。

## 入力

- `execution.Action`
- `PolicyConfig`
- tool name
- tool args

## 出力

- `execution.PolicyDecision`
- PolicyRunner 経由では `tool.ToolResponse`

## 副作用

- PolicyEngine 単体は副作用を持たない。
- PolicyRunner / execution.Service は record 保存と inner runner 実行を行う。

## 永続化

- PolicyEngine 単体は永続化しない。
- PolicyRunner は execution repository に record を保存する。

## ログ

- policy decision は execution record として扱う。
- Viewer event や Worker log と混同しない。

## エラー契約

- deny shell signature は `deny.shell.signature`。
- workspace 外 file_write deny は `deny.workspace.outside`。
- network blocked は `deny.network.blocked`。
- allowlist で host がない場合は `deny.network.host.missing`。
- allowlist 外 host は `deny.network.host.not_allowlisted`。
- default allow は `allow.default`。
- PolicyRunner の denied action は permission error response。

## 変更してはいけない既存挙動

- 判定順。
- decision / reason / matched rule。
- default profile。
- network scope の fallback。
- unknown tool の error。

## 実装手順

1. baseline test を実行する。
2. PolicyEngine の判定ブロックを小関数へ分ける。
3. policy rule の意味、reason、matched rule を変更しない。
4. PolicyRunner の副作用境界は維持する。
5. gofmt を実行する。
6. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/security ./internal/domain/execution ./internal/infrastructure/tools ./cmd/picoclaw
git diff --check
```

## リスク

- policy 判定順を変える。
- deny reason / matched rule を変える。
- PolicyEngine に tool 実行副作用を入れる。
- WorkerExecutionService の protected file check と混同する。
- ToolRunner に policy 判断を戻す。

## 完了条件

- PolicyEngine が action -> decision の責務として読める。
- PolicyRunner が policy + runner + repository の adapter として読める。
- policy rule の意味が変わっていない。
- 対象テストが成功している。
