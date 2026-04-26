# SBV2サーバ担当者向け 修正依頼（最短版）

`192.168.1.33:8765` の TTS サーバについて、まずは **Bridge 契約** を満たすよう修正をお願いします。

現状:

- `GET /health/ready` は成功
- `POST /synthesize` は `500 Internal Server Error`
- `WS /sessions` は正常動作していません
- `GET /api/models_info` と `POST /api/g2p` は `404`

必須対応:

1. `GET /health/ready` を `200` + `{"status":"ready","voices":["female_01", ...]}` で返す
2. `POST /synthesize` を `500` にせず、`2xx` + `audio_url` または `audio_path` を返す
3. `WS /sessions` で `session_start -> text_delta -> session_end` を受け、`audio_chunk_ready -> session_completed` を返す
4. `voice_id=female_01` を受け付ける
5. 接続後に即切断せず、`EOF` や `i/o timeout` が出ないようにする

重要:

- 音声出力はサーバローカル再生ではなく、**ブラウザ再生用の `audio_url` または `audio_path`** を返してください
- `ffplay` やローカルスピーカー再生前提にはしないでください
- `SBV2 direct` は代替案です。まずは Bridge 契約を優先してください
