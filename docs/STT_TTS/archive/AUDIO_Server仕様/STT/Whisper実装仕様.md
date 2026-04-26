# Whisper 実装仕様

RenCrow で **whisper.cpp の `whisper-server`** を ASR として使うときの、**起動・HTTP・voice-bridge 連携**に限定した実装仕様である。  
全体の音声パイプライン要件は [仕様.md](./仕様.md) を参照する。

---

## 1. 構成上の位置

```
[ブラウザ] --WebSocket--> [voice-bridge] --HTTP POST--> [whisper-server :8080]
```

- **voice-bridge** がクライアントから受け取った音声バイナリを **`WHISPER_URL`** へ `multipart/form-data` で転送する。
- **Whisper 本体**は本リポジトリの `devices/audioio/whisper.cpp` をビルドした **`whisper-server.exe`**（起動は `ops/audioio/start-whisper.ps1`）。

---

## 2. whisper-server の起動（`ops/audioio/start-whisper.ps1`）

### 2.1 パス・ポート

| 項目 | 値 |
|------|-----|
| whisper.cpp ルート | `devices/audioio/whisper.cpp`（スクリプト内 `$RepoDir`、既定は `D:\RenCrow\...`） |
| 実行ファイル | 次のいずれか最初に存在するパス: `build\bin\Release\whisper-server.exe`、`build\bin\whisper-server.exe`、`build\Release\whisper-server.exe` |
| 待受ホスト | `0.0.0.0`（`$BindHost`） |
| 待受ポート | **8080**（`$Port`） |
| モデルファイル | `models\ggml-base.bin`（`$ModelPath`） |
| ログ | `logs/audioio/start-whisper.log`、起動後は `whisper.stdout.log` / `whisper.stderr.log` |

### 2.2 固定コマンドライン引数

スクリプトが常に付与する引数:

| 引数 | 意味 |
|------|------|
| `--host`, `--port` | 上記待受 |
| `-m` | モデルパス |
| `-l ja` | 日本語 |
| `--convert` | **ffmpeg** による入力音声の WAV 変換（サーバー側に ffmpeg が必要） |
| `--split-on-word` | 単語境界での分割 |

### 2.3 任意の追加引数（環境変数）

| 環境変数 | 付与される引数 | 注意 |
|----------|----------------|------|
| `REN_WHISPER_FAST=1` | `-bo 1 -nf` | best-of 削減・温度フォールバック無効。速くなるが認識品質が落ちることがある。 |
| `REN_WHISPER_FLASH_ATTN=1` | `-fa` | **CUDA ビルドかつ対応 GPU** を想定。未対応環境では起動失敗しうる。 |

### 2.4 排他・スキップ条件

- グローバルミューテックス `Global\RenCrow-Start-Whisper` で **二重起動を抑止**。
- **8080 が既に LISTEN している**場合は起動をスキップして正常終了。

---

## 3. HTTP API（whisper.cpp 標準）

voice-bridge は **whisper.cpp の `/inference`** と互換であることを前提とする。

### 3.1 エンドポイント

| 項目 | 値 |
|------|-----|
| メソッド | `POST` |
| パス | `/inference`（ベース URL にパスを含めない場合は `http://host:8080/inference`） |

### 3.2 リクエスト

- **`Content-Type`**: `multipart/form-data`
- **フィールド `file`**: 音声バイナリ（voice-bridge は `form-data` でファイルとして付与）
- **フィールド `response_format`**: voice-bridge 実装では **`json`** を付与

### 3.3 レスポンス

- **`response_format=json`** 時、JSON オブジェクトに **`text`** フィールド（認識結果文字列）があることを voice-bridge が読み取る。

---

## 4. voice-bridge 側の実装（`server.js` / `server-https.js`）

### 4.1 接続先

| 環境変数 | 意味 | 未設定時 |
|----------|------|----------|
| `WHISPER_URL` | Whisper の推論 URL（**フルパス**） | `http://127.0.0.1:8080/inference` |

### 4.2 `transcribeBuffer(buffer, mimeType, opts)`

