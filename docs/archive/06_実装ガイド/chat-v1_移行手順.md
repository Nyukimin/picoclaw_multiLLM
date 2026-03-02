# chat-v1 モデル移行手順

`kuro-v1:latest` / `kuro-v2:latest` を `chat-v1:latest` に統一するための手順。

## 前提
- Ollama サーバー（例: kawaguchike-llm）に `kuro-v1:latest` が存在する。

## 手順1: chat-v1 を既存 kuro-v1 からコピー（推奨・最も速い）

Ollama API でコピー（ollama CLI が無い環境でも可能）:

```bash
curl -X POST http://<OLLAMA_HOST>:11434/api/copy \
  -H "Content-Type: application/json" \
  -d '{"source":"kuro-v1:latest","destination":"chat-v1:latest"}'
```

または Ollama サーバー上で CLI を使う場合:

```bash
ollama create chat-v1 -f - << 'EOF'
FROM kuro-v1:latest
PARAMETER num_ctx 8192
EOF
```

## 手順2: 新規作成（kuro-v1 が無い場合）

リポジトリの `Modelfile.chat` を Ollama サーバーへコピーし:

```bash
ollama create chat-v1 -f Modelfile.chat
```

## 手順3: 動作確認

```bash
ollama run chat-v1 "みおです。テストです。"
```

## 手順4: PicoClaw Gateway 再起動

```bash
systemctl --user restart picoclaw-gateway.service
curl -sS http://127.0.0.1:18790/ready
```

## オプション: 旧モデル削除

`chat-v1` が正常動作したら、不要になった kuro-v1 / kuro-v2 を削除:

```bash
ollama rm kuro-v1
ollama rm kuro-v2
```
