package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultChatCLIBaseURL = "http://127.0.0.1:18790"
const chatCLIBaseURLEnv = "RENCROW_CHAT_URL"

type chatCLIEvent struct {
	Seq       int64  `json:"seq,omitempty"`
	Type      string `json:"type"`
	From      string `json:"from"`
	To        string `json:"to,omitempty"`
	Content   string `json:"content"`
	Route     string `json:"route,omitempty"`
	JobID     string `json:"job_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Channel   string `json:"channel,omitempty"`
	ChatID    string `json:"chat_id,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type chatCLIOptions struct {
	BaseURL string
	Message string
	Timeout time.Duration
}

func cmdChat() {
	os.Exit(runChatCommand(os.Args[2:], os.Stdin, os.Stdout, os.Stderr, http.DefaultClient))
}

func runChatCommand(args []string, in io.Reader, out, errOut io.Writer, client *http.Client) int {
	opts, err := parseChatCLIOptions(args)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	if client == nil {
		client = http.DefaultClient
	}
	if opts.Message != "" {
		return runChatOneShot(opts, out, errOut, client)
	}
	return runChatInteractive(opts, in, out, errOut, client)
}

func parseChatCLIOptions(args []string) (chatCLIOptions, error) {
	opts := chatCLIOptions{
		BaseURL: defaultChatCLIBaseURL,
		Timeout: 5 * time.Minute,
	}
	if envURL := strings.TrimSpace(os.Getenv(chatCLIBaseURLEnv)); envURL != "" {
		opts.BaseURL = envURL
	}
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.BaseURL, "url", opts.BaseURL, "RenCrow server base URL")
	fs.StringVar(&opts.Message, "message", "", "send one message and wait for the first response event")
	fs.DurationVar(&opts.Timeout, "timeout", opts.Timeout, "one-shot wait timeout")
	if err := fs.Parse(args); err != nil {
		return opts, fmt.Errorf("usage: picoclaw chat [--url URL] [--message TEXT] [--timeout 30s]")
	}
	if opts.Message == "" && len(fs.Args()) > 0 {
		opts.Message = strings.Join(fs.Args(), " ")
	}
	opts.BaseURL = strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if opts.BaseURL == "" {
		return opts, fmt.Errorf("chat url is required")
	}
	if _, err := url.ParseRequestURI(opts.BaseURL); err != nil {
		return opts, fmt.Errorf("invalid chat url: %w", err)
	}
	if opts.Timeout <= 0 {
		return opts, fmt.Errorf("timeout must be positive")
	}
	return opts, nil
}

func runChatOneShot(opts chatCLIOptions, out, errOut io.Writer, client *http.Client) int {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	events := make(chan chatCLIEvent, 32)
	ready := make(chan error, 1)
	done := make(chan error, 1)
	go func() {
		done <- streamChatCLIEvents(ctx, client, opts.BaseURL, events, ready)
	}()
	if err := <-ready; err != nil {
		fmt.Fprintf(errOut, "chat events unavailable: %v\n", err)
		return 1
	}
	if err := sendChatCLIMessage(ctx, client, opts.BaseURL, opts.Message); err != nil {
		fmt.Fprintf(errOut, "chat send failed: %v\n", err)
		return 1
	}
	sentAt := time.Now()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if !printChatCLIEvent(out, ev, sentAt) {
				continue
			}
			if isChatCLITerminalResponse(ev) {
				return 0
			}
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintf(errOut, "chat events stopped: %v\n", err)
				return 1
			}
		case <-ctx.Done():
			fmt.Fprintf(errOut, "chat response timeout: %v\n", ctx.Err())
			return 1
		}
	}
}

func runChatInteractive(opts chatCLIOptions, in io.Reader, out, errOut io.Writer, client *http.Client) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan chatCLIEvent, 64)
	ready := make(chan error, 1)
	go func() {
		if err := streamChatCLIEvents(ctx, client, opts.BaseURL, events, ready); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(errOut, "chat events stopped: %v\n", err)
		}
	}()
	if err := <-ready; err != nil {
		fmt.Fprintf(errOut, "chat events unavailable: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, "RenCrow terminal chat. Type /quit to exit.")
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "you> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "/quit" || line == "/exit" {
			break
		}
		if err := sendChatCLIMessage(ctx, client, opts.BaseURL, line); err != nil {
			fmt.Fprintf(errOut, "chat send failed: %v\n", err)
			continue
		}
		sentAt := time.Now()
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					fmt.Fprintln(errOut, "chat events stopped")
					return 1
				}
				if ev.Type == "message.received" {
					continue
				}
				if !printChatCLIEvent(out, ev, sentAt) {
					continue
				}
				if isChatCLITerminalResponse(ev) {
					goto nextInput
				}
			case <-ctx.Done():
				fmt.Fprintf(errOut, "chat events stopped: %v\n", ctx.Err())
				return 1
			}
		}
	nextInput:
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(errOut, "chat input failed: %v\n", err)
		return 1
	}
	return 0
}

