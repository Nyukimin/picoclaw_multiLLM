# 実装仕様: RenCrow VTuber連携 v0.1

**作成日**: 2026-03-16  
**ステータス**: Draft  
**前提**:
- 本仕様の稼働対象 Core は `Corer4` とする
- `Corer4` の運用名は `Coder4` / `Kin` / `きん` とする
- `Corer4` は `HP-Win11`、`GPU あり`、`LLM なし`、`OBS`、`VTube Studio`、`Steam` 搭載を前提とする
- Live2D モデルは完成済みで別途用意されている
- キャラクター入力は音声のみとする
- 感情推定は RenCrow が生成可能である

**参考資料**:
- VTube Studio Wiki: Lipsync
- VTube Studio Wiki: Plugins
- VTube Studio Wiki: Recording/Streaming with OBS
- VTube Studio Wiki: Expressions
- YouTube Help: Create a live stream with an encoder

---

## 1. 目的

本仕様は、RenCrow の 2 キャラクターである Mio と Shiro を、それぞれ独立した Live2D VTuber として描画し、OBS 上で合成して YouTube へ配信するための最小構成を定義する。

各キャラクターは以下の 2 系統の入力で動作する。

- 音声入力
- RenCrow が生成する感情状態

VTube Studio は Live2D 描画、音声由来の lipsync、表情・姿勢反映を担い、OBS は 2 キャラクター映像を合成し、YouTube へ encoder 配信する。

---

## 2. スコープ

### 2.1 本仕様に含むもの

- RenCrow から VTube Studio への感情制御
- VTube Studio 2 インスタンス運用
- OBS 上での 2 Source 合成
- YouTube への encoder 配信
- 音声ルーティングの責務分離

### 2.2 本仕様に含まないもの

- Live2D モデル作成
- Cubism 上でのモデリング作業
- 音声合成エンジン自体の実装
- YouTube 配信企画や番組進行設計

---

## 3. 用語

- `RenCrow`: 発話生成、音声出力、感情推定、話者状態管理を担う本体
- `VTS-Mio`: Mio 用 VTube Studio インスタンス
- `VTS-Shiro`: Shiro 用 VTube Studio インスタンス
- `OBS Source`: OBS 上で扱う個別映像入力
- `emotion_label`: RenCrow が出力する離散感情ラベル
- `Custom Parameter`: VTube Studio プラグイン/API から注入する連続制御値

---

## 4. 全体構成

本システムは以下の 4 層で構成する。

### 4.1 RenCrow Layer

役割:

- 発話テキスト生成
- 既存音声出力のキャラクター単位分離
- 感情推定
- 話者状態管理
- VTube Studio 向け制御イベント生成
- VTube Studio インスタンスごとの接続管理

### 4.1.1 RenCrow 内部モジュール分割

初期実装では、RenCrow 側を以下の責務に分割する。

- `voice_router`
  - 既存の音声出力結果を `Mio` / `Shiro` ごとに分離する
- `emotion_provider`
  - キャラクターごとの感情状態を生成する
- `vts_client`
  - VTube Studio WebSocket 接続と API 呼び出しを行う
- `vts_dispatcher`
  - 感情 tick を各 VTube Studio へ個別送信する
- `vts_runtime_registry`
  - キャラクターごとの接続状態、ポート、障害状態を保持する

### 4.2 Audio Routing Layer

役割:

- Mio 用音声と Shiro 用音声を分離する
- 各 VTube Studio インスタンスへ独立した音声入力として渡す

### 4.3 VTube Studio Layer

役割:

- Live2D モデル描画
- 音声由来 lipsync 生成
- RenCrow から受け取った感情パラメータ反映
- hotkey / expression による大表情切り替え
- モデル位置、拡大率、回転制御

運用条件:

- Mio 用と Shiro 用に別インスタンスを起動する
- VTube Studio API は local websocket server として利用する
- API 既定ポートは `8001`、競合時は `8002` 以降へ自動でずれる

### 4.4 OBS / YouTube Layer

役割:

- VTS-Mio / VTS-Shiro を別 Source として配置
- 背景や字幕 Source と合成
- YouTube へ encoder 配信

