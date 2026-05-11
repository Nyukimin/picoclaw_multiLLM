package openai

import (
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

func TestConvertMessagesUsesImageURLParts(t *testing.T) {
	p := NewOpenAIProviderWithOptions("key", "model", "http://example.test", 0)
	got := p.convertMessages(llm.GenerateRequest{Messages: []llm.Message{{
		Role:    "user",
		Content: "画像を見て",
		Parts: []llm.MessagePart{
			{Type: llm.MessagePartText, Text: "画像を見て"},
			{Type: llm.MessagePartImage, MimeType: "image/png", Data: []byte("png")},
		},
	}}})

	content, ok := got[0]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("content type = %T, want multipart array", got[0]["content"])
	}
	if content[0]["type"] != "text" || content[1]["type"] != "image_url" {
		t.Fatalf("unexpected content parts: %#v", content)
	}
	imageURL := content[1]["image_url"].(map[string]interface{})["url"].(string)
	if imageURL != "data:image/png;base64,cG5n" {
		t.Fatalf("image url = %q", imageURL)
	}
}
