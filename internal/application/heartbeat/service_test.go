package heartbeat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	skillbootstrap "github.com/Nyukimin/picoclaw_multiLLM/internal/application/skillgovernance"
	domainrevenue "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/revenue"
	domainskill "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/skillgovernance"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domainworkstream "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/workstream"
)

// mockChatAgent はテスト用のChatAgentモック
type mockChatAgent struct {
	response string
	err      error
	called   bool
	lastMsg  string
}

func (m *mockChatAgent) Chat(ctx context.Context, t task.Task) (string, error) {
	m.called = true
	m.lastMsg = t.UserMessage()
	return m.response, m.err
}

// mockSender はテスト用のNotificationSenderモック
type mockSender struct {
	messages []string
	err      error
}

func (m *mockSender) SendNotification(ctx context.Context, message string) error {
	m.messages = append(m.messages, message)
	return m.err
}

type recordingEventListener struct {
	events []orchestrator.OrchestratorEvent
}

func (r *recordingEventListener) OnEvent(ev orchestrator.OrchestratorEvent) {
	r.events = append(r.events, ev)
}

type memoryWorkstreamHeartbeatStore struct {
	schedules     []domainworkstream.HeartbeatSchedule
	saved         []domainworkstream.HeartbeatSchedule
	steering      []domainworkstream.SteeringItem
	savedSteering []domainworkstream.SteeringItem
	vaultUpdates  []domainworkstream.VaultUpdateLog
}

type heartbeatSkillStore struct {
	manifests []domainskill.SkillManifest
	logs      []domainskill.SkillTriggerLog
}

type memoryRevenueDailyRoutineStore struct {
	market    []domainrevenue.MarketResearchItem
	posts     []domainrevenue.SNSPostMetric
	products  []domainrevenue.Product
	voices    []domainrevenue.CustomerVoice
	events    []domainrevenue.RevenueEvent
	decisions []domainrevenue.HumanDecisionGateRecord
	reports   []domainrevenue.DailyRoutineReport
}

func (s *heartbeatSkillStore) ListSkillManifests(_ context.Context, _ int) ([]domainskill.SkillManifest, error) {
	return append([]domainskill.SkillManifest(nil), s.manifests...), nil
}

func (s *heartbeatSkillStore) SaveSkillTriggerLog(_ context.Context, log domainskill.SkillTriggerLog) error {
	s.logs = append(s.logs, log)
	return nil
}

func (s *memoryRevenueDailyRoutineStore) ListMarketResearchItems(_ context.Context, _ int) ([]domainrevenue.MarketResearchItem, error) {
	return append([]domainrevenue.MarketResearchItem(nil), s.market...), nil
}

func (s *memoryRevenueDailyRoutineStore) ListSNSPostMetrics(_ context.Context, _ int) ([]domainrevenue.SNSPostMetric, error) {
	return append([]domainrevenue.SNSPostMetric(nil), s.posts...), nil
}

func (s *memoryRevenueDailyRoutineStore) ListProducts(_ context.Context, _ int) ([]domainrevenue.Product, error) {
	return append([]domainrevenue.Product(nil), s.products...), nil
}

func (s *memoryRevenueDailyRoutineStore) ListCustomerVoices(_ context.Context, _ int) ([]domainrevenue.CustomerVoice, error) {
	return append([]domainrevenue.CustomerVoice(nil), s.voices...), nil
}

func (s *memoryRevenueDailyRoutineStore) ListRevenueEvents(_ context.Context, _ int) ([]domainrevenue.RevenueEvent, error) {
	return append([]domainrevenue.RevenueEvent(nil), s.events...), nil
}

func (s *memoryRevenueDailyRoutineStore) ListHumanDecisionGateRecords(_ context.Context, _ int) ([]domainrevenue.HumanDecisionGateRecord, error) {
	return append([]domainrevenue.HumanDecisionGateRecord(nil), s.decisions...), nil
}

