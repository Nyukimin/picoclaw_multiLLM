package idlechat

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

const (
	storyChunkMaxRunes = 90
	storyChunkMinRunes = 28
	storyStageMaxRetries = 3
	storySourceMaxAttempts = 3
)

type StorySource struct {
	ID           string
	Title        string
	SourceLabel  string
	Kind         string
	Language     string
	PublicDomain bool
	Text         string
}

type StoryRewritePlan struct {
	SourceTitle  string
	RewriteStyle string
	StoryTitle   string
	Premise      string
	Setting      string
	Viewpoint    string
	Tone         string
	Hook         string
	EndingShape  string
	EndingFlavor string
	CoreMotifs   []string
	MotifMap     []string
}

type StoryBeat struct {
	ID    string
	Label string
}

type StorySkeleton struct {
	ID                  string
	SourceTitle         string
	CanonicalMotifs     []string
	RequiredBeats       []StoryBeat
	RoleConstraints     []string
	TabooOrRule         string
	RewardPunishment    string
	EmotionalAftertaste string
	RecognitionCues     []string
}

type StorySourceAnalysis struct {
	CoreMotifs          []string
	RoleMap             []string
	TabooOrRule         string
	RewardAndPunish     string
	EmotionalAftertaste string
	Skeleton            StorySkeleton
}

type StoryBeatPlan struct {
	Opening   string
	Deviation string
	Reversal  string
	Landing   string
}

type StoryAdaptationPlan struct {
	SkeletonID      string
	RewriteStyle    string
	BeatMappings    []string
	MotifMappings   []string
	RoleRemap       []string
	EndingFlavor    string
	RecognitionCues []string
}

// StoryPrep はソース選択からアダプテーションプラン構築まで（Step 2〜6）をまとめた構造体。
// LLM を使わない決定論的な準備フェーズの出力を保持する。
type StoryPrep struct {
	Source     StorySource
	Analysis   StorySourceAnalysis
	Plan       StoryRewritePlan
	BeatPlan   StoryBeatPlan
	Adaptation StoryAdaptationPlan
}

var storyRewriteStyles = []string{"role_shift", "view_shift", "value_shift", "inversion", "scale_shift"}
var storyGenres = []string{"ノワール", "ホラー", "コメディ", "ノーマル"}
var storyScales = []string{"極小", "極大"}
var storyRandIntn = rand.Intn

// storySourceText は廃止済み。テキストは data/story/<id>.json から読み込む。
func storySourceText(_ string) string {
	return ""
}

func (o *IdleChatOrchestrator) RunStorySession() {
	sessionID := fmt.Sprintf("story-%d", time.Now().Unix())
	startedAt := time.Now().In(jst)

	o.mu.Lock()
	o.chatActive = true
	o.sessionMode = "story"
	o.mu.Unlock()

	style := chooseStoryRewriteStyle(o.GetHistory(12))
	type storySuccess struct {
		source       StorySource
		plan         StoryRewritePlan
		draftText    string
		revisionNote string
		storyText    string
	}
	var result storySuccess
	var ok bool
	usedSources := make(map[string]struct{}, storySourceMaxAttempts)
	for sourceAttempt := 0; sourceAttempt < storySourceMaxAttempts; sourceAttempt++ {
		prep := o.prepareStory(style, usedSources)
		usedSources[prep.Source.Title] = struct{}{}
		draftText, err := o.retryStoryDraft(prep.Source, prep.Analysis, prep.Plan, prep.Adaptation, prep.BeatPlan)
		if err != nil {
			log.Printf("[Story] draft failed after retries (%s): %v", prep.Source.Title, err)
			continue
		}
		if storyDraftMatchesSourceRetelling(prep.Source, draftText) {
			result = storySuccess{
				source:       prep.Source,
				plan:         prep.Plan,
				draftText:    draftText,
				revisionNote: "第1稿が元話の骨格を十分に保っていたため、そのまま採用した。",
				storyText:    draftText,
			}
			ok = true
			break
		}
		storyText, revisionNote, err := o.retryStoryRevision(prep.Source, prep.Analysis, prep.Plan, prep.Adaptation, prep.BeatPlan, draftText)
		if err != nil {
			log.Printf("[Story] revision failed after retries (%s): %v", prep.Source.Title, err)
			candidate := strings.TrimSpace(draftText)
			if candidate == "" || !storyNarrativeLooksLikeProse(candidate) || !storySatisfiesSkeleton(candidate, prep.Analysis.Skeleton, prep.Adaptation) {
				candidate = repairStoryDraft(prep.Source, prep.Analysis, prep.Plan, prep.Adaptation, prep.BeatPlan, draftText)
			}
			if strings.TrimSpace(candidate) == "" || !storyNarrativeLooksLikeProse(candidate) {
				continue
			}
			storyText = candidate
			revisionNote = "改稿が不安定だったため、第1稿を整文して採用した。"
		}
		result = storySuccess{
			source:       prep.Source,
			plan:         prep.Plan,
			draftText:    draftText,
			revisionNote: revisionNote,
			storyText:    storyText,
		}
		ok = true
		break
	}
	if !ok {
		log.Printf("[Story] story generation failed for %d sources, falling back to normal chat", storySourceMaxAttempts)
		o.mu.Lock()
		o.sessionMode = "idle"
		o.currentTopic = ""
		o.mu.Unlock()
		o.runChatSession(StrategySingleGenre)
		o.mu.Lock()
		o.chatActive = false
		o.sessionMode = ""
		o.currentTopic = ""
		o.sessionContext = ""
		o.lastActivity = time.Now()
		o.mu.Unlock()
		return
	}

	currentTopic := fmt.Sprintf("元: %s / 改題: %s / 方式: %s", result.source.Title, result.plan.StoryTitle, result.plan.RewriteStyle)
	o.mu.Lock()
	o.currentTopic = currentTopic
	o.mu.Unlock()

	transcript := make([]string, 0, 12)
	intro := fmt.Sprintf("今夜の物語です。元になったのは『%s』。%s。", result.source.Title, result.plan.Hook)
	for _, chunk := range splitStoryNarration(intro, storyChunkMaxRunes) {
		o.emitStoryChunk(sessionID, chunk)
		transcript = append(transcript, "mio: "+chunk)
	}
	titleLine := fmt.Sprintf("改題は『%s』。", result.plan.StoryTitle)
	for _, chunk := range splitStoryNarration(titleLine, storyChunkMaxRunes) {
		o.emitStoryChunk(sessionID, chunk)
		transcript = append(transcript, "mio: "+chunk)
	}
	for _, chunk := range splitStoryNarration(result.storyText, storyChunkMaxRunes) {
		o.emitStoryChunk(sessionID, chunk)
		transcript = append(transcript, "mio: "+chunk)
	}
	closing := fmt.Sprintf("元の『%s』を下敷きにした、%sの物語でした。", result.source.Title, rewriteStyleLabel(result.plan.RewriteStyle))
	for _, chunk := range splitStoryNarration(closing, storyChunkMaxRunes) {
		o.emitStoryChunk(sessionID, chunk)
		transcript = append(transcript, "mio: "+chunk)
	}

	endedAt := time.Now().In(jst)
	o.saveStorySummary(sessionID, result.source, result.plan, result.draftText, result.revisionNote, result.storyText, transcript, startedAt, endedAt)

	o.mu.Lock()
	o.chatActive = false
	o.sessionMode = ""
	o.currentTopic = ""
	o.sessionContext = ""
	o.lastActivity = time.Now()
	o.mu.Unlock()
}

