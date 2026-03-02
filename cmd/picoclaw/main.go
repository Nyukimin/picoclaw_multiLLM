package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/line"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/claude"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/deepseek"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/ollama"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/openai"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/routing"
)

// coderAdapter はdomain CoderAgentをorchestrator CoderAgentに適応
type coderAdapter struct {
	domainCoder *agent.CoderAgent
}

func (a *coderAdapter) Generate(ctx context.Context, t task.Task, systemPrompt string) (string, error) {
	// 簡易実装：ProposalのPlan部分を返す
	// TODO: 完全なProposal処理を実装
	proposal, err := a.domainCoder.GenerateProposal(ctx, t)
	if err != nil {
		return "", err
	}
	if proposal == nil {
		return "No proposal generated", nil
	}
	return fmt.Sprintf("Plan:\n%s\n\nPatch:\n%s", proposal.Plan(), proposal.Patch()), nil
}

func main() {
	// 設定ファイルパス
	configPath := getConfigPath()

	// 設定読み込み
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Loaded config from: %s", configPath)

	// 依存関係構築
	dependencies := buildDependencies(cfg)

	// HTTPサーバー起動
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting PicoClaw server on %s", addr)

	if err := http.ListenAndServe(addr, dependencies.lineHandler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// Dependencies はアプリケーション依存関係
type Dependencies struct {
	lineHandler http.Handler
}

// buildDependencies は依存関係を構築
func buildDependencies(cfg *config.Config) *Dependencies {
	// 1. LLM Providers
	ollamaChatProvider := ollama.NewOllamaProvider(cfg.Ollama.BaseURL, cfg.Ollama.ChatModel)
	ollamaWorkerProvider := ollama.NewOllamaProvider(cfg.Ollama.BaseURL, cfg.Ollama.WorkerModel)

	var coder1Adapter, coder2Adapter, coder3Adapter *coderAdapter

	// DeepSeek (Coder1) - API キーがある場合のみ
	if cfg.DeepSeek.APIKey != "" {
		deepseekProvider := deepseek.NewDeepSeekProvider(cfg.DeepSeek.APIKey, cfg.DeepSeek.Model)
		domainCoder := agent.NewCoderAgent(deepseekProvider, nil, nil)
		coder1Adapter = &coderAdapter{domainCoder: domainCoder}
		log.Printf("DeepSeek (Coder1) enabled with model: %s", cfg.DeepSeek.Model)
	}

	// OpenAI (Coder2) - API キーがある場合のみ
	if cfg.OpenAI.APIKey != "" {
		openaiProvider := openai.NewOpenAIProvider(cfg.OpenAI.APIKey, cfg.OpenAI.Model)
		domainCoder := agent.NewCoderAgent(openaiProvider, nil, nil)
		coder2Adapter = &coderAdapter{domainCoder: domainCoder}
		log.Printf("OpenAI (Coder2) enabled with model: %s", cfg.OpenAI.Model)
	}

	// Claude (Coder3) - API キーがある場合のみ
	if cfg.Claude.APIKey != "" {
		claudeProvider := claude.NewClaudeProvider(cfg.Claude.APIKey, cfg.Claude.Model)
		domainCoder := agent.NewCoderAgent(claudeProvider, nil, nil)
		coder3Adapter = &coderAdapter{domainCoder: domainCoder}
		log.Printf("Claude (Coder3) enabled with model: %s", cfg.Claude.Model)
	}

	// 2. Routing Components
	classifier := routing.NewLLMClassifier(ollamaChatProvider)
	ruleDictionary := routing.NewRuleDictionary()

	// 3. Agents
	mioAgent := agent.NewMioAgent(ollamaChatProvider, classifier, ruleDictionary)
	shiroAgent := agent.NewShiroAgent(ollamaWorkerProvider, nil, nil) // ToolRunner, MCPClient は後で実装

	// 4. Session Repository
	sessionRepo := session.NewJSONSessionRepository(cfg.Session.StorageDir)

	// セッションディレクトリ作成
	if err := os.MkdirAll(cfg.Session.StorageDir, 0755); err != nil {
		log.Fatalf("Failed to create session directory: %v", err)
	}

	// 5. Application Orchestrator
	orch := orchestrator.NewMessageOrchestrator(
		sessionRepo,
		mioAgent,
		shiroAgent,
		coder1Adapter,
		coder2Adapter,
		coder3Adapter,
	)

	// 6. Adapter (LINE Handler)
	lineHandler := line.NewHandler(orch, "", "") // Channel Secret/Access Token は後で設定

	log.Println("Dependency injection complete")

	return &Dependencies{
		lineHandler: lineHandler,
	}
}

// getConfigPath は設定ファイルパスを取得
func getConfigPath() string {
	if path := os.Getenv("PICOCLAW_CONFIG"); path != "" {
		return path
	}
	return "./config.yaml"
}
