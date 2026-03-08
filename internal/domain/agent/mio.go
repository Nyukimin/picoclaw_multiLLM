package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// ConversationManager はKB保存用のインターフェース（Phase 4.2）
type ConversationManager interface {
	SearchKB(ctx context.Context, domain string, query string, topK int) ([]*conversation.Document, error)
	SaveWebSearchToKB(ctx context.Context, domain string, query string, results []WebSearchResult) error
}

// PersonaEditor はペルソナファイルの読み書きを抽象化する
type PersonaEditor interface {
	ReadPersona() (string, error)
	WritePersona(content string) error
}

// WebSearchResult はWeb検索結果（ToolRunner の GoogleSearchItem と互換）
type WebSearchResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

// MioAgent は Chat（会話・意思決定）を担当するエンティティ
type MioAgent struct {
	llmProvider        llm.LLMProvider
	classifier         Classifier
	ruleDictionary     RuleDictionary
	toolRunner         ToolRunner
	mcpClient          MCPClient
	conversationEngine conversation.ConversationEngine // v5.1: 会話エンジン（nilを許容）
	conversationMgr    ConversationManager             // Phase 4.2: KB自動保存用（nilを許容）
	personaEditor      PersonaEditor                   // ペルソナ自己編集用（nilを許容）
}

// NewMioAgent は新しいMioAgentを作成
func NewMioAgent(
	llmProvider llm.LLMProvider,
	classifier Classifier,
	ruleDictionary RuleDictionary,
	toolRunner ToolRunner,
	mcpClient MCPClient,
	conversationEngine conversation.ConversationEngine, // v5.1: ConversationEngine（nilを許容）
) *MioAgent {
	return &MioAgent{
		llmProvider:        llmProvider,
		classifier:         classifier,
		ruleDictionary:     ruleDictionary,
		toolRunner:         toolRunner,
		mcpClient:          mcpClient,
		conversationEngine: conversationEngine,
		conversationMgr:    nil, // WithConversationManager() でセット
	}
}

// WithConversationManager はConversationManagerを設定（Phase 4.2 KB自動保存用）
func (m *MioAgent) WithConversationManager(mgr ConversationManager) *MioAgent {
	m.conversationMgr = mgr
	return m
}

// WithPersonaEditor はPersonaEditorを設定（ペルソナ自己編集用）
func (m *MioAgent) WithPersonaEditor(editor PersonaEditor) *MioAgent {
	m.personaEditor = editor
	return m
}

// DecideAction はMioによる委譲判断（4段階優先順位）
func (m *MioAgent) DecideAction(ctx context.Context, t task.Task) (routing.Decision, error) {
	// 優先度1: 明示コマンド
	if explicitRoute := m.parseExplicitCommand(t.UserMessage()); explicitRoute != "" {
		return routing.NewDecision(explicitRoute, 1.0, "Explicit command"), nil
	}

	// 優先度2: ルール辞書
	if route, confidence, matched := m.ruleDictionary.Match(t); matched {
		return routing.NewDecision(route, confidence, "Rule dictionary match"), nil
	}

	// 優先度3: 安全側フォールバック（CHAT）
	// 技術的キーワードがルール辞書で捕捉されなかったメッセージは会話として処理
	// LLM分類器は精度向上のためのオプション（レイテンシ優先で現在はスキップ）
	return routing.NewDecision(routing.RouteCHAT, 0.7, "No rule match, default to CHAT"), nil
}

