package conversation

import "time"

// AgentStatus はAgentのタスク状態を管理
type AgentStatus struct {
	AgentName      string    `json:"agent_name"`
	IsIdle         bool      `json:"is_idle"`
	LastTaskTime   time.Time `json:"last_task"`
	CurrentTask    string    `json:"current_task"`
	ConversationOK bool      `json:"conversation_ok"`
}

// NewAgentStatus は新しいAgentStatusを生成
func NewAgentStatus(agentName string) *AgentStatus {
	return &AgentStatus{
		AgentName:      agentName,
		IsIdle:         true,
		ConversationOK: agentName == "mio", // Mioは常時参加可能
	}
}

// CanJoinConversation は会話参加可能かを判定
func (s *AgentStatus) CanJoinConversation() bool {
	// Chat（Mio）は常時参加可能
	if s.AgentName == "mio" {
		return true
	}

	// Worker（Shiro）はタスクが空いている時のみ
	if s.AgentName == "shiro" {
		return s.IsIdle && s.CurrentTask == ""
	}

	// Coder（将来）: 条件未定
	return false
}
