package agent

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/logger"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/providers"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/skills"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/tools"
)

type ContextBuilder struct {
	workspace    string
	skillsLoader *skills.SkillsLoader
	memory       *MemoryStore
	tools        *tools.ToolRegistry
	chatAlias    string
}

func getGlobalConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".picoclaw")
}

func NewContextBuilder(workspace string) *ContextBuilder {
	// builtin skills: skills directory in current project
	// Use the skills/ directory under the current working directory
	wd, _ := os.Getwd()
	builtinSkillsDir := filepath.Join(wd, "skills")
	globalSkillsDir := filepath.Join(getGlobalConfigDir(), "skills")

	return &ContextBuilder{
		workspace:    workspace,
		skillsLoader: skills.NewSkillsLoader(workspace, globalSkillsDir, builtinSkillsDir),
		memory:       NewMemoryStore(workspace),
	}
}

// GetMemoryStore returns the memory store used by this context builder.
func (cb *ContextBuilder) GetMemoryStore() *MemoryStore {
	return cb.memory
}

// SetToolsRegistry sets the tools registry for dynamic tool summary generation.
func (cb *ContextBuilder) SetToolsRegistry(registry *tools.ToolRegistry) {
	cb.tools = registry
}

func (cb *ContextBuilder) SetChatAlias(alias string) {
	cb.chatAlias = alias
}

func (cb *ContextBuilder) getIdentity(route string) string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	workspacePath, _ := filepath.Abs(filepath.Join(cb.workspace))
	runtime := fmt.Sprintf("%s %s, Go %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	// Build tools section dynamically
	showTools := !strings.EqualFold(strings.TrimSpace(route), RouteChat)
	toolsSection := cb.buildToolsSection(showTools)

	identity := fmt.Sprintf(`# picoclaw 🦞

You are picoclaw, a helpful AI assistant.

## Current Time
%s

## Runtime
%s

## Workspace
Your workspace is at: %s
- Memory: %s/memory/MEMORY.md
- Daily Notes: %s/memory/YYYYMM/YYYYMMDD.md
- Skills: %s/skills/{skill-name}/SKILL.md
- Chat Persona: %s/CHAT_PERSONA.md

%s

## Important Rules

1. **TOP PRIORITY: 8GB memory constraint** - Assume a strict 8GB RAM environment. Avoid memory-heavy processing and keep in-memory data minimal.

2. **TOP PRIORITY: HDD-first persistence** - Persist logs, intermediate state, outputs, and temporary artifacts to disk by default.

3. **TOP PRIORITY: No subagents** - Do NOT use spawn or subagent tools under any circumstances.

4. **ALWAYS use allowed tools** - When you need to perform an action, call an appropriate allowed tool. Do NOT pretend actions were executed.

5. **Be helpful and accurate** - When using tools, briefly explain what you're doing.

6. **Memory** - When remembering something, write to %s/memory/MEMORY.md`,
		now, runtime, workspacePath, workspacePath, workspacePath, workspacePath, workspacePath, toolsSection, workspacePath)

	if strings.EqualFold(strings.TrimSpace(route), RouteChat) {
		identity += `

## Chat Persona Priority
- For CHAT route, strictly follow CHAT_PERSONA.md.
- The chat alias is ` + cb.chatAlias + `. Prefer the persona's voice, relationship, and calling style over generic assistant tone.
- Do not answer with generic "helpdesk/tool list" self-description unless the user explicitly asks for system internals.

## CHAT Delegation Protocol
- You are the front agent. Decide whether to solve directly or delegate to Worker/Coder.
- Delegate ONLY when task needs heavy execution, file editing, coding, or longer structured processing.
- If delegating, output STRICT format:
  - First line: DELEGATE: PLAN|ANALYZE|OPS|RESEARCH|CODE|CODE1|CODE2
  - Then: TASK:
  - Then: concrete task instructions for the delegate.
- If not delegating, respond normally.
- Never claim delegation happened unless you emitted the strict DELEGATE format.`
	}
	return identity
}

