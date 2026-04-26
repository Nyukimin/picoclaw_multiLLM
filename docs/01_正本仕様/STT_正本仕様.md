# STT 正本仕様

# STT 正本仕様

## 1. 目的と適用範囲

本仕様は RenCrow における STT（Speech-to-Text）連携の正本仕様を定義する。
対象は次の経路とする。

- 主経路: `Browser -> /stt-ws -> voice-bridge -> Whisper /inference`
- サーバ間連携: `voice-bridge -> Whisper`（HTTP）

本仕様は STT を中心に定義し、TTS（SBV2）詳細は別仕様を参照する。

## 2. 現状整理（2026-04時点）

- `docs/README.md` 上の整理では、音声アダプターは「TTS/Audio Router は部分実装、STT 未実装」。
- Viewer フロントには STT クライアント実装があり、`/stt-ws` へ WebSocket 接続するコードが存在する。
- Whisper 利用方針は `voice-bridge` 経由であり、ブラウザから Whisper 直接呼び出しは主経路ではない。

上記より、STT は「設計/フロント実装は先行、バックエンド統合は未完」の状態である。

## 3. アーキテクチャ

### 3.1 論理構成

```text
[Browser]
  - mic capture
  - send WAV chunks via WebSocket (/stt-ws)
            |
            v
[voice-bridge]
  - VAD
  - Whisper API client
  - draft/final event emit
            |
            v
[Whisper Server]
  - POST /inference
  - return transcript
```

### 3.2 境界責務

- Browser
  - マイク入力取得
  - PCM/WAV 変換
  - `/stt-ws` への送信
  - draft/final 表示
- voice-bridge
  - STT セッション管理
  - VAD（発話区間判定）
  - Whisper 呼び出し
  - `draft`/`final`/`error` イベント返却
- Whisper
  - 音声認識エンジンとしてテキスト化を実行

## 4. 通信契約

### 4.1 Browser -> voice-bridge（WebSocket）

- エンドポイント: `/stt-ws`
- プロトコル:
  - 初期設定メッセージ（JSON）
    - 例: `{ "type": "config", "mimeType": "audio/wav" }`
  - 音声データ（binary）
    - WAV チャンク（16kHz / mono / PCM16 推奨）

### 4.2 voice-bridge -> Browser（WebSocket event）

- `draft`
  - 途中認識テキスト
  - 例: `{ "type": "draft", "text": "..." }`
- `final`
  - 確定テキスト
  - 例: `{ "type": "final", "text": "..." }`
- `error`
  - 認識/接続エラー
  - 例: `{ "type": "error", "error": "..." }`

### 4.3 voice-bridge -> Whisper（HTTP）

- エンドポイント: `POST /inference`
- 接続先: `WHISPER_URL` 環境変数で指定
- 既定例:
  - ローカル: `http://127.0.0.1:8080/inference`
  - リモート: `http://<whisper-host>:8080/inference`

## 5. 設定仕様

### 5.1 環境変数

- `WHISPER_URL`
  - voice-bridge が呼び出す Whisper URL
  - 起動前に設定する

### 5.2 ネットワーク要件

- Whisper サーバは `0.0.0.0:8080` 待受を許可する。
- Whisper サーバ側ファイアウォールで `TCP 8080` 受信を許可する。
- Tailscale/VPN 利用時は到達可能なホスト名またはIPを `WHISPER_URL` に設定する。

## 6. 非推奨構成

- Browser から Whisper への直接 `fetch`
  - CORS/Mixed Content 課題が増えるため非推奨。
- 公開インターネットへの 8080 直接公開
  - TLS 終端や認証がない構成は不可。

## 7. 実装方針（段階）

### Phase 1: 接続確立

- `WHISPER_URL` をリモートWhisperへ向ける。
- `/stt-ws` 経由で認識結果（`draft`/`final`）が返ることを確認する。

### Phase 2: ドメイン統合

- `Transcriber` インターフェースへ STT 入力を統合する。
- STT 確定テキストを既存 Orchestrator 入力へ接続する。

### Phase 3: 運用強化

- タイムアウト/再試行/サーキットブレーカの設計。
- エラーハンドリングの標準化（ユーザー通知と内部ログ分離）。

## 8. 受け入れ基準

- 機能
  - Browser からマイク入力し、`final` テキストが取得できる。
  - STT 確定テキストが会話入力として処理される。
- 接続
  - `WHISPER_URL` を切り替えてローカル/リモート双方で動作する。
- 安全
  - ブラウザは Whisper 直アクセスしない。
  - サーバ間通信前提で CORS 依存を持たない。
- 回帰
  - TTS（SBV2）既存経路へ影響を与えない。

## 9. 未確定事項

