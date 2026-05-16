package orchestrator

import (
	"context"
	"fmt"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

type distributedCoderSelector func(route routing.Route, userMessage string) string
type distributedCoderConfigProvider func() map[string]interface{}
type distributedRetryMaxResolver func() int
type distributedMailboxExecutor func(ctx context.Context, targetAgent string, msg domaintransport.Message, receiveOnAgent string) (domaintransport.Message, error)
type distributedAgentExecutor func(ctx context.Context, targetAgent string, msg domaintransport.Message) (domaintransport.Message, error)

type distributedCodeExecutionCoordinator struct {
	memory         *session.CentralMemory
	emit           messageEventEmitter
	emitNote       distributedNoteEmitter
	selectCoder    distributedCoderSelector
	coderConfigs   distributedCoderConfigProvider
	coderRetryMax  distributedRetryMaxResolver
	executeMailbox distributedMailboxExecutor
	executeToAgent distributedAgentExecutor
}

func newDistributedCodeExecutionCoordinator(
	memory *session.CentralMemory,
	emit messageEventEmitter,
	emitNote distributedNoteEmitter,
	selectCoder distributedCoderSelector,
	coderConfigs distributedCoderConfigProvider,
	coderRetryMax distributedRetryMaxResolver,
	executeMailbox distributedMailboxExecutor,
	executeToAgent distributedAgentExecutor,
) *distributedCodeExecutionCoordinator {
	return &distributedCodeExecutionCoordinator{
		memory:         memory,
		emit:           emit,
		emitNote:       emitNote,
		selectCoder:    selectCoder,
		coderConfigs:   coderConfigs,
		coderRetryMax:  coderRetryMax,
		executeMailbox: executeMailbox,
		executeToAgent: executeToAgent,
	}
}

func (c *distributedCodeExecutionCoordinator) Execute(ctx context.Context, t task.Task, route routing.Route, sessionID, jid string) (string, error) {
	coderAgent := c.selectCoder(route, t.UserMessage())
	if coderAgent == "" {
		return "", fmt.Errorf("no coder mapped for route %s", route)
	}
	log.Printf("[DistributedOrch] code handoff route=%s target=%s job=%s", route, coderAgent, jid)

	c.emit("agent.start", "mio", "shiro", "コードタスクをShiro経由で実行", string(route), jid, sessionID, t.Channel(), t.ChatID())
	c.emitNote("mio", "user", "しろにコード実装の取りまとめをお願いしたよ。", string(route), jid, sessionID, t.Channel(), t.ChatID())
	requestText := t.UserMessage()

	for attempt := 0; attempt <= c.coderRetryMax(); attempt++ {
		c.emit("agent.start", "shiro", coderAgent, requestText, string(route), jid, sessionID, t.Channel(), t.ChatID())
		if attempt == 0 {
			c.emitNote("shiro", "mio", fmt.Sprintf("%sにコーディング依頼しました。進捗を監視して、必要なら作業を前に進めます。", displayAgentName(coderAgent)), string(route), jid, sessionID, t.Channel(), t.ChatID())
		} else {
			c.emit("worker.retry_request", "shiro", coderAgent, fmt.Sprintf("retry=%d", attempt), string(route), jid, sessionID, t.Channel(), t.ChatID())
			c.emitNote("shiro", "mio", fmt.Sprintf("%sに修正版patchを再依頼します。retry=%d", displayAgentName(coderAgent), attempt), string(route), jid, sessionID, t.Channel(), t.ChatID())
		}

		coderMsg := c.buildCoderMessage(coderAgent, sessionID, jid, requestText, route, t, attempt)
		c.memory.RecordMessage(coderMsg)

		coderResult, err := c.executeMailbox(ctx, coderAgent, coderMsg, "mio")
		if err != nil {
			failureKind, reason, retryable := classifyDistributedExecutionError(err)
			if retryable && attempt < c.coderRetryMax() {
				c.emit("worker.classified_failure", "shiro", coderAgent, fmt.Sprintf("%s: %s", failureKind, reason), string(route), jid, sessionID, t.Channel(), t.ChatID())
				requestText = buildCoderRetryInstruction(t.UserMessage(), nil, failureKind, reason, attempt+1)
				continue
			}
			return "", err
		}
		c.emit("agent.response", coderAgent, "shiro", coderResult.Content, string(route), jid, sessionID, t.Channel(), t.ChatID())
		c.emitNote(coderAgent, "shiro", "おわったっす。", string(route), jid, sessionID, t.Channel(), t.ChatID())
		c.emitNote("shiro", "mio", fmt.Sprintf("%sの結果を受け取って、内容確認と仕上げを進めます。", displayAgentName(coderAgent)), string(route), jid, sessionID, t.Channel(), t.ChatID())

		if coderResult.Proposal == nil {
			return c.finishWithoutProposal(ctx, t, route, sessionID, jid, coderAgent, coderResult)
		}

		response, retryReq, retryable, err := c.executeProposal(ctx, t, route, sessionID, jid, coderAgent, coderResult, attempt)
		if err != nil {
			return "", err
		}
		if retryable {
			requestText = retryReq
			continue
		}
		return response, nil
	}
	return "", fmt.Errorf("coder retry budget exhausted for job %s", jid)
}

func (c *distributedCodeExecutionCoordinator) buildCoderMessage(coderAgent, sessionID, jid, requestText string, route routing.Route, t task.Task, attempt int) domaintransport.Message {
	coderMsg := domaintransport.NewMessage("shiro", coderAgent, sessionID, jid, requestText)
	coderMsg.Type = domaintransport.MessageTypeTask
	coderMsg.Context = map[string]interface{}{
		"route":         string(route),
		"retry_attempt": attempt,
		"channel":       t.Channel(),
		"chat_id":       t.ChatID(),
	}
	if configs := c.coderConfigs(); configs != nil {
		if coderCfg, ok := configs[coderAgent]; ok {
			coderMsg.Context["coder_config"] = coderCfg
		}
	}
	return coderMsg
}

func (c *distributedCodeExecutionCoordinator) finishWithoutProposal(ctx context.Context, t task.Task, route routing.Route, sessionID, jid, coderAgent string, coderResult domaintransport.Message) (string, error) {
	c.emit("agent.start", "shiro", "mio", "Coder結果をShiroで整形", string(route), jid, sessionID, t.Channel(), t.ChatID())
	shiroTask := domaintransport.NewMessage("mio", "shiro", sessionID, jid, coderResult.Content)
	shiroTask.Type = domaintransport.MessageTypeTask
	shiroTask.Context = map[string]interface{}{
		"route":       string(route),
		"coder_agent": coderAgent,
		"channel":     t.Channel(),
		"chat_id":     t.ChatID(),
	}
	c.memory.RecordMessage(shiroTask)
	shiroResult, err := c.executeToAgent(ctx, "shiro", shiroTask)
	if err != nil {
		return "", err
	}
	c.emit("agent.response", "shiro", "mio", shiroResult.Content, string(route), jid, sessionID, t.Channel(), t.ChatID())
	c.emitNote("shiro", "mio", fmt.Sprintf("%sの作業が終わりました。", displayAgentName(coderAgent)), string(route), jid, sessionID, t.Channel(), t.ChatID())
	return shiroResult.Content, nil
}

func (c *distributedCodeExecutionCoordinator) executeProposal(ctx context.Context, t task.Task, route routing.Route, sessionID, jid, coderAgent string, coderResult domaintransport.Message, attempt int) (response, retryRequest string, retryable bool, err error) {
	c.emit("agent.start", "shiro", "mio", "CoderのProposalをWorker実行", string(route), jid, sessionID, t.Channel(), t.ChatID())
	execMsg := domaintransport.NewMessage("mio", "shiro", sessionID, jid, "Execute coder proposal")
	execMsg.Type = domaintransport.MessageTypeTask
	execMsg.Context = map[string]interface{}{
		"route":         string(route),
		"coder_agent":   coderAgent,
		"retry_attempt": attempt,
		"channel":       t.Channel(),
		"chat_id":       t.ChatID(),
	}
	execMsg.Proposal = coderResult.Proposal
	c.memory.RecordMessage(execMsg)

	shiroResult, err := c.executeToAgent(ctx, "shiro", execMsg)
	if err != nil {
		failureKind, reason, retryableFailure := classifyDistributedExecutionError(err)
		if retryableFailure && attempt < c.coderRetryMax() {
			c.emit("worker.classified_failure", "shiro", coderAgent, fmt.Sprintf("%s: %s", failureKind, reason), string(route), jid, sessionID, t.Channel(), t.ChatID())
			return "", buildCoderRetryInstruction(t.UserMessage(), coderResult.Proposal, failureKind, reason, attempt+1), true, nil
		}
		return "", "", false, err
	}
	c.emit("agent.response", "shiro", "mio", shiroResult.Content, string(route), jid, sessionID, t.Channel(), t.ChatID())
	c.emitNote("shiro", "mio", fmt.Sprintf("%sの作業が終わりました。", displayAgentName(coderAgent)), string(route), jid, sessionID, t.Channel(), t.ChatID())

	if retryReq, ok := nextCoderRetryRequest(t.UserMessage(), coderResult.Proposal, shiroResult, attempt); ok {
		return "", retryReq, true, nil
	}
	return shiroResult.Content, "", false, nil
}
