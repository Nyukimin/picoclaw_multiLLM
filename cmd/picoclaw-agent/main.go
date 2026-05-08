package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/claude"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/deepseek"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/ollama"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/openai"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/mcp"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tools"
)

const shutdownTimeout = 30 * time.Second

// AgentHandler はスタンドアロンAgentの処理インターフェース
type AgentHandler interface {
	HandleMessage(ctx context.Context, msg domaintransport.Message) (domaintransport.Message, error)
}

// workerHandler はWorkerエージェントのハンドラ
type workerHandler struct {
	shiroAgent       *agent.ShiroAgent
	executionService service.WorkerExecutionService
}

func (h *workerHandler) HandleMessage(ctx context.Context, msg domaintransport.Message) (domaintransport.Message, error) {
	// Proposal付きメッセージ → Worker即時実行
	if msg.Proposal != nil {
		return h.executeProposal(ctx, msg)
	}

	// Proposalなし → ShiroAgentでタスク実行
	return h.executeTask(ctx, msg)
}

// executeProposal はProposalのPatchをWorkerが即時実行
func (h *workerHandler) executeProposal(ctx context.Context, msg domaintransport.Message) (domaintransport.Message, error) {
	// ProposalPayload → domain Proposal に復元
	p := proposal.Reconstruct(
		msg.Proposal.Plan,
		msg.Proposal.Patch,
		msg.Proposal.Risk,
		msg.Proposal.CostHint,
	)

	// JobID をパース
	jobID, err := task.ParseJobID(msg.JobID)
	if err != nil {
		return domaintransport.Message{}, fmt.Errorf("invalid job ID: %w", err)
	}

	// Patch実行
	result, err := h.executionService.ExecuteProposal(ctx, jobID, p)
	if err != nil {
		errResp := domaintransport.NewMessage(msg.To, msg.From, msg.SessionID, msg.JobID,
			fmt.Sprintf("patch execution failed: %v", err))
		errResp.Type = domaintransport.MessageTypeError
		return errResp, nil
	}

	// 結果をResultPayloadに変換
	response := domaintransport.NewMessage(msg.To, msg.From, msg.SessionID, msg.JobID, result.Summary)
	response.Type = domaintransport.MessageTypeResult
	response.Result = &domaintransport.ResultPayload{
		Success:      result.FailedCmds == 0,
		Summary:      result.Summary,
		ExecutedCmds: result.ExecutedCmds,
		FailedCmds:   result.FailedCmds,
		GitCommit:    result.GitCommit,
	}

	return response, nil
}

// executeTask はShiroAgentでタスクを実行
func (h *workerHandler) executeTask(ctx context.Context, msg domaintransport.Message) (domaintransport.Message, error) {
	jobID, err := task.ParseJobID(msg.JobID)
	if err != nil {
		jobID = task.NewJobID()
	}

	t := task.NewTask(jobID, msg.Content, "standalone", "agent")

	result, err := h.shiroAgent.Execute(ctx, t)
	if err != nil {
		errResp := domaintransport.NewMessage(msg.To, msg.From, msg.SessionID, msg.JobID,
			fmt.Sprintf("worker execution failed: %v", err))
		errResp.Type = domaintransport.MessageTypeError
		return errResp, nil
	}

	response := domaintransport.NewMessage(msg.To, msg.From, msg.SessionID, msg.JobID, result)
	response.Type = domaintransport.MessageTypeResult
	response.Result = &domaintransport.ResultPayload{
		Success: true,
		Summary: result,
	}
	return response, nil
}

// coderHandler はCoderエージェントのハンドラ
type coderHandler struct {
	agentName      string
	coderAgent     *agent.CoderAgent // Fallback agent (local config)
	proposalPrompt string
	globalMemory   *agent.LightMemory // 共有メモリ（SSH経由の場合は再利用）
}

