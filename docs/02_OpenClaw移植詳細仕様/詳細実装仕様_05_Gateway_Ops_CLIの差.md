# 詳細実装仕様 05: Gateway/Ops CLIの差

**更新日**: 2026-03-19  
**ステータス**: 現行実装ベース  
**親仕様**: `docs/実装仕様_OpenClaw移植_v1.md`

---

## 1. 概要

RenCrow の Ops CLI は、すでに `run` 以外の運用導線を複数持っている。  
したがって本仕様の焦点は「CLI を追加すること」ではなく、どのコマンドが実装済みで、どこまで JSON 契約が揃っているかを明確にすることである。

---

## 2. 実装済みコマンド

**ファイル**: `cmd/picoclaw/main.go`

現行コマンド:

- `picoclaw status`
- `picoclaw health`
- `picoclaw doctor`
- `picoclaw gateway status`
- `picoclaw gateway restart`
- `picoclaw channels list`
- `picoclaw channels probe`
- `picoclaw ollama status`
- `picoclaw ollama restart`
- `picoclaw logs`
- `picoclaw evidence list|show|summary`

多くのコマンドが `--json` を持ち、機械向け出力に対応する。

---

## 3. JSON 出力契約

現行 CLI JSON は完全統一 DTO ではないが、次の共通フィールドを広く持つ。

- `ok`
- `timestamp`
- `component`
- `status`
- `details`

エラー時は追加で次を返すことが多い。

- `code`
- `hint`

### 3.1 gateway

`gateway status --json` の代表形:

```json
{
  "ok": true,
  "timestamp": "...",
  "component": "gateway",
  "status": "running",
  "details": {
    "url": "http://127.0.0.1:18790/health",
    "status_code": 200
  }
}
```

エラーコード例:

- `E_GATEWAY_UNREACHABLE`
- `E_GATEWAY_UNHEALTHY`
- `E_GATEWAY_RESTART_FAILED`

### 3.2 ollama

`ollama status --json` は health report を含む。  
`ollama restart --json` は restart target を `details.target` に含む。

エラーコード例:

- `E_OLLAMA_RESTART_FAILED`

### 3.3 logs

`logs --json` は最初にメタ JSON を 1 件出し、その後に snapshot もしくは follow ストリームを続ける。

状態:

- `snapshot`
- `streaming`

### 3.4 evidence

`evidence list|show|summary` は `ExecutionReport` 系を返す。  
Viewer の evidence 表示と同系統のデータソースである。

---

## 4. コマンド別の到達点

### 4.1 status

- システム概要表示
- `--json` あり
- deep/usage 相当の詳細を 1 コマンドに統合済み

### 4.2 health

- health checks を実行
- HTTP `/health` と同系統の診断面
- `--json` あり

### 4.3 doctor

- 設定矛盾
- audit path の書き込み可能性
- health down

などを findings として返す。

### 4.4 gateway

- `status`: local gateway health endpoint 参照
- `restart`: systemctl 連携

### 4.5 channels

- `list`: 登録済み adapter 一覧
- `probe`: 各 adapter の疎通確認

### 4.6 ollama

- `status`: model/base_url/health checks
- `restart`: local または SSH 経由再起動

### 4.7 logs

- ログ末尾 100 行 snapshot
- `--follow` で継続表示
- `--json` で先頭メタ情報を付与

### 4.8 evidence

- `list`: recent execution evidence
- `show <job_id>`: 単票表示
- `summary`: status/error_kind 集計

---

## 5. OpenClaw 観点との差分

現行到達点:

- gateway status/restart あり
- channel list/probe あり
- health/status/doctor あり
- logs follow あり
- evidence CLI あり

未到達:

1. CLI 実装は `cmd/picoclaw/main.go` 集約で、専用 package 分離は薄い
2. JSON schema の完全固定版はまだ文書化途上
3. systemd 依存 restart があり、全環境で同じ supervisor とは限らない

---

## 6. 確認観点

- `gateway status --json` が status/code/hint を返す
- `channels probe` が adapter probe を返す
- `logs --follow` が snapshot 後に継続出力する
- `evidence` CLI が `ExecutionReport` を読める

以上をもって、RenCrow の Ops CLI は「不足」ではなく、OpenClaw 的な運用導線をかなり取り込んだ現行実装として扱う。
