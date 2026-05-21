# RenCrow STT Streaming 暫定/確定字幕仕様

## 1. 背景

RenCrow の STT は、Viewer からマイク音声を逐次送信し、STT 側で途中認識を返し、発話確定後に `final` text を通常 chat 入力へ接続するための audio 境界である。

旧 STT 仕様では、字幕を次の 2 層で扱う方針が定義されていた。

- 暫定字幕: 短い chunk / window の認識結果。発話中に書き換わってよい。Chat / LLM には渡さない。
- 確定字幕: 無音、停止、VAD などを契機に長めの発話区間を再解釈した結果。Chat / LLM に渡してよい唯一の STT text。

現行 Viewer は、ブラウザのマイク入力を 16kHz mono PCM16 に resample し、WebSocket binary frame として chunk 送信している。207 STT server は、`start` control、PCM16 raw chunk、`stop` control を受け取り、`progress` / `partial` / `final` を返す。

この仕様では、旧仕様の「暫定字幕 / 確定字幕」方式を、207 STT server の現行 protocol と RenCrow Viewer / Go proxy の実装境界に合わせて再定義する。

## 2. 用語定義

| 用語 | 定義 |
| --- | --- |
| audio chunk | Viewer から STT server へ逐次送る PCM16 little-endian mono binary frame。Chat / LLM 入力ではない。 |
| partial | STT server が直近 window から返す途中認識 text。Viewer 表示・ログ用。Chat / LLM には渡さない。 |
| draft | 旧 voice-bridge 仕様の途中認識 event。現行仕様では `partial` と互換扱いする。 |
| final | STT server が発話確定後に返す確定 text。通常 chat input に接続できる唯一の STT text。 |
| progress | STT server が受信済み音声量を返す event。認識 text ではない。 |
| utterance | 1 回の発話区間。VAD、`stop`、`final_pending`、close、最大発話長などで区切られる。 |
| session_id | STT 接続または発話セッションを追跡する ID。新しい ID を乱立させず、既存 ID で表現できる範囲を優先する。 |
| event_id | STT server が各 STT event / request を追跡する ID。server log との照合に使う。 |
| seq | STT server からの event 順序。`partial` / `final` の並びを確認するために使う。 |
| final text | `final.text` の trim 済み文字列。空文字は chat input にしない。 |
| chat input | RenCrow 通常 chat に渡すユーザー入力。STT では `final text` だけが該当する。 |

## 3. アーキテクチャ

```text
Browser Viewer
  -> Tailscale Serve / LAN HTTP(S)
  -> RenCrow Go /stt proxy
  -> 207 STT server /stt
  -> partial / final
  -> Viewer
  -> final text only
  -> normal chat input
```

### Browser Viewer

- マイク権限を取得する。
- 入力レベルを UI に表示する。
- audio input を 16kHz mono PCM16 に resample する。
- WebSocket を開き、`start` を送る。
- PCM16 raw chunk を binary frame として送る。
- 録音停止時に `stop` を送る。
- `partial` / `draft` は暫定字幕として表示・ログに保持する。
- `final` だけを通常 chat input へ渡す。
- IdleChat へ STT input を直接流さない。

### RenCrow Go `/stt` proxy

- Viewer から見える `/stt` / `/stt-ws` / `/ws` の互換 endpoint を提供する。
- `STT_GATEWAY_URL` または `RENCROW_STT_URL` が設定されている場合、STT server へ WebSocket を透過 proxy する。
- text frame と binary frame を破壊せず転送する。
- STT server への接続失敗は `error` として Viewer に返す。
- Proxy は認識 text を生成しない。HTTP file inference と WS streaming を混同しない。

### 207 STT server

- WebSocket `/stt` を受ける。
- `start` control で sample rate / channels / format を受ける。
- PCM16 little-endian raw chunk を受ける。
- 受信済み音声量を `progress` として返す。
- 直近 window の途中認識を `partial` として返す。
- `stop` / `final_pending` / close / VAD / 最大発話長により `final` を返す。
- HTTP `/v1/audio/transcriptions` は file inference 用であり、WebSocket streaming と入力仕様が違う。

### HTTP file inference との違い

| 項目 | WebSocket streaming | HTTP file inference |
| --- | --- | --- |
| 用途 | 実マイクの逐次入力 | 保存済み WAV などの一括推論 |
| 入力 | PCM16 raw chunk + control JSON | multipart file |
| 途中認識 | `partial` / `draft` | なし |
| 確定 | `final` event | HTTP response text |
| 検証注意 | WAV bytes を直送しない | WAV file をそのまま送ってよい |

