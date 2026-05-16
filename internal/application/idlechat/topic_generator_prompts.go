package idlechat

import (
	"fmt"
	"math/rand"
	"strings"
)

// pickRandom はスライスからn個をランダムに選択
func pickRandom(slice []string, n int) []string {
	if n >= len(slice) {
		// シャッフルして全て返す
		result := make([]string, len(slice))
		copy(result, slice)
		rand.Shuffle(len(result), func(i, j int) {
			result[i], result[j] = result[j], result[i]
		})
		return result
	}

	indices := rand.Perm(len(slice))[:n]
	result := make([]string, n)
	for i, idx := range indices {
		result[i] = slice[idx]
	}
	return result
}

func topicPromptFooter(movieMode bool) string {
	if movieMode {
		return `回答は映画タイトル妄想のお題だけを1行で出力してください。
- 必ず「〜ってどんな映画？」の形にする
- 実在映画名は使わない
- タイトル部分は短く印象的にする
- 質問文は最後の「どんな映画？」だけにする
- 40文字以内を目安に簡潔にする`
	}
	return `回答はお題だけを1行で出力してください。
- 質問文・感想文・呼びかけは禁止
- 「〜って面白いんじゃない？」のような会話調は禁止
- 体言止め、または「〜の関係」「〜を考える」のような題名調にする
- ジャンル名だけで終わらせず、人・物・場所・場面のどれかを1つ必ず入れる
- 40文字以内を目安に簡潔にする`
}

func pickTopicAnchor() topicAnchor {
	return topicAnchorPool[rand.Intn(len(topicAnchorPool))]
}

func buildSingleGenrePrompt(genre string, anchor topicAnchor, movieMode bool) string {
	bannedKeywords := extractBannedKeywords()
	return fmt.Sprintf(`以下のジャンルを深掘りした、興味深い話題を1つ提案してください。

ジャンル: %s
具体アンカー (%s): %s

要件:
- 深い洞察と新しい視点
- 会話が発展する具体性
- エンターテイメント性
- ジャンル名だけで終わらせず、具体アンカーを自然に織り込む

禁止事項:
- %s に関するトピックは避ける
- 「もし〜だったら」形式は使わない
- 教科書的な真面目な説明は避ける
- 直近トピックと類似した内容は避ける
- 抽象語だけで閉じた題名にしない

%s`, genre, anchor.Kind, anchor.Value, strings.Join(bannedKeywords, "、"), topicPromptFooter(movieMode))
}

func buildDoubleGenrePrompt(genres []string, anchor topicAnchor, movieMode bool) string {
	bannedKeywords := extractBannedKeywords()
	return fmt.Sprintf(`以下の2つのジャンルを組み合わせた、面白い話題を1つ提案してください。

ジャンル: %s × %s
具体アンカー (%s): %s

要件:
- 意外な組み合わせだが、深く考えると繋がりが見える
- 会話が深まる具体性
- 適度なエンターテイメント性
- 2ジャンルに具体アンカーを接続し、人・物・場所・場面が見える題名にする

禁止事項:
- %s に関するトピックは避ける
- 「もし〜だったら」形式は使わない
- 教科書的な真面目な組み合わせは避ける
- 直近トピックと類似した内容は避ける
- 抽象語だけで閉じた題名にしない

%s`, genres[0], genres[1], anchor.Kind, anchor.Value, strings.Join(bannedKeywords, "、"), topicPromptFooter(movieMode))
}

// generateSingleGenrePrompt は1ジャンル単体のプロンプトを生成
func generateSingleGenrePrompt(movieMode bool) (string, []string, topicAnchor) {
	genres := pickRandom(genrePool, 1)
	anchor := pickTopicAnchor()
	return buildSingleGenrePrompt(genres[0], anchor, movieMode), genres, anchor
}

// generateDoubleGenrePrompt は2ジャンル掛け合わせのプロンプトを生成
func generateDoubleGenrePrompt(movieMode bool) (string, []string, topicAnchor) {
	genres := pickRandom(genrePool, 2)
	anchor := pickTopicAnchor()
	return buildDoubleGenrePrompt(genres, anchor, movieMode), genres, anchor
}

// generateExternalPrompt は外部刺激を使ったプロンプトを生成
func generateExternalPrompt(movieMode bool) (string, string) {
	cache := getDailyCache()
	if cache == nil {
		// フォールバック: 2ジャンル生成
		p, _, _ := generateDoubleGenrePrompt(movieMode)
		return p, "fallback"
	}

	// Wikipedia or News からランダム選択
	var seed string
	var source string

	if len(cache.WikipediaSeeds) > 0 && len(cache.NewsSeeds) > 0 {
		if rand.Intn(2) == 0 {
			seed = cache.WikipediaSeeds[rand.Intn(len(cache.WikipediaSeeds))]
			source = "Wikipedia"
		} else {
			seed = cache.NewsSeeds[rand.Intn(len(cache.NewsSeeds))]
			source = "News"
		}
	} else if len(cache.WikipediaSeeds) > 0 {
		seed = cache.WikipediaSeeds[rand.Intn(len(cache.WikipediaSeeds))]
		source = "Wikipedia"
	} else if len(cache.NewsSeeds) > 0 {
		seed = cache.NewsSeeds[rand.Intn(len(cache.NewsSeeds))]
		source = "News"
	} else {
		// フォールバック: 2ジャンル
		p, _, _ := generateDoubleGenrePrompt(movieMode)
		return p, "fallback"
	}

	genre := pickRandom(genrePool, 1)[0]
	bannedKeywords := extractBannedKeywords()

	prompt := fmt.Sprintf(`以下の外部刺激とジャンルを組み合わせた、意外性のある話題を1つ提案してください。

外部刺激 (%s): %s
組み合わせジャンル: %s

要件:
- 予想外の切り口を優先
- 深く考察できる具体的な話題
- エンターテイメント性重視

禁止事項:
- %s に関するトピックは避ける
- 「もし〜だったら」形式は使わない

%s`, source, seed, genre, strings.Join(bannedKeywords, "、"), topicPromptFooter(movieMode))

	return prompt, source + ":" + seed
}

// extractBannedKeywords は頻出キーワードを抽出
func extractBannedKeywords() []string {
	return []string{
		"AI", "タイムマシン", "過去", "未来", "宇宙人",
		"もし", "だったら", "なら", "想像", "考えて",
	}
}