// Chat は会話を実行（v5.1: ConversationEngine + キーワードベース自動Web検索）
func (m *MioAgent) Chat(ctx context.Context, t task.Task) (string, error) {
	userMessage := t.UserMessage()

	// === v5.1: ConversationEngine による RecallPack 生成 ===
	var messages []llm.Message
	var recallPack *conversation.RecallPack
	if m.conversationEngine != nil {
		var err error
		recallPack, err = m.conversationEngine.BeginTurn(ctx, t.ChatID(), userMessage)
		if err != nil {
			fmt.Printf("WARN: BeginTurn failed: %v\n", err)
		}
		if recallPack != nil {
			// RecallPack からプロンプトメッセージを生成（system prompt + 過去文脈 + 会話履歴）
			messages = recallPack.ToPromptMessages()
		}
	}

	// ペルソナ調整意図を検出 → 自己編集
	if m.personaEditor != nil && detectPersonaEditIntent(userMessage) {
		result, err := m.editPersona(ctx, userMessage)
		if err != nil {
			log.Printf("[Mio] Persona edit failed: %v", err)
			// フォールバック: 通常の会話として処理を続行
		} else {
			// EndTurn で会話履歴に記録
			if m.conversationEngine != nil {
				if err := m.conversationEngine.EndTurn(ctx, t.ChatID(), userMessage, result); err != nil {
					fmt.Printf("WARN: EndTurn failed: %v\n", err)
				}
			}
			return result, nil
		}
	}

	// キーワードベースでWeb検索が必要か判定
	needsSearch := needsWebSearch(userMessage)

	// Web検索を実行してコンテキストに追加
	if needsSearch && m.toolRunner != nil {
		searchResult, err := m.executeWebSearch(ctx, userMessage)
		if err == nil && searchResult != "" {
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: "以下はWeb検索の結果です。この情報を参考にして質問に答えてください:\n\n" + searchResult,
			})
		}
	}

	latestOther := ""
	if recallPack != nil {
		selfCtx, otherCtx := buildAttributionContextsFromShort(recallPack.ShortContext, conversation.SpeakerMio, 5)
		latestOther = latestOtherMessageFromShort(recallPack.ShortContext, conversation.SpeakerMio)
		messages = append(messages, llm.Message{
			Role: "user",
			Content: fmt.Sprintf(
				"発言帰属ガード:\n- あなたはmio。\n- 自分の過去発言(要約): %s\n- 他者の発言(要約): %s\n要件: 他者の発言を自分の新規アイデアとして扱わない。既出アイデアに触れる場合は発言者を明示する。",
				strings.Join(selfCtx, " / "),
				strings.Join(otherCtx, " / "),
			),
		})
	}

	// ユーザーメッセージを最後に追加
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})

	req := llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   512,
		Temperature: 0.7,
		OnToken:     llm.StreamCallbackFromContext(ctx),
	}

	resp, err := m.llmProvider.Generate(ctx, req)
	if err != nil {
		return "", err
	}

	response := strings.TrimSpace(resp.Content)
	if violatesAttributionInChat(response, latestOther) {
		retryMessages := append([]llm.Message{}, messages...)
		retryMessages = append(retryMessages, llm.Message{
			Role:    "user",
			Content: "直前の返答は発言帰属が曖昧です。誰のアイデアかを明示して1回だけ言い直してください。",
		})
		retryResp, retryErr := m.llmProvider.Generate(ctx, llm.GenerateRequest{
			Messages:    retryMessages,
			MaxTokens:   512,
			Temperature: 0.7,
			OnToken:     llm.StreamCallbackFromContext(ctx),
		})
		if retryErr == nil && strings.TrimSpace(retryResp.Content) != "" {
			response = strings.TrimSpace(retryResp.Content)
		}
	}

	// === v5.1: EndTurn（Store） ===
	if m.conversationEngine != nil {
		if err := m.conversationEngine.EndTurn(ctx, t.ChatID(), userMessage, response); err != nil {
			fmt.Printf("WARN: EndTurn failed: %v\n", err)
		}
	}

	return response, nil
}

// executeWebSearch はWeb検索を実行（内部ヘルパー + Phase 4.2 KB自動保存）
func (m *MioAgent) executeWebSearch(ctx context.Context, query string) (string, error) {
	if m.toolRunner == nil {
		return "", fmt.Errorf("toolRunner not available")
	}

	// クエリから検索キーワードを抽出（不要な部分を除去）
	cleanedQuery := cleanSearchQuery(query)

	args := map[string]interface{}{
		"query": cleanedQuery,
	}

	// V2ツールランナーで構造化データを取得
	toolResp, err := m.toolRunner.ExecuteV2(ctx, "web_search", args)
	if err != nil {
		return "", err
	}

	if toolResp.IsError() {
		return "", fmt.Errorf("%s", toolResp.Error.Message)
	}

	// 表示用の文字列結果
	result := toolResp.String()

	// Phase 4.2: KB自動保存（ConversationManager が設定されている場合）
	if m.conversationMgr != nil && toolResp.Metadata != nil {
		if searchItems, ok := toolResp.Metadata["search_items"].([]interface{}); ok {
			// GoogleSearchItem → WebSearchResult に変換
			webResults := make([]WebSearchResult, 0, len(searchItems))
			for _, item := range searchItems {
				if itemMap, ok := item.(map[string]interface{}); ok {
					webResults = append(webResults, WebSearchResult{
						Title:   getStringField(itemMap, "title"),
						Link:    getStringField(itemMap, "link"),
						Snippet: getStringField(itemMap, "snippet"),
					})
				}
			}

			// KB保存（エラーはログのみ、検索結果は返す）
			domain := inferDomain(query) // クエリから domain を推定
			if err := m.conversationMgr.SaveWebSearchToKB(ctx, domain, cleanedQuery, webResults); err != nil {
				fmt.Printf("WARN: SaveWebSearchToKB failed: %v\n", err)
			}
		}
	}

	return result, nil
}

