package main

import "testing"

func TestParseRepairCLISlashCommand(t *testing.T) {
	cmd, instruction, ok := parseRepairCLISlashCommand("/repair ログを見て修復")
	if !ok || cmd != "run" || instruction != "ログを見て修復" {
		t.Fatalf("parse repair command = %q %q %v", cmd, instruction, ok)
	}

	cmd, _, ok = parseRepairCLISlashCommand("/repair help")
	if !ok || cmd != "help" {
		t.Fatalf("parse repair help = %q %v", cmd, ok)
	}

	if _, _, ok := parseRepairCLISlashCommand("/chat hello"); ok {
		t.Fatal("non-repair command must not match")
	}
}
