package heartbeat

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ctxbuilder "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/context"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/memory"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// ChatAgent はHeartbeatが会話処理を委譲するインターフェース
type ChatAgent interface {
	Chat(ctx context.Context, t task.Task) (string, error)
}

// NotificationSender はユーザーへの通知を送信するインターフェース
type NotificationSender interface {
	SendNotification(ctx context.Context, message string) error
}

// HeartbeatService はHEARTBEAT.mdを定期的に読み込み、エージェントに処理させるサービス
type HeartbeatService struct {
	chatAgent      ChatAgent
	sender         NotificationSender
	workspaceDir   string
	contextBuilder *ctxbuilder.Builder
	interval       time.Duration
	stopCh         chan struct{}
	done           chan struct{}
	mu             sync.Mutex
	running        bool
}

// NewHeartbeatService は新しいHeartbeatServiceを作成
func NewHeartbeatService(
	chatAgent ChatAgent,
	sender NotificationSender,
	workspaceDir string,
	intervalMinutes int,
) *HeartbeatService {
	if intervalMinutes < 5 {
		intervalMinutes = 5
	}
	return &HeartbeatService{
		chatAgent:      chatAgent,
		sender:         sender,
		workspaceDir:   workspaceDir,
		contextBuilder: ctxbuilder.NewBuilder(workspaceDir),
		interval:       time.Duration(intervalMinutes) * time.Minute,
		stopCh:         make(chan struct{}),
		done:           make(chan struct{}),
	}
}

// WithMemoryStore はメモリストアを設定する（オプション）
func (s *HeartbeatService) WithMemoryStore(store memory.Store) *HeartbeatService {
	s.contextBuilder.WithMemoryStore(store)
	return s
}

// Start はHeartbeatサービスをバックグラウンドで開始
func (s *HeartbeatService) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.loop()
	log.Printf("HeartbeatService started (interval: %v, workspace: %s)", s.interval, s.workspaceDir)
}

// Stop はHeartbeatサービスを停止
func (s *HeartbeatService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	<-s.done
	log.Println("HeartbeatService stopped")
}

// loop はHeartbeatの定期実行ループ
func (s *HeartbeatService) loop() {
	defer close(s.done)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if err := s.tick(context.Background()); err != nil {
				log.Printf("[Heartbeat] tick error: %v", err)
			}
		}
	}
}

// tick は1回のHeartbeat処理を実行
func (s *HeartbeatService) tick(ctx context.Context) error {
	// HEARTBEAT.md を読み込み
	heartbeatPath := filepath.Join(s.workspaceDir, "HEARTBEAT.md")
	data, err := os.ReadFile(heartbeatPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("[Heartbeat] HEARTBEAT.md not found, skipping")
			return nil
		}
		return fmt.Errorf("failed to read HEARTBEAT.md: %w", err)
	}

	heartbeatContent := strings.TrimSpace(string(data))
	if heartbeatContent == "" {
		log.Println("[Heartbeat] HEARTBEAT.md is empty, skipping")
		return nil
	}

	// ContextBuilder でコンテキスト + HEARTBEAT.md を組み立て
	message := s.contextBuilder.BuildMessageWithTask("CHAT", "HEARTBEAT TASKS", heartbeatContent)

	// タスクを作成してMioに処理させる
	jobID := task.NewJobID()
	t := task.NewTask(jobID, message, "heartbeat", "heartbeat")

	response, err := s.chatAgent.Chat(ctx, t)
	if err != nil {
		s.logHeartbeat("ERROR", fmt.Sprintf("chat failed: %v", err))
		return fmt.Errorf("chat failed: %w", err)
	}

	// HEARTBEAT_OK なら正常終了（サイレント）
	if strings.TrimSpace(response) == "HEARTBEAT_OK" {
		s.logHeartbeat("OK", "silent")
		return nil
	}

	// HEARTBEAT_OK 以外はユーザーに通知
	s.logHeartbeat("NOTIFY", response)
	if s.sender != nil {
		if err := s.sender.SendNotification(ctx, response); err != nil {
			return fmt.Errorf("failed to send notification: %w", err)
		}
	}

	return nil
}

// logHeartbeat はHeartbeat結果をheartbeat.logに記録
func (s *HeartbeatService) logHeartbeat(status, message string) {
	logPath := filepath.Join(s.workspaceDir, "heartbeat.log")
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] [%s] %s\n", timestamp, status, message)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[Heartbeat] failed to write log: %v", err)
		return
	}
	defer f.Close()
	f.WriteString(entry)
}
