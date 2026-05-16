package main

import (
	"strings"
	"time"
)

const (
	idleChatDefaultVoiceID   = "mio"
	idleChatDefaultVoiceProf = "lumina_female"
	idleChatMaleVoiceID      = "male_01"
	idleChatMaleVoiceProf    = "lumina_male"
)

func idleChatVoiceForSpeaker(speaker string) (voiceID, voiceProfile string) {
	switch normalizeIdleChatCharacterID(speaker) {
	case "shiro":
		return idleChatMaleVoiceID, idleChatMaleVoiceProf
	default:
		return idleChatDefaultVoiceID, idleChatDefaultVoiceProf
	}
}

func normalizeIdleChatCharacterID(speaker string) string {
	switch strings.ToLower(strings.TrimSpace(speaker)) {
	case "shiro", "しろ":
		return "shiro"
	case "mio", "みお":
		return "mio"
	case "れん", "ren", "user":
		return "user"
	default:
		return strings.ToLower(strings.TrimSpace(speaker))
	}
}

func idleChatTimeOfDay() string {
	hour := time.Now().Hour()
	if hour < 6 || hour >= 21 {
		return "night"
	}
	return "day"
}