// prepareStory はソース選択からアダプテーションプラン構築まで（Step 2〜6）を実行する。
// すべて決定論的でLLMを使用しない。
func (o *IdleChatOrchestrator) prepareStory(style string, excluded map[string]struct{}) StoryPrep {
	source := o.selectStorySourceExcluding(excluded)
	analysis := analyzeStorySource(source)
	plan := buildStoryRewritePlan(source, analysis, style)
	beatPlan := groundedStoryBeatPlan(source, analysis, plan)
	adaptation := buildStoryAdaptationPlan(analysis.Skeleton, plan, beatPlan)
	return StoryPrep{
		Source:     source,
		Analysis:   analysis,
		Plan:       plan,
		BeatPlan:   beatPlan,
		Adaptation: adaptation,
	}
}

func (o *IdleChatOrchestrator) retryStoryDraft(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan) (string, error) {
	var lastErr error
	for attempt := 0; attempt < storyStageMaxRetries; attempt++ {
		draftText, err := o.generateStoryDraft(source, analysis, plan, adaptation, beatPlan)
		if err == nil {
			return draftText, nil
		}
		lastErr = err
		log.Printf("[Story] draft retry %d/%d failed (%s): %v", attempt+1, storyStageMaxRetries, source.Title, err)
	}
	if fallback := deterministicStoryDraft(source, analysis, plan, adaptation, beatPlan); fallback != "" {
		log.Printf("[Story] draft retries exhausted (%s), using deterministic fallback", source.Title)
		return fallback, nil
	}
	if fallback := safeStoryRetelling(source, plan); fallback != "" {
		log.Printf("[Story] draft retries exhausted (%s), using source retelling fallback", source.Title)
		return fallback, nil
	}
	return "", fmt.Errorf("draft retries exhausted: %w", lastErr)
}

func (o *IdleChatOrchestrator) retryStoryRevision(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan, draftText string) (string, string, error) {
	var lastErr error
	for attempt := 0; attempt < storyStageMaxRetries; attempt++ {
		storyText, revisionNote, err := o.reviseStoryNarrative(source, analysis, plan, adaptation, beatPlan, draftText)
		if err == nil {
			return storyText, revisionNote, nil
		}
		lastErr = err
		log.Printf("[Story] revision retry %d/%d failed (%s): %v", attempt+1, storyStageMaxRetries, source.Title, err)
	}
	return "", "", fmt.Errorf("revision retries exhausted: %w", lastErr)
}

func (o *IdleChatOrchestrator) emitStoryChunk(sessionID, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	msg := domaintransport.NewMessage("mio", "user", sessionID, "", content)
	msg.Type = domaintransport.MessageTypeIdleChat
	o.memory.RecordMessage(msg)
	ttsDone := o.emitTimelineEvent(TimelineEvent{
		Type:      "idlechat.message",
		From:      "mio",
		To:        "user",
		Content:   content,
		SessionID: sessionID,
	})
	o.waitForTTSDone(ttsDone)
	o.waitBreak(speakerBreak)
}

func (o *IdleChatOrchestrator) saveStorySummary(sessionID string, source StorySource, plan StoryRewritePlan, draftText, revisionNote, storyText string, transcript []string, startedAt, endedAt time.Time) {
	summary := fmt.Sprintf("元作品: %s\n改変方式: %s\n改題: %s\n導入: %s\n余韻: %s\nモチーフ: %s\n改稿: %s", source.Title, rewriteStyleLabel(plan.RewriteStyle), plan.StoryTitle, plan.Premise, plan.EndingFlavor, strings.Join(plan.MotifMap, " / "), revisionNote)
	record := SessionSummary{
		SessionID:         sessionID,
		Title:             fmt.Sprintf("%d月%d日の%sの物語まとめ", endedAt.Month(), endedAt.Day(), truncate(plan.StoryTitle, 24)),
		Topic:             fmt.Sprintf("元: %s / 改題: %s / 方式: %s", source.Title, plan.StoryTitle, plan.RewriteStyle),
		Strategy:          TopicStrategy(fmt.Sprintf("story:%s", plan.RewriteStyle)),
		Summary:           summary,
		SourceTitle:       source.Title,
		RewriteStyle:      plan.RewriteStyle,
		StoryTitle:        plan.StoryTitle,
		StoryText:         storyText,
		StoryDraftText:    draftText,
		StoryRevisionNote: revisionNote,
		StoryEndingFlavor: plan.EndingFlavor,
		StartedAt:         startedAt.Format(time.RFC3339),
		EndedAt:           endedAt.Format(time.RFC3339),
		Turns:             len(transcript),
		TopicProvider:     "shiro",
		SummaryProvider:   "shiro",
		Transcript:        append([]string(nil), transcript...),
	}
	o.mu.Lock()
	o.history = append(o.history, record)
	if len(o.history) > 200 {
		o.history = o.history[len(o.history)-200:]
	}
	store := o.topicStore
	o.mu.Unlock()
	if store != nil {
		if err := store.Append(record); err != nil {
			log.Printf("[Story] topic store append failed: %v", err)
		}
	}
	o.emitTimelineEvent(TimelineEvent{
		Type:      "idlechat.summary",
		From:      "shiro",
		To:        "story_summary",
		Content:   record.Title + "\n" + summary,
		SessionID: sessionID,
	})
}

func chooseStoryRewriteStyle(history []SessionSummary) string {
	candidates := append([]string(nil), storyRewriteStyles...)
	if len(history) > 0 {
		last := strings.TrimSpace(history[0].RewriteStyle)
		if last == "" {
			if s := strings.TrimSpace(string(history[0].Strategy)); strings.HasPrefix(s, "story:") {
				last = strings.TrimPrefix(s, "story:")
			}
		}
		if last != "" {
			filtered := candidates[:0]
			for _, c := range candidates {
				if c != last {
					filtered = append(filtered, c)
				}
			}
			if len(filtered) > 0 {
				candidates = filtered
			}
		}
	}
	return candidates[storyRandIntn(len(candidates))]
}

func (o *IdleChatOrchestrator) selectStorySource() StorySource {
	return o.selectStorySourceExcluding(nil)
}

func (o *IdleChatOrchestrator) selectStorySourceExcluding(excluded map[string]struct{}) StorySource {
	if forceID := os.Getenv("STORY_SOURCE"); forceID != "" {
		for _, src := range storyCorpus {
			if src.ID == forceID {
				return src
			}
		}
	}
	history := o.GetHistory(12)
	blocked := make(map[string]struct{}, 4)
	for _, item := range history {
		if strings.TrimSpace(item.SourceTitle) == "" {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(string(item.Strategy)), "story:") {
			continue
		}
		blocked[item.SourceTitle] = struct{}{}
		if len(blocked) >= 4 {
			break
		}
	}
	pool := make([]StorySource, 0, len(storyCorpus))
	for _, item := range storyCorpus {
		if excluded != nil {
			if _, skip := excluded[item.Title]; skip {
				continue
			}
		}
		if _, ok := blocked[item.Title]; ok {
			continue
		}
		pool = append(pool, item)
	}
	if len(pool) == 0 {
		for _, item := range storyCorpus {
			if excluded != nil {
				if _, skip := excluded[item.Title]; skip {
					continue
				}
			}
			pool = append(pool, item)
		}
	}
	if len(pool) == 0 {
		pool = append(pool, storyCorpus...)
	}
	return pool[storyRandIntn(len(pool))]
}

