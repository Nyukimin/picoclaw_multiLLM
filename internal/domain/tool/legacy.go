package tool

import "context"

// LegacyRunner は RunnerV2 を V1 インターフェース (string, error) に変換するアダプター
// agent.ToolRunner インターフェースを満たす
type LegacyRunner struct {
	inner RunnerV2
}

// NewLegacyRunner は LegacyRunner を作成
func NewLegacyRunner(inner RunnerV2) *LegacyRunner {
	return &LegacyRunner{inner: inner}
}

// Execute は V2 の結果を string に変換して返す
func (l *LegacyRunner) Execute(ctx context.Context, toolName string, args map[string]any) (string, error) {
	resp, err := l.inner.ExecuteV2(ctx, toolName, args)
	if err != nil {
		return "", err
	}
	if resp.IsError() {
		return "", resp.Error
	}
	return resp.String(), nil
}

// List はメタデータからツール名一覧を返す
func (l *LegacyRunner) List(ctx context.Context) ([]string, error) {
	metas, err := l.inner.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(metas))
	for i, m := range metas {
		names[i] = m.ToolID
	}
	return names, nil
}

// ExecuteV2 は構造化レスポンスを返す（V2 インターフェース準拠）
func (l *LegacyRunner) ExecuteV2(ctx context.Context, toolName string, args map[string]any) (*ToolResponse, error) {
	return l.inner.ExecuteV2(ctx, toolName, args)
}