| 項目 | 実装 |
|------|------|
| **最小サイズ** | `MIN_AUDIO_BYTES === 256` 未満は **Whisper を呼ばず** 空文字を返す（断片・空ファイルによる ffmpeg 失敗の抑制）。 |
| **MIME 推定** | バッファ先頭が **RIFF** のとき `audio/wav`。それ以外は WebSocket の `config` で渡した **`mimeType`**。 |
| **ファイル名** | `audioFilenameForMime` — `wav`→`window.wav`、`webm` 等→`window.webm` 等（ffmpeg / 拡張子ヒント用）。 |
| **multipart** | `file` + `response_format=json` |
| **`fetch`** | `node-fetch`。`opts.signal` が渡された場合 **`AbortSignal`** を付与（暫定キャンセル用）。 |
| **HTTP エラー** | `res.ok` でなければ **warn ログ**し空文字（クライアントへ `error` は送らない）。 |
| **成功時** | `data.text` を正規化（空白圧縮・trim）して返す。 |
| **例外** | **`AbortError`** は **再スロー**。それ以外は warn ログして空文字。 |

### 4.3 暫定・確定と Whisper 呼び出しの関係

| 状況 | 動作 |
|------|------|
| 暫定バイナリ | `busy` でなければ `transcribeBuffer`（`signal` 付き）。結果を `draft` で送信。 |
| `busy` 中に届く暫定 | **破棄**（再入しない）。 |
| 確定バイナリ | 進行中の暫定があれば **`abort()`** → `busy` が空くまで待機 → **`transcribeBuffer`（signal なし）** → `final`。 |

※ WebSocket 上のメッセージ順序・`final_pending` の扱いの詳細は [仕様.md](./仕様.md) §3。

---

## 5. クライアントから送られる音声（Whisper が受け取るデータの出所）

voice-bridge は **中継のみ**である。デモ UI（`public/index.html`）では次のとおり。

| 種別 | 形式の目安 |
|------|------------|
| **暫定** | ScriptProcessor の PCM を **16kHz mono WAV** にし、WebSocket バイナリで送信 → サーバーでは RIFF 検出で `audio/wav`。 |
| **確定** | `MediaRecorder` の **1 Blob**（例: `audio/webm`）を `final_pending` の直後に送信 → `config` の MIME ＋ `--convert` で処理。 |

定数（`CHUNK_MS`、`MIN_PCM_SAMPLES_FOR_DRAFT` 等）の一覧は [仕様.md](./仕様.md) §4.3 を参照。

---

## 6. トラブルシューティング（実装観点）

| 現象 | 確認すること |
|------|----------------|
| `FFmpeg conversion failed` / 500 | 入力が短すぎる・壊れている。`MIN_AUDIO_BYTES` 未満は送っていないか。サーバーに **ffmpeg** があるか。`--convert` 前提か。 |
| 暫定が出ない | WebM 断片のみの場合 ffmpeg が通らないことがある → **WAV 暫定**（PCM パス）を使っているか。 |
| 遅い・詰まる | GPU でも **リクエストは直列に近い**。モデルサイズ、`REN_WHISPER_*`、別 PC なら **RTT**。 |
| 接続できない | `WHISPER_URL`、ファイアウォール、Whisper の `--host`（`0.0.0.0` で外部から到達可）。 |

---

## 7. 他の PC から Whisper を利用する手法

構成の前提（§1）と **`WHISPER_URL`** の扱い、別 PC 時のファイアウォール・CORS の整理は、**[sbv2-env/10_WHISPER_REMOTE_PC.md](./sbv2-env/10_WHISPER_REMOTE_PC.md)** に抜き出してある（`09_BROWSER_AND_CORS.md` と同様の体裁）。

---

## 8. 関連ファイル

| パス | 内容 |
|------|------|
| `webui/voice-bridge/server.js` | HTTP + Whisper プロキシ |
| `webui/voice-bridge/server-https.js` | 同上（HTTPS） |
| `ops/audioio/start-whisper.ps1` | whisper-server 起動 |
| `devices/audioio/whisper.cpp` | whisper.cpp ソース・ビルド成果物 |
