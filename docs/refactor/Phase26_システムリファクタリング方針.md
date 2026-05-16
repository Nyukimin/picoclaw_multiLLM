# Phase26 システムリファクタリング方針

## 目的

Phase26 は、RenCrow の仕様と実装構造を照合し、仕様変更時に触る実装箇所を 1 対 1 に説明できる状態へ近づけるための方針である。

Phase1 から Phase25 では、`cmd/picoclaw/main.go`、MessageOrchestrator、CodeExecutor、DistributedOrchestrator に集中していた責務を段階的に分割した。Phase26 では、分割後に残った集中点を「システムとして」評価する。単に大きいファイルを小さくするのではなく、仕様、module、実装箇所、検証条件が対応しているかを確認する。

この方針では、コードが仕様より良い構造になっている場合は、コードを無理に動かさず仕様を更新することを許容する。逆に、仕様が明確であるのに実装が未分化な場合は、仕様に合わせて段階的にコードを分割する。

## 正本仕様との関係

実装判断の一次参照は `docs/01_正本仕様/実装仕様.md` とする。

`docs/10_新仕様/` は、Phase1 から Phase25 までのリファクタリング結果を現在構成として整理した補助仕様である。正本仕様と矛盾する場合は正本仕様を優先する。ただし、現在コードが正本仕様より具体的で、かつ責務境界として妥当な場合は、コードを残し、`docs/10_新仕様/` または正本仕様の更新候補として記録する。

`docs/codebase-map/` は一次解析資料として使う。結合点、ユースケース、潜在バグを確認するための地図であり、最終判断は正本仕様と現在コードで行う。`docs/archive/` は一次参照にしない。

## 判断原則

### 仕様を優先する場合

仕様を優先し、コードを仕様へ寄せる条件は次の通りである。

- 正本仕様に明確な責務境界がある。
- 現在コードが複数仕様を 1 箇所へ集約している。
- 仕様変更時に触る実装箇所が広がり、影響範囲を説明しにくい。
- fallback、Viewer 表示、音声、口パク、ログ、runtime config の扱いが仕様上の禁止事項と矛盾する。
- Chat / Worker / Coder の責務境界が実装上読みにくい。

この場合は、先に対象仕様の入力、出力、副作用、永続化、ログ、エラー契約を固定し、それに合わせてコードを分割する。

### コードを優先し、仕様を更新する場合

コードを優先し、仕様側を更新する条件は次の通りである。

- 現在コードが interface、contract、DTO、event、adapter、repository、provider の境界に沿って分かれている。
- 仕様文書が古く、存在しないファイルや過去の構成を参照している。
- コードの分割が差し替え可能性を高めている。
- コードを仕様へ合わせると、責務が逆に混ざる。
- 既存テストが現在コードの契約を固定しており、仕様より実装の方が具体的である。

この場合は、コードを動かさず、`docs/10_新仕様/` と `docs/refactor/` の実装箇所対応を更新する。

### 仕様とコードの両方を分割する場合

仕様もコードも粗い場合は、両方を分割する。

対象は、1 つの変更で 3 つ以上の主担当 package に広がるもの、または 1 ファイルが起動、handler、provider、usecase、永続化、ログを同時に抱えているものとする。

この場合は、先に仕様を小さい単位へ分け、次にコードを段階的に分割する。仕様変更と実装変更を同じ commit に混ぜず、設計文書、実装、検証の順に進める。

### いったん変更せず、リスクとして記録する場合

次の場合は、直ちにコード変更しない。

- 正本仕様との整合が確認できない。
- live runtime config、Viewer、IdleChat、STT/TTS、LLM provider の挙動変更を伴う。
- 現在テストが不足し、分割による回帰を検出できない。
- codebase-map と現在コードの差分が大きく、先に調査が必要である。
- 分割先の責務名、入力、出力、エラー契約を説明できない。

この場合は、`docs/refactor/` に差分リスクとして記録し、先に検証設計を追加する。

## 1 対 1 判定基準

各仕様は、次の項目を説明できる場合に「実装箇所と 1 対 1 に近い」と判定する。

