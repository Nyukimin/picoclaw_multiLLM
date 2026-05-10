# LLM 切り替え仕様

RenCrow_LLM は OpenAI 互換 API として動作する。
クライアント側は、用途に応じて **Base URL** と **model 名** を切り替えて利用する。

## 切り替え単位

LLM は用途ごとに別プロセス・別ポートで起動する。

| 用途 | Base URL | model | 主な用途 |
| --- | --- | --- | --- |
| Chat | `http://127.0.0.1:8081` | `Chat` | 通常会話、音声 UI、軽い応答 |
| Worker | `http://127.0.0.1:8082` | `Worker` | 要約、整理、RAG、実務処理 |
| Coder | `http://127.0.0.1:8082` | `Coder` | 実装方針、patch 方針、テスト手順 |
| Heavy | `http://127.0.0.1:8083` | `Heavy` | 深い分析、失敗原因調査、最終レビュー |
| Wild | `http://127.0.0.1:8084` | `Wild` | 創作、画像プロンプト、雰囲気・構図分析 |

## 基本ルール

- Chat / Worker / Heavy / Wild は別ポートで呼び分ける。
- Worker と Coder は同じポート `8082` を使い、`model` 名で切り替える。
- 各プロセスは許可された `model` 名だけを受け付ける。
- 例えば Chat 用 endpoint に `model: "Worker"` を送ると拒否される。

## リクエスト例

Chat:

```json
{
  "model": "Chat",
  "messages": [
    { "role": "user", "content": "こんにちは" }
  ],
  "max_tokens": 512
}
```

Worker:

```json
{
  "model": "Worker",
  "messages": [
    { "role": "user", "content": "この文章を要約して" }
  ],
  "max_tokens": 1024
}
```

Coder:

```json
{
  "model": "Coder",
  "messages": [
    { "role": "user", "content": "この変更の実装方針を作って" }
  ],
  "max_tokens": 2048
}
```

## 切り替え判断の目安

| 入力内容 | 推奨 |
| --- | --- |
| 雑談、短い返答、音声対話 | Chat |
| 要約、整理、調査結果の整形 | Worker |
| コード修正方針、影響範囲、テスト観点 | Coder |
| 原因が複雑、前提から見直す必要がある | Heavy |
| 創作、物語、画像生成プロンプト | Wild |

## エラー仕様

許可されていない model 名を指定すると `404` が返る。

```json
{
  "error": {
    "message": "Unknown model alias: Worker. Allowed model(s): Chat",
    "type": "invalid_request_error",
    "param": "model",
    "code": "model_not_found"
  }
}
```

## 推奨クライアント設定

クライアント側では、用途ごとに以下の設定を持つ。

```json
{
  "chat": {
    "base_url": "http://127.0.0.1:8081",
    "model": "Chat"
  },
  "worker": {
    "base_url": "http://127.0.0.1:8082",
    "model": "Worker"
  },
  "coder": {
    "base_url": "http://127.0.0.1:8082",
    "model": "Coder"
  },
  "heavy": {
    "base_url": "http://127.0.0.1:8083",
    "model": "Heavy"
  },
  "wild": {
    "base_url": "http://127.0.0.1:8084",
    "model": "Wild"
  }
}
```

## 注意事項

- 通常運用では Chat と Worker を常駐させる。
- Heavy / Wild は必要時に起動する。
- API 自体は現状認証なし。
- LAN から使う場合は `127.0.0.1` を Mac の IP に置き換える。
- streaming は OpenAI 互換の SSE 形式で利用できる。