// getStringField は map から文字列フィールドを安全に取得
func getStringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// cleanSearchQuery は検索クエリから不要な部分を除去
func cleanSearchQuery(query string) string {
	// 除去するパターン（質問形式の語尾など）
	removePatterns := []string{
		"について教えて", "を教えて", "教えて",
		"について調べて", "を調べて", "調べて",
		"について検索", "を検索", "検索して",
		"とは", "って何", "ってなに",
	}

	cleaned := query
	for _, pattern := range removePatterns {
		cleaned = strings.Replace(cleaned, pattern, "", -1)
	}

	return strings.TrimSpace(cleaned)
}

// parseExplicitCommand は明示コマンドを解析
func (m *MioAgent) parseExplicitCommand(message string) routing.Route {
	// 長いコマンドから順にチェック（/code3 を /code より先に判定）
	commands := []struct {
		cmd   string
		route routing.Route
	}{
		{"/analyze", routing.RouteANALYZE},
		{"/research", routing.RouteRESEARCH},
		{"/code3", routing.RouteCODE3},
		{"/code2", routing.RouteCODE2},
		{"/code1", routing.RouteCODE1},
		{"/code", routing.RouteCODE},
		{"/plan", routing.RoutePLAN},
		{"/chat", routing.RouteCHAT},
		{"/ops", routing.RouteOPS},
	}

	trimmed := strings.TrimSpace(message)
	for _, c := range commands {
		if strings.HasPrefix(trimmed, c.cmd) {
			return c.route
		}
	}

	return ""
}

// needsWebSearch はWeb検索が必要かをキーワードベースで判定する
// 明示的な検索指示キーワード OR 時事・最新情報系のキーワードでトリガー
func needsWebSearch(message string) bool {
	// 明示的な検索意図
	directKeywords := []string{
		"教えて", "調べて", "検索", "とは",
	}
	// 時事・最新情報・鮮度依存
	timelyKeywords := []string{
		"最新", "ニュース", "今日", "昨日", "今週", "今月", "今年",
		"最近", "現在", "いま", "速報", "話題",
		"どうなった", "どうなってる", "進捗", "状況",
		"2024", "2025", "2026", "2027",
		"予定", "リリース", "発売", "公開",
		"結果", "スコア", "勝った", "負けた",
		"値段", "価格", "相場", "株価", "為替",
		"天気", "気温",
	}
	// トピック系（「〜について」で情報を求めている）
	topicKeywords := []string{
		"について",
	}

	for _, kw := range directKeywords {
		if strings.Contains(message, kw) {
			return true
		}
	}
	for _, kw := range timelyKeywords {
		if strings.Contains(message, kw) {
			return true
		}
	}
	for _, kw := range topicKeywords {
		if strings.Contains(message, kw) {
			return true
		}
	}
	return false
}

// detectPersonaEditIntent はペルソナ調整意図を検出する
// トピックキーワード AND アクションキーワードの両方にマッチした場合のみ true
func detectPersonaEditIntent(message string) bool {
	topicKeywords := []string{
		"ペルソナ", "キャラ", "口調", "語尾", "喋り方", "話し方",
		"敬語", "タメ口", "カジュアル", "フォーマル",
		"テンション",
	}
	actionKeywords := []string{
		"変えて", "にして", "やめて", "直して", "調整", "修正",
		"書き換え", "編集", "更新", "して",
	}

	hasTopic := false
	for _, kw := range topicKeywords {
		if strings.Contains(message, kw) {
			hasTopic = true
			break
		}
	}
	if !hasTopic {
		return false
	}

	for _, kw := range actionKeywords {
		if strings.Contains(message, kw) {
			return true
		}
	}
	return false
}

