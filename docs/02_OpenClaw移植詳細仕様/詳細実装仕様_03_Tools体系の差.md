# 詳細実装仕様 03: Tools体系の差

**作成日**: 2026-03-09  
**ステータス**: Draft  
**親仕様**: `docs/実装仕様_OpenClaw移植_v1.md`

---

## 0. OpenClaw原典の理解

- Tools: <https://docs.openclaw.ai/tools>
- Features: <https://docs.openclaw.ai/concepts/features>
- OSS実装: <https://github.com/openclaw/openclaw>

OpenClawは「ツールを第一級の実行単位」として、宣言・実行・監査を分離している。

---

## 1. 現状差分

- PicoClaw現状: ツールは存在するが宣言メタ/能力表/実行ポリシーが分散。
- 差分の本質: Tool Registry と能力カタログが不足。

---

## 2. 実装対象

1. `ToolManifest` 標準化（name/version/input/output/side_effect）
2. `ToolRegistry` 実装（検索・検証・有効化）
3. `ToolCapabilityIndex`（タグ検索: fs/network/browser/media/git）
4. `ToolExecutionEnvelope`（trace_id, timeout, retries, policy_decision）

---

## 3. 契約仕様

```go
type ToolManifest struct {
    ID          string
    Version     string
    Description string
    InputSchema json.RawMessage
    OutputSchema json.RawMessage
    SideEffect  string // none|local_write|network|process
    RequiresApproval bool
    TimeoutSec  int
}
```

```go
type Tool interface {
    Manifest() ToolManifest
    Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}
```

---

## 4. 配置

- `internal/domain/tools/manifest.go`
- `internal/domain/tools/registry.go`
- `internal/application/tools/executor.go`
- `internal/infrastructure/tools/catalog_loader.go`

---

## 5. TDD計画

1. Manifestスキーマ検証テスト
2. バージョン競合時の登録拒否テスト
3. `RequiresApproval=true` の実行拒否テスト
4. 実行Envelopeのタイムアウト/リトライテスト
5. 監査ログにManifest IDが必ず残るテスト

受け入れ基準:
- 全ツールがRegistry経由でのみ実行される
- 非登録ツールの実行は100%拒否

---

## 6. 未決事項

1. ToolManifestの保存先をYAML優先にするかJSON優先にするか
2. 既存MCPツールとの二重登録をどう解消するか
