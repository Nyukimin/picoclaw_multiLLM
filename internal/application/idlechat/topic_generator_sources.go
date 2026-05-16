package idlechat

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// DailySeedCache は1日1回取得する外部シードのキャッシュ
type DailySeedCache struct {
	Date           string    `json:"date"`
	WikipediaSeeds []string  `json:"wikipedia_seeds"`
	NewsSeeds      []string  `json:"news_seeds"`
	FetchedAt      time.Time `json:"fetched_at"`
}

// fetchDailySeeds は1日1回、起動時に外部シードを取得してキャッシュ
func fetchDailySeeds() error {
	today := time.Now().In(jst).Format("2006-01-02")

	cacheMu.RLock()
	if dailyCache != nil && dailyCache.Date == today {
		cacheMu.RUnlock()
		return nil // 既に取得済み
	}
	cacheMu.RUnlock()

	cacheMu.Lock()
	defer cacheMu.Unlock()

	// ダブルチェック
	if dailyCache != nil && dailyCache.Date == today {
		return nil
	}

	log.Printf("[IdleChat] Fetching daily seeds for %s...", today)

	// Wikipedia Random（10件）
	wikiSeeds, err := fetchWikipediaRandom(10)
	if err != nil {
		log.Printf("[IdleChat] Wikipedia fetch failed: %v", err)
		wikiSeeds = []string{} // フォールバック
	}

	// News Headlines（NHK RSS、10件）
	newsSeeds, err := fetchNewsHeadlines(10)
	if err != nil {
		log.Printf("[IdleChat] News fetch failed: %v", err)
		newsSeeds = []string{} // フォールバック
	}

	dailyCache = &DailySeedCache{
		Date:           today,
		WikipediaSeeds: wikiSeeds,
		NewsSeeds:      newsSeeds,
		FetchedAt:      time.Now(),
	}

	log.Printf("[IdleChat] Daily seeds fetched: Wikipedia=%d, News=%d", len(wikiSeeds), len(newsSeeds))
	return nil
}

// fetchWikipediaRandom はWikipedia Random APIから記事タイトルを取得
func fetchWikipediaRandom(limit int) ([]string, error) {
	url := fmt.Sprintf("https://ja.wikipedia.org/w/api.php?action=query&list=random&rnlimit=%d&rnnamespace=0&format=json", limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RenCrow/1.0 (https://github.com/Nyukimin/picoclaw_multiLLM)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("wikipedia api returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Query struct {
			Random []struct {
				Title string `json:"title"`
			} `json:"random"`
		} `json:"query"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	titles := make([]string, 0, len(result.Query.Random))
	for _, item := range result.Query.Random {
		titles = append(titles, item.Title)
	}

	return titles, nil
}

// fetchNewsHeadlines はNHK News RSSトップニュースからヘッドラインを取得
func fetchNewsHeadlines(limit int) ([]string, error) {
	return fetchNewsHeadlinesFrom("https://www.nhk.or.jp/rss/news/cat0.xml", limit)
}

// fetchNewsHeadlinesFrom は指定URLのNHK RSSからヘッドラインを取得
func fetchNewsHeadlinesFrom(rssURL string, limit int) ([]string, error) {
	req, err := http.NewRequest("GET", rssURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RenCrow/1.0 (https://github.com/Nyukimin/picoclaw_multiLLM)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("nhk rss returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 簡易RSSパース（<title>タグ抽出）
	content := string(body)
	headlines := []string{}

	// <item>ブロック内の<title>を抽出
	inItem := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "<item>") {
			inItem = true
		} else if strings.HasPrefix(line, "</item>") {
			inItem = false
		} else if inItem && strings.HasPrefix(line, "<title>") {
			title := strings.TrimPrefix(line, "<title>")
			title = strings.TrimSuffix(title, "</title>")
			title = strings.TrimSpace(title)
			if title != "" && len(headlines) < limit {
				headlines = append(headlines, title)
			}
		}
	}

	return headlines, nil
}

// getDailyCache は現在のキャッシュを取得（スレッドセーフ）
func getDailyCache() *DailySeedCache {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return dailyCache
}
