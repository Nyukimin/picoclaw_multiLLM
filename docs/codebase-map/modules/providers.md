---
run_id: run_20260619_000000
generated_at: 2026-06-19
phase: phase2
module_group: llm_provider
---

# pkg/providers — LLM プロバイダー解析

## 概要

`pkg/providers/` が LLM プロバイダー実装を収容。
全プロバイダーは `LLMProvider` インターフェースを実装し、`pkg/agent/loop.go` から `applyRouteLLM()` 経由で切り替えられる。

## LLMProvider インターフェース

```go
// pkg/providers/types.go
type LLMProvider interface {
    Chat(ctx context.Context, messages []Message, tools []Tool, model string, options *Options) (*LLMResponse, error)
}

type Message struct {
    Role       string   // "system" | "user" | "assistant" | "tool"
    Content    string
    Media      []byte
    ToolCalls  []ToolCall
    ToolCallID string
}

type LLMResponse struct {
    Content   string
    ToolCalls []ToolCall
    Usage     *Usage  // nil の可能性あり
}

type Usage struct {
    InputTokens  int
    OutputTokens int
    TotalTokens  int
}
```

## プロバイダー一覧

| プロバイダー | ファイル | LLM | デフォルトモデル |
|------------|---------|-----|--------------|
| Anthropic Claude | `claude_provider.go` | Claude API | claude-sonnet-4-5-20250929 |
| OpenAI | `openai_provider.go` | OpenAI API | gpt-4 |
| DeepSeek | `deepseek_provider.go` | DeepSeek API | deepseek-chat |
| Ollama | `ollama_provider.go` | Ollama (ローカル) | chat-v1 / worker-v1 |
| GitHub Copilot | `copilot_provider.go` | Copilot API | — |
| Codex CLI | `codex_provider.go` | Codex CLI (subprocess) | — |
| HTTP Generic | `http_provider.go` | 任意 HTTP エンドポイント | — |

## Anthropic Claude プロバイダー（主要）

```go
// pkg/providers/claude_provider.go
type ClaudeProvider struct {
    client *anthropic.Client
    model  string
}

// OAuth 対応（GitHub Copilot 経由など）
func NewClaudeProviderWithTokenSource(tokenSource oauth2.TokenSource, model string) *ClaudeProvider
```

- `anthropics/anthropic-sdk-go v1.22.1` 使用
- デフォルトモデル: `claude-sonnet-4-5-20250929`
- `buildClaudeParams()` で system/user/assistant/tool ロールを変換
- `anthropic.ToolResultBlockParam` で tool_result ロールを処理

## Ollama プロバイダー

```go
// pkg/providers/ollama_provider.go
type OllamaProvider struct {
    baseURL string
    model   string
    keepAlive string // "-1" で常駐化
}
```

- `keep_alive: -1` で Chat/Worker モデルを常駐化（設定必須）
- タイムアウト判定: 3 段階チェック
  1. `context.DeadlineExceeded`
  2. `net.Error.Timeout()`
  3. エラー文字列マッチ
- isOllamaEndpoint: `:11434`, `localhost:11434`, `127.0.0.1:11434` を検出

## ルートと LLM の対応（旧アーキテクチャ）

| ルート | プロバイダー | モデル設定キー |
|-------|------------|-------------|
| CHAT | Ollama | `Routing.ChatModel` |
| OPS / RESEARCH / PLAN / ANALYZE | Ollama Worker | `Routing.WorkerModel` |
| CODE1 | DeepSeek（またはOllama） | `Routing.Coder.Provider1` |
| CODE2 | OpenAI | `Routing.Coder.Provider2` |
| CODE3 | Anthropic Claude | `Routing.Coder.Provider3` |

## applyRouteLLM（loop.go）

```go
func (al *AgentLoop) applyRouteLLM(route Route, session *Session) {
    switch route {
    case RouteChat:
        al.provider = createOllamaProvider(al.config.Routing.ChatProvider)
    case RouteCode3:
        al.provider = createAnthropicProvider(al.config.Routing.Coder.Provider3)
    // ...
    }
}
```

ルーティング決定後に `al.provider` を差し替える。
この設計はスレッドセーフでない点に注意（単一 goroutine で実行されている前提）。

## CreateProvider（3段階フォールバック）

```go
func CreateProvider(spec string, config *Config) LLMProvider
```

1. 明示指定（`"anthropic"`, `"openai"`, `"ollama"` 等）
2. モデル名推論（`gpt-` → OpenAI, `claude-` → Anthropic）
3. OpenRouter へのフォールバック

## 画像ペイロード

- インライン画像は最大 5MB（`maxInlineImageBytes = 5 * 1024 * 1024`）
- 超過分は黙って除外（ログなし）

## 設定環境変数

| 環境変数 | 対応プロバイダー |
|---------|--------------|
| `ANTHROPIC_API_KEY` | Claude API |
| `OPENAI_API_KEY` | OpenAI |
| `DEEPSEEK_API_KEY` | DeepSeek |
| `GITHUB_TOKEN` | GitHub Copilot |

平文保存禁止（CLAUDE.md 3.4.3節）。

## 関連

- [llm_provider.md](llm_provider.md) — 旧バージョン解析
- [modules_order.md](modules_order.md)
- [アーキテクチャ総合.md](../アーキテクチャ総合.md)
