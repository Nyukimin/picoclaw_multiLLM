package patch

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ParsePatch は JSON または Markdown 形式の patch を解析
func ParsePatch(patchStr string) ([]PatchCommand, error) {
	trimmed := strings.TrimSpace(patchStr)

	// JSON コードフェンス（```json ... ```）を許容
	if unwrapped, ok := unwrapCodeFence(trimmed); ok {
		trimmed = strings.TrimSpace(unwrapped)
	}

	// JSON 形式判定（配列/オブジェクト）
	if len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{') {
		return parseJSONPatch(trimmed)
	}

	// Markdown 形式判定（コードブロック検出）
	if strings.Contains(trimmed, "```go:") || strings.Contains(trimmed, "```bash") || strings.Contains(trimmed, "```git") {
		return parseMarkdownPatch(trimmed)
	}

	return nil, fmt.Errorf("unknown patch format: must be JSON array or Markdown code blocks")
}

func unwrapCodeFence(s string) (string, bool) {
	if !strings.HasPrefix(s, "```") || !strings.HasSuffix(s, "```") {
		return "", false
	}
	firstNL := strings.IndexByte(s, '\n')
	if firstNL == -1 {
		return "", false
	}
	header := strings.TrimSpace(strings.TrimPrefix(s[:firstNL], "```"))
	if strings.ToLower(header) != "json" {
		return "", false
	}
	body := s[firstNL+1:]
	lastFence := strings.LastIndex(body, "```")
	if lastFence == -1 {
		return "", false
	}
	return body[:lastFence], true
}

// parseJSONPatch は JSON 配列形式の patch を解析
func parseJSONPatch(patchStr string) ([]PatchCommand, error) {
	var raw any
	if err := json.Unmarshal([]byte(patchStr), &raw); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}
	items, err := extractCommandItems(raw)
	if err != nil {
		return nil, err
	}
	commands := make([]PatchCommand, 0, len(items))
	for i, item := range items {
		cmd, err := normalizeCommand(item)
		if err != nil {
			return nil, fmt.Errorf("command[%d]: %w", i, err)
		}
		commands = append(commands, cmd)
	}
	return commands, nil
}

func extractCommandItems(raw any) ([]map[string]any, error) {
	switch v := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(v))
		for i, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("command[%d]: object is required", i)
			}
			out = append(out, m)
		}
		return out, nil
	case map[string]any:
		for _, key := range []string{"commands", "steps", "patch"} {
			if vv, ok := v[key]; ok {
				arr, ok := vv.([]any)
				if !ok {
					return nil, fmt.Errorf("%s must be an array", key)
				}
				out := make([]map[string]any, 0, len(arr))
				for i, item := range arr {
					m, ok := item.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("%s[%d]: object is required", key, i)
					}
					out = append(out, m)
				}
				return out, nil
			}
		}
		return nil, fmt.Errorf("JSON object must include one of: commands, steps, patch")
	default:
		return nil, fmt.Errorf("JSON patch must be array/object")
	}
}

func normalizeCommand(m map[string]any) (PatchCommand, error) {
	hasTypeField := hasAnyKey(m, "type", "kind", "command_type")
	hasActionField := hasAnyKey(m, "action", "op", "operation")
	hasCanonicalTypeHints := hasAnyKey(m, "type", "action", "target", "content")
	hasCanonicalActionHints := hasAnyKey(m, "type", "action", "target", "content")
	cmd := PatchCommand{
		Type:     Type(asString(pick(m, "type", "kind", "command_type"))),
		Action:   Action(asString(pick(m, "action", "op", "operation"))),
		Target:   asString(pick(m, "target", "path", "file", "command", "cmd", "name")),
		Content:  asString(pick(m, "content", "text", "body")),
		Metadata: make(map[string]string),
	}
	if cmd.Target == "" {
		// 例: {"run":"echo hello"}
		if run := asString(m["run"]); run != "" {
			cmd.Target = run
		}
	}
	inferDefaults(&cmd, hasTypeField, hasActionField, !hasCanonicalTypeHints, !hasCanonicalActionHints)
	if cmd.Type == "" {
		return PatchCommand{}, fmt.Errorf("type is required")
	}
	if cmd.Action == "" {
		return PatchCommand{}, fmt.Errorf("action is required")
	}
	if cmd.Target == "" {
		return PatchCommand{}, fmt.Errorf("target is required")
	}
	return cmd, nil
}

