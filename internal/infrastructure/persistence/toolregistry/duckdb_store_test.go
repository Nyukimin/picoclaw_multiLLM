package toolregistry

import (
	"context"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
)

func newTestStore(t *testing.T) *DuckDBToolRegistryStore {
	t.Helper()
	store, err := NewDuckDBToolRegistryStore(":memory:")
	if err != nil {
		t.Fatalf("NewDuckDBToolRegistryStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestRegisterAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	entry := capability.ToolEntry{
		Name:        "web_search",
		Description: "Search the web",
		SchemaJSON:  `{"type":"function","function":{"name":"web_search"}}`,
		Platforms:   []string{"linux", "windows"},
		Source:      capability.ToolSourceBuiltin,
		Trusted:     true,
		CreatedAt:   time.Now(),
		CreatedBy:   "builtin",
	}

	if err := store.Register(ctx, entry); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := store.Get(ctx, "web_search")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "web_search" {
		t.Errorf("Name = %q, want %q", got.Name, "web_search")
	}
	if len(got.Platforms) != 2 {
		t.Errorf("Platforms len = %d, want 2", len(got.Platforms))
	}
	if got.Source != capability.ToolSourceBuiltin {
		t.Errorf("Source = %q, want %q", got.Source, capability.ToolSourceBuiltin)
	}
}

func TestRegister_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	entry := capability.ToolEntry{
		Name:      "tool_a",
		SchemaJSON: `{}`,
		Platforms: []string{"linux"},
		Source:    capability.ToolSourceBuiltin,
		Trusted:   true,
	}

	// 2回登録しても重複エラーにならない
	if err := store.Register(ctx, entry); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	entry.Description = "Updated description"
	if err := store.Register(ctx, entry); err != nil {
		t.Fatalf("second Register: %v", err)
	}

	got, err := store.Get(ctx, "tool_a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "Updated description" {
		t.Errorf("Description = %q, want %q", got.Description, "Updated description")
	}
}

func TestApprove(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	entry := capability.ToolEntry{
		Name:      "shiro_tool",
		SchemaJSON: `{}`,
		Platforms: []string{"linux"},
		Source:    capability.ToolSourceShiroGenerated,
		Trusted:   false,
	}
	if err := store.Register(ctx, entry); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 未承認なので ListForPlatform に出ない
	list, err := store.ListForPlatform(ctx, "linux")
	if err != nil {
		t.Fatalf("ListForPlatform: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 tools before approval, got %d", len(list))
	}

	if err := store.Approve(ctx, "shiro_tool"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	list, err = store.ListForPlatform(ctx, "linux")
	if err != nil {
		t.Fatalf("ListForPlatform after approval: %v", err)
	}
	if len(list) != 1 || list[0].Name != "shiro_tool" {
		t.Errorf("expected shiro_tool after approval, got %v", list)
	}
}

func TestApprove_NotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.Approve(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestListForPlatform(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tools := []capability.ToolEntry{
		{Name: "linux_only", SchemaJSON: "{}", Platforms: []string{"linux"}, Source: capability.ToolSourceBuiltin, Trusted: true},
		{Name: "windows_only", SchemaJSON: "{}", Platforms: []string{"windows"}, Source: capability.ToolSourceBuiltin, Trusted: true},
		{Name: "cross_platform", SchemaJSON: "{}", Platforms: []string{"linux", "windows"}, Source: capability.ToolSourceBuiltin, Trusted: true},
	}
	for _, e := range tools {
		if err := store.Register(ctx, e); err != nil {
			t.Fatalf("Register %q: %v", e.Name, err)
		}
	}

	linuxTools, err := store.ListForPlatform(ctx, "linux")
	if err != nil {
		t.Fatalf("ListForPlatform(linux): %v", err)
	}
	if len(linuxTools) != 2 {
		t.Errorf("expected 2 linux tools, got %d: %v", len(linuxTools), linuxTools)
	}

	winTools, err := store.ListForPlatform(ctx, "windows")
	if err != nil {
		t.Fatalf("ListForPlatform(windows): %v", err)
	}
	if len(winTools) != 2 {
		t.Errorf("expected 2 windows tools, got %d", len(winTools))
	}

	darwinTools, err := store.ListForPlatform(ctx, "darwin")
	if err != nil {
		t.Fatalf("ListForPlatform(darwin): %v", err)
	}
	if len(darwinTools) != 0 {
		t.Errorf("expected 0 darwin tools, got %d", len(darwinTools))
	}
}
