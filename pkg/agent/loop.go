// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/constants"
	"github.com/sipeed/picoclaw/pkg/health"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/mcp"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/session"
	"github.com/sipeed/picoclaw/pkg/state"
	"github.com/sipeed/picoclaw/pkg/tools"
	"github.com/sipeed/picoclaw/pkg/utils"
)

type AgentLoop struct {
	bus            *bus.MessageBus
	cfg            *config.Config
	provider       providers.LLMProvider
	providerName   string
	workspace      string
	model          string
	contextWindow  int // Maximum context window size in tokens
	maxIterations  int
	loopMaxLoops   int
	loopMaxMillis  int
	sessions       *session.SessionManager
	state          *state.Manager
	contextBuilder *ContextBuilder
	tools          *tools.ToolRegistry
	router         *Router
	running        atomic.Bool
	summarizing    sync.Map // Tracks which sessions are currently being summarized
	channelManager *channels.Manager
	mcpClient      *mcp.Client
}

// processOptions configures how a message is processed
type processOptions struct {
	SessionKey         string // Session identifier for history/context
	Channel            string // Target channel for tool execution
	ChatID             string // Target chat ID for tool execution
	UserMessage        string // User message content (may include prefix)
	Media              []string
	DefaultResponse    string // Response when LLM returns empty
	EnableSummary      bool   // Whether to trigger summarization
	SendResponse       bool   // Whether to send response via bus
	NoHistory          bool   // If true, don't load session history (for heartbeat)
	Route              string // Routed category for logging
	LocalOnly          bool   // /local mode for this session
	Declaration        string // Route declaration prefix
	MaxLoops           int    // Max loop iterations for this turn
	MaxMillis          int    // Max processing time for this turn
	SkipAddUserMessage bool   // When true, don't add user message (used on Ollama recovery retry)
}

const DefaultWorkOverlayTurns = 8

const WorkOverlayDirectiveText = `Mioへ（仕事モード）：
- れんさんの意図とゴールを1〜2文で要約
- 要点→手順→確認の順だが、見出し（結論・手順・確認）は出力しない。自然な文で構成
- 推測は推測と明示。不明は不明と言う
- 長文化しない。網羅を避ける
- 追加提案は最大1件
- 実行していない操作を実行済みと言わない
- 機密情報は出さない`

type WorkCmd struct {
	Kind  string // "on" | "off" | "status"
	Turns int
	Ok    bool
}

func parseWorkCommand(text string) WorkCmd {
	t := strings.TrimSpace(text)
	if t == "/normal" {
		return WorkCmd{Kind: "off", Ok: true}
	}
	if !strings.HasPrefix(t, "/work") {
		return WorkCmd{Ok: false}
	}
	parts := strings.Fields(t)
	if len(parts) == 1 {
		return WorkCmd{Kind: "on", Turns: DefaultWorkOverlayTurns, Ok: true}
	}
	arg := strings.ToLower(parts[1])
	if arg == "off" {
		return WorkCmd{Kind: "off", Ok: true}
	}
	if arg == "status" {
		return WorkCmd{Kind: "status", Ok: true}
	}
	if n, err := strconv.Atoi(arg); err == nil && n > 0 && n <= 50 {
		return WorkCmd{Kind: "on", Turns: n, Ok: true}
	}
	return WorkCmd{Kind: "status", Ok: true}
}

type chatDelegateDirective struct {
	Route string
	Task  string
}

// createToolRegistry creates a tool registry with common tools.
// This is shared between main agent and subagents.
func createToolRegistry(workspace string, restrict bool, cfg *config.Config, msgBus *bus.MessageBus) *tools.ToolRegistry {
	registry := tools.NewToolRegistry()

	// File system tools
	registry.Register(tools.NewReadFileTool(workspace, restrict))
	registry.Register(tools.NewWriteFileTool(workspace, restrict))
	registry.Register(tools.NewListDirTool(workspace, restrict))
	registry.Register(tools.NewEditFileTool(workspace, restrict))
	registry.Register(tools.NewAppendFileTool(workspace, restrict))

	// Shell execution
	registry.Register(tools.NewExecTool(workspace, restrict))

	if searchTool := tools.NewWebSearchTool(tools.WebSearchToolOptions{
		BraveAPIKey:          cfg.Tools.Web.Brave.APIKey,
		BraveMaxResults:      cfg.Tools.Web.Brave.MaxResults,
		BraveEnabled:         cfg.Tools.Web.Brave.Enabled,
		DuckDuckGoMaxResults: cfg.Tools.Web.DuckDuckGo.MaxResults,
		DuckDuckGoEnabled:    cfg.Tools.Web.DuckDuckGo.Enabled,
		PerplexityAPIKey:     cfg.Tools.Web.Perplexity.APIKey,
		PerplexityMaxResults: cfg.Tools.Web.Perplexity.MaxResults,
		PerplexityEnabled:    cfg.Tools.Web.Perplexity.Enabled,
	}); searchTool != nil {
		registry.Register(searchTool)
	}
	registry.Register(tools.NewWebFetchTool(50000))

	// Hardware tools (I2C, SPI) - Linux only, returns error on other platforms
	registry.Register(tools.NewI2CTool())
	registry.Register(tools.NewSPITool())

	// Message tool - available to both agent and subagent
	// Subagent uses it to communicate directly with user
	messageTool := tools.NewMessageTool()
	messageTool.SetSendCallback(func(channel, chatID, content string) error {
		msgBus.PublishOutbound(bus.OutboundMessage{
			Channel: channel,
			ChatID:  chatID,
			Content: content,
		})
		return nil
	})
	registry.Register(messageTool)

	return registry
}

func NewAgentLoop(cfg *config.Config, msgBus *bus.MessageBus, provider providers.LLMProvider) *AgentLoop {
	workspace := cfg.WorkspacePath()
	os.MkdirAll(workspace, 0755)

	restrict := cfg.Agents.Defaults.RestrictToWorkspace

	// Create tool registry for main agent
	toolsRegistry := createToolRegistry(workspace, restrict, cfg, msgBus)

	// NOTE:
	// spawn/subagent tools are intentionally disabled in this deployment.
	// Top-priority operating policy requires no subagent usage.

	sessionsManager := session.NewSessionManager(filepath.Join(workspace, "sessions"))

	// Create state manager for atomic state persistence
	stateManager := state.NewManager(workspace)

	// Create context builder and set tools registry
	contextBuilder := NewContextBuilder(workspace)
	contextBuilder.SetToolsRegistry(toolsRegistry)
	contextBuilder.SetChatAlias(cfg.Routing.LLM.ChatAlias)

	// Initialize MCP client if enabled
	var mcpClient *mcp.Client
	if cfg.MCP.Chrome.Enabled {
		mcpClient = mcp.NewClient(cfg.MCP.Chrome.BaseURL)
	}

	return &AgentLoop{
		bus:            msgBus,
		cfg:            cfg,
		provider:       provider,
		providerName:   strings.ToLower(strings.TrimSpace(cfg.Agents.Defaults.Provider)),
		workspace:      workspace,
		model:          cfg.Agents.Defaults.Model,
		contextWindow:  cfg.Agents.Defaults.MaxTokens, // Restore context window for summarization
		maxIterations:  cfg.Agents.Defaults.MaxToolIterations,
		loopMaxLoops:   cfg.Loop.MaxLoops,
		loopMaxMillis:  cfg.Loop.MaxMillis,
		sessions:       sessionsManager,
		state:          stateManager,
		contextBuilder: contextBuilder,
		tools:          toolsRegistry,
		router:         NewRouter(cfg.Routing, NewClassifier(provider, cfg.Agents.Defaults.Model)),
		summarizing:    sync.Map{},
		mcpClient:      mcpClient,
	}
}

