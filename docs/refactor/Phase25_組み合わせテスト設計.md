# Phase25 組み合わせテスト設計

## 目的

この文書は、Phase24 までのリファクタリング完了判定を受けて、RenCrow の主要機能をどの組み合わせで検証するかを定義する品質保証設計書である。

Phase24 では、リファクタリング対象の構造整理、通常テスト、`test/e2e` のタグ付き E2E が成功している。ただし、それは「リファクタリング差分が主要境界を壊していない」ことの確認であり、Chat / Worker / Coder、Viewer、IdleChat、STT、TTS、LLM provider、transport、runtime config、Memory / Source Registry の全組み合わせが設計済みであることを意味しない。

Phase25 ではテスト実装は行わない。今後の TDD / E2E 実装に使うため、必須ケース、代表ケース、外部依存ケース、ブラウザ観測ケースを分けて設計する。

## 前提

- 正本仕様は `docs/01_正本仕様/実装仕様.md` とする。
- `docs/codebase-map/` は一次解析資料として使い、最終判断は正本仕様と現在コードで行う。
- `docs/archive/` は一次参照にしない。
- fallback は正常成功として扱わない。
- Viewer 表示、音声、口パク、ログを同じ契約として扱わない。
- repo example、live runtime config、Viewer 表示値を混同しない。
- 外部 API、Ollama、local_openai、STT server、TTS server、browser permission、HTTPS が必要な検証は、環境前提を明記する。

## テスト分類

| 分類 | 役割 | 主な対象 | 成功条件 |
| --- | --- | --- | --- |
| unit test | 小さい関数、契約、境界条件を固定する | route helper、contract、DTO、policy、state transition | 外部依存なしで決定的に通る |
| contract test | interface / event / DTO / error contract を固定する | Chat / Worker / Coder、Viewer event、provider response | 実装差し替え時に呼び出し側が壊れない |
| integration test | package 間の組み合わせを確認する | Orchestrator、WorkerExecutionService、persistence | 主要フローの入出力と副作用が一致する |
| httptest e2e | HTTP handler と Application 境界を process 内で確認する | Viewer send、runtime config、LLM ops proxy | 実サーバなしで route / response / event を確認できる |
| live e2e | 実 service、live config、実 endpoint で確認する | `/health`、LLM inference、runtime config | live runtime と実装が一致する |
| browser e2e | 実ブラウザの表示、SSE、音声、権限を確認する | Viewer、IdleChat、STT、TTS、lipsync | DOM 存在ではなく 1 session の状態遷移が成立する |
| manual observation | 自動化が難しい実機観測を記録する | 音声再生、口パク、VTube Studio、外部機器 | 観測ログと期待状態を分けて記録する |
| external dependency e2e | 外部 API / provider 実体との接続を確認する | OpenAI、Claude、DeepSeek、Google Search、Ollama | 未設定 skip を成功扱いせず、準備済み環境で pass する |

## 機能軸

### 役割軸

- Chat: ユーザー対話、route 判断、結果返却を担当し、実行責務を持たない。
- Worker: file / shell / git / test 実行、policy、protected pattern、ログ記録を担当する。
- Coder: plan / patch / proposal 生成を担当し、破壊的操作を直接実行しない。

### route 軸

- `CHAT`
- `PLAN`
- `ANALYZE`
- `OPS`
- `RESEARCH`
- `CODE`
- `CODE1`
- `CODE2`
- `CODE3`

### provider 軸

- `local_openai`
- `ollama`
- `deepseek`
- `openai`
- `claude`
- `gemini`

### transport 軸

- `local`
- `ssh`
- `mailbox`
- `direct`
- `distributed`

### runtime / UI / state 軸

- Viewer
- IdleChat
- STT
- TTS
- Audio router
- VTuber / lipsync
- runtime config
- Memory / Source Registry
- ToolRunner / PolicyEngine
- WorkerExecutionService
- CodeExecutor
- DistributedOrchestrator

## 組み合わせマトリクス

