# 実装仕様: Coder 4体化 + Agent Persona v1.0

**作成日**: 2026-03-26
**ステータス**: 設計完了・実装開始前
**対象ブランチ**: `feature/rencrow`
**関連仕様**:
- `docs/01_正本仕様/実装仕様.md` §4.2 Coder Agent
- `docs/04_実装仕様_機能拡張/実装仕様_分散実行_v4.md` - 分散実行基盤
- `docs/04_実装仕様_機能拡張/実装仕様_エージェントペルソナ_v1.md` - Agent Persona 設計思想

---

## 1. 概要

### 1.1 目的

RenCrow の Coder を 3体（coder1/2/3）から 4体（coder1/2/3/4）に拡張し、同時に Agent Persona v1.0（軽量ペルソナ + 短期記憶）を統合実装する。

**主要目標**:
1. **Coder 4体化**: Gemini を追加し、4つの LLM を使い分け可能に
2. **スロット固定**: coder1〜4 はコードにハードコード（安定性）
3. **名前可変**: aka/ao/gin/kin 等の名前・性格は Config で自由に変更可能（柔軟性）
4. **Agent Persona 統合**: 全 Coder に軽量ペルソナ + 短期記憶を付与
5. **API キー集約**: 全 API キーを Worker の環境変数に集約（運用性向上）

### 1.2 設計原則

| 原則 | 説明 |
|------|------|
| **スロット固定** | `coder1`, `coder2`, `coder3`, `coder4` はコードにハードコード。ルーティング `CODE1`→`coder1` は不変 |
| **名前可変** | `name: "aka"` 等は Config で自由に変更可能。運用環境ごとに異なる名前・LLM を割り当て可能 |
| **集中 Config** | Worker の config.yaml に全設定を記述。分散実行時も Worker が一元管理 |
| **環境変数分離** | 全 API キーは Worker の環境変数に集約。Remote 実行時は SSH 経由で送信 |
| **後方互換** | 既存の coder1/2/3 の動作を変えない。Persona なし・LightMemory なしでも動作 |

### 1.3 非目的（スコープ外）

- ❌ Mio の ConversationEngine（v5.1）の変更
- ❌ Persona の LLM 自己編集（v1.0 では実装しない）
- ❌ Coder の動的追加・削除（スロット数は 4 固定）
- ❌ ルーティングマッピングの Config 化（CODE1→coder1 は固定）

---

## 2. Config 構造

### 2.1 config.yaml 構造

```yaml
# ========================================
# Coder スロット定義（固定: coder1〜4）
# ========================================

coder1:
  name: "aka"                          # 任意の名前（変更可能）
  display_name: "赤"                   # 表示名（変更可能）
  provider: "deepseek"                 # LLM プロバイダー
  model: "deepseek-coder"
  api_key: "${DEEPSEEK_API_KEY}"       # Worker の環境変数
  base_url: "https://api.deepseek.com" # オプション
  personality: |
    あなたは Aka（DeepSeek 担当の Coder）。設計思考が得意で大局的な視点を持つ。
    落ち着いた口調で深い洞察を示す。たまにユーモアを交える。
    提案では全体の設計意図を丁寧に説明し、なぜその設計にしたかの理由を添える。
  tone: "analytical"                   # 口調（TTS 連携用）
  light_memory:
    enabled: true
    max_turns: 3                       # 保持ターン数
  enabled: true

coder2:
  name: "ao"
  display_name: "青"
  provider: "openai"
  model: "gpt-4-turbo"
  api_key: "${OPENAI_API_KEY}"
  personality: |
    あなたは Ao（OpenAI 担当の Coder）。実装力が高く効率を重視するタイプ。
    簡潔に要点を伝える。コードの話になると饒舌になる。
    無駄のない提案を心がけ、パフォーマンスとメンテナンス性を常に意識する。
  tone: "concise"
  light_memory:
    enabled: true
    max_turns: 3
  enabled: true

coder3:
  name: "gin"
  display_name: "銀"
  provider: "claude"
  model: "claude-sonnet-4"
  api_key: "${ANTHROPIC_API_KEY}"      # Worker の環境変数（SSH 経由で送信）
  personality: |
    あなたは Gin（Claude 担当の Coder）。推論が深く慎重に考える。
    複雑な問題を整理し、多角的に検討するのが得意。
    丁寧な説明を心がけ、トレードオフを明示する。
  tone: "thoughtful"
  light_memory:
    enabled: true
    max_turns: 3
  enabled: true

coder4:                                 # 新規スロット
  name: "kin"
  display_name: "金"
  provider: "gemini"
  model: "gemini-2.0-flash-exp"
  api_key: "${GEMINI_API_KEY}"
  personality: |
    あなたは Kin（Gemini 担当の Coder）。高速で反応が良い。
    新しいアプローチを試すのが好きで、実験的な提案も厭わない。
    簡潔かつ明快に伝える。
  tone: "energetic"
  light_memory:
    enabled: true
    max_turns: 3
  enabled: true

# ========================================
# 分散実行設定（Transport 設定）
# ========================================
distributed:
  enabled: true
  transports:
    coder1:                            # スロット名で参照（固定）
      type: "local"
    coder2:
      type: "local"
    coder3:
      type: "ssh"
      remote_host: "bmax2nd-1"
      remote_user: "nyukimin"
      ssh_key_path: "~/.ssh/id_ed25519"
      strict_host_key: true
      remote_agent_path: "C:/picoclaw/picoclaw-agent.exe"
    coder4:
      type: "local"
```

