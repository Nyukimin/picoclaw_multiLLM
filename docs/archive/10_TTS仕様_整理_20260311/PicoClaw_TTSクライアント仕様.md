了解。
では、**RenCrow 側クライアント仕様**だけを切り出して、実装に使える形でまとめるね。

````md
# RenCrow TTS Client 仕様 v0.1
TTS Server 接続クライアント / 実装側仕様

## 1. 目的

本仕様は、RenCrow 側から TTS Server を利用するためのクライアント実装仕様を定義する。

対象は以下。

- TTS Server との接続管理
- session 単位の送受信管理
- LLM ストリームからの text delta 送信
- audio chunk の受信
- 再生順制御
- エラー時の挙動

本仕様は、RenCrow の Chat / Worker / Audio Sink 間の責務境界も含む。

---

## 2. スコープ

### 2.1 本仕様に含むもの

- TTS Client
- Speech Bridge
- Audio Sink への受け渡し
- HTTP health check
- WebSocket session 管理
- text delta 送信
- session_end 送信
- audio chunk 受信
- chunk 順序制御
- timeout / 再接続方針

### 2.2 本仕様に含まないもの

- TTS Server 内部実装
- Emotion Planner 実装
- Speech Planner 実装
- 音声再生デバイスドライバ
- STT
- LLM 推論本体

---

## 3. 全体構成

```text
Gemma3 Stream
  ↓
Speech Bridge
  ↓
TTS Client
  ├─ Health Checker
  ├─ Session Controller
  ├─ WS Sender
  ├─ WS Receiver
  └─ Audio Chunk Reorder Buffer
  ↓
Audio Sink
  ↓
再生 / 保存 / 転送
````

---

## 4. コンポーネント責務

## 4.1 Speech Bridge

### 役割

LLM のストリーミング出力を受け取り、TTS 用 session に変換する。

### 責務

* session_id の発行
* response_id の保持
* voice_id の選択
* speech_mode / event / urgency の付与
* text delta を順番付きで TTS Client に渡す
* 応答終了時に session_end を通知する

### 非責務

* chunking
* 発話整形
* TTS パラメータ変換

---

## 4.2 TTS Client

### 役割

TTS Server との通信を一元管理する。

### 責務

* `/health/ready` による事前確認
* WebSocket 接続
* session_start 送信
* text_delta 送信
* session_end 送信
* audio_chunk_ready 受信
* error 受信
* session_completed 受信
* Audio Sink への受け渡し

### 非責務

* 音声再生
* 発話チャンク生成
* prosody 決定

---

## 4.3 Audio Sink

### 役割

TTS Server から返ってきた audio chunk を順序通りに扱う。

### 責務

* chunk_index による順序管理
* 欠番待機
* timeout 時のスキップ判断
* 再生または保存
* session 単位の終了処理

---

## 5. 接続仕様

## 5.1 接続先

初期値：

* Health API: `http://127.0.0.1:8765`
* WebSocket: `ws://127.0.0.1:8765/sessions`

### 設定項目

* `tts_http_base_url`
* `tts_ws_url`
* `tts_connect_timeout_ms`
* `tts_receive_timeout_ms`
* `tts_chunk_gap_timeout_ms`

---

## 5.2 起動前 health check

RenCrow は session 開始前に必要に応じて `/health/ready` を確認する。

### 成功条件

* status = `ready`
* 要求 voice_id が `voices` に存在する

### 失敗時

* TTS を使わずテキストのみ継続してよい
* リトライは任意
* Chat 本体は停止させない

---

## 6. Session 仕様

## 6.1 session_id

RenCrow 側で払い出す一意識別子。

形式例：

```text
sess-20260311-0001
```

## 6.2 response_id

Chat 側の応答識別子。
RenCrow 内部の追跡用。

## 6.3 session の単位

1 回の AI 応答 = 1 session とする。

* ユーザー発話 1 件に対する AI 応答全体
* 応答の途中で複数 chunk に分割されても同一 session

---

## 7. 送信メッセージ仕様

## 7.1 session_start

TTS session を開始する。

### 送信タイミング

* Chat が応答開始した直後
* 最初の text delta を送る前

### 形式

```json
{
  "type": "session_start",
  "session_id": "sess-001",
  "response_id": "resp-001",
  "voice_id": "female_01",
  "speech_mode": "conversational",
  "context": {
    "event": "conversation",
    "urgency": "normal",
    "conversation_mode": "chat",
    "user_attention_required": false
  }
}
```

