# E2E runtime 確認チェックリスト

## 1. 目的

この文書は、`docs/10_新仕様/31_未実装項目実装仕様.md` の Phase 12「E2E / runtime 要確認項目の棚卸し」を実行するためのチェックリストである。

`17_E2E残課題.md` は残課題の台帳であり、本書は実行担当者が実機 / live runtime / browser / 外部チャネルで確認するときの証跡、成功条件、失敗条件を定義する。

以下を原則とする。

- skip は成功扱いにしない。
- fallback は成功扱いにしない。
- health ok だけで user flow 成立扱いにしない。
- handler / stub / unit test だけで実 API E2E 完了扱いにしない。
- repo example config だけで runtime 確認済みにしない。
- 古いログを根拠にしない。

## 2. 参照仕様

- `docs/10_新仕様/10_検証仕様.md`
- `docs/10_新仕様/13_実装項目インベントリ.md`
- `docs/10_新仕様/17_E2E残課題.md`
- `docs/10_新仕様/31_未実装項目実装仕様.md`

## 3. 共通前提

### 3.1 live runtime config

実機確認では、repo 内の example config ではなく live runtime config を確認する。

確認対象:

- `~/.picoclaw/config.yaml`
- `http://127.0.0.1:18790/health`
- `http://127.0.0.1:18790/viewer/runtime-config`
- `/viewer` の実表示
- event log
- Worker / channel / provider の実ログ

### 3.2 証跡として必ず残すもの

各確認では、以下を記録する。

| 証跡 | 内容 |
| --- | --- |
| 実行日時 | いつ確認したか |
| 実行環境 | local / distributed / browser / external API |
| config source | live config の path または runtime-config response |
| 実行コマンド | `go test`, curl, browser操作、外部イベント送信など |
| route / session | route、job_id、session_id、channel id |
| event log | routing、agent response、attachment、warning、error |
| response | ユーザーに返った本文または失敗表示 |
| 判定 | pass / fail / skipped / blocked |
| skip理由 | secret不足、実機不足、device不足など |

## 4. 確認項目

### 4.0 2026-05-18 local verification result

実行済み:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./...
```

判定:

- pass。
- production code の package test は通過。

実行済み:

```bash
node --test \
  internal/adapter/viewer/viewer_memory_panel.test.mjs \
  internal/adapter/viewer/viewer_idle_mode_buttons.test.mjs \
  internal/adapter/viewer/viewer_audio_button.test.mjs \
  internal/adapter/viewer/viewer_stt_https.test.mjs
```

判定:

- pass。
- 2026-05-18 に `viewer_audio_button.test.mjs` の fake DOM に `document.body` / stable `main` / Source Registry refresh stub を補い、`viewer_stt_https.test.mjs` を現行の初期 `home` tab 契約に合わせた。
- audio / STT / memory / idle mode の Node Viewer contract は 34件 pass。

扱い:

- Go test pass は runtime / handler / domain 層の回帰確認として扱う。
- Viewer audio / STT の Node test pass は静的 contract の回帰確認として扱う。
- 実ブラウザ audio / STT live E2E は別確認であり、Node test pass だけで完了扱いにしない。

### 4.1 live service / Viewer 基本

目的:

- live service が起動し、Viewer と runtime config が現行 binary / live config を反映していることを確認する。

確認コマンド:

```bash
curl -fsS http://127.0.0.1:18790/health
curl -fsS http://127.0.0.1:18790/viewer/runtime-config
PICOCLAW_LIVE_E2E=1 \
GOCACHE=/tmp/picoclaw-gocache \
go test -count=1 -tags=e2e ./test/e2e \
  -run TestE2E_Phase25LiveRuntimeHealthAndViewerConfig -v
```

成功条件:

- `/health` が ok を返す。
- `/viewer/runtime-config` が live runtime の endpoint / feature 状態を返す。
- repo example と live runtime config の差が確認できる場合、live runtime を優先して記録している。

失敗扱い:

- `/health` だけで Viewer / runtime config を確認済みにする。
- 古い binary / 古い service 状態のログを根拠にする。

### 4.2 Viewer browser session

目的:

- Viewer の実ブラウザ操作で、入力、送信、表示、event log、history が 1 セッションとして成立することを確認する。

確認コマンド:

```bash
PICOCLAW_BROWSER_E2E=1 \
PICOCLAW_LIVE_BASE_URL=http://127.0.0.1:18790 \
GOCACHE=/tmp/picoclaw-gocache \
go test -count=1 -tags=e2e ./test/e2e \
  -run TestE2E_Phase25BrowserViewerSessionContract -v
```

成功条件:

- Chat タブで `#micBtn` が visible。
- IdleChat タブへ切り替えると `#idleStart` が visible。
- `/viewer/send` の response が Viewer 表示と一致する。
- event log / history / session state が同一 session として追える。

失敗扱い:

- DOM に要素が存在するだけで visible / clickable を確認しない。
- 現 UI がタブ式であることを無視し、別タブの要素を初期表示で要求する。
- 送信 response と表示本文がずれている。

### 4.3 Source Registry warning 表示

目的:

- Source Registry 由来テキストの prompt injection warning が metadata と Viewer/API 表示に残り、prompt / memory 本文と混ざらないことを確認する。

確認対象:

- `/viewer/source-registry` 系 API
- Viewer Memory / Source Registry UI
- `security_warnings` metadata
- staging / fetch result の warning 件数

成功条件:

- warning metadata が run API response に含まれる。
- Viewer で warning 件数または badge が確認できる。
- warning は reject とは別に扱われる。
- warning 付き item が無審査で promoted されない。

失敗扱い:

- warning を本文に混ぜる。
- warning 付き source を通常 memory として無審査注入する。
- warning を fallback 表示で隠す。

### 4.4 Slack / Discord / Telegram file payload 実 API

目的:

- LINE 以外の外部チャネル file payload が、実 API event から共通 attachment pipeline へ流れることを確認する。

確認対象:

| チャネル | payload | 成功条件 |
| --- | --- | --- |
| Slack | `files[]`, `url_private_download` | download した file が `IncomingFile` / `Attachment` contract へ正規化される |
| Discord | `attachments` | attachment URL / filename / MIME / size が共通 pipeline へ渡る |
| Telegram | `document`, `photo`, `file_id -> getFile -> download` | Telegram file が共通 pipeline へ渡る |

共通確認:

- session_id が channel event と attachment event で追える。
- download 失敗が通常 chat 成功として隠れない。
- MIME 不許可が rejection として返る。
- size 超過が rejection として返る。
- prompt injection warning が metadata として残る。

失敗扱い:

- stub / unit test だけで実 API 済みにする。
- download 失敗を「添付なし通常メッセージ」として処理する。
- MIME 不許可や size 超過を fallback 応答で隠す。

### 4.5 Wild / distributed runtime

目的:

- `RouteWILD` が実機分散接続で fallback なしに Wild agent 経由で応答することを確認する。

確認対象:

- `cmd/picoclaw/runtime_distributed_mode.go`
- `internal/application/orchestrator/distributed_orchestrator_routes.go`
- `internal/infrastructure/transport`
- live / temporary distributed config

確認手順:

1. live service とは別に、一時 distributed config を用意する。
2. local transport または SSH transport で distributed mode を起動する。
3. `/wild` または Wild 判定 message を送信する。
4. route evidence を確認する。
5. `job_id` / `session_id` / response / event log を照合する。

成功条件:

- `routing.decision route=WILD` が記録される。
- `agent.response wild -> mio` 相当の evidence がある。
- `job_id` と `session_id` が同一フローとして追える。
- local / CHAT fallback に落ちていない。

失敗扱い:

- local fallback で応答したものを Wild 成功にする。
- `RouteWILD` の code path 存在だけで完了扱いにする。
- route evidence なしで response だけを見る。

### 4.6 分散全経路

目的:

- Wild 以外の Worker / Coder / Heavy などの分散経路が、transport / job / response / event log まで追えることを確認する。

成功条件:

- 対象 route ごとに dispatch evidence が残る。
- transport 接続、job id、worker response、Chat 返却が一致する。
- fallback route に落ちた場合は fail として記録する。

失敗扱い:

- local 実行に落ちたのに distributed 成功扱いにする。
- fallback を degraded success として扱う。

### 4.7 STT browser live

目的:

- 実ブラウザ mic 入力が通常 chat にだけ送信され、IdleChat へ流れないことを確認する。

確認対象:

- HTTPS / browser permission
- mic capture
- trailing silence
- STT provider log
- final text
- `/viewer/send` または通常 chat input

成功条件:

- mic input から final text が得られる。
- final text が通常 chat に送信される。
- IdleChat へ STT input が流れない。
- busy policy が `queue_latest` または `reject` として記録される。

失敗扱い:

- browser permission なしの mock だけで完了扱いにする。
- transcribing のまま終了しない。
- IdleChat に音声入力が流れる。

### 4.8 TTS / audio / lipsync browser live

目的:

- TTS playback と lipsync trigger が、表示本文ではなく audio chunk / audio event を契機に動くことを確認する。

成功条件:

- TTS provider response と audio event が一致する。
- browser playback が実行される。
- lipsync は audio chunk を契機に動く。
- 表示本文の更新だけで lipsync 成功扱いにしない。

失敗扱い:

- DOM 存在だけで playback 成功扱いにする。
- 表示本文を lipsync の唯一根拠にする。
- audio provider 失敗を無音 fallback で成功扱いにする。

## 5. 結果記録テンプレート

```markdown
## E2E確認結果: {項目名}

- 日時:
- 実行者:
- 環境:
- config:
- コマンド / 操作:
- route / session / job:
- 証跡:
- 結果: pass / fail / skipped / blocked
- skip / fail 理由:
- 更新した docs:
- 次アクション:
```

## 6. docs 反映ルール

確認結果に応じて以下を更新する。

| 確認結果 | 更新先 |
| --- | --- |
| E2E が通った | `13_実装項目インベントリ.md`, `17_E2E残課題.md`, 必要なら `31_未実装項目実装仕様.md` |
| skip / blocked | `17_E2E残課題.md` に環境未準備理由を残す |
| fail | `17_E2E残課題.md` に失敗証跡と次アクションを残す |
| 実装不足が判明 | `31_未実装項目実装仕様.md` の該当 Phase に戻す |

## 7. 完了条件

このチェックリスト自体の完了条件は、全項目が `pass` になることではない。

完了条件は以下である。

- E2E / runtime 要確認項目ごとに、確認済み / skip / blocked / fail が分類されている。
- skip と fail が成功扱いされていない。
- fallback が成功扱いされていない。
- 証跡、config、route、session、log が残っている。
- `13_実装項目インベントリ.md` と `17_E2E残課題.md` の状態が実態と一致している。