### 2.2 環境変数（Worker のみ）

```bash
# ~/.picoclaw/.env
export DEEPSEEK_API_KEY="sk-..."
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."  # coder3 (SSH 経由で送信)
export GEMINI_API_KEY="sk-..."
```

**セキュリティ**:
- `.env` のパーミッション: `chmod 600 ~/.picoclaw/.env`
- Remote マシンには API キー不要（Worker から送信）

---

## 3. Go 構造体定義

### 3.1 Config 構造体

**ファイル**: `internal/adapter/config/config.go`

```go
type Config struct {
    // ... 既存フィールド ...

    // Coder スロット（固定: 4つ）
    Coder1 CoderConfig `yaml:"coder1"`
    Coder2 CoderConfig `yaml:"coder2"`
    Coder3 CoderConfig `yaml:"coder3"`
    Coder4 CoderConfig `yaml:"coder4"`  // 新規追加

    Distributed DistributedConfig `yaml:"distributed"`
}

// CoderConfig は Coder 個別設定
type CoderConfig struct {
    Name        string            `yaml:"name"`         // 任意の名前（aka, ao 等）
    DisplayName string            `yaml:"display_name"` // 表示名（赤, 青 等）
    Provider    string            `yaml:"provider"`     // deepseek/openai/claude/gemini
    Model       string            `yaml:"model"`
    APIKey      string            `yaml:"api_key"`      // 環境変数参照（${...}）
    BaseURL     string            `yaml:"base_url"`     // オプション
    Personality string            `yaml:"personality"`  // Agent Persona 記述
    Tone        string            `yaml:"tone"`         // 口調（TTS 用）
    LightMemory LightMemoryConfig `yaml:"light_memory"`
    Enabled     bool              `yaml:"enabled"`
}

// LightMemoryConfig は短期記憶設定
type LightMemoryConfig struct {
    Enabled  bool `yaml:"enabled"`
    MaxTurns int  `yaml:"max_turns"`  // 保持ターン数（推奨: 3〜5）
}
```

**setDefaults() 更新**:

```go
func (c *Config) setDefaults() {
    // ... 既存のデフォルト設定 ...

    // Coder4 のデフォルト設定
    if c.Coder4.Provider == "" {
        c.Coder4.Provider = "gemini"
    }
    if c.Coder4.Model == "" {
        c.Coder4.Model = "gemini-2.0-flash-exp"
    }
    if c.Coder4.Name == "" {
        c.Coder4.Name = "kin"
    }
    if c.Coder4.DisplayName == "" {
        c.Coder4.DisplayName = "金"
    }
    if c.Coder4.LightMemory.MaxTurns == 0 {
        c.Coder4.LightMemory.MaxTurns = 3
    }
}
```

