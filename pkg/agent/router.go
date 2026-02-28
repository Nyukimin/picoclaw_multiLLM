package agent

import (
	"context"
	"regexp"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/session"
)

const (
	RouteChat     = "CHAT"
	RoutePlan     = "PLAN"
	RouteAnalyze  = "ANALYZE"
	RouteOps      = "OPS"
	RouteResearch = "RESEARCH"
	RouteCode     = "CODE"
	RouteCode1    = "CODE1"
	RouteCode2    = "CODE2"
	RouteCode3    = "CODE3"
)

type RoutingDecision struct {
	Route                string
	Source               string
	Confidence           float64
	Reason               string
	Evidence             []string
	LocalOnly            bool
	PrevRoute            string
	CleanUserText        string
	Declaration          string
	DirectResponse       string
	ErrorReason          string
	ClassifierConfidence float64
}

type Router struct {
	cfg        config.RoutingConfig
	classifier *Classifier
}

func NewRouter(cfg config.RoutingConfig, classifier *Classifier) *Router {
	if cfg.FallbackRoute == "" {
		cfg.FallbackRoute = RouteChat
	}
	if cfg.Classifier.MinConfidence <= 0 {
		cfg.Classifier.MinConfidence = 0.6
	}
	if cfg.Classifier.MinConfidenceForCode <= 0 {
		cfg.Classifier.MinConfidenceForCode = 0.8
	}
	return &Router{cfg: cfg, classifier: classifier}
}

func isAllowedRoute(route string) bool {
	switch route {
	case RouteChat, RoutePlan, RouteAnalyze, RouteOps, RouteResearch, RouteCode, RouteCode1, RouteCode2, RouteCode3:
		return true
	default:
		return false
	}
}

func IsCodeRoute(route string) bool {
	switch route {
	case RouteCode, RouteCode1, RouteCode2, RouteCode3:
		return true
	default:
		return false
	}
}

func (r *Router) Decide(ctx context.Context, userText string, flags session.SessionFlags) RoutingDecision {
	clean := strings.TrimSpace(userText)
	decision := RoutingDecision{
		Route:         RouteChat,
		Source:        "fallback",
		LocalOnly:     flags.LocalOnly,
		PrevRoute:     flags.PrevPrimaryRoute,
		CleanUserText: clean,
	}

	// 1) explicit command
	cmdRoute, cmdLocalOnly, cmdMsg, stripped, ok := parseRouteCommand(clean, flags.LocalOnly)
	if ok {
		decision.LocalOnly = cmdLocalOnly
		decision.CleanUserText = stripped
		decision.Source = "command"
		if cmdMsg != "" {
			decision.DirectResponse = cmdMsg
			decision.Route = flags.PrevPrimaryRoute
			if decision.Route == "" {
				decision.Route = RouteChat
			}
			return decision
		}
		decision.Route = cmdRoute
		decision.Confidence = 1.0
		decision.Reason = "explicit command"
		decision.Declaration = declarationFor(decision.PrevRoute, decision.Route)
		return decision
	}

	// 2) rules (strong signals only)
	route, evidence, matched := matchRule(clean)
	if matched {
		decision.Route = route
		decision.Source = "rules"
		decision.Confidence = 1.0
		decision.Evidence = evidence
		decision.Reason = "strong rule match"
		decision.Declaration = declarationFor(decision.PrevRoute, decision.Route)
		return decision
	}

	// 3) classifier
	if r.cfg.Classifier.Enabled && r.classifier != nil {
		classification, ok := r.classifier.Classify(ctx, clean)
		if ok {
			decision.ClassifierConfidence = classification.Confidence
			minConfidence := r.cfg.Classifier.MinConfidence
			if IsCodeRoute(strings.ToUpper(classification.Route)) {
				minConfidence = r.cfg.Classifier.MinConfidenceForCode
				if !hasStrongCodeEvidence(clean) {
					decision.ErrorReason = "classifier_code_without_strong_evidence"
					decision.Route = RouteChat
					decision.Source = "fallback"
					decision.Declaration = declarationFor(decision.PrevRoute, decision.Route)
					return decision
				}
			}
			if classification.Confidence >= minConfidence {
				decision.Route = classification.Route
				decision.Source = "classifier"
				decision.Confidence = classification.Confidence
				decision.Reason = classification.Reason
				decision.Evidence = classification.Evidence
				decision.Declaration = declarationFor(decision.PrevRoute, decision.Route)
				return decision
			}
			decision.ErrorReason = "classifier_low_confidence"
		} else {
			decision.ErrorReason = "classifier_invalid_output"
		}
	}

	// 4) fallback
	fallback := strings.ToUpper(strings.TrimSpace(r.cfg.FallbackRoute))
	if !isAllowedRoute(fallback) {
		fallback = RouteChat
	}
	decision.Route = fallback
	decision.Source = "fallback"
	decision.Declaration = declarationFor(decision.PrevRoute, decision.Route)
	return decision
}

