# Win11-HP01 への音声サーバ移管手順

## 概要

本書は、RenCrow Live の音声系サーバを旧 `kawaguchike-llm` 系構成から **`Win11-HP01`** へ移管するための運用メモである。

今回の前提は次の通り。

- ブラウザは不定の端末から利用する
- `RenCrow Live` サーバはこのPCで起動する
- `TTS` サーバは `Win11-HP01` 上の `SBV2`
- `STT` サーバは `Win11-HP01` 上の `Whisper`
- 旧 `kawaguchike-llm:8765` は今後使わない

本書の目的は、次の3点を明確にすること。

1. `Win11-HP01` 側で何を待ち受ける必要があるか
2. このPC側でどの設定を `Win11-HP01` に向け替えるか
3. 移管完了を何で確認するか

---

## 1. 現在の整理

### 1.1 旧構成

- `RenCrow Live` の TTS 本線は **TTS Client Bridge**
- 旧設定では `tts.http_base_url` / `tts.ws_url` が `kawaguchike-llm` 側を向いていた
- `tts.sbv2.enabled: false` のため、`POST /synthesis` 直呼び出しは主経路ではなかった

### 1.2 新構成

- `RenCrow Live` サーバ: このPC
- `Whisper` サーバ: `Win11-HP01`
- `SBV2` サーバ: `Win11-HP01`
- ブラウザは常にこのPCの `RenCrow Live` に接続する

### 1.3 重要な前提

`SBV2` が `Win11-HP01` で動いていることと、`RenCrow Live` がその `SBV2` を使えることは別問題である。

`RenCrow Live` が現在の本線で必要とするのは、少なくとも次の **Bridge 契約**である。

- `GET /health/ready`
- `POST /synthesize`
- `WS /sessions`

一方、`server_editor` 単体で確認できているのは次である。

- `GET /api/models_info`
- `POST /api/g2p`
- `POST /api/synthesis` (`audio/wav` バイナリ返却)

そのため、**`server_editor(:8000/api)` に届くだけでは RenCrow Live 本線の移管完了にはならない**。

---

## 2. 既知の接続先

### 2.1 Win11-HP01

- Tailscale FQDN: `win11-hp01.tailb07d8d.ts.net`
- Tailscale IP: `100.96.186.107`

### 2.2 Whisper

`Whisper` は `voice-bridge` から HTTP POST される前提で使う。

- 想定URL: `http://win11-hp01.tailb07d8d.ts.net:8080/inference`
- 既知資料上の待受: `0.0.0.0:8080`

### 2.3 SBV2 server_editor

現在確認できている `SBV2` API は `server_editor` 互換である。

- ベースURL: `http://<host>:8000/api`
- 成功時 `POST /api/synthesis` は `audio/wav` を返す
- `GET /health/ready`、`POST /synthesize`、`WS /sessions` は確認できていない

---

## 3. 移管後の推奨アーキテクチャ

```text
[Browser anywhere]
        |
        v
[This PC: RenCrow Live]
   |                  |
   | voice-bridge     | TTS Client Bridge
   v                  v
[Whisper on Win11]  [SBV2/Bridge on Win11]
  :8080/inference     :<bridge-port>
```

補足:

- ブラウザは `Win11-HP01` を直接叩かず、このPCの `RenCrow Live` を利用する
- `Whisper` はサーバ間通信なので、通常 CORS は不要
- `SBV2` は `RenCrow Live` の本線では Bridge 契約が必要
- `server_editor` のブラウザUIを直接使う場合だけ CORS / Mixed Content が論点になる

---

## 4. Win11-HP01 側で必要なもの

### 4.1 Whisper

必要条件:

- `Whisper` が `0.0.0.0:8080` で待受していること
- Windows Defender Firewall で `TCP 8080` 受信許可があること
- Tailscale 名またはIPでこのPCから到達できること

使用先:

- `WHISPER_URL=http://win11-hp01.tailb07d8d.ts.net:8080/inference`

### 4.2 SBV2

`SBV2` 自体は `Win11-HP01` 上で動くが、`RenCrow Live` から使うには **どの契約で公開するか** を確定する必要がある。

### A. 推奨: Bridge 契約を出す

必要なもの:

- `GET /health/ready`
- `POST /synthesize`
- `WS /sessions`

この場合、`RenCrow Live` 側は `tts.http_base_url` / `tts.ws_url` をそのURLに向ければよい。

### B. 補助: SBV2 直呼び出し契約を出す

必要なもの:

- `POST /synthesis`
- レスポンス JSON に `audio_path` を含むこと

これは `tts.sbv2.enabled: true` のときの別経路であり、現在の本線ではない。

### C. `server_editor` のみ

現時点で確認できているのはこの状態である。

この場合:

- `POST /api/synthesis` は使える
- ただし `RenCrow Live` 本線の `tts.http_base_url` / `tts.ws_url` にはそのまま使えない
- Bridge 相当の別プロセスまたは別APIが必要になる

---

## 5. このPC側の設定変更点

現在の設定抜粋:

```yaml
tts:
  http_base_url: "http://192.168.1.33:8765"
  ws_url: "ws://192.168.1.33:8765/sessions"
  sbv2:
    enabled: false
    base_url: "http://127.0.0.1:5000/synthesis"
```

### 5.1 Whisper

`voice-bridge` 起動前に `WHISPER_URL` を `Win11-HP01` に向ける。