| 判定項目 | 判定内容 |
| --- | --- |
| 主担当モジュール | 仕様変更時に最初に読む package が 1 つに定まる |
| 主担当ファイル | 実装変更の中心ファイルが 1 つ、または同一責務の小ファイル群に定まる |
| 入力 | 外部入力、内部 request、event、DTO が説明できる |
| 出力 | response、event、report、永続化結果が説明できる |
| 副作用 | file、network、process、SSE、audio、log などの副作用が列挙できる |
| 永続化 | DB、JSONL、log、memory、source registry の保存先と責務が説明できる |
| ログ | job_id、session_id、route、status、error の記録責務が説明できる |
| エラー契約 | timeout、nil dependency、empty response、invalid proposal、blocked operation を成功扱いしない |
| 差し替え境界 | interface、contract、event、DTO、adapter、provider、repository が説明できる |
| 検証方法 | unit、contract、integration、httptest、live e2e、browser e2e のどれで固定するか説明できる |

主担当が複数存在しても、adapter / application / domain / infrastructure の層分離として説明できる場合は許容する。許容できないのは、仕様単位ではなく実装都合で変更箇所が拡散している状態である。

## 1 対 1 で説明できない場合の分類

| 分類 | 判断 | 修正方針 |
| --- | --- | --- |
| 仕様が粗すぎる | 1 仕様の中に route、runtime wiring、provider、Viewer 表示、永続化が同居している | 仕様を usecase または責務単位へ分割する |
| 仕様が古い | 実在しないファイル、旧構成、過去の呼び名を参照している | コードを優先し、仕様文書を更新する |
| 実装が未分化 | 1 ファイルが複数仕様の実装中心になっている | 入力、出力、副作用、永続化、ログ、エラー契約ごとに分割計画を作る |
| 実装が仕様より良い | コードは層や contract に沿っているが、仕様が粗い | 実装を残し、仕様を現在コードへ合わせる |
| 複数仕様が混在している | 似ている処理を便利にまとめた結果、変更理由が複数ある | 共有処理を増やさず、意味のある責務単位へ分ける |
| 層分離が不足している | `cmd/` や adapter に usecase / provider / persistence の判断が漏れている | Application / Infrastructure / Adapter へ段階的に移す |

## 現在確認済みの要注意箇所

### `cmd/picoclaw/runtime_dependencies.go`

現在の状態:

- 約 1800 行の `cmd` 配下ファイルである。
- `Dependencies` の定義、依存組み立て、store、Viewer handler、IdleChat、distributed mode、local agent、SSH transport、coder setup、heartbeat、health handler、utility を含む。
- `buildDependencies`、`buildDistributedMode`、local agent 起動、IdleChat handler、coder setup が同居している。

1 対 1 で説明しにくい理由:

- `runtime dependency wiring` という 1 仕様に対して、実際には Viewer wiring、IdleChat wiring、distributed wiring、coder wiring、local agent runtime、health wiring が混在している。
- `cmd/` が composition root であることは正しいが、composition root の中で仕様単位を読みにくくしている。

仕様を変更すべき可能性:

- `docs/10_新仕様/モジュール構成仕様.md` の `runtime dependency wiring` を 1 仕様として扱うのは粗い。
- `runtime wiring` を次の仕様単位へ分けるべきである。
  - Viewer handler wiring
  - IdleChat runtime wiring
  - Distributed runtime wiring
  - Local agent runtime wiring
  - Coder provider wiring
  - Health / heartbeat wiring

コードを分割すべき可能性:

- 高い。
- ただし Application 層の usecase 本体へ踏み込まず、`cmd/picoclaw` 内の composition root helper として段階分割する。

推奨する次アクション:

1. `runtime_dependencies.go` の責務を仕様単位に分類する。
2. `Dependencies` struct の field を Viewer、IdleChat、Distributed、Agent、Health に分類する。
3. `cmd/picoclaw/runtime_viewer.go`、`runtime_idlechat.go`、`runtime_distributed.go`、`runtime_local_agents.go`、`runtime_coders.go` のような責務名の候補を設計する。
4. 実装前に `cmd/picoclaw/*_test.go` と Phase25 E2E で固定する route / runtime 契約を明記する。

### `cmd/picoclaw/health_runtime.go`

現在の状態:

- health / status / doctor CLI、execution stats、evidence summary、health service、local LLM health check、TTS debug URL 推論が同居している。
- `cmdHealth`、`cmdStatus`、`cmdDoctor`、`buildHealthService`、`buildLocalLLMHealthChecks` が同じファイルにある。

1 対 1 で説明しにくい理由:

- health command、status command、doctor command、runtime health service、local LLM health check、TTS debug inference は関連するが、変更理由が同一ではない。
- `health / status runtime helper` という仕様名では範囲が広い。

仕様を変更すべき可能性:

- 中程度。
- 新仕様では health を次の単位へ分けて書くべきである。
  - health HTTP contract
  - status CLI contract
  - doctor CLI contract
  - local LLM health check contract
  - TTS debug inference contract

コードを分割すべき可能性:

- 中程度。
- CLI command と runtime health builder を別ファイルへ分ける価値がある。
- ただし挙動変更を避け、まずファイル分離だけに留める。

推奨する次アクション:

1. `health_runtime.go` を command と builder に分類する。
2. `cmd/picoclaw/health_commands.go` と `cmd/picoclaw/health_service.go` 相当への分離手順を作る。
3. `/ready` を LLM server 一般仕様と誤解しない検証条件を残す。

### `internal/application/service/worker_execution_service.go`

現在の状態:

- Worker execution の中心実装である。
- proposal parse、execution summary、auto commit、sequential / parallel execution、file edit、shell、git、failure classification、protected file 判定、file operation helper を含む。

1 対 1 で説明しにくい理由:

- Worker execution 仕様としては一貫しているが、実装内部では実行制御、operation dispatch、file operation、shell operation、git operation、error classification、安全境界が同居している。
- 仕様が「Worker execution」として大きすぎる可能性がある。

仕様を変更すべき可能性:

- 高い。
- Worker execution を次の仕様へ分けるべきである。
  - proposal command parsing
  - execution orchestration
  - operation dispatch
  - file operation
  - shell operation
  - git operation
  - failure classification
  - execution summary / logging
  - protected file contract

コードを分割すべき可能性:

- 中から高。
- 現在は WorkerExecutionService という application usecase としてのまとまりは妥当であるため、先に仕様を分ける。
- 実装分割は、公開 interface を変えず、内部 helper ファイルへ切り出す段階移行がよい。

推奨する次アクション:

1. WorkerExecutionService の仕様を内部契約ごとに分割する。
2. `worker_execution_file_ops.go`、`worker_execution_shell.go`、`worker_execution_git.go`、`worker_execution_errors.go`、`worker_execution_summary.go` のような候補を設計する。
3. `WorkerExecutionService` interface は維持し、外部呼び出し側への影響を出さない。
4. protected file、workspace、failure classification の拒否ケースを先に検証する。

### `internal/application/orchestrator/distributed_orchestrator.go`

現在の状態:

- DistributedOrchestrator は Phase15 から Phase23 で event、evidence、TTS、session、autonomous、routes、transport、code、coder selection、attribution を分割済みである。
- それでも root file に constructor、setter、ProcessMessage、retry instruction、error classification、transport helper、route mapping、display helper、attribution entry が残っている。

1 対 1 で説明しにくい理由:

- top-level orchestration と retry / classification / display helper が同居している。
- 分割済みの構造は良いが、root file に残すべきものの仕様がまだ明確でない。

仕様を変更すべき可能性:

- 高い。
- コード分割済みの部分は仕様をコードへ合わせるべきである。
- root file に残す責務を `DistributedOrchestrator root contract` として明文化する必要がある。

コードを分割すべき可能性:

- 中程度。
- `buildCoderRetryInstruction`、`classifyDistributedExecutionError`、`proposalFailureKindFromText`、display helper は、retry / error contract の別ファイルへ移す候補である。
- ただし DistributedOrchestrator の public API と Phase15-23 の契約は維持する。

推奨する次アクション:

1. 先に仕様を更新し、root file に残すものを constructor、public setters、ProcessMessage entry に限定する方針を決める。
2. retry instruction と error classification を `distributed_orchestrator_retry.go` または `distributed_orchestrator_error.go` へ移す実装仕様を作る。
3. Phase15-23 の既存テストを baseline とする。

### `runtime_options.go` 記載と実ファイル不一致

方針作成時点の状態:

- `docs/10_新仕様/モジュール構成仕様.md` は `cmd/picoclaw/runtime_options.go` を記載している。
- 現在コードには `cmd/picoclaw/runtime_options.go` が存在しない。
- runtime option 相当は `runtime_dependencies.go`、`routes.go`、`stt_runtime_factory.go`、`llm_runtime_factory.go`、`health_runtime.go` に分散している。

