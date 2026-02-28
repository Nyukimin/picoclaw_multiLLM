package agent

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/session"
)

type classifierMockProvider struct {
	content string
}

func (m *classifierMockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, options map[string]interface{}) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: m.content}, nil
}

func (m *classifierMockProvider) GetDefaultModel() string { return "mock" }

func TestRouter_LocalCommand(t *testing.T) {
	r := NewRouter(config.RoutingConfig{}, nil)
	d := r.Decide(context.Background(), "/local", session.SessionFlags{})
	if !d.LocalOnly {
		t.Fatalf("expected local_only true")
	}
	if d.DirectResponse == "" {
		t.Fatalf("expected direct response for /local")
	}
}

func TestRouter_CodeRejectedWhenLocalOnly(t *testing.T) {
	r := NewRouter(config.RoutingConfig{}, nil)
	d := r.Decide(context.Background(), "/code fix this", session.SessionFlags{LocalOnly: true})
	if d.DirectResponse == "" {
		t.Fatalf("expected rejection response")
	}
}

func TestRouter_RuleRouteCode(t *testing.T) {
	r := NewRouter(config.RoutingConfig{}, nil)
	d := r.Decide(context.Background(), "diff --git a/a.go b/a.go", session.SessionFlags{})
	if d.Route != RouteCode || d.Source != "rules" {
		t.Fatalf("expected rules CODE route, got route=%s source=%s", d.Route, d.Source)
	}
}

func TestRouter_Code1Command(t *testing.T) {
	r := NewRouter(config.RoutingConfig{}, nil)
	d := r.Decide(context.Background(), "/code1 設計書を作って", session.SessionFlags{})
	if d.Route != RouteCode1 || d.Source != "command" {
		t.Fatalf("expected CODE1 command route, got route=%s source=%s", d.Route, d.Source)
	}
	if d.CleanUserText != "設計書を作って" {
		t.Fatalf("expected cleaned text, got %q", d.CleanUserText)
	}
}

func TestRouter_Code2Command(t *testing.T) {
	r := NewRouter(config.RoutingConfig{}, nil)
	d := r.Decide(context.Background(), "/code2 実装して", session.SessionFlags{})
	if d.Route != RouteCode2 || d.Source != "command" {
		t.Fatalf("expected CODE2 command route, got route=%s source=%s", d.Route, d.Source)
	}
}

func TestRouter_Code1RejectedWhenLocalOnly(t *testing.T) {
	r := NewRouter(config.RoutingConfig{}, nil)
	d := r.Decide(context.Background(), "/code1 fix this", session.SessionFlags{LocalOnly: true})
	if d.DirectResponse == "" {
		t.Fatalf("expected rejection response for /code1 in local mode")
	}
}

func TestParseChatDelegateDirective_Code1Code2(t *testing.T) {
	for _, route := range []string{"CODE1", "CODE2"} {
		input := "DELEGATE: " + route + "\nTASK:\ndo something"
		d, ok := parseChatDelegateDirective(input)
		if !ok {
			t.Fatalf("expected valid directive for %s", route)
		}
		if d.Route != route {
			t.Fatalf("expected route %s, got %s", route, d.Route)
		}
	}
}

func TestIsCodeRoute(t *testing.T) {
	for _, r := range []string{RouteCode, RouteCode1, RouteCode2} {
		if !IsCodeRoute(r) {
			t.Fatalf("IsCodeRoute(%s) should be true", r)
		}
	}
	for _, r := range []string{RouteChat, RoutePlan, RouteAnalyze} {
		if IsCodeRoute(r) {
			t.Fatalf("IsCodeRoute(%s) should be false", r)
		}
	}
}

func TestRouter_ClassifierRoute(t *testing.T) {
	cfg := config.RoutingConfig{
		Classifier: config.RoutingClassifierConfig{
			Enabled:              true,
			MinConfidence:        0.6,
			MinConfidenceForCode: 0.8,
		},
		FallbackRoute: RouteChat,
	}
	classifier := NewClassifier(&classifierMockProvider{
		content: `{"route":"PLAN","confidence":0.95,"reason":"planning request","evidence":["spec"]}`,
	}, "mock")
	r := NewRouter(cfg, classifier)
	d := r.Decide(context.Background(), "実装方針を作って", session.SessionFlags{})
	if d.Route != RoutePlan || d.Source != "classifier" {
		t.Fatalf("expected classifier PLAN route, got route=%s source=%s", d.Route, d.Source)
	}
}
