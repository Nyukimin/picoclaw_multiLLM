package conversation

import "context"

// ConversationEngine は Mio の「記憶脳」
// ConversationManager をラップし、RecallPack 生成と Thread 管理を提供
type ConversationEngine interface {
	// BeginTurn はターン開始時に呼び出し、RecallPack を返す
	// 1. Recall（想起）
	// 2. RecallPack 構築（Persona + UserProfile 含む）
	BeginTurn(ctx context.Context, sessionID string, userMessage string) (*RecallPack, error)

	// EndTurn はターン終了時に呼び出し、メッセージ保存を実行
	// 1. ユーザーメッセージの Store
	// 2. 応答の Store
	EndTurn(ctx context.Context, sessionID string, userMessage string, response string) error

	// GetPersona は現在のペルソナ設定を返す
	GetPersona() PersonaState
}
