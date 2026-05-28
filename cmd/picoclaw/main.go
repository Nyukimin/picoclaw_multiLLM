package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
)

// Version 情報（go build -ldflags で注入）
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "run":
		cmdRun()
	case "version", "-v", "--version":
		cmdVersion()
	case "health":
		cmdHealth()
	case "status":
		cmdStatus()
	case "doctor":
		cmdDoctor()
	case "channels":
		cmdChannels()
	case "gateway":
		cmdGateway()
	case "ollama":
		cmdOllama()
	case "logs":
		cmdLogs()
	case "evidence":
		cmdEvidence()
	case "source-registry":
		cmdSourceRegistry()
	case "knowledge":
		cmdKnowledge()
	case "help", "-h", "--help":
		cmdHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		cmdHelp()
		os.Exit(1)
	}
}

// cmdRun はHTTPサーバーを起動する（デフォルトコマンド）
func cmdRun() {
	configPath := getConfigPath()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("RenCrow %s (commit: %s, built: %s)", Version, Commit, BuildDate)
	log.Printf("Loaded config from: %s", configPath)

	dependencies := buildDependencies(cfg)

	// Graceful shutdown用シグナル
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		log.Printf("Received signal: %v, shutting down...", sig)
		dependencies.Shutdown()
		os.Exit(0)
	}()

	// HTTPサーバー起動
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting RenCrow server on %s", addr)

	mux := http.NewServeMux()
	registerChannelRoutes(mux, dependencies)

	// Live Viewer
	sttRuntime := buildSTTRuntime(cfg)
	debugSystemOpts := sttRuntime.DebugOptions
	llmOpsToken := strings.TrimSpace(os.Getenv("LLM_OPS_TOKEN"))
	debugSystemOpts.LLMOpsConfigured = cfg.LLMOps.Enabled && strings.TrimSpace(cfg.LLMOps.BaseURL) != ""
	debugSystemOpts.LLMOpsEnabled = debugSystemOpts.LLMOpsConfigured && llmOpsToken != ""
	debugSystemOpts.LLMOpsBaseURL = cfg.LLMOps.BaseURL
	debugSystemOpts.RuntimeReadiness = buildRuntimeDependencyReadiness(cfg, dependencies)
	debugSystemOpts.LocalLLM = viewer.LocalLLMRuntimeConfig{
		Enabled:           cfg.LocalLLM.Enabled,
		Provider:          cfg.LocalLLM.Provider,
		ChatBaseURL:       localLLMBaseURLForAlias(cfg, "chat"),
		WorkerBaseURL:     localLLMBaseURLForAlias(cfg, "worker"),
		HeavyBaseURL:      localLLMBaseURLForAlias(cfg, "heavy"),
		WildBaseURL:       localLLMBaseURLForAlias(cfg, "wild"),
		ChatModel:         cfg.LocalLLM.ChatModel,
		WorkerModel:       cfg.LocalLLM.WorkerModel,
		HeavyModel:        localLLMModelForAlias(cfg, "heavy"),
		WildModel:         cfg.LocalLLM.WildModel,
		TimeoutSec:        cfg.LocalLLM.TimeoutSec,
		GlobalConcurrency: cfg.LocalLLM.GlobalConcurrency,
		ModelConcurrency:  cfg.LocalLLM.ModelConcurrency,
	}
	debugSystemOpts.WebwrightFetch = viewer.WebwrightFetchRuntimeConfig{
		Enabled:           cfg.WebwrightFetch.Enabled,
		RunnerPath:        cfg.WebwrightFetch.RunnerPath,
		ConfigPath:        cfg.WebwrightFetch.ConfigPath,
		OutputDir:         cfg.WebwrightFetch.OutputDir,
		StagingOutputDir:  cfg.WebwrightFetch.StagingOutputDir,
		UvxFrom:           cfg.WebwrightFetch.UvxFrom,
		Python:            cfg.WebwrightFetch.Python,
		ResponsesEndpoint: cfg.WebwrightFetch.ResponsesEndpoint,
		Model:             cfg.WebwrightFetch.Model,
		APIKeyConfigured:  strings.TrimSpace(cfg.WebwrightFetch.APIKey) != "",
	}
	if cfg.LLMOps.Enabled && strings.TrimSpace(cfg.LLMOps.BaseURL) != "" && llmOpsToken == "" {
		log.Printf("WARN: llm_ops is enabled in config but LLM_OPS_TOKEN is empty; Viewer MLX control API disabled")
	}
	registerViewerBaseRoutes(mux, cfg, dependencies, debugSystemOpts)
	registerLLMOpsRoutes(mux, cfg, dependencies, &debugSystemOpts)
	registerSTTAndAudioRoutes(mux, sttRuntime, dependencies)
	registerViewerDynamicRoutes(mux, dependencies)
	registerEntryAndChromeRoutes(mux, dependencies)
	registerIdleChatRoutes(mux, dependencies)
	registerHealthRoutes(mux, dependencies, cfg)

	server := &http.Server{
		Addr:    addr,
		Handler: withTailscaleViewerOnlyGuard(mux),
		ConnState: func(conn net.Conn, state http.ConnState) {
			log.Printf("[ConnState] %s -> %s (remote: %s)", state.String(), conn.LocalAddr(), conn.RemoteAddr())
		},
	}
	if cfg.Server.TLS.Enabled {
		err = server.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
	} else {
		err = server.ListenAndServe()
	}
	if err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// getConfigPath は設定ファイルパスを取得
func getConfigPath() string {
	if path := os.Getenv("PICOCLAW_CONFIG"); path != "" {
		return path
	}
	return "./config.yaml"
}

func defaultAssetsGitRepoPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return filepath.Join(".picoclaw", "assets-repo")
	}
	return filepath.Join(homeDir, ".picoclaw", "assets-repo")
}
