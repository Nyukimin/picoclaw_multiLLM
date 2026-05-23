package memory

import "testing"

func TestValidateNamespace(t *testing.T) {
	for _, namespace := range []string{"conv:123", "user:ren", "char:mio", "kb:ai"} {
		if err := ValidateNamespace(namespace); err != nil {
			t.Fatalf("ValidateNamespace(%q) failed: %v", namespace, err)
		}
	}
	for _, namespace := range []string{"", "user:", "misc:ren", "kb:"} {
		if err := ValidateNamespace(namespace); err == nil {
			t.Fatalf("ValidateNamespace(%q) should fail", namespace)
		}
	}
}

func TestCanPromoteUserMemoryRequiresEvidence(t *testing.T) {
	if err := CanPromoteUserMemory(MemoryStateConfirmed, nil, "normal", "user_explicit"); err == nil {
		t.Fatal("confirmed user memory without evidence should fail")
	}
	if err := CanPromoteUserMemory(MemoryStatePinned, []string{"evt_1"}, "normal", ""); err == nil {
		t.Fatal("pinned user memory without explicit reason should fail")
	}
	if err := CanPromoteUserMemory(MemoryStatePinned, []string{"evt_1"}, "sensitive", "user_explicit"); err == nil {
		t.Fatal("sensitive memory should not auto-promote")
	}
	if err := CanPromoteUserMemory(MemoryStateConfirmed, []string{"evt_1"}, "normal", "user_explicit"); err != nil {
		t.Fatalf("confirmed with evidence should pass: %v", err)
	}
}
