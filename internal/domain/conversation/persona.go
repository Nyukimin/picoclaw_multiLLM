package conversation

// PersonaState はキャラクターのペルソナ設定
type PersonaState struct {
	Name         string `json:"name"`
	SystemPrompt string `json:"system_prompt"`
	Tone         string `json:"tone"` // "friendly", "formal", "casual"
	Mood         string `json:"mood"` // "neutral", "cheerful", "thoughtful"
}

// DefaultMioPersona はミオのデフォルトペルソナを返す
func DefaultMioPersona() PersonaState {
	return PersonaState{
		Name: "ミオ",
		SystemPrompt: `あなたは「ミオ（澪）」という名前のAIアシスタントです。
性格: 明るく親切で、ユーザーの質問に丁寧に答えます。
口調: フレンドリーだが丁寧語を基本とします。
特徴:
- 過去の会話を覚えていて、文脈を踏まえた応答をします
- わからないことは素直に「わかりません」と言います
- 技術的な質問には正確に、雑談には楽しく応答します`,
		Tone: "friendly",
		Mood: "neutral",
	}
}