---

## 5. ランタイム構成

最小ランタイム構成は以下とする。

- 稼働 Core: `Corer4` (`Coder4` / `Kin` / `きん`)
- RenCrow 本体: 1
- VTube Studio インスタンス: 2
- OBS: 1
- YouTube Live: 1

構成例:

```text
RenCrow
├ Audio-Out-Mio   -> Audio-In-Mio   -> VTS-Mio   -> NDI Source-Mio   -> OBS
└ Audio-Out-Shiro -> Audio-In-Shiro -> VTS-Shiro -> NDI Source-Shiro -> OBS

OBS -> YouTube Live
```

OBS への映像入力は原則 `NDI` を使う。  
理由は以下の通り。

- Virtual Webcam は 1 ストリームのみ
- 複数 VTube Studio 起動時、NDI では番号付きの別 Source として認識できる
- 透明背景付きの合成に向く

### 5.1 実行時分離要件

- `Mio` と `Shiro` の音声経路は独立していること
- `Mio` と `Shiro` の感情制御経路は独立していること
- 片方の VTube Studio 接続障害がもう片方へ波及しないこと
- 音声出力は VTube Studio 制御系の障害と独立して継続すること

---

## 6. キャラクター単位の責務

### 6.1 Mio パイプライン

```text
RenCrow(Mio) -> 音声出力 -> VTS-Mio 音声入力 -> lipsync -> Live2D 描画 -> OBS Source(Mio)
```

### 6.2 Shiro パイプライン

```text
RenCrow(Shiro) -> 音声出力 -> VTS-Shiro 音声入力 -> lipsync -> Live2D 描画 -> OBS Source(Shiro)
```

### 6.3 感情制御責務

RenCrow は各キャラクターの感情推定結果を独立して保持し、それぞれの VTube Studio インスタンスへ個別送信する。

- Mio 向け制御は `VTS-Mio` の API ポートへ送る
- Shiro 向け制御は `VTS-Shiro` の API ポートへ送る

---

## 7. 責務分離

### 7.1 RenCrow が担当するもの

- 発話内容生成
- 話者選択
- 感情推定
- 話者ごとの発話中状態管理
- VTube Studio へ送る制御値生成

VTube Studio 単体では意味ベースの演技判断を持たせず、`happy`、`thinking`、`annoyed` のような意味解釈は RenCrow が担う。

### 7.2 VTube Studio が担当するもの

- lipsync
- Live2D パラメータ反映
- hotkey による expression 切り替え
- モデル位置、拡大率、回転制御

### 7.3 OBS が担当するもの

- 2 キャラクター映像の合成
- 背景・字幕との合成
- YouTube への送出

---

## 8. VTube Studio 利用方針

### 8.1 音声のみで成立させる動き

VTube Studio は音声由来の以下の lipsync 系パラメータを出力できる前提とする。

- `VoiceA`
- `VoiceI`
- `VoiceU`
- `VoiceE`
- `VoiceO`
- `VoiceVolume`
- `VoiceFrequency`
- `VoiceSilence`

これらは口形状だけでなく、任意の Live2D パラメータへ割り当て可能とする。初期実装では以下の用途を推奨する。

- `VoiceA/I/U/E/O`: 口パク
- `VoiceVolume`: 声量に応じた体揺れや頬の変化
- `VoiceFrequency`: 眉や頭部の微変化
- `VoiceSilence`: 待機表情への復帰

### 8.2 感情反映方針

- 小さな感情変化は連続値制御で反映する
- 大きな顔切り替えは hotkey / expression で反映する

全部を hotkey に寄せると切り替えが不自然になりやすく、全部を連続値に寄せると顔の意味が曖昧になるため、二層構成を標準とする。

---

## 9. 感情制御インターフェース仕様

### 9.1 更新周期

各キャラクターごとに、RenCrow は `50ms` から `100ms` 周期で感情状態を出力する。

### 9.2 出力フィールド

- `speaking`: `0 | 1`
- `valence`: `-1.0 .. 1.0`
- `arousal`: `0.0 .. 1.0`
- `intensity`: `0.0 .. 1.0`
- `emotion_label`: `neutral | happy | calm | thinking | surprised | annoyed | sad`