---

### 3.2 ルーティング定義

**ファイル**: `internal/domain/routing/route.go`

```go
const (
    RouteCHAT     Route = "CHAT"
    RoutePLAN     Route = "PLAN"
    RouteANALYZE  Route = "ANALYZE"
    RouteOPS      Route = "OPS"
    RouteRESEARCH Route = "RESEARCH"
    RouteCODE     Route = "CODE"
    RouteCODE1    Route = "CODE1"  // → coder1 (固定)
    RouteCODE2    Route = "CODE2"  // → coder2 (固定)
    RouteCODE3    Route = "CODE3"  // → coder3 (固定)
    RouteCODE4    Route = "CODE4"  // → coder4 (固定・新規)
)

// IsCoderRoute は Coder ルートかを判定
func (r Route) IsCoderRoute() bool {
    return r == RouteCODE || r == RouteCODE1 || r == RouteCODE2 ||
           r == RouteCODE3 || r == RouteCODE4
}

// RouteToCoderSlot はルートからスロット名を返す
func RouteToCoderSlot(route Route) string {
    switch route {
    case RouteCODE1:
        return "coder1"
    case RouteCODE2:
        return "coder2"
    case RouteCODE3:
        return "coder3"
    case RouteCODE4:
        return "coder4"
    default:
        return ""
    }
}
```

---

### 3.3 Agent Persona 基盤

#### 3.3.1 AgentPersona 型

**ファイル**: `internal/domain/agent/persona.go` (新規)

```go
package agent

// AgentPersona は Shiro/Coder 向けの軽量ペルソナ定義。
// conversation.PersonaState（Mio 専用）とは独立した型。
type AgentPersona struct {
    Name        string // 表示名（例: "赤", "Aka"）
    Personality string // ペルソナ記述（system prompt 先頭に前置される）
    Tone        string // 口調ヒント（例: "calm", "analytical"）将来の TTS 連携用
}

// BuildSystemPrompt はペルソナブロックとタスクプロンプトを合成する。
// Personality が空文字の場合は taskPrompt をそのまま返す（後方互換）。
// 合成順序: Personality + "\n\n" + taskPrompt
func (p AgentPersona) BuildSystemPrompt(taskPrompt string) string {
    if p.Personality == "" {
        return taskPrompt
    }
    return p.Personality + "\n\n" + taskPrompt
}
```

**テスト**: `internal/domain/agent/persona_test.go` (新規)

```go
func TestAgentPersona_BuildSystemPrompt(t *testing.T) {
    tests := []struct {
        name       string
        persona    AgentPersona
        taskPrompt string
        want       string
    }{
        {
            name: "personality あり",
            persona: AgentPersona{
                Name:        "Aka",
                Personality: "あなたは Aka。設計思考が得意。",
                Tone:        "analytical",
            },
            taskPrompt: "次のタスクを実行してください。",
            want:       "あなたは Aka。設計思考が得意。\n\n次のタスクを実行してください。",
        },
        {
            name: "personality なし（後方互換）",
            persona: AgentPersona{
                Name: "Coder",
            },
            taskPrompt: "次のタスクを実行してください。",
            want:       "次のタスクを実行してください。",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := tt.persona.BuildSystemPrompt(tt.taskPrompt)
            if got != tt.want {
                t.Errorf("BuildSystemPrompt() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

---

#### 3.3.2 LightMemory 型

**ファイル**: `internal/domain/agent/light_memory.go` (新規)

```go
package agent

