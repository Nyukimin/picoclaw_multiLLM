# RenCrow_TTS/STT 復旧依頼（2026-04-28）

## 現象
- クライアント側で音声ボタンが赤のまま戻らないことがあります。
- Chat 側 `picoclaw` は稼働中（再起動済み）ですが、音声疎通は失敗継続しています。

## 観測結果（2026-04-28 12:28頃 JST）
- STT health `https://192.168.1.36:8090/health` が `000`（到達不可）
- STT 推論 `http://192.168.1.36:8080/inference` が `connection refused`
- E2E では `speech_start` 後に `error`、`final` が返らない

## 依頼
`192.168.1.36` 側で RenCrow_TTS/STT（特に Whisper `8080`）プロセスの再起動と疎通確認をお願いします。

## サーバ側で実施してほしい確認
- サービス/プロセス再起動（TTS/STT/Whisper）
- ポート待受確認（`8080`, `8090`, `8765`）
- API 確認

```bash
curl -sS http://127.0.0.1:8080/health
curl -k -sS https://127.0.0.1:8090/health
curl -k -sS https://127.0.0.1:8090/ready
```

- 推論確認（最重要）

```bash
curl -sS -X POST "http://127.0.0.1:8080/inference" \
  -F "file=@<known-good.wav>" \
  -F "response_format=json"
```

- 失敗時はサーバログ共有をお願いします（再起動直後〜失敗時刻）。

## 合格条件
- `8080/inference` が成功して `text` を返す
- `8090/health` と `8090/ready` が安定して `200`
- その後こちらの E2E で `inference_success: 1/1` かつ `ws_success: 1/1`