### 9.3 意味

- `speaking`
  - 発話中フラグ
  - lipsync 有効化の補助条件として使う
- `valence`
  - 快・不快の連続値
  - 眉、口角、頬などの方向性制御に使う
- `arousal`
  - 覚醒度
  - 目の開き、呼吸、揺れ量に使う
- `intensity`
  - 演技の強さ
  - 各変化量の最大幅を制御する
- `emotion_label`
  - 離散感情ラベル
  - 大きな表情切り替えを expression / hotkey で行う

### 9.4 初期 JSON 仕様

```json
{
  "type": "emotion_tick",
  "character": "mio",
  "timestamp_ms": 1742083200000,
  "payload": {
    "speaking": 1,
    "valence": 0.72,
    "arousal": 0.58,
    "intensity": 0.64,
    "emotion_label": "happy"
  }
}
```

### 9.5 キャラクター識別子

初期実装では `character` は以下のいずれかとする。

- `mio`
- `shiro`

### 9.6 公開イベント型

初期実装で外部連携上の論理イベント型として公開するのは `emotion_tick` のみとする。

- `emotion_tick`
  - RenCrow から VTube Studio へ送る連続感情制御イベント

初期版では OBS / YouTube 向けの追加イベントは定義しない。

### 9.7 ランタイム型

実装時は少なくとも以下のランタイム情報を保持する。

#### `CharacterVTState`

- `character_id`
- `audio_input_target`
- `vts_endpoint`
- `connection_state`
- `last_emotion_tick_at`
- `last_hotkey_at`
- `last_error`

#### `VTSConnectionConfig`

- `host`
- `port`
- `character_id`
- `expression_map`
- `tick_interval_ms`

`connection_state` は以下のいずれかとする。

- `disconnected`
- `connecting`
- `ready`
- `degraded`

---

## 10. VTube Studio 反映ルール

初期マッピングは以下を標準案とする。

| RenCrow 値 | VTS 内の反映対象 | 反映方式 |
|------------|------------------|----------|
| `speaking` | lipsync 有効状態 | 補助フラグ |
| `valence` | 眉、口角、頬 | Custom Parameter |
| `arousal` | 目の開き、呼吸、揺れ量 | Custom Parameter |
| `intensity` | 各変化量の倍率 | Custom Parameter |
| `emotion_label` | 大表情切替 | hotkey / expression |

### 10.1 Expression 運用

`emotion_label` は `.exp3.json` ベースの expression として管理し、VTube Studio 側では hotkey 経由を標準運用とする。

初期ラベルと推奨 expression 名:

- `neutral`
- `happy`
- `calm`
- `thinking`
- `surprised`
- `annoyed`
- `sad`

### 10.2 競合解決

連続値制御と expression が競合する場合は以下を優先する。

1. `emotion_label` による expression
2. `intensity`
3. `valence` / `arousal`
4. 音声由来パラメータ

---

## 11. 音声ルーティング仕様

Mio と Shiro の音声は必ず別系統に分離する。

### 11.1 責務分離

初期実装では、音声ルーティング責務は VTube Studio 側ではなく `Coder4` 側の `AudioRouter` が持つ。

- `VTS module`
  - `emotion_tick` の受信
  - 表情制御
  - lipsync 用マイク入力の受け取り
- `AudioRouter`
  - `tts.audio_chunk` の受信
  - `character_id` ごとの再生先分離
  - Windows 音声デバイスへの再生

VTube Studio API は音声そのものを中継しない。口パク入力は OS 音声デバイス経由で与える。

### 11.2 Coder4 AudioRouter

`Coder4` には Windows 用 `picoclaw-agent.exe --agent audio_router` を配置し、`/audio-router/events` を SSE 購読させる。

- 入力イベント: `tts.audio_chunk`
- 必須フィールド:
  - `session_id`
  - `chunk_index`
  - `character_id`
  - `audio_url`
- 振り分け規則:
  - `character_id == "mio"` -> `Mio` 用再生デバイス
  - `character_id == "shiro"` -> `Shiro` 用再生デバイス

