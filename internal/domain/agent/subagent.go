package agent

// SubagentTask はサブエージェントに渡すタスク
type SubagentTask struct {
	AgentName    string // サブエージェント名（ログ・識別用）
	Instruction  string // タスク指示文
	SystemPrompt string // システムプロンプト（空の場合はデフォルト使用）
}

// SubagentResult はサブエージェント実行結果
type SubagentResult struct {
	AgentName  string // 実行したサブエージェント名
	Output     string // 最終出力テキスト
	Iterations int    // ReActループの反復回数
}
