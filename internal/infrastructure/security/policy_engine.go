package security

import (
	"fmt"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	domainsecurity "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/security"
)

// PolicyConfig はポリシー判定設定
type PolicyConfig struct {
	Mode              string
	ApprovalMode      string
	NetworkScope      string
	NetworkAllowed    []string
	DenyCommands      []string
	Workspace         string
	WorkspaceEnforced bool
}

// PolicyEngine は実行可否判定を行う
type PolicyEngine struct {
	cfg   PolicyConfig
	guard *SandboxGuard
}

func NewPolicyEngine(cfg PolicyConfig) *PolicyEngine {
	return &PolicyEngine{cfg: cfg, guard: NewSandboxGuard()}
}

func (e *PolicyEngine) Evaluate(action execution.Action) execution.PolicyDecision {
	// 1) 強制 deny: shell の禁止コマンド
	if action.Tool == "shell" {
		if cmd, ok := action.Arguments["command"].(string); ok && e.guard.IsCommandDenied(cmd, e.cfg.DenyCommands) {
			return execution.PolicyDecision{
				Decision:      execution.DecisionDeny,
				Reason:        "blocked shell command signature",
				MatchedRuleID: "deny.shell.signature",
			}
		}
	}

	// 2) 強制 deny: workspace 外書き込み
	if e.cfg.WorkspaceEnforced && action.Tool == "file_write" {
		if p, ok := action.Arguments["path"].(string); ok {
			if !e.guard.IsPathWithinWorkspace(p, e.cfg.Workspace) {
				return execution.PolicyDecision{
					Decision:      execution.DecisionDeny,
					Reason:        fmt.Sprintf("path outside workspace: %s", p),
					MatchedRuleID: "deny.workspace.outside",
				}
			}
		}
	}

	// 2.5) 強制 deny: ネットワーク権限
	networkScope := strings.TrimSpace(e.cfg.NetworkScope)
	if networkScope == "" {
		profile := e.profileByMode()
		networkScope = profile.NetworkScope
	}
	if isNetworkTool(action.Tool) {
		switch networkScope {
		case "blocked":
			return execution.PolicyDecision{
				Decision:      execution.DecisionDeny,
				Reason:        "network access blocked by policy",
				MatchedRuleID: "deny.network.blocked",
			}
		case "allowlist":
			host, ok := e.guard.ExtractNetworkHost(action.Arguments)
			if !ok {
				return execution.PolicyDecision{
					Decision:      execution.DecisionDeny,
					Reason:        "network host is required under allowlist policy",
					MatchedRuleID: "deny.network.host.missing",
				}
			}
			allowed := e.cfg.NetworkAllowed
			if len(allowed) == 0 {
				allowed = []string{"localhost", "127.0.0.1", "::1"}
			}
			if !e.guard.IsHostAllowed(host, allowed) {
				return execution.PolicyDecision{
					Decision:      execution.DecisionDeny,
					Reason:        fmt.Sprintf("host not in allowlist: %s", host),
					MatchedRuleID: "deny.network.host.not_allowlisted",
				}
			}
		}
	}

	approvalMode := strings.TrimSpace(e.cfg.ApprovalMode)
	if approvalMode == "" {
		approvalMode = e.profileByMode().ApprovalMode
	}

	// 3) 承認モード
	switch approvalMode {
	case "always":
		return execution.PolicyDecision{
			Decision:      execution.DecisionAsk,
			Reason:        "approval_mode=always",
			MatchedRuleID: "ask.always",
		}
	case "on_demand":
		if action.RequiresApproval {
			return execution.PolicyDecision{
				Decision:      execution.DecisionAsk,
				Reason:        "tool requires approval",
				MatchedRuleID: "ask.tool.requires_approval",
			}
		}
	}

	return execution.PolicyDecision{
		Decision:      execution.DecisionAllow,
		Reason:        "policy allow",
		MatchedRuleID: "allow.default",
	}
}

func (e *PolicyEngine) profileByMode() domainsecurity.SecurityProfile {
	switch strings.TrimSpace(strings.ToLower(e.cfg.Mode)) {
	case "strict":
		return domainsecurity.StrictProfile()
	case "dev":
		return domainsecurity.DevProfile()
	default:
		return domainsecurity.BalancedProfile()
	}
}

func isNetworkTool(toolName string) bool {
	switch strings.TrimSpace(strings.ToLower(toolName)) {
	case "web_search", "http_request", "fetch_url":
		return true
	default:
		return false
	}
}
