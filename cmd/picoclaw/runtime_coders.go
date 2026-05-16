package main

import (
	"context"
	"log"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	capdomain "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	llmfactory "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/factory"
)

type coderAdapter struct {
	domainCoder *agent.CoderAgent
}

func (a *coderAdapter) Generate(ctx context.Context, t task.Task, systemPrompt string) (string, error) {
	return a.domainCoder.GenerateWithPrompt(ctx, t, systemPrompt)
}

func (a *coderAdapter) GenerateProposal(ctx context.Context, t task.Task) (*proposal.Proposal, error) {
	return a.domainCoder.GenerateProposal(ctx, t)
}

// buildCoderCapabilities は NodeCapabilities と config から []CoderCapability を構築する（Phase 3）
func buildCoderCapabilities(nodeCaps capdomain.NodeCapabilities, cfg *config.Config) []capdomain.CoderCapability {
	// 検出結果の LLM を "provider/model" でインデックス
	detected := make(map[string]capdomain.LLMCapability)
	for _, l := range nodeCaps.LLMs {
		detected[l.ProviderName+"/"+l.ModelName] = l
	}

	// プロバイダー別デフォルト品質（llm_quality_overrides に記載がない場合の fallback）
	providerDefault := map[string]int{
		"claude": 5, "openai": 4, "deepseek": 3, "ollama": 2,
	}

	type coderEntry struct {
		name string
		cc   config.CoderConfig
	}
	entries := []coderEntry{
		{"coder1", cfg.Coder1},
		{"coder2", cfg.Coder2},
		{"coder3", cfg.Coder3},
		{"coder4", cfg.Coder4},
	}

	caps := make([]capdomain.CoderCapability, 0, len(entries))
	anyUsable := false
	for _, e := range entries {
		var quality int
		var available bool

		if l, ok := detected[e.cc.Provider+"/"+e.cc.Model]; ok {
			quality = l.Quality
			available = e.cc.Enabled && l.Available
		} else {
			quality = cfg.Capability.LLMQualityOverrides[e.cc.Model]
			if quality == 0 {
				quality = providerDefault[e.cc.Provider]
			}
			available = e.cc.Enabled && e.cc.APIKey != ""
		}

		if quality > 0 {
			anyUsable = true
		}
		caps = append(caps, capdomain.CoderCapability{
			Name:      e.name,
			Quality:   quality,
			Available: available,
		})
	}

	if !anyUsable {
		return nil // 品質情報なし → 静的チェーンにフォールバック
	}
	return caps
}

// setupCoders は Config から Coder1-4 を初期化（v4.1 Agent Persona 対応）
func setupCoders(cfg *config.Config) (coder1, coder2, coder3, coder4 *coderAdapter) {
	// Shared LightMemory instances (セッション単位で共有)
	var globalLightMemory *agent.LightMemory

	coderConfigs := []struct {
		name   string
		config config.CoderConfig
		out    **coderAdapter
	}{
		{"coder1", cfg.Coder1, &coder1},
		{"coder2", cfg.Coder2, &coder2},
		{"coder3", cfg.Coder3, &coder3},
		{"coder4", cfg.Coder4, &coder4},
	}

	for _, cc := range coderConfigs {
		if !cc.config.Enabled {
			log.Printf("[setupCoders] %s (%s) disabled", cc.name, cc.config.Name)
			continue
		}

		// LLM Provider 生成
		provider, err := llmfactory.CreateProvider(cc.config)
		if err != nil {
			log.Printf("[setupCoders] %s (%s) provider creation failed: %v", cc.name, cc.config.Name, err)
			continue
		}
		if provider == nil {
			log.Printf("[setupCoders] %s (%s) provider is nil (Enabled=false or error)", cc.name, cc.config.Name)
			continue
		}

		// CoderAgent 作成
		domainCoder := agent.NewCoderAgent(provider, nil, nil, cfg.Prompts.CoderProposal)

		// Agent Persona 設定（persona_file 優先、なければ personality、最後に characters/<name>）
		personality, source := resolveCoderPersonality(cfg, cc.config)
		if source != "" {
			log.Printf("[setupCoders] %s (%s) persona loaded from %s", cc.name, cc.config.DisplayName, source)
		}
		if personality != "" {
			coderPersona := agent.AgentPersona{
				Name:        cc.config.Name,
				Personality: personality,
				Tone:        cc.config.Tone,
			}
			domainCoder.WithPersona(coderPersona)
			log.Printf("[setupCoders] %s (%s) persona enabled", cc.name, cc.config.DisplayName)
		}

		// LightMemory 設定（全 Coder で共有）
		if cc.config.LightMemory.Enabled {
			if globalLightMemory == nil {
				maxTurns := cc.config.LightMemory.MaxTurns
				if maxTurns <= 0 {
					maxTurns = 3
				}
				globalLightMemory = agent.NewLightMemory(maxTurns)
				log.Printf("[setupCoders] LightMemory initialized with maxTurns=%d", maxTurns)
			}
			domainCoder.WithLightMemory(globalLightMemory)
			log.Printf("[setupCoders] %s (%s) LightMemory enabled", cc.name, cc.config.DisplayName)
		}

		// coderAdapter 作成
		*cc.out = &coderAdapter{domainCoder: domainCoder}
		log.Printf("[setupCoders] %s (%s) enabled: provider=%s model=%s",
			cc.name, cc.config.DisplayName, cc.config.Provider, cc.config.Model)
	}

	return
}

func coderConfigWithRuntimePersonality(cfg *config.Config, coderCfg config.CoderConfig) config.CoderConfig {
	personality, _ := resolveCoderPersonality(cfg, coderCfg)
	if personality != "" {
		coderCfg.Personality = personality
	}
	return coderCfg
}

func resolveCoderPersonality(cfg *config.Config, coderCfg config.CoderConfig) (string, string) {
	if cfg != nil && coderCfg.PersonaFile != "" {
		if content, ok := config.LoadPersonaFile(cfg.WorkspaceDir, coderCfg.PersonaFile); ok {
			return content, "file: " + coderCfg.PersonaFile
		}
	}
	if strings.TrimSpace(coderCfg.Personality) != "" {
		return coderCfg.Personality, "inline personality"
	}
	if cfg != nil && cfg.Prompts != nil {
		name := strings.ToLower(strings.TrimSpace(coderCfg.Name))
		if content := strings.TrimSpace(cfg.Prompts.CharacterPrompts[name]); content != "" {
			return content, "character bundle: " + name
		}
	}
	return "", ""
}
