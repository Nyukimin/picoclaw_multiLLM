package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	personaFilename = "CHAT_PERSONA.md"
	maxPersonaSize  = 4096 // 4KB
)

// FilePersonaEditor はファイルベースのペルソナ読み書き実装
type FilePersonaEditor struct {
	filePath string
}

// NewFilePersonaEditor は新しい FilePersonaEditor を作成する
func NewFilePersonaEditor(workspaceDir string) *FilePersonaEditor {
	return &FilePersonaEditor{
		filePath: filepath.Join(workspaceDir, personaFilename),
	}
}

// ReadPersona は現在のペルソナ設定を読み込む
func (e *FilePersonaEditor) ReadPersona() (string, error) {
	data, err := os.ReadFile(e.filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read persona file: %w", err)
	}
	return string(data), nil
}

// WritePersona はペルソナ設定を書き込む
func (e *FilePersonaEditor) WritePersona(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("persona content must not be empty")
	}
	if len(content) > maxPersonaSize {
		return fmt.Errorf("persona content exceeds %d bytes", maxPersonaSize)
	}
	return os.WriteFile(e.filePath, []byte(content+"\n"), 0644)
}