var storySettingsByGenre = map[string][]string{
	"ノワール": {"深夜の港の倉庫街", "錆びた橋の下の路地", "雨の降る古い商店街"},
	"ホラー":  {"霧の深い山道", "廃屋の離れ", "人気のない神社の境内"},
	"コメディ": {"にぎやかな市場の片隅", "田舎の郵便局前", "裏通りの八百屋"},
	"ノーマル": {"川沿いの小さな町", "山あいの集落", "海辺の漁村"},
}

func buildStoryRewritePlan(source StorySource, analysis StorySourceAnalysis, style string) StoryRewritePlan {
	genre := storyGenres[storyRandIntn(len(storyGenres))]
	norm := normalizeStoryRewriteStyle(style)
	axis := storyTransformationAxis(source, norm)
	var scale string
	if norm == "scale_shift" {
		scale = storyScales[storyRandIntn(len(storyScales))]
	}
	settings := storySettingsByGenre[genre]
	setting := settings[storyRandIntn(len(settings))]
	coreMotifs := append([]string(nil), analysis.Skeleton.CanonicalMotifs...)
	return StoryRewritePlan{
		SourceTitle:  source.Title,
		RewriteStyle: norm,
		StoryTitle:   planStoryTitle(source, norm, genre, scale),
		Premise:      axis + "を、" + genre + "の文脈で描く。",
		Setting:      setting,
		Viewpoint:    planStoryViewpoint(norm),
		Tone:         planStoryTone(genre),
		Hook:         axis,
		EndingShape:  planStoryEndingShape(norm),
		EndingFlavor: planStoryEndingFlavor(norm),
		CoreMotifs:   coreMotifs,
		MotifMap:     defaultStoryMotifMap(norm, coreMotifs),
	}
}

func planStoryTitle(source StorySource, style, genre, scale string) string {
	switch style {
	case "role_shift":
		return genre + "版・" + source.Title
	case "view_shift":
		return source.Title + "のそばにいた人"
	case "value_shift":
		return source.Title + "の裏返し"
	case "inversion":
		return "もし" + source.Title + "が逆だったら"
	case "scale_shift":
		if scale == "極小" {
			return "小さな" + source.Title
		}
		return "大きな" + source.Title
	default:
		return "今の" + source.Title
	}
}

func planStoryViewpoint(style string) string {
	switch style {
	case "role_shift":
		return "対立役の近接一人称"
	case "view_shift":
		return "傍観者の近接三人称"
	case "value_shift":
		return "語り手の俯瞰"
	case "inversion":
		return "因果を知る者の三人称"
	case "scale_shift":
		return "外側から見た三人称"
	default:
		return "語り手の三人称"
	}
}

func planStoryTone(genre string) string {
	switch genre {
	case "ノワール":
		return "乾いた緊張"
	case "ホラー":
		return "息をひそめた不穏"
	case "コメディ":
		return "軽快な滑稽"
	default:
		return "生活圏の手触りを残す静かな短編"
	}
}

func planStoryEndingShape(style string) string {
	switch style {
	case "role_shift":
		return "力の構造が最後に反転する"
	case "view_shift":
		return "視点の差が静かに明かされる"
	case "value_shift":
		return "価値観の転倒が露呈する"
	case "inversion":
		return "因果が逆に着地する"
	case "scale_shift":
		return "スケールが変わることで別の真実が見える"
	default:
		return "静かな余韻で終わる"
	}
}

func planStoryEndingFlavor(style string) string {
	switch style {
	case "role_shift":
		return "構造の露呈"
	case "view_shift":
		return "立場の差"
	case "value_shift":
		return "喪失"
	case "inversion":
		return "皮肉"
	case "scale_shift":
		return "眩暈"
	default:
		return "余韻"
	}
}

func storyTransformationAxis(source StorySource, style string) string {
	spec, ok := storySpecForSource(source)
	if ok {
		if axis, found := spec.Twists[normalizeStoryRewriteStyle(style)]; found && strings.TrimSpace(axis) != "" {
			return axis
		}
	}
	switch normalizeStoryRewriteStyle(style) {
	case "view_shift":
		return fmt.Sprintf("『%s』を、主役のすぐ近くにいた人物の立場からの見え方の差", source.Title)
	case "value_shift":
		return fmt.Sprintf("『%s』の報いや救いが、別の価値観から見たときに逆転する構造", source.Title)
	case "inversion":
		return fmt.Sprintf("『%s』の因果や報いが逆だったら何が残るか", source.Title)
	case "scale_shift":
		return fmt.Sprintf("『%s』の力や出来事の及ぶ範囲が変わったとき、何が見えるか", source.Title)
	default:
		return fmt.Sprintf("『%s』の役割と従属の非対称が生む構造", source.Title)
	}
}

func (o *IdleChatOrchestrator) generateStoryDraft(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan) (string, error) {
	return o.generateStoryDraftByBeats(source, analysis, plan, adaptation, beatPlan)
}

func (o *IdleChatOrchestrator) generateStoryDraftByBeats(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan) (string, error) {
	openingSeed := storyOpeningSeed(source, plan)
	type beatSection struct {
		label   string
		content string
	}
	sections := []beatSection{
		{label: "導入", content: beatPlan.Opening},
		{label: "逸脱", content: beatPlan.Deviation},
		{label: "反転", content: beatPlan.Reversal},
		{label: "着地", content: beatPlan.Landing},
	}
	paragraphs := make([]string, 0, len(sections))
	for i, section := range sections {
		context := ""
		if len(paragraphs) > 0 {
			context = paragraphs[len(paragraphs)-1]
		}
		messages := []llm.Message{
			{Role: "system", Content: "あなたは朗読短編作家です。指定された場面を、登場人物の行動または対話を中心にした短い段落で書いてください。情景描写だけの文から始めないでください。"},
			{Role: "user", Content: fmt.Sprintf(`元作品: %s
元話の要約本文:
%s
改題: %s
改変方式: %s
舞台: %s
視点: %s
今回書く場面: %s
この場面の役割: %s
前の段落:
%s
必須モチーフ: %s
認識手がかり: %s

要件:
- この場面だけを2〜4文で書く
- 元話の要約本文の文章をそのままコピーしない。要約本文を理解したうえで、別の言葉で書く
- 会社名、開発計画、ランキング制度、観光客、SNS、スマホ、モデル、広告の話にしない
- 比喩は多用しない
- 新しい固有名詞を増やさない
- 人物の行動か対話で場面を進める
- 元話の骨格が分かる出来事を書く
- 教訓のまとめ、抽象的な総括、象徴の説明を書かない
- 前の段落で書いた出来事や文を言い直さない
- %s
- 出力は本文だけ`, source.Title, source.Text, plan.StoryTitle, plan.RewriteStyle, plan.Setting, plan.Viewpoint, section.label, section.content, context, strings.Join(plan.CoreMotifs, " / "), strings.Join(adaptation.RecognitionCues, " / "), beatInstruction(i, openingSeed))},
		}
		resp, err := o.providerForSpeaker("shiro").Generate(o.ctx, llm.GenerateRequest{
			Messages:    messages,
			MaxTokens:   300,
			Temperature: 0.3,
		})
		if err != nil {
			return "", fmt.Errorf("beat draft failed: %w", err)
		}
		paragraph := normalizeStoryNarrative(resp.Content)
		if paragraph == "" || storyHasOutlineLanguage(paragraph) || storyHasOverblownSetting(paragraph) {
			log.Printf("[Story] beat %d rejected (%s) outline/overblown: %s", i, source.Title, storyLogSnippet(paragraph))
			return "", fmt.Errorf("empty story draft")
		}
		if storyParagraphIsVerbatimCopy(source, paragraph) {
			log.Printf("[Story] beat %d rejected (%s) verbatim copy: %s", i, source.Title, storyLogSnippet(paragraph))
			return "", fmt.Errorf("empty story draft")
		}
		paragraphs = append(paragraphs, paragraph)
	}
	story := strings.TrimSpace(strings.Join(paragraphs, "\n\n"))
	if !storyNarrativeLooksLikeProse(story) || !storySatisfiesSkeleton(story, analysis.Skeleton, adaptation) {
		if fallback := deterministicStoryDraft(source, analysis, plan, adaptation, beatPlan); fallback != "" {
			return fallback, nil
		}
		return "", fmt.Errorf("empty story draft")
	}
	return story, nil
}

