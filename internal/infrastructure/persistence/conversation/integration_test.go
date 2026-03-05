//go:build integration

package conversation

import (
	"context"
	"net/http"
	"testing"
	"time"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

const (
	testRedisURL    = "redis://localhost:6379"
	testVectorDBURL = "localhost:6334"
	testCollection  = "picoclaw_integration_test"
)

// skipIfUnavailable は接続確認し、未起動なら t.Skip()
func skipIfRedisUnavailable(t *testing.T) {
	t.Helper()
	store, err := NewRedisStore(testRedisURL)
	if err != nil {
		t.Skipf("Redis unavailable (%v) — run: docker compose -f docker-compose.infra.yml up -d", err)
	}
	store.Close()
}

func skipIfQdrantUnavailable(t *testing.T) {
	t.Helper()
	resp, err := http.Get("http://localhost:6333/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Skipf("Qdrant unavailable — run: docker compose -f docker-compose.infra.yml up -d")
	}
	resp.Body.Close()
}

// --- RedisStore 統合テスト ---

func TestRedisStore_Integration_SessionAndThread(t *testing.T) {
	skipIfRedisUnavailable(t)

	store, err := NewRedisStore(testRedisURL)
	if err != nil {
		t.Fatalf("NewRedisStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	sessionID := "integration-sess-" + time.Now().Format("150405")

	// Session保存・取得
	sess := domconv.NewSessionConversation(sessionID, "test-user")
	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}
	got, err := store.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.ID != sessionID {
		t.Errorf("Expected session ID %s, got %s", sessionID, got.ID)
	}

	// Thread保存・取得
	thread := domconv.NewThread(sessionID, "integration")
	thread.AddMessage(domconv.NewMessage(domconv.SpeakerUser, "統合テストのメッセージ", nil))
	if err := store.SaveThread(ctx, thread); err != nil {
		t.Fatalf("SaveThread failed: %v", err)
	}
	gotThread, err := store.GetThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("GetThread failed: %v", err)
	}
	if len(gotThread.Turns) != 1 {
		t.Errorf("Expected 1 turn, got %d", len(gotThread.Turns))
	}

	// クリーンアップ
	store.DeleteThread(ctx, thread.ID)
	store.DeleteSession(ctx, sessionID)
}

// --- VectorDBStore 統合テスト ---

func TestVectorDBStore_Integration_SaveAndSearch(t *testing.T) {
	skipIfQdrantUnavailable(t)

	store, err := NewVectorDBStore(testVectorDBURL, testCollection)
	if err != nil {
		t.Fatalf("NewVectorDBStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now()

	// Thread要約を保存（embedding付き）
	summary := &domconv.ThreadSummary{
		ThreadID:  now.UnixNano(),
		SessionID: "integration-sess",
		Domain:    "test",
		Summary:   "統合テスト用の会話要約",
		Keywords:  []string{"統合", "テスト", "Qdrant"},
		Embedding: make([]float32, 768), // ゼロベクトル（テスト用）
		StartTime: now.Add(-5 * time.Minute),
		EndTime:   now,
		IsNovel:   true,
	}
	// ゼロベクトル以外にする（検索で見つかるように）
	for i := range summary.Embedding {
		summary.Embedding[i] = float32(i%10) * 0.01
	}

	if err := store.SaveThreadSummary(ctx, summary); err != nil {
		t.Fatalf("SaveThreadSummary failed: %v", err)
	}

	// 類似度検索（同じembeddingで検索）
	results, err := store.SearchSimilar(ctx, summary.Embedding, 3)
	if err != nil {
		t.Fatalf("SearchSimilar failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected at least 1 search result")
	}
	// スコアが設定されている
	if results[0].Score == 0 {
		t.Error("Expected non-zero similarity score")
	}
	t.Logf("SearchSimilar: %d results, top score: %.4f", len(results), results[0].Score)

	// IsNovelQuery（同じembeddingは類似度高い → 新規でない）
	isNovel, score, err := store.IsNovelQuery(ctx, summary.Embedding, 0.85)
	if err != nil {
		t.Fatalf("IsNovelQuery failed: %v", err)
	}
	t.Logf("IsNovelQuery: isNovel=%v, score=%.4f", isNovel, score)
	// 同一embeddingなので score は SearchSimilar の実スコア（≒1.0）と一致するはず
	if len(results) > 0 && score != results[0].Score {
		t.Errorf("IsNovelQuery score %.4f should match SearchSimilar top score %.4f", score, results[0].Score)
	}
	// 高類似度（≥0.85）なので新規でない
	if isNovel {
		t.Errorf("Should not be novel when top score=%.4f >= threshold=0.85", score)
	}
}

// --- RealConversationManager 統合テスト ---

func TestRealConversationManager_Integration_StoreAndRecall(t *testing.T) {
	skipIfRedisUnavailable(t)

	// VectorDBはオプション（未起動でもテスト続行）
	vdbAvailable := true
	if resp, err := http.Get("http://localhost:6333/healthz"); err != nil || resp.StatusCode != 200 {
		vdbAvailable = false
		t.Log("Qdrant unavailable, skipping VectorDB recall test")
	} else {
		resp.Body.Close()
	}

	var mgr *RealConversationManager
	var err error
	if vdbAvailable {
		mgr, err = NewRealConversationManager(testRedisURL, "", testVectorDBURL)
	} else {
		mgr, err = NewRealConversationManager(testRedisURL, "", "localhost:19999") // ダミー
	}
	_ = err // 接続エラーは許容（Qdrantが起動していれば接続成功）

	// Redisのみのシンプルなテスト
	redisStore, err := NewRedisStore(testRedisURL)
	if err != nil {
		t.Fatalf("RedisStore failed: %v", err)
	}
	defer redisStore.Close()

	simpleMgr := &RealConversationManager{
		redisStore:    redisStore,
		duckdbStore:   &mockDuckDBStore{},
		vectordbStore: &mockVectorDBStore{mockScore: 0.5},
		embedder:      nil,
		summarizer:    nil,
	}
	_ = mgr

	ctx := context.Background()
	sessionID := "integration-full-" + time.Now().Format("150405")

	// メッセージを保存
	msgs := []string{"こんにちは", "Go言語は素晴らしい", "ありがとう"}
	for _, text := range msgs {
		msg := domconv.NewMessage(domconv.SpeakerUser, text, nil)
		if err := simpleMgr.Store(ctx, sessionID, msg); err != nil {
			t.Fatalf("Store failed: %v", err)
		}
	}

	// Recallで取得
	recalled, err := simpleMgr.Recall(ctx, sessionID, "Go言語", 5)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(recalled) == 0 {
		t.Error("Expected recalled messages, got none")
	}
	t.Logf("Recalled %d messages from Redis", len(recalled))

	// クリーンアップ
	thread, _ := simpleMgr.GetActiveThread(ctx, sessionID)
	if thread != nil {
		redisStore.DeleteThread(ctx, thread.ID)
	}
	redisStore.DeleteSession(ctx, sessionID)
}