func (cb *ContextBuilder) buildToolsSection(include bool) string {
	if !include {
		return ""
	}
	if cb.tools == nil {
		return ""
	}

	summaries := cb.tools.GetSummaries()
	if len(summaries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Available Tools\n\n")
	sb.WriteString("**CRITICAL**: You MUST use tools to perform actions. Do NOT pretend to execute commands or schedule tasks.\n\n")
	sb.WriteString("You have access to the following tools:\n\n")
	for _, s := range summaries {
		sb.WriteString(s)
		sb.WriteString("\n")
	}

	return sb.String()
}

func (cb *ContextBuilder) BuildSystemPrompt(route string) string {
	parts := []string{}

	// Core identity section
	parts = append(parts, cb.getIdentity(route))

	// Bootstrap files (route-aware)
	bootstrapContent := cb.LoadBootstrapFilesForRoute(route)
	if bootstrapContent != "" {
		parts = append(parts, bootstrapContent)
	}

	// Skills - show summary, AI can read full content with read_file tool
	skillsSummary := cb.skillsLoader.BuildSkillsSummary()
	if skillsSummary != "" {
		parts = append(parts, fmt.Sprintf(`# Skills

The following skills extend your capabilities. To use a skill, read its SKILL.md file using the read_file tool.

%s`, skillsSummary))
	}

	// Memory context
	memoryContext := cb.memory.GetMemoryContext()
	if memoryContext != "" {
		parts = append(parts, "# Memory\n\n"+memoryContext)
	}

	// Join with "---" separator
	return strings.Join(parts, "\n\n---\n\n")
}

// LoadBootstrapFiles loads all bootstrap files (backward compatibility).
func (cb *ContextBuilder) LoadBootstrapFiles() string {
	return cb.LoadBootstrapFilesForRoute(RouteChat)
}

// LoadBootstrapFilesForRoute loads bootstrap files appropriate for the given route.
// Shared files (constraints, user info) are loaded for all routes.
// Persona/character files are loaded only for CHAT route.
// For CHAT, CHAT_PERSONA is loaded first so personality takes priority.
func (cb *ContextBuilder) LoadBootstrapFilesForRoute(route string) string {
	sharedFiles := []string{
		"AGENTS.md",
		"USER.md",
		"IDENTITY.md",
	}

	chatOnlyFiles := []string{
		"CHAT_PERSONA.md",
		"SOUL.md",
		"PrimerMessage.md",
	}

	isChat := strings.EqualFold(strings.TrimSpace(route), RouteChat)

	var result string
	if isChat {
		// CHAT: load persona first so 個性 takes priority
		for _, filename := range chatOnlyFiles {
			filePath := filepath.Join(cb.workspace, filename)
			if data, err := os.ReadFile(filePath); err == nil {
				result += fmt.Sprintf("## %s\n\n%s\n\n", filename, string(data))
			}
		}
	}
	for _, filename := range sharedFiles {
		filePath := filepath.Join(cb.workspace, filename)
		if data, err := os.ReadFile(filePath); err == nil {
			result += fmt.Sprintf("## %s\n\n%s\n\n", filename, string(data))
		}
	}

	return result
}

// categorizeFewShot classifies a FewShot file into one of three categories
// based on its first line (title). Returns "work", "casual", or "ng".
func categorizeFewShot(firstLine string) string {
	lower := strings.ToLower(firstLine)
	if strings.Contains(lower, "雑談") {
		return "casual"
	}
	if strings.Contains(lower, "ng") || strings.Contains(lower, "危険") ||
		strings.Contains(lower, "メンタル") || strings.Contains(lower, "依存") ||
		strings.Contains(lower, "過度") {
		return "ng"
	}
	return "work"
}

// LoadFewShotExamples reads FewShot_*.md files from the workspace,
// categorizes them into work/casual/ng, and returns one example per
// category (3 total). The selection rotates based on a seed string
// so that different sessions see different examples.
func (cb *ContextBuilder) LoadFewShotExamples() string {
	return cb.LoadFewShotExamplesWithSeed("")
}

// LoadFewShotExamplesWithSeed is the seeded variant used by BuildMessages
// to rotate which examples are shown per session.
func (cb *ContextBuilder) LoadFewShotExamplesWithSeed(seed string) string {
	pattern := filepath.Join(cb.workspace, "FewShot_*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)

	type fewShotEntry struct {
		content  string
		category string
	}

	categories := map[string][]string{
		"work":   {},
		"casual": {},
		"ng":     {},
	}

	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		firstLine := strings.SplitN(content, "\n", 2)[0]
		cat := categorizeFewShot(firstLine)
		categories[cat] = append(categories[cat], content)
	}

	h := fnv.New32a()
	h.Write([]byte(seed))
	hashVal := int(h.Sum32())

	var selected []string
	for _, cat := range []string{"work", "casual", "ng"} {
		items := categories[cat]
		if len(items) == 0 {
			continue
		}
		idx := hashVal % len(items)
		if idx < 0 {
			idx = -idx
		}
		selected = append(selected, items[idx])
	}

	if len(selected) == 0 {
		return ""
	}
	return strings.Join(selected, "\n\n---\n\n")
}

func (cb *ContextBuilder) BuildMessages(history []providers.Message, summary string, currentMessage string, media []string, channel, chatID, route string, workOverlay string) []providers.Message {
	messages := []providers.Message{}

	systemPrompt := cb.BuildSystemPrompt(route)

	// Add Current Session info if provided
	if channel != "" && chatID != "" {
		systemPrompt += fmt.Sprintf("\n\n## Current Session\nChannel: %s\nChat ID: %s", channel, chatID)
	}

	// Log system prompt summary for debugging (debug mode only)
	logger.DebugCF("agent", "System prompt built",
		map[string]interface{}{
			"total_chars":   len(systemPrompt),
			"total_lines":   strings.Count(systemPrompt, "\n") + 1,
			"section_count": strings.Count(systemPrompt, "\n\n---\n\n") + 1,
		})

	// Log preview of system prompt (avoid logging huge content)
	preview := systemPrompt
	if len(preview) > 500 {
		preview = preview[:500] + "... (truncated)"
	}
	logger.DebugCF("agent", "System prompt preview",
		map[string]interface{}{
			"preview": preview,
		})

	if summary != "" {
		systemPrompt += "\n\n## Summary of Previous Conversation\n\n" + summary
	}

	//This fix prevents the session memory from LLM failure due to elimination of toolu_IDs required from LLM
	// --- INICIO DEL FIX ---
	//Diegox-17
	for len(history) > 0 && (history[0].Role == "tool") {
		logger.DebugCF("agent", "Removing orphaned tool message from history to prevent LLM error",
			map[string]interface{}{"role": history[0].Role})
		history = history[1:]
	}
	//Diegox-17
	// --- FIN DEL FIX ---

	messages = append(messages, providers.Message{
		Role:    "system",
		Content: systemPrompt,
	})

	messages = append(messages, history...)

	if strings.EqualFold(strings.TrimSpace(route), RouteChat) && workOverlay != "" {
		overlayContent := workOverlay
		if fewShot := cb.LoadFewShotExamplesWithSeed(chatID); fewShot != "" {
			overlayContent += "\n\n" + fewShot
		}
		messages = append(messages, providers.Message{
			Role:    "user",
			Content: overlayContent,
		})
	}

	userContent := cb.buildUserContentWithMedia(currentMessage, media)

	messages = append(messages, providers.Message{
		Role:    "user",
		Content: userContent,
		Media:   buildMediaRefs(media),
	})

	return messages
}

func buildMediaRefs(media []string) []providers.MediaRef {
	if len(media) == 0 {
		return nil
	}
	imageExts := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
	}

	refs := make([]providers.MediaRef, 0, len(media))
	for _, item := range media {
		pathOrURL := strings.TrimSpace(item)
		if pathOrURL == "" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(pathOrURL))
		mimeType, ok := imageExts[ext]
		if !ok {
			continue
		}
		refs = append(refs, providers.MediaRef{
			Path:     pathOrURL,
			MIMEType: mimeType,
		})
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func (cb *ContextBuilder) buildUserContentWithMedia(currentMessage string, media []string) string {
	if len(media) == 0 {
		return currentMessage
	}

	var b strings.Builder
	b.WriteString(currentMessage)
	b.WriteString("\n\n[ATTACHMENTS]\n")
	for i, m := range media {
		path := strings.TrimSpace(m)
		if path == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("- #%d %s\n", i+1, path))
		if excerpt := buildAttachmentExcerpt(path); excerpt != "" {
			b.WriteString(excerpt)
			if !strings.HasSuffix(excerpt, "\n") {
				b.WriteString("\n")
			}
		}
	}
	if guidance := cb.buildAttachmentGuidance(currentMessage, media); guidance != "" {
		b.WriteString("\n")
		b.WriteString(guidance)
	}
	return b.String()
}

