package conversation

import "time"

// ThreadStatus はThreadの状態
type ThreadStatus string

const (
	ThreadActive   ThreadStatus = "active"
	ThreadClosed   ThreadStatus = "closed"
	ThreadArchived ThreadStatus = "archived"
)

// Thread は「話題のまとまり」（6〜8ターン相当）
type Thread struct {
	ID        int64        `json:"thread_id"`
	SessionID string       `json:"session_id"`
	Domain    string       `json:"domain"`
	Turns     []Message    `json:"turns"`
	Targets   []string     `json:"targets"`
	Cooldown  map[string]int `json:"ct"`
	StartTime time.Time    `json:"ts_start"`
	EndTime   *time.Time   `json:"ts_end,omitempty"`
	Status    ThreadStatus `json:"status"`
}

// NewThread は新しいThreadを生成
func NewThread(sessionID string, domain string) *Thread {
	return &Thread{
		ID:        generateThreadID(),
		SessionID: sessionID,
		Domain:    domain,
		Turns:     make([]Message, 0, 12),
		Targets:   []string{},
		Cooldown:  make(map[string]int),
		StartTime: time.Now(),
		Status:    ThreadActive,
	}
}

// AddMessage はThreadにMessageを追加（最大12件保持）
func (t *Thread) AddMessage(msg Message) {
	t.Turns = append(t.Turns, msg)
	if len(t.Turns) > 12 {
		t.Turns = t.Turns[len(t.Turns)-12:]
	}
}

// Close はThreadを終了
func (t *Thread) Close() {
	now := time.Now()
	t.EndTime = &now
	t.Status = ThreadClosed
}

// generateThreadID はユニークなThread IDを生成（簡易実装）
func generateThreadID() int64 {
	return time.Now().UnixNano()
}
