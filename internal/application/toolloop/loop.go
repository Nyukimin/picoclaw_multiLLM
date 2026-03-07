package toolloop

import (
	"context"
	"fmt"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
)

// Config はツールループの設定
type Config struct {
	MaxIterations int // 最大反復回数（0の場合デフォルト10）
}

func (c Config) maxIterations() int {
	if c.MaxIterations > 0 {
		return c.MaxIterations
	}
	return 10
}

// Run はReActループを実行する
//
// フロー:
//  1. provider.Chat(messages + tools) を呼び出す
//  2. レスポンスに tool_calls がある場合:
//     a. 各 tool_call を toolRunner.ExecuteV2() で実行
//     b. 実行結果を role="tool" メッセージとして messages に追加
//     c. 1. に戻る
//  3. tool_calls がない場合（通常テキスト応答）:
//     → レスポンスの Content を返す
//  4. MaxIterations を超えた場合:
//     → 最後のレスポンスの Content を返す（途中打ち切り）
func Run(ctx context.Context, provider llm.ToolCallingProvider,
	toolRunner tool.RunnerV2, toolDefs []llm.ToolDefinition,
	messages []llm.ChatMessage, cfg Config) (string, error) {

	maxIter := cfg.maxIterations()

	for i := 0; i < maxIter; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		resp, err := provider.Chat(ctx, llm.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			return "", fmt.Errorf("chat error at iteration %d: %w", i, err)
		}

		// assistantメッセージを履歴に追加
		messages = append(messages, resp.Message)

		// tool_calls がなければ最終応答
		if len(resp.Message.ToolCalls) == 0 {
			return resp.Message.Content, nil
		}

		// 各ツール呼び出しを実行
		for _, tc := range resp.Message.ToolCalls {
			result, err := toolRunner.ExecuteV2(ctx, tc.Function.Name, tc.Function.Arguments)

			var content string
			if err != nil {
				content = fmt.Sprintf("Error: %v", err)
			} else if result != nil && result.Error == nil {
				content = fmt.Sprintf("%v", result.Result)
			} else if result != nil && result.Error != nil {
				content = fmt.Sprintf("Error: %s", result.Error.Message)
			} else {
				content = "Error: nil response"
			}

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: tc.ID,
			})
		}
	}

	// MaxIterations超過 → 最後のassistantメッセージを返す
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			return messages[i].Content, nil
		}
	}

	return "", fmt.Errorf("max iterations (%d) exceeded with no assistant response", maxIter)
}