func (h *coderHandler) HandleMessage(ctx context.Context, msg domaintransport.Message) (domaintransport.Message, error) {
	jobID, err := task.ParseJobID(msg.JobID)
	if err != nil {
		jobID = task.NewJobID()
	}

	t := task.NewTask(jobID, msg.Content, "standalone", "agent")

	// v4.1: Message.Context から CoderConfig を抽出して動的に Provider 作成
	activeAgent := h.coderAgent // デフォルトはローカル設定の Agent
	if msg.Context != nil {
		if coderCfgRaw, ok := msg.Context["coder_config"]; ok {
			// CoderConfig を復元
			coderCfg, err := extractCoderConfig(coderCfgRaw)
			if err != nil {
				log.Printf("[coderHandler] Failed to extract CoderConfig from Context: %v", err)
			} else {
				// CoderConfig から Provider を動的作成
				provider, err := createProviderFromConfig(coderCfg)
				if err != nil {
					log.Printf("[coderHandler] Failed to create provider from CoderConfig: %v", err)
				} else {
					// 一時的な CoderAgent を作成
					tempAgent := agent.NewCoderAgent(provider, nil, nil, h.proposalPrompt)

					// Persona 適用
					if coderCfg.Personality != "" {
						persona := agent.AgentPersona{
							Name:        coderCfg.Name,
							Personality: coderCfg.Personality,
							Tone:        coderCfg.Tone,
						}
						tempAgent.WithPersona(persona)
						log.Printf("[coderHandler] Applied Persona: %s", coderCfg.Name)
					}

					// LightMemory 適用（SSH 経由では共有インスタンス再利用）
					if coderCfg.LightMemory.Enabled {
						if h.globalMemory == nil {
							h.globalMemory = agent.NewLightMemory(coderCfg.LightMemory.MaxTurns)
						}
						tempAgent.WithLightMemory(h.globalMemory)
						log.Printf("[coderHandler] Applied LightMemory: max_turns=%d", coderCfg.LightMemory.MaxTurns)
					}

					activeAgent = tempAgent
					log.Printf("[coderHandler] Using remote CoderConfig: provider=%s, model=%s", coderCfg.Provider, coderCfg.Model)
				}
			}
		}
	}

	// CoderAgentでProposal生成
	p, err := activeAgent.GenerateProposal(ctx, t)
	if err != nil {
		errResp := domaintransport.NewMessage(msg.To, msg.From, msg.SessionID, msg.JobID,
			fmt.Sprintf("proposal generation failed: %v", err))
		errResp.Type = domaintransport.MessageTypeError
		return errResp, nil
	}

	if p == nil {
		errResp := domaintransport.NewMessage(msg.To, msg.From, msg.SessionID, msg.JobID,
			"proposal generation returned empty result (invalid format)")
		errResp.Type = domaintransport.MessageTypeError
		return errResp, nil
	}

	response := domaintransport.NewMessage(msg.To, msg.From, msg.SessionID, msg.JobID,
		fmt.Sprintf("Proposal generated by %s", h.agentName))
	response.Type = domaintransport.MessageTypeResult
	response.Proposal = &domaintransport.ProposalPayload{
		Plan:     p.Plan(),
		Patch:    p.Patch(),
		Risk:     p.Risk(),
		CostHint: p.CostHint(),
	}

	return response, nil
}

// extractCoderConfig は Message.Context から CoderConfig を抽出
func extractCoderConfig(raw interface{}) (config.CoderConfig, error) {
	// JSON 経由で送られてくるため、map[string]interface{} として扱う
	cfgMap, ok := raw.(map[string]interface{})
	if !ok {
		return config.CoderConfig{}, fmt.Errorf("coder_config is not a map")
	}

	// JSON として再エンコード → デコード
	jsonBytes, err := json.Marshal(cfgMap)
	if err != nil {
		return config.CoderConfig{}, fmt.Errorf("failed to marshal coder_config: %w", err)
	}

	var coderCfg config.CoderConfig
	if err := json.Unmarshal(jsonBytes, &coderCfg); err != nil {
		return config.CoderConfig{}, fmt.Errorf("failed to unmarshal coder_config: %w", err)
	}

	return coderCfg, nil
}

// createProviderFromConfig は CoderConfig から LLM Provider を作成
func createProviderFromConfig(cfg config.CoderConfig) (llm.LLMProvider, error) {
	// APIKey が環境変数参照形式（${...}）の場合は展開
	apiKey := os.ExpandEnv(cfg.APIKey)

	switch cfg.Provider {
	case "deepseek":
		if apiKey == "" {
			return nil, fmt.Errorf("DeepSeek provider requires API key")
		}
		model := cfg.Model
		if model == "" {
			model = "deepseek-chat"
		}
		return deepseek.NewDeepSeekProvider(apiKey, model), nil

	case "openai":
		if apiKey == "" {
			return nil, fmt.Errorf("OpenAI provider requires API key")
		}
		model := cfg.Model
		if model == "" {
			model = "gpt-4"
		}
		return openai.NewOpenAIProvider(apiKey, model), nil

	case "claude":
		if apiKey == "" {
			return nil, fmt.Errorf("Claude provider requires API key")
		}
		model := cfg.Model
		if model == "" {
			model = "claude-3-5-sonnet-20241022"
		}
		return claude.NewClaudeProvider(apiKey, model), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

// loadDotEnv は指定パスの.envファイルを読み込み、未設定の環境変数をセット
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // ファイルがなければスキップ
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
		if os.Getenv(key) == "" { // 既存の環境変数を上書きしない
			os.Setenv(key, val)
		}
	}
}