## 4. 通信契約

### Viewer -> STT

#### WebSocket open

Viewer は `/viewer/runtime-config` の `stt_stream_url` を優先して接続する。Tailscale Viewer では browser-facing URL として `wss://<tailnet-host>/stt` を使い、LAN では設定された LAN URL を使う。

#### start

接続後、音声 chunk より前に送る。

```json
{
  "type": "start",
  "sample_rate": 16000,
  "channels": 1,
  "format": "pcm_s16le"
}
```

`language` を送る場合は `ja` を指定する。未指定時は STT server config の既定値を使う。

#### PCM16 binary chunk

- binary frame で送る。
- format: PCM16 little-endian raw。
- channels: 1。
- sample rate: `start.sample_rate` と一致させる。
- WAV header を含めない。
- WAV whole file / WAV chunk を streaming の正常入力として扱わない。

#### stop

録音停止時に送る。

```json
{ "type": "stop" }
```

`stop` は finalization request であり、`stop` 送信後にさらに audio chunk を送らない。

#### close

`stop` 送信後、`final` または `error` を受けるまで待つ。ブラウザやネットワーク都合で close-only になった場合でも、STT server 側が音声を保持していれば finalization を試みる。ただし RenCrow Viewer の正常系は `stop` 明示である。

#### final_pending

旧仕様互換 event として扱う。

```json
{ "type": "final_pending" }
```

新規実装では `stop` を優先する。`final_pending` は旧 `/stt-ws` / `/ws` 互換または既存クライアント移行用とする。

### STT -> Viewer

#### ready

```json
{
  "type": "ready",
  "event_id": "evt_stt_...",
  "provider": "whisperkit",
  "model": "large-v3-v20240930_turbo",
  "sample_rate": 16000
}
```

接続または `start` 受理を示す。`ready` は認識成功ではない。

#### progress

```json
{
  "type": "progress",
  "event_id": "evt_stt_...",
  "duration": 3.0,
  "bytes": 96000
}
```

`duration` / `bytes` は受信済み音声全体の値である。認識済み window の値ではなく、認識 text でもない。

#### partial / draft

```json
{
  "type": "partial",
  "session_id": "viewer-session",
  "event_id": "evt_stt_...",
  "text": "テストを",
  "seq": 2,
  "stability": 0.6,
  "start_ms": 1000,
  "end_ms": 3000,
  "is_final": false
}
```

`partial` は暫定字幕である。旧 `draft` は同じ暫定字幕として扱う。Viewer は表示・ログ・デバッグ trace には使ってよいが、Chat / LLM へ送ってはいけない。

#### final

```json
{
  "type": "final",
  "session_id": "viewer-session",
  "event_id": "evt_stt_...",
  "text": "テスト",
  "seq": 3,
  "language": "ja",
  "duration": 3.0,
  "is_final": true,
  "reason": "stop"
}
```

`final.text` が通常 chat input の唯一の入力元である。空文字、no speech、error は chat input にしない。

#### closed

```json
{
  "type": "closed",
  "event_id": "evt_stt_...",
  "reason": "client_closed"
}
```

接続終了を示す。`closed` は `final` の代替ではない。`final` がない `closed` は未確定終了として扱う。

#### error

```json
{
  "type": "error",
  "event_id": "evt_stt_...",
  "error_code": "NO_SPEECH",
  "message": "speech was not detected"
}
```

`error` は通常 chat 成功として隠さない。Viewer は session state と STT log に残す。

## 5. 状態遷移

```text
idle
  -> connecting
  -> ready
  -> recording
  -> receiving partial
  -> finalizing
  -> finalized
  -> closed

任意状態
  -> error
  -> closed
```

| 状態 | 意味 | 主な event |
| --- | --- | --- |
| idle | STT 未開始 | mic off |
| connecting | WebSocket 接続中 | open pending |
| ready | server が受理可能 | `ready` |
| recording | audio chunk 送信中 | binary chunk / `progress` |
| receiving partial | 暫定 text 受信中 | `partial` / `draft` |
| finalizing | `stop` / VAD / close 後の確定待ち | `stop`, `final_pending` |
| finalized | `final` 受信済み | `final` |
| error | STT 失敗 | `error`, timeout, proxy failure |
| closed | 接続終了 | close / `closed` |