### 11.3 Coder4 実デバイス構成

`Coder4` の初期構成は以下に固定する。

- `mio` 再生先:
  - `CABLE-A Input (VB-Audio Virtual Cable A)`
  - Device ID: `{0.0.0.00000000}.{2c3d4825-5957-4eac-9bf3-a4b11a5191f9}`
- `shiro` 再生先:
  - `CABLE-B Input (VB-Audio Virtual Cable B)`
  - Device ID: `{0.0.0.00000000}.{54dc1edf-2e33-496d-a7d6-7dd244fcbd4c}`

### 11.4 通常Chat お題生成の具体化

通常Chat の `single` / `double` お題は、ジャンル名だけで閉じた抽象題名に寄りすぎないよう、`具体アンカー` を必ず 1 つ含める。

- 対象:
  - `single`
  - `double`
- 対象外:
  - `external`
  - 映画モード (`「〜」ってどんな映画？`)

`具体アンカー` は次のいずれか 1 つを指す。

- 人物
  - 例: `アーティスト`、`学芸員`、`修理屋`
- 物
  - 例: `古いカセットテープ`、`壊れたオルゴール`
- 場所
  - 例: `港町の倉庫街`、`始発前の駅`
- 場面
  - 例: `雨上がりの朝`、`展示替えの直前`

生成ルールは以下とする。

- `single`
  - `ジャンル 1 つ + 具体アンカー 1 つ`
- `double`
  - `ジャンル 2 つ + 具体アンカー 1 つ`
- prompt には `人・物・場所・場面のどれかを 1 つ必ず入れる` を明示する
- prompt には `抽象語だけで閉じた題名にしない` を明示する
- fallback 題名も、可能な限り具体アンカーを含めた題名にする

期待する改善点:

- `音楽`、`記憶`、`比喩` のような抽象ジャンル語だけで終わる題名を減らす
- `ジャンル -> 人物/現場/物` の振れ幅を作る
- 会話開始直後に、場面や対象物のイメージが立ち上がるようにする

### 11.5 Story モード

IdleChat に `story` 専用モードを追加する。

- 起動方式:
  - Viewer からの手動起動
  - `forecast` と並ぶ専用モード
- 作品取得:
  - repo 同梱の public-domain 物語 corpus を使用する
  - 初期 corpus は昔話+童話を約 24 本
- 役割分担:
  - `Shiro`: 元作品全文の読解、改変プラン生成、短編本文生成
  - `Mio`: 完成した短編の読み上げ

改変方式は次の 3 種を持つ。

- `role_shift`
  - 役割や職能を入れ替え、現代の地続きの世界で再配置する
- `view_shift`
  - 脇役や裏方など、元作品とは別視点から再構成する
- `value_shift`
  - 報酬や善悪の意味を反転させる

生成パイプライン:

1. 元作品全文を選ぶ
2. 元作品の `必須モチーフ`、`禁忌/約束`、`報酬/罰`、`読後感` を分析する
3. `Shiro` が改変プランと `導入 / 逸脱 / 反転 / 着地` の 4 ビートを作る
4. `Shiro` が面白さ優先の第1稿を作る
5. `Shiro` が第1稿を改稿し、因果と読後感を整えた第2稿を作る
6. `Mio` が第2稿だけを文単位で分割読み上げする

元作品らしさを保つため、各作品には `必須モチーフ` を定義する。

- 例:
  - `舌切り雀`
    - `舌を切る`
    - `小さいつづら`
    - `大きいつづら`
    - `欲深さの報い`
  - `浦島太郎`
    - `亀を助ける`
    - `竜宮城`
    - `玉手箱`
    - `時間のずれ`

Story モードの改変では、これらの必須モチーフを削除してはならない。時勢の過去/未来移行は行わず、舞台は現代の地続きの世界に固定する。

- `role_shift`
  - 元モチーフを役割転換・職能転換した形で再登場させる
- `view_shift`
  - 元モチーフを脇役側の観測・誤解・証拠として再登場させる
- `value_shift`
  - 元モチーフを報酬/罰や善悪の反転として再登場させる