| 対象機能 | 入力 | 経路 | 依存モジュール | 期待結果 | 検証種別 | 必須度 | 外部依存 | 既存テスト | 不足テスト | 優先度 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Chat route | 通常会話 | adapter -> MessageOrchestrator -> Mio/Chat | routing、session、event、TTS hook | Worker / Coder 実行責務を持たず、Chat response を返す | unit / integration / live e2e | 必須 | live LLM は任意 | `message_orchestrator_test.go`, `test/e2e/routing_test.go` | live local_openai / ollama での Chat route pass | 高 |
| Worker route | 実行指示 | MessageOrchestrator -> WorkerExecutionService | proposal、patch、policy、tools、execution log | file / shell / git / policy / log contract を守る | unit / integration | 必須 | shell/git は sandbox 条件あり | `message_orchestrator_route_chain_contract_test.go`, service tests | route 入口から WorkerExecutionService までの代表 e2e | 高 |
| Coder route | `/code`, CODE 系自然文 | MessageOrchestrator -> CodeExecutor -> Coder -> Worker | CodeExecutor、proposal、WorkerExecutionService | Coder は plan / patch 生成、Worker が実行 | unit / integration / external e2e | 必須 | API provider は外部依存 | `code_executor_test.go`, `message_orchestrator_code_path_test.go` | provider 別 proposal / generate の live e2e | 高 |
| CODE1 / CODE2 / CODE3 | 明示 route | route dispatcher -> CodeExecutor | coder selection、proposal path | route 差分と coder selection が維持される | unit / integration / e2e | 必須 | CODE provider は環境依存 | `message_orchestrator_code3_test.go`, `code_executor_test.go`, `TestE2E_Routing_ChromeKeywords_CODE3` | CODE1 / CODE2 の tagged e2e | 高 |
| fallback / error path | nil provider、timeout、empty response | provider / orchestrator / verifier | error contract、event、report | fallback を成功扱いせず、失敗として返す | contract / integration | 必須 | timeout は fake で再現可能 | `message_orchestrator_route_chain_contract_test.go`, `verify_contract_test.go` | IdleChat empty response の Viewer 表示 e2e | 高 |
| Viewer send | `/viewer/send` | Viewer handler -> orchestrator | adapter/viewer、runtime config、route prefix | message が期待 prefix / model alias で処理される | httptest e2e / browser e2e | 必須 | browser は必要時 | `TestE2E_ViewerModelSwitch_RuntimeConfigStartAndSend` | 実ブラウザで送信から表示まで 1 session | 高 |
| Viewer runtime config / LLM ops | `/viewer/runtime-config`, `/viewer/llm-ops/*` | Viewer handler -> ops proxy | config、LLMOpsProxy、Viewer JS | repo example ではなく有効 runtime が表示される | httptest e2e / live e2e / browser e2e | 必須 | management API は外部依存 | `model_switch_test.go`, `main_status_health_test.go` | live `~/.picoclaw/config.yaml` と Viewer 表示の照合 | 高 |
| IdleChat raw/view/audio | IdleChat session | idlechat -> event -> Viewer -> TTS | idlechat orchestrator、event hub、TTS bridge、Viewer JS | raw、表示本文、音声、口パク trigger が分離される | integration / browser e2e / manual | 必須 | LLM、TTS、browser | `idlechat_tts_test.go`, orchestrator event tests | ブラウザで session 開始から終了までの raw/view/audio 照合 | 高 |
| STT normal chat | mic input | HTTPS Viewer -> STT provider -> chat input | stt runtime、viewer stt handler、browser permission | STT input は通常 chat に入り、IdleChat に入らない | integration / browser e2e / external e2e | 必須 | HTTPS、browser、STT server | `main_stt_gateway_test.go` | 実ブラウザ録音、trailing silence、chat input 反映 | 高 |
| TTS browser playback | response text | TTS bridge -> audio route -> browser playback | TTS provider、audio URL、Viewer playback | TTS chunk は音声と口パク trigger であり表示本文の唯一根拠ではない | unit / integration / browser e2e | 必須 | TTS server、browser | `tts_support_test.go`, `tts_browser_audio_test.go`, `tts_client_bridge_test.go` | 実ブラウザ再生と表示本文の分離確認 | 高 |
| Audio router / lipsync | TTS event | audio router SSE -> playback / VTuber | audiorouter、vtuber、Viewer event | audio / lipsync / log が混同されない | integration / browser e2e / manual | 重要 | audio device、VTS | `vtuber_support_test.go`, audio router package tests | VTube Studio / device map を含む live observation | 中 |
| distributed local | distributed message | DistributedOrchestrator -> local transport | route dispatcher、session、evidence、transport | local agent 経路で evidence と response が残る | unit / integration | 必須 | なし | `distributed_orchestrator_test.go`, Phase15-23 tests | end-to-end route matrix の整理 | 高 |
| distributed ssh | distributed message | DistributedOrchestrator -> SSH transport | transport、coder selection、evidence | 接続済み SSH のみ使用し、未接続は失敗扱い | unit / live e2e | 重要 | SSH host | `main_distributed_mode_test.go`, `distributed_orchestrator_test.go` | 実 SSH agent での live e2e | 中 |
| distributed mailbox / direct | distributed message | transport executor | mailbox/direct transport、route mapping | transport ごとの error contract が維持される | unit / integration / live e2e | 重要 | agent 実体 | Phase20 tests | mailbox/direct live representative | 中 |
| Memory / Source Registry | source save、conversation event | Viewer / CLI -> L1 store -> staging -> promote | L1SQLiteStore、sourcefetcher、viewer handlers | observed / candidate / validated / promoted を飛ばさない | unit / integration / browser e2e | 必須 | DB / browser | source registry CLI tests、persistence tests | Viewer memory/source panels の 1 session | 高 |
| ToolRunner / PolicyEngine | tool request | policy -> tool runner | policy、security、tools、workspace | allowed / blocked / needs-review の理由が残る | unit / integration | 必須 | network tool は外部依存 | infrastructure/tools, security tests | Worker route 入口との代表 integration | 高 |
| runtime config | repo example / live config | config load -> runtime wiring -> Viewer | config、cmd runtime、health、LLM ops | repo example と live config を混同しない | unit / live e2e | 必須 | live service | `main_local_llm_test.go`, `main_status_health_test.go` | live `/health` と Viewer runtime config 照合 | 高 |
| external LLM providers | prompt | provider Generate | provider factory、middleware、raw log | non-empty response、timeout/error が契約通り | external dependency e2e | 重要 | API key | `test/e2e/api_test.go` | key 準備済み CI / 手動環境で pass 記録 | 中 |
| Google Search | web_search | ToolRunner -> Google API | ToolRunner、API key、CSE | Chat / Worker の検索設定が分離される | external dependency e2e | 重要 | API key / CSE | `test/e2e/search_test.go` | key 準備済み環境で Chat/Worker 両方 pass | 中 |