## 6. テキスト確定ルール

- `partial` / `draft` は Viewer 表示・STT log・debug trace 用である。
- `partial` / `draft` は Chat / LLM に渡さない。
- `progress` は音声量の進捗であり、文字列ではない。
- `final.text` だけを通常 chat input にできる。
- `final.text` が空の場合は通常 chat input にしない。
- `final` が来る前に `partial` を chat input として送る fallback は原則禁止する。
- fallback が必要な診断モードを設ける場合は、通常 chat input ではなく「暫定扱い」として UI と log に明示する。
- STT input は通常 chat のみに接続する。IdleChat へ直接流さない。

## 7. Viewer UI 仕様

### マイク入力レベル表示

Viewer は録音中の PCM16 chunk から入力レベルを算出し、マイクボタン上に表示する。これはマイク入力の有無を示す UI であり、STT 認識成功ではない。

### STT 接続状態

Viewer は少なくとも次を表示する。

- mic on / off
- STT off / connecting / connected / waiting
- session_id または `(unknown)`
- action error

### partial 表示

`partial` / `draft` は暫定字幕として表示できる。確定前であることが分かる UI とし、通常 chat input と混同しない。

### final 表示

`final` は確定字幕として表示し、通常 chat input へ接続する。`final` を受けたら入力欄に反映し、通常 chat 送信処理へ進める。

### セッション ID 表示

STT server から `session_id` / `event_id` が返る場合、Viewer はコピー可能または log で追跡可能にする。

### STT ログ保存

Viewer は次を保存できるようにする。

- client URL
- ws URL
- test time
- session_id
- `progress`
- `partial` / `draft`
- `final`
- `error`
- 保存した client WAV

STT log は観測証跡であり、Chat / LLM 入力そのものではない。

## 8. エラー仕様

| エラー | 判定 | Viewer 表示 | Chat 入力 |
| --- | --- | --- | --- |
| no speech | STT server が発話を検出できない | error / no speech | 送らない |
| provider timeout | STT provider が timeout | error / timeout | 送らない |
| websocket close | `final` なしで close | 未確定終了 | 送らない |
| invalid audio format | WAV header 直送、sample width 不一致など | invalid audio | 送らない |
| sample rate mismatch | `start.sample_rate` と実 chunk が一致しない | config error | 送らない |
| 207 STT unreachable | 207 `/stt` に接続不可 | STT unreachable | 送らない |
| RenCrow proxy failure | Go `/stt` proxy 失敗 | proxy failure | 送らない |

Error path を fallback 成功として扱わない。Unit test や HTTP health が OK でも、実 microphone -> WS -> `final` -> normal chat input が成立しない場合は E2E 成功ではない。

## 9. 検証仕様

### Viewer 実マイク E2E

- Tailscale Viewer または LAN Viewer を開く。
- マイク権限を許可する。
- 入力レベルが反応することを確認する。
- `ready` / `progress` / `partial` / `final` を STT log で確認する。
- `final.text` が通常 chat input として処理されることを確認する。

### 207 direct WS

WAV を検証に使う場合は、WAV whole file bytes を WebSocket に送らない。WAV から PCM16 raw を取り出し、`start` と `stop` を明示して送る。

```text
WAV file
  -> decode header
  -> extract PCM16 raw
  -> ws://192.168.1.207:8766/stt
  -> start(sample_rate=16000, channels=1, format=pcm_s16le)
  -> binary PCM16 chunks
  -> stop
  -> expect final
```

### RenCrow `/stt` proxy WS

207 direct WS と同じ PCM16 raw + `start` + `stop` を `ws://127.0.0.1:18790/stt` または Tailscale `wss://<host>/stt` へ送る。Direct と proxy の event 内容が大きくズレないことを確認する。

### HTTP `/v1/audio/transcriptions`

HTTP file inference は、保存 WAV の一括推論確認に使う。WS streaming の代替確認として扱わない。

### `scripts/stt_e2e_probe.py` の修正方針

- WS 検証時に WAV whole file bytes を送らない。
- WAV を `wave` 等で decode し、PCM16 raw を chunk 分割して送る。
- 接続後に `start` を送る。
- 終了時に `stop` を送る。
- `partial` と `final` を別々に記録する。
- `final` がない成功を E2E 成功扱いしない。ただし partial-only の診断は別項目に記録する。

## 10. 移行方針