```powershell
$env:WHISPER_URL = "http://win11-hp01.tailb07d8d.ts.net:8080/inference"
```

### 5.2 TTS Bridge を `Win11-HP01` に向ける

`Win11-HP01` 上に Bridge 契約がある場合、次を差し替える。

```yaml
tts:
  http_base_url: "http://<win11-hp01-bridge-host>:<bridge-port>"
  ws_url: "ws://<win11-hp01-bridge-host>:<bridge-port>/sessions"
```

例:

```yaml
tts:
  http_base_url: "http://win11-hp01.tailb07d8d.ts.net:8765"
  ws_url: "ws://win11-hp01.tailb07d8d.ts.net:8765/sessions"
```

### 5.3 SBV2 直呼び出しを使う場合

`SBV2` 直呼び出しを有効にするなら次を設定する。

```yaml
tts:
  sbv2:
    enabled: true
    base_url: "http://<win11-hp01-host>:5000/synthesis"
    voice_id: "mio"
    timeout_sec: 20
```

ただし、`Win11-HP01` 側に `POST /synthesis` 契約が存在すると確定してから有効化すること。

---

## 6. ネットワーク要件

### 6.1 Tailscale

このPCと `Win11-HP01` の双方で Tailscale が有効であること。

接続には次のいずれかを使う。

- `win11-hp01.tailb07d8d.ts.net`
- `100.96.186.107`

### 6.2 Windows Defender Firewall

最低限、次の受信許可を確認する。

- `TCP 8080` (`Whisper`)
- `TCP 8000` (`server_editor` を直接使う場合)
- `TCP <bridge-port>` (`TTS Bridge` を使う場合)
- `TCP 5000` (`/synthesis` を使う場合)

### 6.3 CORS

- `Whisper` のサーバ間通信では通常不要
- `RenCrow Live` の本線で Bridge をサーバ間利用する場合も通常不要
- `server_editor` をブラウザから直接 `fetch` する場合だけ `origins` の調整が必要

---

## 7. 移管手順

1. `Win11-HP01` 上で `Whisper` を起動する
2. `Whisper` の `8080` が Tailscale 経由で到達できることを確認する
3. `Win11-HP01` 上で `SBV2` の公開方式を確定する
4. `Bridge` を使う場合は `GET /health/ready`、`POST /synthesize`、`WS /sessions` が使えることを確認する
5. このPCの `WHISPER_URL` を `Win11-HP01` に向ける
6. このPCの `tts.http_base_url` / `tts.ws_url` を `Win11-HP01` に向ける
7. `RenCrow Live` を起動して TTS / STT の両方を確認する
8. 旧 `kawaguchike-llm` 依存が残っていないことを確認する

---

## 8. 動作確認チェックリスト

### 8.1 Whisper

- [ ] `http://win11-hp01.tailb07d8d.ts.net:8080/inference` に到達できる
- [ ] `voice-bridge` から音声認識が通る
- [ ] ブラウザは `Whisper` を直接叩いていない

### 8.2 TTS Bridge

- [ ] `GET /health/ready` が `200` を返す
- [ ] `POST /synthesize` が `audio_path` または `audio_url` を返す
- [ ] `WS /sessions` が接続できる
- [ ] `session_start -> text_delta -> session_end` で音声生成が完走する

### 8.3 SBV2

- [ ] `Win11-HP01` 上で `SBV2` のモデルがロードされている
- [ ] 実際に音声WAVが生成される
- [ ] 必要な `voice_id` が利用できる

### 8.4 RenCrow Live

- [ ] このPCのブラウザから利用できる
- [ ] 別端末のブラウザからも利用できる
- [ ] TTS は `Win11-HP01` を使っている
- [ ] STT は `Win11-HP01` を使っている
- [ ] 旧 `192.168.1.33:8765` への依存がない

---

## 9. 未確定事項

移管完了前に、次を確定する必要がある。

1. `Win11-HP01` で使う TTS Bridge のベースURL
2. `Win11-HP01` 側で `WS /sessions` を提供するかどうか
3. `Win11-HP01` 側で `POST /synthesis` を提供するかどうか
4. `server_editor` のみで運用するのか、Bridge 契約を別途提供するのか

---

## 10. ロールバック

`Win11-HP01` 移管に問題がある場合、次で旧構成へ戻せる。

1. このPCの `tts.http_base_url` を旧 `kawaguchike-llm` 向けに戻す
2. このPCの `tts.ws_url` を旧 `kawaguchike-llm` 向けに戻す
3. `WHISPER_URL` を旧構成に戻す
4. `RenCrow Live` を再起動して疎通確認する

ただし、今回の方針では旧 `kawaguchike-llm` は最終的に廃止対象であるため、ロールバックは一時措置としてのみ扱う。

---

## 11. 結論

`Win11-HP01` への移管で最も重要なのは、**`SBV2` をどの契約で `RenCrow Live` に見せるか** である。

- `Whisper` は `8080/inference` で整理しやすい
- `SBV2` は `server_editor` だけでなく、**RenCrow Live 本線で必要な Bridge 契約**を満たせるかを確認する必要がある

移管完了条件は次の通り。

1. このPCの `RenCrow Live` が `Win11-HP01` の `Whisper` を使える
2. このPCの `RenCrow Live` が `Win11-HP01` の `SBV2` を Bridge 本線で使える
3. ブラウザはどこからでもこのPCの `RenCrow Live` に接続できる
