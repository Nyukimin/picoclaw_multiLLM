package gemini

import (
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

func TestConvertMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []llm.Message
		want     []geminiContent
	}{
		{
			name: "system role を user に変換",
			messages: []llm.Message{
				{Role: "system", Content: "You are a helpful assistant"},
				{Role: "user", Content: "Hello"},
			},
			want: []geminiContent{
				{Role: "user", Parts: []geminiPart{{Text: "You are a helpful assistant"}}},
				{Role: "user", Parts: []geminiPart{{Text: "Hello"}}},
			},
		},
		{
			name: "assistant role を model に変換",
			messages: []llm.Message{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
			want: []geminiContent{
				{Role: "user", Parts: []geminiPart{{Text: "Hello"}}},
				{Role: "model", Parts: []geminiPart{{Text: "Hi there!"}}},
			},
		},
		{
			name: "複数メッセージ",
			messages: []llm.Message{
				{Role: "system", Content: "System prompt"},
				{Role: "user", Content: "Question 1"},
				{Role: "assistant", Content: "Answer 1"},
				{Role: "user", Content: "Question 2"},
			},
			want: []geminiContent{
				{Role: "user", Parts: []geminiPart{{Text: "System prompt"}}},
				{Role: "user", Parts: []geminiPart{{Text: "Question 1"}}},
				{Role: "model", Parts: []geminiPart{{Text: "Answer 1"}}},
				{Role: "user", Parts: []geminiPart{{Text: "Question 2"}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertMessages(tt.messages)

			if len(got) != len(tt.want) {
				t.Errorf("convertMessages() length = %d, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i].Role != tt.want[i].Role {
					t.Errorf("convertMessages()[%d].Role = %s, want %s", i, got[i].Role, tt.want[i].Role)
				}
				if len(got[i].Parts) != len(tt.want[i].Parts) {
					t.Errorf("convertMessages()[%d].Parts length = %d, want %d", i, len(got[i].Parts), len(tt.want[i].Parts))
					continue
				}
				if got[i].Parts[0].Text != tt.want[i].Parts[0].Text {
					t.Errorf("convertMessages()[%d].Parts[0].Text = %s, want %s", i, got[i].Parts[0].Text, tt.want[i].Parts[0].Text)
				}
			}
		})
	}
}

func TestNewProvider(t *testing.T) {
	apiKey := "test-api-key"
	model := "gemini-2.0-flash-exp"

	provider := NewProvider(apiKey, model)

	if provider.apiKey != apiKey {
		t.Errorf("NewProvider().apiKey = %s, want %s", provider.apiKey, apiKey)
	}
	if provider.model != model {
		t.Errorf("NewProvider().model = %s, want %s", provider.model, model)
	}
	if provider.client == nil {
		t.Error("NewProvider().client is nil")
	}
}

func TestName(t *testing.T) {
	provider := NewProvider("test-key", "test-model")
	if name := provider.Name(); name != "gemini" {
		t.Errorf("Name() = %s, want 'gemini'", name)
	}
}
