# routing-policy.md

## 0. 目的
Coder呼び分け（DeepSeek→ChatGPT昇格）を、LLMの気分ではなくルールで決めて安定運用する。  
Spawnは禁止し、エージェント数は増やさない。

## 1. ルーティング対象
- 対象：Workerが発行する「実装要求（implement）」イベント
- Coder実行先：
  - primary：DeepSeek（Coder1）
  - fallback：ChatGPT（Coder2）

## 2. 基本ルール
- 既定は DeepSeek
- 昇格条件に該当したら ChatGPT

## 3. 昇格条件（最低限）
以下のいずれかで ChatGPT に昇格する。

### 3.1 失敗回数ベース
- DeepSeekで連続失敗 >= 2  
  失敗の定義例：
  - 受け入れ条件（acceptance）を満たさない
  - ビルド/テストが通らない
  - 指示にない変更が混ざる（スコープ逸脱）
  - 手順が再現できない

### 3.2 タスク特性ベース（最初から昇格してよい）
- 設計判断/リファクタが主
- 影響範囲が大（複数モジュール、広範な差分）
- 原因切り分けが主（再現条件が揺れる、依存が複雑）

## 4. 送信ペイロード最小化（外部Coder共通）
外部に送るのは必要最小限に絞る。

必須
- goal（1文）
- constraints（禁止事項・前提）
- expected_output
- acceptance（受け入れ条件）
- 再現手順（ある場合）
- 関連ファイル（必要部分のみ）
- エラーログ（必要行のみ）

禁止
- APIキー、トークン、個人情報
- リポジトリ丸ごと
- 不要な大量ログ

マスク方針
- `*_KEY` / `*_TOKEN` / `Authorization:` / cookie類は送信前に自動置換
- 置換後の値をログに残さない

## 5. 検品（Workerの固定責務）
- Coder出力は必ずWorkerが acceptance で検品する
- 不合格なら「不合格理由＋修正依頼」を返す（EventIdを引き継ぐ）

## 6. 疑似コード（例）
```pseudo
function route_coder(request):
  if request.task_traits.contains("refactor") or request.scope == "large" or request.needs_debug == true:
    return "chatgpt"  // immediate escalate

  if request.fail_count_deepseek >= 2:
    return "chatgpt"

  return "deepseek"
```

## 7. 観測ログ（最低限）
各 implement イベントで以下を残す（EventIdと紐づけ）。
- selected_provider = deepseek|chatgpt
- fail_count_deepseek
- reason = default|failed_twice|large_scope|refactor|debug
- acceptance_result = pass|fail
