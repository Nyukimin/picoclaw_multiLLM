package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/claude"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/deepseek"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/openai"
)

func main() {
	// コマンドライン引数でCoderタイプとタスクを受け取る
	if len(os.Args) < 3 {
		fmt.Println("Usage: test-coder <coder-type> <task-description>")
		fmt.Println("Example: test-coder deepseek 'hello.goにHello World関数を追加'")
		fmt.Println("Example: test-coder openai 'main.goにロギング機能を追加'")
		fmt.Println("Example: test-coder claude 'pkg/test/にユニットテストを追加'")
		os.Exit(1)
	}
	coderType := os.Args[1]
	taskDescription := os.Args[2]

	// 設定読み込み
	cfg, err := config.LoadConfig("./config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx := context.Background()
	var coder *agent.CoderAgent
	var coderName string

	// Coderタイプに応じてプロバイダー選択
	switch coderType {
	case "deepseek", "coder1":
		apiKey := os.Getenv("DEEPSEEK_API_KEY")
		if apiKey == "" {
			apiKey = cfg.DeepSeek.APIKey
		}
		if apiKey == "" {
			log.Fatal("DEEPSEEK_API_KEY not set")
		}
		provider := deepseek.NewDeepSeekProvider(apiKey, cfg.DeepSeek.Model)
		coder = agent.NewCoderAgent(provider, nil, nil)
		coderName = "Coder1 (DeepSeek)"

	case "openai", "coder2":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			apiKey = cfg.OpenAI.APIKey
		}
		if apiKey == "" {
			log.Fatal("OPENAI_API_KEY not set")
		}
		provider := openai.NewOpenAIProvider(apiKey, cfg.OpenAI.Model)
		coder = agent.NewCoderAgent(provider, nil, nil)
		coderName = "Coder2 (OpenAI)"

	case "claude", "coder3":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			apiKey = cfg.Claude.APIKey
		}
		if apiKey == "" {
			log.Fatal("ANTHROPIC_API_KEY not set")
		}
		provider := claude.NewClaudeProvider(apiKey, cfg.Claude.Model)
		coder = agent.NewCoderAgent(provider, nil, nil)
		coderName = "Coder3 (Claude)"

	default:
		log.Fatalf("Unknown coder type: %s (use: deepseek, openai, or claude)", coderType)
	}

	// Task作成
	jobID := task.NewJobID()
	t := task.NewTask(jobID, taskDescription, "cli", "test-user")

	fmt.Printf("🤖 Coder: %s\n", coderName)
	fmt.Printf("📝 Task: %s\n", taskDescription)
	fmt.Println("⏳ Generating Proposal...")

	// Proposal生成
	proposal, err := coder.GenerateProposal(ctx, t)
	if err != nil {
		log.Fatalf("❌ Failed to generate proposal: %v", err)
	}

	// 結果表示
	sep := strings.Repeat("=", 60)
	fmt.Println("\n" + sep)
	fmt.Println("📋 PLAN")
	fmt.Println(sep)
	fmt.Println(proposal.Plan())

	fmt.Println("\n" + sep)
	fmt.Println("🔧 PATCH")
	fmt.Println(sep)
	fmt.Println(proposal.Patch())

	fmt.Println("\n" + sep)
	fmt.Println("⚠️  RISK")
	fmt.Println(sep)
	fmt.Println(proposal.Risk())

	if proposal.CostHint() != "" {
		fmt.Println("\n" + sep)
		fmt.Println("💰 COST HINT")
		fmt.Println(sep)
		fmt.Println(proposal.CostHint())
	}

	fmt.Println("\n✅ Proposal generated successfully!")
}