Phase26 実施後の状態:

- `docs/10_新仕様/モジュール構成仕様.md` は、`cmd/picoclaw/runtime_options.go` を現行ファイルではなく将来の分離候補として記載している。
- 現行実装箇所は `runtime_dependencies.go`、`routes.go`、`stt_runtime_factory.go`、`llm_runtime_factory.go`、`health_runtime.go` として明記済みである。

1 対 1 で説明しにくい理由:

- 仕様が実在しない実装箇所を参照している。
- これはコードが悪いとは限らず、仕様が先行して未来構成を書いている可能性がある。

仕様を変更すべき可能性:

- 高い。
- 現状仕様としては `runtime_options.go` を既存実装のように書かない。
- 「候補」または「今後の分離先」として記載し直すべきである。

コードを分割すべき可能性:

- 中程度。
- Viewer runtime config、debug system options、LLM Ops options、STT stream URL などが分散しているため、将来的に `runtime_options.go` を作る価値はある。
- ただし先に仕様を正し、実装は別 Phase とする。

推奨する次アクション:

1. `docs/10_新仕様/モジュール構成仕様.md` の `runtime_options.go` を「現行ファイル」ではなく「分離候補」に直す。
2. runtime option の現行実装箇所を対応表に明記する。
3. `runtime_options.go` を作る場合は、Viewer 表示契約と live runtime config の意味を変えない実装仕様を先に作る。

### Viewer / IdleChat / STT / TTS / LLM provider / Memory / Source Registry

現在の状態:

- Viewer は `internal/adapter/viewer` と Viewer assets に分かれている。
- IdleChat は `internal/application/idlechat` と Viewer handler、TTS wiring がまたがる。
- STT は `cmd/picoclaw/stt_runtime_factory.go` と `internal/infrastructure/stt` に分かれている。
- TTS は `cmd/picoclaw/tts_runtime_factory.go` と `internal/infrastructure/tts` に分かれている。
- LLM provider は `cmd/picoclaw/llm_runtime_factory.go`、`internal/infrastructure/llm/factory`、`internal/infrastructure/llm/middleware` に分かれている。
- Memory / Source Registry は domain、persistence、application、Viewer handler に分かれている。

1 対 1 で説明しにくい理由:

- これらは runtime 上、adapter / application / infrastructure に分かれるべき仕様である。
- 単一ファイルへの 1 対 1 を求めると、Clean Architecture の層分離と矛盾する。

仕様を変更すべき可能性:

- 高い。
- 1 仕様 1 ファイルではなく、1 仕様 1 主担当 module と層ごとの補助実装として書くべきである。

コードを分割すべき可能性:

- 領域による。
- STT の `stt_runtime_factory.go` は大きく、runtime setup、websocket、audio payload、HTTP inference、timeout 調整が同居しているため分割候補である。
- Viewer / IdleChat / TTS / LLM provider / Memory は、現在の層分離を仕様へ反映することを優先する。

推奨する次アクション:

1. `docs/10_新仕様/モジュール構成仕様.md` の対応表を「主担当 module」と「層別実装箇所」に分ける。
2. STT runtime は別 Phase で詳細な分割仕様を作る。
3. Viewer / IdleChat / Audio は表示、音声、口パク、ログの契約を別々に検証する。

## 修正方針

### 仕様だけを直す対象

- `runtime_options.go` の現行ファイル扱い。
- DistributedOrchestrator の分割済み collaborator 構成。
- Viewer / IdleChat / STT / TTS / LLM provider / Memory / Source Registry の層別実装対応。

これらは、まず仕様を現在コードへ合わせる。コード変更は別 Phase で判断する。

### コードだけを直す対象

Phase26 時点では、コードだけを直す対象は定めない。仕様と実装対応の方針が先である。

コード変更が必要な場合も、必ず個別 Phase の実装仕様を作成してから進める。

### 仕様とコードの両方を直す対象

- `cmd/picoclaw/runtime_dependencies.go`
- `cmd/picoclaw/health_runtime.go`
- `internal/application/service/worker_execution_service.go`
- `cmd/picoclaw/stt_runtime_factory.go`

これらは、仕様が粗いことと実装が集中していることの両方がある。先に仕様を分け、次にコードを小さく分割する。

### 変更せずリスクとして記録する対象