func storyParagraphLooksAtmospheric(paragraph string) bool {
	sentences := splitStorySentences(paragraph)
	if len(sentences) >= 2 && storyStartsWithAtmosphere(strings.TrimSpace(sentences[1])) {
		return true
	}
	if strings.Count(paragraph, "まるで") > 1 {
		return true
	}
	return false
}

func storyParagraphRepeatsContext(context, paragraph string) bool {
	context = strings.TrimSpace(context)
	paragraph = strings.TrimSpace(paragraph)
	if context == "" || paragraph == "" {
		return false
	}
	contextSentences := splitStorySentences(context)
	paragraphSentences := splitStorySentences(paragraph)
	seen := make(map[string]struct{}, len(contextSentences))
	for _, sentence := range contextSentences {
		seen[storySignature(sentence)] = struct{}{}
	}
	repeats := 0
	for _, sentence := range paragraphSentences {
		if _, ok := seen[storySignature(sentence)]; ok {
			repeats++
		}
	}
	return repeats > 0
}

func beatInstruction(index int, openingSeed string) string {
	if index == 0 {
		return "第1文は必ずこの文をそのまま使う: " + openingSeed
	}
	return "前の段落の続きとして、登場人物の行動か対話で書き始める（情景描写だけの文から始めない）"
}

func (o *IdleChatOrchestrator) reviseStoryNarrative(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan, draftText string) (string, string, error) {
	openingSeed := storyOpeningSeed(source, plan)
	messages := []llm.Message{
		{Role: "system", Content: "あなたは朗読短編の編集者です。第1稿の面白さを残しつつ、因果、余韻、読後感を整えた第2稿に直してください。"},
		{Role: "user", Content: fmt.Sprintf(`次の第1稿を改稿して、第2稿を作ってください。

元作品: %s
元話の要約本文:
%s
改題: %s
改変方式: %s
余韻: %s
必須モチーフ: %s
必須イベント順: %s
認識手がかり: %s
禁忌/約束: %s
報酬と罰: %s
ビート:
- 導入: %s
- 逸脱: %s
- 反転: %s
- 着地: %s

第1稿:
%s

改稿方針:
- 元話の要約本文の文章をそのままコピーしない。要約本文を理解したうえで、別の言葉で書く
- 第1文は次の文を一字一句変えずに保つ: %s
- 第1稿の良い飛躍は消しすぎない
- 因果が飛ぶ箇所だけを補う
- 結末で %s が残るようにする
- ひねりは「%s」という一点に絞り、それ以外は元話の骨格へ戻しすぎるくらいでよい
- 必須モチーフの位置を聞き取りやすくする
- 必須イベント順と認識手がかりを落とさない
- 導入 -> 逸脱 -> 反転 -> 余韻 が感じられるように整える
- 説明臭くしない
- 元話の骨格を、事件と場面として再演する
- 4〜8段落相当の短編として落ち着かせる
- 各段落で誰かの行動、対話、決断のどれかを必ず進める
- 必須イベントごとに少なくとも1つ、目に見える場面が残るようにする
- 「元の『%s』で禁じられていた」などの設計説明文を本文に入れない
- 「最初の違和感として立ち上がる」「ここで〜が意外な意味に変わる」「最後に残るのは〜だ」を本文に入れない
- 新しい固有名詞をむやみに増やさない
- 舞台は現代の地続きの世界に固定する
- 年号を出すなら現在に近いものだけにし、未来年代や時代跳躍を入れない
- 巨大企業、AI支配、世界規模の陰謀へ話を膨らませず、生活圏の事件として整える
- SNS、観光客、スマホ、会員制施設、権限トークンのような手癖の現代化を避ける
- 会社名、開発計画、ランキング制度、不動産会社、プロジェクト名を新しく作らない
- 比喩を減らし、冒頭2文のどちらかで必ず人物の行動か対話を始める
- 第1文は必ず人物の行動か対話で始める
- 幼少期の思い出、象徴的な回想、説明のための脇道を新しく足さない
- 教訓の言い直し、象徴の説明、抽象的な総括で終わらせない
- 一人称か三人称の自然な物語文にし、二人称で説教や勧誘をしない
- 出力は次の形式だけ
REVISION_NOTE:
STORY:`, source.Title, source.Text, plan.StoryTitle, plan.RewriteStyle, plan.EndingFlavor, strings.Join(plan.CoreMotifs, " / "), strings.Join(storyBeatLabels(analysis.Skeleton.RequiredBeats), " -> "), strings.Join(analysis.Skeleton.RecognitionCues, " / "), analysis.TabooOrRule, analysis.RewardAndPunish, beatPlan.Opening, beatPlan.Deviation, beatPlan.Reversal, beatPlan.Landing, draftText, openingSeed, plan.EndingFlavor, plan.Premise, source.Title)},
	}
	resp, err := o.providerForSpeaker("shiro").Generate(o.ctx, llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   1400,
		Temperature: 0.25,
	})
	if err != nil {
		return "", "", err
	}
	revisionNote, story := parseStoryRevision(resp.Content)
	story = normalizeStoryNarrative(story)
	if story == "" {
		log.Printf("[Story] revision rejected (%s): empty response", source.Title)
		return "", "", fmt.Errorf("invalid revised story")
	}
	if !storyNarrativeLooksSettled(story, draftText, plan, beatPlan) {
		log.Printf("[Story] revision rejected (%s): settling check failed: %s", source.Title, storyLogSnippet(story))
		return "", "", fmt.Errorf("invalid revised story")
	}
	if !storyNarrativeLooksLikeProse(story) {
		log.Printf("[Story] revision rejected (%s): prose check failed: %s", source.Title, storyLogSnippet(story))
		return "", "", fmt.Errorf("invalid revised story")
	}
	if !storySatisfiesSkeleton(story, analysis.Skeleton, adaptation) {
		log.Printf("[Story] revision rejected (%s): skeleton check failed: %s", source.Title, storyLogSnippet(story))
		return "", "", fmt.Errorf("invalid revised story")
	}
	if revisionNote == "" {
		revisionNote = fallbackStoryRevisionNote(plan, beatPlan)
	}
	return story, revisionNote, nil
}

func fallbackStoryBeatPlan(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan) StoryBeatPlan {
	return StoryBeatPlan{
		Opening:   fmt.Sprintf("%s。%sが最初の違和感として立ち上がる。", plan.Hook, analysis.TabooOrRule),
		Deviation: fmt.Sprintf("%s。ここで%sが意外な意味に変わる。", plan.Premise, firstStoryMotifLabel(plan.MotifMap)),
		Reversal:  fmt.Sprintf("%s。その飛躍が、%sによって因果として結び直される。", plan.EndingShape, analysis.RewardAndPunish),
		Landing:   fmt.Sprintf("最後に残るのは%sだ。", plan.EndingFlavor),
	}
}

