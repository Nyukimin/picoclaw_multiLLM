# SBV2 サーバ担当向け確認文（RenCrow Live 連携）

RenCrow Live から `Win11-HP01` 上の SBV2 を利用するため、**現在の SBV2 サーバが `POST /synthesis` か `GET /health/ready + WS /sessions` のどちらか、または両方を提供しているか**を確認する質問です。  
以下をそのまま担当者へ送れます。

---

SBV2サーバ連携について確認させてください。  
RenCrow Live から利用したいため、現在のSBV2サーバがどのAPIを提供しているか確認したいです。

## 1. 現在動いているSBV2サーバの種類
1. 今動いているのは `server_editor` のブラウザUIですか？
2. それとは別に、外部アプリから呼べる API サーバがありますか？
3. 現在の待受ホストとポートを教えてください
   - 例: `0.0.0.0:5000`
   - 例: `0.0.0.0:8000`
   - 例: `0.0.0.0:8765`

## 2. `/synthesis` API について
1. `POST /synthesis` はありますか？
2. ある場合、フルURLを教えてください
   - 例: `http://win11-hp01:5000/synthesis`
3. リクエストJSONは次の形で受けられますか？

```json
{
  "text": "こんにちは",
  "voice_id": "mio",
  "emotion": "calm",
  "speed": 1.0,
  "pitch": 0.0
}
```

4. レスポンスJSONは次の形で返せますか？

```json
{
  "audio_path": "/tmp/sbv2.wav",
  "duration_ms": 1234,
  "voice_id": "mio"
}
```

5. `audio_path` は必ず返りますか？
6. `voice_id` に指定できる一覧を教えてください
7. タイムアウト20秒以内で応答できそうですか？

## 3. TTS Client Bridge API について
1. `GET /health/ready` はありますか？
2. ある場合、フルURLを教えてください
   - 例: `http://win11-hp01:8765/health/ready`
3. レスポンスは次の形式ですか？

```json
{
  "status": "ready",
  "voices": ["female_01", "male_01", "mio"]
}
```

4. WebSocket `/sessions` はありますか？
5. ある場合、フルURLを教えてください
   - 例: `ws://win11-hp01:8765/sessions`
6. 以下の流れに対応していますか？
   - `session_start`
   - `text_delta`
   - `session_end`
   - `audio_chunk_ready`
   - `session_completed`

## 4. ネットワーク到達性
1. このPCから `Win11-HP01` のSBV2へ Tailscale 経由でアクセスできますか？
2. Tailscale 名またはIPでの接続先を教えてください
   - 例: `http://win11-hp01.tailb07d8d.ts.net:5000/synthesis`
3. Windows Defender Firewall で必要ポートは受信許可されていますか？

## 5. 動作確認
可能なら、以下を実際に試した結果を教えてください。

### `/synthesis` がある場合
- curl または PowerShell での疎通確認結果
- 実際のレスポンス例

### `/health/ready` がある場合
- `GET /health/ready` のレスポンス例

### `/sessions` がある場合
- WebSocket 接続できるか
- サンプルメッセージで音声生成まで通るか

## 6. 結論として知りたいこと
最終的に、現在のSBV2サーバは次のどれですか？

1. `POST /synthesis` でそのまま利用可能
2. `GET /health/ready + WS /sessions` で利用可能
3. 両方利用可能
4. `server_editor` UI だけで、RenCrow Live から使うAPIはまだ無い

---

## 相手から最低限ほしい回答（メモ）

1. 使える URL  
2. 対応している API 形式  
3. 実レスポンス例  

返答後、RenCrow Live 側の設定値を確定する。