1. Viewer から `start` を明示送信する。
2. Viewer 停止時に `stop` を明示送信し、`final` を待ってから close する。
3. `partial` / `draft` を暫定字幕 UI として表示する。
4. `final` のみ通常 chat input へ接続する。
5. 旧 `draft` は `partial` と互換扱いする。
6. `final_pending` は旧仕様互換として残すが、新規正常系は `stop` に寄せる。
7. `/stt-ws` / `/ws` は互換 endpoint として維持する。
8. `scripts/stt_e2e_probe.py` を PCM16 raw + `start` + `stop` に修正する。

## 現行実装との差分

| 項目 | 現行 | 本仕様 |
| --- | --- | --- |
| Viewer start | 明示送信していない | 接続後に `start` を送る |
| Viewer stop | 明示送信していない | 停止時に `stop` を送る |
| Viewer audio | PCM16 raw chunk 送信済み | 継続 |
| partial / draft | `partial` / `draft` を lastRecognition として保持 | 暫定字幕 UI として明示し、Chat へ送らない |
| final 未到達時 | 停止時に latest partial を final 扱いで送る補助実装がある | 原則禁止。診断モード以外では Chat へ送らない |
| Go proxy | text / binary frame を透過 | 継続 |
| Go fallback WS | provider に chunk ごと WAV 化して推論し `draft` を返す | fallback は正常系ではない。E2E 成功扱いしない |
| WS probe | WAV bytes を直送していた | PCM16 raw + `start` + `stop` に修正する |
| HTTP inference | 保存 WAV の推論確認に使用 | WS streaming の代替にしない |

## 実装タスク一覧

### local regression

- Viewer: WebSocket open 時に `start` control を送る。
- Viewer: `stopSTT()` で `stop` control を送り、`final` / `error` / timeout を待ってから close する。
- Viewer: `partial` / `draft` を暫定字幕として表示する UI を追加する。
- Viewer: `final` 未到達時に latest partial を通常 chat 送信する fallback を削除または診断モードへ隔離する。
- Viewer tests: start / chunk / stop / final の contract test を追加する。
- Probe: `scripts/stt_e2e_probe.py` を PCM16 raw + `start` + `stop` に修正する。
- Go tests: `/stt` proxy が JSON control と binary chunk を透過することを確認する。

### external dependency

- 207 STT server が `start` / PCM16 raw chunk / `stop` / `final` を安定して返すこと。
- 207 STT server log に `event_id` / `session_id` / VAD / partial / final / error が追跡可能に出ること。
- Tailscale Viewer 経由で `wss://<tailnet-host>/stt` が継続利用できること。

### blocked

- 実マイク E2E は、ブラウザ端末のマイク権限、入力デバイス、音量、207 STT server 稼働状態に依存する。
- `final` が返らない状態では通常 chat input 接続を完了扱いにしない。
- fallback / partial-only / HTTP file inference 成功は STT streaming E2E 成功ではない。

## 検証チェックリスト

- [ ] Viewer runtime-config が正しい `stt_stream_url` を返す。
- [ ] Viewer が WebSocket open 後に `start` を送る。
- [ ] Viewer が PCM16 little-endian raw chunk を binary frame で送る。
- [ ] Viewer が stop 時に `{ "type": "stop" }` を送る。
- [ ] 207 direct WS で `ready` / `progress` / `partial` / `final` が返る。
- [ ] RenCrow `/stt` proxy WS で direct WS と同等の event が返る。
- [ ] Tailscale `wss://<tailnet-host>/stt` で同等の event が返る。
- [ ] `partial` / `draft` が Viewer 暫定字幕・ログに表示される。
- [ ] `partial` / `draft` が Chat / LLM に送られない。
- [ ] `final.text` だけが通常 chat input に送られる。
- [ ] `final` なし close は未確定終了として扱われる。
- [ ] no speech / timeout / invalid audio / proxy failure が error として見える。
- [ ] `scripts/stt_e2e_probe.py` が WAV whole file を WS に直送しない。
- [ ] HTTP `/v1/audio/transcriptions` は file inference として別枠で記録される。

## 実装・確認メモ

### 2026-05-21 実装済み