import (
    "sync"
    "time"

    "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

// LightMemory はセッション単位のインメモリ短期会話履歴。
// プロセス再起動でリセット（意図的）。goroutine-safe。
type LightMemory struct {
    mu       sync.Mutex
    sessions map[string]*sessionBuffer // key: sessionID（LINE UserID 等）
    maxTurns int                       // セッションあたりの最大保持ターン数
}

type sessionBuffer struct {
    turns []turn
}

type turn struct {
    userMessage string
    response    string
    timestamp   time.Time
}

// NewLightMemory は新しい LightMemory を作成する。
// maxTurns はセッションあたりの保持ターン数上限（推奨: 3〜5）。
func NewLightMemory(maxTurns int) *LightMemory {
    return &LightMemory{
        sessions: make(map[string]*sessionBuffer),
        maxTurns: maxTurns,
    }
}

// Record は会話ターン（userMessage + response ペア）を記録する。
// maxTurns を超えた古いターンは FIFO で破棄される。
func (m *LightMemory) Record(sessionID, userMessage, response string) {
    m.mu.Lock()
    defer m.mu.Unlock()

    buf, exists := m.sessions[sessionID]
    if !exists {
        buf = &sessionBuffer{turns: make([]turn, 0, m.maxTurns)}
        m.sessions[sessionID] = buf
    }

    buf.turns = append(buf.turns, turn{
        userMessage: userMessage,
        response:    response,
        timestamp:   time.Now(),
    })

    // FIFO で古いターンを削除
    if len(buf.turns) > m.maxTurns {
        buf.turns = buf.turns[len(buf.turns)-m.maxTurns:]
    }
}

// RecentMessages は指定セッションの直近ターンを llm.Message スライスとして返す。
// user/assistant の交互メッセージを返す。system prompt は含まない。
// セッションが存在しない場合は nil を返す。
func (m *LightMemory) RecentMessages(sessionID string) []llm.Message {
    m.mu.Lock()
    defer m.mu.Unlock()

    buf, exists := m.sessions[sessionID]
    if !exists || len(buf.turns) == 0 {
        return nil
    }

    messages := make([]llm.Message, 0, len(buf.turns)*2)
    for _, t := range buf.turns {
        messages = append(messages,
            llm.Message{Role: "user", Content: t.userMessage},
            llm.Message{Role: "assistant", Content: t.response},
        )
    }

    return messages
}

// Clear は指定セッションの履歴を削除する。
func (m *LightMemory) Clear(sessionID string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    delete(m.sessions, sessionID)
}

// ClearAll は全セッションの履歴を削除する（日次カットオーバー用）。
func (m *LightMemory) ClearAll() {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.sessions = make(map[string]*sessionBuffer)
}
```

**テスト**: `internal/domain/agent/light_memory_test.go` (新規)

```go
func TestLightMemory_RecordAndRetrieve(t *testing.T) {
    memory := NewLightMemory(3)
    sessionID := "test-session"

    // 2 ターン記録
    memory.Record(sessionID, "user1", "response1")
    memory.Record(sessionID, "user2", "response2")

    // 取得
    messages := memory.RecentMessages(sessionID)

    // 検証
    if len(messages) != 4 {
        t.Errorf("Expected 4 messages, got %d", len(messages))
    }

    expected := []struct {
        role    string
        content string
    }{
        {"user", "user1"},
        {"assistant", "response1"},
        {"user", "user2"},
        {"assistant", "response2"},
    }

    for i, exp := range expected {
        if messages[i].Role != exp.role {
            t.Errorf("Message[%d].Role = %s, want %s", i, messages[i].Role, exp.role)
        }
        if messages[i].Content != exp.content {
            t.Errorf("Message[%d].Content = %s, want %s", i, messages[i].Content, exp.content)
        }
    }
}

func TestLightMemory_MaxTurns(t *testing.T) {
    memory := NewLightMemory(2)
    sessionID := "test-session"

    // 3 ターン記録（maxTurns=2 を超える）
    memory.Record(sessionID, "user1", "response1")
    memory.Record(sessionID, "user2", "response2")
    memory.Record(sessionID, "user3", "response3")

    messages := memory.RecentMessages(sessionID)

    // 最新 2 ターンのみ保持されている
    if len(messages) != 4 {  // 2 ターン × 2 メッセージ
        t.Errorf("Expected 4 messages (2 turns), got %d", len(messages))
    }

    // 古い user1/response1 は削除されている
    if messages[0].Content == "user1" {
        t.Error("Old turn should be deleted")
    }
    if messages[0].Content != "user2" {
        t.Errorf("First message should be user2, got %s", messages[0].Content)
    }
}
```

---

### 3.4 CoderAgent 拡張

**ファイル**: `internal/domain/agent/coder.go`

```go
type CoderAgent struct {
    llmProvider    llm.LLMProvider
    toolRunner     ToolRunner
    mcpClient      MCPClient
    proposalPrompt string

    // Agent Persona 追加
    persona        AgentPersona   // 新規フィールド
    lightMemory    *LightMemory   // 新規フィールド（nil = 無効）
}

// WithPersona は Persona を設定する（Builder パターン）
func (c *CoderAgent) WithPersona(p AgentPersona) *CoderAgent {
    c.persona = p
    return c
}

// WithLightMemory は LightMemory を設定する（Builder パターン）
func (c *CoderAgent) WithLightMemory(m *LightMemory) *CoderAgent {
    c.lightMemory = m
    return c
}

// GenerateProposal は Proposal を生成する（改修版）
func (c *CoderAgent) GenerateProposal(ctx context.Context, t task.Task) (*proposal.Proposal, error) {
    // 1. Persona 適用（system prompt 構築）
    fullSystemPrompt := c.persona.BuildSystemPrompt(c.proposalPrompt)

    // 2. Messages 構築
    messages := []llm.Message{
        {Role: "system", Content: fullSystemPrompt},
    }

    // 3. LightMemory 過去ターン注入
    if c.lightMemory != nil {
        if recent := c.lightMemory.RecentMessages(t.ChatID()); len(recent) > 0 {
            messages = append(messages, recent...)
        }
    }

    // 4. 現在のリクエスト
    messages = append(messages, llm.Message{Role: "user", Content: t.UserMessage()})

    // 5. LLM 呼び出し
    req := llm.GenerateRequest{
        Messages:    messages,
        MaxTokens:   8192,
        Temperature: 0.5,
    }
    resp, err := c.llmProvider.Generate(ctx, req)
    if err != nil {
        return nil, err
    }

    // 6. Proposal 抽出
    p, err := extractProposal(resp.Content)
    if err != nil {
        return nil, err
    }

    // 7. LightMemory 記録（Plan 部分のみ）
    if c.lightMemory != nil && p != nil {
        c.lightMemory.Record(t.ChatID(), t.UserMessage(), p.Plan())
    }

    return p, nil
}
```

---

### 3.5 LLM Provider Factory

**ファイル**: `internal/infrastructure/llm/factory.go` (新規)

```go
package llm

import (
    "fmt"

    "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
    "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
    "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/anthropic"
    "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/deepseek"
    "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/gemini"
    "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/openai"
)

// CreateProvider は Config から LLMProvider を作成する
func CreateProvider(cfg config.CoderConfig) (llm.LLMProvider, error) {
    switch cfg.Provider {
    case "deepseek":
        return deepseek.NewProvider(cfg.APIKey, cfg.Model, cfg.BaseURL), nil
    case "openai":
        return openai.NewProvider(cfg.APIKey, cfg.Model), nil
    case "claude":
        return anthropic.NewProvider(cfg.APIKey, cfg.Model), nil
    case "gemini":
        return gemini.NewProvider(cfg.APIKey, cfg.Model), nil
    default:
        return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
    }
}
```

---

### 3.6 Gemini Provider 実装

**ファイル**: `internal/infrastructure/llm/gemini/provider.go` (新規)

```go
package gemini

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"

    "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

const (
    geminiAPIEndpoint = "https://generativelanguage.googleapis.com/v1beta/models"
)

type Provider struct {
    apiKey string
    model  string
    client *http.Client
}

func NewProvider(apiKey, model string) *Provider {
    return &Provider{
        apiKey: apiKey,
        model:  model,
        client: &http.Client{},
    }
}

func (p *Provider) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
    // Gemini API リクエスト構築
    geminiReq := geminiGenerateRequest{
        Contents: convertMessages(req.Messages),
        GenerationConfig: geminiGenerationConfig{
            Temperature:     req.Temperature,
            MaxOutputTokens: req.MaxTokens,
        },
    }

    // API 呼び出し
    url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiAPIEndpoint, p.model, p.apiKey)
    body, err := json.Marshal(geminiReq)
    if err != nil {
        return nil, fmt.Errorf("marshal request: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := p.client.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("http request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        bodyBytes, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("gemini API error: %s (status %d)", string(bodyBytes), resp.StatusCode)
    }

    // レスポンス解析
    var geminiResp geminiGenerateResponse
    if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
        return nil, fmt.Errorf("decode response: %w", err)
    }

    if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
        return nil, fmt.Errorf("no content in response")
    }

    content := geminiResp.Candidates[0].Content.Parts[0].Text

    return &llm.GenerateResponse{
        Content: content,
    }, nil
}