---

## 7.2 text_delta

LLM の逐次出力を送る。

### 送信タイミング

* 新しい文字列 delta を受け取るたび
* 順序保証が必要

### 形式

```json
{
  "type": "text_delta",
  "session_id": "sess-001",
  "seq": 1,
  "text": "今日はですね、",
  "emitted_at": "2026-03-11T08:00:00+09:00"
}
```

### ルール

* `seq` は 1 からの連番
* 欠番を作らない
* 同一 session 内では順序を崩さない

---

## 7.3 session_end

session の終了を通知する。

### 送信タイミング

* LLM 応答が終了した時
* もう text delta が来ないと確定した時

### 形式

```json
{
  "type": "session_end",
  "session_id": "sess-001",
  "is_final": true
}
```

### 注意

* 未送信バッファ flush のため、必ず送る
* text が空でも送る

---

## 8. 受信メッセージ仕様

## 8.1 session_started

session 開始の受付通知。

```json
{
  "type": "session_started",
  "session_id": "sess-001",
  "voice_id": "female_01"
}
```

### クライアント側動作

* session 状態を `streaming` にする

---

## 8.2 audio_chunk_ready

音声 chunk の生成完了通知。

```json
{
  "type": "audio_chunk_ready",
  "session_id": "sess-001",
  "chunk_index": 0,
  "text": "今日はですね、",
  "audio_path": "cache/sess-001_000.wav",
  "sample_rate": 44100,
  "pause_after": "short"
}
```

### クライアント側動作

* Reorder Buffer に格納
* 再生可能順なら Audio Sink へ渡す

---

## 8.3 session_completed

TTS Server 側で session 処理完了。

```json
{
  "type": "session_completed",
  "session_id": "sess-001"
}
```

### クライアント側動作

* 以後その session に text_delta を送らない
* 未再生 chunk の再生完了を待って閉じる

---

## 8.4 error

エラー通知。

```json
{
  "type": "error",
  "session_id": "sess-001",
  "code": "INVALID_SEQ",
  "message": "expected seq=2, got seq=4"
}
```

### 代表コード

* `SESSION_NOT_FOUND`
* `INVALID_SEQ`
* `VOICE_NOT_FOUND`
* `MODEL_NOT_READY`
* `SYNTH_FAILED`
* `SESSION_TIMEOUT`

### クライアント側動作

* ログ記録
* session を abort または degraded mode へ移行
* Chat 本体は継続可能

---

## 9. Audio Chunk Reorder Buffer 仕様

## 9.1 目的

音声 chunk を `chunk_index` 順に再生する。

## 9.2 管理項目

* `expected_chunk_index`
* `pending_chunks`
* `last_chunk_received_at`
* `session_completed_received`

## 9.3 再生条件

* `expected_chunk_index` の chunk が揃ったら即再生
* 再生後 `expected_chunk_index += 1`

## 9.4 欠番時の動作

* 先の chunk が来ても pending に保持
* `tts_chunk_gap_timeout_ms` を超えたら欠番スキップ可

## 9.5 session 終了時

* `session_completed` を受信
* pending を昇順に再生可能な限り処理
* timeout 後に閉じる

---

## 10. 状態管理

## 10.1 Client Session State

```json
{
  "session_id": "sess-001",
  "response_id": "resp-001",
  "voice_id": "female_01",
  "status": "streaming",
  "next_seq": 3,
  "expected_chunk_index": 1,
  "pending_chunks": {},
  "started_at": "2026-03-11T08:00:00+09:00",
  "last_send_at": "2026-03-11T08:00:01+09:00",
  "last_receive_at": "2026-03-11T08:00:02+09:00"
}
```

## 10.2 状態一覧

* `created`
* `starting`
* `streaming`
* `finishing`
* `completed`
* `error`
* `aborted`

---

## 11. voice_id 選択仕様

## 11.1 基本方針

RenCrow 側は voice 実体を知らず、`voice_id` のみを選択する。

## 11.2 既定値

* 通常会話: `female_01`

## 11.3 将来拡張

* 状況に応じた `male_01`
* narrator voice
* notification voice

---

## 12. speech_mode 指定仕様

