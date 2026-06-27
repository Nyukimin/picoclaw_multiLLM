# IdleChat Story-Simple 完成本文ストック仕様

**作成日**: 2026-06-28
**対象**: IdleChat `story-simple`
**親仕様**: `IdleChat仕様.md`, `IdleChat_Topic_Cache仕様.md`, `実装仕様_story-simple_v1.md`

## 1. 目的

`story-simple` は、昔話の骨格と置換主人公から、最後まで完結した短編本文を生成して配信する。

IdleChat 本体の進行中に本文生成待ちで止まらないよう、`story-simple` はお題メモではなく、完成本文を10件ストックする。

## 2. ストック単位

`story-simple` の stock item は、次を1単位として保持する。

- `tale_title`
- `synopsis`
- `protagonist`
- `topic`
- `category = story-simple`
- `strategy = story-simple`
- `story_title`
- `story_text`
- `quality_review`
- `revision_note`

`story_text` が空の item は stock に入れない。
topic だけ、または元話と主人公だけの seed は、完成本文 stock として扱わない。

## 3. 生成手順

完成本文の生成は Worker を使う。

基本手順:

1. `simpleStoryTales` から元話を選ぶ。
2. `protagonistOptions` から置換主人公を選ぶ。
3. Worker が短編本文を生成する。
4. 機械チェックで、主人公、元話、本文量、完結性を確認する。
5. Worker が面白さと完結性を判定する。
6. 機械チェック不合格、または Worker 判定 fail の場合、Worker が修正稿を作る。
7. 修正後も完成条件を満たさない場合は、その item を stock しない。

## 4. 完成条件

完成本文は、少なくとも次を満たす。

- 置換主人公が本文またはタイトルに出ている。
- 元話タイトルまたは元話の要素が残っている。
- 企画メモや「もし〜だったら」の説明文ではなく、物語本文である。
- 事件が解決し、オチまたは余韻で終わる。
- Viewer / TTS で段落配信できるだけの本文量がある。

## 5. 補充条件

stock target は10件。

- 起動後にバックグラウンドで10件まで補充する。
- 1件 pop したら、目標数へ戻す補充を予約する。
- `chatActive / chatBusy / workerBusy` 中は、新規生成を開始しない。
- LLM を使う stock 補充は同時多発させない。
- stock が空の場合のみ、同じ `story-simple` の完成本文を同期生成してよい。

同期生成に失敗した場合、Story へ fallback しない。

## 6. セッション利用

`RunSimpleStorySession()` は、完成本文 stock から1件 pop して使う。

セッション中は原則として本文生成を行わず、次を配信する。

- 導入発話
- 生成済みタイトル
- 完成済み本文
- 締め発話

Viewer / TTS / History / Summary では `story-simple` として追跡する。
`story` category / sessionMode / pipeline に戻してはいけない。

## 7. 実装チェックリスト

- [ ] `story-simple` stock target が10件である。
- [ ] stock item が `story_text` を必須にしている。
- [ ] Worker で生成、判定、必要時修正を行う。
- [ ] 完結していない本文を stock しない。
- [ ] pop 後にバックグラウンド補充を予約する。
- [ ] `chatActive / chatBusy / workerBusy` 中は補充生成を開始しない。
- [ ] stock が空でも Story へ fallback しない。
- [ ] セッションは完成本文を Viewer / TTS に段落配信する。
