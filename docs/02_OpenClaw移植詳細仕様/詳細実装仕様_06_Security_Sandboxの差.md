# 詳細実装仕様 06: Security/Sandboxの差

**作成日**: 2026-03-09  
**ステータス**: In Progress  
**親仕様**: `docs/実装仕様_OpenClaw移植_v1.md`  
**関連**: `詳細実装仕様_01_実行基盤とセキュリティ境界.md`

---

## 0. OpenClaw原典の理解

- Gateway Security: <https://docs.openclaw.ai/gateway/security>
- Tools: <https://docs.openclaw.ai/tools>

OpenClawはセキュリティを実行時制御で担保し、ツールの権限境界を明示する。

---

## 1. 現状差分

- RenCrow現状: ガードは存在するが、権限モデル・承認モード・隔離レベルが統一されていない。
- 差分の本質: 「誰が何をどこまでできるか」のポリシー表現不足。

---

## 2. 実装対象

1. `SecurityProfile`（strict/balanced/dev）
2. 実行権限スコープ（filesystem/network/process/git）
3. 承認モード（never/on-demand/always）
4. サンドボックスレベル（workspace/process/container）
5. 監査イベント分類（security.decision / security.violation）

### 2.1 実装進捗（2026-03-10）

- 実装済み:
  - `SecurityProfile` に `sandbox_level` を追加（workspace/process/container）
  - Profileプリセット: `strict` / `balanced` / `dev`
  - `policy_mode=dev` を設定で許可
  - strict想定のネットワーク制御:
    - `network_scope=allowlist` で host allowlist 判定（`url`/`host` 引数から抽出）
    - `network_scope=blocked` でネットワーク系ツール拒否
  - 設定配線:
    - `security.network_scope` / `security.network_allowlist` を config で指定可能
    - 実行時に PolicyEngine へ配線
  - 監査イベント種別:
    - allow/ask -> `security.decision`
    - deny -> `security.violation`
- 次フェーズ:
  - container sandbox の実行基盤統合（現状はモデル定義のみ）

---

## 3. 契約仕様

```go
type SecurityProfile struct {
    Name            string
    ApprovalMode    string // never|on_demand|always
    FilesystemScope string // workspace|readonly|none
    NetworkScope    string // blocked|allowlist|full
    ProcessScope    string // none|limited|full
    GitScope        string // read|safe_write|full
}
```

強制拒否ルール:
- `rm -rf /`
- `git reset --hard`
- workspace外への書き込み
- allowlist外ホストへの送信（strict時）

---

## 4. 配置

- `internal/domain/security/profile.go`
- `internal/application/security/enforcement_service.go`
- `internal/infrastructure/security/policy_store.go`
- `internal/infrastructure/security/audit_logger.go`

---

## 5. TDD計画

1. Profile別許可行列テスト
2. 禁止コマンド拒否テスト
3. allowlist外通信拒否テスト
4. 監査イベント完全性テスト
5. 承認モード切替の回帰テスト

受け入れ基準:
- strictプロファイルで危険操作の実行成功が0件
- すべての拒否が監査ログに残る

---

## 6. 未決事項

1. container sandbox導入を初期フェーズで必須にするか
2. 開発環境(dev)の例外許可範囲をどこまで認めるか