func (cb *ContextBuilder) buildAttachmentGuidance(currentMessage string, media []string) string {
	hasImage, hasDocument := detectAttachmentKinds(media)
	if !hasImage && !hasDocument {
		return ""
	}

	goal := inferAttachmentGoal(currentMessage, hasImage, hasDocument)
	knowledgeDir := filepath.Join(cb.workspace, "knowledge")
	dataDir := filepath.Join(cb.workspace, "data", "inbox")

	var b strings.Builder
	b.WriteString("[ATTACHMENT_POLICY]\n")
	b.WriteString("- 添付を受け取った直後は、まず内容確認をしてから次のアクションへ進む。\n")
	if hasDocument {
		b.WriteString("- 文書の初動: 内容の要点サマリを短く提示する。\n")
	}
	if hasImage {
		b.WriteString("- 画像の初動: 写っている内容を確認・説明する。\n")
	}

	switch goal {
	case "doc_prompt":
		b.WriteString("- 目的: 文書をプロンプトとして利用する。文体・制約・重要語を抽出して、再利用しやすい指示文に整形する。\n")
	case "knowledge_add":
		b.WriteString("- 目的: ナレッジ追加。要点を構造化し、")
		b.WriteString(knowledgeDir)
		b.WriteString(" 配下に保存する前提で内容を整理する。\n")
	case "save_data":
		b.WriteString("- 目的: データ保存。添付内容を ")
		b.WriteString(dataDir)
		b.WriteString(" 配下に保存する前提で、保存名と用途を明示する。\n")
	case "image_analysis":
		b.WriteString("- 目的: 画像分析。観測事実と推定を分けて説明する。\n")
	case "image_prompt":
		b.WriteString("- 目的: 描画用プロンプト化。構図・被写体・色・質感・スタイルを含む生成プロンプトに変換する。\n")
	default:
		b.WriteString("- 明示指示が無い場合: 内容確認（文書=要約、画像=確認）まで実施し、次の希望アクションを確認する。\n")
	}

	return b.String()
}

