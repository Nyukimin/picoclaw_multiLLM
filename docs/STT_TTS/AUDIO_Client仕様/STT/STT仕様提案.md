了解です。
このスレッドの内容を整理して、**HTTP 接続を含む STT 仕様書**としてまとめます。
コピーしやすいように、全文をコードブロックで置きます。

````markdown
# STT仕様書
## Chat向け音声入力系（Whisperベース）
### 2026-04-05 時点整理版

---

## 1. 目的

本仕様は、Chat に対する音声入力系（STT: Speech To Text）の構成・通信・状態遷移・判定方針を定義する。

本件では、以下を満たすことを目的とする。

- ユーザーは任意の端末・場所から Chat に音声入力できる
- 音声入力の入口は Web ブラウザとする
- 音声認識は Whisper サーバで行う
- 音声認識結果は、まず Chat を介さないテスト経路で文字列表示できること
- 安定後に Chat 本体へ接続すること
- 認識結果は、日本語向けの暫定字幕／確定字幕の二層で扱うこと

---

## 2. 用語

- Chat  
  ユーザーとの会話インタフェース本体。  
  Whisper 入力、LLM 問い合わせ、SBV2 出力を束ねるオーケストレータ。

- Worker / Coder  
  本仕様の STT テスト経路には直接関与しない。

- Whisper サーバ  
  Chat 配下の音声入力 I/O。音声を文字列化する。

- voice-bridge  
  ブラウザと Whisper の間に置く軽量な中継サーバ。  
  Web UI 提供、WebSocket 受信、Whisper HTTP 呼び出し、暫定字幕／確定字幕制御を担う。

- テスト経路  
  音声 → Whisper → 文字列表示のみ  
  Chat / LLM / SBV2 を通さない切り分け用経路。

- 本番経路  
  音声 → Whisper → Chat → LLM → Chat → SBV2

---

## 3. 全体方針

### 3.1 基本構成

Chat は以下の責務を持つ。

- ユーザーとの会話 UI
- Whisper からの入力文字列の提示
- LLM への問い合わせ
- SBV2 による音声出力

Whisper および SBV2 は Chat 配下の I/O とする。

### 3.2 段階導入方針

音声入力が安定して通るまでは、Chat を経由しないテスト経路を維持する。

段階は以下。

1. 音声 → Whisper → 文字列表示
2. 音声 → Whisper → Chat 表示
3. 音声 → Whisper → Chat → LLM
4. 音声 → Whisper → Chat → LLM → SBV2

### 3.3 HTTPS 方針

ブラウザのマイク入力は secure context が必要なため、HTTP 直打ちは使わない。  
本件では、Tailscale Serve を用いて Chat の PC 上の `voice-bridge` を HTTPS 公開する。

---

## 4. 現在の確認済み構成

### 4.1 役割配置

- Chat / Whisper / voice-bridge 配置先: `Win11-HP`
- マイク入力端末: `Lenovo`
- ブラウザ表示端末: `Lenovo`
- ネットワーク入口: Tailscale Serve

### 4.2 現在の入口 URL

- ブラウザ用 URL  
  `https://win11-hp01.tailb07d8d.ts.net/`

### 4.3 Tailscale Serve 設定

- 外部公開（tailnet 内のみ）  
  `https://win11-hp01.tailb07d8d.ts.net/`
- 内部転送先  
  `http://127.0.0.1:8090`

### 4.4 Whisper サーバ

- 実行ファイル  
  `D:\RenCrow\devices\audioio\whisper.cpp\build\bin\whisper-server.exe`
- ポート  
  `8080`
- 起動スクリプト  
  `D:\RenCrow\ops\audioio\start-whisper.ps1`
- 現在の主要引数  
  - `--host 0.0.0.0`
  - `--port 8080`
  - `-m D:\RenCrow\devices\audioio\whisper.cpp\models\ggml-base.bin`
  - `-l ja`
  - `--convert`
  - `--split-on-word`

### 4.5 voice-bridge

- 配置  
  `D:\RenCrow\webui\voice-bridge`
- ポート  
  `8090`
- 提供内容  
  - 静的 Web UI
  - WebSocket `/ws`
  - Whisper HTTP 中継
  - 暫定字幕／確定字幕制御

---

## 5. 論理構成

### 5.1 テスト経路

Lenovo Browser
→ Tailscale HTTPS
→ Win11-HP: voice-bridge
→ Win11-HP: Whisper
→ Lenovo Browser に文字列表示

### 5.2 本番経路（将来）