func (al *AgentLoop) Run(ctx context.Context) error {
	al.running.Store(true)

	for al.running.Load() {
		select {
		case <-ctx.Done():
			return nil
		default:
			msg, ok := al.bus.ConsumeInbound(ctx)
			if !ok {
				continue
			}

			response, err := al.processMessage(ctx, msg)
			if err != nil {
				response = fmt.Sprintf("Error processing message: %v", err)
			}

			if response != "" {
				// Check if the message tool already sent a response during this round.
				// If so, skip publishing to avoid duplicate messages to the user.
				alreadySent := false
				if tool, ok := al.tools.Get("message"); ok {
					if mt, ok := tool.(*tools.MessageTool); ok {
						alreadySent = mt.HasSentInRound()
					}
				}

				if !alreadySent {
					outbound := bus.OutboundMessage{
						Channel: msg.Channel,
						ChatID:  msg.ChatID,
						Content: response,
					}
					// Special handling: when Worker/Coder finishes, reply to the remembered origin message ID.
					flags := al.sessions.GetFlags(msg.SessionKey)
					if flags.PendingOriginReply && flags.OriginMessageID != "" {
						outbound.Metadata = map[string]string{
							"origin_message_id": flags.OriginMessageID,
							"origin_route":      flags.OriginRoute,
							"reply_mode":        "origin",
						}
						flags.PendingOriginReply = false
						al.sessions.SetFlags(msg.SessionKey, flags)
						_ = al.sessions.Save(msg.SessionKey)
					}
					al.bus.PublishOutbound(outbound)
				}
			}
		}
	}

	return nil
}

func (al *AgentLoop) Stop() {
	al.running.Store(false)
}

func (al *AgentLoop) RegisterTool(tool tools.Tool) {
	al.tools.Register(tool)
}

func (al *AgentLoop) SetChannelManager(cm *channels.Manager) {
	al.channelManager = cm
}

// RecordLastChannel records the last active channel for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChannel(channel string) error {
	return al.state.SetLastChannel(channel)
}

// RecordLastChatID records the last active chat ID for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChatID(chatID string) error {
	return al.state.SetLastChatID(chatID)
}

func (al *AgentLoop) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

func (al *AgentLoop) ProcessDirectWithChannel(ctx context.Context, content, sessionKey, channel, chatID string) (string, error) {
	msg := bus.InboundMessage{
		Channel:    channel,
		SenderID:   "cron",
		ChatID:     chatID,
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessage(ctx, msg)
}

// ProcessHeartbeat processes a heartbeat request without session history.
// Each heartbeat is independent and doesn't accumulate context.
func (al *AgentLoop) ProcessHeartbeat(ctx context.Context, content, channel, chatID string) (string, error) {
	return al.runAgentLoop(ctx, processOptions{
		SessionKey:      "heartbeat",
		Channel:         channel,
		ChatID:          chatID,
		UserMessage:     content,
		DefaultResponse: "I've completed processing but have no response to give.",
		EnableSummary:   false,
		SendResponse:    false,
		NoHistory:       true, // Don't load session history for heartbeat
		Route:           RouteChat,
		MaxLoops:        al.maxIterations,
		MaxMillis:       al.loopMaxMillis,
	})
}

func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Add message preview to log (show full content for error messages)
	var logContent string
	if strings.Contains(msg.Content, "Error:") || strings.Contains(msg.Content, "error") {
		logContent = msg.Content // Full content for errors
	} else {
		logContent = utils.Truncate(msg.Content, 80)
	}
	logger.InfoCF("agent", fmt.Sprintf("Processing message from %s:%s: %s", msg.Channel, msg.SenderID, logContent),
		map[string]interface{}{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
		})

	// Route system messages to processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	// Daily session cutover: archive yesterday's session to daily note and reset.
	al.maybeDailyCutover(msg.SessionKey)

	// Check for commands
	if response, handled := al.handleCommand(ctx, msg); handled {
		return response, nil
	}

	flags := al.sessions.GetFlags(msg.SessionKey)
	decision := al.router.Decide(ctx, msg.Content, flags)
	// LINE channel is chat-only by product rule.
	// Force CHAT route regardless of classifier/rules output.
	if msg.Channel == "line" && strings.ToUpper(strings.TrimSpace(decision.Route)) != RouteChat {
		decision.Route = RouteChat
		decision.Source = "line_forced_chat"
		decision.Confidence = 1.0
		decision.Reason = "line channel is chat-only"
		decision.Evidence = []string{"channel=line"}
		decision.Declaration = ""
		decision.ErrorReason = ""
	}
	logger.InfoCF("agent", "mvp.routing",
		map[string]interface{}{
			"session_key":           msg.SessionKey,
			"initial_route":         decision.Route,
			"source":                decision.Source,
			"classifier_confidence": decision.ClassifierConfidence,
			"error_reason":          decision.ErrorReason,
		})
	flags.LocalOnly = decision.LocalOnly
	// Special handling: remember origin message ID for Worker/Coder completion reply.
	if msg.Channel == "line" {
		originMessageID := strings.TrimSpace(msg.Metadata["message_id"])
		if originMessageID != "" && strings.ToUpper(strings.TrimSpace(decision.Route)) != RouteChat {
			flags.OriginMessageID = originMessageID
			flags.OriginRoute = decision.Route
			flags.PendingOriginReply = true
		}
	}

	// Notify user when Chat agent delegates work to Worker/Coder (or other non-chat routes).
	if strings.ToUpper(strings.TrimSpace(decision.Route)) != RouteChat && !constants.IsInternalChannel(msg.Channel) {
		role, alias := al.resolveRouteRoleAlias(decision.Route)
		display := role
		if alias != "" && !strings.EqualFold(alias, role) {
			display = fmt.Sprintf("%s（%s）", role, alias)
		}
		chatAlias := al.cfg.Routing.LLM.ChatAlias
		if chatAlias == "" {
			chatAlias = "Chat"
		}
		al.bus.PublishOutbound(bus.OutboundMessage{
			Channel: msg.Channel,
			ChatID:  msg.ChatID,
			Content: fmt.Sprintf("%sから%sに作業依頼して進めるね。完了したら報告するよ。", chatAlias, display),
		})
	}
	if decision.DirectResponse != "" {
		al.sessions.SetFlags(msg.SessionKey, flags)
		al.sessions.Save(msg.SessionKey)
		return decision.DirectResponse, nil
	}

	// Process as user message
	userMessage := decision.CleanUserText
	if userMessage == "" {
		userMessage = msg.Content
	}
	restoreRouteLLM, err := al.applyRouteLLM(decision.Route)
	if err != nil {
		return "", fmt.Errorf("failed to switch LLM for route %s: %w", decision.Route, err)
	}
	defer restoreRouteLLM()

	// Keep chat conversations naturally continuous.
	// CHAT route should prefer full per-user history over aggressive auto-summarization.
	enableSummary := true
	if strings.EqualFold(strings.TrimSpace(decision.Route), RouteChat) {
		enableSummary = false
	}

	opts := processOptions{
		SessionKey:      msg.SessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     userMessage,
		Media:           msg.Media,
		DefaultResponse: "I've completed processing but have no response to give.",
		EnableSummary:   enableSummary,
		SendResponse:    false,
		Route:           decision.Route,
		LocalOnly:       decision.LocalOnly,
		Declaration:     decision.Declaration,
		MaxLoops:        al.loopMaxLoops,
		MaxMillis:       al.loopMaxMillis,
	}
	response, err := al.runAgentLoop(ctx, opts)

	// LINE指示ごと: ヘルスチェック（失敗してなくても実行）→Ollama落ち/モデル未ロードなら再起動→リトライ
	if al.routeUsesOllama(decision.Route) && al.cfg.Providers.Ollama.APIBase != "" {
		checkURL := strings.TrimSuffix(al.cfg.Providers.Ollama.APIBase, "/v1")
		ollamaOK, ollamaMsg := health.OllamaCheck(checkURL, 5*time.Second)()
		required := al.buildOllamaRequiredModels()
		modelsOK, modelsMsg := true, ""
		if len(required) > 0 {
			modelsOK, modelsMsg = health.OllamaModelsCheck(checkURL, 5*time.Second, required)()
		}
		logger.InfoCF("agent", "ollama health check",
			map[string]interface{}{
				"ollama_ok": ollamaOK, "ollama_msg": ollamaMsg,
				"models_ok": modelsOK, "models_msg": modelsMsg,
				"llm_err": err != nil,
			})
		needsRestart := !ollamaOK || !modelsOK
		if needsRestart && strings.TrimSpace(al.cfg.Providers.OllamaRestartCommand) != "" {
			cmd := exec.CommandContext(ctx, "sh", "-c", al.cfg.Providers.OllamaRestartCommand)
			if runErr := cmd.Run(); runErr != nil {
				logger.WarnCF("agent", "ollama restart failed", map[string]interface{}{"error": runErr.Error()})
			} else if err != nil {
				time.Sleep(10 * time.Second)
				opts.SkipAddUserMessage = true
				response, err = al.runAgentLoop(ctx, opts)
			}
		}
	}

	// CODE3 の出力処理：plan/patch を解析して承認要求を生成
	if err == nil && strings.EqualFold(strings.TrimSpace(decision.Route), RouteCode3) {
		coderOutput, parseErr := parseCoder3Output(response)
		if parseErr != nil {
			logger.WarnCF("agent", "coder3.parse_error", map[string]interface{}{
				"error": parseErr.Error(),
			})
			response = fmt.Sprintf("Coder3 の出力解析に失敗しました: %v", parseErr)
		} else {
			// Worker による即時実行
			logger.InfoCF("worker", "worker.patch_execution_start", map[string]interface{}{
				"job_id":      coderOutput.JobID,
				"patch_type":  detectPatchType(coderOutput.Patch),
				"session_key": msg.SessionKey,
			})

			result, execErr := al.executeWorkerPatch(ctx, coderOutput.Patch, msg.SessionKey)
			if execErr != nil {
				response = fmt.Sprintf("patch の実行に失敗したよ😢\n\nエラー: %v\n\nPlan:\n%s", execErr, coderOutput.Plan)
				logger.ErrorCF("worker", "worker.patch_execution_error", map[string]interface{}{
					"job_id": coderOutput.JobID,
					"error":  execErr.Error(),
				})
			} else {
				response = fmt.Sprintf("実行完了！✨\n\n%s\n\n詳細:\n%s\n\nPlan:\n%s",
					result.Summary,
					formatExecutionResults(result.Results),
					coderOutput.Plan)

				if result.GitCommit != "" {
					response += fmt.Sprintf("\n\nGit コミット: %s", result.GitCommit)
				}

				logger.InfoCF("worker", "worker.patch_execution_complete", map[string]interface{}{
					"job_id":        coderOutput.JobID,
					"success":       result.Success,
					"executed_cmds": result.ExecutedCmds,
					"failed_cmds":   result.FailedCmds,
				})
			}
		}
	}

	if err == nil && strings.EqualFold(strings.TrimSpace(decision.Route), RouteChat) {
		if directive, ok := parseChatDelegateDirective(response); ok {
			if !constants.IsInternalChannel(msg.Channel) {
				role, alias := al.resolveRouteRoleAlias(directive.Route)
				display := role
				if alias != "" && !strings.EqualFold(alias, role) {
					display = fmt.Sprintf("%s（%s）", role, alias)
				}
				taskLabel := summarizeDelegationTask(directive.Task)
				al.bus.PublishOutbound(bus.OutboundMessage{
					Channel: msg.Channel,
					ChatID:  msg.ChatID,
					Content: buildDelegationStartNotice(msg.SessionKey, display, taskLabel),
				})
			}

			delegateResult, delegateErr := al.executeChatDelegation(ctx, msg, directive, decision.LocalOnly)
			if delegateErr != nil {
				err = delegateErr
			} else {
				finalResponse, finalizeErr := al.finalizeDelegationWithChat(ctx, msg, directive, delegateResult)
				if finalizeErr != nil {
					err = finalizeErr
				} else {
					if !constants.IsInternalChannel(msg.Channel) {
						role, alias := al.resolveRouteRoleAlias(directive.Route)
						display := role
						if alias != "" && !strings.EqualFold(alias, role) {
							display = fmt.Sprintf("%s（%s）", role, alias)
						}
						taskLabel := summarizeDelegationTask(directive.Task)
						response = buildDelegationDoneNotice(msg.SessionKey, display, taskLabel) + "\n" + finalResponse
					} else {
						response = finalResponse
					}
				}
			}
		}
	}
	if err != nil {
		stopReason := "error"
		if errors.Is(err, context.DeadlineExceeded) {
			stopReason = "timeout"
			response = "ここまでで一度停止したよ。続けるために必要な情報を追記してね。"
			err = nil
		}
		logger.WarnCF("agent", "mvp.stop",
			map[string]interface{}{
				"session_key":  msg.SessionKey,
				"final_route":  decision.Route,
				"stop_reason":  stopReason,
				"error_reason": decision.ErrorReason,
			})
	}
	flags.PrevPrimaryRoute = decision.Route
	al.sessions.SetFlags(msg.SessionKey, flags)
	al.sessions.Save(msg.SessionKey)
	logger.InfoCF("agent", "mvp.route.final",
		map[string]interface{}{
			"session_key":           msg.SessionKey,
			"final_route":           decision.Route,
			"classifier_confidence": decision.ClassifierConfidence,
			"error_reason":          decision.ErrorReason,
		})
	return response, err
}

