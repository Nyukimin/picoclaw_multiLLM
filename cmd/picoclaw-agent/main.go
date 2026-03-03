package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

const shutdownTimeout = 30 * time.Second

// AgentHandler はスタンドアロンAgentの処理インターフェース
type AgentHandler interface {
	HandleMessage(ctx context.Context, msg domaintransport.Message) (domaintransport.Message, error)
}

// workerHandler はWorkerエージェントのハンドラ（スタブ）
type workerHandler struct{}

func (h *workerHandler) HandleMessage(ctx context.Context, msg domaintransport.Message) (domaintransport.Message, error) {
	// NOTE: Phase 5で実際のWorker実行ロジックに接続予定
	response := domaintransport.NewMessage(msg.To, msg.From, msg.SessionID, msg.JobID, "worker executed")
	response.Type = domaintransport.MessageTypeResult
	response.Result = &domaintransport.ResultPayload{
		Success: true,
		Summary: "standalone worker execution (stub)",
	}
	return response, nil
}

// coderHandler はCoderエージェントのハンドラ（スタブ）
type coderHandler struct{}

func (h *coderHandler) HandleMessage(ctx context.Context, msg domaintransport.Message) (domaintransport.Message, error) {
	// NOTE: Phase 5で実際のCoder生成ロジックに接続予定
	response := domaintransport.NewMessage(msg.To, msg.From, msg.SessionID, msg.JobID, "coder generated")
	response.Type = domaintransport.MessageTypeResult
	response.Proposal = &domaintransport.ProposalPayload{
		Plan:  "standalone coder plan (stub)",
		Patch: "{}",
	}
	return response, nil
}

func main() {
	standalone := flag.Bool("standalone", false, "Run in standalone mode")
	agentType := flag.String("agent", "", "Agent type: worker or coder")
	flag.Parse()

	if !*standalone {
		fmt.Fprintln(os.Stderr, "picoclaw-agent must be run with --standalone flag")
		os.Exit(1)
	}

	if *agentType == "" {
		fmt.Fprintln(os.Stderr, "picoclaw-agent requires --agent flag (worker or coder)")
		os.Exit(1)
	}

	var handler AgentHandler
	switch *agentType {
	case "worker":
		handler = &workerHandler{}
	case "coder":
		handler = &coderHandler{}
	default:
		fmt.Fprintf(os.Stderr, "Unknown agent type: %s (supported: worker, coder)\n", *agentType)
		os.Exit(1)
	}

	log.SetOutput(os.Stderr) // stdoutはJSON通信に使うのでstderrにログ出力
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

	if err := runMessageLoop(ctx, handler); err != nil {
		log.Printf("[picoclaw-agent] Message loop ended: %v", err)
	}

	log.Println("[picoclaw-agent] Shutdown complete")
}

// runMessageLoop はstdin/stdout上のJSON通信ループ
func runMessageLoop(ctx context.Context, handler AgentHandler) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	encoder := json.NewEncoder(os.Stdout)

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