Lenovo Browser
→ Tailscale HTTPS
→ Win11-HP: Chat Front
→ Win11-HP: Whisper
→ Chat
→ LLM
→ Chat
→ SBV2
→ 音声出力

---

## 6. 通信仕様

## 6.1 Browser → voice-bridge

### 方式
- HTTPS で静的ページ取得
- WebSocket で音声データと状態通知を送信

### WebSocket 接続先
- `wss://{location.host}/ws`

### 送信メッセージ種別

#### 1. config
初回接続時に送る設定。

```json
{
  "type": "config",
  "mimeType": "audio/webm;codecs=opus"
}
````

#### 2. vad

ブラウザ側簡易 VAD による発話状態。

```json
{
  "type": "vad",
  "speaking": true
}
```

#### 3. finalize

無音後の再解釈が完了し、現在の draft を確定してよいことを通知する。

```json
{
  "type": "finalize"
}
```

#### 4. binary audio chunk

音声チャンク本体。
WebSocket の binary frame として送信する。

---

## 6.2 voice-bridge → Whisper

### 方式

HTTP POST

### 接続先

* `http://127.0.0.1:8080/inference`

### Content-Type

* `multipart/form-data`

### 送信項目

* `file`
* `response_format=json`

### 例

* `file=@window.webm`
* `response_format=json`

### 戻り

```json
{
  "text": "認識結果"
}
```

---

## 6.3 voice-bridge → Browser

### WebSocket メッセージ種別

#### draft

暫定字幕更新

```json
{
  "type": "draft",
  "text": "暫定字幕"
}
```

#### final

確定字幕

```json
{
  "type": "final",
  "text": "確定字幕"
}
```

#### reply_reset

返答表示クリア

```json
{
  "type": "reply_reset"
}
```

#### reply_delta

返答の逐次表示

```json
{
  "type": "reply_delta",
  "text": "返答の一部"
}
```

#### error

エラー表示

```json
{
  "type": "error",
  "text": "エラーメッセージ"
}
```

---

## 7. 字幕制御仕様

## 7.1 基本方針

字幕は 1 層ではなく、以下の 2 層で扱う。

* 暫定字幕
* 確定字幕

### 暫定字幕

* 短いチャンク単位で更新する
* 意味が変わったら書き直してよい
* LLM には渡さない

### 確定字幕

* 無音を契機に、長め窓で再解釈した結果で確定する
* Chat / LLM に渡すのは確定字幕のみ

## 7.2 正しい挙動

* 話している間
  → チャンクで暫定字幕を更新する
* 無音になった時
  → 少し長めの窓で再解釈する
* 再解釈後
  → 字幕を書き直して確定する

つまり本件は、**チャンク単送のみ**ではなく、
**無音＋チャンクで修正する方式**を採る。

---

## 8. Browser 側 STT 入力仕様

## 8.1 マイク取得

* `navigator.mediaDevices.getUserMedia({ audio: true })`

## 8.2 録音方式

* `MediaRecorder`
* MIME は以下優先

  * `audio/webm;codecs=opus`
  * `audio/webm`

## 8.3 短周期チャンク

* 短周期で chunk を生成し、暫定字幕用に送る

## 8.4 無音判定

* ブラウザ側で簡易 RMS ベース VAD を持つ
* `speaking=true/false` を WebSocket 送信する

## 8.5 無音後の再解釈

* 無音で閉じたタイミングで、長め窓を対象に再解釈
* その完了後に `finalize` を送る

---

## 9. voice-bridge 側状態

voice-bridge は最低限、以下の状態を持つ。

* `mimeType`
* `draftText`
* `speaking`
* `busy`
* `finalizing`

補足:
現時点の簡易版では保持していないが、正式仕様では次も持てるようにしてよい。

* `rollingAudioWindow`
* `lastSpeechAt`
* `utteranceStartAt`
* `stablePrefix`
* `draftTail`

---

## 10. Whisper との関係

## 10.1 Whisper の役割

Whisper は「聞こえたものを文字列化する」役に限定する。

Whisper の責務:

* 音声認識
* 日本語出力
* 語境界寄りの分割補助

Whisper に期待しすぎないこと:

* 意味再構成
* 危険操作の判断
* 文脈解釈

## 10.2 Whisper の誤りと Chat の扱い

Whisper の誤文字列でも、Chat 側がある程度読み替えて正しい挙動に寄せることはあり得る。
ただし、それを前提に設計しない。

方針:

* 生文字列は残す
* 正規化候補を別に持つ
* 危険な操作は確認する

---

## 11. 正規化仕様（LLMなし前提）