func parseChatDelegateDirective(content string) (chatDelegateDirective, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return chatDelegateDirective{}, false
	}
	lines := strings.Split(trimmed, "\n")

	firstIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			firstIdx = i
			break
		}
	}
	if firstIdx < 0 {
		return chatDelegateDirective{}, false
	}

	firstLine := strings.TrimSpace(lines[firstIdx])
	upperFirst := strings.ToUpper(firstLine)
	if !strings.HasPrefix(upperFirst, "DELEGATE:") {
		return chatDelegateDirective{}, false
	}
	routeRaw := strings.TrimSpace(firstLine[len("DELEGATE:"):])
	route := strings.ToUpper(routeRaw)
	switch route {
	case RoutePlan, RouteAnalyze, RouteOps, RouteResearch, RouteCode, RouteCode1, RouteCode2, RouteCode3:
	default:
		return chatDelegateDirective{}, false
	}

	taskStart := -1
	taskLineValue := ""
	for i := firstIdx + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(strings.ToUpper(line), "TASK:") {
			taskStart = i
			taskLineValue = strings.TrimSpace(line[len("TASK:"):])
			break
		}
	}
	if taskStart < 0 {
		return chatDelegateDirective{}, false
	}

	task := taskLineValue
	if taskStart+1 < len(lines) {
		rest := strings.TrimSpace(strings.Join(lines[taskStart+1:], "\n"))
		if rest != "" {
			if task != "" {
				task += "\n"
			}
			task += rest
		}
	}
	task = strings.TrimSpace(task)
	if task == "" {
		return chatDelegateDirective{}, false
	}

	return chatDelegateDirective{
		Route: route,
		Task:  task,
	}, true
}

func summarizeDelegationTask(task string) string {
	trimmed := strings.TrimSpace(task)
	if trimmed == "" {
		return "この件"
	}
	lines := strings.Split(trimmed, "\n")
	first := strings.TrimSpace(lines[0])
	if first == "" {
		return "この件"
	}
	const maxRunes = 32
	r := []rune(first)
	if len(r) > maxRunes {
		first = string(r[:maxRunes]) + "..."
	}
	return first
}

func buildDelegationStartNotice(seedKey, workerDisplay, taskLabel string) string {
	templates := []string{
		"%sの作業を%sにお願いするね。進んだらすぐ共有するよ。",
		"%sについては%sにお願いするね。まとまり次第、私から伝えるよ。",
		"%sの作業、%sにお願いするね。終わったらすぐ報告するよ。",
	}
	return chooseDelegationTemplate(seedKey+"|start|"+workerDisplay+"|"+taskLabel, templates, taskLabel, workerDisplay)
}

func buildDelegationDoneNotice(seedKey, workerDisplay, taskLabel string) string {
	templates := []string{
		"%sが%sの作業終わったって。内容をまとめて返すね。",
		"%sから%sの作業終わったって報告きたよ。結果を共有するね。",
		"%sが%sの作業終わったって。ここから私が最終整理して伝えるね。",
	}
	return chooseDelegationTemplate(seedKey+"|done|"+workerDisplay+"|"+taskLabel, templates, workerDisplay, taskLabel)
}

