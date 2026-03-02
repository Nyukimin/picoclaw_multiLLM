# 分散実行実装仕様 v4.0 作成完了レポート

**作業日**: 2026-03-02
**ステータス**: ✅ 完了

---

## 作業内容

### 1. 新規ドキュメント作成

**`docs/実装仕様_分散実行_v4.md`**（2,334行）

分散実行の完全な実装仕様を作成：

**章構成**:
- 0. エグゼクティブサマリ（v4.0の位置付け、期待効果、8週間実装期間）
- 1. 背景と目的（単一プロセスの課題、分散実行のメリット）
- 2. アーキテクチャ概要（v3.0 vs v4.0、Transport層の責務）
- 3. Phase 1: ローカル通信層（2週間）
  - Transport インターフェース、LocalTransport、MessageRouter
  - テスト計画、成功基準
- 4. Phase 2: リモート通信基盤（3週間）
  - SSHTransport、Agentスタンドアロンモード
  - TransportFactory、設定ファイル拡張
- 5. Phase 3: 分散環境対応（2週間）
  - Session/Memory分散管理、通信ログ、接続プール
- 6. 通信フォーマット仕様（JSON Schema、Proposal/Result）
- 7. Session/Memory分散管理（階層的ログ伝播、永続化）
- 8. セキュリティとエラーハンドリング（SSH公開鍵認証、再接続）
- 9. 実装プラン（8週間 + 本番移行4週間）
- 10. テスト戦略（単体/統合/E2E/パフォーマンス/障害注入）
- 11. 移行パス（4ステップ、12週間）
- 付録A: JSON Schema定義
- 付録B: SSH接続設定例
- 付録C: 用語集

**主要設計決定**:
1. Transport層抽象化（Local/SSH透過的切り替え）
2. SSH + JSON通信（stdin/stdout、1行1メッセージ）
3. Agentスタンドアロンモード（`cmd/picoclaw-agent/`）
4. 階層的Session同期（Coder → Worker → Mio）
5. v3.0互換性（`distributed.enabled`フラグ）
6. 段階的移行（4ステップ、12週間）

### 2. 既存ドキュメント更新

**`docs/README.md`**:
- v4.0ドキュメントへのリンク追加（正本仕様の3番目）
- `Chat_Worker_Coder_アーキテクチャ.md` を「アーキテクチャ設計」に追加

**`docs/実装仕様_v3.md`**:
- 付録D「v4.0分散実行への拡張」を追加
- v3.0とv4.0の関係性、互換性を明記

**`CLAUDE.md`**:
- 「6.1 正本仕様」に v4.0 とアーキテクチャドキュメントの参照追加

---

## 期待効果（v4.0実装後）

| 効果 | 詳細 | KPI |
|------|------|-----|
| **負荷分散** | LLM呼び出しを複数CPUに分散 | CPU使用率 70% → 40% |
| **並列実行** | 複数Coderが同時動作 | 応答時間 50%短縮 |
| **スケーラビリティ** | マシン追加でAgent数を増やせる | 1マシン → 4マシン対応 |
| **独立性** | Agent単位でアップデート・再起動 | ダウンタイム 90%削減 |
| **保守性** | Agent単位の障害分離 | 障害影響範囲 -75% |
| **透明性** | すべての通信がログに記録 | デバッグ時間 -60% |

---

## 実装スケジュール（8週間 + 本番移行4週間）

### Phase 1: ローカル通信層（Week 1-2）

**成果物**:
- `internal/domain/transport/transport.go` - Transport インターフェース
- `internal/infrastructure/transport/local.go` - LocalTransport（チャネルベース）
- `internal/infrastructure/transport/router.go` - MessageRouter
- 単体テスト（カバレッジ90%以上）

**成功基準**:
- ✅ Transport抽象化完成
- ✅ ローカル通信でJSON送受信動作
- ✅ v3.0との共存確認（`distributed.enabled: false`）

### Phase 2: リモート通信基盤（Week 3-5）

**成果物**:
- `internal/infrastructure/transport/ssh.go` - SSHTransport
- `cmd/picoclaw-agent/main.go` - Agentスタンドアロンモード
- `internal/infrastructure/transport/factory.go` - TransportFactory
- `config.yaml` の `distributed` セクション
- E2Eテスト

**成功基準**:
- ✅ SSH経由でメッセージ送受信
- ✅ Agentスタンドアロンモード動作
- ✅ 設定ファイルでLocal/SSH切り替え可能

