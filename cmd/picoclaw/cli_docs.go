package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type docsCLIResult struct {
	ID      string `json:"id"`
	Repo    string `json:"repo"`
	Path    string `json:"path"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Score   int    `json:"score"`
}

type docsCLIDetail struct {
	ID      string `json:"id"`
	Repo    string `json:"repo"`
	Path    string `json:"path"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type docsCLISearchResponse struct {
	Items []docsCLIResult `json:"items"`
}

type docsCLIDetailResponse struct {
	Doc docsCLIDetail `json:"doc"`
}

func cmdDocs() {
	os.Exit(runDocsCommand(os.Args[2:], os.Stdout, os.Stderr, http.DefaultClient))
}

func runDocsCommand(args []string, out, errOut io.Writer, client *http.Client) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: rencrow docs search QUERY | rencrow docs show DOC_ID")
		return 2
	}
	switch args[0] {
	case "search", "find":
		return runDocsSearchCommand(args[1:], out, errOut, client)
	case "show", "detail":
		return runDocsShowCommand(args[1:], out, errOut, client)
	default:
		fmt.Fprintf(errOut, "unknown docs command: %s\n", args[0])
		return 2
	}
}

func runDocsSearchCommand(args []string, out, errOut io.Writer, client *http.Client) int {
	opts := struct {
		baseURL string
		limit   int
		json    bool
	}{
		baseURL: defaultChatCLIBaseURL,
		limit:   10,
	}
	if envURL := strings.TrimSpace(os.Getenv(chatCLIBaseURLEnv)); envURL != "" {
		opts.baseURL = envURL
	}
	fs := flag.NewFlagSet("docs search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.baseURL, "url", opts.baseURL, "RenCrow server base URL")
	fs.IntVar(&opts.limit, "limit", opts.limit, "maximum result count")
	fs.BoolVar(&opts.json, "json", opts.json, "output JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(errOut, "usage: rencrow docs search [--url URL] [--limit N] [--json] QUERY")
		return 2
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		fmt.Fprintln(errOut, "docs search query is required")
		return 2
	}
	baseURL := normalizeDocsCLIBaseURL(opts.baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	items, err := fetchDocsCLISearch(ctx, client, baseURL, query, opts.limit)
	if err != nil {
		fmt.Fprintf(errOut, "docs search failed: %v\n", err)
		return 1
	}
	if opts.json {
		writeJSONCLI(out, docsCLISearchResponse{Items: items}, true)
		return 0
	}
	printDocsCLIResults(out, items)
	return 0
}

func runDocsShowCommand(args []string, out, errOut io.Writer, client *http.Client) int {
	opts := struct {
		baseURL string
		json    bool
	}{
		baseURL: defaultChatCLIBaseURL,
	}
	if envURL := strings.TrimSpace(os.Getenv(chatCLIBaseURLEnv)); envURL != "" {
		opts.baseURL = envURL
	}
	fs := flag.NewFlagSet("docs show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.baseURL, "url", opts.baseURL, "RenCrow server base URL")
	fs.BoolVar(&opts.json, "json", opts.json, "output JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(errOut, "usage: rencrow docs show [--url URL] [--json] DOC_ID")
		return 2
	}
	id := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if id == "" {
		fmt.Fprintln(errOut, "docs show id is required")
		return 2
	}
	baseURL := normalizeDocsCLIBaseURL(opts.baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	doc, err := fetchDocsCLIDetail(ctx, client, baseURL, id)
	if err != nil {
		fmt.Fprintf(errOut, "docs show failed: %v\n", err)
		return 1
	}
	if opts.json {
		writeJSONCLI(out, docsCLIDetailResponse{Doc: doc}, true)
		return 0
	}
	fmt.Fprintf(out, "# %s\n\nid: %s\nrepo: %s\npath: %s\n\n%s\n", doc.Title, doc.ID, doc.Repo, doc.Path, doc.Content)
	return 0
}

func handleDocsCLIAtCommand(ctx context.Context, client *http.Client, baseURL, line string, out, errOut io.Writer) bool {
	query := strings.TrimSpace(strings.TrimPrefix(line, "@"))
	if query == "" {
		fmt.Fprintln(out, "docs search: @QUERY")
		return true
	}
	items, err := fetchDocsCLISearch(ctx, client, normalizeDocsCLIBaseURL(baseURL), query, 8)
	if err != nil {
		fmt.Fprintf(errOut, "docs search failed: %v\n", err)
		return true
	}
	printDocsCLIResults(out, items)
	return true
}

func fetchDocsCLISearch(ctx context.Context, client *http.Client, baseURL, query string, limit int) ([]docsCLIResult, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if limit <= 0 {
		limit = 10
	}
	endpoint := fmt.Sprintf("%s/viewer/docs/search?q=%s&limit=%d", normalizeDocsCLIBaseURL(baseURL), url.QueryEscape(strings.TrimSpace(query)), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		text, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(text)))
	}
	var payload docsCLISearchResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 1024*1024)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode docs search: %w", err)
	}
	return payload.Items, nil
}

func fetchDocsCLIDetail(ctx context.Context, client *http.Client, baseURL, id string) (docsCLIDetail, error) {
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := fmt.Sprintf("%s/viewer/docs/detail?id=%s", normalizeDocsCLIBaseURL(baseURL), url.QueryEscape(strings.TrimSpace(id)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return docsCLIDetail{}, err
	}
	res, err := client.Do(req)
	if err != nil {
		return docsCLIDetail{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		text, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return docsCLIDetail{}, fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(text)))
	}
	var payload docsCLIDetailResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 2*1024*1024)).Decode(&payload); err != nil {
		return docsCLIDetail{}, fmt.Errorf("decode docs detail: %w", err)
	}
	return payload.Doc, nil
}

func printDocsCLIResults(out io.Writer, items []docsCLIResult) {
	if len(items) == 0 {
		fmt.Fprintln(out, "docs: no matches")
		return
	}
	for i, item := range items {
		fmt.Fprintf(out, "%d. %s\n   id: %s\n   path: %s/%s\n", i+1, item.Title, item.ID, item.Repo, item.Path)
		if strings.TrimSpace(item.Snippet) != "" {
			fmt.Fprintf(out, "   %s\n", item.Snippet)
		}
	}
}

func normalizeDocsCLIBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return defaultChatCLIBaseURL
	}
	return baseURL
}
