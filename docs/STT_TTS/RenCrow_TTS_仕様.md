# RenCrow_TTS 仕様

## 1. 目的

RenCrow_TTS は、Chat が生成した文字列と TTS Provider の間に入る制御モジュールである。

目的は、Chat の長い応答をそのまま 1 本の音声にせず、短い発話単位へ分割し、音声生成と Viewer 表示を同じ単位で同期させることである。

基本フロー:

```text
Chat文字列
  -> RenCrow_TTS Controller
  -> SBV2 Provider
  -> tts.audio_chunk
  -> Viewer
```

本仕様では TTS Provider として現在 SBV2 を使うが、RenCrow_TTS Controller は Provider 非依存の制御層として扱う。

## 2. 責務分離

### 2.1 Chat

Chat は発話元の応答文字列を生成する。

Chat は原則として音声ファイル生成、TTS API 詳細、Viewer 再生順序を直接制御しない。

Chat から RenCrow_TTS Controller へ渡す情報:

- `session_id`
- `response_id`
- `character_id`
- `voice_id`
- `text`
- `emotion` または発話文脈

### 2.2 RenCrow_TTS Controller

RenCrow_TTS Controller は、Chat文字列を音声・字幕同期用の発話単位に変換する中間制御層である。

主な責務:

- Chat文字列を短い発話単位へ分割する
- 発話単位ごとに TTS Provider へ合成依頼する
- `chunk_index` を単調増加で採番する
- Provider 応答を `tts.audio_chunk` に変換する
- Viewer が再生順序を判断できる情報を付与する
- 音声と表示文字列が同じ chunk 由来になり、再生タイミングと表示タイミングが同期するよう保証する

### 2.3 SBV2 Provider

SBV2 Provider は、1つの発話単位を1つの音声ファイルへ変換する。

Provider は以下を受け取る。

- `text`
- `voice_id` または解決済み voice 情報
- `output_dir`
- 必要に応じた Provider 固有パラメータ

現在の SBV2 サーバ仕様では `/voice` を使う。

代表 voice 解決:

```text
mio / female_01   -> amitaro    -> model_id=0 / speaker_id=0 / style=Neutral
shiro / male_01   -> shi-gozaki -> model_id=6 / speaker_id=0 / style=Neutral
```

### 2.4 Viewer

Viewer は `tts.audio_chunk` を受け取り、音声再生キューへ積む。

Viewer は再生開始時に、その chunk の `text` を現在発話中の文字列として表示する。  
再生終了または再生エラー時に、次の chunk へ進む。

Viewer は、TTS対象の長文全体を先に一括表示しない。  
発話表示は音声再生中の chunk を単位とし、音声が次 chunk へ進むタイミングで表示文字列も次 chunk へ切り替える。

Live Mode では配信用レイアウトを優先し、Now Playing 表示を出さない場合がある。ただし内部的な再生順序と音声同期は維持する。

## 3. 発話単位分割

RenCrow_TTS Controller は、Chat文字列を短い文またはワンフレーズ単位に分割する。

分割の基本方針:

- 1 chunk は1つの音声ファイルとして生成する
- 1 chunk は Viewer の1つの現在発話表示単位になる
- 長文を丸ごと1本の音声にしない
- 長文を Viewer に丸ごと一括表示しない
- 句点までの短い文を優先する
- 長すぎる文は自然な境界で追加分割する

境界の優先順位:

1. 強い境界: `。`, `！`, `？`, `.`, `!`, `?`, 改行
2. 弱い境界: `、`, `，`, `,`, `;`, `；`, `:`, `：`
3. 空白
4. 最大長到達時の強制分割

目安:

```text
最小長: 6文字
最大長: 72文字
```

短い相づちや感嘆など、6文字未満でも自然な独立発話として扱うべき場合は、最終 flush 時に chunk 化してよい。

## 4. セッションと chunk

RenCrow_TTS Controller は、Chat応答ごとに TTS セッションを開始する。

セッション開始時の情報:

```text
session_id
response_id
character_id
voice_id
speech_mode
conversation_mode
```

各 chunk には以下を付与する。

```text
session_id
response_id
utterance_id
chunk_index
character_id
text
audio_path または audio_url
track
```

`chunk_index` は同一 `(session_id, track)` 内で `0` から単調増加する。

`utterance_id` は以下を推奨する。

```text
{session_id}:{chunk_index4桁}
```

例:

```text
idle-123-tts-456:0002
```

## 5. `tts.audio_chunk` 契約

RenCrow_TTS Controller は、音声生成が完了した chunk ごとに `tts.audio_chunk` を発行する。

`tts.audio_chunk` は、文字列 chunk と音声 chunk を結びつける同期単位である。  
共通IDの名称そのものより、同じ payload 内の `text` と `audio_path` / `audio_url` が同じ発話単位を指し、Viewer で同時に扱われることを必須条件とする。

Payload 例:

```json
{
  "session_id": "idle-123-tts-456",
  "response_id": "resp-789",
  "utterance_id": "idle-123-tts-456:0000",
  "chunk_index": 0,
  "character_id": "mio",
  "text": "今日はいい天気ですね。",
  "audio_path": "viewer-tts-abc.wav",
  "audio_url": "",
  "track": "default"
}
```

必須:

- `session_id`
- `utterance_id`
- `chunk_index`
- `character_id`
- `text`
- `audio_path` または `audio_url`

推奨:

- `response_id`
- `track`

Viewer は `text` をその chunk の字幕・現在発話表示として扱う。  
つまり、表示される文字列と再生される音声は同じ chunk から来なければならない。

`utterance_id` は追跡・デバッグ用の共通IDとして推奨するが、Viewer の同期保証は `tts.audio_chunk` payload 単位で成立する。

## 6. 再生順序

Viewer は `(session_id, track, chunk_index)` を優先して昇順再生する。

`track` が空の場合は `default` とみなす。

基本順序:

```text
chunk 0 の音声再生開始
  -> chunk 0 の text を表示
chunk 0 の音声終了
  -> chunk 1 の音声再生開始
  -> chunk 1 の text を表示
...
全chunk終了
  -> 表示をクリア、または最後の表示を保持する
```

表示をクリアするか保持するかは Viewer 表示モードごとの方針とする。

## 7. Viewer 表示契約

Viewer の発話表示は、TTS 再生キューと同期する。

禁止:

- TTS対象の長文全文を、音声再生前に一括で発話表示する
- chunk 0 の音声再生中に、chunk 1 以降の文字列を現在発話として表示する
- 音声chunkと異なる `text` を現在発話として表示する

必須:

- 音声再生開始時に、その音声chunkと同じ `tts.audio_chunk` payload の `text` を表示する
- 次の音声chunkへ進む時、表示文字列も次chunkの `text` に切り替える
- 音声再生が停止・失敗した場合、表示も停止状態へ戻す、または次chunkへ進む

通常 Viewer:

- `Now Playing` などの現在発話表示を使い、chunk単位の文字列を表示してよい。
- Timeline に全文メッセージを残す場合でも、現在発話表示とは分離する。

Live Mode:

- 配信用レイアウトでは `Now Playing` を非表示にしてよい。
- ただし、発話表示を行う場合は必ずchunk単位で表示する。
- 長文全文を中央Chatに一括表示して、音声だけ後追いで再生する表示は禁止する。

## 8. SBV2 Provider 契約

現在の SBV2 Provider は `POST /voice` を利用する。

リクエスト形式:

```text
POST /voice?text=<urlencoded>&model_id=<id>&speaker_id=0&style=Neutral
```

Provider は1回の呼び出しにつき、1つの chunk 音声を返す。

RenCrow_TTS Controller 側は、Provider の `/voice` 仕様を Viewer に漏らさない。  
Viewer が知るのは `tts.audio_chunk` の `audio_path` または `audio_url` だけである。

## 9. エラー時の扱い

1 chunk の音声生成に失敗した場合、Controller は以下のいずれかで処理する。

- 当該 chunk をスキップして次 chunk へ進む
- セッションを中断する
- 再試行してから失敗扱いにする

推奨:

- transport error は最大2回まで再試行
- Provider の不正入力エラーは再試行しない
- `engine_unavailable` 系は短いバックオフ後に再試行
- 失敗時もログに `session_id`, `chunk_index`, `character_id`, `text` を残す

Viewer には、音声が存在しない `tts.audio_chunk` を送らない。

## 10. 現行実装との対応

現行実装で対応する主な場所:

```text
internal/application/orchestrator/tts_support.go
  - Chat文字列の句点優先 chunk 分割

internal/application/orchestrator/tts_bridge.go
  - TTSBridge インターフェース

internal/infrastructure/tts/sbv2_tts_bridge.go
  - SBV2直結TTS Bridge

internal/infrastructure/tts/sbv2_provider.go
  - SBV2 /voice Provider

cmd/picoclaw/tts_client_bridge.go
  - TTS Bridge 構築と tts.audio_chunk 発行

internal/adapter/viewer/viewer.html
  - tts.audio_chunk 受信、再生キュー、現在発話表示
```

注意:

- 通常 Chat ストリーミング経路では、句点優先分割の仕組みが存在する。
- IdleChat や SBV2 直結経路では、入力文がそのまま1 chunk になる箇所が残りうる。
- 今後は RenCrow_TTS Controller として分割・採番・発行責務を集約する。

## 11. 完了条件

RenCrow_TTS Controller の実装完了条件:

- Chat の長文応答が短い chunk に分割される
- chunk ごとに SBV2 Provider が呼ばれる
- `tts.audio_chunk.text` と再生音声が一致する
- Viewer が音声再生開始時に、その音声 chunk と同じ payload の `text` を表示する
- Viewer がTTS対象の長文を一括表示せず、chunk単位で現在発話表示を更新する
- `chunk_index` が同一 session 内で単調増加する
- Viewer は音声再生タイミングに合わせて該当 chunk の文字列を表示する
- Live Mode では不要な Now Playing 表示を出さず、再生同期だけを維持する

## 12. 参照

- `docs/STT_TTS/AUDIO_Client仕様/TTS/仕様.md`
- `docs/STT_TTS/AUDIO_Client仕様/TTS/実装仕様.md`
- `docs/STT_TTS/AUDIO_Client仕様/TTS/old/REN_CROW_TTS_BRIDGE_SPEC.md`
- `docs/STT_TTS/SBV2_サーバ仕様.md`