func defaultGroundedStorySetting(source StorySource) string {
	switch source.ID {
	case "momotaro":
		return "川沿いの町とその外れ"
	case "urashima":
		return "海辺の町と港の近く"
	case "kaguya":
		return "町外れの竹林と古い家"
	case "issun":
		return "下町の長屋と小さな店"
	case "hanasaka":
		return "庭のある古い家と近所の道"
	case "shitakiri":
		return "町外れの家と小さな鳥小屋"
	case "kasajizo":
		return "雪の積もる町はずれの道"
	case "kintaro":
		return "山あいの集落"
	case "sarukani":
		return "畑の残る町はずれ"
	case "tsuru":
		return "雪の降る田舎町の小さな家"
	case "kobutori":
		return "山裾の家と夜の小屋"
	case "bunbuku":
		return "古道具屋のある商店街"
	case "redriding":
		return "町外れの林道と祖母の家"
	case "cinderella":
		return "町なかの家と公民館の祝賀会"
	case "snowwhite":
		return "山あいの町と共同住宅"
	case "hansel":
		return "町外れの林道と菓子店"
	case "bremen":
		return "街道沿いの町と古い家"
	case "puss":
		return "河原の町と古い屋敷"
	case "threepigs":
		return "郊外の住宅地"
	case "beauty":
		return "町はずれの古い洋館"
	case "aladdin":
		return "市場通りと古い倉庫"
	case "ali40":
		return "乾いた商店街と外れの倉庫"
	case "hadaka":
		return "祭りを控えた町役場"
	case "match":
		return "年の瀬の商店街"
	case "littlemermaid":
		return "海辺の町と防波堤"
	case "sleepingbeauty":
		return "古い屋敷と長く閉ざされた部屋"
	case "frogprince":
		return "池のある古い家"
	default:
		return "現代の地方都市とその周辺"
	}
}

func groundedStoryBeatPlan(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan) StoryBeatPlan {
	labels := storyBeatLabels(analysis.Skeleton.RequiredBeats)
	opening := firstOrFallback(labels, 0, "導入")
	deviation := firstOrFallback(labels, 1, "逸脱")
	reversal := firstOrFallback(labels, 2, "反転")
	landing := firstOrFallback(labels, 3, "着地")

	return StoryBeatPlan{
		Opening:   fmt.Sprintf("%sを、%sで始める。", opening, plan.Setting),
		Deviation: fmt.Sprintf("%sが起こり、%sというひねりが見える。", deviation, plan.RewriteStyleLabel()),
		Reversal:  fmt.Sprintf("%sによって、%sという元話の骨格をはっきり見せる。", reversal, joinSome(plan.CoreMotifs, 2)),
		Landing:   fmt.Sprintf("%sで終え、最後に%sが残る。", landing, plan.EndingFlavor),
	}
}

func (p StoryRewritePlan) RewriteStyleLabel() string {
	return rewriteStyleLabel(p.RewriteStyle)
}

func firstOrFallback(items []string, idx int, fallback string) string {
	if idx >= 0 && idx < len(items) && strings.TrimSpace(items[idx]) != "" {
		return items[idx]
	}
	return fallback
}

func joinSome(items []string, max int) string {
	if len(items) == 0 {
		return "元話の手がかり"
	}
	if len(items) > max {
		items = items[:max]
	}
	return strings.Join(items, "と")
}

func storyOpeningSeed(source StorySource, plan StoryRewritePlan) string {
	switch source.ID {
	case "momotaro":
		return "その夜、桃太郎は川沿いの倉庫で小さな包みを仲間に配った。"
	case "urashima":
		return "浦島は海辺の道で、子どもに囲まれていた小さな亀を助けた。"
	case "kaguya":
		return "翁は町外れの竹林で、ひときわ光る竹を見つけて立ち止まった。"
	case "issun":
		return "一寸ほどの背丈しかない若者は、椀のように小さな舟を押して町へ向かった。"
	case "hanasaka":
		return "おじいさんは庭の土を掘る犬の前にしゃがみこみ、何が出るのか息をひそめた。"
	case "shitakiri":
		return "おじいさんは傷ついた雀を手のひらにのせ、誰にも見つからないよう家へ連れ帰った。"
	case "kasajizo":
		return "おじいさんは売れ残った笠を背負い、雪の道で立ち止まった。"
	case "kintaro":
		return "金太郎は山道の真ん中で丸太を持ち上げ、動物たちを笑わせた。"
	case "sarukani":
		return "蟹は握り飯を抱えたまま、猿に差し出された柿の種を見つめた。"
	case "tsuru":
		return "男は雪の畑で羽を傷めた鶴を見つけ、ためらいながら縄をほどいた。"
	case "kobutori":
		return "こぶを気にする老人は、雨をよけて古い小屋へ駆け込んだ。"
	case "bunbuku":
		return "古道具屋の主人は、寺から預かった茶釜を店の奥へ運びこんだ。"
	case "redriding":
		return "赤い頭巾の娘は、包みを抱えて祖母の家へ向かう林道に入った。"
	case "cinderella":
		return "娘は灰だらけの手を洗い、公民館の明かりを遠くから見上げた。"
	case "snowwhite":
		return "娘は追い立てられるように町を離れ、山あいの家の戸をたたいた。"
	case "hansel":
		return "兄妹は細い道へ入り、帰り道の目印に白い小石を落としていった。"
	case "bremen":
		return "年老いたロバは荷を降ろされる前に家を出て、街道を歩き始めた。"
	case "puss":
		return "猫は主人の前に立ち、まず一足の長靴を買ってくれと頼んだ。"
	case "threepigs":
		return "三人のきょうだいは町はずれの空き地に立ち、それぞれ別の家を建て始めた。"
	case "beauty":
		return "娘は父の代わりに、町はずれの古い洋館の門をくぐった。"
	case "aladdin":
		return "若者は市場通りの裏で声をかけてきた男に連れられ、古い倉庫へ入った。"
	case "ali40":
		return "木こりの男は荷を背負って歩く途中で、岩陰に人影が集まるのを見つけた。"
	case "hadaka":
		return "仕立て屋たちは町役場へ呼ばれ、誰にも見えない布の話を始めた。"
	case "match":
		return "少女は年の瀬の商店街で足を止め、売れ残った細い箱を抱え直した。"
	case "littlemermaid":
		return "海辺の娘は防波堤の向こうを見つめ、沖から戻る船を待っていた。"
	case "sleepingbeauty":
		return "若者は長く閉ざされていた部屋の扉を押し開け、中の静けさに息をのんだ。"
	case "frogprince":
		return "娘は池のほとりで金のまりを落とし、水面をのぞきこんだ。"
	default:
		return fmt.Sprintf("%sは%sで足を止め、これから起こる出来事の気配を見つけた。", source.Title, defaultGroundedStorySetting(source))
	}
}

func fallbackStoryNarrative(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, beatPlan StoryBeatPlan) string {
	return deterministicStoryDraft(source, analysis, plan, buildStoryAdaptationPlan(analysis.Skeleton, plan, beatPlan), beatPlan)
}

