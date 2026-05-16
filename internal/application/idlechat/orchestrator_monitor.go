package idlechat

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

func (o *IdleChatOrchestrator) monitorLoop() {
	defer o.wg.Done()

	ticker := time.NewTicker(idleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			go o.checkAndStartChat()
		}
	}
}

func (o *IdleChatOrchestrator) checkAndStartChat() {
	o.mu.Lock()
	idleDuration := time.Since(o.lastActivity)
	threshold := o.interval
	now := time.Now()
	nextTopicAt := o.nextTopicAt
	alreadyActive := o.chatActive
	chatBusy := o.chatBusy
	workerBusy := o.workerBusy
	manualMode := o.manualMode
	o.mu.Unlock()

	if alreadyActive {
		return
	}
	if chatBusy || workerBusy {
		return
	}
	if !nextTopicAt.IsZero() && now.Before(nextTopicAt) {
		return
	}
	if !manualMode && idleDuration < threshold {
		return
	}

	o.mu.Lock()
	o.chatActive = true
	plan := o.nextIdleSessionPlanLocked()
	o.sessionMode = plan.mode
	o.mu.Unlock()

	log.Printf("[IdleChat] Idle for %v, starting %s session", idleDuration.Round(time.Second), plan.mode)
	switch plan.mode {
	case "forecast":
		if plan.domain == nil {
			log.Printf("[Forecast] Missing domain in session plan, skipping")
		} else {
			o.runForecastDomainSession(*plan.domain)
		}
	case "story-simple":
		o.RunSimpleStorySession()
	default:
		o.runChatSession(plan.strategy)
	}

	o.mu.Lock()
	o.chatActive = false
	o.sessionMode = ""
	o.currentTopic = ""
	o.lastActivity = time.Now() // セッション終了でアイドル計測をリセット
	o.mu.Unlock()
}

func (o *IdleChatOrchestrator) runChatSession(strategy TopicStrategy) {
	sessionID := fmt.Sprintf("idle-%d", time.Now().Unix())
	startedAt := time.Now().In(jst)
	remainingTurns := o.maxTurns
	totalTurns := 0
	topicIndex := 0

	for remainingTurns > 0 {
		segmentID := fmt.Sprintf("%s-topic-%02d", sessionID, topicIndex)
		topicIndex++
		topic, strategy := o.generateTopicFromChat(segmentID, strategy)
		o.mu.Lock()
		o.currentTopic = topic
		o.mu.Unlock()
		log.Printf("[IdleChat] Topic: %s (%s, session=%s)", topic, strategy, segmentID)
		o.emitTopicToTimeline(segmentID, topic, strategy)

		segmentTurns := 0
		loopDetected := false
		loopReason := ""
		sessionInterrupted := false
		generationFailed := false
		transcript := make([]string, 0, remainingTurns)
		currentSpeaker := o.chatSpeakerIndex()

		for turn := 0; turn < remainingTurns; turn++ {
			select {
			case <-o.ctx.Done():
				return
			default:
			}

			o.mu.Lock()
			if !o.chatActive {
				o.mu.Unlock()
				log.Printf("[IdleChat] Session interrupted at turn %d", turn)
				sessionInterrupted = true
				loopReason = "interrupted"
				break
			}
			o.mu.Unlock()

			speaker := o.participants[currentSpeaker]
			nextSpeaker := o.participants[(currentSpeaker+1)%len(o.participants)]

			response, rawResponse, err := o.generateResponseWithRaw(speaker, nextSpeaker, segmentID, turn, segmentTurns, topic)
			if err != nil {
				log.Printf("[IdleChat] Generation error: %v", err)
				generationFailed = true
				if errors.Is(err, errIdleInvalidResponse) {
					loopReason = "invalid_response"
				} else {
					loopReason = "generation_error"
				}
				break
			}
			if isResponseTooSimilar(response, transcript) {
				loopDetected = true
				loopReason = "pre_emit_similarity"
				log.Printf("[IdleChat] Repetitive response detected before emit, summarize and restart")
				break
			}

			response = ensureTrailingPeriod(response)

			msg := domaintransport.NewMessage(speaker, nextSpeaker, segmentID, "", response)
			msg.Type = domaintransport.MessageTypeIdleChat
			o.memory.RecordMessage(msg)
			o.emitTimelineEvent(TimelineEvent{
				Type:       "idlechat.message",
				From:       speaker,
				To:         nextSpeaker,
				Content:    response,
				RawContent: rawResponse,
				SessionID:  segmentID,
			})
			transcript = append(transcript, fmt.Sprintf("%s: %s", speaker, response))
			segmentTurns++

			log.Printf("[IdleChat] [Turn %d] %s→%s: %s", turn, speaker, nextSpeaker, truncate(response, 80))
			o.waitBreak(speakerBreak)

			if segmentTurns >= maxTurnsPerTopic {
				loopDetected = true
				loopReason = "topic_turn_limit"
				log.Printf("[IdleChat] Topic turn limit reached (%d), summarize and switch topic", maxTurnsPerTopic)
				break
			}

			if reason := detectLoopReason(transcript); reason != "" {
				loopDetected = true
				loopReason = reason
				log.Printf("[IdleChat] Loop/repetition detected, summarize and restart with new topic")
				break
			}
			currentSpeaker = (currentSpeaker + 1) % len(o.participants)
		}

		remainingTurns -= segmentTurns
		totalTurns += segmentTurns
		endedAt := time.Now().In(jst)
		if segmentTurns > 0 {
			displayStrategy := TopicStrategy(fmt.Sprintf("%s: %s", strategy, truncate(topic, 30)))
			summary := o.saveSummary(segmentID, topic, displayStrategy, transcript, startedAt, endedAt, segmentTurns, loopDetected || sessionInterrupted || generationFailed, loopReason)
			o.speakSummary(segmentID, summary)
		}
		cooldown := topicBreak
		if sessionInterrupted || generationFailed {
			idleCooldown := o.interval
			if idleCooldown > cooldown {
				cooldown = idleCooldown
			}
		}
		o.mu.Lock()
		o.nextTopicAt = endedAt.Add(cooldown)
		o.mu.Unlock()

		if segmentTurns == 0 || sessionInterrupted || generationFailed || remainingTurns <= 0 {
			break
		}
		log.Printf("[IdleChat] Switching topic after %d turns (%d remaining)", segmentTurns, remainingTurns)
		o.waitBreak(cooldown)
	}

	log.Printf("[IdleChat] Session %s completed (%d turns)", sessionID, totalTurns)
}