func (s *memoryRevenueDailyRoutineStore) SaveDailyRoutineReport(_ context.Context, item domainrevenue.DailyRoutineReport) error {
	if err := domainrevenue.ValidateDailyRoutineReport(item); err != nil {
		return err
	}
	s.reports = append(s.reports, item)
	return nil
}

func (m *memoryWorkstreamHeartbeatStore) ListHeartbeatSchedules(_ context.Context, _ int) ([]domainworkstream.HeartbeatSchedule, error) {
	return append([]domainworkstream.HeartbeatSchedule(nil), m.schedules...), nil
}

func (m *memoryWorkstreamHeartbeatStore) SaveHeartbeatSchedule(_ context.Context, item domainworkstream.HeartbeatSchedule) error {
	m.saved = append(m.saved, item)
	m.schedules = append([]domainworkstream.HeartbeatSchedule{item}, m.schedules...)
	return nil
}

func (m *memoryWorkstreamHeartbeatStore) ListSteeringItems(_ context.Context, _ int) ([]domainworkstream.SteeringItem, error) {
	return append([]domainworkstream.SteeringItem(nil), m.steering...), nil
}

func (m *memoryWorkstreamHeartbeatStore) SaveSteeringItem(_ context.Context, item domainworkstream.SteeringItem) error {
	m.savedSteering = append(m.savedSteering, item)
	m.steering = append([]domainworkstream.SteeringItem{item}, m.steering...)
	return nil
}

func (m *memoryWorkstreamHeartbeatStore) SaveVaultUpdateLog(_ context.Context, item domainworkstream.VaultUpdateLog) error {
	m.vaultUpdates = append(m.vaultUpdates, item)
	return nil
}

func TestNewHeartbeatService(t *testing.T) {
	t.Run("minimum interval is 5 minutes", func(t *testing.T) {
		svc := NewHeartbeatService(&mockChatAgent{}, &mockSender{}, "/tmp", 1)
		if svc.interval != 5*time.Minute {
			t.Errorf("expected 5m, got %v", svc.interval)
		}
	})

	t.Run("normal interval", func(t *testing.T) {
		svc := NewHeartbeatService(&mockChatAgent{}, &mockSender{}, "/tmp", 30)
		if svc.interval != 30*time.Minute {
			t.Errorf("expected 30m, got %v", svc.interval)
		}
	})
}

func TestTick_HeartbeatOKEmitsViewerEvent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check system status"), 0644)

	listener := &recordingEventListener{}
	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	svc := NewHeartbeatService(agent, &mockSender{}, dir, 30).WithEventListener(listener)

	if err := svc.tick(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listener.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(listener.events))
	}
	ev := listener.events[0]
	if ev.Type != "heartbeat.ok" || ev.From != "heartbeat" || ev.Channel != "heartbeat" || ev.Route != "HEARTBEAT" {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if ev.Content != "silent" {
		t.Fatalf("unexpected event content: %q", ev.Content)
	}
}

func TestTick_NotificationEmitsViewerEvent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check alerts"), 0644)

	listener := &recordingEventListener{}
	agent := &mockChatAgent{response: "Disk usage is 95%"}
	svc := NewHeartbeatService(agent, &mockSender{}, dir, 30).WithEventListener(listener)

	if err := svc.tick(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listener.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(listener.events))
	}
	ev := listener.events[0]
	if ev.Type != "heartbeat.notify" {
		t.Fatalf("expected heartbeat.notify, got %+v", ev)
	}
	if ev.Content != "Disk usage is 95%" {
		t.Fatalf("unexpected event content: %q", ev.Content)
	}
}

func TestTick_MissingFileEmitsViewerSkipEvent(t *testing.T) {
	dir := t.TempDir()
	listener := &recordingEventListener{}
	svc := NewHeartbeatService(&mockChatAgent{response: "HEARTBEAT_OK"}, &mockSender{}, dir, 30).WithEventListener(listener)

	if err := svc.tick(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listener.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(listener.events))
	}
	if listener.events[0].Type != "heartbeat.skip" {
		t.Fatalf("expected heartbeat.skip, got %+v", listener.events[0])
	}
}