// Gemini API 型定義
type geminiGenerateRequest struct {
    Contents         []geminiContent        `json:"contents"`
    GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
    Role  string        `json:"role"`
    Parts []geminiPart  `json:"parts"`
}

type geminiPart struct {
    Text string `json:"text"`
}

type geminiGenerationConfig struct {
    Temperature     float64 `json:"temperature"`
    MaxOutputTokens int     `json:"maxOutputTokens"`
}

type geminiGenerateResponse struct {
    Candidates []geminiCandidate `json:"candidates"`
}

type geminiCandidate struct {
    Content geminiContent `json:"content"`
}

// convertMessages は llm.Message を Gemini 形式に変換
func convertMessages(messages []llm.Message) []geminiContent {
    contents := make([]geminiContent, 0, len(messages))
    for _, msg := range messages {
        role := msg.Role
        if role == "system" {
            role = "user"  // Gemini は system role をサポートしないため user に変換
        }
        contents = append(contents, geminiContent{
            Role: role,
            Parts: []geminiPart{
                {Text: msg.Content},
            },
        })
    }
    return contents
}
```

**テスト**: `internal/infrastructure/llm/gemini/provider_test.go` (新規)

---

## 4. 実装フェーズ

### Phase 0: Gemini Provider 実装

**作業内容**:
- [ ] `internal/infrastructure/llm/gemini/provider.go` 作成
- [ ] `internal/infrastructure/llm/gemini/provider_test.go` 作成
- [ ] Gemini API 統合テスト（実 API 呼び出し確認）

**成功基準**:
- Gemini API で正常に応答取得できる
- エラーハンドリングが適切

---

### Phase 1: Agent Persona 基盤

**作業内容**:
- [ ] `internal/domain/agent/persona.go` 作成
- [ ] `internal/domain/agent/persona_test.go` 作成
- [ ] `internal/domain/agent/light_memory.go` 作成
- [ ] `internal/domain/agent/light_memory_test.go` 作成

**成功基準**:
- 全テスト PASS
- BuildSystemPrompt() が正しく動作
- LightMemory が maxTurns で FIFO 削除する

---

### Phase 2: Config 拡張

**作業内容**:
- [ ] `config.go` に `Coder4 CoderConfig` フィールド追加
- [ ] `CoderConfig`, `LightMemoryConfig` 構造体定義
- [ ] `setDefaults()` に Coder4 デフォルト値追加
- [ ] `Validate()` に Coder4 検証ロジック追加
- [ ] `config_test.go` 更新

**成功基準**:
- Config 読み込みテスト PASS
- Coder4 デフォルト値が正しく設定される

---

### Phase 3: ルーティング拡張

**作業内容**:
- [ ] `route.go` に `RouteCODE4` 定数追加
- [ ] `IsCoderRoute()` に CODE4 追加
- [ ] `RouteToCoderSlot()` 関数追加
- [ ] `route_test.go` 更新

**成功基準**:
- ルーティングテスト PASS
- CODE4 が正しく coder4 にマッピングされる

---

### Phase 4: LLM Factory

**作業内容**:
- [ ] `internal/infrastructure/llm/factory.go` 作成
- [ ] `CreateProvider()` 実装
- [ ] エラーハンドリング（unknown provider）

**成功基準**:
- 4 種類の Provider を正しく生成できる
- unknown provider でエラーを返す

---

### Phase 5: CoderAgent 拡張

**作業内容**:
- [ ] `coder.go` に `persona`, `lightMemory` フィールド追加
- [ ] `WithPersona()`, `WithLightMemory()` メソッド追加
- [ ] `GenerateProposal()` 改修（Persona 適用、LightMemory 注入・記録）
- [ ] テスト更新

**成功基準**:
- Persona が system prompt に反映される
- LightMemory が過去ターンを注入する
- 既存テスト PASS（後方互換）

---

### Phase 6: Orchestrator 改修

**作業内容**:
- [ ] `message_orchestrator.go` に `coder4 *agent.CoderAgent` 追加
- [ ] `NewMessageOrchestrator()` シグネチャ更新
- [ ] `selectCoderForRoute()` に CODE4 追加
- [ ] `code_executor.go` に `coder4` 追加
- [ ] `NewDefaultCodeExecutor()` シグネチャ更新
- [ ] `selectCoderForRoute()` に CODE4 追加

**影響ファイル**:
- `message_orchestrator.go`
- `code_executor.go`
- `distributed_orchestrator.go`

**成功基準**:
- コンパイル成功
- 既存テスト PASS

---

### Phase 7: main.go 配線

**作業内容**:
- [ ] `setupCoders()` 関数実装
  - Config から 4 coders 読み込み
  - Provider 作成（Factory 使用）
  - CoderAgent インスタンス化
  - Persona 適用
  - LightMemory 適用
- [ ] Orchestrator 初期化コード更新
- [ ] 起動ログ出力追加

**成功基準**:
- picoclaw 起動成功
- 起動ログに coder1〜4 の情報が出力される

---

### Phase 8: Remote 実行対応

**作業内容**:
- [ ] `distributed_orchestrator.go` 改修
  - `Message.Context` に API キー・設定を含める
- [ ] `cmd/picoclaw-agent/main.go` 改修
  - `Message.Context` から設定を抽出
  - Provider 作成・Persona 適用

**成功基準**:
- coder3 (SSH 経由) が正常に動作する
- API キーが Worker から送信される

---

### Phase 9: テスト更新

**作業内容**:
- [ ] 全テストファイルで `coder1, coder2, coder3` → `coder1, coder2, coder3, coder4` 更新
- [ ] モック作成（coder4 用）
- [ ] 統合テスト追加

**影響ファイル（約20ファイル）**:
- `*_test.go` で coder を参照する全テスト

**成功基準**:
- `go test ./...` 全 PASS

---

### Phase 10: 統合テスト

**作業内容**:
- [ ] 4 coders 動作確認（Local 実行）
- [ ] Agent Persona 動作確認（応答に反映）
- [ ] LightMemory 動作確認（2 ターン目で文脈保持）
- [ ] 分散実行確認（coder3 SSH 経由）
- [ ] Gemini Provider 動作確認

**成功基準**:
- CODE1/2/3/4 ルーティングが正常動作
- Persona が応答文体に反映される
- LightMemory が文脈を保持する

---

## 5. 検証項目

### 5.1 機能検証

| 項目 | 検証内容 | 期待結果 |
|------|----------|----------|
| Coder 起動 | picoclaw 起動時に 4 coders が初期化される | 起動ログに coder1〜4 の情報が出力される |
| ルーティング | CODE1/2/3/4 が正しいスロットにルーティングされる | coder1/2/3/4 が実行される |
| Gemini Provider | Gemini API で Proposal 生成できる | 正常な Proposal が返る |
| Agent Persona | Persona が応答に反映される | 「あなたは Aka」等の文体が応答に現れる |
| LightMemory | 2 ターン目で 1 ターン目の文脈を参照する | 「前回は〜」等の文脈参照が見られる |
| Remote 実行 | coder3 が SSH 経由で動作する | Windows の picoclaw-agent が実行され、結果が返る |
| API キー送信 | Worker の環境変数が Remote へ送信される | Remote 側でログに API キー長が出力される |

### 5.2 後方互換性検証

| 項目 | 検証内容 | 期待結果 |
|------|----------|----------|
| Persona なし | `personality: ""` でも動作する | 既存の応答と同じ |
| LightMemory なし | `enabled: false` でも動作する | 過去ターン注入なしで動作 |
| coder1/2/3 | 既存の 3 coders の動作が変わらない | 既存テスト PASS |

### 5.3 パフォーマンス検証

| 項目 | 基準値 | 測定方法 |
|------|--------|----------|
| LightMemory メモリ使用量 | < 1MB | maxTurns=3 × 100 sessions でメモリ使用量測定 |
| SSH 通信オーバーヘッド | < 100ms | coder3 実行時の往復時間測定 |

---

## 6. マイグレーション

### 6.1 既存環境からの移行

**既存設定（v3）**:
```yaml
deepseek:
  api_key: "..."
  model: "..."
