package orchestrator

import (
	"context"
	"fmt"
	"strings"

	autonomousapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/autonomous"
	contractapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/contract"
	domaincontract "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/contract"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func (o *MessageOrchestrator) executeAutonomousTask(ctx context.Context, t task.Task, route routing.Route, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	if !isAutonomousRoute(route) {
		return "", fmt.Errorf("unknown route: %s", route)
	}
	contract, err := contractapp.NormalizeRequestWithRoute(t.UserMessage(), route.String())
	if err != nil {
		return "", err
	}
	result, err := autonomousapp.RunExecutor(ctx, autonomousapp.ExecuteRequest{
		JobID:      t.JobID().String(),
		Route:      route.String(),
		Capability: capabilityForRoute(route),
		Contract:   contract,
		MaxRepair:  o.maxRepairOrDefault(),
		Observe: func(stage autonomousapp.Stage) {
			o.emit("entry.stage", channel, "system", string(stage), route.String(), t.JobID().String(), sessionID, channel, chatID)
		},
		ReportStore: o.reporter,
		Execute: func(execCtx context.Context, attempt int, failureKind, failureReason string) (autonomousapp.AttemptResult, error) {
			execTask := t
			if attempt > 0 {
				execTask = execTask.WithUserMessage(buildExecutorRetryMessage(t.UserMessage(), route, failureKind, failureReason, attempt))
			}
			resp, runErr := o.executeRouteDirect(execCtx, execTask, route, sessionID, channel, chatID, ttsSessionID)
			return autonomousapp.AttemptResult{
				Response:      resp,
				Steps:         routeExecutionSteps(route, runErr == nil),
				FailureKind:   classifyExecutorFailure(runErr),
				FailureReason: errorString(runErr),
			}, runErr
		},
		Verify: func(_ context.Context, c domaincontract.Contract, last autonomousapp.AttemptResult) (bool, string, string, error) {
			ok, kind, reason := verifyByContract(route, c, last)
			return ok, kind, reason, nil
		},
	})
	if err != nil {
		return result.Response, err
	}
	return result.Response, nil
}

func capabilityForRoute(route routing.Route) autonomousapp.CapabilityPack {
	switch route {
	case routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3, routing.RouteCODE4:
		return autonomousapp.CapabilityCodeChange
	default:
		return autonomousapp.CapabilityGenericExecution
	}
}

func isAutonomousRoute(route routing.Route) bool {
	switch route {
	case routing.RouteOPS, routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3, routing.RouteCODE4, routing.RoutePLAN, routing.RouteANALYZE, routing.RouteRESEARCH, routing.RouteWILD:
		return true
	default:
		return false
	}
}

func routeExecutionSteps(route routing.Route, ok bool) []string {
	items := []string{"routing.decision"}
	switch route {
	case routing.RouteOPS:
		items = append(items, "shiro.execute")
	case routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3, routing.RouteCODE4:
		items = append(items, "shiro.delegate", "coder.execute", "shiro.verify")
	case routing.RoutePLAN:
		items = append(items, "mio.plan")
	case routing.RouteANALYZE:
		items = append(items, "heavy.analyze")
	case routing.RouteRESEARCH:
		items = append(items, "mio.research")
	case routing.RouteWILD:
		items = append(items, "wild.generate")
	}
	if ok {
		items = append(items, "done")
	} else {
		items = append(items, "error")
	}
	return items
}

func classifyExecutorFailure(err error) string {
	if err == nil {
		return ""
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "proposal"):
		return "proposal_invalid"
	case strings.Contains(lower, "not found"), strings.Contains(lower, "exit status 127"):
		return "command_missing"
	case strings.Contains(lower, "provider"), strings.Contains(lower, "model"), strings.Contains(lower, "ollama"):
		return "provider_unavailable"
	default:
		return "apply"
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func responseLooksLikeFailure(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "失敗: 0") || strings.Contains(lower, "failures: 0") || strings.Contains(lower, "failed: 0") {
		return false
	}
	return strings.Contains(lower, "error") || strings.Contains(lower, "失敗") ||
		strings.Contains(content, "エラー")
}

func shortFailureReason(content string) string {
	text := strings.TrimSpace(content)
	if len(text) <= 160 {
		return text
	}
	return text[:157] + "..."
}

// verifyByContract はルートと実行契約に基づいて AttemptResult を検証する。
// verifyAutonomousRouteResponse の後継。
func verifyByContract(
	route routing.Route,
	c domaincontract.Contract,
	last autonomousapp.AttemptResult,
) (bool, string, string) {
	// (1) 全ルート共通: 空レスポンス拒否
	if strings.TrimSpace(last.Response) == "" {
		return false, "verification_failed", "empty response"
	}

	// (2) TTS CapabilityPack 検証
	if isTTSCapability(c) {
		return verifyTTSResult(last)
	}

	// (3) CODE ルート検証
	if isCodeRoute(route) {
		if looksLikeNonExecutable(last.Response) {
			return false, "non_executable_output",
				"Coder output contains design document only; executable patch is required"
		}
		if responseLooksLikeFailure(last.Response) {
			return false, "verification_failed", shortFailureReason(last.Response)
		}
		return true, "", ""
	}

	// (4) OPS / PLAN / ANALYZE / RESEARCH
	if responseLooksLikeFailure(last.Response) {
		return false, "verification_failed", shortFailureReason(last.Response)
	}
	return true, "", ""
}

// isTTSCapability は契約の Acceptance フィールドから TTS CapabilityPack かどうかを判定する。
func isTTSCapability(c domaincontract.Contract) bool {
	for _, a := range c.Acceptance {
		if strings.Contains(a, "実再生") || strings.Contains(a, "音声ファイル生成") {
			return true
		}
	}
	return false
}

// verifyTTSResult は TTS CapabilityPack の E2E 検証を行う。
// PlaybackCode/TTSAudioFile が未設定の場合は暫定フォールバック（レスポンス文字列チェック）。
func verifyTTSResult(last autonomousapp.AttemptResult) (bool, string, string) {
	// Phase 2 で TTS ブリッジ結果が注入されるまでの暫定フォールバック
	if last.TTSAudioFile == "" && last.PlaybackCode == 0 {
		if responseLooksLikeFailure(last.Response) {
			return false, "verification_failed", shortFailureReason(last.Response)
		}
		return true, "", ""
	}
	if last.TTSAudioFile == "" {
		return false, "tts_no_audio", "音声ファイルが生成されていない (TTSAudioFile が空)"
	}
	if last.PlaybackCode != 0 {
		return false, "playback_failed",
			fmt.Sprintf("再生コマンドが終了コード %d で終了した", last.PlaybackCode)
	}
	return true, "", ""
}

// looksLikeNonExecutable は Coder の出力が設計文書のみで実行可能形式を含まないかを判定する。
func looksLikeNonExecutable(response string) bool {
	lower := strings.ToLower(response)
	executables := []string{
		"```",              // コードブロック
		"patch:",           // Shiro patch セクション
		"apply:",           // patch 適用指示
		"execute:",         // 実行指示
		"$ ",               // シェルコマンド
		"#!/",              // シェバン
		"execution result", // formatExecutionResult のセクションヘッダー（実行証跡）
		"success rate",     // formatExecutionResult の実行結果（実行証跡）
	}
	for _, marker := range executables {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

func buildExecutorRetryMessage(userMessage string, route routing.Route, failureKind, failureReason string, attempt int) string {
	return fmt.Sprintf(`%s

## Executor Retry Context
- retry_attempt: %d
- route: %s
- failure_kind: %s
- failure_reason: %s

## Requirements
- Keep the response executable and directly verifiable
- Include the missing repair steps in the next result
- Do not defer required fixes to the user
`, userMessage, attempt, route, fallbackString(failureKind, "unknown"), fallbackString(failureReason, "execution failed"))
}
