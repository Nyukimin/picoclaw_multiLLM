# Ollama「個性（人格）」の上書き運用：提案メモ（くろ/作業船）

## ゴール
- 秘書キャラ「くろ」を **Ollamaの別名モデル**として固定しつつ、試行錯誤で何度でも上書きできるようにする。
- 事故（人格が急に変わってデバッグ不能）を避けるために、**世代管理 → 安定したら alias を上書き**の二段階にする。
- 2モデル常駐を想定：  
  - くろ（対話/最終文）：`dsasai/llama3-elyza-jp-8b:latest`  
  - 作業船（段取り/JSON）：`qwen2.5:7b`

---

## 結論（運用方針）
### 方針A：開発中（推奨）
- `kuro-v1`, `kuro-v2`, … のように **世代を増やす**
- OpenClaw側は当面 `kuro-current` のような“参照名”に統一し、**切り替えは参照名の差し替え**で行う（※実装上は設定値の差し替え）

### 方針B：固まったら（推奨）
- 安定版を `kuro` に **上書き（再 create）**して固定運用
- 旧世代は必要に応じて `ollama rm` で掃除

---

## 1) くろ（秘書）の作り方（世代管理）
### Modelfile（例：Modelfile.kuro）
```text
FROM dsasai/llama3-elyza-jp-8b:latest
SYSTEM あなたは秘書「くろ」。日本語は自然で簡潔。結論→手順→確認の順。推測は推測と明示。不要な提案はしない。出力は短め。
PARAMETER temperature 0.4
```

### PowerShell（作成）
```powershell
# 例：v2 を作る
ollama create kuro-v2 -f .\Modelfile.kuro

# 起動テスト
ollama run kuro-v2 "今日のToDoを3つに整理して。短く。"
```

---

## 2) 参照名（alias）で“切替を一箇所に集約”
Ollamaには「別名の別名」みたいなシンボリックリンク機能はないので、運用上は次のどちらかにします。

### 方式1（シンプル）：OpenClaw側の設定でモデル名を差し替える
- OpenClawの設定で `MODEL_KURO=kuro-v2` のように指定し、更新時にそこだけ変える

### 方式2（割り切り）：`kuro` を参照名として上書き
- 試験が終わった世代を `kuro` として再作成して固定
```powershell
# 安定した kuro-v2 を採用したい場合：
# Modelfile.kuro を v2 内容にして、kuro を上書き
ollama create kuro -f .\Modelfile.kuro
```

---

## 3) 作業船（worker）を JSON 専用に固定（推奨）
コード生成は外部LLMに任せる前提なので、ローカルは **段取り/構造化**に寄せる。

### Modelfile（例：Modelfile.worker）
```text
FROM qwen2.5:7b
SYSTEM あなたは作業船。出力は常にJSONのみ。自然文は禁止。目的→タスク分解→受け入れ条件→リスク→次の実行、をJSONで返す。
PARAMETER temperature 0.2
```

### PowerShell（作成）
```powershell
ollama create worker-v1 -f .\Modelfile.worker
ollama run worker-v1 "OpenClawのバックグラウンド作業の最小構成を計画して"
```

---

## 4) 上書きは何回でもできる？（答え）
- **できる。**同じ名前で `ollama create <name> -f ...` を実行すれば運用上は上書きになる。
- ただし内部的に世代が残ることがあるので、必要に応じて掃除する。

---

## 5) 掃除（不要モデルの削除）
```powershell
ollama list
ollama rm kuro-v1
ollama rm worker-v0
```

---

## 6) おすすめの最終形（れんさん向け）
- くろ：`kuro`（ELYZA 8B由来、自然日本語の対話/最終文）
- 作業船：`worker`（Qwen2.5 7B由来、JSON専用の段取り役）
- 試行錯誤中は `kuro-vN / worker-vN` を増やし、採用時に `kuro / worker` を上書きして固定

---
