# PicoClaw Emotion Planner Algorithm Specification

## 1. Goal

Emotion Planner は、イベント・文脈・発話テキストから発話に適した EmotionState を決定する。

EmotionState は以下を含む。

- primary_emotion
- emotion_vector
- prosody
- reason_trace

---

## 2. Decision Pipeline

Emotion Planner は以下の順序で感情を決定する。

1. Event から Base Emotion Vector を取得する
2. Context Rules を適用する
3. Text Feature Rules を適用する
4. VoiceProfile Bias を適用する
5. Vector を正規化する
6. Primary Emotion を決定する
7. Prosody を生成する
8. Reason Trace を出力する

---

## 3. Internal Emotion Vector

内部表現は以下の6軸とする。

- warmth
- cheerfulness
- seriousness
- alertness
- calmness
- expressiveness

各値の範囲は 0.0 から 1.0 とする。

---

## 4. Base Emotion Mapping

各イベントは初期 Emotion Vector を持つ。

例:

- task_success
- task_failure
- approval_requested
- approval_completed
- warning
- error
- analysis_report
- conversation
- system_notification

各イベントに対し、Emotion Vector を設定ファイルで管理する。

---

## 5. Context Adjustment

Context は Emotion Vector に対する差分補正として適用する。

補正ルールはテーブル形式で管理する。

補正対象の例:

- user_waiting_time_sec
- time_of_day
- previous_event
- retry_count
- consecutive_failures
- conversation_mode
- urgency
- user_attention_required

---

## 6. Text Feature Adjustment

Text から軽量特徴を抽出し、Emotion Vector に微補正を加える。

特徴の例:

- gratitude
- apology
- confirmation
- warning_phrase
- success_phrase
- uncertainty_phrase

Text Feature の寄与率は全体の 10% を上限とする。

---

## 7. VoiceProfile Bias

VoiceProfile は人格表現のための固定補正値を持つ。

VoiceProfile は以下を含む。

- axis bias
- axis cap

VoiceProfile Bias は Context と Text の補正後に適用する。

---

## 8. Normalization

補正後の Vector は 0.0 から 1.0 に clamp する。

矛盾しやすい軸の組み合わせには整形ルールを適用する。

例:

- alertness が高い場合は calmness を上限制限する
- cheerfulness が高い場合は seriousness を上限制限する
- seriousness が高い場合は expressiveness を上限制限する

---

## 9. Primary Emotion Derivation

最終 Vector から primary_emotion を決定する。

初期ルール:

1. alertness >= 0.65 → alert
2. seriousness >= 0.60 → serious
3. cheerfulness >= 0.55 → cheerful
4. warmth >= 0.50 → warm
5. otherwise → calm

---

## 10. Prosody Generation

最終 Vector から prosody を生成する。

生成対象:

- speed
- pitch
- pause
- expressiveness

計算式はエンジン非依存の正規化値として保持し、TTS Adapter で各エンジンの値に変換する。

---

## 11. Reason Trace

Emotion Planner は判断根拠を reason_trace として出力する。

reason_trace は以下を含む。

- event
- applied_context_rules
- applied_text_features
- voice_profile

---

## 12. Fail Safe

異常時は以下の方針で処理する。

- 未知イベントは system_notification として扱う
- context 欠落時は無補正
- text feature 抽出失敗時は無補正
- vector は常に clamp する
- prosody はエンジン安全範囲に clamp する

---

## 13. Future Extension

将来的に LLM を補助的に導入できるが、最終感情決定はルールベースを維持する。

LLM の利用対象は以下に限定する。

- text feature extraction support
- special-case suggestion
- rule tuning support
