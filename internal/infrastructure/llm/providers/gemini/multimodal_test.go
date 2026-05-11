package gemini

import (
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

func TestConvertMessagesUsesInlineDataParts(t *testing.T) {
	got := convertMessages([]llm.Message{{
		Role:    "user",
		Content: "画像を見て",
		Parts: []llm.MessagePart{
			{Type: llm.MessagePartText, Text: "画像を見て"},
			{Type: llm.MessagePartImage, MimeType: "image/png", Data: []byte("png")},
		},
	}})

	parts := got[0].Parts
	if parts[0].Text != "画像を見て" {
		t.Fatalf("text part = %q", parts[0].Text)
	}
	if parts[1].InlineData == nil || parts[1].InlineData.MimeType != "image/png" || parts[1].InlineData.Data != "cG5n" {
		t.Fatalf("unexpected inline data: %#v", parts[1].InlineData)
	}
}
