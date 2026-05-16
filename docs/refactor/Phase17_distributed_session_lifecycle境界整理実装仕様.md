# Phase17 distributed session lifecycle 境界整理実装仕様

## 1. Phase17 の目的

Phase17 は、`DistributedOrchestrator` に残っている session load / create / task add / save を `distributedSessionLifecycle` という private collaborator 境界へ整理する段階である。

目的は次の通り。

- session 永続化境界を `SessionRepository` へ閉じる。
- `ProcessMessage` の top-level orchestration を薄くする。
- Phase15 event / evidence 境界と Phase16 TTS lifecycle 境界を維持する。
- route dispatch、transport executor、TTS lifecycle、event/evidence には踏み込まない。
- MessageOrchestrator の `messageSessionLifecycle` と安易に共通化せず、分散側の既存契約を固定する。

Phase17 は構造整理であり、session policy の仕様変更ではない。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/distributed_orchestrator.go`
  - `ProcessMessage` の session load / create。
  - `ProcessMessage` の `sess.AddTask(t)` と `sessionRepo.Save`。
  - `loadOrCreateSession`
- 新規追加する `distributedSessionLifecycle`
- `SessionRepository`
- distributed session focused tests。

## 3. 対象外

Phase17 では次を対象外にする。

- session policy の仕様変更。
- MessageOrchestrator 側の session lifecycle 変更。
- distributed route dispatcher 分割。
- distributed transport executor 分割。
- distributed autonomous coordinator 分割。
- event / evidence 境界の追加変更。
- TTS lifecycle の追加変更。
- node selection。
- coder config。
- handler / DTO / SSE event / Viewer JS / CSS。
- IdleChat。
- STT / TTS provider。
- LLM provider。
- runtime config。
- 未追跡の `tests/`。

## 4. 現在の session lifecycle 構造

### load / create

`ProcessMessage` の冒頭で `loadOrCreateSession(ctx, req.SessionID, req.Channel, req.ChatID)` を呼ぶ。

現在の `loadOrCreateSession` は、`sessionRepo.Load` が error を返した場合、error の種類を問わず `session.NewSession(id, channel, chatID)` を返す。

契約:

- 既存 session が読めた場合はその session を使う。
- load error が返った場合は新規 session を作る。
- load error を `ProcessMessage` error として返さない。
- `[DistributedOrch] Session loaded/created:` ログを維持する。

### task add / save

route execution 成功後、TTS end 後に次を行う。

- `sess.AddTask(t)`
- `sessionRepo.Save(ctx, sess)`

契約:

- route execution が失敗した場合は completed task として保存しない。
- save error は `[DistributedOrch] ProcessMessage ERROR: failed to save session:` としてログに残す。
- save error は `failed to save session: ...` として caller に返す。

## 5. 提案する collaborator

### `distributedSessionLifecycle`

`distributedSessionLifecycle` は private struct とする。初期段階では interface 化しない。

配置:

- `internal/application/orchestrator/distributed_orchestrator_session.go` に定義する。
- `DistributedOrchestrator` に field として持たせる。
- `NewDistributedOrchestrator` で組み立てる。

dependency:

- `SessionRepository`

MessageOrchestrator の `messageSessionLifecycle` と共通化しない理由:

- MessageOrchestrator は `session.ErrSessionNotFound` の場合だけ新規 session を作る。
- DistributedOrchestrator は load error の種類を問わず新規 session を作る既存挙動を持つ。
- Phase17 は挙動変更をしないため、分散側専用 collaborator として固定する。

## 6. `distributedSessionLifecycle` の契約

入力:

- `context.Context`
- `ProcessMessageRequest`
- `*session.Session`
- `task.Task`

出力:

- `LoadForRequest` は `*session.Session` と error を返す。
- `SaveCompletedTask` は error を返す。

副作用:

- session load。
- session の task history 更新。
- session save。
- log 出力。

永続化:

- `SessionRepository.Load`
- `SessionRepository.Save`

ログ:

- load / create 成功: `[DistributedOrch] Session loaded/created:`
- save error: `[DistributedOrch] ProcessMessage ERROR: failed to save session:`

エラー契約:

- load error は新規 session に変換し、caller error にしない。
- save error は wrapping して caller に返す。

変更してはいけない既存挙動:

- load error 全般を新規 session にすること。
- route execution 成功後だけ task add / save すること。
- save error のログ prefix。
- save error の wrapping message。

## 7. 実装手順

1. baseline test を実行する。
2. `distributedSessionLifecycle` を `distributed_orchestrator_session.go` に追加する。
3. `DistributedOrchestrator` に `sessions *distributedSessionLifecycle` field を追加する。
4. `NewDistributedOrchestrator` で lifecycle を組み立てる。
5. `ProcessMessage` の session load を `sessions.LoadForRequest` へ置き換える。
6. `ProcessMessage` の task add / save を `sessions.SaveCompletedTask` へ置き換える。
7. `loadOrCreateSession` の処理を collaborator へ移す。
8. 既存のログ prefix と error message を変えない。
9. gofmt を実行する。
10. focused test と全体 test を実行する。
11. `docs/refactor/Phase17_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

session focused:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase17|TestDistributedOrchestrator_ProcessMessage_(LocalRoute|SavesEvidenceOnSuccess|SavesEvidenceOnChatError)'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- load error 全般を新規 session にする既存挙動を、`session.ErrSessionNotFound` 限定へ変えてしまう。
- route execution 失敗時に session save してしまう。
- save error のログ / error wrapping を変えてしまう。
- MessageOrchestrator 側と無理に共通化する。
- session save と evidence save を混同する。

## 10. 完了条件

Phase17 の完了条件は次の通り。

- `docs/refactor/Phase17_distributed_session_lifecycle境界整理実装仕様.md` が作成されている。
- 現在の distributed session lifecycle 構造が棚卸しされている。
- `distributedSessionLifecycle` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- load error 全般を新規 session にする既存挙動を維持する方針が明記されている。
- コード変更は行っていない。