func TestTick_HeartbeatOK(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check system status"), 0644)

	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !agent.called {
		t.Error("expected agent.Chat to be called")
	}
	if len(sender.messages) != 0 {
		t.Errorf("expected no notification, got %d", len(sender.messages))
	}

	logData, _ := os.ReadFile(filepath.Join(dir, "heartbeat.log"))
	if !strings.Contains(string(logData), "[OK]") {
		t.Error("expected [OK] in heartbeat.log")
	}
}

func TestTick_Notification(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check alerts"), 0644)

	agent := &mockChatAgent{response: "Disk usage is 95%"}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(sender.messages))
	}
	if sender.messages[0] != "Disk usage is 95%" {
		t.Errorf("expected 'Disk usage is 95%%', got %q", sender.messages[0])
	}
}

func TestTick_NoFile(t *testing.T) {
	dir := t.TempDir()

	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agent.called {
		t.Error("expected agent.Chat NOT to be called when file is missing")
	}
}

func TestTick_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("   \n  "), 0644)

	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agent.called {
		t.Error("expected agent.Chat NOT to be called for empty file")
	}
}

func TestTick_ChatError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check"), 0644)

	agent := &mockChatAgent{err: context.DeadlineExceeded}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, dir, 30)

	err := svc.tick(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "chat failed") {
		t.Errorf("expected 'chat failed' error, got: %v", err)
	}
}

func TestRunDueWorkstreamHeartbeatsCreatesDraftReportAndPendingVaultUpdate(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryWorkstreamHeartbeatStore{
		schedules: []domainworkstream.HeartbeatSchedule{{
			HeartbeatID:  "hb_revenue_daily",
			WorkstreamID: "ws_revenue",
			ScheduleText: "daily 08:00",
			Task:         "昨日の投稿反応を確認する",
			Status:       domainworkstream.StatusActive,
			NextRunAt:    now.Add(-time.Minute),
			CreatedAt:    now.Add(-24 * time.Hour),
		}},
	}
	listener := &recordingEventListener{}
	agent := &mockChatAgent{response: "draft report body"}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, dir, 30).
		WithWorkstreamStore(store).
		WithEventListener(listener)

	report, err := svc.RunDueWorkstreamHeartbeats(context.Background(), now)
	if err != nil {
		t.Fatalf("RunDueWorkstreamHeartbeats failed: %v", err)
	}
	if report.Checked != 1 || report.Run != 1 || report.Failed != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("workstream heartbeat must be draft-only, sent notifications: %#v", sender.messages)
	}
	if len(store.vaultUpdates) != 1 {
		t.Fatalf("expected pending vault update, got %#v", store.vaultUpdates)
	}
	if store.vaultUpdates[0].ReviewStatus != "pending" || store.vaultUpdates[0].UpdateType != "heartbeat_draft_report" {
		t.Fatalf("unexpected vault update: %#v", store.vaultUpdates[0])
	}
	body, err := os.ReadFile(store.vaultUpdates[0].FilePath)
	if err != nil {
		t.Fatalf("read draft report: %v", err)
	}
	if !strings.Contains(string(body), "draft report body") || !strings.Contains(string(body), "昨日の投稿反応を確認する") {
		t.Fatalf("unexpected draft report body: %s", string(body))
	}
	if len(store.saved) != 1 || store.saved[0].LastRunAt.IsZero() || !store.saved[0].NextRunAt.After(now) {
		t.Fatalf("expected updated schedule with next run, got %#v", store.saved)
	}
	if !agent.called || !strings.Contains(agent.lastMsg, "draft report only") {
		t.Fatalf("expected draft-only task sent to chat agent, got called=%v msg=%q", agent.called, agent.lastMsg)
	}
}

