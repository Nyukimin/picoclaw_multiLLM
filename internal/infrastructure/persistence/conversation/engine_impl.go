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
	manager domconv.ConversationManager
	persona domconv.PersonaState
}

// NewRealConversationEngine は新しい ConversationEngine を作成
func NewRealConversationEngine(
	manager domconv.ConversationManager,
	persona domconv.PersonaState,
) *RealConversationEngine {
	return &RealConversationEngine{
		manager: manager,
		persona: persona,
	}
}

// BeginTurn はターン開始時に Recall + RecallPack 構築を実行
func (e *RealConversationEngine) BeginTurn(ctx context.Context, sessionID string, userMessage string) (*domconv.RecallPack, error) {
	pack := &domconv.RecallPack{
		Persona:     e.persona,
		Constraints: domconv.DefaultConstraints(),
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
func (e *RealConversationEngine) EndTurn(ctx context.Context, sessionID string, userMessage string, response string) error {
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

	return nil
}

// GetPersona は現在のペルソナ設定を返す
func (e *RealConversationEngine) GetPersona() domconv.PersonaState {
	return e.persona
}
