package conversation

import (
	"context"
	"log"
	"strings"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

// RealConversationEngine は ConversationEngine の実装
// 既存の RealConversationManager をラップし、RecallPack 生成を追加
type RealConversationEngine struct {
	manager          domconv.ConversationManager
	persona          domconv.PersonaState
	detector         domconv.ThreadBoundaryDetector // nil の場合はスレッド自動検出無効
	profileExtractor domconv.ProfileExtractor       // nil の場合はプロファイル抽出無効
	profiles         map[string]domconv.UserProfile  // インメモリキャッシュ
}

// NewRealConversationEngine は新しい ConversationEngine を作成
func NewRealConversationEngine(
	manager domconv.ConversationManager,
	persona domconv.PersonaState,
) *RealConversationEngine {
	return &RealConversationEngine{
		manager:  manager,
		persona:  persona,
		profiles: make(map[string]domconv.UserProfile),
	}
}

// WithDetector はスレッド境界検出器を設定する（オプション）
func (e *RealConversationEngine) WithDetector(d domconv.ThreadBoundaryDetector) *RealConversationEngine {
	e.detector = d
	return e
}

// WithProfileExtractor はプロファイル抽出器を設定する（オプション）
func (e *RealConversationEngine) WithProfileExtractor(pe domconv.ProfileExtractor) *RealConversationEngine {
	e.profileExtractor = pe
	return e
}

// BeginTurn はターン開始時に Recall + RecallPack 構築を実行
func (e *RealConversationEngine) BeginTurn(ctx context.Context, sessionID string, userMessage string) (*domconv.RecallPack, error) {
	pack := &domconv.RecallPack{
		Persona:     e.persona,
		Constraints: domconv.DefaultConstraints(),
	}

	// UserProfile 読み込み
	if profile, ok := e.profiles[sessionID]; ok {
		pack.UserProfile = profile
	}

	// Recall（想起）
	recallMessages, err := e.manager.Recall(ctx, sessionID, userMessage, 3)
	if err != nil {
		log.Printf("[ConversationEngine] WARN: Recall failed: %v", err)
		return pack, nil
	}

	// Recall 結果を RecallPack に分類
	for _, msg := range recallMessages {
		switch {
		case msg.Speaker == domconv.SpeakerUser || msg.Speaker == domconv.SpeakerMio:
			// 短期記憶（Thread.Turns）: そのまま ShortContext に
			pack.ShortContext = append(pack.ShortContext, msg)

		case msg.Speaker == domconv.SpeakerSystem && strings.HasPrefix(msg.Msg, "[Summary]"):
			// 中期記憶（DuckDB ThreadSummary）: MidSummaries に変換
			summary := strings.TrimPrefix(msg.Msg, "[Summary] ")
			pack.MidSummaries = append(pack.MidSummaries, domconv.ThreadSummary{
				Summary: summary,
			})

		case msg.Speaker == domconv.SpeakerSystem && strings.HasPrefix(msg.Msg, "[LongTermMemory]"):
			// 長期記憶（VectorDB）: LongFacts に変換
			fact := strings.TrimPrefix(msg.Msg, "[LongTermMemory] ")
			pack.LongFacts = append(pack.LongFacts, fact)

		default:
			// その他のシステムメッセージは LongFacts に
			if msg.Msg != "" {
				pack.LongFacts = append(pack.LongFacts, msg.Msg)
			}
		}
	}

	return pack, nil
}

// EndTurn はターン終了時にメッセージ保存を実行
// スレッド境界検出器が設定されている場合、Store前にトピック変化を検出する
func (e *RealConversationEngine) EndTurn(ctx context.Context, sessionID string, userMessage string, response string) error {
	// スレッド境界検出（detector が設定されている場合）
	if e.detector != nil {
		thread, err := e.manager.GetActiveThread(ctx, sessionID)
		if err == nil && thread != nil {
			result := e.detector.Detect(thread, userMessage, "")
			if result.ShouldCreateNew {
				log.Printf("[ConversationEngine] Thread boundary detected: %s (score=%.2f)", result.Reason, result.Score)
				if _, err := e.manager.FlushThread(ctx, thread.ID); err != nil {
					log.Printf("[ConversationEngine] WARN: FlushThread failed: %v", err)
				}
				if _, err := e.manager.CreateThread(ctx, sessionID, thread.Domain); err != nil {
					log.Printf("[ConversationEngine] WARN: CreateThread failed: %v", err)
				}
			}
		}
	}

	// ユーザーメッセージを記憶
	userMsg := domconv.NewMessage(domconv.SpeakerUser, userMessage, nil)
	if err := e.manager.Store(ctx, sessionID, userMsg); err != nil {
		log.Printf("[ConversationEngine] WARN: Store (user) failed: %v", err)
	}

	// Mio の応答を記憶
	mioMsg := domconv.NewMessage(domconv.SpeakerMio, response, nil)
	if err := e.manager.Store(ctx, sessionID, mioMsg); err != nil {
		log.Printf("[ConversationEngine] WARN: Store (mio) failed: %v", err)
	}

	// UserProfile 自動抽出（best-effort）
	if e.profileExtractor != nil {
		thread, err := e.manager.GetActiveThread(ctx, sessionID)
		if err == nil && thread != nil {
			existing := e.profiles[sessionID]
			result, err := e.profileExtractor.Extract(ctx, thread, existing)
			if err != nil {
				log.Printf("[ConversationEngine] WARN: ProfileExtract failed: %v", err)
			} else if result != nil && result.HasData() {
				if existing.UserID == "" {
					existing = domconv.NewUserProfile(sessionID)
				}
				existing.Merge(result.NewPreferences, result.NewFacts)
				e.profiles[sessionID] = existing
				log.Printf("[ConversationEngine] UserProfile updated: +%d prefs, +%d facts",
					len(result.NewPreferences), len(result.NewFacts))
			}
		}
	}

	return nil
}

// GetPersona は現在のペルソナ設定を返す
func (e *RealConversationEngine) GetPersona() domconv.PersonaState {
	return e.persona
}

// FlushCurrentThread は現在のスレッドを強制フラッシュする
func (e *RealConversationEngine) FlushCurrentThread(ctx context.Context, sessionID string) error {
	thread, err := e.manager.GetActiveThread(ctx, sessionID)
	if err != nil {
		return err
	}
	if _, err := e.manager.FlushThread(ctx, thread.ID); err != nil {
		return err
	}
	_, err = e.manager.CreateThread(ctx, sessionID, thread.Domain)
	return err
}

// GetStatus は会話セッションの現在状態を返す
func (e *RealConversationEngine) GetStatus(ctx context.Context, sessionID string) (*domconv.ConversationStatus, error) {
	thread, err := e.manager.GetActiveThread(ctx, sessionID)
	if err != nil {
		return &domconv.ConversationStatus{
			SessionID: sessionID,
		}, nil
	}
	return &domconv.ConversationStatus{
		SessionID:    sessionID,
		ThreadID:     thread.ID,
		ThreadDomain: thread.Domain,
		TurnCount:    len(thread.Turns),
		ThreadStart:  thread.StartTime,
		ThreadStatus: thread.Status,
	}, nil
}

// ResetSession はセッションをリセットする
func (e *RealConversationEngine) ResetSession(ctx context.Context, sessionID string) error {
	thread, err := e.manager.GetActiveThread(ctx, sessionID)
	if err == nil && thread != nil {
		if _, err := e.manager.FlushThread(ctx, thread.ID); err != nil {
			log.Printf("[ConversationEngine] WARN: FlushThread during reset failed: %v", err)
		}
	}
	_, err = e.manager.CreateThread(ctx, sessionID, "general")
	return err
}