func TestRunDueWorkstreamHeartbeatsCreatesRevenueDailyRoutineDraftReport(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	workstreamStore := &memoryWorkstreamHeartbeatStore{
		schedules: []domainworkstream.HeartbeatSchedule{{
			HeartbeatID:  "hb_revenue_daily",
			WorkstreamID: "ws_revenue",
			ScheduleText: "daily 08:00",
			Task:         "収益化の日次ルーチンとして市場調査と顧客の声を確認する",
			Status:       domainworkstream.StatusActive,
			NextRunAt:    now.Add(-time.Minute),
			CreatedAt:    now.Add(-24 * time.Hour),
		}},
	}
	revenueStore := &memoryRevenueDailyRoutineStore{
		market: []domainrevenue.MarketResearchItem{{ItemID: "mkt_1", SourcePlatform: "note"}},
		posts:  []domainrevenue.SNSPostMetric{{PostID: "post_1", Platform: "x"}},
		products: []domainrevenue.Product{{
			ProductID:   "prod_1",
			ProductName: "商品設計シート",
			Status:      "draft",
		}},
		voices:    []domainrevenue.CustomerVoice{{VoiceID: "voice_1", RawText: "ここがわからない", PermissionStatus: "unknown"}},
		events:    []domainrevenue.RevenueEvent{{EventID: "rev_1", EventType: "purchase", Amount: 980, CustomerID: "cust_1"}},
		decisions: []domainrevenue.HumanDecisionGateRecord{{DecisionID: "dec_1", DecisionType: "external_publish", ApprovalStatus: "pending", GateStatus: "needs_review"}},
	}
	sender := &mockSender{}
	svc := NewHeartbeatService(&mockChatAgent{response: "revenue draft"}, sender, dir, 30).
		WithWorkstreamStore(workstreamStore).
		WithRevenueDailyRoutineStore(revenueStore)

	report, err := svc.RunDueWorkstreamHeartbeats(context.Background(), now)
	if err != nil {
		t.Fatalf("RunDueWorkstreamHeartbeats failed: %v", err)
	}
	if report.Run != 1 {
		t.Fatalf("unexpected run report: %+v", report)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("revenue heartbeat must not send external notifications: %#v", sender.messages)
	}
	if len(revenueStore.reports) != 1 {
		t.Fatalf("expected revenue daily routine report, got %#v", revenueStore.reports)
	}
	daily := revenueStore.reports[0]
	if daily.Status != "draft_report" || daily.ExternalSendApplied {
		t.Fatalf("expected draft-only revenue report: %#v", daily)
	}
	if daily.WorkstreamID != "ws_revenue" || daily.MarketResearch != 1 || daily.SNSPosts != 1 || daily.Products != 1 || daily.CustomerVoices != 1 || daily.RevenueEvents != 1 || daily.PaidCustomers != 1 || daily.PendingDecisions != 1 {
		t.Fatalf("unexpected revenue daily routine report: %#v", daily)
	}
}

func TestRunDueWorkstreamHeartbeatsRecordsSkillBootstrap(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	workstreamStore := &memoryWorkstreamHeartbeatStore{
		schedules: []domainworkstream.HeartbeatSchedule{{
			HeartbeatID:  "hb_1",
			WorkstreamID: "ws_1",
			ScheduleText: "daily 08:00",
			Task:         "作業ログを確認する",
			Status:       domainworkstream.StatusActive,
			NextRunAt:    now.Add(-time.Minute),
			CreatedAt:    now,
		}},
	}
	skillStore := &heartbeatSkillStore{
		manifests: []domainskill.SkillManifest{{
			SkillID:        "core.workstream-heartbeat",
			Enabled:        true,
			IntentTriggers: []string{"workstream_heartbeat"},
		}},
	}
	skills := skillbootstrap.NewBootstrapService(skillStore).WithNow(func() time.Time { return now })
	svc := NewHeartbeatService(&mockChatAgent{response: "draft"}, &mockSender{}, dir, 30).
		WithWorkstreamStore(workstreamStore).
		WithSkillBootstrap(skills)

	if _, err := svc.RunDueWorkstreamHeartbeats(context.Background(), now); err != nil {
		t.Fatalf("RunDueWorkstreamHeartbeats failed: %v", err)
	}
	if len(skillStore.logs) != 1 {
		t.Fatalf("expected skill bootstrap log, got %#v", skillStore.logs)
	}
	if skillStore.logs[0].SkillID != "core.workstream-heartbeat" || skillStore.logs[0].Status != domainskill.TriggerStatusTriggered {
		t.Fatalf("unexpected skill log: %#v", skillStore.logs[0])
	}
	if skillStore.logs[0].WorkstreamID != "ws_1" {
		t.Fatalf("expected workstream id in skill log, got %#v", skillStore.logs[0])
	}
}

