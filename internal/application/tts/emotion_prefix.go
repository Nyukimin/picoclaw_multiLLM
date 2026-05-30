package ttsapp

import "strings"

type EmojiPaletteItem struct {
	Emoji       string
	Label       string
	Instruction string
}

var EmotionEmojiPaletteItems = []EmojiPaletteItem{
	{Emoji: "😊", Label: "明るい", Instruction: "自然に嬉しそうに"},
	{Emoji: "😆", Label: "喜び", Instruction: "弾むように嬉しく"},
	{Emoji: "🥰", Label: "親しみ", Instruction: "あたたかく好意的に"},
	{Emoji: "🫶", Label: "優しく", Instruction: "やわらかく丁寧に"},
	{Emoji: "😌", Label: "安堵", Instruction: "落ち着いて満足げに"},
	{Emoji: "😇", Label: "穏やか", Instruction: "静かで澄んだ雰囲気"},
	{Emoji: "😎", Label: "自信", Instruction: "余裕のある口調で"},
	{Emoji: "😏", Label: "からかう", Instruction: "少し得意げに"},
	{Emoji: "🤭", Label: "含み笑い", Instruction: "こらえた笑いを混ぜて"},
	{Emoji: "🤔", Label: "考え中", Instruction: "迷いながら考えるように"},
	{Emoji: "😮", Label: "感嘆", Instruction: "はっと驚いて"},
	{Emoji: "😲", Label: "驚き", Instruction: "大きく驚いて"},
	{Emoji: "😳", Label: "動揺", Instruction: "戸惑いを含めて"},
	{Emoji: "🫣", Label: "照れ", Instruction: "恥ずかしそうに"},
	{Emoji: "🥺", Label: "不安", Instruction: "弱々しく頼りなげに"},
	{Emoji: "😟", Label: "心配", Instruction: "不安そうに"},
	{Emoji: "😰", Label: "焦り", Instruction: "慌てて緊張気味に"},
	{Emoji: "😨", Label: "恐れ", Instruction: "怖がるように"},
	{Emoji: "😭", Label: "悲しみ", Instruction: "泣きそうに"},
	{Emoji: "😢", Label: "寂しさ", Instruction: "静かに悲しく"},
	{Emoji: "😞", Label: "落胆", Instruction: "しょんぼりと"},
	{Emoji: "😖", Label: "苦しげ", Instruction: "つらそうに"},
	{Emoji: "😠", Label: "怒り", Instruction: "強く不満げに"},
	{Emoji: "😤", Label: "不満", Instruction: "むっとして"},
	{Emoji: "😒", Label: "呆れ", Instruction: "冷めた調子で"},
	{Emoji: "🙄", Label: "うんざり", Instruction: "あきれたように"},
	{Emoji: "😪", Label: "眠そう", Instruction: "気だるく"},
	{Emoji: "🥱", Label: "退屈", Instruction: "だるそうに"},
	{Emoji: "🥴", Label: "酔い", Instruction: "ふらついた雰囲気で"},
	{Emoji: "💪", Label: "力強く", Instruction: "力を込めて"},
	{Emoji: "💥", Label: "勢い", Instruction: "勢いよくはっきりと"},
	{Emoji: "🙏", Label: "お願い", Instruction: "懇願するように"},
	{Emoji: "🤫", Label: "静かに", Instruction: "ひそやかに"},
	{Emoji: "📖", Label: "朗読", Instruction: "落ち着いたナレーション"},
}

func EnsureEmotionPrefix(text string, emotion *EmotionState) string {
	return EnsureEmotionPrefixForCharacter(text, emotion, "")
}

func EnsureEmotionPrefixForCharacter(text string, emotion *EmotionState, characterID string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if HasEmotionPrefix(trimmed) {
		return trimmed
	}
	return emotionPrefixForCharacterText(emotion, characterID, trimmed) + trimmed
}

func HasEmotionPrefix(text string) bool {
	trimmed := strings.TrimSpace(text)
	for _, item := range EmotionEmojiPaletteItems {
		if item.Emoji != "" && strings.HasPrefix(trimmed, item.Emoji) {
			return true
		}
	}
	return false
}

func emotionPrefixForState(emotion *EmotionState) string {
	return emotionPrefixForCharacterText(emotion, "", "")
}

func emotionPrefixForCharacterText(emotion *EmotionState, characterID, text string) string {
	if isIdleChatTopicSpeechText(text) {
		return "😊"
	}
	feature := classifyEmotionText(text)
	speaker := strings.ToLower(strings.TrimSpace(characterID))
	switch speaker {
	case "mio", "female_01", "female_01_mio":
		if feature != "" {
			return prefixForFeature(feature, true)
		}
		if fromState := emotionPrefixForMioStateOnly(emotion); fromState != "" && fromState != "😌" && fromState != "😇" {
			return fromState
		}
		return "😊"
	case "shiro", "male_01", "male":
		if feature != "" && isStrongEmotionFeature(feature, text) {
			return prefixForFeature(feature, false)
		}
		if fromState := emotionPrefixForStateOnly(emotion); fromState == "😰" || fromState == "😮" || fromState == "💥" {
			return fromState
		}
		return "😇"
	default:
		if feature != "" {
			return prefixForFeature(feature, false)
		}
		return emotionPrefixForStateOnly(emotion)
	}
}

