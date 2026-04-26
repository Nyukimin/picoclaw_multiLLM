# SBV2 TTS 現状仕様メモ

## 概要

本メモは、2026-04-06 時点の `RenCrow Live` と `Win11-HP01` 上 `SBV2` / `Whisper` の接続前提を、実装観点でまとめた現状仕様である。

- `Whisper` は `voice-bridge` から HTTP POST で利用する
- `SBV2` は `server_editor` 互換 API を TTS 専用で利用する
- `RenCrow Live` 本線の TTS Client Bridge 契約と、`SBV2` 側の提供 API は一致していない

## 1. 現在の構成

```text
[Browser]
   |
   v
[This PC: RenCrow Live]
   |                    |
   | voice-bridge       | TTS adapter / bridge
   v                    v
[Whisper on Win11]    [SBV2 on Win11]
  :8080/inference      :8000/api/*
```

前提:

- ブラウザはこの PC 上の `RenCrow Live` に接続する
- ブラウザが `Whisper` や `SBV2` を直接叩く前提ではない
- `Whisper` と `SBV2` は `Win11-HP01` 上で待ち受ける

## 2. Whisper 側の現状仕様

`Whisper` は `voice-bridge` からサーバ間通信で利用する。

- 接続先は `WHISPER_URL`
- 想定 URL は `http://<win11-host>:8080/inference`
- `Win11-HP01` 側は `0.0.0.0:8080` で待受
- Windows Defender Firewall で `TCP 8080` の受信許可が必要
- ブラウザからの直接アクセスは主経路ではない

現状結論:

- `Whisper` は設定差し替えで接続可能
- `CORS` は通常問題にならない

## 3. SBV2 側の現状仕様

### 3.1 利用方針

利用するのは `Editor UI` ではなく、`server_editor` 互換の TTS HTTP API である。

### 3.2 確認済みエンドポイント

- `GET /api/models_info`
- `POST /api/g2p`
- `POST /api/synthesis`

未確認ではなく、現時点で「ない」と整理しているもの:

- `GET /health/ready`
- `POST /synthesize`
- `WS /sessions`
- ルート直下の `POST /synthesis`

### 3.3 `POST /api/synthesis` 契約

- メソッド: `POST`
- パス: `/api/synthesis`
- `Content-Type: application/json`
- 成功時レスポンス: `audio/wav` バイナリ直返し
- JSON の `audio_path` は返さない

最小に近いリクエスト例:

```json
{
  "model": "jvnv-F1-jp",
  "modelFile": "model_assets\\jvnv-F1-jp\\jvnv-F1-jp_e160_s14000.safetensors",
  "text": "こんにちは",
  "moraToneList": [
    { "mora": "コ", "tone": 0 },
    { "mora": "ン", "tone": 1 },
    { "mora": "ニ", "tone": 1 },
    { "mora": "チ", "tone": 1 },
    { "mora": "ワ", "tone": 0 }
  ],
  "speaker": "jvnv-F1-jp"
}
```

補足:

- `moraToneList` は通常 `POST /api/g2p` の返却を使う
- `speaker` は `voice_id` ではなく、`models_info.speakers[]` の値を渡す
- `modelFile` は `models_info.files[]` から選ぶ

### 3.4 `GET /api/models_info` 契約

レスポンスはモデル配列で、各要素に少なくとも次が含まれる。

- `name`
- `files[]`
- `styles[]`
- `speakers[]`

例:

```json
[
  {
    "name": "jvnv-F1-jp",
    "files": [
      "model_assets\\jvnv-F1-jp\\jvnv-F1-jp_e160_s14000.safetensors"
    ],
    "styles": [
      "Neutral",
      "Angry",
      "Disgust",
      "Fear",
      "Happy",
      "Sad",
      "Surprise"
    ],
    "speakers": [
      "jvnv-F1-jp"
    ]
  }
]
```

話者指定の確定手順:

1. `GET /api/models_info` で使う `model` を決める
2. その要素の `speakers[]` から 1 つ選ぶ
3. `POST /api/synthesis` の `speaker` に入れる

## 4. RenCrow Live 側の現実装

### 4.1 設定受け口

現状の `RenCrow Live` は、TTS 接続先として次を持つ。

- `tts.http_base_url`
- `tts.ws_url`
- `tts.sbv2.base_url`
- `tts.voice_id`

現設定は旧サーバ向けになっている。

### 4.2 現在の本線契約

`RenCrow Live` の TTS Client Bridge は次の契約を前提にしている。

- `GET /health/ready`
- `POST /synthesize`
- `WS /sessions`

この経路では、`audio_path` または `audio_url` を扱う。

## 5. 差分と結論

### 5.1 差分

`RenCrow Live` 本線と `SBV2` 提供 API の差分は次のとおり。

| 項目 | RenCrow Live 本線 | SBV2 server_editor 互換 |
|------|-------------------|-------------------------|
| Ready チェック | `GET /health/ready` | なし |
| Fallback HTTP | `POST /synthesize` | なし |
| Streaming | `WS /sessions` | なし |
| TTS 実行 | `audio_path` / `audio_url` を返す | `audio/wav` を直返し |
| 話者指定 | `voice_id` | `speaker` |
| 前処理 | 内部前提 | `POST /api/g2p` で `moraToneList` を生成 |

### 5.2 結論

- `Whisper` は設定変更で接続できる
- `SBV2` は今のまま `tts.http_base_url` / `tts.ws_url` を差し替えても本線には接続できない
- `SBV2` を使うには、`server_editor` 互換 API を叩く `RenCrow` 側 Adapter が必要

## 6. SBV2 Adapter の実装方針

最小フロー:

1. 応答文を短いチャンクへ分割する
2. 読みやすさのため句読点を補う
3. 各チャンクごとに `POST /api/g2p`
4. `moraToneList` を使って `POST /api/synthesis`
5. 返ってきた `audio/wav` を保存し、順番どおり再生する

文節送信の前提:

- 全文一括送信はしない
- 文節寄りの短い単位で送る
- 句読点を付けた状態で送る
- 再生順はチャンク順で維持する

## 7. 現時点の実装対象

優先度順:

1. `Whisper` の接続先を `Win11-HP01` に向ける
2. `SBV2` 用の `models_info -> g2p -> synthesis` クライアントを実装する
3. 文節分割と句読点補正を実装する
4. `audio/wav` 保存と再生キューへの受け渡しを実装する

## 8. 未確定事項

実装前に最終確認するとよい項目:

- 既定で使う `model`
- 既定で使う `speaker`
- 文節分割の細かさ
- 生成 WAV の保存先とクリーンアップ方針

## 関連ドキュメント

- `docs/STT_TTS/STT/10_WHISPER_REMOTE_PC.md`
- `docs/STT_TTS/COMMON/11_WIN11_HP01_SERVER_MIGRATION.md`
- `docs/STT_TTS/COMMON/09_BROWSER_AND_CORS.md`
- `docs/STT_TTS/TTS/SBV2_SERVER_TEAM_CONTACT_QUESTIONS.md`
- `config/SBV2_SERVER_IMPLEMENTATION_REQUIREMENTS.md`
- `config/SBV2_SERVER_REQUEST_TEMPLATE.md`
