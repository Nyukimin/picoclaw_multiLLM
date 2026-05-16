package idlechat

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

const (
	idleCheckInterval              = 30 * time.Second
	maxTurnsPerTopic               = 12
	idleChatResponseMaxTokens      = 512
	idleChatRetryMaxTokens         = 256
	idleChatShiroResponseMaxTokens = 768
	idleChatShiroRetryMaxTokens    = 384
	idleChatShiroSummaryMaxTokens  = 1200
	idleChatQualityReviewMaxTokens = 900
	speakerBreak                   = 500 * time.Millisecond  // 話者交代ブレイク（TTS完了後）
	topicBreak                     = 1000 * time.Millisecond // 話題交代ブレイク（TTS完了後）
)

var idleChatTTSWaitTimeout = 35 * time.Second

var idleChatLLMGenerateTimeout = 45 * time.Second

var jst = time.FixedZone("JST", 9*60*60)

var randSeedOnce sync.Once

var errIdleInvalidResponse = errors.New("idlechat invalid response")

var errIdleGenerationFailed = errors.New("idlechat generation failed")

var promptLeakLineRe = regexp.MustCompile(`(?i)(^|[。．\n])[^。．\n]{0,30}(発言として受け|要件[:：]|発言帰属ガード)[^。．\n]*`)

type SessionSummary struct {
	SessionID         string        `json:"session_id"`
	Title             string        `json:"title"`
	Topic             string        `json:"topic"`
	Strategy          TopicStrategy `json:"strategy"` // 生成戦略（旧 Category）
	Summary           string        `json:"summary"`
	QualityReview     string        `json:"quality_review,omitempty"`
	PromptGuidance    string        `json:"prompt_guidance,omitempty"`
	SourceTitle       string        `json:"source_title,omitempty"`
	RewriteStyle      string        `json:"rewrite_style,omitempty"`
	StoryTitle        string        `json:"story_title,omitempty"`
	StoryText         string        `json:"story_text,omitempty"`
	StoryDraftText    string        `json:"story_draft_text,omitempty"`
	StoryRevisionNote string        `json:"story_revision_note,omitempty"`
	StoryEndingFlavor string        `json:"story_ending_flavor,omitempty"`
	StartedAt         string        `json:"started_at"`
	EndedAt           string        `json:"ended_at"`
	Turns             int           `json:"turns"`
	LoopRestarted     bool          `json:"loop_restarted"`
	LoopReason        string        `json:"loop_reason,omitempty"`
	TopicProvider     string        `json:"topic_provider"`
	SummaryProvider   string        `json:"summary_provider"`
	Transcript        []string      `json:"transcript,omitempty"`
}

type TimelineEvent struct {
	Type       string
	From       string
	To         string
	Content    string
	RawContent string
	SessionID  string
}

// IdleChatOrchestrator はアイドル時のAgent間雑談を管理

type IdleChatOrchestrator struct {
	llmProvider      llm.LLMProvider
	speakerLLMs      map[string]llm.LLMProvider
	forecastProvider llm.LLMProvider // 未来展望セッションの思考用（Coder2等の高性能モデル）
	sessionContext   string          // 現在のセッション固有コンテキスト（既出テーマ等）
	memory           *session.CentralMemory
	participants     []string
	intervalMin      int
	interval         time.Duration
	maxTurns         int
	temperature      float64
	personalities    map[string]string

	lastActivity  time.Time
	chatActive    bool
	chatBusy      bool
	workerBusy    bool
	manualMode    bool
	sessionMode   string
	currentTopic  string
	promptGuides  []string
	autoStep      int
	forecastStep  int
	nextTopicAt   time.Time
	history       []SessionSummary
	emitEvent     func(TimelineEvent) <-chan struct{}
	topicStore    *TopicStore
	topicStockBuf *forecastTopicStock // 未来展望お題ストック
	recentTopics  func(context.Context, int) ([]string, error)

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	wg     sync.WaitGroup
}

type idleSessionPlan struct {
	mode     string
	strategy TopicStrategy
	domain   *ForecastDomain
}

// SetEventEmitter sets an optional timeline event emitter used by viewer SSE.
// The callback returns a channel that closes when TTS playback completes (nil = no TTS).