func sendChatCLIMessage(ctx context.Context, client *http.Client, baseURL, message string) error {
	body, err := json.Marshal(map[string]string{"message": message})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/viewer/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		text, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(text)))
	}
	return nil
}

func streamChatCLIEvents(ctx context.Context, client *http.Client, baseURL string, events chan<- chatCLIEvent, ready chan<- error) error {
	defer close(events)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/viewer/events", nil)
	if err != nil {
		ready <- err
		return err
	}
	req.Header.Set("Last-Event-ID", "9223372036854775807")
	res, err := client.Do(req)
	if err != nil {
		ready <- err
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		err := fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(text)))
		ready <- err
		return err
	}
	ready <- nil
	return parseChatCLISSE(ctx, res.Body, events)
}

func parseChatCLISSE(ctx context.Context, src io.Reader, events chan<- chatCLIEvent) error {
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var data strings.Builder
	flush := func() error {
		raw := strings.TrimSpace(data.String())
		data.Reset()
		if raw == "" {
			return nil
		}
		var ev chatCLIEvent
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			return nil
		}
		select {
		case events <- ev:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") || strings.HasPrefix(line, "id:") || strings.HasPrefix(line, "event:") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := flush(); err != nil {
		return err
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return ctx.Err()
}

func printChatCLIEvent(out io.Writer, ev chatCLIEvent, sentAt time.Time) bool {
	switch ev.Type {
	case "message.received":
		fmt.Fprintf(out, "user> %s\n", ev.Content)
	case "routing.decision":
		fmt.Fprintf(out, "route> %s %s\n", strings.TrimSpace(ev.Route), strings.TrimSpace(ev.Content))
	case "agent.response":
		from := strings.TrimSpace(ev.From)
		if from == "" {
			from = "agent"
		}
		fmt.Fprintf(out, "%s> token/sec: %.1f\n%s\n", from, estimateChatCLITokensPerSecond(ev.Content, sentAt), ev.Content)
	case "agent.error", "mailbox.error", "worker.classified_failure":
		fmt.Fprintf(out, "error> %s\n", ev.Content)
	default:
		return false
	}
	return true
}

func estimateChatCLITokensPerSecond(content string, sentAt time.Time) float64 {
	elapsed := time.Since(sentAt).Seconds()
	if elapsed < 0.001 {
		elapsed = 0.001
	}
	tokens := estimateChatCLIOutputTokens(content)
	if tokens < 1 {
		tokens = 1
	}
	return float64(tokens) / elapsed
}

func estimateChatCLIOutputTokens(content string) int {
	content = strings.TrimSpace(content)
	if content == "" {
		return 1
	}
	tokens := 0
	inASCIIWord := false
	for _, r := range content {
		if r <= 127 && (r == '_' || r == '-' || r == '\'' || r == '.' || r == '/' || r == ':' || r == '@' || r == '#' || r == '$' || r == '%' || r == '&' || r == '+' || r == '=' || r == '?' || r == '!' || r == ',' || r == ';' || r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}' || r == '"' || r == '`') {
			if inASCIIWord {
				inASCIIWord = false
			}
			continue
		}
		if r <= 127 {
			if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
				inASCIIWord = false
				continue
			}
			if !inASCIIWord {
				tokens++
				inASCIIWord = true
			}
			continue
		}
		inASCIIWord = false
		tokens++
	}
	if tokens < 1 {
		return 1
	}
	return tokens
}

func isChatCLITerminalResponse(ev chatCLIEvent) bool {
	return ev.Type == "agent.response" || ev.Type == "agent.error" || ev.Type == "mailbox.error" || ev.Type == "worker.classified_failure"
}