## 11.1 基本方針

正規化のみなら、最初は LLM 不要とする。
まずは以下で構成する。

* 辞書
* ルール
* あいまい一致

## 11.2 処理順

1. Whisper 生文字列を受ける
2. 軽い正規化をかける
3. 単語単位に切る
4. 辞書照合する
5. 未知語数・未知語率を計算する
6. 判定フラグを立てる

## 11.3 辞書の種類

* 固有名詞辞書
  例: Chat, Worker, Coder, Whisper, SBV2, Shiro, Mio
* 誤変換辞書
  例: Chst → Chat, Wisper → Whisper
* 一時辞書
  直前会話で出た語
* 操作語辞書
  例: 開く, 渡す, 停止, 再起動

## 11.4 判定結果

* `OK`
* `RECONSTRUCT`
* `CONFIRM`

---

## 12. 曖昧判定仕様

曖昧さは「意味がわからない」ではなく、
**候補が 1 つに決まらない状態**として扱う。

## 12.1 曖昧の条件

* 対象候補が複数残る
* 操作候補が複数残る
* 参照語（これ / それ / あれ / さっきの）が未解決
* 文の骨格が足りない
* 危険語なのに確信が低い

## 12.2 判定主体

曖昧判定そのものは Chat 側のロジックで行う。
LLM は必要時のみ補助に使う。

つまり、

* Chat = 審判
* LLM = 救済要員

である。

---

## 13. LLM 利用方針

## 13.1 初期方針

正規化だけなら LLM 不要。
意味再構成が必要な場合のみ、必要最小限に使う。

## 13.2 使いどころ

* `OK`
  → LLM 不要
* `RECONSTRUCT`
  → 必要に応じて LLM 使用
* `CONFIRM`
  → まず確認。LLM に丸投げしない

## 13.3 超軽量日本語 LLM の扱い

超軽量日本語 LLM を入れる場合は、用途を以下に限定する。

* 誤文字列の補正候補生成
* 曖昧な語の補正候補提示

任せないこと:

* 最終意図確定
* 危険操作の実行判断
* Worker/Coder への命令生成

---

## 14. 実装上の重要判断

## 14.1 HTTPS 方式

自己署名証明書の端末配布方式は採用しない。
Tailscale Serve による tailnet 内 HTTPS を採用する。

## 14.2 voice-bridge の配置

voice-bridge は Whisper と同じ PC に置く。
今回は Win11-HP に置く。

理由:

* `WHISPER_URL = http://127.0.0.1:8080/inference` を維持できる
* 切り分けが容易
* HTTP 経路が短く安定する

## 14.3 ブラウザ端末

Lenovo はマイク端末兼ブラウザ端末。
Whisper は Win11-HP にあるが、ブラウザで開いた Lenovo のマイクを使う。

---

## 15. 現時点の動作確認結果

### 確認済み

* `start-whisper.ps1` 単体起動成功
* Whisper 8080 LISTEN 成功
* `GET /` で 200 応答成功
* `/inference` へ `jfk.wav` 投入で JSON 応答成功
* `-l ja --convert --split-on-word` の起動反映成功
* `voice-bridge` 8090 起動成功
* Tailscale Serve で `https://win11-hp01.tailb07d8d.ts.net/` の公開成功
* Lenovo 側ブラウザから画面表示成功
* 一度は暫定字幕表示まで到達

### 既知の問題

* WebM チャンクの単純連結は `FFmpeg conversion failed.` を起こしうる
* したがって、最終方式は「チャンク連結前提」ではなく「無音＋チャンク再解釈」へ寄せる必要がある

---

## 16. 今後の実装順

1. `index.html` を「無音＋チャンク再解釈」方式へ確定
2. `server.js` を finalize 前提の二段字幕方式へ確定
3. テスト経路で安定化
4. Chat に字幕入力を接続
5. Chat → LLM を接続
6. Chat → SBV2 を接続

---

## 17. 仕様上の結論

* STT の入口は Browser + Tailscale HTTPS とする
* Whisper は Chat 配下の入力 I/O とする
* まずは Chat を通さないテスト経路を維持する
* 字幕は「暫定」と「確定」の二層にする
* 確定は「無音＋再解釈」で行う
* 正規化はまず LLM なしで行う
* 曖昧判定は Chat 側ロジックで行う
* LLM は必要時のみ意味再構成に使う

---

```

必要なら次に、この仕様書をそのまま  
**`index.html` / `server.js` / `Chat 連携API` の3分割実装仕様**に落とします。
```