- Viewer マイク入力レベル表示を追加した。
- Viewer WebSocket open 時に `start` control を送るようにした。
- Viewer 停止時に残り PCM16 chunk を送信後、`stop` control を送り、`final` / `error` / timeout / close を待って終了処理へ進むようにした。
- 207 STT の partial 推論に 6 秒以上かかるケースがあるため、Viewer の final 待ち timeout を 30 秒にした。
- 保存 WAV の realtime probe では 1 秒無音 tail を付けた場合に `final` が安定したため、Viewer 停止時も残り PCM16 chunk の後に 1 秒の無音 tail を送り、その後 `stop` control を送るようにした。
- `partial` / `draft` を通常 chat input へ送る停止時 fallback を削除し、`final.text` のみ通常 chat input に接続する contract test を追加した。
- `partial` / `draft` と `final` を入力欄とは別の STT 字幕 UI に表示するようにした。
- `scripts/stt_e2e_probe.py` を WAV decode -> PCM16 raw chunk -> `start` -> binary chunks -> `stop` protocol に修正し、`final` がない WS 結果を success 扱いしないようにした。
- `scripts/stt_e2e_probe.py` に `--require-ws-final` を追加し、WS round の `final` が欠ける場合は non-zero exit にした。
- Go `/stt` proxy が JSON control と PCM16 binary chunk を透過する E2E test を追加した。

### 2026-05-21 確認済み

- `node --test internal/adapter/viewer/viewer_stt_https.test.mjs`
- `python3 -m py_compile scripts/stt_e2e_probe.py scripts/stt_e2e_probe_test.py`
- `python3 -m unittest scripts/stt_e2e_probe_test.py`
- `GOCACHE=/tmp/picoclaw-gocache go test ./...`
- `git diff --check`
- `make install` 後に `picoclaw.service` を再起動し、`http://127.0.0.1:18790/health`、local Viewer、Tailscale Viewer 200、配信中 HTML / JS 反映を確認した。
- `tmp/client_stt_input_latest.wav` を使い、次の WS endpoint で `ready` / `progress` / `partial` / `final` を確認した。
  - `ws://192.168.1.207:8766/stt`
  - `ws://127.0.0.1:18790/stt`
  - `wss://fujitsu-ubunts.tailb07d8d.ts.net/stt`
- Playwright Chromium の fake microphone で local Viewer を開き、ブラウザ `getUserMedia` -> Viewer PCM16 chunk -> 207 STT の経路で `start` / binary chunk / `stop` 送信と `partial` 受信を確認した。同 run では 207 STT が `NO_SPEECH_DETECTED` を返し、Viewer は `STT recognition unavailable: 音声が検出されませんでした。` を session 表示に残し、通常 chat input へ送信しなかった。
- Playwright Chromium の fake microphone run で、207 STT から `final` / `error` が返らない場合に Viewer が `STT error: STT final unavailable: timed out waiting for final` を字幕欄と session 表示に残し、通常 chat input へ送信しないことを確認した。
- `python3 scripts/stt_e2e_probe.py --wav tmp/client_stt_input_latest.wav --provider-rounds 0 --ws-rounds 1 --ws-wait 20 --ws-url ws://127.0.0.1:18790/stt --require-ws-final` が、`final` なしの runtime result を exit code 2 として失敗扱いにすることを確認した。同 run では `ready` / `progress` までで timeout しており、STT streaming E2E 成功ではない。
- `python3 scripts/stt_e2e_probe.py --wav tmp/client_stt_input_latest.wav --provider-rounds 0 --ws-rounds 1 --ws-wait 60 --ws-realtime --ws-tail-silence-ms 1000 --require-ws-final` を使い、次の WS endpoint で `final` が返ることを確認した。
  - `ws://192.168.1.207:8766/stt`
  - `ws://127.0.0.1:18790/stt`
  - `wss://fujitsu-ubunts.tailb07d8d.ts.net/stt`
- Playwright Chromium の fake microphone は、run ごとに `partial` / `NO_SPEECH_DETECTED` / timeout の揺れがあり、`final` -> 通常 chat input 送信の完了証跡にはできなかった。

### 残る未確認

- 実ブラウザのマイク操作で、Mic ON -> 入力レベル -> `partial` 表示 -> `stop` -> `final` -> 通常 chat input 送信までを 1 セッション通して確認すること。
- no speech は実 runtime 表示まで確認済み。provider timeout / invalid audio / proxy failure などの error path は、実 runtime 表示として網羅確認すること。

## 分類

