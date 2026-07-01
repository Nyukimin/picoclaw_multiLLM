# RenCrow Runtime Config

## 目的

picoclaw が利用する LLM / TTS / STT のサーバ所在は、ユーザー環境側 Config の `rencrow` ブロックを正本とする。

Windows では `C:\Users\nyuki\.picoclaw\config.yaml` を実運用の正本 Config とする。macOS / Linux では `$HOME/.picoclaw/config.yaml` を同じ位置づけとする。

Repository 内の `run/local-windows-runtime.yaml` は、実運用 Config のスナップショットであり、自動 fallback 先ではない。起動時に正本 Config が見つからない、または壊れている場合は、Repo 内 Config へ黙って切り替えず、起動失敗として扱う。

Viewer や picoclaw server は、LLM / TTS / STT の backend を直接呼ばない。Runtime request は必ず次の経路を通す。

- LLM: `picoclaw -> RenCrow_LLM -> LLM backend`
- TTS: `picoclaw -> RenCrow_TTS -> TTS backend`
- STT: `picoclaw -> RenCrow_STT -> STT backend`
- STT が LLM の音声入力機能を使う場合も、Viewer / picoclaw から見た入口は RenCrow_STT とする。その場合の内部経路は `picoclaw -> RenCrow_STT -> RenCrow_LLM -> LLM audio backend` と表現する。

`local_llm`、`llm_ops`、`tts`、`stt` の既存ブロックは互換用に残す。新規のサーバ所在定義は `rencrow.llm` / `rencrow.tts` / `rencrow.stt` に寄せる。

## Config の責務

### 実運用 Config

実運用 Config は、現在この端末から到達すべき RenCrow module server の所在を持つ。

- Windows: `C:\Users\nyuki\.picoclaw\config.yaml`
- macOS / Linux: `$HOME/.picoclaw/config.yaml`

ここには、LAN IP、Tailscale IP、port、RenCrow_LLM / RenCrow_TTS / RenCrow_STT の配置先、health path、timeout、voice id など、実行環境ごとに変わる値を書く。
ただし `rencrow.stt.engine=llm_audio` の場合、独立した RenCrow_STT server は起動しない構成として扱う。この場合の `rencrow.stt` は STT 機能の入口設定であり、外部 STT server の所在ではない。

秘密値そのものは書かない。token は `token_env` で環境変数名だけを書く。

### Repo 内スナップショット

Repo 内の `run/local-windows-runtime.yaml` は、実運用 Config の参照用スナップショットとする。

用途は以下に限定する。

- 実運用 Config の現在形を開発時に比較する
- 変更内容をレビューしやすくする
- 復元や再現の手がかりにする
- 明示的に `PICOCLAW_CONFIG=run/local-windows-runtime.yaml` を指定した検証で使う

Repo 内スナップショットは、自動 fallback に使わない。古いスナップショットで起動できてしまうと、実際とは異なる LLM / TTS / STT を見に行くためである。

### 起動時の Config 選択

起動時の Config 選択は次の順序とする。

1. `PICOCLAW_CONFIG` が指定されている場合、その path を使う。
2. `PICOCLAW_CONFIG` が未指定の場合、ユーザー環境側の `.picoclaw/config.yaml` を使う。
3. どちらも使えない場合は起動失敗とし、Repo 内 Config へ暗黙 fallback しない。

Repo 内スナップショットを使う場合は、必ず `PICOCLAW_CONFIG` で明示する。

## 共通構造

各 module server は共通して以下を持つ。

```yaml
enabled: true
base_url: "http://127.0.0.1:7870"
public_base_url: ""
token_env: "RENCROW_MODULE_TOKEN"
timeout_ms: 120000
tls_skip_verify: false
health:
  live_path: "/health/live"
  ready_path: "/health/ready"
  poll_interval_ms: 500
```

- `base_url`: picoclaw が module server へアクセスする URL。
- `public_base_url`: Viewer や別端末から参照する公開 URL。未指定なら `base_url` を使う。
- `token_env`: Bearer token などを読む環境変数名。秘密値そのものは YAML に書かない。
- `timeout_ms`: module server への通常 request timeout。
- `health.live_path`: process が生きていることを確認する path。
- `health.ready_path`: 推論や変換を受けられる状態を確認する path。
- `health.poll_interval_ms`: ready 待ちの poll 間隔。Chat 切替では 500ms を標準とする。

## LLM