### Phase 3: 分散環境対応（Week 6-8）

**成果物**:
- `internal/domain/session/distributed.go` - Session同期
- `internal/infrastructure/transport/logger.go` - 通信ログ
- `internal/infrastructure/transport/pool.go` - 接続プール
- パフォーマンステスト
- 運用ドキュメント

**成功基準**:
- ✅ 複数Agentの会話履歴がMioに集約
- ✅ すべての通信がログ記録
- ✅ パフォーマンス劣化20%以内（v3.0比）

### 本番移行（Week 9-12）

**4ステップ移行**:
1. Week 9: 開発環境でv4.0有効化
2. Week 10-11: ステージング環境で段階的有効化
3. Week 12: 本番環境で段階的移行（10% → 50% → 100%）

---

## 重要な技術仕様

### Transport インターフェース

```go
type Transport interface {
    Send(ctx context.Context, msg Message) error
    Receive(ctx context.Context) (Message, error)
    Close() error
    IsHealthy() bool
}
```

### メッセージ構造

```json
{
  "from": "Worker",
  "to": "Coder3",
  "session_id": "session_abc123",
  "job_id": "job_20260302_001",
  "message": "hello.goを作成してください",
  "context": { /* ... */ },
  "proposal": null,
  "result": null,
  "timestamp": "2026-03-02T15:30:00Z"
}
```

### 設定ファイル拡張

```yaml
distributed:
  enabled: true  # false で v3.0 モード
  transports:
    mio:
      type: local
    shiro:
      type: ssh
      remote_host: "192.168.1.100:22"
      remote_user: picoclaw
      ssh_key_path: ~/.ssh/id_picoclaw
    gin:
      type: ssh
      remote_host: "192.168.1.101:22"
      remote_user: picoclaw
      ssh_key_path: ~/.ssh/id_picoclaw
```

---

## v3.0との互換性

**不変（v3.0と同じ）**:
- ✅ Domain/Application層のインターフェース
- ✅ Agent エンティティ（Mio/Shiro/Coder）
- ✅ Proposal/Patch値オブジェクト
- ✅ MessageOrchestrator

**追加（v4.0）**:
- ➕ `internal/domain/transport/` - Transport抽象化
- ➕ `internal/infrastructure/transport/` - Local/SSH実装
- ➕ `cmd/picoclaw-agent/` - スタンドアロンモード
- ➕ `config.yaml` の `distributed` セクション

**変更（最小限）**:
- 🔧 `MessageOrchestrator` - Transport層との統合

**互換性確認**:
```bash
# v3.0モードで全テスト実行
export PICOCLAW_DISTRIBUTED_ENABLED=false
go test ./...

# v4.0モードで全テスト実行
export PICOCLAW_DISTRIBUTED_ENABLED=true
go test ./...
```

---

## ロールバック手順（5分以内）

1. 設定ファイル変更: `distributed.enabled: false`
2. サービス再起動: `sudo systemctl restart picoclaw`
3. 動作確認: 全機能テスト
4. ログ確認: v3.0モードで動作しているか確認

**データロス**: なし（v3.0のMemory形式を維持）

---

## 次のアクション

### 即座に実施可能

1. **Phase 1実装開始**（Week 1-2）
   - Transport インターフェース定義
   - LocalTransport 実装
   - MessageRouter 実装
   - 単体テスト作成

2. **設定ファイル準備**
   ```yaml
   distributed:
     enabled: false  # 初期状態はv3.0モード
   ```

### 事前準備（Phase 2開始前）

1. SSH鍵ペア生成（Ed25519推奨）
2. リモートホストへの公開鍵配置
3. `known_hosts` にホスト鍵登録
4. リモートホストに `picoclaw-agent` バイナリ配置

---

## 参照ドキュメント

**実装仕様**:
- `docs/実装仕様_分散実行_v4.md` - v4.0の完全な実装仕様（本ドキュメント）
- `docs/実装仕様_v3.md` - v3.0 Clean Architecture基盤（付録Dにv4.0参照あり）
- `docs/仕様.md` - 上位要件

**設計思想**:
- `docs/Chat_Worker_Coder_アーキテクチャ.md` （484-661行目）- 分散実行の設計思想

**プロジェクトルール**:
- `CLAUDE.md` （6.1節）- v4.0ドキュメントへの参照

---

**作業完了日**: 2026-03-02
**ステータス**: ドキュメント作成完了、実装準備完了
**次のマイルストーン**: Phase 1実装開始（Week 1）