func chooseDelegationTemplate(seed string, templates []string, args ...interface{}) string {
	if len(templates) == 0 {
		return ""
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	idx := int(h.Sum32() % uint32(len(templates)))
	return fmt.Sprintf(templates[idx], args...)
}

func (al *AgentLoop) executeChatDelegation(ctx context.Context, msg bus.InboundMessage, directive chatDelegateDirective, localOnly bool) (string, error) {
	restoreRouteLLM, err := al.applyRouteLLMWithTask(directive.Route, directive.Task)
	if err != nil {
		return "", fmt.Errorf("failed to switch LLM for delegated route %s: %w", directive.Route, err)
	}
	defer restoreRouteLLM()

	delegateSessionKey := fmt.Sprintf("%s:delegate:%d", msg.SessionKey, time.Now().UnixNano())
	return al.runAgentLoop(ctx, processOptions{
		SessionKey:      delegateSessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     directive.Task,
		DefaultResponse: "作業は完了したけど、結果を返せなかったよ。",
		EnableSummary:   true,
		SendResponse:    false,
		NoHistory:       true,
		Route:           directive.Route,
		LocalOnly:       localOnly,
		MaxLoops:        al.loopMaxLoops,
		MaxMillis:       al.loopMaxMillis,
	})
}

func (al *AgentLoop) finalizeDelegationWithChat(ctx context.Context, msg bus.InboundMessage, directive chatDelegateDirective, delegateResult string) (string, error) {
	restoreRouteLLM, err := al.applyRouteLLM(RouteChat)
	if err != nil {
		return "", fmt.Errorf("failed to switch back to chat LLM: %w", err)
	}
	defer restoreRouteLLM()

	finalPrompt := fmt.Sprintf(
		"[DELEGATION_RESULT]\nRoute: %s\nTask:\n%s\n\nResult:\n%s\n\nこの結果を踏まえてユーザー向け最終回答を返して。ここでは DELEGATE 形式を絶対に出力せず、最終回答のみ返すこと。",
		directive.Route,
		directive.Task,
		delegateResult,
	)

	finalResponse, err := al.runAgentLoop(ctx, processOptions{
		SessionKey:      msg.SessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     finalPrompt,
		DefaultResponse: delegateResult,
		EnableSummary:   false,
		SendResponse:    false,
		Route:           RouteChat,
		MaxLoops:        al.loopMaxLoops,
		MaxMillis:       al.loopMaxMillis,
	})
	if err != nil {
		return "", err
	}
	if _, delegatedAgain := parseChatDelegateDirective(finalResponse); delegatedAgain {
		return delegateResult, nil
	}
	return finalResponse, nil
}

func (al *AgentLoop) applyRouteLLM(route string) (func(), error) {
	return al.applyRouteLLMWithTask(route, "")
}

func (al *AgentLoop) applyRouteLLMWithTask(route, taskText string) (func(), error) {
	actualRoute := route
	if route == RouteCode && taskText != "" {
		actualRoute = selectCoderRoute(taskText)
	}
	role, alias := al.resolveRouteRoleAlias(actualRoute)
	targetProvider, targetModel := al.resolveRouteLLMWithTask(actualRoute, taskText)
	if targetModel == "" {
		targetModel = al.model
	}
	if targetProvider == al.providerName && targetModel == al.model {
		return func() {}, nil
	}

	cfg := config.DefaultConfig()
	cfg.Providers = al.cfg.Providers
	cfg.Agents.Defaults.Workspace = al.cfg.Agents.Defaults.Workspace
	cfg.Agents.Defaults.RestrictToWorkspace = al.cfg.Agents.Defaults.RestrictToWorkspace
	cfg.Agents.Defaults.Provider = targetProvider
	cfg.Agents.Defaults.Model = targetModel

	routeProvider, err := providers.CreateProvider(cfg)
	if err != nil {
		return nil, err
	}

	prevProvider := al.provider
	prevProviderName := al.providerName
	prevModel := al.model

	al.provider = routeProvider
	al.providerName = targetProvider
	al.model = targetModel

	logger.InfoCF("agent", "route.llm.selected", map[string]interface{}{
		"route":    route,
		"role":     role,
		"alias":    alias,
		"provider": targetProvider,
		"model":    targetModel,
	})

	return func() {
		al.provider = prevProvider
		al.providerName = prevProviderName
		al.model = prevModel
	}, nil
}

func (al *AgentLoop) resolveRouteLLM(route string) (string, string) {
	return al.resolveRouteLLMWithTask(route, "")
}

func (al *AgentLoop) resolveRouteLLMWithTask(route, taskText string) (string, string) {
	defaultProvider := strings.ToLower(strings.TrimSpace(al.cfg.Agents.Defaults.Provider))
	defaultModel := strings.TrimSpace(al.cfg.Agents.Defaults.Model)
	llmCfg := al.cfg.Routing.LLM

	chooseProvider := func(base, override string) string {
		if trimmed := strings.ToLower(strings.TrimSpace(override)); trimmed != "" {
			return trimmed
		}
		return strings.ToLower(strings.TrimSpace(base))
	}
	chooseModel := func(base, override string) string {
		if trimmed := strings.TrimSpace(override); trimmed != "" {
			return trimmed
		}
		return strings.TrimSpace(base)
	}

	resolveCoder1 := func() (string, string) {
		p := chooseProvider(defaultProvider, llmCfg.CoderProvider)
		m := chooseModel(defaultModel, llmCfg.CoderModel)
		if strings.TrimSpace(llmCfg.CoderProvider) == "" {
			p = chooseProvider(p, llmCfg.CodeProvider)
		}
		if strings.TrimSpace(llmCfg.CoderModel) == "" {
			m = chooseModel(m, llmCfg.CodeModel)
		}
		return p, m
	}

	resolveCoder2 := func() (string, string) {
		if strings.TrimSpace(llmCfg.Coder2Provider) == "" && strings.TrimSpace(llmCfg.Coder2Model) == "" {
			return resolveCoder1()
		}
		return chooseProvider(defaultProvider, llmCfg.Coder2Provider), chooseModel(defaultModel, llmCfg.Coder2Model)
	}

	resolveCoder3 := func() (string, string) {
		if strings.TrimSpace(llmCfg.Coder3Provider) == "" && strings.TrimSpace(llmCfg.Coder3Model) == "" {
			return resolveCoder2()
		}
		return chooseProvider(defaultProvider, llmCfg.Coder3Provider), chooseModel(defaultModel, llmCfg.Coder3Model)
	}

	switch strings.ToUpper(strings.TrimSpace(route)) {
	case RouteCode1:
		return resolveCoder1()
	case RouteCode2:
		return resolveCoder2()
	case RouteCode3:
		return resolveCoder3()
	case RouteCode:
		selected := selectCoderRoute(taskText)
		if selected == RouteCode1 {
			return resolveCoder1()
		}
		if selected == RouteCode3 {
			return resolveCoder3()
		}
		return resolveCoder2()
	case RouteChat:
		return chooseProvider(defaultProvider, llmCfg.ChatProvider), chooseModel(defaultModel, llmCfg.ChatModel)
	default:
		workerProvider := chooseProvider(defaultProvider, llmCfg.WorkerProvider)
		workerModel := chooseModel(defaultModel, llmCfg.WorkerModel)
		if strings.TrimSpace(llmCfg.WorkerProvider) == "" {
			workerProvider = chooseProvider(workerProvider, llmCfg.ChatProvider)
		}
		if strings.TrimSpace(llmCfg.WorkerModel) == "" {
			workerModel = chooseModel(workerModel, llmCfg.ChatModel)
		}
		return workerProvider, workerModel
	}
}

func selectCoderRoute(taskText string) string {
	if taskText == "" {
		return RouteCode2
	}
	lower := strings.ToLower(taskText)

	// CODE3: 高品質コーディング/推論向け
	code3Keywords := []string{
		"高品質", "仕様策定", "複雑な推論", "重大バグ", "失敗コスト",
		"クリティカル", "本番環境", "production",
		"high quality", "critical", "complex reasoning",
	}
	for _, kw := range code3Keywords {
		if strings.Contains(lower, kw) {
			return RouteCode3
		}
	}

	// CODE1: 仕様設計向け
	code1Keywords := []string{
		"仕様", "設計", "文書", "論点", "意思決定",
		"要件定義", "アーキテクチャ", "構成案", "比較検討",
		"spec", "design", "architecture", "requirements", "rfc",
		"proposal", "decision",
	}
	for _, kw := range code1Keywords {
		if strings.Contains(lower, kw) {
			return RouteCode1
		}
	}

	// CODE2: デフォルト
	return RouteCode2
}

func (al *AgentLoop) routeUsesOllama(route string) bool {
	provider, _ := al.resolveRouteLLM(route)
	return strings.EqualFold(strings.TrimSpace(provider), "ollama")
}

func (al *AgentLoop) buildOllamaRequiredModels() []health.ModelRequirement {
	var required []health.ModelRequirement
	type pair struct {
		provider, model string
	}
	llm := al.cfg.Routing.LLM
	for _, p := range []pair{
		{llm.ChatProvider, llm.ChatModel},
		{llm.WorkerProvider, llm.WorkerModel},
		{llm.CoderProvider, llm.CoderModel},
		{llm.Coder2Provider, llm.Coder2Model},
	} {
		if !strings.EqualFold(strings.TrimSpace(p.provider), "ollama") || p.model == "" {
			continue
		}
		name := p.model
		if idx := strings.Index(name, "/"); idx != -1 {
			name = name[idx+1:]
		}
		required = append(required, health.ModelRequirement{Name: name, MaxContext: 8192})
	}
	return required
}

func (al *AgentLoop) resolveRouteRoleAlias(route string) (string, string) {
	llmCfg := al.cfg.Routing.LLM
	switch strings.ToUpper(strings.TrimSpace(route)) {
	case RouteCode, RouteCode1:
		alias := strings.TrimSpace(llmCfg.CoderAlias)
		if alias == "" {
			alias = "Coder"
		}
		return "Coder", alias
	case RouteCode2:
		alias := strings.TrimSpace(llmCfg.Coder2Alias)
		if alias == "" {
			alias = strings.TrimSpace(llmCfg.CoderAlias)
		}
		if alias == "" {
			alias = "Coder2"
		}
		return "Coder2", alias
	case RouteCode3:
		alias := strings.TrimSpace(llmCfg.Coder3Alias)
		if alias == "" {
			alias = "Coder3"
		}
		return "Coder3", alias
	case RouteChat:
		alias := strings.TrimSpace(llmCfg.ChatAlias)
		if alias == "" {
			alias = "Chat"
		}
		return "Chat", alias
	default:
		alias := strings.TrimSpace(llmCfg.WorkerAlias)
		if alias == "" {
			alias = "Worker"
		}
		return "Worker", alias
	}
}

func (al *AgentLoop) processSystemMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Verify this is a system message
	if msg.Channel != "system" {
		return "", fmt.Errorf("processSystemMessage called with non-system message channel: %s", msg.Channel)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]interface{}{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// Parse origin channel from chat_id (format: "channel:chat_id")
	var originChannel string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
	} else {
		// Fallback
		originChannel = "cli"
	}

	// Extract subagent result from message content
	// Format: "Task 'label' completed.\n\nResult:\n<actual content>"
	content := msg.Content
	if idx := strings.Index(content, "Result:\n"); idx >= 0 {
		content = content[idx+8:] // Extract just the result part
	}

	// Skip internal channels - only log, don't send to user
	if constants.IsInternalChannel(originChannel) {
		logger.InfoCF("agent", "Subagent completed (internal channel)",
			map[string]interface{}{
				"sender_id":   msg.SenderID,
				"content_len": len(content),
				"channel":     originChannel,
			})
		return "", nil
	}

	// Agent acts as dispatcher only - subagent handles user interaction via message tool
	// Don't forward result here, subagent should use message tool to communicate with user
	logger.InfoCF("agent", "Subagent completed",
		map[string]interface{}{
			"sender_id":   msg.SenderID,
			"channel":     originChannel,
			"content_len": len(content),
		})

	// Agent only logs, does not respond to user
	return "", nil
}

