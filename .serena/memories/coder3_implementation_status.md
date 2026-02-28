# Coder3（Gin）実装状況

**最終更新**: 2026-02-25

## 概要

Coder3（愛称: Gin）は Anthropic Claude API を使用する高品質コーディング専用ルート。承認フロー必須。

## 完了した実装（Phase 1-4）

### Phase 1: ルーティング拡張
- **pkg/agent/router.go**: RouteCode3, RouteApprove, RouteDeny 定数を追加
- **pkg/agent/router.go**: /code3, /approve, /deny コマンドを追加
- **pkg/agent/router.go**: IsCodeRoute() に CODE3 を追加
- **pkg/config/config.go**: RouteLLMConfig に Coder3Alias/Provider/Model フィールドを追加
- **pkg/agent/loop.go**: selectCoderRoute() に CODE3 キーワード追加

### Phase 2: 承認インフラ
- **pkg/approval/job.go**: GenerateJobID() 関数（YYYYMMDD-HHMMSS-xxxxxxxx 形式）
- **pkg/approval/manager.go**: Manager 型、CreateJob/GetJob/Approve/Deny/IsApproved メソッド
- **pkg/approval/message.go**: FormatApprovalRequest() 関数
- **pkg/session/manager.go**: SessionFlags に PendingApprovalJobID フィールドを追加
- **pkg/logger/logger.go**: 承認ログ関数5つ（LogApprovalRequested, LogApprovalGranted, LogApprovalDenied, LogApprovalAutoApproved, LogCoderPlanGenerated）

### Phase 3: 承認フローロジック
- **pkg/agent/loop.go**: approvalMgr フィールドを追加
- **pkg/agent/loop.go**: Coder3Output 構造体、parseCoder3Output() 関数を追加
- **pkg/agent/loop.go**: handleCommand() に /approve, /deny ハンドラを追加

### Phase 4: 統合
- **pkg/agent/loop.go**: processMessage() に CODE3 出力処理を追加（パース → ジョブ作成 → セッション保存 → ログ → 承認要求メッセージ生成）

### 設定ファイル
- **config/config.example.json**: coder3 設定例を追加
- **~/.picoclaw/config.json**: providers.anthropic セクションを追加
- **~/.picoclaw/config.json**: routing.llm に coder3_alias/provider/model を追加

## 設定

### 愛称とモデル
```json
{
  "routing": {
    "llm": {
      "coder3_alias": "Gin",
      "coder3_provider": "anthropic",
      "coder3_model": "claude-sonnet-4-5-20250929"
    }
  }
}
```

### API キー設定
```json
{
  "providers": {
    "anthropic": {
      "api_key": "sk-ant-api03-xxxxx",
      "api_base": ""
    }
  }
}
```

**注意**: api_base は空欄でOK（デフォルトで https://api.anthropic.com/v1 が使われる）

## テスト状況

- ユニットテスト: pkg/approval/ で15テスト（全てパス）
- 統合テスト: 未実施（API キー設定後に実施予定）

## 次のステップ: MCP Chrome 統合

### 目的
Coder3 にブラウザ操作機能を追加する。**承認フロー（job_id）を通してのみ実行可能**。

### 構成
```
PicoClaw (Linux) → HTTP → Win11 (100.83.235.65:12306) → mcp-chrome → Chrome 拡張機能
```

### Win11 側のセットアップ（未実施）
1. mcp-chrome-bridge をインストール: `npm install -g mcp-chrome-bridge`
2. Chrome 拡張機能をインストール（https://github.com/hangwin/mcp-chrome）
3. mcp-chrome-bridge を起動（ポート 12306）

### PicoClaw 側の実装（未実施）
1. MCP クライアントを実装（pkg/mcp/ パッケージ新規作成）
2. HTTP 経由で http://100.83.235.65:12306/mcp に接続
3. Coder3 に Chrome 操作ツールを追加（pkg/tools/ に新規ツール）
4. 承認フローに「ブラウザ操作」リスクを追加

### 参考資料
- mcp-chrome GitHub: https://github.com/hangwin/mcp-chrome
- Chrome MCP Server Documentation: https://lobehub.com/mcp/hangwin-mcp-chrome
- Claude Code Chrome integration: https://code.claude.com/docs/en/chrome

## 重要な設計判断

### Coder3 のみ承認フロー実装
- 選択肢A（採用）: Coder3 のみ plan/patch 分離と承認フローを実装
- 選択肢B（却下）: Coder1/Coder2 も含めた全面リファクタリング
- 理由: 既存コード（Coder1/Coder2）は実際には plan/patch 分離されていない。Coder3 のみ新しい設計を適用。

### API 方式と Chrome 操作の選択
- **Coder3 の推論**: Anthropic API（有料、従量課金）を使用
- **Chrome 操作**: mcp-chrome 経由で実現
  - Coder3 は Chrome 操作の **plan を生成**
  - Worker が **承認済み job_id** を確認して実行
  - 承認方式: `/approve <job_id>` または 永続承認（期限付き）
- **セキュリティ**: Chrome 操作は Auto-Approve 対象外、すべて job_id で追跡

## トラブルシューティング

### API キーが認識されない場合
1. config.json の providers.anthropic.api_key を確認
2. api_base は空欄のままでOK
3. systemctl --user restart picoclaw-gateway で再起動

### ルーティングが機能しない場合
1. /code3 コマンドで明示的にルーティング
2. ログを確認: ~/.picoclaw/logs/gateway.log
3. classifier が無効の場合は強制コマンド（/code3）が必要
