# 別 PC から Whisper（whisper-server）を RenCrow で使う

RenCrow の想定は **[ブラウザ] → WebSocket → [voice-bridge] → HTTP POST → [whisper-server :8080]** である。  
ブラウザが Whisper に直接触れないため、**voice-bridge が動く PC** と **Whisper が動く PC** を分けたときの整理をする。

一次情報: [Whisper実装仕様.md](../Whisper実装仕様.md)、`webui/voice-bridge/server.js` の `WHISPER_URL`。

---

## 状況別

### 1. Whisper と voice-bridge が同じ PC・ブラウザだけ別 PC

- Whisper は **`http://127.0.0.1:8080/inference`**（`WHISPER_URL` 未設定時の既定）。
- ブラウザは **`http://<そのPCのLAN IP>:8090`** で voice-bridge に接続する。
- **voice-bridge 側 PC**のファイアウォールで **TCP 8090（voice-bridge のポート）** を LAN から許可する。

### 2. Whisper だけ別 PC（推奨・仕様どおり）

- **Whisper サーバ PC**: `start-whisper.ps1` で起動。待受 **`0.0.0.0:8080`**。
- **voice-bridge を動かす PC**: 環境変数 **`WHISPER_URL`** に Whisper PC の URL を指定する（下記「対処パターン A」）。
- **Whisper PC**のファイアウォールで **TCP 8080 受信**を許可する（voice-bridge から届くため）。

### 3. 別 PC のブラウザから `http://<WhisperのIP>:8080` に直接 `fetch` する

- RenCrow の主経路ではない（[Whisper実装仕様.md](../Whisper実装仕様.md) §1）。
- **CORS**: whisper.cpp の応答ヘッダ次第で **ブラウザがブロック**することがある。
- **推奨**: voice-bridge 経由にするか、同一オリジンのプロキシを挟む。

### 4. **HTTPS** のページから **HTTP** の whisper-server を呼ぶ

- ブラウザ直叩きでは **Mixed Content** でブロックされやすい。
- **voice-bridge 経由**なら、ブラウザは **HTTPS の voice-bridge** にだけ接続し、voice-bridge → Whisper は **LAN 内 HTTP** でよい（`server-https.js` 等）。

---

## 対処パターン（推奨順）

### A. 別 PC に Whisper を置く（最も素直）

**voice-bridge を起動する前**に、voice-bridge 側で:

```powershell
$env:WHISPER_URL = "http://<WhisperサーバのLAN IP>:8080/inference"
```

例:

```text
WHISPER_URL=http://192.168.1.50:8080/inference
```

- voice-bridge の `fetch` は **サーバー間通信**のため **CORS は通常不要**（ブラウザが Whisper に直接アクセスしない限り）。
- ブラウザは従来どおり **`http://<voice-bridge PC>:8090`** 等へ接続。

### B. インターネット越し・Tailscale

- **VPN / Tailscale** で Whisper PC に届くホスト名（または IP）を `WHISPER_URL` に書く。
- 無防備に **公網に 8080 を晒さない**。必要なら **TLS 終端・認証**（リバースプロキシ）。

### C. ブラウザから whisper-server に直接 POST したい場合

- **multipart** `POST /inference` は技術的には可能だが、CORS・運用の負担が増える。
- **C**: 同一オリジンの **リバースプロキシ**で `/inference` を whisper に転送する。

---

## まとめ

| やりたいこと | 最低限必要なこと |
|--------------|------------------|
| Whisper だけ別マシン | Whisper PC: **8080 待受 + ファイアウォール許可**。voice-bridge PC: **`WHISPER_URL=http://<Whisper IP>:8080/inference`** |
| ブラウザだけ別マシン（ASR はローカル） | voice-bridge PC の **8090（等）を LAN 開放**。`WHISPER_URL` は既定のまま |
| ブラウザから Whisper に直接 | **非推奨**。CORS 対応が必要になりやすい → **voice-bridge 経由**を推奨 |

一次情報: `WHISPER_URL` 既定は `webui/voice-bridge/server.js`、Whisper の待受は `ops/audioio/start-whisper.ps1`（`0.0.0.0:8080`）。
