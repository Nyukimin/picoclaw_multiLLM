package tts

import "github.com/Nyukimin/picoclaw_multiLLM/modules/core"

type ProviderHealthSnapshot struct {
	Provider string
	Ready    bool
	Detail   string
	Metadata map[string]any
}

func BuildProviderHealth(snapshot ProviderHealthSnapshot) core.HealthReport {
	if !snapshot.Ready {
		detail := snapshot.Detail
		if detail == "" {
			detail = "tts provider is nil"
		}
		return core.HealthReport{
			Module:   "tts",
			Status:   core.HealthDown,
			Detail:   detail,
			Metadata: snapshot.Metadata,
		}
	}
	metadata := snapshot.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["provider"] = snapshot.Provider
	return core.HealthReport{
		Module:   "tts",
		Status:   core.HealthLive,
		Ready:    true,
		Detail:   snapshot.Detail,
		Metadata: metadata,
	}
}
