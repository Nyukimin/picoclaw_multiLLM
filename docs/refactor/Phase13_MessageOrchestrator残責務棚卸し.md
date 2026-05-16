# Phase13 MessageOrchestrator 残責務棚卸し

## 1. Phase13 の目的

Phase13 は、Phase8-12 で collaborator 化した `MessageOrchestrator` 周辺責務を棚卸しし、`MessageOrchestrator` が top-level orchestration として十分に薄くなったかを確認する判断フェーズである。

この Phase ではコード変更を行わず、次の大きなリファクタリング対象を決める。

## 2. Phase8-12 で分離した collaborator

Phase8-12 で分離した collaborator は次の通り。

- `messageResponseAssembler`
  - `ProcessMessageResponse` の組み立て。
  - chat command response の組み立て。
- `messageSessionLifecycle`
  - session load / create / save。
  - session 永続化境界。
- `preRoutingCommandHandler`
  - route decision 前の chat command handling。
  - handled command の route decision bypass。
- `routeDecisionCoordinator`
  - Mio route decision。
  - `routing.decision` event emission。
- `idleBusyGuardFactory`
  - IdleChat activity / chat busy / worker busy guard。
- `autonomousExecutionCoordinator`
  - autonomous executor request assembly。
  - retry / verify / report store 連携。
- `messageRouteDispatcher`
  - CHAT と non-CHAT route の分岐。
  - route-specific execution。
  - CodeExecutor handoff。
- `messageTTSLifecycle`
  - TTS session start / end。
  - stream hook。
  - TTS / VTuber push。
- `messageEventPort`
  - Viewer event emission。
  - nil listener skipped log。
  - `message.received` event。
- `messageTaskContextBuilder`
  - task / job ID / attachment event / TTS session ID assembly。

## 3. `MessageOrchestrator` に残っている責務

現在 `MessageOrchestrator` に残っている責務は次の通り。

- public request / response / interface 定義。
- dependency holding。
- constructor による collaborator assembly。
- public setter。
- `ProcessMessage` の top-level orchestration。
- maxRepair の default 解決。
- TTS enabled 判定。
- backward compatible な `emit` / `emitMessageReceived` thin wrapper。

`ProcessMessage` に残っている流れは次の通り。

1. start log。
2. chat busy guard。
3. session load / create。
4. `message.received` event。
5. pre-routing command。
6. task context build。
7. route decision。
8. route を task に反映。
9. TTS session start。
10. worker busy guard。
11. route execution。
12. TTS session end。
13. session save。
14. completion log。
15. response assembly。

この流れは top-level orchestration として説明できる。

## 4. 残してよい責務

次は `MessageOrchestrator` に残してよい。

- `ProcessMessage` の順序制御。
- collaborator の組み立て。
- public setter による後注入。
- public interface と request / response 型。
- route chain の大枠を読める程度の top-level flow。

理由は、ここをさらに細かく分けると処理順序が見えにくくなり、Phase6 で固定した route chain 契約の追跡性が下がるためである。

## 5. これ以上分けない方がよい責務

次は現時点では追加分割しない。

- `ProcessMessage` の順序制御。
  - 分割しすぎると session、routing、TTS、execution、save の順序が見えにくくなる。
- constructor の collaborator assembly。
  - まだ external DI へ移していないため、composition root としてここに残す方が安全。
- public setter。
  - 既存 adapter / main wiring との互換性を保つ必要がある。
- `emit` / `emitMessageReceived` thin wrapper。
  - 互換性のため残してよい。実体は `messageEventPort` に移っている。

## 6. 次 Phase 候補

次の候補は複数ある。

### 候補 A: distributed orchestrator 整理

`DistributedOrchestrator` には、MessageOrchestrator と似た route dispatch、TTS、event、autonomous execution の責務が残っている。

利点:

- MessageOrchestrator で得た collaborator 分割方針を横展開できる。
- 分散実行 route chain の見通しがよくなる。

注意点:

- remote transport / node selection / retry / timeout が絡むため、一気に分割しない。
- MessageOrchestrator と同じ collaborator を無理に共有しない。

### 候補 B: CodeExecutor / WorkerExecutionService 連携再確認

Phase6 で契約テストは固定済みだが、Phase8-12 後に CODE route の境界を再確認する。

利点:

- Coder / Worker 境界の安全性をさらに強められる。

注意点:

- Phase4 / Phase5 で既に大きく整理済みのため、今すぐ追加分割する優先度は distributed orchestrator より低い。

### 候補 C: Viewer / IdleChat 境界確認

Viewer event、IdleChat、TTS、口パクの境界を横断確認する。

利点:

- ユーザー観測に直結する。

注意点:

- ブラウザ確認や live runtime が必要になりやすい。
- route chain の構造整理とは別 Phase にした方がよい。

### 候補 D: README / 実装仕様更新前の総合検証

実装に合わせて仕様書や README を更新する前に、総合テストと現状整理を行う。

利点:

- 文書更新の準備になる。

注意点:

- まだ distributed orchestrator に大きな責務集中が残っているため、最終文書化には早い。

## 7. おすすめ Phase14

おすすめは Phase14: `DistributedOrchestrator` の残責務棚卸しと分割計画である。

理由:

- MessageOrchestrator は top-level orchestration として説明できる状態になった。
- 一方で `DistributedOrchestrator` には route dispatch、event、TTS、autonomous execution、remote transport が集中している。
- 次に実装へ入る前に、まず分散側の責務を棚卸しし、MessageOrchestrator と同じ分割を適用できる部分と、分散固有として残す部分を分ける必要がある。

Phase14 はいきなり実装せず、まず実装仕様用プロンプトと実装仕様を作成する。

## 8. 検証済みコマンド

Phase8-12 の各実装で、主に次を実行済みである。

```bash
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator -run 'TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator -run 'TestPhase10|TestMessageOrchestrator_TTSBridge_'
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator -run 'TestPhase12|TestPhase11|TestMessageOrchestrator_RouteChainContract_'
git diff --check
```

Phase13 は文書判断フェーズのためコード変更は行っていない。

## 9. 完了条件

Phase13 の完了条件は次の通り。

- Phase8-12 で分離した collaborator が一覧化されている。
- `MessageOrchestrator` に残っている責務が棚卸しされている。
- 残してよい責務と、これ以上分けない方がよい責務が区別されている。
- 次 Phase 候補が複数提示されている。
- おすすめ Phase14 が明記されている。
- コード変更は行っていない。
- 未追跡の `tests/` は触っていない。
