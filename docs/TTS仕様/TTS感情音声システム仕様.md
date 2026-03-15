
```markdown
# RenCrow Emotion Voice System Specification
Emotion Planner + TTS Adapter

Version: 1.0  
Author: RenCrow Architecture  
Status: Draft

---

# 1. Purpose

RenCrow の音声出力を以下の条件で実装する。

- 状況に応じた感情を持つ音声を生成する
- TTSエンジンに依存しない設計にする
- 将来のTTS変更に耐える

このため、音声生成を以下の3層構造とする。

```

Chat → Emotion Planner → TTS Adapter → TTS Engine

```

Emotion Planner は **発話感情を決定する**。  
TTS Adapter は **感情状態を各TTSエンジンのパラメータに変換する**。

---

# 2. System Architecture

## 2.1 Component Overview

```

Chat
├ Dialogue Manager
├ Text Planner
├ Emotion Planner
└ TTS Request Builder
↓
TTS Adapter
↓
TTS Engine
↓
Audio Output

```

---

# 3. Responsibility Separation

## Chat

役割

- ユーザーとの対話窓口
- 世間話・会話文脈の管理
- Worker結果のユーザー向け翻訳
- Emotion決定
- TTS生成要求

Chat は RenCrow の **人格レイヤー** として振る舞う。

---

## Worker

役割

- 処理の実行
- 処理結果の要約
- Emotion判断に必要なメタデータの提供

Worker は **感情決定を行わない**。

---

## TTS Adapter

役割

- EmotionState を各TTSエンジンのパラメータへ変換

TTS Adapter は **ロジックを持たない変換レイヤー** とする。

---

# 4. Emotion Planner

Emotion Planner は Chat コンポーネント内に配置する。

理由

- Chat が世間話を管理する唯一のコンポーネント
- Chat がすべてのユーザー報告の窓口
- 発話感情は「処理結果」ではなく「伝え方」に属する

Worker は感情を決定せず、判断材料のみ提供する。

---

# 5. Emotion Decision Model

Emotion Planner は三段階で感情を決定する。

```

Base Emotion ← Event
Context Adjustment ← Context
Text Adjustment ← Text Features

```

重み

```

Event     70%
Context   20%
Text      10%

````

---

# 6. EmotionState

Emotion Planner の出力。

```json
{
  "emotion": "warm",
  "intensity": 0.45,
  "speed": 1.05,
  "pitch": 1.02,
  "pause": "normal",
  "expressiveness": 0.35,
  "reason": {
    "event": "approval_completed",
    "context": ["user_waiting"],
    "text_features": ["gratitude"]
  }
}
````

---

# 7. Emotion Categories

初期バージョンでは以下を使用する。

```

calm
warm
cheerful
serious
alert

```

説明

**calm**
落ち着いた説明

**warm**
柔らかい会話

**cheerful**
成功・ポジティブ

**serious**
注意・重要説明

**alert**
警告

---

# 8. Event Input

Emotion Planner の主要入力。

```

task_success
task_failure
approval_requested
approval_completed
warning
error
analysis_report
conversation
system_notification

```

イベント → Base Emotion

```

task_success → cheerful
task_failure → serious
approval_requested → calm
approval_completed → warm
warning → alert
error → alert
analysis_report → calm
conversation → warm
system_notification → calm

```

---

# 9. Context Input

Emotion Planner に渡される文脈情報。

```json
{
  "conversation_mode": "report",
  "user_waiting_time": 25,
  "time_of_day": "night",
  "previous_event": "task_failure",
  "retry_count": 1
}
```

文脈による影響例

長時間待ち
→ warmth 上昇

深夜
→ speech_speed 減少

失敗後成功
→ cheerful 上昇

---

# 10. Text Features

発話テキストから検出する軽量特徴。

例

```

gratitude
apology
confirmation
warning_phrase
success_phrase

```

例

```

ありがとうございます → gratitude
問題ありません → confirmation

```

---

# 11. Worker Response Format

Worker は Emotion Planner 用メタデータを含む。

```json
{
  "event_type": "task_success",
  "severity": "low",
  "requires_user_attention": false,
  "user_impact": "medium",
  "retry_count": 1,
  "summary": "同期処理が完了しました",
  "details": {
    "duration_sec": 42,
    "items_processed": 18
  }
}
```

---

# 12. TTS Adapter Interface

Adapter インタフェース。

```

generateVoice(text, emotionState, voiceProfile)

```

戻り値

```

audioBuffer

```

---

# 13. Azure Adapter Example

EmotionState

```

emotion = warm
intensity = 0.4
speed = 1.05
pitch = 1.02

```

SSML変換

```

style = friendly
rate = +5%
pitch = +2%

```

SSML例

```xml
<speak>
  <voice name="ja-JP-NanamiNeural">
    <prosody rate="+5%" pitch="+2%">
      確認できました。続行します。
    </prosody>
  </voice>
</speak>
```

---

# 14. ElevenLabs Adapter Example

EmotionState

```

emotion = cheerful
intensity = 0.6
expressiveness = 0.5

```

変換

```

stability
similarity_boost
style_exaggeration

```

例

```

stability = 0.45
similarity_boost = 0.75
style_exaggeration = 0.5

```

---

# 15. Voice Profile

声の人格は VoiceProfile で管理する。

```json
{
  "voice_id": "lumina",
  "base_pitch": 1.0,
  "base_speed": 1.0,
  "warmth_bias": 0.1,
  "expressiveness_bias": 0.2
}
```

---

# 16. Audio Cache

音声生成コスト削減のためキャッシュを行う。

キー

```

hash(
text,
emotion,
voice_profile,
tts_engine
)

```

同一条件の場合は音声を再利用する。

---

# 17. Voice Generation Pipeline

```

Worker Result
      ↓
Chat
      ↓
Text Planner
      ↓
Emotion Planner
      ↓
TTS Adapter
      ↓
TTS Engine
      ↓
Audio Cache
      ↓
Audio Output

```

---

# 18. Future Extensions

追加Emotion

```

relief
curious
playful
concerned

```

将来拡張

* LLM補助Emotion Planner
* ユーザー個別音声調整
* Voice Cloning
* Local TTS統合

```