func declarationFor(prevRoute, curRoute string) string {
	if curRoute == "" || prevRoute == curRoute || curRoute == RouteChat {
		return ""
	}
	switch curRoute {
	case RouteCode:
		return "コーディングするね。"
	case RouteCode1:
		return "設計・仕様をまとめるね。"
	case RouteCode2:
		return "コーディングするね。"
	case RouteCode3:
		return "高品質なコードを作るね。"
	case RouteAnalyze:
		return "整理して分析するね。"
	case RoutePlan:
		return "段取りを組むね。"
	case RouteOps:
		return "手順で案内するね。"
	case RouteResearch:
		return "調べてまとめるね。"
	default:
		return ""
	}
}

func parseRouteCommand(text string, localOnly bool) (route string, nextLocalOnly bool, directMsg string, stripped string, ok bool) {
	nextLocalOnly = localOnly
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return "", nextLocalOnly, "", text, false
	}
	cmd := strings.ToLower(parts[0])
	rest := ""
	if len(parts) > 1 {
		rest = strings.TrimSpace(strings.Join(parts[1:], " "))
	}

	switch cmd {
	case "/local":
		nextLocalOnly = true
		return "", nextLocalOnly, "ローカル専用モードを有効にしたよ。クラウド利用が必要なら /cloud を使ってね。", rest, true
	case "/cloud":
		nextLocalOnly = false
		return "", nextLocalOnly, "ローカル専用モードを解除したよ。", rest, true
	case "/chat":
		return RouteChat, nextLocalOnly, "", rest, true
	case "/plan":
		return RoutePlan, nextLocalOnly, "", rest, true
	case "/analyze":
		return RouteAnalyze, nextLocalOnly, "", rest, true
	case "/ops":
		return RouteOps, nextLocalOnly, "", rest, true
	case "/research":
		return RouteResearch, nextLocalOnly, "", rest, true
	case "/code":
		if nextLocalOnly {
			return "", nextLocalOnly, "いまは /local モード中だからCODE実行はできないよ。/cloud で解除してから試してね。", rest, true
		}
		return RouteCode, nextLocalOnly, "", rest, true
	case "/code1":
		if nextLocalOnly {
			return "", nextLocalOnly, "いまは /local モード中だからCODE実行はできないよ。/cloud で解除してから試してね。", rest, true
		}
		return RouteCode1, nextLocalOnly, "", rest, true
	case "/code2":
		if nextLocalOnly {
			return "", nextLocalOnly, "いまは /local モード中だからCODE実行はできないよ。/cloud で解除してから試してね。", rest, true
		}
		return RouteCode2, nextLocalOnly, "", rest, true
	case "/code3":
		if nextLocalOnly {
			return "", nextLocalOnly, "いまは /local モード中だからCODE実行はできないよ。/cloud で解除してから試してね。", rest, true
		}
		return RouteCode3, nextLocalOnly, "", rest, true
	default:
		return "", nextLocalOnly, "", text, false
	}
}

func matchRule(text string) (string, []string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return RouteChat, nil, false
	}
	if hasStrongCodeEvidence(trimmed) {
		return RouteCode, []string{"strong_code_evidence"}, true
	}
	if regexp.MustCompile(`(?i)\b(systemctl|journalctl|docker|ssh|kubectl)\b`).MatchString(trimmed) {
		return RouteOps, []string{"ops_keywords"}, true
	}
	if regexp.MustCompile(`(?i)\b(集計|傾向|統計|analyze|analysis|csv|json)\b`).MatchString(trimmed) {
		return RouteAnalyze, []string{"analyze_keywords"}, true
	}
	if regexp.MustCompile(`(?i)\bhttps?://\S+|\b(出典|最新|比較|research)\b`).MatchString(trimmed) {
		return RouteResearch, []string{"research_keywords"}, true
	}
	if regexp.MustCompile(`(?i)\b(仕様|設計|構成|段取り|plan|architecture|requirements)\b`).MatchString(trimmed) {
		return RoutePlan, []string{"plan_keywords"}, true
	}
	return RouteChat, nil, false
}

func hasStrongCodeEvidence(text string) bool {
	if strings.Contains(text, "```") ||
		strings.Contains(text, "diff --git") ||
		strings.Contains(text, "Traceback (most recent call last)") {
		return true
	}
	return regexp.MustCompile(`(?i)\b(go\.mod|dockerfile|package\.json|\.go|\.py|\.ts|\.tsx|\.yaml|\.yml)\b`).MatchString(text)
}
