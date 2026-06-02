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

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	revenueapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/revenue"
	skillbootstrap "github.com/Nyukimin/picoclaw_multiLLM/internal/application/skillgovernance"
	ctxbuilder "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/context"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/memory"
	domainskill "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/skillgovernance"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domainworkstream "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/workstream"
)

// ChatAgent はHeartbeatが会話処理を委譲するインターフェース
type ChatAgent interface {
	Chat(ctx context.Context, t task.Task) (string, error)
}

// NotificationSender はユーザーへの通知を送信するインターフェース
type NotificationSender interface {
	SendNotification(ctx context.Context, message string) error
}

type WorkstreamHeartbeatStore interface {
	ListHeartbeatSchedules(ctx context.Context, limit int) ([]domainworkstream.HeartbeatSchedule, error)
	SaveHeartbeatSchedule(ctx context.Context, item domainworkstream.HeartbeatSchedule) error
	ListSteeringItems(ctx context.Context, limit int) ([]domainworkstream.SteeringItem, error)
	SaveSteeringItem(ctx context.Context, item domainworkstream.SteeringItem) error
	SaveVaultUpdateLog(ctx context.Context, item domainworkstream.VaultUpdateLog) error
}

type RevenueDailyRoutineStore = revenueapp.DailyRoutineStore

type IdleChatSequenceMonitor interface {
	CheckIdleChatSequence(ctx context.Context, now time.Time) IdleChatSequenceCheck
}

type IdleChatSequenceCheck struct {
	Status     string
	Active     bool
	Recovered  bool
	Stage      string
	Detail     string
	SessionID  string
	Generation uint64
	AgeSeconds int64
	Action     string
	Error      string
	CheckedAt  time.Time
}

// HeartbeatService はHEARTBEAT.mdを定期的に読み込み、エージェントに処理させるサービス
type HeartbeatService struct {
	chatAgent        ChatAgent
	sender           NotificationSender
	workspaceDir     string
	contextBuilder   *ctxbuilder.Builder
	listener         orchestrator.EventListener
	workstreamStore  WorkstreamHeartbeatStore
	revenueStore     RevenueDailyRoutineStore
	revenueRoutine   *revenueapp.DailyRoutineService
	skills           *skillbootstrap.BootstrapService
	idleChatMonitor  IdleChatSequenceMonitor
	interval         time.Duration
	idleChatInterval time.Duration
	stopCh           chan struct{}
	done             chan struct{}
	mu               sync.Mutex
	running          bool
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
		chatAgent:        chatAgent,
		sender:           sender,
		workspaceDir:     workspaceDir,
		contextBuilder:   ctxbuilder.NewBuilder(workspaceDir),
		interval:         time.Duration(intervalMinutes) * time.Minute,
		idleChatInterval: time.Minute,
		stopCh:           make(chan struct{}),
		done:             make(chan struct{}),
	}
}

// WithMemoryStore はメモリストアを設定する（オプション）
func (s *HeartbeatService) WithMemoryStore(store memory.Store) *HeartbeatService {
	s.contextBuilder.WithMemoryStore(store)
	return s
}

// WithEventListener sends Heartbeat results to external monitors such as Viewer SSE.
func (s *HeartbeatService) WithEventListener(listener orchestrator.EventListener) *HeartbeatService {
	s.listener = listener
	return s
}

// WithWorkstreamStore enables draft-only Workstream heartbeat execution.
func (s *HeartbeatService) WithWorkstreamStore(store WorkstreamHeartbeatStore) *HeartbeatService {
	s.workstreamStore = store
	return s
}

// WithRevenueDailyRoutineStore enables draft-only Revenue daily routine recording for revenue Workstream heartbeats.
func (s *HeartbeatService) WithRevenueDailyRoutineStore(store RevenueDailyRoutineStore) *HeartbeatService {
	s.revenueStore = store
	s.revenueRoutine = revenueapp.NewDailyRoutineService(store)
	return s
}

func (s *HeartbeatService) WithSkillBootstrap(service *skillbootstrap.BootstrapService) *HeartbeatService {
	s.skills = service
	return s
}

func (s *HeartbeatService) WithIdleChatSequenceMonitor(monitor IdleChatSequenceMonitor) *HeartbeatService {
	s.idleChatMonitor = monitor
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
	idleChatTicker := time.NewTicker(s.idleChatInterval)
	defer idleChatTicker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-idleChatTicker.C:
			s.runIdleChatSequenceCheck(context.Background(), time.Now().UTC())
		case <-ticker.C:
			ctx := context.Background()
			if err := s.tick(ctx); err != nil {
				log.Printf("[Heartbeat] tick error: %v", err)
			}
			if _, err := s.RunDueWorkstreamHeartbeats(ctx, time.Now().UTC()); err != nil {
				log.Printf("[Heartbeat] workstream tick error: %v", err)
			}
		}
	}
}