// runAgentLoop is the core message processing logic.
// It handles context building, LLM calls, tool execution, and response handling.
func (al *AgentLoop) runAgentLoop(ctx context.Context, opts processOptions) (string, error) {
	loopCtx := ctx
	cancel := func() {}
	if opts.MaxMillis > 0 {
		loopCtx, cancel = context.WithTimeout(ctx, time.Duration(opts.MaxMillis)*time.Millisecond)
	}
	defer cancel()

	// 0. Record last channel for heartbeat notifications (skip internal channels)
	if opts.Channel != "" && opts.ChatID != "" {
		// Don't record internal channels (cli, system, subagent)
		if !constants.IsInternalChannel(opts.Channel) {
			channelKey := fmt.Sprintf("%s:%s", opts.Channel, opts.ChatID)
			if err := al.RecordLastChannel(channelKey); err != nil {
				logger.WarnCF("agent", "Failed to record last channel: %v", map[string]interface{}{"error": err.Error()})
			}
		}
	}

	// 1. Update tool contexts
	al.updateToolContexts(opts.Channel, opts.ChatID)

	// 2. Build messages (skip history for heartbeat)
	var history []providers.Message
	var summary string
	if !opts.NoHistory {
		history = al.sessions.GetHistory(opts.SessionKey)
		summary = al.sessions.GetSummary(opts.SessionKey)
	}
	workOverlay := ""
	if !opts.NoHistory {
		overlayFlags := al.sessions.GetFlags(opts.SessionKey)
		if overlayFlags.WorkOverlayTurnsLeft > 0 {
			workOverlay = overlayFlags.WorkOverlayDirective
		}
	}
	messages := al.contextBuilder.BuildMessages(
		history,
		summary,
		opts.UserMessage,
		opts.Media,
		opts.Channel,
		opts.ChatID,
		opts.Route,
		workOverlay,
	)

	// 3. Save user message to session (skip on retry - already added)
	if !opts.SkipAddUserMessage {
		al.sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)
	}

	// 4. Run LLM iteration loop
	finalContent, iteration, err := al.runLLMIteration(loopCtx, messages, opts)
	if err != nil {
		return "", err
	}

	// If last tool had ForUser content and we already sent it, we might not need to send final response
	// This is controlled by the tool's Silent flag and ForUser content

	// 5. Handle empty response
	if finalContent == "" {
		finalContent = opts.DefaultResponse
	}
	if opts.Declaration != "" && finalContent != "" {
		finalContent = opts.Declaration + "\n" + finalContent
	}

	// 6. Work Overlay ターン消費（LLM成功時のみ）
	if strings.EqualFold(strings.TrimSpace(opts.Route), RouteChat) {
		currentFlags := al.sessions.GetFlags(opts.SessionKey)
		if currentFlags.WorkOverlayTurnsLeft > 0 {
			currentFlags.WorkOverlayTurnsLeft--
			if currentFlags.WorkOverlayTurnsLeft <= 0 {
				currentFlags.WorkOverlayTurnsLeft = 0
				currentFlags.WorkOverlayDirective = ""
			}
			al.sessions.SetFlags(opts.SessionKey, currentFlags)
		}
	}

	// 7. Save final assistant message to session
	al.sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
	al.sessions.Save(opts.SessionKey)

	// 7. Optional: summarization
	if opts.EnableSummary {
		al.maybeSummarize(opts.SessionKey, opts.Channel, opts.ChatID)
	}

	// 8. Optional: send response via bus
	if opts.SendResponse {
		al.bus.PublishOutbound(bus.OutboundMessage{
			Channel: opts.Channel,
			ChatID:  opts.ChatID,
			Content: finalContent,
		})
	}

	// 9. Log response
	responsePreview := utils.Truncate(finalContent, 120)
	logger.InfoCF("agent", fmt.Sprintf("Response: %s", responsePreview),
		map[string]interface{}{
			"session_key":  opts.SessionKey,
			"iterations":   iteration,
			"final_length": len(finalContent),
		})

	return finalContent, nil
}