openai:
  api_key: "..."
claude:
  api_key: "..."
```

**新設定（v4）**:
```yaml
coder1:
  name: "aka"
  provider: "deepseek"
  api_key: "${DEEPSEEK_API_KEY}"
  model: "deepseek-coder"
  enabled: true
# ... coder2, coder3, coder4 同様
```

**移行手順**:
1. config.yaml に `coder1〜4` セクション追加
2. 環境変数に API キー設定
3. picoclaw 再起動
4. 動作確認後、古い `deepseek:` 等のセクションを削除（オプション）

---

## 7. トラブルシューティング

### 7.1 Gemini Provider エラー

**症状**: `gemini API error: ... (status 400)`

**原因**: API キーが無効、または model 名が間違っている

**対処**:
```bash
# API キー確認
echo $GEMINI_API_KEY

# model 名確認（Gemini API ドキュメント参照）
# 正: gemini-2.0-flash-exp
# 誤: gemini-2.0-flash
```

### 7.2 Persona が反映されない

**症状**: 応答に Persona の文体が現れない

**原因**: `personality: ""` が空文字、または LLM が無視している

**対処**:
```yaml
# personality を具体的に記述
coder1:
  personality: |
    あなたは Aka。次の性格を持つ：
    - 設計思考が得意
    - 落ち着いた口調
    （具体的な指示を追加）
