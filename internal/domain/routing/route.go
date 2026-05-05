package routing

// Route はルーティング先を表す型
type Route string

// ルーティングカテゴリの定数定義
const (
	RouteCHAT     Route = "CHAT"     // 会話・意思決定
	RoutePLAN     Route = "PLAN"     // 計画策定
	RouteANALYZE  Route = "ANALYZE"  // 分析
	RouteOPS      Route = "OPS"      // 運用操作
	RouteRESEARCH Route = "RESEARCH" // 調査
	RouteWILD     Route = "WILD"     // 創作・画像プロンプト・雰囲気抽出
	RouteCODE     Route = "CODE"     // コーディング（汎用）
	RouteCODE1    Route = "CODE1"    // 仕様設計向け（スロット1）
	RouteCODE2    Route = "CODE2"    // 実装向け（スロット2）
	RouteCODE3    Route = "CODE3"    // 高品質コーディング/推論（スロット3）
	RouteCODE4    Route = "CODE4"    // 高速コーディング/実験（スロット4）
)

// String はRouteの文字列表現を返す
func (r Route) String() string {
	return string(r)
}

// IsCoderRoute はCoderルートかを判定
func (r Route) IsCoderRoute() bool {
	return r == RouteCODE || r == RouteCODE1 || r == RouteCODE2 || r == RouteCODE3 || r == RouteCODE4
}

// RouteToCoderSlot は Route を Coder スロット名にマッピングする。
// CODE（汎用）は coder1 にフォールバック。
// Coder ルートでない場合は空文字列を返す。
func (r Route) RouteToCoderSlot() string {
	switch r {
	case RouteCODE, RouteCODE1:
		return "coder1"
	case RouteCODE2:
		return "coder2"
	case RouteCODE3:
		return "coder3"
	case RouteCODE4:
		return "coder4"
	default:
		return ""
	}
}

// Decision はルーティング決定の結果を表す
type Decision struct {
	Route      Route   // 決定されたルート
	Confidence float64 // 確信度（0.0 - 1.0）
	Reason     string  // 決定理由
}

// NewDecision は新しいDecisionを作成
func NewDecision(route Route, confidence float64, reason string) Decision {
	return Decision{
		Route:      route,
		Confidence: confidence,
		Reason:     reason,
	}
}