- `/stt-ws` の最終配置（Go本体実装か、voice-bridge別プロセスか）。
- Whisper へのリクエスト詳細（multipart仕様、model指定、language指定）。
- STT 用設定を `config.yaml` に正式追加するか、環境変数運用を継続するか。

## 10. voice-bridge（STT Gateway）実装詳細

### 10.1 概要

voice-bridge は STT Gateway の現行実装であり、Node.js プロセスとして動作する。

- **役割**: Browser からの WebSocket 音声ストリームを受け取り、VAD で発話区間を判定し、Whisper へ HTTP POST で転送する
- **実行場所**: Win11-HP01（`192.168.1.36`）
- **ポート**: `:8090`
- **ソース**: `server.js`（+ `stt-gateway-contract.js`、`server-https.js`）

### 10.2 物理構成

```text
Browser
  ↓ wss://fujitsu-ubunts:18790/stt-ws
RenCrow Chat Server（Go, fujitsu-ubunts）← STT_GATEWAY_URL が設定されている場合は透過プロキシ
  ↓ ws://192.168.1.36:8090/stt-ws
voice-bridge（Node.js, Win11-HP01 :8090）← STT Gateway 本体
  ↓ HTTP POST multipart/form-data
Whisper（whisper.cpp, Win11-HP01 :8080/inference）
```

### 10.3 接続設定

| 項目 | 値 |
|---|---|
| Chat Server 側設定 | `STT_GATEWAY_URL=ws://192.168.1.36:8090/stt-ws` |
| voice-bridge 起動コマンド | `npm start`（voice-bridge ディレクトリ内） |
| voice-bridge WebSocket パス | `/stt-ws`、`/ws`（両方受け付ける） |
| Whisper 接続先 | `STT_PROVIDER_URL=http://192.168.1.36:8080/inference`（voice-bridge 側の環境変数） |

### 10.4 Go Chat Server のプロキシ動作

`STT_GATEWAY_URL` 環境変数の設定状態により動作が変わる：

| `STT_GATEWAY_URL` | 動作 |
|---|---|
| **設定あり** | `/stt-ws` を voice-bridge へ透過 WebSocket プロキシ |
| **未設定**（フォールバック） | Go が直接 Whisper を呼ぶ（VAD なし・簡易実装） |

**本番運用では `STT_GATEWAY_URL` を設定し voice-bridge 経由を使用すること。**

### 10.5 voice-bridge の主要機能

- **Silero VAD**（`@ricky0123/vad-node`）: ML モデルによる発話開始/終了の高精度判定
- **RNNoise**（`@jitsi/rnnoise-wasm`）: WASM ベースのノイズ除去
- **`session_info` 送出**: 接続時に `{"type":"session_info","session_id":"sess-..."}` を送信
- **draft / final**: VAD 発話終了時に final 確定、発話中は draft を定期送信（`STT_DRAFT_ENABLED=true` 時）
- **busy policy**: 同時推論の再入制御（`drop` / `queue_latest`）

### 10.6 voice-bridge 主要環境変数

| 環境変数 | デフォルト | 説明 |
|---|---|---|
| `STT_PROVIDER_URL` / `WHISPER_URL` | `http://127.0.0.1:8080/inference` | Whisper エンドポイント |
| `STT_PORT` / `PORT` | `8090` | 待受ポート |
| `STT_DRAFT_ENABLED` | `false` | draft イベント有効化 |
| `STT_SILENCE_END_MS` | `850` | 無音判定ウィンドウ（ms） |
| `STT_MIN_AUDIO_BYTES` | `32044` | 推論対象の最小音声サイズ |

### 10.7 依存パッケージ

voice-bridge の起動には以下が必要：

| パッケージ | 用途 |
|---|---|
| `express` | HTTP サーバー |
| `ws` | WebSocket サーバー |
| `node-fetch` (v2) | Whisper HTTP クライアント |
| `form-data` | multipart 組み立て |
| `@ricky0123/vad-node` | Silero VAD |
| `@jitsi/rnnoise-wasm` | RNNoise ノイズ除去 |

### 10.8 注意事項

- voice-bridge は Chat Server（Go）とは別プロセスで起動する
- 起動順序: `Whisper → voice-bridge → Chat Server（Go）`
- `STT_GATEWAY_URL` 未設定時は Go のフォールバック実装が動作するが、VAD なしのため認識品質が低下する
- `session_info` による `session_id` はフォールバック実装では送出されない

## 12. 参照

- `docs/10_WHISPER_REMOTE_PC.md`
- `docs/11_WIN11_HP01_SERVER_MIGRATION.md`
- `docs/12_SBV2_TTS_現状仕様.md`
- `docs/04_実装仕様_機能拡張/実装仕様_チャネル拡張_v1.md`
- `docs/02_OpenClaw移植詳細仕様/詳細実装仕様_07_アプリ・ノード統合.md`
- `docs/README.md`