- live runtime config の意味変更。
- Viewer 表示、音声、口パク、ログの挙動変更。
- IdleChat raw/view/audio 契約変更。
- LLM provider の fallback / timeout / empty response 契約変更。
- Memory / Source Registry の状態遷移変更。

これらは Phase26 の対象外であり、実装変更が必要になった場合は停止して別仕様化する。

## 段階移行計画

### Phase A: 仕様・実装対応表の確定

目的:

- `docs/10_新仕様/モジュール構成仕様.md` の対応表を、現行実装と矛盾しない形に直す。
- `runtime_options.go` の扱いを、現行ファイルではなく分離候補として明記する。
- 仕様変更対象ごとに、主担当 module、層別実装箇所、確認テストを分ける。

完了条件:

- 実在しないファイルを現行実装として参照していない。
- 主担当 module と補助実装箇所が分かれている。
- コード変更は行っていない。

### Phase B: 仕様が古い箇所の更新

目的:

- Phase1 から Phase25 の実装結果に合わせ、仕様側を更新する。
- DistributedOrchestrator、CodeExecutor、MessageOrchestrator の分割済み構造を新仕様へ反映する。

完了条件:

- コードの良い構造を仕様が正しく説明している。
- 仕様とコードの差分リスクが残る場合は明記されている。

### Phase C: 実装が未分化な箇所の分割

目的:

- `runtime_dependencies.go`、`health_runtime.go`、`worker_execution_service.go`、`stt_runtime_factory.go` を個別 Phase で分割する。

推奨順:

1. `runtime_dependencies.go` の仕様分類とファイル分離。
2. `health_runtime.go` の command / service 分離。
3. `worker_execution_service.go` の internal helper 分離。
4. `stt_runtime_factory.go` の runtime setup / websocket / audio payload / HTTP inference 分離。

完了条件:

- 各 Phase に実装仕様書がある。
- 公開 contract、route、DTO、SSE event、runtime config の意味を変えていない。
- 対象 package の test と必要な E2E が通っている。

### Phase D: 仕様と実装の再照合

目的:

- 分割後、仕様変更時に触る場所が狭まったか確認する。
- 1 対 1 判定表を更新する。

完了条件:

- 変更対象ごとの主担当 module が説明できる。
- 1 つの仕様変更が無秩序に複数 package へ広がっていない。

### Phase E: 組み合わせテストと E2E で確認

目的:

- Phase25 の組み合わせテスト設計に基づき、構造変更が主要フローを壊していないことを確認する。

完了条件:

- `GOCACHE=/tmp/picoclaw-gocache go test ./...` が成功している。
- 変更範囲に応じて `-tags=e2e` の代表テストを実行している。
- Viewer / IdleChat / STT / TTS に触れた場合は、実ブラウザまたは同等の E2E で最低 1 session を追っている。

## 禁止事項

- 仕様変更とコード変更を同じ根拠なしに混ぜない。
- 正本仕様を置き換えた扱いにしない。
- `cmd/picoclaw/main.go` から移した集中を別の巨大ファイルへ移すだけで終えない。
- 巨大な `service` / `manager` / `helper` / `util` を新設しない。
- 「便利だから共有する」「似ているからまとめる」だけの共通化をしない。
- fallback を正常系として扱わない。
- Viewer 表示、音声、口パク、ログを混同しない。
- repo example と live runtime config を混同しない。
- archive 文書を一次参照にしない。
- Viewer / IdleChat / STT / TTS / LLM provider の挙動変更を構造変更に混ぜない。

## 検証方針

### 仕様だけ変更した場合

- `docs/10_新仕様/` と `docs/refactor/` の相互矛盾を確認する。
- 実在しないファイルを現行実装として書いていないか確認する。
- `git diff --check` を実行する。
- コードテストは原則不要だが、仕様が既存テスト名を参照する場合はパスの存在を確認する。

### コードだけ変更した場合

- Phase26 では原則行わない。
- 後続 Phase で行う場合は、対象 package に近い unit / contract test を先に固定する。
- gofmt と `GOCACHE=/tmp/picoclaw-gocache go test <対象package>` を実行する。

### 仕様とコードを両方変更した場合

- 先に仕様文書を commit し、その後に実装する。
- 実装後、仕様の入力、出力、副作用、永続化、ログ、エラー契約と差分がないか確認する。
- `GOCACHE=/tmp/picoclaw-gocache go test ./...` を基本確認とする。