// protectStdout はstdout fd を通信専用fd として早期確保し、fd1 を stderr にリダイレクトする。
// これにより CGO ライブラリ等の想定外の stdout 書き込みから JSON 通信チャネルを保護する。
func protectStdout() io.Writer {
	fd, err := syscall.Dup(syscall.Stdout)
	if err != nil {
		return os.Stdout
	}
	if err := syscall.Dup2(syscall.Stderr, syscall.Stdout); err != nil {
		syscall.Close(fd)
		return os.Stdout
	}
	return os.NewFile(uintptr(fd), "json-out")
}

func main() {
	standalone := flag.Bool("standalone", false, "Run in standalone mode")
	agentType := flag.String("agent", "", "Agent type: worker, coder1, coder2, coder3, audio_router")
	configPath := flag.String("config", "./config.yaml", "Path to config file")
	flag.Parse()

	// JSON 通信チャネルを汚染から保護（ライブラリ init より前に実行）
	jsonOut := protectStdout()

	if !*standalone {
		fmt.Fprintln(os.Stderr, "picoclaw-agent must be run with --standalone flag")
		os.Exit(1)
	}

	if *agentType == "" {
		fmt.Fprintln(os.Stderr, "picoclaw-agent requires --agent flag (worker, coder1, coder2, coder3, audio_router)")
		os.Exit(1)
	}

	// .envファイルを読み込み（~/.picoclaw/.env または configと同ディレクトリの.env）
	homeDir, _ := os.UserHomeDir()

	// stdoutはJSON通信に使うので、ログはstderrとファイルに出力
	logFile, err := os.OpenFile(filepath.Join(homeDir, ".picoclaw", "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
		defer logFile.Close()
	} else {
		log.SetOutput(os.Stderr)
	}
	loadDotEnv(filepath.Join(homeDir, ".picoclaw", ".env"))
	loadDotEnv(filepath.Join(filepath.Dir(*configPath), ".env"))

	// 設定読み込み
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if *agentType == "audio_router" {
		log.Printf("[picoclaw-agent] Starting standalone %s agent", *agentType)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			sig := <-sigCh
			log.Printf("[picoclaw-agent] Received signal: %v, shutting down...", sig)
			cancel()
		}()

		if err := runAudioRouter(ctx, cfg, *configPath, flag.Args()); err != nil && err != context.Canceled {
			log.Fatalf("AudioRouter failed: %v", err)
		}
		log.Println("[picoclaw-agent] Shutdown complete")
		return
	}

	handler, err := initHandler(*agentType, cfg)
	if err != nil {
		log.Fatalf("Failed to initialize handler: %v", err)
	}

	log.Printf("[picoclaw-agent] Starting standalone %s agent", *agentType)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// SIGTERM/SIGINT graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		log.Printf("[picoclaw-agent] Received signal: %v, shutting down...", sig)
		cancel()
	}()

	if err := runMessageLoop(ctx, handler, jsonOut); err != nil {
		log.Printf("[picoclaw-agent] Message loop ended: %v", err)
	}

	log.Println("[picoclaw-agent] Shutdown complete")
}

// initHandler はagentTypeに応じたハンドラを初期化
func initHandler(agentType string, cfg *config.Config) (AgentHandler, error) {
	switch agentType {
	case "worker":
		return initWorkerHandler(cfg)
	case "coder1":
		return initCoderHandler("coder1", cfg)
	case "coder2":
		return initCoderHandler("coder2", cfg)
	case "coder3":
		return initCoderHandler("coder3", cfg)
	case "coder4":
		return initCoderHandler("coder4", cfg)
	default:
		return nil, fmt.Errorf("unknown agent type: %s (supported: worker, coder1, coder2, coder3, coder4, audio_router)", agentType)
	}
}