func repairStoryDraft(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan, draftText string) string {
	repaired := stripStoryMetaLeak(draftText)
	repaired = strings.ReplaceAll(repaired, "。。", "。")
	repaired = strings.TrimSpace(repaired)
	if repaired == "" {
		if fallback := deterministicStoryDraft(source, analysis, plan, adaptation, beatPlan); fallback != "" {
			return stripStoryMetaLeak(fallback)
		}
		return stripStoryMetaLeak(safeStoryRetelling(source, plan))
	}
	if !strings.Contains(repaired, firstToken(plan.EndingFlavor)) && strings.TrimSpace(beatPlan.Landing) != "" {
		if !strings.HasSuffix(repaired, "。") {
			repaired += "。"
		}
		repaired = strings.TrimSpace(repaired + " " + beatPlan.Landing)
	}
	if !storySatisfiesSkeleton(repaired, analysis.Skeleton, adaptation) {
		if fallback := deterministicStoryDraft(source, analysis, plan, adaptation, beatPlan); fallback != "" {
			return stripStoryMetaLeak(fallback)
		}
		return stripStoryMetaLeak(safeStoryRetelling(source, plan))
	}
	return repaired
}

func storyDraftMatchesSourceRetelling(source StorySource, draftText string) bool {
	// A verbatim copy is not a "good retelling" — it's just copying.
	if storyParagraphIsVerbatimCopy(source, draftText) {
		return false
	}
	draftText = normalizeStoryNarrative(draftText)
	sourceText := normalizeStoryNarrative(source.Text)
	if draftText == "" || sourceText == "" {
		return false
	}
	sentences := splitStorySentences(sourceText)
	hits := 0
	for i := 0; i < len(sentences) && i < 3; i++ {
		if strings.Contains(draftText, sentences[i]) {
			hits++
		}
	}
	return hits >= 2
}

// storyParagraphIsVerbatimCopy returns true if paragraph contains any complete
// sentence (≥15 runes) from source.Text verbatim. Used to detect when the model
// copies the source synopsis instead of writing original prose.
func storyParagraphIsVerbatimCopy(source StorySource, paragraph string) bool {
	sourceText := normalizeStoryNarrative(source.Text)
	if sourceText == "" || paragraph == "" {
		return false
	}
	sentences := splitStorySentences(sourceText)
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if utf8.RuneCountInString(sentence) < 15 {
			continue
		}
		if strings.Contains(paragraph, sentence) {
			return true
		}
	}
	return false
}

func deterministicStoryDraft(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan) string {
	opening := storyOpeningSeed(source, plan)
	motif0 := storyMappedMotif(plan.MotifMap, 0, firstStoryMotifLabel(plan.MotifMap))
	motif1 := storyMappedMotif(plan.MotifMap, 1, motif0)
	motif2 := storyMappedMotif(plan.MotifMap, 2, motif1)
	baseSentences := splitStorySentences(normalizeStoryNarrative(source.Text))
	if len(baseSentences) == 0 {
		baseSentences = []string{opening}
	}
	paragraphs := []string{
		opening + " " + fmt.Sprintf("その場では、%sと%sの名がひそやかに広まり始めていた。", motif0, motif1),
		storyDeterministicParagraph(baseSentences, 1),
		storyDeterministicParagraph(baseSentences, 2),
		storyDeterministicParagraph(baseSentences, 3) + " " + fmt.Sprintf("あとに残ったのは、%sに近い静けさだった。", plan.EndingFlavor),
	}
	story := normalizeStoryNarrative(strings.Join(paragraphs, "\n\n"))
	if !storyNarrativeLooksLikeProse(story) {
		return ""
	}
	if !storySatisfiesSkeleton(story, analysis.Skeleton, adaptation) {
		story = normalizeStoryNarrative(story + "\n\n" + fmt.Sprintf("%s、%s、%sは順番どおりにそこへ現れた。", motif0, motif1, motif2))
		if !storySatisfiesSkeleton(story, analysis.Skeleton, adaptation) {
			return ""
		}
	}
	return story
}

func storyDeterministicParagraph(sentences []string, idx int) string {
	if idx < len(sentences) {
		return strings.TrimSpace(sentences[idx])
	}
	return strings.TrimSpace(sentences[len(sentences)-1])
}

func safeStoryRetelling(source StorySource, plan StoryRewritePlan) string {
	opening := storyOpeningSeed(source, plan)
	body := normalizeStoryNarrative(source.Text)
	if body == "" {
		return ""
	}
	return normalizeStoryNarrative(opening + "\n\n" + body + "\n\n" + fmt.Sprintf("そのあとに残ったのは、%sに近い静けさだった。", plan.EndingFlavor))
}

func storyMappedMotif(motifMap []string, idx int, fallback string) string {
	if idx >= 0 && idx < len(motifMap) {
		if token := firstToken(motifMap[idx]); token != "" {
			return token
		}
	}
	return fallback
}

func rewriteStyleLabel(style string) string {
	switch normalizeStoryRewriteStyle(style) {
	case "role_shift":
		return "役割転換"
	case "view_shift":
		return "視点変更"
	case "value_shift":
		return "価値反転"
	case "inversion":
		return "因果反転"
	case "scale_shift":
		return "規模変換"
	default:
		return style
	}
}

func normalizeStoryRewriteStyle(style string) string {
	switch strings.TrimSpace(style) {
	case "role_shift", "what_if", "if", "役割転換", "もしも転換":
		return "role_shift"
	case "view_shift", "視点変更":
		return "view_shift"
	case "value_shift", "価値反転":
		return "value_shift"
	default:
		return strings.TrimSpace(style)
	}
}

func normalizeStoryEndingFlavor(flavor string) string {
	switch strings.TrimSpace(flavor) {
	case "報い", "救い", "喪失", "皮肉":
		return strings.TrimSpace(flavor)
	default:
		return "余韻"
	}
}

func fallbackStorySetting(style string) string {
	switch strings.TrimSpace(style) {
	case "view_shift":
		return "同じ事件を横から見ている地域コミュニティ"
	case "value_shift":
		return "善意と損得が衝突する商店街"
	default:
		return "深夜の物流と生活が交差する町"
	}
}

func fallbackStoryViewpoint(style string) string {
	switch strings.TrimSpace(style) {
	case "view_shift":
		return "元の脇役の一人称"
	case "value_shift":
		return "正しさを信じていた当事者の一人称"
	default:
		return "役目を押しつけられた当事者"
	}
}

func analyzeStorySource(source StorySource) StorySourceAnalysis {
	skeleton := storySkeleton(source)
	return StorySourceAnalysis{
		CoreMotifs:          skeleton.CanonicalMotifs,
		RoleMap:             skeleton.RoleConstraints,
		TabooOrRule:         skeleton.TabooOrRule,
		RewardAndPunish:     skeleton.RewardPunishment,
		EmotionalAftertaste: skeleton.EmotionalAftertaste,
		Skeleton:            skeleton,
	}
}

func buildStoryAdaptationPlan(skeleton StorySkeleton, plan StoryRewritePlan, beatPlan StoryBeatPlan) StoryAdaptationPlan {
	beatMappings := []string{
		fmt.Sprintf("%s=>%s", labelOrBeatID(skeleton.RequiredBeats, 0), beatPlan.Opening),
		fmt.Sprintf("%s=>%s", labelOrBeatID(skeleton.RequiredBeats, 1), beatPlan.Deviation),
		fmt.Sprintf("%s=>%s", labelOrBeatID(skeleton.RequiredBeats, 2), beatPlan.Reversal),
		fmt.Sprintf("%s=>%s", labelOrBeatID(skeleton.RequiredBeats, 3), beatPlan.Landing),
	}
	return StoryAdaptationPlan{
		SkeletonID:      skeleton.ID,
		RewriteStyle:    plan.RewriteStyle,
		BeatMappings:    beatMappings,
		MotifMappings:   append([]string(nil), plan.MotifMap...),
		RoleRemap:       append([]string(nil), skeleton.RoleConstraints...),
		EndingFlavor:    plan.EndingFlavor,
		RecognitionCues: append([]string(nil), skeleton.RecognitionCues...),
	}
}

