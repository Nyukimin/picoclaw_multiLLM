# Viewer 仕様

## 1. 目的

Viewer は、RenCrow の実行状態、会話、TTS再生、IdleChat、ジョブ進捗、実行証跡をブラウザで確認・操作するための統合UIである。

Viewer は単なるログ表示ではなく、以下を同時に扱う。

- Chat / Worker / Coder / IdleChat のイベント表示
- TTS音声再生と発話表示
- Live配信用表示
- ジョブ進捗と実行証跡
- IdleChat 操作
- STT入力補助
- Viewerからのメッセージ送信

対象実装:

```text
internal/adapter/viewer/viewer.html
internal/adapter/viewer/handler.go
internal/adapter/viewer/hub.go
internal/adapter/viewer/evidence_handler.go
internal/adapter/viewer/audio_router_sse.go
cmd/picoclaw/main.go
```

## 2. 基本URL

通常Viewer:

```text
http://<RenCrowホスト>:18790/viewer
```

Live Mode:

```text
http://<RenCrowホスト>:18790/viewer?mode=live
```

Live Mode は配信用の表示モードであり、入力欄や一部操作UIを隠し、表示密度と視認性を優先する。

## 3. 全体構成

```text
Orchestrator / IdleChat / TTS / STT
  -> EventHub.OnEvent()
  -> /viewer/events (SSE)
  -> viewer.html
  -> Timeline / Live Mode / TTS再生 / 各パネル
```

主要コンポーネント:

- `viewer.html`
  - 単一ページUI
- `EventHub`
  - SSE用イベント蓄積・配信
- `/viewer/events`
  - Viewer本体向けSSE
- `/audio-router/events`
  - audio-router向け `tts.audio_chunk` 抽出SSE
- `/viewer/send`
  - Viewer入力欄からChatへメッセージ送信

## 4. タブ構成

Viewer は以下のタブを持つ。

| タブ | 役割 |
|---|---|
| `Ops` | 運用者向けの現在状態要約 |
| `Overview` | Agent状態の俯瞰 |
| `Progress` | Agent / Job の進捗 |
| `Timeline` | 会話・イベントの時系列表示 |
| `System` | routing / entry / TTS / STT 等の技術イベント確認 |
| `IdleChat` | IdleChat 状態・履歴・操作 |
| `Sessions` | session 単位の集約 |
| `Jobs` | job と execution evidence の参照 |

## 5. EventHub / SSE

Viewer は `/viewer/events` を EventSource で購読する。

SSEイベントは `orchestrator.OrchestratorEvent` をJSON化したものである。

基本方針:

- 初回接続時に直近履歴を送る
- 以後はリアルタイムにイベントを追記する
- `Last-Event-ID` により再接続時の既読分をスキップする
- EventHub は短期ライブ観測用であり、完全な監査ログ保存は目的としない

現行の目安:

```text
履歴件数: 200
subscriber channel容量: 64
```

遅いクライアントにはイベントがdropされうる。

## 6. Timeline 表示

Timeline は会話系・進行系イベントを時系列で表示する。

代表イベント:

- `message.received`
- `routing.decision`
- `agent.note`
- `agent.thinking`
- `agent.response`
- `idlechat.message`
- `idlechat.summary`
- `tts.audio_chunk`

Timeline は履歴・観測用途の表示であり、TTSの現在発話表示とは役割を分ける。

TTS対象テキストの全文をTimelineに残す場合でも、音声再生に同期する発話表示は `tts.audio_chunk` 単位で行う。

## 7. TTS表示・再生契約

Viewer は `tts.audio_chunk` を受け取り、音声再生キューへ積む。

`tts.audio_chunk` は、文字列chunkと音声chunkを結びつける同期単位である。

必須payload:

```json
{
  "session_id": "tts-session",
  "utterance_id": "tts-session:0000",
  "chunk_index": 0,
  "character_id": "mio",
  "text": "今日はいい天気ですね。",
  "audio_path": "viewer-tts-abc.wav",
  "audio_url": "",
  "track": "default"
}
```

Viewer の再生順序:

```text
tts.audio_chunk受信
  -> 再生キューへ追加
  -> (session_id, track, chunk_index) を優先して昇順再生
  -> 音声再生開始時に同payloadの text を現在発話として表示
  -> 音声終了時に次chunkへ進む
```

## 8. Chunk単位表示

Viewer は、TTS対象の長文を現在発話として一括表示してはならない。

発話表示は、音声再生中のchunk単位で行う。

禁止:

- TTS対象の長文全文を音声再生前に現在発話として一括表示する
- chunk 0 の音声再生中に chunk 1 以降の文字列を現在発話として表示する
- 音声chunkと異なる `text` を現在発話として表示する

必須:

- 音声再生開始時に、その音声chunkと同じ `tts.audio_chunk` payload の `text` を表示する
- 次の音声chunkへ進む時、表示文字列も次chunkの `text` へ切り替える
- 音声再生が停止・失敗した場合、現在発話表示も停止状態へ戻す、または次chunkへ進む

補足:

- `utterance_id` は追跡・デバッグ用の推奨IDである。
- 同期保証は、同じ `tts.audio_chunk` payload 内の `text` と `audio_path` / `audio_url` を同時に扱うことで成立する。

## 9. Now Playing

通常Viewerでは、`Now Playing` などの現在発話表示を使って、再生中chunkの `text` を表示してよい。

`Now Playing` はTimelineとは別の表示であり、音声再生と同期する一時表示である。

通常Viewer:

- 再生開始時に `character_id + text` を表示する
- 再生終了時にクリアする、または次chunkへ切り替える

Live Mode:

- 配信用レイアウトを優先し、`Now Playing` は非表示にしてよい
- ただし、発話表示を行う場合は必ずchunk単位にする
- 長文全文を中央Chatに一括表示し、音声だけ後追いする表示は禁止する

## 10. Live Mode

Live Mode は配信用のViewer表示である。

URL:

```text
/viewer?mode=live
```

Live Mode の方針:

- 入力欄、操作ボタン、filters、toastなどを隠す
- Topicバーを表示する
- 中央Chat表示とMio/Shiroアイコンが重ならないようにする
- `Now Playing` は表示しない
- TTS再生順序とchunk同期は通常Viewerと同じにする

アイコン配置:

- Mio は左
- Shiro は右
- Topicバーに重ねない
- 中央Chatに重ねない
- 安全な余白が確保できない画面幅では、重ねるより非表示を優先する

## 11. IdleChat 連携

Viewer は IdleChat の状態表示と手動操作を提供する。

REST API:

| エンドポイント | メソッド | 用途 |
|---|---|---|
| `/viewer/idlechat/start` | `POST` | 通常IdleChat開始 |
| `/viewer/idlechat/forecast` | `POST` | 未来展望開始 |
| `/viewer/idlechat/story` | `POST` | Story開始 |
| `/viewer/idlechat/story-simple` | `POST` | Simple Story開始 |
| `/viewer/idlechat/stop` | `POST` | 停止 |
| `/viewer/idlechat/status` | `GET` | 状態取得 |
| `/viewer/idlechat/logs` | `GET` | 履歴取得 |

IdleChatイベント:

- `idlechat.message`
- `idlechat.viewer`
- `idlechat.tts`
- `idlechat.summary`

`idlechat.viewer` はViewer表示用、`idlechat.tts` はTTS用として扱う。  
TTS読み上げの現在発話表示は、最終的に `tts.audio_chunk` のchunk単位表示へ揃える。

## 12. STT UI

Viewer はSTT補助UIを持つ。

主な機能:

- 音声入力ボタン
- STT接続状態表示
- STTログ送信
- 入力WAV保存
- STT自動テスト起動