func (s *HeartbeatService) runIdleChatSequenceCheck(ctx context.Context, now time.Time) IdleChatSequenceCheck {
	if s.idleChatMonitor == nil {
		return IdleChatSequenceCheck{Status: "disabled", CheckedAt: now.UTC()}
	}
	report := s.idleChatMonitor.CheckIdleChatSequence(ctx, now.UTC())
	if report.CheckedAt.IsZero() {
		report.CheckedAt = now.UTC()
	}
	status := strings.TrimSpace(report.Status)
	if status == "" {
		status = "unknown"
		report.Status = status
	}
	if report.Error != "" {
		log.Printf("[Heartbeat] idlechat sequence check error: %s", report.Error)
		s.emitEvent("heartbeat.idlechat_sequence.error", report.Error)
		return report
	}
	log.Printf("[Heartbeat] idlechat sequence check: status=%s active=%t recovered=%t stage=%s detail=%s session=%s age=%ds generation=%d action=%s",
		report.Status, report.Active, report.Recovered, report.Stage, report.Detail, report.SessionID, report.AgeSeconds, report.Generation, report.Action)
	s.emitEvent("heartbeat.idlechat_sequence."+status, fmt.Sprintf("active=%t recovered=%t stage=%s detail=%s session=%s age=%ds action=%s",
		report.Active, report.Recovered, report.Stage, report.Detail, report.SessionID, report.AgeSeconds, report.Action))
	return report
}

// tick は1回のHeartbeat処理を実行
func (s *HeartbeatService) tick(ctx context.Context) error {
	// HEARTBEAT.md を読み込み
	heartbeatPath := filepath.Join(s.workspaceDir, "HEARTBEAT.md")
	data, err := os.ReadFile(heartbeatPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("[Heartbeat] HEARTBEAT.md not found, skipping")
			s.emitEvent("heartbeat.skip", "HEARTBEAT.md not found")
			return nil
		}
		wrapped := fmt.Errorf("failed to read HEARTBEAT.md: %w", err)
		s.emitEvent("heartbeat.error", wrapped.Error())
		return wrapped
	}

	heartbeatContent := strings.TrimSpace(string(data))
	if heartbeatContent == "" {
		log.Println("[Heartbeat] HEARTBEAT.md is empty, skipping")
		s.emitEvent("heartbeat.skip", "HEARTBEAT.md is empty")
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
		s.emitEvent("heartbeat.error", fmt.Sprintf("chat failed: %v", err))
		return fmt.Errorf("chat failed: %w", err)
	}

	// HEARTBEAT_OK なら正常終了（サイレント）
	if strings.TrimSpace(response) == "HEARTBEAT_OK" {
		s.logHeartbeat("OK", "silent")
		s.emitEvent("heartbeat.ok", "silent")
		return nil
	}

	// HEARTBEAT_OK 以外はユーザーに通知
	s.logHeartbeat("NOTIFY", response)
	s.emitEvent("heartbeat.notify", response)
	if s.sender != nil {
		if err := s.sender.SendNotification(ctx, response); err != nil {
			s.emitEvent("heartbeat.error", fmt.Sprintf("failed to send notification: %v", err))
			return fmt.Errorf("failed to send notification: %w", err)
		}
	}

	return nil
}

type WorkstreamHeartbeatRunReport struct {
	Checked int
	Run     int
	Skipped int
	Failed  int
}

