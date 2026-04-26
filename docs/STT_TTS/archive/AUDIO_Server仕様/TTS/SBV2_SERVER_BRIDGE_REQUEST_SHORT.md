# SBV2サーバ担当者向け 修正依頼（短縮版）

対象サーバは `192.168.1.33:8765` の TTS サーバです。  
RenCrow 側の browser-only 対応は実装済みです。音声出力はサーバローカル再生ではなく、**最終的にブラウザ再生できる形**にしてください。

## 現在確認している状態

- `GET /health/ready` は成功
- `POST /synthesize` は `500 Internal Server Error`
- `GET /api/models_info` と `POST /api/g2p` は `404 Not Found`
- そのため、まずは **Bridge 契約** の実装・修正をお願いします

## 最優先で直してほしい点

1. `POST /synthesize` の `500` を解消し、正常時は `2xx` を返すこと
2. `WS /sessions` を正常動作させること
3. `audio_chunk_ready` と `/synthesize` のレスポンスに `audio_url` または `audio_path` を返すこと
4. 接続後に `EOF` や即切断にならないこと

## 必須要件

### 1. `GET /health/ready`

- `200 OK`
- JSON を返す
- `status` は必ず `"ready"`
- `voices` に利用可能な `voice_id` を含める
- 少なくとも `female_01` を含める

レスポンス例:

```json
{
  "status": "ready",
  "voices": ["female_01", "male_01", "mio"]
}
```

### 2. `POST /synthesize`

- `Content-Type: application/json`
- 少なくとも `text` と `voice_id` を受け取る
- `emotion_state` は受け取っても無視して可
- 正常時は `2xx`
- **`audio_url` または `audio_path` を必ず返す**
- ブラウザ再生のため、可能なら `audio_url` を優先
- サーバ側で `ffplay` やローカル再生を前提にしない

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

### 3. `WS /sessions`

- WebSocket 接続後に即切断しない
- `session_start` を受けた後、`text_delta` を待てること
- `text_delta` を受けたら `audio_chunk_ready` を 0 回以上返すこと
- 最後に `session_completed` を返すこと
- 異常時は `type=error` を返すこと
- `audio_chunk_ready` には `audio_url` または `audio_path` を含めること
- `chunk_index` は `0,1,2...` で単調増加

## 話者ID要件

- `female_01` を必須対応
- 可能なら `male_01` も対応
- 未対応 `voice_id` には明確な `4xx` とメッセージを返す

## 事前確認してほしい項目

1. `GET /health/ready` が `200` で `status=ready` を返す
2. `voices` に `female_01` が含まれる
3. `POST /synthesize` が `2xx` で `audio_url` または `audio_path` を返す
4. `WS /sessions` で `session_start -> text_delta -> session_end` に対し `audio_chunk_ready -> session_completed` を返せる
5. 異常時に `type=error` を返せる
6. `500 Internal Server Error` が解消されている

## 補足

- `audio_url` は `http://192.168.1.33:8765/...` のようにブラウザから取得可能な URL にしてください
- `audio_path` を返す場合は、RenCrow またはブラウザが最終的に取得できる導線を用意してください
- `SBV2 direct` 契約は代替案です。**まずは Bridge 契約を優先**してください