RenCrow 側は session 開始時に `speech_mode` を指定する。

候補：

* `conversational`
* `notification`
* `report`
* `warning`
* `approval_prompt`

### 初期方針

* 通常会話 -> `conversational`
* 報告系 -> `report`
* 警告系 -> `warning`

---

## 13. エラー処理

## 13.1 接続失敗

### 条件

* WebSocket 接続失敗
* HTTP health check 失敗

### 動作

* TTS なしで応答継続可
* ログに degraded mode を記録
* 必要なら一定回数再試行

---

## 13.2 session_start 失敗

### 条件

* `VOICE_NOT_FOUND`
* `MODEL_NOT_READY`

### 動作

* その session の TTS を無効化
* Chat 応答は継続

---

## 13.3 text_delta 送信失敗

### 条件

* 接続切断
* timeout
* 送信例外

### 動作

* session を abort
* Audio Sink 側に終了通知
* Chat 応答は継続

---

## 13.4 audio chunk 受信失敗

### 条件

* 欠番
* session_completed が来ない
* chunk が破損

### 動作

* timeout 後に再生打ち切り
* 後続 session は継続可能

---

## 14. timeout 仕様

### 推奨値

* `tts_connect_timeout_ms = 3000`
* `tts_receive_timeout_ms = 15000`
* `tts_chunk_gap_timeout_ms = 3000`

### 定義

* connect timeout: WS 接続確立待ち
* receive timeout: 受信全体の待機
* chunk gap timeout: 欠番 chunk の待機時間

---

## 15. ログ仕様

## 15.1 必須ログ項目

* `session_id`
* `response_id`
* `voice_id`
* `seq`
* `chunk_index`
* `event`
* `elapsed_ms`

## 15.2 ログイベント

* `tts_health_check_start`
* `tts_health_check_ready`
* `tts_ws_connect`
* `tts_session_start_sent`
* `tts_text_delta_sent`
* `tts_audio_chunk_received`
* `tts_audio_chunk_play_start`
* `tts_audio_chunk_play_done`
* `tts_session_end_sent`
* `tts_session_completed_received`
* `tts_error_received`
* `tts_session_abort`

---

## 16. 実装インタフェース案

## 16.1 Speech Bridge

```python
class SpeechBridge:
    async def start_session(
        self,
        response_id: str,
        voice_id: str,
        speech_mode: str,
        context: dict,
    ) -> str: ...

    async def push_text(self, session_id: str, text: str) -> None: ...

    async def end_session(self, session_id: str) -> None: ...
```

---

## 16.2 TTS Client

```python
class TTSClient:
    async def ensure_ready(self) -> bool: ...

    async def connect(self) -> None: ...

    async def send_session_start(self, payload: dict) -> None: ...

    async def send_text_delta(self, payload: dict) -> None: ...

    async def send_session_end(self, payload: dict) -> None: ...

    async def receive_loop(self) -> None: ...
```

---

## 16.3 Audio Sink

```python
class AudioSink:
    async def submit_chunk(
        self,
        session_id: str,
        chunk_index: int,
        audio_path: str,
        text: str,
        pause_after: str,
    ) -> None: ...

    async def complete_session(self, session_id: str) -> None: ...
```

---

## 17. 実装順序

### Phase 1

* `/health/ready` を叩く HTTP Client
* `POST /synthesize` を叩く単発 Client

### Phase 2

* WebSocket 接続
* `session_start / text_delta / session_end` 送信

### Phase 3

* `audio_chunk_ready` 受信
* Audio Chunk Reorder Buffer

### Phase 4

* Audio Sink 実装
* 順次再生

### Phase 5

* timeout / abort
* エラーハンドリング整備

---

## 18. v0.1 完成条件

以下を満たした時点で RenCrow TTS Client v0.1 完了とする。

* `/health/ready` が確認できる
* `session_start` を送れる
* `text_delta` を順番に送れる
* `session_end` を送れる
* `audio_chunk_ready` を受け取れる
* chunk_index 順に再生できる
* 接続失敗時に TTS なし継続ができる

---

## 19. 一行要約

RenCrow TTS Client は、LLM の文字列ストリームを session 単位で TTS Server に送信し、返ってくる音声 chunk を順序制御して Audio Sink に渡す責務を持つ。

```

次はこれをそのまま **Python のクラス骨格** に落とせる。
```