改変プランには最低限以下を含める。

- `source_title`
- `rewrite_style`
- `story_title`
- `premise`
- `ending_flavor`
- `core_motifs`
- `motif_map`

`motif_map` は `元モチーフ => 改変後の具体物/出来事` を表し、本文生成時にもそのまま強制条件として渡す。

第1稿と第2稿の役割分担:

- 第1稿
  - 面白さ優先
  - 中盤で一度大きく逸脱してよい
- 第2稿
  - 第1稿の面白さを残しつつ、因果と余韻を整える
  - 明示的な起承転結ラベルは使わないが、`導入 / 逸脱 / 反転 / 着地` が感じられるようにする

要件:

- 最初の 2 文で `何をどうひねったか` が分かること
- 必須モチーフが本文中に具体的に出ること
- 元話をなぞるだけでなく、対立や選択の意味が変わっていること
- 第2稿で読後感が未着地にならないこと
- それでも聞き手が `元は何の話か` を感じ取れること

TTS 運用:

- 本文は句点・改行・文字数上限を基準に分割する
- 各チャンクは既存 `idlechat.message -> TTS` 経路へ流す
- 元作品名は Viewer と履歴ログの両方に残す

補足:

- `AudioRouter` が再生するのは `Input` 側デバイスである
- VTube Studio 側のマイク入力には対応する `Output` 側を割り当てる
- `In 16ch` デバイスは初期運用では使わない

### 11.4 VTube Studio 側入力設定

`Coder4` での VTube Studio 入力先は以下に固定する。

- `VTS-Mio`
  - マイク入力: `CABLE-A Output (VB-Audio Virtual Cable A)`
- `VTS-Shiro`
  - マイク入力: `CABLE-B Output (VB-Audio Virtual Cable B)`

これにより、`AudioRouter -> VB-CABLE -> VTS` の 1:1 配線を維持する。

### 11.5 運用確認

起動確認は以下の順で行う。

1. `Coder4` で `picoclaw-agent.exe --standalone --agent audio_router --config C:\Users\nyuki\.picoclaw\config.yaml` を起動する
2. `VTS-Mio` と `VTS-Shiro` を別インスタンスで起動する
3. `Mio` 発話で `CABLE-A` 系のみが動くことを確認する
4. `Shiro` 発話で `CABLE-B` 系のみが動くことを確認する
5. Viewer の `tts.audio_chunk.character_id` と実再生先が一致することを確認する

必須構成:

- `Audio-In-Mio -> VTS-Mio`
- `Audio-In-Shiro -> VTS-Shiro`

制約:

- 両インスタンスで同一音声入力デバイスを共有しない
- 各キャラクターの音声経路は論理的にも物理的にも分離する

理由:

- 分離しない場合、両キャラクターが同一音声で同時に口パクする
- 二者会話ではなく分身挙動になる

---

## 12. OBS 仕様

OBS では少なくとも以下の Source を構成する。

- `Source-Mio`: VTS-Mio の NDI Source
- `Source-Shiro`: VTS-Shiro の NDI Source
- 背景 Source
- 字幕または Browser Source

### 12.1 OBS 合成要件

- Mio / Shiro を個別に位置調整できること
- 背景透過を維持できること
- 各 Source の表示順を固定できること

### 12.2 NDI 採用理由

- 複数 VTube Studio を別 Source として拾いやすい
- 透明背景に向く
- UI を含まない映像経路を構成しやすい

### 12.3 性能注意

- NDI は CPU 使用率を押し上げる可能性がある
- 不要なら Virtual Webcam と同時有効化しない

---

## 13. YouTube 配信仕様

YouTube 配信は encoder 配信を前提とする。

OBS 側で以下を設定する。

- 配信 URL
- Stream Key
- 解像度
- フレームレート
- ビットレート

運用:

1. YouTube Live Control Room でストリームを作成する
2. OBS にサーバー URL / Stream Key を設定する
3. OBS から送出し、YouTube 側でプレビューを確認する
4. 問題がなければ `Go live` する

---

## 14. API 接続仕様

### 14.1 接続単位