| 分類 | 項目 |
| --- | --- |
| local regression | Viewer start/stop control、partial UI、final-only Chat 接続、probe 修正、Go proxy test |
| external dependency | 207 STT server、WhisperKit、MacBook 207 launchd、Tailscale Serve |
| blocked | 実マイク・実ブラウザ・207 runtime が必要な E2E、final 未返却時の chat 接続 |

## Goal 実行用作業ルール

この仕様を Goal に設定して実装する場合は、未完了項目を小さな検証済み commit 単位で順に処理する。

各単位では、実装前に次を定義する。

- 対象: Viewer / Go proxy / probe / docs / runtime 確認のどれか。
- 変更範囲: 触るファイルと触らないファイル。
- 検証コマンド: Node test、Go test、`git diff --check`、runtime / Viewer 確認のどれを行うか。
- 完了条件: `partial` / `draft` / `final`、Chat input、error 表示、proxy 透過など、何が確認できれば完了か。

実装後は、該当テストと必要な runtime / Viewer 確認を行う。確認済みの関連ファイルだけを選択的に stage し、日本語 commit message で commit する。commit 後は push する。push できたら、不要なユーザー確認を待たずに次の未完了項目へ進む。

### 推奨 commit 単位

1. `scripts/stt_e2e_probe.py` を PCM16 raw + `start` + `stop` protocol に修正する。
2. Viewer から WebSocket open 時に `start` を送る。
3. Viewer 停止時に `stop` を送り、`final` / `error` / timeout を待ってから close する。
4. Viewer の `partial` / `draft` 暫定字幕 UI を追加する。
5. `final` 未到達時に latest partial を通常 chat 送信する fallback を削除または診断モードへ隔離する。
6. Go `/stt` proxy の JSON control / binary chunk 透過 test を追加する。
7. runtime / Viewer E2E 確認結果をこの仕様または残課題台帳へ反映する。

各 commit は 1 つの責務だけを持つ。Viewer / Go proxy / probe / docs / runtime 証跡を、責務が曖昧なまま 1 commit に混ぜない。

### 標準検証

変更内容に応じて、以下から必要なものを選ぶ。

```bash
node --test internal/adapter/viewer/viewer_stt_https.test.mjs
node --test internal/adapter/viewer/viewer_memory_panel.test.mjs
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw ./internal/adapter/viewer ./internal/infrastructure/stt -count=1
git diff --check
```

runtime / Viewer に触れた場合は、必要に応じて次も確認する。

- `systemctl --user stop picoclaw.service` を含むクリーン停止後に `make install` / restart する。
- `http://127.0.0.1:18790/health` が OK である。
- `/viewer/runtime-config` が 207 STT / TTS endpoint を返す。
- LAN または Tailscale Viewer で Mic ON から `ready` / `progress` / `partial` / `final` を 1 セッション追う。
- Tailscale Viewer 配信に関わる場合は `tailscale serve status --json` と Viewer HTTPS 200 を確認する。

### stage / commit / push ルール

- worktree 全体を一括 stage しない。
- 確認済みの関連ファイルだけを `git add` する。
- 日本語 commit message で commit する。
- commit 後は push する。
- push 後に、commit hash、検証コマンド、次に進む対象を短く報告する。
- push 対象に未確認差分、別責務の差分、live E2E 生成物、一時生成物、`vault/` が混ざりそうな場合は停止する。

### 停止条件

次の場合だけ作業を止めて報告または質問する。

- テスト失敗や runtime / Viewer 確認失敗が、現在の作業範囲内で短時間に解消できない。
- 変更が複数領域へ広がり、1 commit の責務が曖昧になった。
- 207 STT server、MacBook launchd、ブラウザ実マイク、Tailscale、外部 secret など、作業者側で準備できない依存が必要になった。
- destructive operation、依存追加、CI / deploy 設定変更、ファイル削除、セーフガード変更が必要になった。
- push 対象に未確認差分、別責務の差分、または live E2E 証跡ファイルが混ざりそうになった。
- Goal の達成条件が、この仕様または `docs/01_正本仕様/STT_正本仕様.md` と矛盾している。

### 禁止事項

- blocked / skipped / fail を成功扱いしない。
- fallback 成功、partial-only、HTTP file inference 成功を STT streaming E2E 成功扱いしない。
- `partial` / `draft` を通常 chat input として送る変更を、明示的な診断モードなしに入れない。
- Viewer 表示、STT log、Chat input、TTS 音声、口パク trigger を混同しない。
- live E2E 証跡、`tmp/` の録音、`vault/`、一時生成物を明示指示なく commit しない。
