# SBV2 サーバ担当向け確認文（RenCrow Live 連携）

RenCrow Live から `Win11-HP01` 上の SBV2 を利用するため、**次のどの契約に相当するか**を確認したいです。

- **PicoClaw 互換**: `POST …/synthesis`（JSON で `audio_path` 返却）＋任意で **Bridge**（`/health/ready`・`WS /sessions`）
- **Style-Bert-VITS2 Editor（`server_editor`）互換**: `POST …/api/synthesis`（**レスポンスは WAV バイナリ**、`g2p` / `models_info` 前提）

以下をそのまま担当者へ送れます。

---

SBV2サーバ連携について確認させてください。  
RenCrow Live から利用したいため、現在のSBV2サーバが **どの API 形態か**教えてください。

## 1. 現在動いているSBV2サーバの種類
1. 今動いているのは `server_editor`（Style-Bert-VITS2 Editor）のブラウザ UI ですか？
2. それとは別に、外部アプリから呼べる HTTP API はありますか？
3. 待受 **ホスト**と **ポート**を教えてください（例: `0.0.0.0:8000`）
4. API のパスに **`/api` プレフィックス**はありますか？（例: `/api/synthesis`）

## 2-A. PicoClaw 互換の `POST /synthesis` について（該当する場合）
1. **`POST /synthesis`**（ルート直下、JSON 入出力）はありますか？
2. ある場合、**フル URL** を教えてください  
   - 例: `http://win11-hp01:5000/synthesis`  
   - 例: `https://win11-hp01.tailxxxxx.ts.net:5000/synthesis`
3. 次の **リクエスト JSON** を受けられますか？

```json
{
  "text": "こんにちは",
  "voice_id": "mio",
  "emotion": "calm",
  "speed": 1.0,
  "pitch": 0.0
}
```

4. 次の **レスポンス JSON** を返せますか？

```json
{
  "audio_path": "/tmp/sbv2.wav",
  "duration_ms": 1234,
  "voice_id": "mio"
}
```

5. `audio_path` は必ず返りますか？
6. `voice_id` に指定できる一覧を教えてください
7. **約 20 秒以内**に応答できそうですか？

## 2-B. Style-Bert-VITS2 `server_editor` 互換について（該当する場合）
※ RenCrow 同梱の標準はこちらに近いです。

1. **`POST …/api/synthesis`** はありますか？（パス全体を教えてください）
2. **レスポンス**は **JSON ではなく `audio/wav` のバイナリ**ですか？
3. **`GET …/api/models_info`**（モデル名・重みファイル名・話者一覧）はありますか？
4. **`POST …/api/g2p`**（テキストからモーラ列）はありますか？
5. 合成時の `modelFile` は **`models_info` の `files[]` の文字列をそのまま**渡す形式ですか？
6. **約 20〜120 秒**程度の初回・長文でも運用上問題ない見込みですか？

## 3. TTS Client Bridge API について
1. `GET /health/ready` はありますか？
2. ある場合、**フル URL** を教えてください  
   - 例: `http://win11-hp01:8765/health/ready`
3. 次の形式の **JSON** ですか？

```json
{
  "status": "ready",
  "voices": ["female_01", "male_01", "mio"]
}
```

4. WebSocket **`/sessions`** はありますか？
5. ある場合、**フル URL** を教えてください  
   - 例: `ws://win11-hp01:8765/sessions`
6. 次の流れに対応していますか？
   - `session_start` / `text_delta` / `session_end`
   - `audio_chunk_ready` / `session_completed`

## 4. ネットワーク・ブラウザ（CORS）
1. 当方 PC から `Win11-HP01` の SBV2 へ **Tailscale** で届きますか？
2. **接続先のホスト名または URL** を教えてください（HTTP/HTTPS、ポート含む）  
   - 例: `http://win11-hp01.tailb07d8d.ts.net:8000/api/`
3. **Windows Defender Firewall** で、上記ポートの **受信許可**はありますか？
4. **別 PC のブラウザ**から SBV2 の API を **直接 `fetch`** する必要がある場合、**CORS**（許可オリジン）は設定済みですか？（サーバー間のみなら不要な場合があります）

## 5. 動作確認
可能なら、実際に試した結果を教えてください。

### 2-A（PicoClaw 互換 `/synthesis`）がある場合
- `curl` または PowerShell の結果
- **実レスポンス**の例（JSON）

### 2-B（`server_editor` 互換 `/api/...`）がある場合
- `GET /api/version` または `GET /api/models_info` の結果
- `POST /api/synthesis` の **HTTP ステータス**と **`Content-Type`**、**ボディ先頭数バイト**（WAV なら `RIFF`）

### §3（Bridge）がある場合
- `GET /health/ready` のレスポンス例
- WebSocket 接続可否と、簡単な送受信例

## 6. 結論として知りたいこと
現在の SBV2 は、次の **どれに該当**しますか？（複数可）

1. **PicoClaw 互換**: `POST /synthesis`（JSON で `audio_path` 等）がそのまま使える  
2. **Bridge**: `GET /health/ready` と `WS /sessions` が使える  
3. **Editor 互換**: `POST /api/synthesis`（WAV 返却）＋`g2p` / `models_info` 等  
4. **1 と 2 の両方**  
5. **3 のみ**（PicoClaw 形式の `/synthesis` は無い）  
6. **ブラウザ UI のみ**で、外部向け API は未整備／別途予定

---

## 相手から最低限ほしい回答（こちら用メモ）

1. **使えるベース URL**（スキーム・ホスト・ポート・`/api` の有無）  
2. **上記 6 の結論番号**と、**実レスポンス例**（JSON または WAV の `Content-Type`）  
3. **Tailscale / ファイアウォール**で当方から届くか  

返答後、RenCrow Live 側の設定値を確定する。