```

### 7.3 LightMemory が動作しない

**症状**: 2 ターン目で過去ターンを参照しない

**原因**: `enabled: false`、または sessionID が異なる

**対処**:
```yaml
# enabled を true に
coder1:
  light_memory:
    enabled: true
    max_turns: 3

# ログで sessionID を確認
tail -f ~/.picoclaw/logs/picoclaw.log | grep sessionID
```

---

## 8. 参照

### 8.1 関連仕様

- **正本仕様**: `docs/01_正本仕様/実装仕様.md` §4.2 Coder Agent
- **分散実行**: `docs/04_実装仕様_機能拡張/実装仕様_分散実行_v4.md`
- **Agent Persona 設計思想**: `docs/04_実装仕様_機能拡張/実装仕様_エージェントペルソナ_v1.md`
- **分散実行セットアップ**: `docs/運用ガイド/分散実行_前提条件とセットアップ.md`

### 8.2 実装ファイル

**新規ファイル（7ファイル）**:
1. `internal/domain/agent/persona.go`
2. `internal/domain/agent/persona_test.go`
3. `internal/domain/agent/light_memory.go`
4. `internal/domain/agent/light_memory_test.go`
5. `internal/infrastructure/llm/gemini/provider.go`
6. `internal/infrastructure/llm/gemini/provider_test.go`
7. `internal/infrastructure/llm/factory.go`

**主要修正ファイル**:
1. `internal/adapter/config/config.go`
2. `internal/domain/routing/route.go`
3. `internal/domain/agent/coder.go`
4. `internal/application/orchestrator/message_orchestrator.go`
5. `internal/application/orchestrator/code_executor.go`
6. `cmd/picoclaw/main.go`
7. `cmd/picoclaw-agent/main.go`

---

**最終更新**: 2026-03-26
**メンテナンス**: 実装完了後、本仕様書の「ステータス」を「実装完了」に更新すること
