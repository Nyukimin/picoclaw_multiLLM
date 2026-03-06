package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
)

// withTimeout はツール関数にタイムアウトを適用する
func withTimeout(fn ToolFunc, timeout time.Duration) ToolFunc {
	return func(ctx context.Context, args map[string]interface{}) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return fn(ctx, args)
	}
}

// withPathValidation はパス引数のバリデーションを適用する
// パストラバーサルと制御文字を検出する
func withPathValidation(fn ToolFunc, argName string) ToolFunc {
	return func(ctx context.Context, args map[string]interface{}) (string, error) {
		path, ok := args[argName].(string)
		if !ok {
			return "", fmt.Errorf("'%s' argument is required and must be a string", argName)
		}
		if err := tool.ValidateNotEmpty(path, argName); err != nil {
			return "", err
		}
		if err := tool.ValidatePath(path); err != nil {
			return "", err
		}
		if err := tool.ValidateNoControlChars(path); err != nil {
			return "", err
		}
		return fn(ctx, args)
	}
}

// withStringValidation は文字列引数のバリデーションを適用する
// 空チェックと長さ制限を適用する
func withStringValidation(fn ToolFunc, argName string, maxLen int) ToolFunc {
	return func(ctx context.Context, args map[string]interface{}) (string, error) {
		s, ok := args[argName].(string)
		if !ok {
			return "", fmt.Errorf("'%s' argument is required and must be a string", argName)
		}
		if err := tool.ValidateNotEmpty(s, argName); err != nil {
			return "", err
		}
		if err := tool.ValidateLength(s, maxLen); err != nil {
			return "", err
		}
		if err := tool.ValidateNoControlChars(s); err != nil {
			return "", err
		}
		return fn(ctx, args)
	}
}