RenCrow は各 VTube Studio インスタンスと独立接続する。

- 接続 A: `VTS-Mio`
- 接続 B: `VTS-Shiro`

### 14.2 ポート発見

初期実装では以下のいずれかで接続先を確定する。

- 手動設定
- 起動時ログまたは接続確認で検出

既定値は以下とする。

- `VTS-Mio`: `8001`
- `VTS-Shiro`: `8002`

ただし、実際の割当は起動順や競合状況で変動しうるため、固定値前提では運用しない。

初期版では自動ポート検出は実装対象外とし、設定値ベースで接続する。

### 14.3 制御対象

API / プラグイン経由で以下を制御対象とする。

- hotkey 発火
- tracking parameter 制御
- custom tracking parameter 注入
- モデル位置、拡大率、回転

### 14.4 接続失敗時の挙動

- VTube Studio 接続失敗時は、該当キャラクターの表情制御のみ停止する
- VTube Studio 接続失敗時でも、該当キャラクターの音声出力は継続する
- 切断時はバックグラウンドで再接続を試みる
- 再接続待機中は最新 tick のみを対象とし、再送キューは持たない

---

## 15. Corer4 (`Coder4` / `Kin` / `きん`) 向け運用条件

対象 Core は `Corer4` とし、`HP-Win11`、`GPU あり`、`LLM なし`、`OBS`、`VTube Studio`、`Steam` 搭載を前提とする。

対象 PC がマルチ GPU 構成である場合、OBS と VTube Studio は同じ dedicated GPU 側へ寄せる。

必須運用:

- OBS と VTube Studio を同一 GPU 上で動作させる
- 重い同時起動時は VTube Studio の優先度を確認する
- タスクマネージャーで CPU / GPU 使用率を監視する

推奨運用:

- VTube Studio は `60 FPS` 制限を標準とする
- NDI と Virtual Webcam の同時有効化を避ける

---

## 16. 初期実装の標準構成

v0.1 の唯一解として、初期実装は以下に固定する。

1. Mio / Shiro の完成済み Live2D モデルを用意する
2. VTube Studio を 2 インスタンス起動する
3. NDI で OBS に 2 Source として取り込む
4. 各 VTS に別々の音声入力を与える
5. RenCrow から各 VTS に `valence` `arousal` `intensity` `emotion_label` `speaking` を送る
6. 口パクは VTube Studio 標準 lipsync を使う
7. 感情制御は RenCrow 主導で行う

この構成は、VTube Studio の複数起動、NDI 多重取り込み、API によるパラメータ注入、hotkey 制御の範囲に収まる最小構成である。

---

## 17. 非機能要件

### 17.1 安定性

- 片方の VTube Studio インスタンス障害が他方へ波及しないこと
- Mio / Shiro の制御経路を分離すること

### 17.2 性能

- 感情更新は `50ms` から `100ms` 周期で処理落ちしないこと
- OBS 合成時に配信フレームレートを維持できること

### 17.3 可観測性

少なくとも以下をログ出力対象とする。

- キャラクターごとの接続先ポート
- 感情更新送信成功/失敗
- hotkey 発火成功/失敗
- 音声ルーティング異常
- 再接続開始/成功/失敗

### 17.4 障害分離

- `Mio` と `Shiro` の障害は互いに独立に扱う
- 一方の hotkey 発火失敗は他方の連続値制御に影響しない
- 制御系障害は既存音声出力を停止させない
- VTS 障害時のフォールバックは「音声のみ出力」とする

---

## 18. 既知の注意点

- NDI は複数 Source と透明背景に強い一方、CPU コストが増える
- Virtual Webcam は 1 ストリーム前提のため、2 キャラクター独立運用には不向き
- ポート番号は `8001` 固定ではなく、自動で後続番号へずれることがある
- 音声分離を怠ると両キャラクターが同時に同じ口パクを行う

---

## 19. 設定仕様

RenCrow 側には最低限以下の設定を持たせる。