// runLLMIteration executes the LLM call loop with tool handling.
// Returns the final content, iteration count, and any error.
func (al *AgentLoop) runLLMIteration(ctx context.Context, messages []providers.Message, opts processOptions) (string, int, error) {
	iteration := 0
	var finalContent string
	limit := al.maxIterations
	if opts.MaxLoops > 0 {
		limit = opts.MaxLoops
	}

	for iteration < limit {
		iteration++

		logger.DebugCF("agent", "LLM iteration",
			map[string]interface{}{
				"iteration": iteration,
				"max":       limit,
			})

		// Chat agent is a conversation-only gateway, so CHAT route runs without tool calls.
		providerToolDefs := []providers.ToolDefinition{}
		if strings.ToUpper(strings.TrimSpace(opts.Route)) != RouteChat {
			providerToolDefs = al.tools.ToProviderDefs()
		}

		// Log LLM request details
		logger.DebugCF("agent", "LLM request",
			map[string]interface{}{
				"iteration":         iteration,
				"model":             al.model,
				"messages_count":    len(messages),
				"tools_count":       len(providerToolDefs),
				"max_tokens":        8192,
				"temperature":       0.7,
				"system_prompt_len": len(messages[0].Content),
			})

		// Log full messages (detailed)
		logger.DebugCF("agent", "Full LLM request",
			map[string]interface{}{
				"iteration":     iteration,
				"messages_json": formatMessagesForLog(messages),
				"tools_json":    formatToolsForLog(providerToolDefs),
			})

		var response *providers.LLMResponse
		var err error

		// Retry loop for context/token errors
		maxRetries := 2
		for retry := 0; retry <= maxRetries; retry++ {
			response, err = al.provider.Chat(ctx, messages, providerToolDefs, al.model, map[string]interface{}{
				"max_tokens":  8192,
				"temperature": 0.7,
			})

			if err == nil {
				break // Success
			}

			errMsg := strings.ToLower(err.Error())
			// Check for context window errors (provider specific, but usually contain "token" or "invalid")
			isContextError := strings.Contains(errMsg, "token") ||
				strings.Contains(errMsg, "context") ||
				strings.Contains(errMsg, "invalidparameter") ||
				strings.Contains(errMsg, "length")

			if isContextError && retry < maxRetries {
				logger.WarnCF("agent", "Context window error detected, attempting compression", map[string]interface{}{
					"error": err.Error(),
					"retry": retry,
				})

				// Notify user on first retry only
				if retry == 0 && !constants.IsInternalChannel(opts.Channel) && opts.SendResponse {
					al.bus.PublishOutbound(bus.OutboundMessage{
						Channel: opts.Channel,
						ChatID:  opts.ChatID,
						Content: "⚠️ Context window exceeded. Compressing history and retrying...",
					})
				}

				// Force compression
				al.forceCompression(opts.SessionKey)

				// Rebuild messages with compressed history
				// Note: We need to reload history from session manager because forceCompression changed it
				newHistory := al.sessions.GetHistory(opts.SessionKey)
				newSummary := al.sessions.GetSummary(opts.SessionKey)

				// Re-create messages for the next attempt
				// We keep the current user message (opts.UserMessage) effectively
				messages = al.contextBuilder.BuildMessages(
					newHistory,
					newSummary,
					opts.UserMessage,
					nil,
					opts.Channel,
					opts.ChatID,
					opts.Route,
					"",
				)

				// Important: If we are in the middle of a tool loop (iteration > 1),
				// rebuilding messages from session history might duplicate the flow or miss context
				// if intermediate steps weren't saved correctly.
				// However, al.sessions.AddFullMessage is called after every tool execution,
				// so GetHistory should reflect the current state including partial tool execution.
				// But we need to ensure we don't duplicate the user message which is appended in BuildMessages.
				// BuildMessages(history...) takes the stored history and appends the *current* user message.
				// If iteration > 1, the "current user message" was already added to history in step 3 of runAgentLoop.
				// So if we pass opts.UserMessage again, we might duplicate it?
				// Actually, step 3 is: al.sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)
				// So GetHistory ALREADY contains the user message!

				// CORRECTION:
				// BuildMessages combines: [System] + [History] + [CurrentMessage]
				// But Step 3 added CurrentMessage to History.
				// So if we use GetHistory now, it has the user message.
				// If we pass opts.UserMessage to BuildMessages, it adds it AGAIN.

				// For retry in the middle of a loop, we should rely on what's in the session.
				// BUT checking BuildMessages implementation:
				// It appends history... then appends currentMessage.

				// Logic fix for retry:
				// If iteration == 1, opts.UserMessage corresponds to the user input.
				// If iteration > 1, we are processing tool results. The "messages" passed to Chat
				// already accumulated tool outputs.
				// Rebuilding from session history is safest because it persists state.
				// Start fresh with rebuilt history.

				// Special case: standard BuildMessages appends "currentMessage".
				// If we are strictly retrying the *LLM call*, we want the exact same state as before but compressed.
				// However, the "messages" argument passed to runLLMIteration is constructed by the caller.
				// If we rebuild from Session, we need to know if "currentMessage" should be appended or is already in history.

				// In runAgentLoop:
				// 3. sessions.AddMessage(userMsg)
				// 4. runLLMIteration(..., UserMessage)

				// So History contains the user message.
				// BuildMessages typically appends the user message as a *new* pending message.
				// Wait, standard BuildMessages usage in runAgentLoop:
				// messages := BuildMessages(history (has old), UserMessage)
				// THEN AddMessage(UserMessage).
				// So "history" passed to BuildMessages does NOT contain the current UserMessage yet.

				// But here, inside the loop, we have already saved it.
				// So GetHistory() includes the current user message.
				// If we call BuildMessages(GetHistory(), UserMessage), we get duplicates.

				// Hack/Fix:
				// If we are retrying, we rebuild from Session History ONLY.
				// We pass empty string as "currentMessage" to BuildMessages
				// because the "current message" is already saved in history (step 3).

				messages = al.contextBuilder.BuildMessages(
					newHistory,
					newSummary,
					"", // Empty because history already contains the relevant messages
					nil,
					opts.Channel,
					opts.ChatID,
					opts.Route,
					"",
				)

				continue
			}

			// Real error or success, break loop
			break
		}

		if err != nil {
			isTimeout := errors.Is(err, context.DeadlineExceeded) ||
				strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded") ||
				strings.Contains(strings.ToLower(err.Error()), "timeout")
			logger.ErrorCF("agent", "LLM call failed",
				map[string]interface{}{
					"iteration":  iteration,
					"error":      err.Error(),
					"is_timeout": isTimeout,
					"note":       "If is_timeout: PicoClaw gave up before Ollama responded; Ollama may have responded",
				})
			return "", iteration, fmt.Errorf("LLM call failed after retries: %w", err)
		}

		// Check if no tool calls - we're done
		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			logger.InfoCF("agent", "LLM response without tool calls (direct answer)",
				map[string]interface{}{
					"iteration":     iteration,
					"content_chars": len(finalContent),
				})
			break
		}

		// Log tool calls
		toolNames := make([]string, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.InfoCF("agent", "LLM requested tool calls",
			map[string]interface{}{
				"tools":     toolNames,
				"count":     len(response.ToolCalls),
				"iteration": iteration,
			})

		// Build assistant message with tool calls
		assistantMsg := providers.Message{
			Role:    "assistant",
			Content: response.Content,
		}
		for _, tc := range response.ToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: &providers.FunctionCall{
					Name:      tc.Name,
					Arguments: string(argumentsJSON),
				},
			})
		}
		messages = append(messages, assistantMsg)

		// Save assistant message with tool calls to session
		al.sessions.AddFullMessage(opts.SessionKey, assistantMsg)

		// Execute tool calls
		for _, tc := range response.ToolCalls {
			// Log tool call with arguments preview
			argsJSON, _ := json.Marshal(tc.Arguments)
			argsPreview := utils.Truncate(string(argsJSON), 200)
			logger.InfoCF("agent", fmt.Sprintf("Tool call: %s(%s)", tc.Name, argsPreview),
				map[string]interface{}{
					"tool":      tc.Name,
					"iteration": iteration,
				})

			// Create async callback for tools that implement AsyncTool
			// NOTE: Following openclaw's design, async tools do NOT send results directly to users.
			// Instead, they notify the agent via PublishInbound, and the agent decides
			// whether to forward the result to the user (in processSystemMessage).
			asyncCallback := func(callbackCtx context.Context, result *tools.ToolResult) {
				// Log the async completion but don't send directly to user
				// The agent will handle user notification via processSystemMessage
				if !result.Silent && result.ForUser != "" {
					logger.InfoCF("agent", "Async tool completed, agent will handle notification",
						map[string]interface{}{
							"tool":        tc.Name,
							"content_len": len(result.ForUser),
						})
				}
			}

			toolResult := al.tools.ExecuteWithContext(ctx, tc.Name, tc.Arguments, opts.Channel, opts.ChatID, asyncCallback)

			// Send ForUser content to user immediately if not Silent
			if !toolResult.Silent && toolResult.ForUser != "" && opts.SendResponse {
				al.bus.PublishOutbound(bus.OutboundMessage{
					Channel: opts.Channel,
					ChatID:  opts.ChatID,
					Content: toolResult.ForUser,
				})
				logger.DebugCF("agent", "Sent tool result to user",
					map[string]interface{}{
						"tool":        tc.Name,
						"content_len": len(toolResult.ForUser),
					})
			}

			// Determine content for LLM based on tool result
			contentForLLM := toolResult.ForLLM
			if contentForLLM == "" && toolResult.Err != nil {
				contentForLLM = toolResult.Err.Error()
			}

			toolResultMsg := providers.Message{
				Role:       "tool",
				Content:    contentForLLM,
				ToolCallID: tc.ID,
			}
			messages = append(messages, toolResultMsg)

			// Save tool result message to session
			al.sessions.AddFullMessage(opts.SessionKey, toolResultMsg)
		}
	}

	return finalContent, iteration, nil
}