関連API:

| エンドポイント | メソッド | 用途 |
|---|---|---|
| `/stt` | `WS` | STT Gateway WebSocket proxy |
| `/viewer/stt/log` | `POST` | クライアント側STTログ保存 |
| `/viewer/stt/wav` | `POST` | 入力WAV保存 |
| `/viewer/stt/autotest` | `POST` | STT自動テスト |

## 13. Viewer送信

Viewer は `/viewer/send` でユーザーメッセージを送信できる。

Request:

```json
{
  "message": "こんにちは"
}
```

仕様:

- `POST` のみ
- `message` 必須
- HTTP応答は即時 `{"ok": true}`
- 実処理結果はSSEイベントとしてViewerへ返る
- Rolesタブで選択中の送信先がある場合、Viewerは送信前に明示ルーティングコマンドを付与する
- `mio` / Chat は本文をそのまま送信する
- `shiro` / Worker は `/ops` を付与する
- `aka` / Wild は `/wild` を付与する
- `ao` / `gin` / `kin` はそれぞれ `/code2` / `/code3` / `/code4` を付与する
- ユーザー入力がすでに `/ops` / `/wild` / `/code*` で始まる場合は、入力された明示指定を優先する

## 14. Evidence / Jobs

Viewer は execution evidence と job 状態を参照する。

代表API:

| エンドポイント | メソッド | 用途 |
|---|---|---|
| `/viewer/evidence/recent` | `GET` | 直近evidence一覧 |
| `/viewer/evidence/detail` | `GET` | job単位evidence詳細 |
| `/viewer/evidence/summary` | `GET` | evidence集計 |
| `/viewer/jobs` | `GET` | job一覧 |
| `/viewer/job/detail` | `GET` | job詳細 |

## 15. Memory昇格操作

Viewer Memoryタブは、L1 memoryを `candidate` / `confirmed` に更新し、`user:` / `char:` / `kb:` namespaceへ明示昇格できる。

昇格操作:

- UIは `target_kind` と `target_id` を送る
- APIは `target_kind` + `target_id` から `user:<id>` / `char:<id>` / `kb:<id>` を組み立てる
- 組み立てたnamespaceは `ValidateL1Namespace` を通過した場合のみpromoterへ渡す
- 互換用に `target_namespace` を直接渡す形式も受け付けるが、同じvalidatorを通す

staging validator連携:

- `memory_candidate` はvalidation通過後に自動昇格できる
- 自動昇格は `L1StagingValidationPolicy.AutoPromoteMemoryCandidate` が有効な場合のみ行う
- targetは `meta.target_namespace` を優先し、無い場合はstaging item自身の `user:` / `char:` / `kb:` namespaceを使う
- `conv:` namespaceは自動昇格targetにしない

staging archive:

- L1 staging itemはDuckDB `l1_staging_item_archive` へ保存できる
- `ExportL1ArchivesParquet()` は `l1_staging_item.parquet` を出力する
- raw_text / summary_draft / validation_status / keywords / metaをParquetへ保持する

Source Registry CLI:

- `picoclaw source-registry save --source-id <id> --url <url> --kind <kind> --license-note <text>` で登録できる
- `--trust-score`, `--interval-sec`, `--namespace`, `--disabled`, `--json` を指定できる
- `picoclaw source-registry list --json` で登録済みsourceを確認できる
- CLIは `conversation.l1_sqlite_path` のL1 SQLite storeを使う

## 16. 参照

- `docs/03_設計文書/ログViewer仕様.md`
- `docs/03_設計文書/実装仕様_ログViewer_v1.md`
- `docs/01_正本仕様/10_ログ.md`
- `docs/07_IdleChat仕様/IdleChat仕様.md`
- `docs/STT_TTS/RenCrow_TTS_仕様.md`
- `docs/STT_TTS/AUDIO_Client仕様/TTS/実装仕様.md`