// waitForTTSDone はTTS完了チャネルを待つ。nilなら即座に返る。

func (o *IdleChatOrchestrator) waitForTTSDone(ch <-chan struct{}) {
	if ch == nil {
		return
	}
	timeout := idleChatTTSWaitTimeout
	if timeout <= 0 {
		select {
		case <-o.ctx.Done():
		case <-ch:
		}
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-o.ctx.Done():
		return
	case <-ch:
	case <-timer.C:
		log.Printf("[IdleChat] TTS completion wait timed out after %s; continuing conversation", timeout)
	}
}

// waitBreak はTTS完了後の沈黙を待つ。

func (o *IdleChatOrchestrator) waitBreak(d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-o.ctx.Done():
		return
	case <-timer.C:
	}
}

// ensureTrailingPeriod はセリフ末尾に句読点がなければ「。」を追記する。

func ensureTrailingPeriod(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	last, _ := utf8.DecodeLastRuneInString(s)
	switch last {
	case '。', '！', '？', '!', '?', '…':
		return s
	}
	return s + "。"
}

func (o *IdleChatOrchestrator) nextIdleSessionPlanLocked() idleSessionPlan {
	normalStrategies := []TopicStrategy{
		StrategySingleGenre,
		StrategyDoubleGenre,
		StrategyExternalStimulus,
	}
	if o.autoStep < len(normalStrategies) {
		plan := idleSessionPlan{
			mode:     "idle",
			strategy: normalStrategies[o.autoStep],
		}
		o.autoStep++
		return plan
	}
	domain := forecastDomains[o.forecastStep%len(forecastDomains)]
	o.forecastStep = (o.forecastStep + 1) % len(forecastDomains)
	o.autoStep = 0
	return idleSessionPlan{
		mode:   "forecast",
		domain: &domain,
	}
}

func (o *IdleChatOrchestrator) chatSpeakerIndex() int {
	for i, p := range o.participants {
		if strings.EqualFold(p, "mio") {
			return i
		}
	}
	return 0
}
