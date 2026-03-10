# 詳細実装仕様 05: Gateway/Ops CLIの差

**作成日**: 2026-03-09  
**ステータス**: In Progress  
**親仕様**: `docs/実装仕様_OpenClaw移植_v1.md`

---

## 0. OpenClaw原典の理解

- Gateway: <https://docs.openclaw.ai/gateway>
- CLI: <https://docs.openclaw.ai/cli>

OpenClawは運用者向けCLIを標準装備し、Gateway運用（状態確認・診断・復旧）をCLI導線で完結させる。

---

## 1. 現状差分

- PicoClaw現状: `run`中心で運用CLIが不足。
- 差分の本質: 運用時の観測と復旧コマンドが体系化されていない。

---

## 2. 実装対象コマンド

1. `picoclaw gateway status`
2. `picoclaw gateway restart`（systemd連携）
3. `picoclaw channels list`
4. `picoclaw channels probe`
5. `picoclaw status --deep --usage`
6. `picoclaw health --json`
7. `picoclaw doctor`
8. `picoclaw logs --follow`

### 2.1 実装進捗（2026-03-10）

- 実装済み:
  - `picoclaw gateway status --json`（標準JSON + エラーコード）
  - `picoclaw gateway restart --json`
  - `picoclaw channels list --json`
  - `picoclaw channels probe --json`
  - `picoclaw status --deep --usage --json`（詳細/利用統計の統合出力）
  - `picoclaw health --json`（Ops JSON契約形式）
  - `picoclaw doctor --json`（findingsの構造化出力）
  - `picoclaw logs --json --follow`（初期メタJSON + 後続ログストリーム）
- 既存実装維持:
  - 既存のテキスト出力モード（`--json` なし）

---

## 3. 契約仕様

### 3.1 JSON出力標準

```json
{
  "ok": true,
  "timestamp": "2026-03-09T12:00:00Z",
  "component": "gateway",
  "status": "running",
  "details": {}
}
```

### 3.2 エラー標準

```json
{
  "ok": false,
  "code": "E_GATEWAY_UNREACHABLE",
  "hint": "picoclaw gateway restart を実行"
}
```

---

## 4. 配置

- `cmd/picoclaw/cli/*`
- `internal/application/ops/*`
- `internal/adapter/cli/*`

---

## 5. TDD計画

1. コマンド引数パース単体テスト
2. `--json` 出力契約テスト
3. systemd非環境でのフォールバックテスト
4. `channels probe` 疎通失敗テスト
5. `doctor` 設定矛盾検出テスト

受け入れ基準:
- 全Ops CLIで終了コードが規約通り
- 障害時に`doctor`で原因候補が提示される

---

## 6. 未決事項

1. restartをCLI直接実行にするか外部Supervisor委譲にするか
2. `logs --follow` の実装をローカルファイル限定にするか