func (s *HeartbeatService) RunDueWorkstreamHeartbeats(ctx context.Context, now time.Time) (WorkstreamHeartbeatRunReport, error) {
	var report WorkstreamHeartbeatRunReport
	if s.workstreamStore == nil {
		return report, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	schedules, err := s.workstreamStore.ListHeartbeatSchedules(ctx, 1000)
	if err != nil {
		s.emitEvent("workstream.heartbeat.error", fmt.Sprintf("failed to list schedules: %v", err))
		return report, err
	}
	seen := map[string]struct{}{}
	for _, schedule := range schedules {
		if _, ok := seen[schedule.HeartbeatID]; ok {
			continue
		}
		seen[schedule.HeartbeatID] = struct{}{}
		report.Checked++
		if schedule.Status != domainworkstream.StatusActive || !heartbeatDue(schedule, now) {
			report.Skipped++
			continue
		}
		if err := s.runWorkstreamHeartbeat(ctx, schedule, now); err != nil {
			report.Failed++
			s.emitEvent("workstream.heartbeat.error", err.Error())
			return report, err
		}
		report.Run++
	}
	if report.Run > 0 {
		s.emitEvent("workstream.heartbeat.completed", fmt.Sprintf("run=%d skipped=%d", report.Run, report.Skipped))
	}
	return report, nil
}

func (s *HeartbeatService) runWorkstreamHeartbeat(ctx context.Context, schedule domainworkstream.HeartbeatSchedule, now time.Time) error {
	if s.skills != nil {
		if _, err := s.skills.Record(ctx, domainskill.TaskContext{
			Text:         schedule.Task,
			Intent:       "workstream_heartbeat",
			Agent:        "Worker",
			WorkstreamID: schedule.WorkstreamID,
		}, []string{"core.workstream-heartbeat", "core.workstream"}); err != nil {
			return fmt.Errorf("workstream heartbeat %s skill bootstrap failed: %w", schedule.HeartbeatID, err)
		}
	}
	pendingSteering, err := s.pendingSteeringForWorkstream(ctx, schedule.WorkstreamID)
	if err != nil {
		return fmt.Errorf("workstream heartbeat %s steering checkpoint failed: %w", schedule.HeartbeatID, err)
	}
	message := s.contextBuilder.BuildMessageWithTask(
		"CHAT",
		"WORKSTREAM HEARTBEAT DRAFT",
		fmt.Sprintf("workstream_id: %s\nheartbeat_id: %s\nschedule: %s\ntask: %s\n\nsafe_checkpoint_steering:\n%s\n\n制約: draft report only。投稿、送信、販売、外部書き込みは行わない。",
			schedule.WorkstreamID,
			schedule.HeartbeatID,
			schedule.ScheduleText,
			schedule.Task,
			formatSteeringForPrompt(pendingSteering),
		),
	)
	jobID := task.NewJobID()
	t := task.NewTask(jobID, message, "workstream-heartbeat", "heartbeat")
	response, err := s.chatAgent.Chat(ctx, t)
	if err != nil {
		return fmt.Errorf("workstream heartbeat %s chat failed: %w", schedule.HeartbeatID, err)
	}
	reportPath, err := s.writeWorkstreamHeartbeatReport(schedule, now, response)
	if err != nil {
		return fmt.Errorf("workstream heartbeat %s report failed: %w", schedule.HeartbeatID, err)
	}
	update := domainworkstream.VaultUpdateLog{
		UpdateID:     fmt.Sprintf("vul_%s_%d", schedule.HeartbeatID, now.UnixNano()),
		WorkstreamID: schedule.WorkstreamID,
		FilePath:     reportPath,
		UpdateType:   "heartbeat_draft_report",
		ReviewStatus: "pending",
		CreatedAt:    now.UTC(),
	}
	if err := s.workstreamStore.SaveVaultUpdateLog(ctx, update); err != nil {
		return fmt.Errorf("workstream heartbeat %s vault update log failed: %w", schedule.HeartbeatID, err)
	}
	if shouldRunRevenueDailyRoutine(schedule) {
		if err := s.runRevenueDailyRoutine(ctx, schedule, now); err != nil {
			return fmt.Errorf("workstream heartbeat %s revenue daily routine failed: %w", schedule.HeartbeatID, err)
		}
	}
	if err := s.markSteeringApplied(ctx, pendingSteering, now); err != nil {
		return fmt.Errorf("workstream heartbeat %s steering apply failed: %w", schedule.HeartbeatID, err)
	}
	schedule.LastRunAt = now.UTC()
	schedule.NextRunAt = nextHeartbeatRun(schedule, now)
	if err := s.workstreamStore.SaveHeartbeatSchedule(ctx, schedule); err != nil {
		return fmt.Errorf("workstream heartbeat %s schedule update failed: %w", schedule.HeartbeatID, err)
	}
	s.emitEvent("workstream.heartbeat.draft_report", reportPath)
	return nil
}

func shouldRunRevenueDailyRoutine(schedule domainworkstream.HeartbeatSchedule) bool {
	text := strings.ToLower(strings.Join([]string{schedule.HeartbeatID, schedule.WorkstreamID, schedule.Task}, "\n"))
	keywords := []string{
		"revenue",
		"収益",
		"売上",
		"市場調査",
		"sns",
		"商品",
		"顧客の声",
	}
	for _, keyword := range keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func (s *HeartbeatService) runRevenueDailyRoutine(ctx context.Context, schedule domainworkstream.HeartbeatSchedule, now time.Time) error {
	if s.revenueRoutine == nil {
		return nil
	}
	result, err := s.revenueRoutine.RunDailyRoutine(ctx, revenueapp.DailyRoutineRequest{
		ReportID:     fmt.Sprintf("rev_daily_%s_%d", safePathSegment(schedule.HeartbeatID), now.UnixNano()),
		WorkstreamID: schedule.WorkstreamID,
		Date:         now.UTC().Format("2006-01-02"),
		Now:          now.UTC(),
	})
	if err != nil {
		return err
	}
	s.emitEvent("revenue.daily_routine.draft_report", fmt.Sprintf("%s:%s", result.Agent, result.Report.ReportID))
	return nil
}

func (s *HeartbeatService) pendingSteeringForWorkstream(ctx context.Context, workstreamID string) ([]domainworkstream.SteeringItem, error) {
	if s.workstreamStore == nil {
		return nil, nil
	}
	items, err := s.workstreamStore.ListSteeringItems(ctx, 1000)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var pending []domainworkstream.SteeringItem
	for _, item := range items {
		if item.WorkstreamID != workstreamID {
			continue
		}
		if _, ok := seen[item.SteeringID]; ok {
			continue
		}
		seen[item.SteeringID] = struct{}{}
		if strings.TrimSpace(item.Status) == "pending" {
			pending = append(pending, item)
		}
	}
	return pending, nil
}

func (s *HeartbeatService) markSteeringApplied(ctx context.Context, items []domainworkstream.SteeringItem, now time.Time) error {
	for _, item := range items {
		item.Status = "applied"
		item.AppliedAt = now.UTC()
		if err := s.workstreamStore.SaveSteeringItem(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func formatSteeringForPrompt(items []domainworkstream.SteeringItem) string {
	if len(items) == 0 {
		return "- none"
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		target := strings.TrimSpace(item.TargetArtifactID)
		if target == "" {
			target = "workstream"
		}
		lines = append(lines, fmt.Sprintf("- %s [%s]: %s", item.SteeringID, target, strings.TrimSpace(item.Instruction)))
	}
	return strings.Join(lines, "\n")
}

func (s *HeartbeatService) writeWorkstreamHeartbeatReport(schedule domainworkstream.HeartbeatSchedule, now time.Time, body string) (string, error) {
	dir := filepath.Join(s.workspaceDir, "workstream_heartbeats", safePathSegment(schedule.WorkstreamID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s.md", safePathSegment(schedule.HeartbeatID), now.UTC().Format("20060102T150405Z"))
	path := filepath.Join(dir, name)
	content := fmt.Sprintf("# Workstream Heartbeat Draft\n\n- workstream_id: %s\n- heartbeat_id: %s\n- schedule: %s\n- created_at: %s\n\n## Task\n\n%s\n\n## Draft Report\n\n%s\n",
		schedule.WorkstreamID,
		schedule.HeartbeatID,
		schedule.ScheduleText,
		now.UTC().Format(time.RFC3339),
		schedule.Task,
		strings.TrimSpace(body),
	)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func heartbeatDue(schedule domainworkstream.HeartbeatSchedule, now time.Time) bool {
	return schedule.NextRunAt.IsZero() || !schedule.NextRunAt.After(now.UTC())
}

func nextHeartbeatRun(schedule domainworkstream.HeartbeatSchedule, now time.Time) time.Time {
	text := strings.TrimSpace(strings.ToLower(schedule.ScheduleText))
	if strings.HasPrefix(text, "daily ") {
		clock := strings.TrimSpace(strings.TrimPrefix(text, "daily "))
		parts := strings.Split(clock, ":")
		if len(parts) == 2 {
			hour, hourErr := parseTwoDigitInt(parts[0])
			minute, minuteErr := parseTwoDigitInt(parts[1])
			if hourErr == nil && minuteErr == nil && hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59 {
				next := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), hour, minute, 0, 0, time.UTC)
				if !next.After(now.UTC()) {
					next = next.Add(24 * time.Hour)
				}
				return next
			}
		}
	}
	return now.UTC().Add(24 * time.Hour)
}

func parseTwoDigitInt(raw string) (int, error) {
	if len(raw) == 0 || len(raw) > 2 {
		return 0, fmt.Errorf("invalid number")
	}
	value := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid number")
		}
		value = value*10 + int(r-'0')
	}
	return value, nil
}

func safePathSegment(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", "..", "_", " ", "_")
	return replacer.Replace(raw)
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

func (s *HeartbeatService) emitEvent(eventType, content string) {
	if s.listener == nil {
		return
	}
	s.listener.OnEvent(orchestrator.NewEvent(
		eventType,
		"heartbeat",
		"viewer",
		content,
		"HEARTBEAT",
		"",
		"heartbeat",
		"heartbeat",
		"viewer",
	))
}