// editPersona はペルソナファイルを LLM で書き換える
func (m *MioAgent) editPersona(ctx context.Context, userMessage string) (string, error) {
	current, err := m.personaEditor.ReadPersona()
	if err != nil {
		return "", fmt.Errorf("read persona: %w", err)
	}

	log.Printf("[Mio] Persona edit requested: %q", truncateLog(userMessage, 100))
	log.Printf("[Mio] Persona before: %q", truncateLog(current, 100))

	// LLM にペルソナ書き換えを依頼
	prompt := fmt.Sprintf(
		"以下は現在のペルソナ設定です:\n\n%s\n\n"+
			"ユーザーの要求: %s\n\n"+
			"上記の要求に基づいて、ペルソナ設定を書き換えてください。\n"+
			"形式（マークダウン）と基本構造は維持してください。\n"+
			"書き換えた設定のみを出力してください。説明や前置きは不要です。",
		current, userMessage,
	)

	req := llm.GenerateRequest{
		Messages:    []llm.Message{{Role: "user", Content: prompt}},
		MaxTokens:   1024,
		Temperature: 0.3,
	}

	resp, err := m.llmProvider.Generate(ctx, req)
	if err != nil {
		return "", fmt.Errorf("generate new persona: %w", err)
	}

	newPersona := strings.TrimSpace(resp.Content)
	if newPersona == "" {
		return "", fmt.Errorf("LLM returned empty persona")
	}

	log.Printf("[Mio] Persona after: %q", truncateLog(newPersona, 100))

	if err := m.personaEditor.WritePersona(newPersona); err != nil {
		return "", fmt.Errorf("write persona: %w", err)
	}

	return "ペルソナ設定を更新しました。次の会話から反映されます。", nil
}

// truncateLog はログ用に文字列を切り詰める
func truncateLog(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// inferDomain はクエリから適切な domain を推定する
func inferDomain(query string) string {
	query = strings.ToLower(query)

	// プログラミング関連
	programmingKeywords := []string{
		"プログラミング", "コード", "言語", "関数", "変数", "クラス",
		"python", "go", "rust", "javascript", "java", "c++",
		"アルゴリズム", "データ構造", "フレームワーク", "ライブラリ",
	}
	for _, kw := range programmingKeywords {
		if strings.Contains(query, kw) {
			return "programming"
		}
	}

	// エンターテイメント関連
	entertainmentKeywords := []string{
		"映画", "ドラマ", "アニメ", "漫画", "ゲーム", "音楽",
		"俳優", "声優", "監督", "アーティスト",
	}
	for _, kw := range entertainmentKeywords {
		if strings.Contains(query, kw) {
			return "entertainment"
		}
	}

	// 料理関連
	cookingKeywords := []string{
		"料理", "レシピ", "食材", "調理", "食べ物", "飲み物",
		"レストラン", "カフェ",
	}
	for _, kw := range cookingKeywords {
		if strings.Contains(query, kw) {
			return "cooking"
		}
	}

	// 科学・技術関連
	scienceKeywords := []string{
		"科学", "物理", "化学", "生物", "数学", "天文",
		"技術", "工学", "AI", "機械学習",
		"量子", "相対性", "宇宙", "素粒子", "エネルギー",
	}
	for _, kw := range scienceKeywords {
		if strings.Contains(query, kw) {
			return "science"
		}
	}

	// デフォルトは general
	return "general"
}

func buildAttributionContextsFromShort(short []conversation.Message, self conversation.Speaker, limit int) ([]string, []string) {
	selfCtx := make([]string, 0, limit)
	otherCtx := make([]string, 0, limit)
	for i := len(short) - 1; i >= 0 && (len(selfCtx) < limit || len(otherCtx) < limit); i-- {
		msg := strings.TrimSpace(short[i].Msg)
		if msg == "" {
			continue
		}
		line := truncateLog(msg, 80)
		if short[i].Speaker == self {
			if len(selfCtx) < limit {
				selfCtx = append(selfCtx, line)
			}
			continue
		}
		if len(otherCtx) < limit {
			otherCtx = append(otherCtx, fmt.Sprintf("%s: %s", short[i].Speaker, line))
		}
	}
	if len(selfCtx) == 0 {
		selfCtx = append(selfCtx, "なし")
	}
	if len(otherCtx) == 0 {
		otherCtx = append(otherCtx, "なし")
	}
	return selfCtx, otherCtx
}

func latestOtherMessageFromShort(short []conversation.Message, self conversation.Speaker) string {
	for i := len(short) - 1; i >= 0; i-- {
		if short[i].Speaker == self {
			continue
		}
		return strings.TrimSpace(short[i].Msg)
	}
	return ""
}

func violatesAttributionInChat(response, latestOther string) bool {
	resp := normalizeAttributionText(response)
	other := normalizeAttributionText(latestOther)
	if resp == "" || other == "" || resp != other {
		return false
	}
	lower := strings.ToLower(response)
	return !strings.Contains(lower, "あなた") && !strings.Contains(lower, "君") && !strings.Contains(lower, "相手")
}

func normalizeAttributionText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "　", "")
	s = strings.ReplaceAll(s, "。", "")
	s = strings.ReplaceAll(s, "、", "")
	s = strings.ReplaceAll(s, "！", "")
	s = strings.ReplaceAll(s, "？", "")
	return s
}
