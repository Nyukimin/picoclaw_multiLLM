package capability_test

import (
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

func TestSelectCoder_CODE3_DirectMatch(t *testing.T) {
	coders := []capability.CoderCapability{
		{Name: "coder1", Quality: 3, Available: true},
		{Name: "coder2", Quality: 4, Available: true},
		{Name: "coder3", Quality: 5, Available: true},
	}
	selected, degraded, err := capability.SelectCoder(coders, routing.RouteCODE3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected != "coder3" {
		t.Errorf("expected coder3, got %s", selected)
	}
	if degraded != "" {
		t.Errorf("expected no degradation, got %s", degraded)
	}
}

func TestSelectCoder_CODE3_DegradeToQuality4(t *testing.T) {
	coders := []capability.CoderCapability{
		{Name: "coder1", Quality: 3, Available: true},
		{Name: "coder2", Quality: 4, Available: true},
		{Name: "coder3", Quality: 5, Available: false},
	}
	selected, degraded, err := capability.SelectCoder(coders, routing.RouteCODE3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected != "coder2" {
		t.Errorf("expected coder2, got %s", selected)
	}
	if degraded != routing.RouteCODE2 {
		t.Errorf("expected degradedRoute CODE2, got %q", degraded)
	}
}

func TestSelectCoder_CODE3_DegradeToQuality3(t *testing.T) {
	coders := []capability.CoderCapability{
		{Name: "coder1", Quality: 3, Available: true},
		{Name: "coder2", Quality: 4, Available: false},
		{Name: "coder3", Quality: 5, Available: false},
	}
	selected, degraded, err := capability.SelectCoder(coders, routing.RouteCODE3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected != "coder1" {
		t.Errorf("expected coder1, got %s", selected)
	}
	if degraded != routing.RouteCODE1 {
		t.Errorf("expected degradedRoute CODE1, got %q", degraded)
	}
}

func TestSelectCoder_AllUnavailable_Error(t *testing.T) {
	coders := []capability.CoderCapability{
		{Name: "coder1", Quality: 3, Available: false},
		{Name: "coder2", Quality: 4, Available: false},
		{Name: "coder3", Quality: 5, Available: false},
	}
	_, _, err := capability.SelectCoder(coders, routing.RouteCODE3)
	if err == nil {
		t.Error("expected error when all coders unavailable")
	}
}

func TestSelectCoder_CODE_PicksHighestQuality(t *testing.T) {
	coders := []capability.CoderCapability{
		{Name: "coder1", Quality: 3, Available: true},
		{Name: "coder2", Quality: 4, Available: true},
		{Name: "coder3", Quality: 5, Available: false},
	}
	selected, _, err := capability.SelectCoder(coders, routing.RouteCODE)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected != "coder2" {
		t.Errorf("expected coder2 (highest available quality), got %s", selected)
	}
}

func TestSelectCoder_CODE2_DirectMatch(t *testing.T) {
	coders := []capability.CoderCapability{
		{Name: "coder1", Quality: 3, Available: true},
		{Name: "coder2", Quality: 4, Available: true},
	}
	selected, degraded, err := capability.SelectCoder(coders, routing.RouteCODE2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected != "coder2" {
		t.Errorf("expected coder2, got %s", selected)
	}
	if degraded != "" {
		t.Errorf("expected no degradation, got %q", degraded)
	}
}

func TestSelectCoder_SameQuality_AlphabeticalOrder(t *testing.T) {
	coders := []capability.CoderCapability{
		{Name: "coder4", Quality: 4, Available: true},
		{Name: "coder2", Quality: 4, Available: true},
	}
	selected, _, err := capability.SelectCoder(coders, routing.RouteCODE)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected != "coder2" {
		t.Errorf("expected coder2 (alphabetically first among equal quality), got %s", selected)
	}
}