## 必須ケース

### Chat / Worker / Coder

- Chat route は Worker / Coder 実行責務を持たない。
- Worker route は file / shell / git / policy / log contract を守る。
- Coder route は plan / patch / proposal 生成に留まり、破壊的操作を直接実行しない。
- `CODE`, `CODE1`, `CODE2`, `CODE3` は route 差分と coder selection 差分を維持する。
- invalid proposal は Worker に到達しない。
- Worker error、generate error、unknown route は成功 response に変換しない。

### Provider / error

- nil provider、missing key、timeout、empty response を正常成功として扱わない。
- provider fallback は「選択経路」または「エラー経路」として記録し、成功の根拠にしない。
- raw log、display response、event response を混同しない。

### Viewer / IdleChat / Audio

- Viewer send、runtime config、LLM ops の handler contract を固定する。
- IdleChat は raw response、view data、audio trigger を分ける。
- STT input は通常 chat のみに流し、IdleChat には流さない。
- TTS chunk は表示本文の唯一根拠にしない。
- audio、lipsync、log はそれぞれ別の観測対象として確認する。

### Runtime / distributed / memory

- repo example と live runtime config を混同しない。
- distributed local / ssh / mailbox / direct route の代表ケースを持つ。
- Memory / Source Registry は observed / candidate / validated / promoted の境界を飛ばさない。

## 代表ケース

全組み合わせを機械的に総当たりしない。理由は、route、provider、transport、UI、音声、runtime config を全掛け合わせにすると、外部依存の失敗と仕様上の失敗が混ざり、品質判断が曖昧になるためである。

代表ケースは次の基準で選ぶ。

- 責務境界をまたぐケースを優先する。
- 障害時にユーザー観測へ直接出るケースを優先する。
- fallback、timeout、empty response、nil dependency など誤って成功扱いしやすいケースを優先する。
- 外部依存がある場合、fake / httptest で契約を固定し、live e2e は代表 endpoint に限定する。
- 既存テストが薄い領域を優先する。
- 変更頻度が高い route dispatch、runtime config、Viewer / IdleChat、Worker execution、provider factory を優先する。

## 外部依存ケース

