# STT 現状報告・確認依頼（クライアント側 → サーバー側）
作成: 2026-04-14

---

## 1. 今日判明したこと（クライアント側の問題）

### session_id が (unknown) だった本当の原因

`session_id: (unknown)` はサーバー側の問題ではありませんでした。  
クライアント側（RenCrow Go サーバー）の実装バグです。

#### 実際に起きていた経路

```
Browser
  ↓ wss://fujitsu-ubunts:18790/stt-ws
RenCrow Chat Server（Go, fujitsu-ubunts）← ここが /stt-ws を自前処理していた
  ↓ HTTP POST /inference（直接）
Whisper（Win11-HP01 :8080）
```

**voice-bridge（:8090）は経路に存在していませんでした。**

Go の `handleSTTWebSocket` が voice-bridge を経由せず Whisper を直接呼んでおり、
`session_info` イベントを送出していなかったため `session_id` が `(unknown)` になっていました。

サーバー側（voice-bridge）は正しく `session_info` を送出していたことが、
過去のログ（`ws_session_open`）から確認できています。問題はクライアント側にありました。

---

## 2. クライアント側で実施した修正（本日）

### Go サーバーに voice-bridge プロキシを追加

`/stt-ws` エンドポイントを voice-bridge への透過 WebSocket プロキシに変更しました。

```
# 設定（環境変数）
STT_GATEWAY_URL=ws://192.168.1.36:8090/stt-ws
```

設定後の経路：
```
Browser
  ↓ wss://fujitsu-ubunts:18790/stt-ws
RenCrow Chat Server（Go）← 透過プロキシに変更
  ↓ ws://192.168.1.36:8090/stt-ws
voice-bridge（Win11-HP01 :8090）
  ↓ HTTP POST /inference
Whisper（Win11-HP01 :8080）
```

---

## 3. サーバー側への確認事項

### Q1. voice-bridge は現在 :8090 で動いていますか？

以下のコマンドで確認をお願いします：
```powershell
netstat -ano | findstr ":8090"
# または
Get-Process node | Select-Object Id, CPU, WorkingSet
```

### Q2. voice-bridge が受け付ける WebSocket パスは `/stt-ws` ですか？

`tmp/bridge/server.js`（新バージョン）では `/stt-ws` と `/ws` の両方を受け付ける実装になっています。
現行の voice-bridge のパスを確認してください。

### Q3. voice-bridge の新バージョンへの差し替えはお済みですか？

以下のファイルを Win11-HP01 の voice-bridge ディレクトリに上書きコピーしてください：
- `tmp/bridge/server.js`
- `tmp/bridge/stt-gateway-contract.js`

差し替え後、`node_modules` に `@ricky0123/vad-node` が必要です（新規依存）。
既存の `node_modules` にない場合は `npm install @ricky0123/vad-node` が必要です。

---

## 4. 過去ログの証拠（サーバーは正しく動いていた）

以下のログから、voice-bridge は正しく `session_info` を送出していたことが確認できています：

```
[stt] {"ts":"2026-04-13T08:24:55.886Z","event":"ws_session_open","session_id":"sess-mnwxg0gd-w2xphi"}
[stt] {"ts":"2026-04-13T08:24:56.464Z","event":"ws_final_emit","session_id":"sess-mnwxg0gd-w2xphi","source":"provider","text":"...","text_len":28}
```

---

## 5. 認識品質について

### e2e probe と実際の認識結果の乖離

| 経路 | テキスト |
|---|---|
| ブラウザ（ストリーミング） | `あお。`（正解） |
| e2e probe（WAV 一括送信） | `AEUAO.O.HAYO.O.ございます。 天使。`（不正解） |

同一音声（`tmp/client_stt_input_latest.wav`）を使っているにも関わらず結果が異なります。

**考えられる原因**:  
WAV 録音の先頭に無音・ノイズが含まれており、一括送信すると Whisper がノイズ部分を誤認識する。
ストリーミング経路では voice-bridge の VAD がノイズ区間を除外しているため正しく認識できていた可能性があります。

voice-bridge 経由の経路が復活すれば、この問題も自然に改善される見込みです。

---

## 6. 現在の未解決事項

| 項目 | 状態 |
|---|---|
| `STT_GATEWAY_URL` の設定 | 設定待ち（picoclaw 再起動が必要） |
| Win11-HP01 の voice-bridge 起動確認 | 確認待ち |
| voice-bridge 新バージョン差し替え | 依頼中 |
| e2e probe の認識精度 | voice-bridge 経由復活後に再評価 |