```yaml
rencrow:
  llm:
    enabled: true
    base_url: "http://192.168.1.205:8079"
    token_env: "RENCROW_LLM_TOKEN"
    timeout_ms: 120000
    health:
      live_path: "/health/live"
      ready_path: "/health/ready"
      poll_interval_ms: 500
    endpoints:
      chat_path: "/v1/chat/completions"
      responses_path: "/v1/responses"
      status_path: "/v1/status"
      start_path: "/v1/control/start"
      stop_path: "/v1/control/stop"
      restart_path: "/v1/control/restart"
    default_recipient: "mio"
    recipients:
      mio:
        role: "chat"
        model: "Chat"
        selection: "Chat"
      shiro:
        role: "chatworker"
        model: "ChatWorker"
        selection: "ChatWorker"
      kuro:
        role: "heavy"
        model: "Heavy"
        selection: "Heavy"
      midori:
        role: "wild"
        model: "Wild"
        selection: "Wild"
```

`recipients` は Chat 画面の宛先と RenCrow_LLM 上の selection / 推論 model を対応づける。Viewer は `to=mio|shiro|kuro|midori` だけを送る。route / model / backend の解決は Viewer では行わない。

`selection` は起動/切替管理用、`model` は推論 request 用である。`shiro.selection=ChatWorker` は RenCrow_LLM 側で `Worker` role 起動に正規化されるが、`shiro.model=ChatWorker` はそのまま推論 request に使う。

## TTS

```yaml
rencrow:
  tts:
    enabled: true
    base_url: "http://192.168.1.205:7870"
    public_base_url: "http://192.168.1.205:7870"
    audio_base_url: "http://192.168.1.205:7870"
    timeout_ms: 120000
    health:
      live_path: "/health/live"
      ready_path: "/health/ready"
      poll_interval_ms: 500
    endpoints:
      synthesize_path: "/api/tts"
      voices_path: "/api/voices"
      audio_path_prefix: "/audio/"
    default_voice: "mio"
    voices:
      mio:
        voice_id: "mio"
        voice_name: "Mio"
      shiro:
        voice_id: "shiro"
        voice_name: "Shiro"
```

TTS backend の checkpoint、device、precision などは RenCrow_TTS 側の責務とする。picoclaw は speaker に対応する voice と text を module server に渡す。

## STT

```yaml
rencrow:
  stt:
    enabled: true
    base_url: ""
    stream_url: ""
    engine: "llm_audio"
    language: "ja"
    model: "default"
    busy_policy: "queue_latest"
    vad: true
    timeout_ms: 60000
    llm_audio:
      llm_ref: "rencrow.llm"
      model: "default"
      endpoint_path: "/v1/audio/transcriptions"
      prompt: "Transcribe the audio into Japanese text."
      response_format: "text"
    health:
      live_path: "/health/live"
      ready_path: "/health/ready"
      poll_interval_ms: 500
    endpoints:
      transcribe_path: "/api/stt"
      stream_path: "/stt/stream"
```

`stream_url` は WebSocket の外部公開 URL が `base_url` から機械的に導出できない場合に使う。空の場合、Viewer は picoclaw の同一 origin `/stt` を使う。

`engine` は RenCrow_STT の内部実装方式を表す。

- `external_http`: RenCrow_STT が専用 STT backend へ音声を渡す。
- `llm_audio`: RenCrow_STT が RenCrow_LLM の音声入力 endpoint へ音声を渡し、文字起こし結果を STT final として返す。

`llm_audio.llm_ref` は参照先の LLM module を示す論理名であり、通常は `rencrow.llm` とする。物理 port や backend model の実配置は RenCrow_LLM 側の責務で、Viewer や picoclaw は直接扱わない。

`engine=llm_audio` では、`base_url` / `stream_url` は空でよい。STT 専用プロセスを health 監視せず、ready 判定は RenCrow_LLM 側の状態で行う。

## 運用ルール

- 実運用の正本 Config はユーザー環境側の `.picoclaw/config.yaml` とする。
- Repo 内の `run/local-windows-runtime.yaml` はスナップショットであり、暗黙 fallback 先にしない。
- `.picoclaw/config.yaml` が読めない場合は、誤ったサーバへ接続しないため起動失敗とする。
- 秘密値は YAML に書かず、`token_env` で環境変数名だけを書く。
- module server の配置先は Windows / macOS / Linux のいずれでもよい。Config には OS 固有の path ではなく URL を書く。
- `enabled=true` で `base_url` が空の場合は設定エラーとする。ただし `rencrow.stt.engine=llm_audio` は独立 STT server を持たないため例外とする。
- Viewer は module server の URL を直接使わない。Viewer は picoclaw の endpoint だけを呼ぶ。
- backend の物理 port や model path は RenCrow_LLM / RenCrow_TTS / RenCrow_STT 側に閉じ込める。