func labelOrBeatID(beats []StoryBeat, idx int) string {
	if idx >= 0 && idx < len(beats) && strings.TrimSpace(beats[idx].Label) != "" {
		return beats[idx].Label
	}
	switch idx {
	case 0:
		return "導入"
	case 1:
		return "逸脱"
	case 2:
		return "反転"
	default:
		return "着地"
	}
}

func storyBeatLabels(beats []StoryBeat) []string {
	out := make([]string, 0, len(beats))
	for _, beat := range beats {
		if strings.TrimSpace(beat.Label) == "" {
			continue
		}
		out = append(out, beat.Label)
	}
	return out
}

func parseStoryRevision(raw string) (string, string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	note := ""
	story := raw
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 {
		return "", strings.TrimSpace(raw)
	}
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "REVISION_NOTE:") {
		note = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[0]), "REVISION_NOTE:"))
		story = strings.Join(lines[1:], "\n")
		if strings.HasPrefix(strings.TrimSpace(story), "STORY:") {
			story = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(story), "STORY:"))
		}
	}
	return note, strings.TrimSpace(story)
}

func normalizeStoryNarrative(story string) string {
	story = strings.ReplaceAll(story, "\r\n", "\n")
	story = strings.ReplaceAll(story, "\r", "\n")
	story = strings.ReplaceAll(story, "REVISION_NOTE:", "")
	story = strings.ReplaceAll(story, "STORY:", "")
	lines := strings.Split(story, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(kept) > 0 && kept[len(kept)-1] != "" {
				kept = append(kept, "")
			}
			continue
		}
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "*   **") || strings.HasPrefix(line, "- ") {
			break
		}
		if strings.HasPrefix(line, "わかりました。") || strings.HasPrefix(line, "以下に、") || strings.HasPrefix(line, "いかがでしょうか") {
			continue
		}
		if strings.HasPrefix(line, "（余韻）") || strings.HasPrefix(line, "(余韻)") || strings.HasPrefix(line, "余韻:") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	out = stripStoryMetaSentences(out)
	out = dedupeStoryParagraphs(out)
	out = dedupeStorySentences(out)
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return out
}

func fallbackStoryRevisionNote(plan StoryRewritePlan, beatPlan StoryBeatPlan) string {
	return fmt.Sprintf("逸脱を残しつつ、%s から %s へ因果が通るよう整えた。", truncate(beatPlan.Deviation, 18), plan.EndingFlavor)
}

func storyNarrativeLooksSettled(story, draft string, plan StoryRewritePlan, beatPlan StoryBeatPlan) bool {
	if storyHasMetaLeak(story) {
		return false
	}
	// Story must end with a proper sentence terminator, not trail off mid-sentence.
	trimmed := strings.TrimSpace(story)
	if trimmed == "" {
		return false
	}
	last := []rune(trimmed)
	lastChar := last[len(last)-1]
	if lastChar != '。' && lastChar != '！' && lastChar != '？' && lastChar != '」' && lastChar != '』' {
		return false
	}
	return true
}

func storyNarrativeLooksLikeProse(story string) bool {
	story = strings.TrimSpace(story)
	if utf8.RuneCountInString(story) < 160 {
		return false
	}
	if !storyOpensWithActionOrDialogue(story) {
		return false
	}
	if storyHasOutlineLanguage(story) {
		return false
	}
	if storyHasOverblownSetting(story) {
		return false
	}
	if storyHasDistractingDigression(story) {
		return false
	}
	sentenceCount := strings.Count(story, "。") + strings.Count(story, "！") + strings.Count(story, "？")
	if sentenceCount < 3 {
		return false
	}
	actionHits := 0
	for _, token := range []string{"言っ", "聞い", "見", "向か", "渡し", "開け", "閉め", "走", "歩", "置", "差し出", "隠", "助け", "届", "待っ", "座っ"} {
		if strings.Contains(story, token) {
			actionHits++
		}
	}
	return actionHits >= 1
}

func storyHasOverblownSetting(story string) bool {
	patterns := []string{
		"AI開発部", "量子コンピューター", "未来テック", "2040", "2041", "2042", "2043", "2044", "2045",
		"巨大企業", "世界最大手", "社会の神経回路", "ご招待ありがとうございます", "会員限定リゾート",
		"大規模言語モデル", "量子", "システム部門の地下室", "世界規模", "未来都市", "近未来",
		"SNS", "いいね", "観光客", "スマホ", "会員制", "保養施設", "トークン", "権限", "高層", "地下保守",
		"不動産会社", "株式会社", "開発計画", "プロジェクト", "ランキング", "評価システム", "商業施設", "アプリ",
	}
	for _, pattern := range patterns {
		if strings.Contains(story, pattern) {
			return true
		}
	}
	if strings.Count(story, "まるで") >= 5 {
		return true
	}
	head := story
	if utf8.RuneCountInString(head) > 60 {
		head = string([]rune(head)[:60])
	}
	if strings.Contains(head, "あなたは") {
		return true
	}
	return false
}

func storyHasDistractingDigression(story string) bool {
	patterns := []string{
		"幼い頃",
		"子どもの頃",
		"思い出した",
		"思い出す",
		"記憶のよう",
		"象徴している",
		"物語の一部だった",
		"結局のところ",
		"最も恐ろしい",
		"悪だった",
		"唯一無二の宝",
	}
	for _, pattern := range patterns {
		if strings.Contains(story, pattern) {
			return true
		}
	}
	return false
}

func storyOpensWithActionOrDialogue(story string) bool {
	firstSentence := firstStorySentences(story, 1)
	if storyStartsWithAtmosphere(firstSentence) {
		return false
	}
	head := firstStorySentences(story, 2)
	if head == "" {
		return false
	}
	if strings.Count(head, "まるで") > 1 {
		return false
	}
	if strings.Contains(head, "「") || strings.Contains(head, "『") || strings.Contains(head, "“") {
		return true
	}
	for _, token := range []string{
		"行っ", "来", "入", "出", "渡", "開", "閉", "運", "置", "持", "返", "逃", "走", "見",
		"拾", "渡し", "呼", "言", "頼", "座", "立", "向か", "届け", "売", "買", "隠", "探",
		"助け", "差し出", "断", "受け取",
		"駆け", "配", "押", "飛", "叩", "打", "食べ", "飲", "投", "引",
	} {
		if strings.Contains(head, token) {
			return true
		}
	}
	return false
}

func storyStartsWithAtmosphere(sentence string) bool {
	sentence = strings.TrimSpace(sentence)
	for _, prefix := range []string{"雨", "風", "雪", "夜", "朝", "夕", "月", "光", "薄明かり", "霧", "静けさ"} {
		if strings.HasPrefix(sentence, prefix) {
			return true
		}
	}
	return false
}

func firstStorySentences(story string, limit int) string {
	parts := splitStorySentences(story)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > limit {
		parts = parts[:limit]
	}
	return strings.Join(parts, "")
}

func splitStorySentences(story string) []string {
	var (
		sentences []string
		buf       strings.Builder
	)
	for _, r := range story {
		buf.WriteRune(r)
		switch r {
		case '。', '！', '？', '\n':
			part := strings.TrimSpace(buf.String())
			if part != "" {
				sentences = append(sentences, part)
			}
			buf.Reset()
		}
	}
	if tail := strings.TrimSpace(buf.String()); tail != "" {
		sentences = append(sentences, tail)
	}
	return sentences
}

