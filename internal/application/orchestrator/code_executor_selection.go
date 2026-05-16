package orchestrator

import (
	"fmt"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

// codeTarget はコーダー選択の結果
type codeTarget struct {
	name          string
	coder         CoderAgent
	systemPrompt  string
	release       func()        // CoderStatus解放用（オプション）
	degradedRoute routing.Route // 品質縮退が発生した場合の実際のルート（空 = 縮退なし）
}

// selectCoderForRoute はルートに応じてCoderを選択
func (e *DefaultCodeExecutor) selectCoderForRoute(route routing.Route) (codeTarget, error) {
	// Phase 3: 動的選択（coderCaps が設定されている場合）
	if e.coderCaps != nil {
		return e.selectDynamicCoderForRoute(route)
	}

	// 後方互換: 静的チェーン（coderCaps が nil の場合）
	if name, prompt, ok := explicitCodeRouteTarget(route); ok {
		return e.selectExplicitCoderForRoute(route, name, prompt)
	}

	switch route {
	case routing.RouteCODE:
		return e.selectAvailableCoderForGenericRoute(route)
	default:
		return codeTarget{}, fmt.Errorf("unknown code route: %s", route)
	}
}

func (e *DefaultCodeExecutor) selectDynamicCoderForRoute(route routing.Route) (codeTarget, error) {
	chosen, degraded, err := capability.SelectCoder(e.coderCaps, route)
	if err != nil {
		return codeTarget{}, fmt.Errorf("%s route: %w", route, err)
	}
	coder := e.coderByName(chosen)
	if coder == nil {
		return codeTarget{}, fmt.Errorf("%s route: selected coder %s is not initialized", route, chosen)
	}
	log.Printf("[CodeExecutor] coder selected route=%s target=%s mode=dynamic degraded=%s", route, chosen, degraded)
	return codeTarget{
		name:          chosen,
		coder:         coder,
		systemPrompt:  systemPromptForRoute(route),
		degradedRoute: degraded,
	}, nil
}

func (e *DefaultCodeExecutor) selectExplicitCoderForRoute(route routing.Route, name, prompt string) (codeTarget, error) {
	coder := e.coderByName(name)
	if coder == nil {
		return codeTarget{}, fmt.Errorf("%s route requested but no %s available", route, name)
	}
	log.Printf("[CodeExecutor] coder selected route=%s target=%s mode=explicit", route, name)
	return codeTarget{name: name, coder: coder, systemPrompt: prompt}, nil
}

func (e *DefaultCodeExecutor) selectAvailableCoderForGenericRoute(route routing.Route) (codeTarget, error) {
	// 汎用CODEルート: coder1→coder2→coder3→coder4の順でフォールバック
	type coderEntry struct {
		name  string
		coder CoderAgent
	}
	chain := []coderEntry{
		{name: "coder1", coder: e.coder1},
		{name: "coder2", coder: e.coder2},
		{name: "coder3", coder: e.coder3},
		{name: "coder4", coder: e.coder4},
	}
	for _, c := range chain {
		if c.coder == nil {
			log.Printf("[CodeExecutor] coder skip route=%s target=%s reason=unavailable", route, c.name)
			continue
		}
		// CoderStatusがあれば、busy checkを行う
		if e.coderStatus != nil {
			if !e.coderStatus.Acquire(c.name) {
				log.Printf("[CodeExecutor] coder skip route=%s target=%s reason=busy", route, c.name)
				continue
			}
			// Acquire成功時はreleaseを設定
			coderName := c.name
			log.Printf("[CodeExecutor] coder selected route=%s target=%s mode=auto", route, coderName)
			return codeTarget{
				name:         coderName,
				coder:        c.coder,
				systemPrompt: "You are a code generation assistant.",
				release: func() {
					e.coderStatus.Release(coderName)
				},
			}, nil
		}
		// CoderStatusがない場合は単純に選択
		log.Printf("[CodeExecutor] coder selected route=%s target=%s mode=auto", route, c.name)
		return codeTarget{
			name:         c.name,
			coder:        c.coder,
			systemPrompt: "You are a code generation assistant.",
		}, nil
	}
	if e.coderStatus != nil {
		return codeTarget{}, fmt.Errorf("CODE route requested but all coders are busy or unavailable")
	}
	return codeTarget{}, fmt.Errorf("CODE route requested but all coders are unavailable")
}

// systemPromptForRoute はルートに対応するシステムプロンプトを返す
func systemPromptForRoute(route routing.Route) string {
	switch route {
	case routing.RouteCODE1:
		return "You are a specification design assistant."
	case routing.RouteCODE2:
		return "You are an implementation assistant."
	case routing.RouteCODE3:
		return "You are a high-quality code review and reasoning assistant."
	case routing.RouteCODE4:
		return "You are a fast prototyping and experimental coding assistant."
	default:
		return "You are a code generation assistant."
	}
}

// coderByName は名前からCoderAgentを取得
func (e *DefaultCodeExecutor) coderByName(name string) CoderAgent {
	switch name {
	case "coder1":
		return e.coder1
	case "coder2":
		return e.coder2
	case "coder3":
		return e.coder3
	case "coder4":
		return e.coder4
	default:
		return nil
	}
}

// explicitCodeRouteTarget はCODE1/CODE2/CODE3/CODE4の明示的ルートを判定
func explicitCodeRouteTarget(route routing.Route) (name, prompt string, ok bool) {
	switch route {
	case routing.RouteCODE1:
		return "coder1", "You are a specification design assistant.", true
	case routing.RouteCODE2:
		return "coder2", "You are an implementation assistant.", true
	case routing.RouteCODE3:
		return "coder3", "You are a high-quality code review and reasoning assistant.", true
	case routing.RouteCODE4:
		return "coder4", "You are a fast prototyping and experimental coding assistant.", true
	default:
		return "", "", false
	}
}