func TestRunDueWorkstreamHeartbeatsAppliesPendingSteeringAtSafeCheckpoint(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryWorkstreamHeartbeatStore{
		schedules: []domainworkstream.HeartbeatSchedule{{
			HeartbeatID:  "hb_1",
			WorkstreamID: "ws_1",
			ScheduleText: "daily 08:00",
			Task:         "作業ログを確認する",
			Status:       domainworkstream.StatusActive,
			NextRunAt:    now.Add(-time.Minute),
			CreatedAt:    now,
		}},
		steering: []domainworkstream.SteeringItem{
			{
				SteeringID:       "stq_1",
				WorkstreamID:     "ws_1",
				TargetArtifactID: "art_1",
				Instruction:      "見出しを具体化する",
				Status:           "pending",
				CreatedAt:        now.Add(-time.Hour),
			},
			{
				SteeringID:   "stq_other",
				WorkstreamID: "ws_other",
				Instruction:  "別workstream",
				Status:       "pending",
				CreatedAt:    now.Add(-time.Hour),
			},
		},
	}
	agent := &mockChatAgent{response: "draft"}
	svc := NewHeartbeatService(agent, &mockSender{}, dir, 30).WithWorkstreamStore(store)

	if _, err := svc.RunDueWorkstreamHeartbeats(context.Background(), now); err != nil {
		t.Fatalf("RunDueWorkstreamHeartbeats failed: %v", err)
	}
	if !strings.Contains(agent.lastMsg, "stq_1 [art_1]: 見出しを具体化する") {
		t.Fatalf("expected pending steering in prompt, got %q", agent.lastMsg)
	}
	if strings.Contains(agent.lastMsg, "stq_other") {
		t.Fatalf("other workstream steering leaked into prompt: %q", agent.lastMsg)
	}
	if len(store.savedSteering) != 1 {
		t.Fatalf("expected one applied steering, got %#v", store.savedSteering)
	}
	applied := store.savedSteering[0]
	if applied.SteeringID != "stq_1" || applied.Status != "applied" || applied.AppliedAt.IsZero() {
		t.Fatalf("unexpected applied steering: %#v", applied)
	}
}

func TestRunDueWorkstreamHeartbeatsSkipsInactiveOrFutureSchedules(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryWorkstreamHeartbeatStore{
		schedules: []domainworkstream.HeartbeatSchedule{
			{
				HeartbeatID:  "hb_paused",
				WorkstreamID: "ws_1",
				ScheduleText: "daily 08:00",
				Task:         "paused",
				Status:       domainworkstream.StatusPaused,
				NextRunAt:    now.Add(-time.Minute),
				CreatedAt:    now,
			},
			{
				HeartbeatID:  "hb_future",
				WorkstreamID: "ws_1",
				ScheduleText: "daily 08:00",
				Task:         "future",
				Status:       domainworkstream.StatusActive,
				NextRunAt:    now.Add(time.Hour),
				CreatedAt:    now,
			},
		},
	}
	agent := &mockChatAgent{response: "should not run"}
	svc := NewHeartbeatService(agent, &mockSender{}, t.TempDir(), 30).WithWorkstreamStore(store)

	report, err := svc.RunDueWorkstreamHeartbeats(context.Background(), now)
	if err != nil {
		t.Fatalf("RunDueWorkstreamHeartbeats failed: %v", err)
	}
	if report.Checked != 2 || report.Run != 0 || report.Skipped != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if agent.called {
		t.Fatal("expected no chat call for skipped schedules")
	}
}

