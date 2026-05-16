# Phase14 DistributedOrchestrator 残責務棚卸し

## 1. Phase14 の目的

Phase14 は、`DistributedOrchestrator` に集中している責務を棚卸しし、分散実行側の分割計画を作成する判断フェーズである。

Phase8-13 で `MessageOrchestrator` は top-level orchestration として説明できる状態になった。一方で `DistributedOrchestrator` は、分散 transport、node selection、mailbox retry、evidence、TTS、event、route dispatch、autonomous execution を 1 ファイル内に多く抱えている。

この Phase ではコード変更を行わず、Phase15 の最小実装対象を決める。

## 2. DistributedOrchestrator の現在責務

現在の `DistributedOrchestrator` は次を担当している。

- dependency holding。
- distributed timeout / retry default 解決。
- node capability / coder config の保持。
- event emission。
- session load / create / save。
- `ProcessMessage` top-level orchestration。
- Mio route decision。
- route notice event。
- task / job ID / TTS session ID assembly。
- TTS session start / end / stream hook / push。
- IdleChat busy guard。
- distributed route dispatch。
- autonomous execution。
- remote transport execution。
- mailbox / SSH / local router execution。
- distributed Code route retry。
- distributed evidence report 保存。
- attribution guard。

## 3. MessageOrchestrator と共通する責務

MessageOrchestrator と共通する責務は次の通り。

- session lifecycle。
- response assembly。
- route decision。
- idle busy guard。
- task context build。
- event emission。
- TTS lifecycle。
- autonomous execution。
- route dispatch。

ただし、これらを MessageOrchestrator の collaborator と無理に共有しない。分散側は transport / node / mailbox / remote timeout が絡むため、まず distributed 専用 collaborator として切り出す。

## 4. DistributedOrchestrator 固有の責務

DistributedOrchestrator 固有の責務は次の通り。

- `transport.MessageRouter` を使う remote / local agent dispatch。
- `domaintransport.Transport` を使う SSH transport。
- `CentralMemory` への distributed message 記録。
- `NodeSelector` と node resource profile による coder selection。
- coder config を remote message context に含める処理。
- mailbox 経由の retry / timeout。
- distributed evidence report。
- distributed attribution guard。
- remote result / proposal payload の扱い。

これらは通常の `MessageOrchestrator` と同じ粒度で切ると境界が崩れるため、分散固有の collaborator として設計する。

## 5. 分けてよい候補

### distributed session lifecycle

session load / create / save を分離する候補。

価値:

- `ProcessMessage` の前後処理を薄くできる。

注意:

- 現在の `loadOrCreateSession` は load error をすべて新規 session にしているため、正本仕様との関係を確認してから変更する。

### distributed event port

`emit` / `emitNote` / `emitProgress` を分離する候補。

価値:

- Viewer event と distributed execution log を分けやすい。

注意:

- 現在の distributed `emit` は listener nil で log を出さない。MessageOrchestrator の `messageEventPort` と挙動が違うため、安易に共通化しない。

### distributed TTS lifecycle

TTS start / end / stream hook / push を分離する候補。

価値:

- Phase10 の考え方を分散側にも適用できる。

注意:

- distributed では TTS start error 時に `ttsSessionID = ""` にしている。MessageOrchestrator と挙動が違うため、共通化せず分散側契約として固定する。

### distributed route dispatcher

`executeDistributed` / `executeDistributedDirect` を分離する候補。

価値:

- remote dispatch / local chat / code route / wild / heavy / default agent dispatch の責務が見えやすくなる。

注意:

- transport / memory / TTS / event / attribution guard が絡むため、いきなり切ると大きすぎる。

### distributed autonomous coordinator

`executeAutonomousDistributed` を分離する候補。

価値:

- retry / verify / RunExecutor request assembly を route dispatch から分けられる。

注意:

- route direct executor と distributed route dispatcher の結合が強いため、先に event / TTS / evidence を整理した方が安全。

### distributed transport executor

`executeToAgent` / mailbox / SSH execution 周辺を分離する候補。

価値:

- remote execution の技術詳細を route dispatcher から分けられる。

注意:

- timeout、retry、mailbox、SSH、router が絡むため、契約テストを先に厚くする必要がある。

### distributed evidence reporter

`saveExecutionReport` と distributed evidence helper を分離する候補。

価値:

- evidence report と Viewer event / execution response を分離できる。
- error / success status の契約を明確化できる。

注意:

- report store nil や goal/jobID 空の no-op 条件を維持する。

## 6. まだ分けない方がよい候補

現時点でまだ分けない方がよいものは次の通り。

- distributed route dispatcher 全体。
  - 先に event / TTS / evidence / session の小さい境界を固定した方が安全。
- transport executor 全体。
  - mailbox / SSH / router / timeout の契約テストが不足している。
- NodeSelector 周辺。
  - route dispatch と coder config に絡むため、分散 route dispatcher 分割後に扱う方がよい。

## 7. Phase15 のおすすめ

おすすめは Phase15: distributed event / evidence 境界整理である。

理由:

- 分散側は Viewer event、distributed execution report、transport log が混ざりやすい。
- event / evidence を先に分けると、その後の TTS lifecycle、route dispatcher、autonomous coordinator 分割時に観測契約を壊しにくい。
- MessageOrchestrator でも event port 化が Phase11 で有効だったため、分散側でも同じ考え方を使える。ただし挙動は安易に共通化しない。

Phase15 の最小対象は次に限定する。

- `emit`
- `emitNote`
- `emitProgress`
- `saveExecutionReport`
- distributed evidence helper

対象外:

- route dispatch。
- TTS lifecycle。
- transport executor。
- node selection。

## 8. 検証方針

Phase15 以降で実装する場合は、baseline / after として次を実行する。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestDistributed'
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

route chain / CodeExecutor に触った場合は、MessageOrchestrator 側の契約テストも回す。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
```

## 9. 完了条件

Phase14 の完了条件は次の通り。

- DistributedOrchestrator の現在責務が棚卸しされている。
- MessageOrchestrator と共通する責務が整理されている。
- DistributedOrchestrator 固有の責務が整理されている。
- 分けてよい候補と、まだ分けない方がよい候補が区別されている。
- Phase15 のおすすめが明記されている。
- コード変更は行っていない。
- 未追跡の `tests/` は触っていない。
