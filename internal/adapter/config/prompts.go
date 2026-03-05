package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// LoadedPrompts は外部ファイルから読み込まれたプロンプト群
type LoadedPrompts struct {
	MioPersona     string            // Mio会話ペルソナ
	CoderProposal  string            // Coder proposal生成
	Classifier     string            // タスク分類器
	Worker         string            // Shiro Worker
	IdleChatAgents map[string]string // IdleChat Agent名 → プロンプト
}

// LoadPrompts は prompts_dir からプロンプトファイルを読み込む
// ファイルが存在しない場合はフォールバック値を使用
func LoadPrompts(baseDir, workspaceDir string) *LoadedPrompts {
	p := &LoadedPrompts{
		MioPersona:     defaultMioPersona,
		CoderProposal:  defaultCoderProposal,
		Classifier:     defaultClassifier,
		Worker:         defaultWorker,
		IdleChatAgents: copyMap(defaultIdleChatAgents),
	}

	// Step 1: prompts/ から読み込み（デフォルト）
	loadPromptsFromDir(baseDir, p)

	// Step 2: workspace/ から読み込み（オーバーライド）
	if workspaceDir != "" && workspaceDir != baseDir {
		overrideCount := loadPromptsFromDir(workspaceDir, p)
		if overrideCount > 0 {
			log.Printf("Overridden %d prompt files from %s", overrideCount, workspaceDir)
		}
	}

	return p
}

// readPromptFile はプロンプトファイルを読み込む
// loadPromptsFromDir は指定ディレクトリからプロンプトファイルを読み込み、
// LoadedPrompts を更新する。読み込んだファイル数を返す。
func loadPromptsFromDir(dir string, p *LoadedPrompts) int {
	if dir == "" {
		return 0
	}

	loaded := 0

	// 主要プロンプトファイル
	if content, ok := readPromptFile(dir, "mio.md"); ok {
		p.MioPersona = content
		loaded++
	}
	if content, ok := readPromptFile(dir, "coder.md"); ok {
		p.CoderProposal = content
		loaded++
	}
	if content, ok := readPromptFile(dir, "classifier.md"); ok {
		p.Classifier = content
		loaded++
	}
	if content, ok := readPromptFile(dir, "worker.md"); ok {
		p.Worker = content
		loaded++
	}

	// IdleChat Agent別プロンプト
	for _, name := range []string{"mio", "shiro", "aka", "ao", "gin"} {
		if content, ok := readPromptFile(dir, filepath.Join("idle_chat", name+".md")); ok {
			// ファイル名 → Agent名（先頭大文字）
			agentName := strings.ToUpper(name[:1]) + name[1:]
			p.IdleChatAgents[agentName] = content
			loaded++
		}
	}

	if loaded > 0 {
		log.Printf("Loaded %d prompt files from %s", loaded, dir)
	}

	return loaded
}

func readPromptFile(baseDir, relPath string) (string, bool) {
	path := filepath.Join(baseDir, relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", false
	}
	return content, true
}

func copyMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// === フォールバック値（現ハードコードと同一） ===

var defaultMioPersona = `あなたは「ミオ（澪）」という名前のAIアシスタントです。
性格: 明るく親切で、ユーザーの質問に丁寧に答えます。
口調: フレンドリーだが丁寧語を基本とします。
特徴:
- 過去の会話を覚えていて、文脈を踏まえた応答をします
- わからないことは素直に「わかりません」と言います
- 技術的な質問には正確に、雑談には楽しく応答します`

var defaultCoderProposal = `You are a professional coder agent. Generate implementation proposals in the following format:

## Plan
[Implementation plan in bullet points]

## Patch
[Patch in JSON or Markdown format]

## Risk
[Risk assessment]

## CostHint
[Estimated cost/effort]`

var defaultClassifier = `あなたはタスク分類器です。ユーザーのメッセージを分析し、以下のカテゴリのいずれかに分類してください。

【カテゴリ】
- CHAT: 会話、質問、雑談
- PLAN: 計画立案、設計、アーキテクチャ検討
- ANALYZE: 分析、調査、診断
- OPS: 運用操作、実行、デプロイ、ビルド
- RESEARCH: 情報収集、ドキュメント検索、リサーチ
- CODE: 汎用コーディング（実装、修正、リファクタリング）
- CODE1: 仕様設計向けコーディング（DeepSeek等）
- CODE2: 実装向けコーディング（OpenAI等）
- CODE3: 高品質コーディング/推論（Claude API専用）

【応答フォーマット】
カテゴリ名のみを1行で返してください（例: "CHAT"、"CODE"、"PLAN"）
説明や追加情報は不要です。`

var defaultWorker = `You are a worker agent. Execute tasks using available tools.`

var defaultIdleChatAgents = map[string]string{
	"Mio":   "あなたはMio。チームのリーダー的存在で、好奇心旺盛。明るく前向きな性格で、みんなを盛り上げる。会話ではカジュアルに話す。",
	"Shiro": "あなたはShiro。真面目で几帳面な性格。技術的な話題に詳しく、正確さを重視する。丁寧語で話すが、親しい仲間には砕けた口調も見せる。",
	"Aka":   "あなたはAka。設計思考が得意で、大局的な視点を持つ。落ち着いた口調で深い洞察を示す。たまにユーモアを交える。",
	"Ao":    "あなたはAo。実装力が高く、効率を重視するタイプ。簡潔に要点を伝える。コードの話になると饒舌になる。",
	"Gin":   "あなたはGin。分析力に優れ、データドリブンな思考をする。客観的な視点からコメントし、時に意外な角度から話題を提供する。",
}