func storyHasOutlineLanguage(story string) bool {
	patterns := []string{
		"どうひねったか",
		"よく分からないけど",
		"物語の始まりを予感",
		"最初の違和感として立ち上がる",
		"意外な意味に変わる",
		"因果として結び直される",
		"最後に残るのは",
		"という感触だった",
		"導入:",
		"逸脱:",
		"反転:",
		"着地:",
		"要件:",
		"改稿方針:",
		"REVISION_NOTE:",
		"STORY:",
	}
	for _, pattern := range patterns {
		if strings.Contains(story, pattern) {
			return true
		}
	}
	return false
}

func stripStoryMetaSentences(story string) string {
	sentences := splitStorySentences(story)
	if len(sentences) == 0 {
		return strings.TrimSpace(story)
	}
	filtered := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		if strings.Contains(sentence, "どうひねったか") ||
			strings.Contains(sentence, "よく分からないけど") ||
			strings.Contains(sentence, "物語の始まりを予感") {
			continue
		}
		filtered = append(filtered, sentence)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func dedupeStoryParagraphs(story string) string {
	parts := strings.Split(strings.TrimSpace(story), "\n\n")
	if len(parts) == 0 {
		return strings.TrimSpace(story)
	}
	seen := make(map[string]struct{}, len(parts))
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := storySignature(part)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		kept = append(kept, part)
	}
	return strings.TrimSpace(strings.Join(kept, "\n\n"))
}

func dedupeStorySentences(story string) string {
	sentences := splitStorySentences(story)
	if len(sentences) == 0 {
		return strings.TrimSpace(story)
	}
	seen := make(map[string]int, len(sentences))
	kept := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}
		key := storySignature(sentence)
		if seen[key] >= 1 {
			continue
		}
		seen[key]++
		kept = append(kept, sentence)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func storySignature(text string) string {
	replacer := strings.NewReplacer(
		" ", "", "　", "", "\n", "", "。", "", "、", "", "！", "", "？", "",
		"「", "", "」", "", "（", "", "）", "", "(", "", ")", "", "『", "", "』", "",
	)
	return replacer.Replace(strings.TrimSpace(text))
}

func storyLogSnippet(story string) string {
	story = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(story, "\n", " "), "\r", " "))
	runes := []rune(story)
	if len(runes) > 180 {
		return string(runes[:180]) + "..."
	}
	return story
}

func storyHasMetaLeak(story string) bool {
	patterns := []string{
		"元の『",
		"元作品",
		"禁じられていたのは",
		"ここではそれが別の形",
		"読後感だった",
		"改変方式",
		"必須モチーフ",
		"報酬と罰",
	}
	for _, pattern := range patterns {
		if strings.Contains(story, pattern) {
			return true
		}
	}
	return false
}

func stripStoryMetaLeak(story string) string {
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(story, "\r\n", "\n"), "\r", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if storyHasMetaLeak(line) {
			continue
		}
		kept = append(kept, line)
	}
	clean := strings.TrimSpace(strings.Join(kept, "\n"))
	for strings.Contains(clean, "。。") {
		clean = strings.ReplaceAll(clean, "。。", "。")
	}
	return clean
}

func storySatisfiesSkeleton(story string, skeleton StorySkeleton, adaptation StoryAdaptationPlan) bool {
	if strings.TrimSpace(story) == "" {
		return false
	}
	if !storyHasRecognitionCues(story, skeleton) {
		return false
	}
	return true
}

func storyHasRecognitionCues(story string, skeleton StorySkeleton) bool {
	if len(skeleton.RecognitionCues) == 0 {
		return true
	}
	hits := 0
	for _, cue := range skeleton.RecognitionCues {
		if cue != "" && strings.Contains(story, cue) {
			hits++
		}
	}
	need := 2
	if len(skeleton.RecognitionCues) < need {
		need = len(skeleton.RecognitionCues)
	}
	if need == 0 {
		return true
	}
	return hits >= need
}


func firstStoryMotifLabel(motifMap []string) string {
	if len(motifMap) == 0 {
		return "元話の核"
	}
	return firstToken(motifMap[0])
}

func firstToken(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "=>") {
		parts := strings.SplitN(s, "=>", 2)
		s = strings.TrimSpace(parts[1])
	}
	for _, sep := range []string{"、", " ", "の", "と"} {
		if idx := strings.Index(s, sep); idx > 0 {
			return strings.TrimSpace(s[:idx])
		}
	}
	return s
}

func defaultStoryMotifMap(style string, motifs []string) []string {
	out := make([]string, 0, len(motifs))
	for _, motif := range motifs {
		out = append(out, motif+"=>"+transformMotif(style, motif))
	}
	return out
}

func transformMotif(style, motif string) string {
	style = normalizeStoryRewriteStyle(style)
	switch style {
	case "view_shift":
		switch motif {
		case "舌を切る":
			return "声を奪われた理由"
		case "小さいつづら":
			return "控えめな贈り物"
		case "大きいつづら":
			return "欲の大きい選択肢"
		case "玉手箱":
			return "開けるなと言われた包み"
		case "時間のずれ":
			return "待っていた側の空白"
		case "亀を助ける":
			return "見捨てずに庇った相手"
		}
	case "value_shift":
		switch motif {
		case "舌を切る":
			return "善意の名で発言権を奪う処置"
		case "小さいつづら":
			return "控えめだが自由のある謝礼"
		case "大きいつづら":
			return "豪華だが断れない支援"
		case "玉手箱":
			return "開けば借りを負う封筒"
		case "時間のずれ":
			return "戻った時に生まれる社会的な空白"
		case "亀を助ける":
			return "助けた後に責任まで背負う相手"
		}
	default:
		switch motif {
		case "舌を切る":
			return "言葉を奪う処分"
		case "小さいつづら":
			return "小さな箱"
		case "大きいつづら":
			return "大きな箱"
		case "玉手箱":
			return "禁を破る箱"
		case "時間のずれ":
			return "帰還後の時間差"
		case "亀を助ける":
			return "弱った相手を助ける"
		}
	}
	return motif
}

func storySkeleton(source StorySource) StorySkeleton {
	if spec, ok := storySpecForSource(source); ok {
		return spec.Skeleton
	}
	log.Printf("[Story] skeleton not found for source %q: spec missing in JSON", source.ID)
	return StorySkeleton{ID: source.ID, SourceTitle: source.Title}
}

func splitStoryNarration(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = storyChunkMaxRunes
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var out []string
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		for utf8.RuneCountInString(para) > maxRunes {
			idx := bestStorySplitIndex(para, maxRunes)
			head := strings.TrimSpace(para[:idx])
			if head != "" {
				out = append(out, head)
			}
			para = strings.TrimSpace(para[idx:])
		}
		if para != "" {
			out = append(out, para)
		}
	}
	return out
}

func bestStorySplitIndex(s string, maxRunes int) int {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return len(s)
	}
	limit := maxRunes
	if limit < storyChunkMinRunes {
		limit = storyChunkMinRunes
	}
	best := -1
	for i := limit - 1; i >= storyChunkMinRunes-1 && i < len(runes); i-- {
		switch runes[i] {
		case '。', '！', '？', '!', '?':
			return len(string(runes[:i+1]))
		case '、', '，', ',', '」':
			if best < 0 {
				best = len(string(runes[:i+1]))
			}
		}
	}
	if best > 0 {
		return best
	}
	return len(string(runes[:maxRunes]))
}