func TestTick_NilSender(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check"), 0644)

	agent := &mockChatAgent{response: "Alert: something is wrong"}
	svc := NewHeartbeatService(agent, nil, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartStop(t *testing.T) {
	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, t.TempDir(), 5)

	svc.Start()
	svc.Start() // 二重起動しないこと

	time.Sleep(50 * time.Millisecond)
	svc.Stop()
	svc.Stop() // 二重停止しないこと
}

func TestContextBuilder_WithWorkspaceFiles(t *testing.T) {
	dir := t.TempDir()

	// workspace ファイル群を作成
	os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("Agent rules here"), 0644)
	os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("Soul values here"), 0644)
	os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte("Identity info"), 0644)
	os.WriteFile(filepath.Join(dir, "USER.md"), []byte("User prefs"), 0644)
	os.WriteFile(filepath.Join(dir, "CHAT_PERSONA.md"), []byte("Mio persona"), 0644)

	// skills
	os.MkdirAll(filepath.Join(dir, "skills", "weather"), 0755)
	os.WriteFile(filepath.Join(dir, "skills", "weather", "SKILL.md"), []byte("# Weather lookup"), 0644)

	svc := NewHeartbeatService(&mockChatAgent{}, &mockSender{}, dir, 30)

	// tick 経由で ContextBuilder が使われることを確認
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check system status"), 0644)
	agent := svc.chatAgent.(*mockChatAgent)
	agent.response = "HEARTBEAT_OK"

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := agent.lastMsg

	// workspace コンテキストが含まれること（ContextBuilder は CHAT ルートで ChatOnly も含む）
	if !strings.Contains(msg, "# AGENT\nAgent rules here") {
		t.Error("expected AGENT.md content")
	}
	if !strings.Contains(msg, "# SOUL\nSoul values here") {
		t.Error("expected SOUL.md content")
	}
	if !strings.Contains(msg, "# IDENTITY\nIdentity info") {
		t.Error("expected IDENTITY.md content")
	}
	if !strings.Contains(msg, "weather: Weather lookup") {
		t.Error("expected skills summary")
	}

	// HEARTBEAT タスクが末尾にあること
	if !strings.Contains(msg, "# HEARTBEAT TASKS\nCheck system status") {
		t.Error("expected HEARTBEAT TASKS section")
	}

	// コンテキストとタスクの区切り
	if !strings.Contains(msg, "===") {
		t.Error("expected separator between context and tasks")
	}
}

func TestContextBuilder_NoWorkspaceFiles(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check system status"), 0644)

	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	svc := NewHeartbeatService(agent, &mockSender{}, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// workspace ファイルがなければ HEARTBEAT タスクのみ
	if agent.lastMsg != "Check system status" {
		t.Errorf("expected plain heartbeat content, got: %q", agent.lastMsg)
	}
}

func TestTick_WithWorkspaceContext(t *testing.T) {
	dir := t.TempDir()

	// workspace + HEARTBEAT.md
	os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("Be concise"), 0644)
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check alerts"), 0644)

	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	svc := NewHeartbeatService(agent, &mockSender{}, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// MioAgentに送信されたメッセージにworkspaceコンテキストが含まれること
	if !strings.Contains(agent.lastMsg, "# AGENT\nBe concise") {
		t.Error("expected workspace context in message sent to agent")
	}
	if !strings.Contains(agent.lastMsg, "# HEARTBEAT TASKS\nCheck alerts") {
		t.Error("expected heartbeat tasks in message sent to agent")
	}
}