### Viewer / IdleChat / STT / TTS / LLM provider / Memory / Source Registry

- Viewer は DOM 存在だけでなく、表示本文、SSE event、event log、session state を確認する。
- IdleChat は raw response、view data、audio trigger、口パク trigger を分ける。
- STT は通常 Chat 入力だけへ流し、IdleChat へ流さない。
- TTS chunk は表示本文の唯一根拠にしない。
- LLM provider は timeout、empty response、nil provider、fallback を成功扱いしない。
- Memory / Source Registry は observed、candidate、validated、promoted の遷移を飛ばさない。

### 1 対 1 対応の改善確認

次を満たす場合に改善と判定する。

- 仕様変更時の主担当 module が明確である。
- 主担当以外の変更理由が adapter / application / infrastructure の層分離として説明できる。
- 1 つの仕様変更で無関係な helper、manager、util を触らない。
- 変更前より対象 package と確認テストを狭く指定できる。
- 差し替え時の影響範囲を説明できる。

## 次に進むべき作業

Phase26 では、仕様側の矛盾修正と、挙動を変えないファイル分離を実施した。

Phase26 後に残る次作業候補:

1. `runtime_dependencies.go` に残る Viewer handler wiring、store setup、provider setup をさらに分類する。
2. runtime option assembly を実装として分けるか、現行の層別実装を仕様として固定するかを判断する。
3. `cmd/picoclaw` 配下の composition root helper が Application / Infrastructure の usecase 本体を持っていないか再照合する。

この順に進める理由は、Phase26 で大きな集中点を分離した後も、`runtime_dependencies.go` は依然として最も大きい composition root helper であり、Viewer wiring、store setup、provider setup の境界が残っているためである。

## Phase26 実施結果

### 仕様更新

- `docs/10_新仕様/` を追加し、新仕様の概要、モジュール構成、検証仕様を整理した。
- `docs/10_新仕様/モジュール構成仕様.md` に、仕様変更対象と実装箇所の対応表を追加した。
- `runtime_options.go` は現行ファイルではなく将来の分離候補として扱うよう修正した。
- Worker execution、DistributedOrchestrator、health / status runtime、STT runtime、runtime dependency wiring の実装箇所対応を更新した。

### コード分離

| 対象 | 分離後の主なファイル | 目的 |
| --- | --- | --- |
| WorkerExecutionService | `worker_execution_file_ops.go`, `worker_execution_shell.go`, `worker_execution_git.go`, `worker_execution_errors.go`, `worker_execution_summary.go` | file / shell / git / error / summary の内部責務を分ける |
| DistributedOrchestrator | `distributed_orchestrator_runtime.go`, `distributed_orchestrator_retry.go`, `distributed_orchestrator_display.go` | runtime helper、retry/error contract、display notice を root file から分ける |
| health runtime | `health_commands.go`, `health_runtime.go` | CLI command と health service builder を分ける |
| runtime dependencies | `runtime_local_agents.go`, `runtime_idlechat_handlers.go`, `runtime_dependencies.go` | local agent runtime と IdleChat handler wiring を composition root helper から分ける |
| STT runtime | `stt_runtime_config.go`, `stt_runtime_websocket.go`, `stt_runtime_audio.go`, `stt_runtime_http.go`, `stt_runtime_factory.go` | URL inference、WebSocket、audio payload、HTTP inference、runtime assembly を分ける |

### 検証結果

- `GOCACHE=/tmp/picoclaw-gocache go test ./...` 成功。
- `GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e` 成功。
- `PICOCLAW_LIVE_E2E=1 GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e -run TestE2E_Phase25LiveRuntimeHealthAndViewerConfig -v` 成功。
- `PICOCLAW_BROWSER_E2E=1 PICOCLAW_LIVE_BASE_URL=http://127.0.0.1:18790 GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e -run TestE2E_Phase25BrowserViewerSessionContract -v` 成功。
- `git diff --check` 成功。

## 完了条件

- `docs/refactor/Phase26_システムリファクタリング方針.md` が作成されている。
- 仕様優先、コード優先、両方修正、変更保留の判断基準が書かれている。
- 現在の要注意箇所が分類されている。
- 次に実装へ進む前の仕様修正単位が明確になっている。
- コード変更を行っていない。