func inferDefaults(cmd *PatchCommand, hasTypeField, hasActionField, allowTypeInference, allowActionInference bool) {
	if cmd.Type == "" && !hasTypeField && allowTypeInference {
		if cmd.Content != "" || looksLikeFilePath(cmd.Target) || cmd.Action == ActionCreate || cmd.Action == ActionUpdate || cmd.Action == ActionDelete || cmd.Action == ActionAppend {
			cmd.Type = TypeFileEdit
		} else if strings.HasPrefix(cmd.Target, "git ") || cmd.Action == ActionAdd || cmd.Action == ActionCommit || cmd.Action == ActionReset || cmd.Action == ActionCheckout {
			cmd.Type = TypeGitOperation
		} else {
			cmd.Type = TypeShellCommand
		}
	}

	if cmd.Action == "" && !hasActionField && allowActionInference {
		switch cmd.Type {
		case TypeFileEdit:
			cmd.Action = ActionUpdate
		case TypeGitOperation:
			cmd.Action = ActionRun
		default:
			cmd.Action = ActionRun
		}
	}
}

func hasAnyKey(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func looksLikeFilePath(s string) bool {
	if s == "" {
		return false
	}
	if strings.Contains(s, "/") || strings.Contains(s, "\\") {
		return true
	}
	return strings.Contains(s, ".")
}

func pick(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return ""
	}
}

// parseMarkdownPatch は Markdown コードブロック形式の patch を解析
func parseMarkdownPatch(patchStr string) ([]PatchCommand, error) {
	// 位置情報付きコマンド
	type positionedCommand struct {
		pos int
		cmd PatchCommand
	}
	var positioned []positionedCommand

	// 正規表現: ```言語:ファイルパス\n内容\n```
	// 例: ```go:src/main.go\npackage main\n```
	// (?s) フラグ: DOTALL モード（.が改行にもマッチ）
	reCodeBlock := regexp.MustCompile(`(?s)` + "```" + `([a-z]+):([^\n]+)\n(.*?)` + "```")
	matches := reCodeBlock.FindAllStringSubmatchIndex(patchStr, -1)

	for _, match := range matches {
		lang := patchStr[match[2]:match[3]]
		filePath := strings.TrimSpace(patchStr[match[4]:match[5]])
		content := patchStr[match[6]:match[7]]

		// ファイル編集コマンドに変換
		cmd := PatchCommand{
			Type:    TypeFileEdit,
			Action:  ActionUpdate, // デフォルトはupdate（既存ファイル上書き）
			Target:  filePath,
			Content: content,
			Metadata: map[string]string{
				"language": lang,
			},
		}
		positioned = append(positioned, positionedCommand{pos: match[0], cmd: cmd})
	}

	// 正規表現: ```bash\nコマンド\n```
	// 例: ```bash\ngo test ./...\n```
	// (?s) フラグ: DOTALL モード（複数行コマンド対応）
	reShellBlock := regexp.MustCompile(`(?s)` + "```bash\n(.*?)" + "```")
	shellMatches := reShellBlock.FindAllStringSubmatchIndex(patchStr, -1)

	for _, match := range shellMatches {
		command := strings.TrimSpace(patchStr[match[2]:match[3]])

		cmd := PatchCommand{
			Type:   TypeShellCommand,
			Action: ActionRun,
			Target: command,
		}
		positioned = append(positioned, positionedCommand{pos: match[0], cmd: cmd})
	}

	// 正規表現: ```git\nコマンド\n```
	// 例: ```git\nadd .\ncommit -m "message"\n```
	reGitBlock := regexp.MustCompile(`(?s)` + "```git\n(.*?)" + "```")
	gitMatches := reGitBlock.FindAllStringSubmatchIndex(patchStr, -1)

	for _, match := range gitMatches {
		gitCommand := strings.TrimSpace(patchStr[match[2]:match[3]])

		cmd := PatchCommand{
			Type:   TypeGitOperation,
			Action: ActionRun, // Git操作は詳細パースが必要
			Target: gitCommand,
		}
		positioned = append(positioned, positionedCommand{pos: match[0], cmd: cmd})
	}

	if len(positioned) == 0 {
		return nil, fmt.Errorf("no valid code blocks found in Markdown patch")
	}

	// 出現順にソート
	sort.Slice(positioned, func(i, j int) bool {
		return positioned[i].pos < positioned[j].pos
	})

	// コマンドリストに変換
	commands := make([]PatchCommand, len(positioned))
	for i, p := range positioned {
		commands[i] = p.cmd
	}

	return commands, nil
}