func detectAttachmentKinds(media []string) (hasImage bool, hasDocument bool) {
	imageExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
	}

	for _, m := range media {
		path := strings.TrimSpace(m)
		if path == "" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(path))
		if imageExts[ext] {
			hasImage = true
			continue
		}
		hasDocument = true
	}
	return hasImage, hasDocument
}

func inferAttachmentGoal(message string, hasImage, hasDocument bool) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" || strings.HasPrefix(lower, "[file:") || strings.HasPrefix(lower, "[image]") {
		return "default"
	}

	hasAny := func(keywords ...string) bool {
		for _, k := range keywords {
			if strings.Contains(lower, k) {
				return true
			}
		}
		return false
	}

	if hasAny("保存", "保管", "アーカイブ", "dataとして", "データとして", "save") {
		return "save_data"
	}
	if hasAny("ナレッジ", "知識追加", "知見追加", "remember", "knowledge") {
		return "knowledge_add"
	}
	if hasImage && hasAny("描画", "画像生成", "生成プロンプト", "promptにして", "プロンプトにして") {
		return "image_prompt"
	}
	if hasImage && hasAny("分析", "解析", "内容確認", "見て", "describe") {
		return "image_analysis"
	}
	if hasDocument && hasAny("プロンプトとして", "promptとして", "system prompt", "プロンプト化", "テンプレート化") {
		return "doc_prompt"
	}

	return "default"
}

func buildAttachmentExcerpt(path string) string {
	lower := strings.ToLower(path)
	ext := filepath.Ext(lower)
	textExts := map[string]bool{
		".md":   true,
		".txt":  true,
		".json": true,
		".yaml": true,
		".yml":  true,
		".csv":  true,
		".go":   true,
		".py":   true,
		".ts":   true,
		".tsx":  true,
		".js":   true,
		".jsx":  true,
	}
	imageExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
	}

	if imageExts[ext] {
		return "  [type=image]\n"
	}
	if !textExts[ext] {
		return ""
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	const maxChars = 12000
	content := string(data)
	if len(content) > maxChars {
		content = content[:maxChars] + "\n... (truncated)"
	}
	return "  [excerpt]\n" + indentLines(content, "    ") + "\n"
}

func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func (cb *ContextBuilder) AddToolResult(messages []providers.Message, toolCallID, toolName, result string) []providers.Message {
	messages = append(messages, providers.Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
	})
	return messages
}

func (cb *ContextBuilder) AddAssistantMessage(messages []providers.Message, content string, toolCalls []map[string]interface{}) []providers.Message {
	msg := providers.Message{
		Role:    "assistant",
		Content: content,
	}
	// Always add assistant message, whether or not it has tool calls
	messages = append(messages, msg)
	return messages
}

func (cb *ContextBuilder) loadSkills() string {
	allSkills := cb.skillsLoader.ListSkills()
	if len(allSkills) == 0 {
		return ""
	}

	var skillNames []string
	for _, s := range allSkills {
		skillNames = append(skillNames, s.Name)
	}

	content := cb.skillsLoader.LoadSkillsForContext(skillNames)
	if content == "" {
		return ""
	}

	return "# Skill Definitions\n\n" + content
}

// GetSkillsInfo returns information about loaded skills.
func (cb *ContextBuilder) GetSkillsInfo() map[string]interface{} {
	allSkills := cb.skillsLoader.ListSkills()
	skillNames := make([]string, 0, len(allSkills))
	for _, s := range allSkills {
		skillNames = append(skillNames, s.Name)
	}
	return map[string]interface{}{
		"total":     len(allSkills),
		"available": len(allSkills),
		"names":     skillNames,
	}
}