```yaml
vtuber:
  enabled: true
  tick_interval_ms: 100
  characters:
    mio:
      audio_output: "Audio-Out-Mio"
      vts_host: "127.0.0.1"
      vts_port: 8001
      expression_map:
        neutral: "ExpNeutral"
        happy: "ExpHappy"
        calm: "ExpCalm"
        thinking: "ExpThinking"
        surprised: "ExpSurprised"
        annoyed: "ExpAnnoyed"
        sad: "ExpSad"
    shiro:
      audio_output: "Audio-Out-Shiro"
      vts_host: "127.0.0.1"
      vts_port: 8002
      expression_map:
        neutral: "ExpNeutral"
        happy: "ExpHappy"
        calm: "ExpCalm"
        thinking: "ExpThinking"
        surprised: "ExpSurprised"
        annoyed: "ExpAnnoyed"
        sad: "ExpSad"
audio_router:
  enabled: true
  sse_url: "http://100.96.186.107:18790/audio-router/events"
  connect_timeout_ms: 5000
  download_timeout_ms: 15000
  retry_delay_ms: 2000
  buffer_ms: 120
  device_map:
    mio:
      device_id: "{0.0.0.00000000}.{2c3d4825-5957-4eac-9bf3-a4b11a5191f9}"
      display_name: "CABLE-A Input (VB-Audio Virtual Cable A)"
    shiro:
      device_id: "{0.0.0.00000000}.{54dc1edf-2e33-496d-a7d6-7dd244fcbd4c}"
      display_name: "CABLE-B Input (VB-Audio Virtual Cable B)"
```

設定ルール:

- `vtuber.enabled=false` の場合、VTube Studio 連携は無効化する
- `tick_interval_ms` の初期値は `100` とする
- キャラクターごとの `audio_output` と `vts_port` は必須とする
- expression 名は VTube Studio 側に事前定義されている前提とする

---

## 20. 受け入れテスト

最低限、以下の動作確認を受け入れ条件とする。

### 20.1 接続分離

- `Mio` と `Shiro` が別ポートへ個別接続される
- `Mio` の感情更新が `Shiro` に送られない
- `Shiro` の感情更新が `Mio` に送られない

### 20.2 音声分離

- `Mio` の音声出力が `VTS-Mio` にのみ入る
- `Shiro` の音声出力が `VTS-Shiro` にのみ入る
- 同一音声入力共有時の誤動作が設定上防止される

### 20.3 感情反映

- `emotion_label=happy` が対応 expression / hotkey に変換される
- `valence` `arousal` `intensity` が連続値制御に変換される
- `speaking=0` 時に lipsync 補助状態が無効になる

### 20.4 障害時継続

- VTS 切断時に該当キャラクターの表情制御が停止する
- VTS 切断時でも該当キャラクターの音声出力が継続する
- 再接続後に最新 tick 送信が再開される
- VTube Studio 制御失敗時でも音声出力が継続する

---

## 21. 今後の拡張候補

本仕様では扱わないが、将来拡張として以下を想定する。

- モジュール構成への落とし込み
- イベント定義の厳密化
- JSON Schema 化
- 自動ポート検出
- 感情からカメラワークやレイアウトを変える OBS 連携
- 3 キャラクター以上への拡張

---

## 付録 A. 初期イベント例

```json
{
  "type": "emotion_tick",
  "character": "shiro",
  "timestamp_ms": 1742083200123,
  "payload": {
    "speaking": 1,
    "valence": -0.22,
    "arousal": 0.61,
    "intensity": 0.73,
    "emotion_label": "annoyed"
  }
}
```

## 付録 B. 参考 URL

- https://github.com/DenchiSoft/VTubeStudio/wiki/Lipsync
- https://github.com/DenchiSoft/VTubeStudio/wiki/Plugins
- https://github.com/DenchiSoft/VTubeStudio/wiki/Recording-Streaming-with-OBS/22b63214e6de3a7e7de2bd0cc515558ff47d3e2e
- https://github.com/DenchiSoft/VTubeStudio/wiki/Expressions-%28a.k.a.-Stickers-or-Emotes%29
- https://github.com/DenchiSoft/VTubeStudio/wiki/Lag-Troubleshooting
- https://support.google.com/youtube/answer/2907883?hl=en
