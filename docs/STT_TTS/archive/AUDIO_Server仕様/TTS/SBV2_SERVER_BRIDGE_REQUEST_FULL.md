# SBV2サーバ担当者向け 実装依頼（技術者向け完全版）

対象サーバは `192.168.1.33:8765` の TTS サーバです。  
目的は、RenCrow からの TTS 呼び出しを正常化し、**音声出力は常にブラウザのみ**で再生できるようにすることです。

RenCrow 側の browser-only 対応は実装済みです。  
そのため、サーバ側では `ffplay` やローカルスピーカー再生を前提にせず、**ブラウザが取得できる `audio_url` または解決可能な `audio_path` を返すこと**を前提にしてください。

## 1. 現在確認できている状態

- `GET /health/ready` は成功します
- `POST /synthesize` は `500 Internal Server Error` です
- `GET /api/models_info` は `404 Not Found` です
- `POST /api/g2p` は `404 Not Found` です

この状態から、まずは **Bridge 契約** を満たすように修正してください。  
`SBV2 direct` 契約は代替案であり、最初は不要です。

## 2. 最優先で直してほしい点

1. `POST /synthesize` の `500 Internal Server Error` を解消すること
2. `WS /sessions` を正常動作させること
3. `audio_chunk_ready` と `/synthesize` のレスポンスに `audio_url` または `audio_path` を返すこと
4. WebSocket 接続後に `EOF` で落ちないようにすること

## 3. 必須要件

### 3.1 Readyチェック API

- エンドポイントは `GET /health/ready`
- 正常時は `HTTP 200`
- レスポンスは JSON
- `status` は必ず `"ready"`
- `voices` には使用可能な `voice_id` 一覧を返す
- `female_01` を利用可能にする

レスポンス例:

```json
{
  "status": "ready",
  "voices": ["female_01", "male_01", "mio"]
}
```

### 3.2 Fallback音声生成 API

- エンドポイントは `POST /synthesize`
- `Content-Type: application/json` を受け付ける
- 少なくとも `text` と `voice_id` を受け取る
- `emotion_state` は受け取っても無視して構わない
- 正常時は `HTTP 2xx`
- レスポンスには **`audio_path` または `audio_url` のどちらか必須**
- 可能なら `audio_url` を返す
- `audio_path` を返す場合は、RenCrow またはブラウザ側で最終的に取得できる導線を用意する
- `500 Internal Server Error` を返さないように修正する

リクエスト例:

```json
{
  "text": "こんにちは",
  "voice_id": "female_01",
  "emotion_state": {
    "primary_emotion": "calm"
  }
}
```

レスポンス例:

```json
{
  "text": "こんにちは",
  "audio_path": "/tmp/oneshot.wav",
  "audio_url": "http://192.168.1.33:8765/audio/oneshot.wav"
}
```

### 3.3 WebSocketセッション API

- エンドポイントは `WS /sessions`
- RenCrow は WebSocket 接続後に `session_start` を送る
- その後、RenCrow は `text_delta` を複数回送る
- 最後に RenCrow は `session_end` を送る
- サーバは `audio_chunk_ready` を 0 回以上返す
- 最後に `session_completed` を返す
- 異常時は `type=error` を返す
- `audio_chunk_ready` には **`audio_path` または `audio_url` のどちらか必須**
- `chunk_index` は `0,1,2...` のように単調増加
- 接続確立後に即切断しない

クライアントから送る `session_start` 例:

```json
{
  "type": "session_start",
  "session_id": "sess-123",
  "response_id": "resp-123",
  "character": "mio",
  "voice_id": "female_01",
  "speech_mode": "conversational",
  "context": {
    "event": "conversation",
    "urgency": "normal",
    "conversation_mode": "chat",
    "user_attention_required": false,
    "user_waiting_time_sec": 0
  }
}
```

クライアントから送る `text_delta` 例:

```json
{
  "type": "text_delta",
  "session_id": "sess-123",
  "seq": 1,
  "text": "こんにちは",
  "emitted_at": "2026-04-06T12:00:00Z",
  "emotion_state": {
    "primary_emotion": "warm",
    "prosody": {
      "speed": 1.0,
      "pitch": 0.0,
      "pause": 0.1,
      "expressiveness": 0.6
    }
  }
}
```

クライアントから送る `session_end` 例:

```json
{
  "type": "session_end",
  "session_id": "sess-123",
  "is_final": true
}
```

サーバから返す `audio_chunk_ready` 例:

```json
{
  "type": "audio_chunk_ready",
  "chunk_index": 0,
  "text": "こんにちは",
  "audio_path": "/tmp/chunk0.wav",
  "audio_url": "http://192.168.1.33:8765/audio/chunk0.wav",
  "pause_after": "short"
}
```

サーバから返す `session_completed` 例:

```json
{
  "type": "session_completed",
  "session_id": "sess-123"
}
```

サーバから返す `error` 例:

```json
{
  "type": "error",
  "code": "synthesis_failed",
  "message": "detail message"
}
```

## 4. 音声ファイルの扱い

- `audio_url` を返す場合は、ブラウザから直接取得できる URL にしてください
- `audio_url` は `http://192.168.1.33:8765/...` のような到達可能 URL にしてください
- `audio_path` を返す場合は、サーバ内の一時ファイルでも構いませんが、RenCrow 側またはブラウザ側で最終的に取得できる導線が必要です
- 可能なら `audio_url` を優先してください
- Windows 形式パス `cache\\x.wav` を返す場合は、実在ファイルに解決できるようにしてください

## 5. 話者IDの要件

- `female_01` を受け付けてください
- `male_01` を受け付けられると望ましいです
- `GET /health/ready` の `voices` に、受け付け可能な `voice_id` を正しく返してください
- `voice_id` が未対応なら、明確な `4xx` とメッセージを返してください

## 6. タイムアウトと安定性

- 接続直後に WebSocket を閉じないでください
- `session_start` を受けた後、`text_delta` を待てるようにしてください
- `text_delta` を受けたら、タイムアウトせずに `audio_chunk_ready` を返してください
- `session_end` を受けたら、残りチャンクを返した後で `session_completed` を返してください
- 現在の `EOF` や `i/o timeout` が出ないように修正してください

## 7. 事前動作確認項目

- `GET /health/ready` が `200` を返すこと
- `status` が `"ready"` であること
- `voices` に `female_01` が含まれること
- `POST /synthesize` が `2xx` を返すこと
- `POST /synthesize` のレスポンスに `audio_path` または `audio_url` が入ること
- `WS /sessions` で接続成功すること
- `session_start -> text_delta -> session_end` を受けて `audio_chunk_ready -> session_completed` を返せること
- 異常時に `type=error` を返せること
- `500 Internal Server Error` を解消すること

## 8. 代替案

もし Bridge 契約の実装が難しい場合のみ、代替として `SBV2 direct` 契約を実装してください。

その場合の最低要件:

- `GET /api/models_info`
- `POST /api/g2p`
- `POST /api/synthesis`
- `POST /api/synthesis` は `audio/wav` バイナリを直接返す

ただし、この代替案は現状の `8765` では見えていないため、**まずは Bridge 契約の修正を優先**してください。

## 9. 補足

- RenCrow 側の browser-only 対応は実装済みです
- サーバ側でローカル再生する設計にはしないでください
- ブラウザが取得できる `audio_url` を返せる構成が最も望ましいです
