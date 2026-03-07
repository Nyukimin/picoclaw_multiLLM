package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/ollama"
)

const usageText = `kb-admin - Knowledge Base 管理ツール

使い方:
  kb-admin <command> [options]

コマンド:
  list <domain>           指定ドメインのドキュメント一覧を表示
  search <domain> <query> KB検索をテスト実行
  stats                   統計情報を表示（全コレクション）
  cleanup <domain> <days> 指定日数より古いドキュメントを削除

オプション:
  --config <path>  設定ファイルのパス (default: ./config.yaml)

例:
  kb-admin list programming
  kb-admin search movie "おすすめの映画"
  kb-admin stats
  kb-admin cleanup general 30
`

func main() {
	configPath := flag.String("config", "./config.yaml", "Path to config file")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usageText)
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	// 設定読み込み
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// ConversationManager 初期化
	mgr, err := initManager(cfg)
	if err != nil {
		log.Fatalf("Failed to init manager: %v", err)
	}
	defer mgr.Close()

	ctx := context.Background()

	// コマンド実行
	command := args[0]
	switch command {
	case "list":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: domain required")
			fmt.Fprintln(os.Stderr, "Usage: kb-admin list <domain>")
			os.Exit(1)
		}
		if err := cmdList(ctx, mgr, args[1]); err != nil {
			log.Fatalf("list failed: %v", err)
		}

	case "search":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: domain and query required")
			fmt.Fprintln(os.Stderr, "Usage: kb-admin search <domain> <query>")
			os.Exit(1)
		}
		domain := args[1]
		query := strings.Join(args[2:], " ")
		if err := cmdSearch(ctx, mgr, domain, query); err != nil {
			log.Fatalf("search failed: %v", err)
		}

	case "stats":
		if err := cmdStats(ctx, mgr); err != nil {
			log.Fatalf("stats failed: %v", err)
		}

	case "cleanup":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: domain and days required")
			fmt.Fprintln(os.Stderr, "Usage: kb-admin cleanup <domain> <days>")
			os.Exit(1)
		}
		domain := args[1]
		var days int
		if _, err := fmt.Sscanf(args[2], "%d", &days); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid days '%s'\n", args[2])
			os.Exit(1)
		}
		if err := cmdCleanup(ctx, mgr, domain, days); err != nil {
			log.Fatalf("cleanup failed: %v", err)
		}

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

// loadConfig は設定ファイルを読み込み、環境変数を展開する
func loadConfig(path string) (*config.Config, error) {
	// .env ファイル読み込み
	homeDir, _ := os.UserHomeDir()
	loadDotEnv(filepath.Join(homeDir, ".picoclaw", ".env"))
	loadDotEnv(filepath.Join(filepath.Dir(path), ".env"))

	return config.LoadConfig(path)
}

// loadDotEnv は指定パスの.envファイルを読み込み、未設定の環境変数をセット
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

// initManager は RealConversationManager を初期化
func initManager(cfg *config.Config) (*conversationpersistence.RealConversationManager, error) {
	if !cfg.Conversation.Enabled {
		return nil, fmt.Errorf("conversation system is not enabled in config")
	}

	mgr, err := conversationpersistence.NewRealConversationManager(
		cfg.Conversation.RedisURL,
		cfg.Conversation.DuckDBPath,
		cfg.Conversation.VectorDBURL,
	)
	if err != nil {
		return nil, err
	}

	// Embedder 注入（embed_model が設定されている場合）
	if cfg.Conversation.EmbedModel != "" {
		embedder := ollama.NewOllamaEmbedder(cfg.Ollama.BaseURL, cfg.Conversation.EmbedModel)
		mgr.WithEmbedder(embedder)
		log.Printf("KB-Admin: Embedder injected (model: %s)", cfg.Conversation.EmbedModel)
	} else {
		log.Println("Warning: No embed_model configured - KB search may not work correctly")
	}

	return mgr, nil
}

// cmdList はドメイン内のドキュメント一覧を表示
func cmdList(ctx context.Context, mgr *conversationpersistence.RealConversationManager, domain string) error {
	fmt.Printf("📚 Domain: %s\n\n", domain)

	// TODO: RealConversationManager に ListKBDocuments を公開する
	// 現状は直接VectorDBStoreにアクセスできないため、search コマンドで代用
	fmt.Println("⚠️  Note: Use 'search' command with a broad query to list documents")
	fmt.Println("Example: kb-admin search", domain, "\"*\"")

	return nil
}

// cmdSearch はKB検索をテスト実行
func cmdSearch(ctx context.Context, mgr *conversationpersistence.RealConversationManager, domain string, query string) error {
	fmt.Printf("🔍 Searching KB in domain '%s' for: %s\n\n", domain, query)

	docs, err := mgr.SearchKB(ctx, domain, query, 10)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(docs) == 0 {
		fmt.Println("No documents found.")
		return nil
	}

	fmt.Printf("Found %d documents:\n\n", len(docs))
	for i, doc := range docs {
		fmt.Printf("--- Document %d ---\n", i+1)
		fmt.Printf("ID:     %s\n", doc.ID)
		fmt.Printf("Source: %s\n", doc.Source)
		fmt.Printf("Score:  %.4f\n", doc.Score)
		fmt.Printf("Created: %s\n", doc.CreatedAt.Format(time.RFC3339))

		// Content preview (first 200 chars)
		content := doc.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Printf("Content:\n%s\n\n", content)
	}

	return nil
}

// cmdStats は統計情報を表示
func cmdStats(ctx context.Context, mgr *conversationpersistence.RealConversationManager) error {
	fmt.Println("📊 Knowledge Base Statistics")

	// TODO: RealConversationManager に GetKBCollections/GetKBStats を公開する
	// 現状は既知のドメインを手動チェック
	domains := []string{"general", "programming", "movie", "anime", "tech", "history"}

	fmt.Println("\nChecking known domains:")
	for _, domain := range domains {
		// 簡易チェック: SearchKB で検索してみる
		docs, err := mgr.SearchKB(ctx, domain, "test", 1)
		if err != nil {
			fmt.Printf("  ❌ %s - Error: %v\n", domain, err)
			continue
		}
		status := "empty"
		if len(docs) > 0 {
			status = "has documents"
		}
		fmt.Printf("  ✓ %s - %s\n", domain, status)
	}

	fmt.Println("\nNote: Use 'search <domain> <query>' to explore specific domains")
	return nil
}

// cmdCleanup は古いドキュメントを削除
func cmdCleanup(ctx context.Context, mgr *conversationpersistence.RealConversationManager, domain string, days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)
	fmt.Printf("🗑️  Cleanup Policy\n")
	fmt.Printf("Domain: %s\n", domain)
	fmt.Printf("Delete documents older than: %d days (before %s)\n\n", days, cutoff.Format("2006-01-02"))

	// TODO: RealConversationManager に DeleteOldKBDocuments を公開する
	fmt.Println("⚠️  Cleanup requires administrative access to VectorDB")
	fmt.Println("This feature will be implemented in Phase 4.2 production deployment")
	fmt.Println("\nFor manual cleanup, use Qdrant web UI or API directly:")
	fmt.Printf("  Collection: kb_%s\n", domain)
	fmt.Printf("  Filter: created_at < %d\n", cutoff.Unix())

	return nil
}
