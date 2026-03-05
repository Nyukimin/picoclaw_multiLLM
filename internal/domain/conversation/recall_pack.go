package conversation

import (
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

// PromptConstraints はプロンプト組み立ての制約
type PromptConstraints struct {
	MaxTotalTokens    int // LLM の MaxContext（デフォルト: 8192）
	MaxPromptTokens   int // プロンプトに使えるトークン（デフォルト: 4000）
	MaxResponseTokens int // 応答用トークン（デフォルト: 512）
}

// DefaultConstraints はデフォルトのトークン制約を返す
func DefaultConstraints() PromptConstraints {
	return PromptConstraints{
		MaxTotalTokens:    8192,
		MaxPromptTokens:   4000,
		MaxResponseTokens: 512,
	}
}

// RecallPack は Recall 結果を構造化した LLM プロンプト注入用フォーマット
type RecallPack struct {
	// ShortContext: 現在の Thread 内の直近メッセージ（最大12件）
	ShortContext []Message

	// MidSummaries: 同一セッション内の過去 Thread 要約（最大3件）
	MidSummaries []ThreadSummary

	// LongFacts: VectorDB から類似検索した過去の知識（最大3件）
	LongFacts []string

	// KBSnippets: ドメイン知識ベースからの関連情報（最大2件）
	KBSnippets []string

	// Persona: キャラクター設定
	Persona PersonaState

	// UserProfile: ユーザーの好み・傾向
	UserProfile UserProfile

	// Constraints: トークン上限等
	Constraints PromptConstraints
}

// HasContext は RecallPack に何らかの文脈があるかを返す
func (rp *RecallPack) HasContext() bool {
	return len(rp.ShortContext) > 0 ||
		len(rp.MidSummaries) > 0 ||
		len(rp.LongFacts) > 0 ||
		len(rp.KBSnippets) > 0
}

// ToPromptMessages は RecallPack を llm.Message のスライスに変換
// userMessage は含めない（呼び出し側で追加する）
func (rp *RecallPack) ToPromptMessages() []llm.Message {
	var messages []llm.Message

	// 1. システムプロンプト（Persona + UserProfile）
	systemPrompt := rp.Persona.SystemPrompt
	if profileText := rp.UserProfile.ToPromptText(); profileText != "" {
		systemPrompt += "\n\n" + profileText
	}
	if systemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// 2. 過去文脈（中期要約 + 長期事実 + KB）
	contextText := ""
	if len(rp.MidSummaries) > 0 {
		contextText += "【過去の会話から思い出したこと】\n"
		for _, s := range rp.MidSummaries {
			contextText += "- " + s.Summary + "\n"
		}
	}
	if len(rp.LongFacts) > 0 {
		if contextText == "" {
			contextText += "【過去の会話から思い出したこと】\n"
		}
		for _, f := range rp.LongFacts {
			contextText += "- " + f + "\n"
		}
	}
	if len(rp.KBSnippets) > 0 {
		contextText += "【参考知識】\n"
		for _, kb := range rp.KBSnippets {
			contextText += kb + "\n"
		}
	}
	if contextText != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: contextText,
		})
	}

	// 3. 直近の会話履歴（ShortContext）
	for _, msg := range rp.ShortContext {
		role := "user"
		switch msg.Speaker {
		case SpeakerMio:
			role = "assistant"
		case SpeakerUser:
			role = "user"
		default:
			role = "system"
		}
		messages = append(messages, llm.Message{
			Role:    role,
			Content: msg.Msg,
		})
	}

	return messages
}