// updateToolContexts updates the context for tools that need channel/chatID info.
func (al *AgentLoop) updateToolContexts(channel, chatID string) {
	// Use ContextualTool interface instead of type assertions
	if tool, ok := al.tools.Get("message"); ok {
		if mt, ok := tool.(tools.ContextualTool); ok {
			mt.SetContext(channel, chatID)
		}
	}
	if tool, ok := al.tools.Get("spawn"); ok {
		if st, ok := tool.(tools.ContextualTool); ok {
			st.SetContext(channel, chatID)
		}
	}
	if tool, ok := al.tools.Get("subagent"); ok {
		if st, ok := tool.(tools.ContextualTool); ok {
			st.SetContext(channel, chatID)
		}
	}
}

// maybeDailyCutover checks whether the session has crossed a daily boundary
// (CutoverHour, default 04:00) and, if so, saves the session content as a daily
// note and resets the session for the new day.
func (al *AgentLoop) maybeDailyCutover(sessionKey string) {
	updated := al.sessions.GetUpdatedTime(sessionKey)
	if updated.IsZero() {
		return
	}

	now := time.Now()
	boundary := GetCutoverBoundary(now)

	if !updated.Before(boundary) {
		return
	}

	history := al.sessions.GetHistory(sessionKey)
	summary := al.sessions.GetSummary(sessionKey)

	if len(history) == 0 && summary == "" {
		al.sessions.ResetSession(sessionKey)
		al.sessions.Save(sessionKey)
		return
	}

	var recentLines []string
	for _, msg := range history {
		if msg.Role == "user" || msg.Role == "assistant" {
			line := fmt.Sprintf("- **%s**: %s", msg.Role, utils.Truncate(msg.Content, 200))
			recentLines = append(recentLines, line)
		}
	}

	note := FormatCutoverNote(summary, recentLines)
	if note != "" {
		noteDate := GetLogicalDate(updated)
		ms := al.contextBuilder.GetMemoryStore()
		if err := ms.SaveDailyNoteForDate(noteDate, note); err != nil {
			logger.WarnCF("agent", "Failed to save daily cutover note", map[string]interface{}{
				"session_key": sessionKey,
				"note_date":   noteDate.Format("2006-01-02"),
				"error":       err.Error(),
			})
		} else {
			logger.InfoCF("agent", "Daily cutover: session archived to daily note", map[string]interface{}{
				"session_key": sessionKey,
				"note_date":   noteDate.Format("2006-01-02"),
				"messages":    len(history),
			})
		}
	}

	al.sessions.ResetSession(sessionKey)
	al.sessions.Save(sessionKey)
}

// maybeSummarize triggers summarization if the session history exceeds thresholds.
func (al *AgentLoop) maybeSummarize(sessionKey, channel, chatID string) {
	newHistory := al.sessions.GetHistory(sessionKey)
	tokenEstimate := al.estimateTokens(newHistory)
	threshold := al.contextWindow * 75 / 100

	if len(newHistory) > 20 || tokenEstimate > threshold {
		if _, loading := al.summarizing.LoadOrStore(sessionKey, true); !loading {
			go func() {
				defer al.summarizing.Delete(sessionKey)
				// Notify user about optimization if not an internal channel
				if !constants.IsInternalChannel(channel) {
					al.bus.PublishOutbound(bus.OutboundMessage{
						Channel: channel,
						ChatID:  chatID,
						Content: "⚠️ Memory threshold reached. Optimizing conversation history...",
					})
				}
				al.summarizeSession(sessionKey)
			}()
		}
	}
}

// forceCompression aggressively reduces context when the limit is hit.
// It drops the oldest 50% of messages (keeping system prompt and last user message).
func (al *AgentLoop) forceCompression(sessionKey string) {
	history := al.sessions.GetHistory(sessionKey)
	if len(history) <= 4 {
		return
	}

	// Keep system prompt (usually [0]) and the very last message (user's trigger)
	// We want to drop the oldest half of the *conversation*
	// Assuming [0] is system, [1:] is conversation
	conversation := history[1 : len(history)-1]
	if len(conversation) == 0 {
		return
	}

	// Helper to find the mid-point of the conversation
	mid := len(conversation) / 2

	// New history structure:
	// 1. System Prompt
	// 2. [Summary of dropped part] - synthesized
	// 3. Second half of conversation
	// 4. Last message

	// Simplified approach for emergency: Drop first half of conversation
	// and rely on existing summary if present, or create a placeholder.

	droppedCount := mid
	keptConversation := conversation[mid:]

	newHistory := make([]providers.Message, 0)
	newHistory = append(newHistory, history[0]) // System prompt

	// Add a note about compression
	compressionNote := fmt.Sprintf("[System: Emergency compression dropped %d oldest messages due to context limit]", droppedCount)
	// If there was an existing summary, we might lose it if it was in the dropped part (which is just messages).
	// The summary is stored separately in session.Summary, so it persists!
	// We just need to ensure the user knows there's a gap.

	// We only modify the messages list here
	newHistory = append(newHistory, providers.Message{
		Role:    "system",
		Content: compressionNote,
	})

	newHistory = append(newHistory, keptConversation...)
	newHistory = append(newHistory, history[len(history)-1]) // Last message

	// Update session
	al.sessions.SetHistory(sessionKey, newHistory)
	al.sessions.Save(sessionKey)

	logger.WarnCF("agent", "Forced compression executed", map[string]interface{}{
		"session_key":  sessionKey,
		"dropped_msgs": droppedCount,
		"new_count":    len(newHistory),
	})
}

// GetStartupInfo returns information about loaded tools and skills for logging.
func (al *AgentLoop) GetStartupInfo() map[string]interface{} {
	info := make(map[string]interface{})

	// Tools info
	tools := al.tools.List()
	info["tools"] = map[string]interface{}{
		"count": len(tools),
		"names": tools,
	}

	// Skills info
	info["skills"] = al.contextBuilder.GetSkillsInfo()

	return info
}

// formatMessagesForLog formats messages for logging
func formatMessagesForLog(messages []providers.Message) string {
	if len(messages) == 0 {
		return "[]"
	}

	var result string
	result += "[\n"
	for i, msg := range messages {
		result += fmt.Sprintf("  [%d] Role: %s\n", i, msg.Role)
		if len(msg.ToolCalls) > 0 {
			result += "  ToolCalls:\n"
			for _, tc := range msg.ToolCalls {
				result += fmt.Sprintf("    - ID: %s, Type: %s, Name: %s\n", tc.ID, tc.Type, tc.Name)
				if tc.Function != nil {
					result += fmt.Sprintf("      Arguments: %s\n", utils.Truncate(tc.Function.Arguments, 200))
				}
			}
		}
		if msg.Content != "" {
			content := utils.Truncate(msg.Content, 200)
			result += fmt.Sprintf("  Content: %s\n", content)
		}
		if msg.ToolCallID != "" {
			result += fmt.Sprintf("  ToolCallID: %s\n", msg.ToolCallID)
		}
		result += "\n"
	}
	result += "]"
	return result
}

// formatToolsForLog formats tool definitions for logging
func formatToolsForLog(tools []providers.ToolDefinition) string {
	if len(tools) == 0 {
		return "[]"
	}

	var result string
	result += "[\n"
	for i, tool := range tools {
		result += fmt.Sprintf("  [%d] Type: %s, Name: %s\n", i, tool.Type, tool.Function.Name)
		result += fmt.Sprintf("      Description: %s\n", tool.Function.Description)
		if len(tool.Function.Parameters) > 0 {
			result += fmt.Sprintf("      Parameters: %s\n", utils.Truncate(fmt.Sprintf("%v", tool.Function.Parameters), 200))
		}
	}
	result += "]"
	return result
}