| 外部依存 | 対象 | skip 可能条件 | skip の記録方法 | 成功扱いにしてよい条件 |
| --- | --- | --- | --- | --- |
| API key | Claude / DeepSeek / OpenAI / Gemini | key が未設定 | test log に provider 名と未設定理由を出す | key 設定済み環境で Generate が成功 |
| Ollama | Chat / Mio / local route | endpoint 到達不能 | base_url と到達不能理由を出す | `/api/tags` 到達後に Generate が成功 |
| local_openai | Chat / Worker / Heavy / Wild | endpoint 到達不能 | role alias と base_url を出す | lightweight inference が成功 |
| Google Search | Chat / Worker search | API key または CSE 未設定 | Chat / Worker どちらの設定不足か出す | search result が空でなく URL を含む |
| STT server | Viewer mic / STT file / websocket | STT endpoint 到達不能 | provider URL と route を出す | final transcript が通常 chat input に入る |
| TTS server | TTS chunk / browser playback | TTS endpoint 到達不能 | provider 名、voice、URL を出す | audio URL が取得でき、browser playback が開始する |
| Browser permission / HTTPS | mic、audio、SSE、Viewer | secure context または permission 不足 | browser log と permission 状態を記録 | 1 session の表示、event、audio trigger が成立 |
| SSH agent | distributed ssh | host 未設定または接続不能 | host、agent、route を記録 | 接続済み agent で evidence と response が残る |
| VTube Studio / audio device | lipsync / audio router | device / VTS 未起動 | device map、VTS endpoint を記録 | lipsync trigger と音声再生が別々に観測できる |

skip は外部依存未準備の記録であり、機能成功ではない。CI や通常開発環境で skip する場合も、対応する fake / httptest / contract test が別に存在することを確認する。

## Browser / Viewer 検証方針

Browser / Viewer では DOM 要素の存在だけで成功扱いしない。

最低 1 session について次を開始から終了まで追う。

- 入力
- route
- response
- SSE event
- Viewer event log
- history / session state
- error / invalid response 表示
- TTS audio trigger
- lipsync trigger
- 終了状態

IdleChat では特に次を分ける。

- LLM raw response
- 編集後の表示本文
- diagnostic / test-mode 表示
- TTS chunk
- 口パク trigger
- topic store / history / summary
- fallback / invalid response

STT/TTS では次を分ける。

- browser permission / HTTPS
- provider endpoint
- websocket / HTTP route
- trailing silence
- transcript
- audio URL
- browser playback
- lipsync event

## TDD 実装順

1. unit / contract test
   - route helper、error contract、event DTO、provider response、state transition を固定する。
2. integration test
   - MessageOrchestrator、CodeExecutor、WorkerExecutionService、DistributedOrchestrator、L1SQLiteStore の代表経路を接続する。
3. httptest e2e
   - Viewer send、runtime config、LLM ops、memory/source registry handler を process 内で確認する。
4. external dependency e2e
   - API key、Ollama、local_openai、Google Search、STT/TTS endpoint がある環境でのみ実行する。
5. browser / live e2e
   - Viewer / IdleChat / STT / TTS / lipsync の 1 session を実ブラウザで追う。
6. manual observation
   - 音声デバイス、VTube Studio、実機マイクなど自動化が不安定なものを観測記録として残す。

1 回の Phase で複数の責務を混ぜない。たとえば Viewer 表示契約と STT provider contract は別 Phase に分ける。

## 既存テストで確認済みの大分類

- MessageOrchestrator の route chain、fallback error、session、TTS bridge。
- CodeExecutor の CODE route、proposal path、generate path、Worker error。
- DistributedOrchestrator の event、evidence、TTS lifecycle、session、autonomous、route dispatcher、transport、code execution、coder selection、attribution guard。
- `cmd/picoclaw` の composition root 周辺、health/status、local LLM config、STT route、TTS audio route、IdleChat TTS。
- `test/e2e` の Viewer model switch、CODE3 routing、外部 provider / Ollama / Google Search の環境依存 E2E。

## 不足テストの大分類

- Chat / Worker / Coder × route × provider の live 代表 E2E。
- CODE1 / CODE2 の tagged E2E。
- Viewer send から描画完了までの browser E2E。
- IdleChat raw / view / audio / history / invalid response の browser E2E。
- STT mic input が通常 chat にだけ入る browser E2E。
- TTS browser playback と lipsync trigger の live / browser E2E。
- Memory / Source Registry の Viewer panel を含む 1 session E2E。
- distributed ssh / mailbox / direct の実 agent 代表 E2E。
- API key 準備済み環境での external provider E2E。
- live `~/.picoclaw/config.yaml`、`/health`、Viewer runtime config の照合 E2E。

## 完了条件

- この文書に組み合わせマトリクスがある。
- 必須ケースが Chat / Worker / Coder、Viewer、IdleChat、STT、TTS、runtime config、distributed、Memory / Source Registry を含んでいる。
- 外部依存ケースと skip 条件が明記されている。
- Browser / Viewer 検証方針が明記されている。
- 今後の TDD 実装順が明記されている。
- テスト実装とコード変更を行っていない。
