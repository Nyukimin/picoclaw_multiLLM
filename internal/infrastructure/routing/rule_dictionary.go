package routing

import (
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// RuleDictionary はキーワードベースのルール辞書実装
type RuleDictionary struct {
	rules []rule
}

// rule は単一のルールを表す
type rule struct {
	keywords   []string
	route      routing.Route
	confidence float64
}

// NewRuleDictionary は新しいRuleDictionaryを作成
func NewRuleDictionary() *RuleDictionary {
	return &RuleDictionary{
		rules: []rule{
			// CODE関連キーワード
			{
				keywords:   []string{"実装して", "修正して", "リファクタリング", "テストを追加", "コードを書", "バグを直", "関数を作"},
				route:      routing.RouteCODE,
				confidence: 0.85,
			},
			// PLAN関連キーワード
			{
				keywords:   []string{"計画", "プラン", "設計して", "アーキテクチャ", "方針を決"},
				route:      routing.RoutePLAN,
				confidence: 0.85,
			},
			// ANALYZE関連キーワード
			{
				keywords:   []string{"分析して", "調査して", "解析して", "診断して", "レビューして"},
				route:      routing.RouteANALYZE,
				confidence: 0.85,
			},
			// OPS関連キーワード
			{
				keywords:   []string{"実行して", "起動して", "デプロイ", "ビルドして", "停止して", "再起動"},
				route:      routing.RouteOPS,
				confidence: 0.85,
			},
			// RESEARCH関連キーワード
			{
				keywords:   []string{"調べて", "検索して", "リサーチ", "情報を集", "ドキュメントを探"},
				route:      routing.RouteRESEARCH,
				confidence: 0.85,
			},
		},
	}
}

// Match はタスクメッセージをルールと照合
func (d *RuleDictionary) Match(t task.Task) (routing.Route, float64, bool) {
	message := strings.ToLower(t.UserMessage())

	// ルールを順番にチェック
	for _, rule := range d.rules {
		for _, keyword := range rule.keywords {
			if strings.Contains(message, strings.ToLower(keyword)) {
				return rule.route, rule.confidence, true
			}
		}
	}

	return "", 0.0, false
}