// summarizeSession summarizes the conversation history for a session.
func (al *AgentLoop) summarizeSession(sessionKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	history := al.sessions.GetHistory(sessionKey)
	summary := al.sessions.GetSummary(sessionKey)

	// Keep last 4 messages for continuity
	if len(history) <= 4 {
		return
	}

	toSummarize := history[:len(history)-4]

	// Oversized Message Guard
	// Skip messages larger than 50% of context window to prevent summarizer overflow
	maxMessageTokens := al.contextWindow / 2
	validMessages := make([]providers.Message, 0)
	omitted := false

	for _, m := range toSummarize {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		// Estimate tokens for this message
		msgTokens := len(m.Content) / 2 // Use safer estimate here too (2.5 -> 2 for integer division safety)
		if msgTokens > maxMessageTokens {
			omitted = true
			continue
		}
		validMessages = append(validMessages, m)
	}

	if len(validMessages) == 0 {
		return
	}

	// Multi-Part Summarization
	// Split into two parts if history is significant
	var finalSummary string
	if len(validMessages) > 10 {
		mid := len(validMessages) / 2
		part1 := validMessages[:mid]
		part2 := validMessages[mid:]

		s1, _ := al.summarizeBatch(ctx, part1, "")
		s2, _ := al.summarizeBatch(ctx, part2, "")

		// Merge them
		mergePrompt := fmt.Sprintf("Merge these two conversation summaries into one cohesive summary:\n\n1: %s\n\n2: %s", s1, s2)
		resp, err := al.provider.Chat(ctx, []providers.Message{{Role: "user", Content: mergePrompt}}, nil, al.model, map[string]interface{}{
			"max_tokens":  1024,
			"temperature": 0.3,
		})
		if err == nil {
			finalSummary = resp.Content
		} else {
			finalSummary = s1 + " " + s2
		}
	} else {
		finalSummary, _ = al.summarizeBatch(ctx, validMessages, summary)
	}

	if omitted && finalSummary != "" {
		finalSummary += "\n[Note: Some oversized messages were omitted from this summary for efficiency.]"
	}

	if finalSummary != "" {
		al.sessions.SetSummary(sessionKey, finalSummary)
		al.sessions.TruncateHistory(sessionKey, 4)
		al.sessions.Save(sessionKey)
	}
}

// summarizeBatch summarizes a batch of messages.
func (al *AgentLoop) summarizeBatch(ctx context.Context, batch []providers.Message, existingSummary string) (string, error) {
	prompt := "Provide a concise summary of this conversation segment, preserving core context and key points.\n"
	if existingSummary != "" {
		prompt += "Existing context: " + existingSummary + "\n"
	}
	prompt += "\nCONVERSATION:\n"
	for _, m := range batch {
		prompt += fmt.Sprintf("%s: %s\n", m.Role, m.Content)
	}

	response, err := al.provider.Chat(ctx, []providers.Message{{Role: "user", Content: prompt}}, nil, al.model, map[string]interface{}{
		"max_tokens":  1024,
		"temperature": 0.3,
	})
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// estimateTokens estimates the number of tokens in a message list.
// Uses a safe heuristic of 2.5 characters per token to account for CJK and other
// overheads better than the previous 3 chars/token.
func (al *AgentLoop) estimateTokens(messages []providers.Message) int {
	totalChars := 0
	for _, m := range messages {
		totalChars += utf8.RuneCountInString(m.Content)
	}
	// 2.5 chars per token = totalChars * 2 / 5
	return totalChars * 2 / 5
}

func (al *AgentLoop) handleCommand(ctx context.Context, msg bus.InboundMessage) (string, bool) {
	content := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(content, "/") {
		return "", false
	}

	parts := strings.Fields(content)
	if len(parts) == 0 {
		return "", false
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "/show":
		if len(args) < 1 {
			return "Usage: /show [model|channel]", true
		}
		switch args[0] {
		case "model":
			return fmt.Sprintf("Current model: %s", al.model), true
		case "channel":
			return fmt.Sprintf("Current channel: %s", msg.Channel), true
		default:
			return fmt.Sprintf("Unknown show target: %s", args[0]), true
		}

	case "/list":
		if len(args) < 1 {
			return "Usage: /list [models|channels]", true
		}
		switch args[0] {
		case "models":
			// TODO: Fetch available models dynamically if possible
			return "Available models: glm-4.7, claude-3-5-sonnet, gpt-4o (configured in config.json/env)", true
		case "channels":
			if al.channelManager == nil {
				return "Channel manager not initialized", true
			}
			channels := al.channelManager.GetEnabledChannels()
			if len(channels) == 0 {
				return "No channels enabled", true
			}
			return fmt.Sprintf("Enabled channels: %s", strings.Join(channels, ", ")), true
		default:
			return fmt.Sprintf("Unknown list target: %s", args[0]), true
		}

	case "/switch":
		if len(args) < 3 || args[1] != "to" {
			return "Usage: /switch [model|channel] to <name>", true
		}
		target := args[0]
		value := args[2]

		switch target {
		case "model":
			oldModel := al.model
			al.model = value
			return fmt.Sprintf("Switched model from %s to %s", oldModel, value), true
		case "channel":
			// This changes the 'default' channel for some operations, or effectively redirects output?
			// For now, let's just validate if the channel exists
			if al.channelManager == nil {
				return "Channel manager not initialized", true
			}
			if _, exists := al.channelManager.GetChannel(value); !exists && value != "cli" {
				return fmt.Sprintf("Channel '%s' not found or not enabled", value), true
			}

			// If message came from CLI, maybe we want to redirect CLI output to this channel?
			// That would require state persistence about "redirected channel"
			// For now, just acknowledged.
			return fmt.Sprintf("Switched target channel to %s (Note: this currently only validates existence)", value), true
		default:
			return fmt.Sprintf("Unknown switch target: %s", target), true
		}

	case "/work":
		wc := parseWorkCommand(content)
		if !wc.Ok {
			return "使い方: /work [N] | /work status | /work off", true
		}
		flags := al.sessions.GetFlags(msg.SessionKey)
		switch wc.Kind {
		case "on":
			flags.WorkOverlayTurnsLeft = wc.Turns
			flags.WorkOverlayDirective = WorkOverlayDirectiveText
			al.sessions.SetFlags(msg.SessionKey, flags)
			al.sessions.Save(msg.SessionKey)
			return fmt.Sprintf("了解しました。以後%dターン、仕事モードで進めます。", wc.Turns), true
		case "off":
			flags.WorkOverlayTurnsLeft = 0
			flags.WorkOverlayDirective = ""
			al.sessions.SetFlags(msg.SessionKey, flags)
			al.sessions.Save(msg.SessionKey)
			return "了解しました。会話モードに戻します。", true
		case "status":
			if flags.WorkOverlayTurnsLeft > 0 {
				return fmt.Sprintf("仕事モード残り：%dターン。解除は /normal です。", flags.WorkOverlayTurnsLeft), true
			}
			return "現在は会話モードです。仕事モードは /work で有効にできます。", true
		}

	case "/normal":
		flags := al.sessions.GetFlags(msg.SessionKey)
		flags.WorkOverlayTurnsLeft = 0
		flags.WorkOverlayDirective = ""
		al.sessions.SetFlags(msg.SessionKey, flags)
		al.sessions.Save(msg.SessionKey)
		return "了解しました。会話モードに戻します。", true

	}

	return "", false
}

// Coder3Output は Coder3 の出力フォーマット
type Coder3Output struct {
	JobID    string                 `json:"job_id"`
	Plan     string                 `json:"plan"`
	Patch    string                 `json:"patch"`
	Risk     map[string]interface{} `json:"risk"`
	CostHint map[string]interface{} `json:"cost_hint"`
}

// parseCoder3Output は Coder3 のレスポンスを解析
func parseCoder3Output(response string) (*Coder3Output, error) {
	var output Coder3Output
	if err := json.Unmarshal([]byte(response), &output); err != nil {
		return nil, fmt.Errorf("failed to parse Coder3 output: %w", err)
	}

	// 必須フィールドのチェック
	if output.JobID == "" {
		return nil, fmt.Errorf("missing required field: job_id")
	}
	if output.Plan == "" {
		return nil, fmt.Errorf("missing required field: plan")
	}

	return &output, nil
}

// detectPatchType は patch の形式を判定する
func detectPatchType(patch string) string {
	trimmed := strings.TrimSpace(patch)
	if strings.HasPrefix(trimmed, "[") {
		return "json"
	}
	if strings.Contains(patch, "```") {
		return "markdown"
	}
	return "unknown"
}

// formatExecutionResults は Worker の実行結果を整形して返す
func formatExecutionResults(results []CommandResult) string {
	var lines []string
	for i, r := range results {
		status := "✓"
		if !r.Success {
			status = "✗"
		}
		line := fmt.Sprintf("%s [%d] %s %s %s (%dms)",
			status, i+1, r.Command.Type, r.Command.Action, r.Command.Target, r.Duration)
		if r.Error != "" {
			line += fmt.Sprintf("\n    エラー: %s", r.Error)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
