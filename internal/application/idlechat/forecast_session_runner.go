package idlechat

import (
	"fmt"
	"log"
	"time"

	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

// RunForecastSession は6ドメインを順に回す未来展望セッションを実行する。
func (o *IdleChatOrchestrator) RunForecastSession() {
	sessionID := fmt.Sprintf("forecast-%d", time.Now().Unix())
	startedAt := time.Now().In(jst)
	sessionDomains := append([]ForecastDomain(nil), forecastDomains...)

	log.Printf("[Forecast] Session %s started (%d domains, max %d turns/domain)", sessionID, len(sessionDomains), forecastTurnsPerDomain)

	o.mu.Lock()
	o.chatActive = true
	o.sessionMode = "forecast"
	o.mu.Unlock()

	totalTurns := o.runForecastSessionDomains(sessionID, startedAt, sessionDomains)

	o.mu.Lock()
	o.chatActive = false
	o.sessionMode = ""
	o.currentTopic = ""
	o.sessionContext = ""
	o.lastActivity = time.Now()
	o.mu.Unlock()
	log.Printf("[Forecast] Session %s completed (%d total turns)", sessionID, totalTurns)
}

func (o *IdleChatOrchestrator) runForecastDomainSession(domain ForecastDomain) {
	sessionID := fmt.Sprintf("forecast-%d", time.Now().Unix())
	startedAt := time.Now().In(jst)
	totalTurns := o.runForecastSessionDomains(sessionID, startedAt, []ForecastDomain{domain})
	log.Printf("[Forecast] Session %s completed (%d total turns)", sessionID, totalTurns)
}

func (o *IdleChatOrchestrator) runForecastSessionDomains(sessionID string, startedAt time.Time, sessionDomains []ForecastDomain) int {
	totalTurns := 0

	for domainIdx, domain := range sessionDomains {
		select {
		case <-o.ctx.Done():
			return totalTurns
		default:
		}

		o.mu.Lock()
		if !o.chatActive {
			o.mu.Unlock()
			log.Printf("[Forecast] Session interrupted before domain %s", domain.Name)
			return totalTurns
		}
		o.mu.Unlock()

		// ドメインアナウンス
		announce := fmt.Sprintf("%sのテーマの時間です。", domain.Name)
		log.Printf("[Forecast] [Domain %d/%d] %s", domainIdx+1, len(sessionDomains), domain.Name)

		announceMsg := domaintransport.NewMessage("user", "mio", sessionID, "", announce)
		announceMsg.Type = domaintransport.MessageTypeIdleChat
		o.memory.RecordMessage(announceMsg)
		ttsDone := o.emitTimelineEvent(TimelineEvent{
			Type:      "idlechat.message",
			From:      "user",
			To:        "mio",
			Content:   announce,
			SessionID: sessionID,
		})
		o.waitForTTSDone(ttsDone)

		// ドメイン特化トピック生成: ストックから取得（空ならインライン生成）
		displayTopic, seeds := o.popForecastTopic(domain)
		llmTopic := buildForecastLLMTopic(domain, displayTopic, seeds)

		o.mu.Lock()
		o.currentTopic = fmt.Sprintf("[%s] %s", domain.Name, displayTopic)
		o.mu.Unlock()

		// Viewer/TTS にはシンプルなお題を表示
		topicAnnounce := fmt.Sprintf("お題は、%s", displayTopic)
		topicMsg := domaintransport.NewMessage("user", "mio", sessionID, "", topicAnnounce)
		topicMsg.Type = domaintransport.MessageTypeIdleChat
		o.memory.RecordMessage(topicMsg)
		ttsDone = o.emitTimelineEvent(TimelineEvent{
			Type:      "idlechat.message",
			From:      "user",
			To:        "mio",
			Content:   topicAnnounce,
			SessionID: sessionID,
		})
		o.waitForTTSDone(ttsDone)
		o.waitBreak(topicBreak)

		// ドメイン内ターンループ（generateResponse には詳細版 llmTopic を渡す）
		topic := displayTopic // saveSummary 用
		transcript := make([]string, 0, forecastTurnsPerDomain)
		coveredThemes := make([]string, 0, 8)
		currentSpeaker := o.chatSpeakerIndex()
		segmentTurns := 0
		loopReason := ""
		interrupted := false
		genFailed := false

		// ドメイン開始時に sessionContext をクリア
		o.mu.Lock()
		o.sessionContext = ""
		o.mu.Unlock()

		for turn := 0; turn < forecastTurnsPerDomain; turn++ {
			select {
			case <-o.ctx.Done():
				return totalTurns
			default:
			}

			o.mu.Lock()
			if !o.chatActive {
				o.mu.Unlock()
				interrupted = true
				loopReason = "interrupted"
				break
			}
			o.mu.Unlock()

			speaker := o.participants[currentSpeaker]
			nextSpeaker := o.participants[(currentSpeaker+1)%len(o.participants)]

			// チェックポイント: 既出テーマを蓄積し sessionContext に反映
			if segmentTurns > 0 && segmentTurns%forecastCheckpointInterval == 0 {
				newThemes := o.extractCoveredThemes(domain, displayTopic, transcript, coveredThemes)
				if len(newThemes) > 0 {
					coveredThemes = append(coveredThemes, newThemes...)
					o.updateForecastSessionContext(domain, displayTopic, coveredThemes)
					log.Printf("[Forecast] Checkpoint at turn %d: covered themes now %d", segmentTurns, len(coveredThemes))
				}
			}

			// LLM には詳細な背景情報付きトピックを渡す
			response, err := o.generateResponse(speaker, nextSpeaker, sessionID, totalTurns+turn, segmentTurns, llmTopic)
			if err != nil {
				log.Printf("[Forecast] Generation error: %v", err)
				genFailed = true
				loopReason = "generation_error"
				break
			}
			if isResponseTooSimilar(response, transcript) {
				loopReason = "pre_emit_similarity"
				log.Printf("[Forecast] Repetitive response, moving to next domain")
				break
			}

			response = ensureTrailingPeriod(response)

			msg := domaintransport.NewMessage(speaker, nextSpeaker, sessionID, "", response)
			msg.Type = domaintransport.MessageTypeIdleChat
			o.memory.RecordMessage(msg)
			ttsDone := o.emitTimelineEvent(TimelineEvent{
				Type:      "idlechat.message",
				From:      speaker,
				To:        nextSpeaker,
				Content:   response,
				SessionID: sessionID,
			})
			transcript = append(transcript, fmt.Sprintf("%s: %s", speaker, response))
			segmentTurns++
			totalTurns++

			log.Printf("[Forecast] [%s Turn %d] %s→%s: %s", domain.Name, turn, speaker, nextSpeaker, truncate(response, 80))
			o.waitForTTSDone(ttsDone)
			o.waitBreak(speakerBreak)

			if reason := detectLoopReason(transcript); reason != "" {
				loopReason = reason
				log.Printf("[Forecast] Loop detected in %s, moving to next domain", domain.Name)
				break
			}
			currentSpeaker = (currentSpeaker + 1) % len(o.participants)
		}

		// ドメイン要約保存（Coder2で要約 + 継続考察テーマ付与）
		endedAt := time.Now().In(jst)
		if segmentTurns > 0 {
			summary := o.saveForecastSummary(sessionID, domain, topic, transcript, startedAt, endedAt, segmentTurns,
				interrupted || genFailed || loopReason != "", loopReason)
			o.speakSummary(sessionID, summary)
		}

		if interrupted {
			return totalTurns
		}

		// ドメイン間ブレイク（最後のドメイン以外）
		if domainIdx < len(sessionDomains)-1 {
			o.waitBreak(topicBreak)
		}
	}

	return totalTurns
}
