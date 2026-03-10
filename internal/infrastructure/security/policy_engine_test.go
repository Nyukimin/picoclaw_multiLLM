package security

import (
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
)

func TestPolicyEngine_Evaluate(t *testing.T) {
	engine := NewPolicyEngine(PolicyConfig{
		ApprovalMode:      "on_demand",
		DenyCommands:      []string{"rm -rf", "git reset --hard"},
		Workspace:         "/workspace",
		WorkspaceEnforced: true,
	})

	denyCmd := execution.Action{
		Tool:      "shell",
		Arguments: map[string]any{"command": "rm -rf /tmp"},
	}
	if d := engine.Evaluate(denyCmd); d.Decision != execution.DecisionDeny {
		t.Fatalf("expected deny for blocked command, got %s", d.Decision)
	}

	denyPath := execution.Action{
		Tool:      "file_write",
		Arguments: map[string]any{"path": "/etc/passwd", "content": "x"},
	}
	if d := engine.Evaluate(denyPath); d.Decision != execution.DecisionDeny {
		t.Fatalf("expected deny for outside workspace, got %s", d.Decision)
	}

	ask := execution.Action{Tool: "shell", RequiresApproval: true, Arguments: map[string]any{"command": "echo hi"}}
	if d := engine.Evaluate(ask); d.Decision != execution.DecisionAsk {
		t.Fatalf("expected ask for requires approval, got %s", d.Decision)
	}

	allow := execution.Action{Tool: "file_read", Arguments: map[string]any{"path": "/workspace/a.txt"}}
	if d := engine.Evaluate(allow); d.Decision != execution.DecisionAllow {
		t.Fatalf("expected allow for safe action, got %s", d.Decision)
	}
}

func TestPolicyEngine_Evaluate_StrictNetworkAllowlist(t *testing.T) {
	engine := NewPolicyEngine(PolicyConfig{
		Mode:           "strict",
		ApprovalMode:   "never",
		NetworkScope:   "allowlist",
		NetworkAllowed: []string{"api.openai.com"},
	})

	deny := execution.Action{
		Tool:      "web_search",
		Arguments: map[string]any{"url": "https://evil.com/search"},
	}
	d := engine.Evaluate(deny)
	if d.Decision != execution.DecisionDeny {
		t.Fatalf("expected deny for non-allowlisted network host, got %s", d.Decision)
	}
	if d.MatchedRuleID != "deny.network.host.not_allowlisted" {
		t.Fatalf("unexpected rule id: %s", d.MatchedRuleID)
	}

	allow := execution.Action{
		Tool:      "web_search",
		Arguments: map[string]any{"url": "https://api.openai.com/v1/models"},
	}
	if d := engine.Evaluate(allow); d.Decision != execution.DecisionAllow {
		t.Fatalf("expected allow for allowlisted host, got %s", d.Decision)
	}
}

func TestPolicyEngine_Evaluate_DevModeAllowsRiskyProcess(t *testing.T) {
	engine := NewPolicyEngine(PolicyConfig{
		Mode:         "dev",
		ApprovalMode: "",
	})
	act := execution.Action{
		Tool:      "shell",
		Arguments: map[string]any{"command": "echo ok"},
	}
	d := engine.Evaluate(act)
	if d.Decision != execution.DecisionAllow {
		t.Fatalf("expected allow in dev default mode, got %s", d.Decision)
	}
}