func isIdleChatTopicSpeechText(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(trimmed, "きょうのおだい")
}

func emotionPrefixForStateOnly(emotion *EmotionState) string {
	if emotion == nil {
		return "😌"
	}
	primary := strings.ToLower(strings.TrimSpace(emotion.PrimaryEmotion))
	if primary == "" {
		return "😌"
	}
	v := emotion.EmotionVector
	switch primary {
	case "alert":
		if v.Alertness >= 0.78 {
			return "😰"
		}
		return "😮"
	case "serious":
		return "🤔"
	case "cheerful":
		if v.Cheerfulness >= 0.72 {
			return "😆"
		}
		return "😊"
	case "warm":
		if v.Warmth >= 0.70 {
			return "🥰"
		}
		return "🫶"
	case "calm":
		return "😌"
	default:
		return "📖"
	}
}

func emotionPrefixForMioStateOnly(emotion *EmotionState) string {
	if emotion == nil {
		return "😊"
	}
	primary := strings.ToLower(strings.TrimSpace(emotion.PrimaryEmotion))
	if primary == "" {
		return "😊"
	}
	v := emotion.EmotionVector
	switch primary {
	case "alert":
		if v.Alertness >= 0.78 {
			return "😰"
		}
		return "😮"
	case "serious":
		return "🤔"
	case "cheerful":
		if v.Cheerfulness >= 0.78 {
			return "😆"
		}
		return "😊"
	case "warm":
		return "😊"
	case "calm":
		return "😊"
	default:
		return "😊"
	}
}

func classifyEmotionText(text string) string {
	lower := strings.ToLower(text)
	switch {
	case containsAny(lower, "ありがとう", "嬉しい", "楽しい", "最高", "よかった", "すごい", "素敵", "いいね", "やった", "成功", "できました", "完了", "thank"):
		return "joy"
	case containsAny(lower, "好き", "かわいい", "大切", "親し"):
		return "affection"
	case containsAny(lower, "ごめん", "すみません", "申し訳", "お願い", "頼む", "助けて"):
		return "plead"
	case containsAny(lower, "なぜ", "どうして", "考え", "迷", "かもしれ", "おそらく", "たぶん", "一方で", "ただし"):
		return "thinking"
	case containsAny(lower, "驚", "びっくり", "まさか", "はっと", "えっ", "!?"):
		return "surprise"
	case containsAny(lower, "怖", "恐", "不安", "心配", "危険", "緊張"):
		return "fear"
	case containsAny(lower, "急", "焦", "まずい", "大変", "すぐ", "警告", "注意", "エラー", "失敗"):
		return "alert"
	case containsAny(lower, "悲しい", "寂しい", "つらい", "泣", "落胆", "しょんぼり"):
		return "sad"
	case containsAny(lower, "怒", "不満", "むっと", "許せ", "ひどい"):
		return "anger"
	case containsAny(lower, "呆", "あきれ", "うんざり", "退屈", "だる"):
		return "tired"
	case containsAny(lower, "静か", "内緒", "ひそ", "そっと"):
		return "quiet"
	case containsAny(lower, "!", "！"):
		return "energy"
	default:
		return ""
	}
}

func prefixForFeature(feature string, expressive bool) string {
	switch feature {
	case "joy":
		if expressive {
			return "😆"
		}
		return "😊"
	case "affection":
		if expressive {
			return "🥰"
		}
		return "🫶"
	case "plead":
		return "🙏"
	case "thinking":
		return "🤔"
	case "surprise":
		if expressive {
			return "😲"
		}
		return "😮"
	case "fear":
		if expressive {
			return "😨"
		}
		return "😟"
	case "alert":
		if expressive {
			return "😰"
		}
		return "😮"
	case "sad":
		if expressive {
			return "😭"
		}
		return "😢"
	case "anger":
		if expressive {
			return "😠"
		}
		return "😤"
	case "tired":
		if expressive {
			return "🙄"
		}
		return "😒"
	case "quiet":
		return "🤫"
	case "energy":
		if expressive {
			return "💥"
		}
		return "💪"
	default:
		return "😌"
	}
}

func isStrongEmotionFeature(feature, text string) bool {
	switch feature {
	case "surprise", "fear", "alert", "sad", "anger", "energy":
		return true
	case "joy":
		return containsAny(text, "最高", "やった", "すごい", "！", "!")
	case "affection":
		return containsAny(text, "大切", "好き", "安心")
	case "plead":
		return containsAny(text, "お願い", "助けて", "頼む")
	default:
		return false
	}
}