// initWorkerHandler はWorkerハンドラを初期化
func initWorkerHandler(cfg *config.Config) (*workerHandler, error) {
	model := strings.TrimSpace(cfg.Ollama.WorkerModel)
	if model == "" {
		model = cfg.Ollama.Model
	}
	ollamaProvider := ollama.NewOllamaProviderWithNumCtx(cfg.Ollama.BaseURL, model, 16384)
	toolRunnerCfg := tools.ToolRunnerConfig{
		GoogleAPIKey:         os.Getenv("GOOGLE_API_KEY_WORKER"),
		GoogleSearchEngineID: os.Getenv("GOOGLE_SEARCH_ENGINE_ID_WORKER"),
	}
	toolRunner := tools.NewToolRunner(toolRunnerCfg)
	mcpClient := mcp.NewMCPClient()
	shiroAgent := agent.NewShiroAgent(ollamaProvider, toolRunner, mcpClient, cfg.Prompts.Worker, nil)
	executionService := service.NewWorkerExecutionService(cfg.Worker)

	log.Printf("[picoclaw-agent] Worker initialized (workspace=%s)", cfg.Worker.Workspace)

	return &workerHandler{
		shiroAgent:       shiroAgent,
		executionService: executionService,
	}, nil
}

// initCoderHandler はCoderハンドラを初期化
func initCoderHandler(agentName string, cfg *config.Config) (*coderHandler, error) {
	// v4.1: Unified CoderConfig を使用
	var coderCfg config.CoderConfig
	switch agentName {
	case "coder1":
		coderCfg = cfg.Coder1
	case "coder2":
		coderCfg = cfg.Coder2
	case "coder3":
		coderCfg = cfg.Coder3
	case "coder4":
		coderCfg = cfg.Coder4
	default:
		return nil, fmt.Errorf("unknown coder: %s", agentName)
	}

	if !coderCfg.Enabled {
		return nil, fmt.Errorf("%s is not enabled in config", agentName)
	}

	// CoderConfig から Provider を作成
	provider, err := createProviderFromConfig(coderCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider for %s: %w", agentName, err)
	}

	// CoderAgent 作成
	coderAgent := agent.NewCoderAgent(provider, nil, nil, cfg.Prompts.CoderProposal)

	// Persona 適用
	if coderCfg.Personality != "" {
		persona := agent.AgentPersona{
			Name:        coderCfg.Name,
			Personality: coderCfg.Personality,
			Tone:        coderCfg.Tone,
		}
		coderAgent.WithPersona(persona)
		log.Printf("[picoclaw-agent] %s: Applied Persona '%s'", agentName, coderCfg.Name)
	}

	// LightMemory 適用
	if coderCfg.LightMemory.Enabled {
		memory := agent.NewLightMemory(coderCfg.LightMemory.MaxTurns)
		coderAgent.WithLightMemory(memory)
		log.Printf("[picoclaw-agent] %s: Applied LightMemory (max_turns=%d)", agentName, coderCfg.LightMemory.MaxTurns)
	}

	log.Printf("[picoclaw-agent] %s initialized: provider=%s, model=%s", agentName, coderCfg.Provider, coderCfg.Model)

	return &coderHandler{
		agentName:      agentName,
		coderAgent:     coderAgent,
		proposalPrompt: cfg.Prompts.CoderProposal,
	}, nil
}

// runMessageLoop はstdin/stdout上のJSON通信ループ
func runMessageLoop(ctx context.Context, handler AgentHandler, jsonOut io.Writer) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	encoder := json.NewEncoder(jsonOut)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		var msg domaintransport.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Printf("[picoclaw-agent] Failed to decode message: %v", err)
			// エラーをJSON応答として返す
			errResp := domaintransport.NewErrorMessage("agent", "unknown", "", "", fmt.Sprintf("decode error: %v", err))
			encoder.Encode(errResp)
			continue
		}

		// タイムアウト付きでハンドラ実行
		handlerCtx, handlerCancel := context.WithTimeout(ctx, shutdownTimeout)
		response, err := handler.HandleMessage(handlerCtx, msg)
		handlerCancel()

		if err != nil {
			log.Printf("[picoclaw-agent] Handler error: %v", err)
			errResp := domaintransport.NewErrorMessage(msg.To, msg.From, msg.SessionID, msg.JobID, fmt.Sprintf("handler error: %v", err))
			if encErr := encoder.Encode(errResp); encErr != nil {
				return fmt.Errorf("encode error response: %w", encErr)
			}
			continue
		}

		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("encode response: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin scanner: %w", err)
	}

	return nil // stdin closed (normal termination)
}
