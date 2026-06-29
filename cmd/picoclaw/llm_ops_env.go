package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const llmOpsTokenEnvName = "LLM_OPS_TOKEN"

func loadLLMOpsTokenEnvFile() (bool, string, error) {
	return loadLLMOpsTokenEnvFileAt(defaultLLMOpsEnvPath())
}

func defaultLLMOpsEnvPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return filepath.Join(".picoclaw", "llm_ops.env")
	}
	return filepath.Join(homeDir, ".picoclaw", "llm_ops.env")
}

func loadLLMOpsTokenEnvFileAt(path string) (bool, string, error) {
	if strings.TrimSpace(os.Getenv(llmOpsTokenEnvName)) != "" {
		return false, path, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, path, nil
		}
		return false, path, err
	}
	for i, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if i == 0 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != llmOpsTokenEnvName {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if strings.TrimSpace(value) == "" {
			return false, path, nil
		}
		if err := os.Setenv(llmOpsTokenEnvName, value); err != nil {
			return false, path, err
		}
		return true, path, nil
	}
	return false, path, nil
}
