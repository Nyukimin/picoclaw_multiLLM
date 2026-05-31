package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	webgatherapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/webgather"
	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
	webgatherinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/webgather"
	modulewebgather "github.com/Nyukimin/picoclaw_multiLLM/modules/webgather"
)

func cmdWebGather() {
	configPath := getConfigPath()
	store, err := loadWebGatherStore(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize web gather store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()
	usecase := webgatherapp.NewUseCase(
		webgatherinfra.NewHTTPFetcher(),
		webgatherinfra.NewBasicExtractor(),
		webgatherapp.NewL1StagingWriter(store),
	)
	code := runWebGatherCommand(os.Args[2:], webGatherCLIDeps{
		Fetcher:        usecase,
		SearchCache:    webgatherapp.NewL1SearchCache(store),
		SourceRegistry: store,
	}, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

type webGatherFetcher interface {
	FetchURL(ctx context.Context, req modulewebgather.FetchRequest) (modulewebgather.FetchResponse, error)
}

type webGatherSearcher interface {
	Search(ctx context.Context, req modulewebgather.SearchRequest) (modulewebgather.SearchResponse, error)
}

type webGatherSourceRegistry interface {
	SaveSourceRegistryEntry(ctx context.Context, entry conversationpersistence.L1SourceRegistryEntry) (*conversationpersistence.L1SourceRegistryEntry, error)
}

type webGatherCLIDeps struct {
	Fetcher        webGatherFetcher
	SearchCache    webgatherapp.SearchCache
	SourceRegistry webGatherSourceRegistry
}

func runWebGatherCommand(args []string, deps webGatherCLIDeps, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: picoclaw web-gather [url|search|search-and-fetch|register-url] ...")
		return 2
	}
	subcmd := strings.ToLower(strings.TrimSpace(args[0]))
	switch subcmd {
	case "url":
		req, jsonOut, err := parseWebGatherURLArgs(args[1:])
		if err != nil {
			fmt.Fprintf(errOut, "%v\n", err)
			return 2
		}
		if deps.Fetcher == nil {
			fmt.Fprintln(errOut, "web gather fetcher is not configured")
			return 1
		}
		resp, err := deps.Fetcher.FetchURL(context.Background(), req)
		if jsonOut {
			writeJSONCLI(out, resp, true)
		}
		if err != nil {
			if !jsonOut {
				fmt.Fprintf(errOut, "web-gather failed: %s: %s\n", resp.ErrorCode, resp.ErrorMessage)
			}
			return 1
		}
		if !jsonOut {
			fmt.Fprintf(out, "web gather staged: %s | %s | %s | warnings=%d\n", resp.StagingID, resp.FinalURL, resp.RawHash, len(resp.SecurityWarnings))
		}
		return 0
	case "search":
		req, searxngURL, jsonOut, err := parseWebGatherSearchArgs(args[1:])
		if err != nil {
			fmt.Fprintf(errOut, "%v\n", err)
			return 2
		}
		providers := map[string]modulewebgather.SearchProvider{}
		if strings.TrimSpace(searxngURL) != "" {
			providers["searxng"] = webgatherinfra.NewSearXNGProvider(searxngURL)
		}
		searcher := webgatherapp.NewSearchUseCase(deps.SearchCache, providers)
		resp, err := searcher.Search(context.Background(), req)
		if jsonOut {
			writeJSONCLI(out, resp, true)
		}
		if err != nil {
			if !jsonOut {
				if e := resp.Diagnostics["error"]; e != nil {
					fmt.Fprintf(errOut, "web-gather search failed: %v\n", e)
				} else {
					fmt.Fprintf(errOut, "web-gather search failed: %v\n", err)
				}
			}
			return 1
		}
		if !jsonOut {
			fmt.Fprintf(out, "web gather search: provider=%s results=%d cache_hit=%v\n", resp.Provider, len(resp.Results), resp.Diagnostics["cache_hit"])
			for _, result := range resp.Results {
				fmt.Fprintf(out, "%d. %s\n   %s\n   %s\n", result.Rank, result.Title, result.Snippet, result.URL)
			}
		}
		return 0
	case "search-and-fetch":
		req, searxngURL, jsonOut, err := parseWebGatherSearchAndFetchArgs(args[1:])
		if err != nil {
			fmt.Fprintf(errOut, "%v\n", err)
			return 2
		}
		if deps.Fetcher == nil {
			fmt.Fprintln(errOut, "web gather fetcher is not configured")
			return 1
		}
		providers := map[string]modulewebgather.SearchProvider{}
		if strings.TrimSpace(searxngURL) != "" {
			providers["searxng"] = webgatherinfra.NewSearXNGProvider(searxngURL)
		}
		searcher := webgatherapp.NewSearchUseCase(deps.SearchCache, providers)
		usecase := webgatherapp.NewSearchAndFetchUseCase(searcher, deps.Fetcher)
		resp, err := usecase.SearchAndFetch(context.Background(), req)
		if jsonOut {
			writeJSONCLI(out, resp, true)
		}
		if err != nil {
			if !jsonOut {
				if e := resp.Diagnostics["error"]; e != nil {
					fmt.Fprintf(errOut, "web-gather search-and-fetch failed: %v\n", e)
				} else {
					fmt.Fprintf(errOut, "web-gather search-and-fetch failed: %v\n", err)
				}
			}
			return 1
		}
		if !jsonOut {
			fmt.Fprintf(out, "web gather search-and-fetch: provider=%s items=%d fetch_errors=%v\n", resp.Provider, len(resp.Items), resp.Diagnostics["fetch_error_cnt"])
			for _, item := range resp.Items {
				fmt.Fprintf(out, "%d. %s\n   %s\n   fetch=%s staging=%s\n", item.SearchResult.Rank, item.SearchResult.Title, item.SearchResult.URL, item.Fetch.Status, item.Fetch.StagingID)
			}
		}
		return 0
	case "register-url":
		entry, jsonOut, err := parseWebGatherRegisterURLArgs(args[1:])
		if err != nil {
			fmt.Fprintf(errOut, "%v\n", err)
			return 2
		}
		if deps.SourceRegistry == nil {
			fmt.Fprintln(errOut, "source registry is not configured")
			return 1
		}
		saved, err := deps.SourceRegistry.SaveSourceRegistryEntry(context.Background(), entry)
		if err != nil {
			fmt.Fprintf(errOut, "failed to register web gather source: %v\n", err)
			return 1
		}
		if jsonOut {
			writeJSONCLI(out, map[string]any{"entry": sourceRegistryCLIEntry(*saved)}, false)
			return 0
		}
		fmt.Fprintf(out, "registered web gather source: %s\n", saved.SourceID)
		return 0
	default:
		fmt.Fprintf(errOut, "unknown web-gather subcommand: %s\n", subcmd)
		fmt.Fprintln(errOut, "usage: picoclaw web-gather [url|search|search-and-fetch|register-url] ...")
		return 2
	}
}

func parseWebGatherURLArgs(args []string) (modulewebgather.FetchRequest, bool, error) {
	req := modulewebgather.FetchRequest{
		Namespace:       modulewebgather.DefaultNamespace,
		FetchProvider:   modulewebgather.DefaultFetchProvider,
		Extractor:       modulewebgather.DefaultExtractor,
		StoreStaging:    true,
		StoreStagingSet: true,
		LicenseNote:     modulewebgather.DefaultLicenseNote,
		Policy:          modulewebgather.DefaultFetchPolicy(),
	}
	jsonOut := false
	urlSet := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "--json":
			jsonOut = true
		case "--allow-localhost":
			req.Policy.AllowLocalhost = true
		case "--dry-run":
			req.DryRun = true
		case "--namespace", "--source-id", "--extractor", "--timeout-sec", "--max-body-bytes", "--max-redirects", "--license-note":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return req, jsonOut, fmt.Errorf("%s requires a value", arg)
			}
			value := strings.TrimSpace(args[i+1])
			i++
			switch arg {
			case "--namespace":
				req.Namespace = value
			case "--source-id":
				req.SourceID = value
			case "--extractor":
				if !isAllowedWebGatherExtractor(value) {
					return req, jsonOut, fmt.Errorf("unsupported extractor: %s", value)
				}
				req.Extractor = value
			case "--timeout-sec":
				sec, err := strconv.Atoi(value)
				if err != nil || sec <= 0 {
					return req, jsonOut, fmt.Errorf("invalid --timeout-sec: %s", value)
				}
				req.Policy.RequestTimeout = time.Duration(sec) * time.Second
			case "--max-body-bytes":
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil || n <= 0 {
					return req, jsonOut, fmt.Errorf("invalid --max-body-bytes: %s", value)
				}
				req.Policy.MaxBodyBytes = n
			case "--max-redirects":
				n, err := strconv.Atoi(value)
				if err != nil || n < 0 {
					return req, jsonOut, fmt.Errorf("invalid --max-redirects: %s", value)
				}
				req.Policy.MaxRedirects = n
			case "--license-note":
				req.LicenseNote = value
			}
		default:
			if strings.HasPrefix(arg, "--") {
				return req, jsonOut, fmt.Errorf("unknown web-gather url option: %s", arg)
			}
			if urlSet {
				return req, jsonOut, errors.New("web-gather url accepts exactly one URL")
			}
			req.URL = arg
			urlSet = true
		}
	}
	if strings.TrimSpace(req.URL) == "" {
		return req, jsonOut, errors.New("url is required")
	}
	return req, jsonOut, nil
}

func isAllowedWebGatherExtractor(value string) bool {
	switch strings.TrimSpace(value) {
	case "go_readability", "html_basic", "plain_text", "json_text":
		return true
	default:
		return false
	}
}

func parseWebGatherSearchArgs(args []string) (modulewebgather.SearchRequest, string, bool, error) {
	req := modulewebgather.SearchRequest{
		Provider:  modulewebgather.DefaultSearchProvider,
		Limit:     modulewebgather.DefaultSearchLimit,
		Language:  modulewebgather.DefaultSearchLanguage,
		Freshness: modulewebgather.DefaultSearchFreshness,
		Namespace: "kb:research",
	}
	searxngURL := ""
	jsonOut := false
	querySet := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "--json":
			jsonOut = true
		case "--refresh":
			req.Refresh = true
		case "--provider", "--limit", "--language", "--freshness", "--namespace", "--searxng-url":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return req, searxngURL, jsonOut, fmt.Errorf("%s requires a value", arg)
			}
			value := strings.TrimSpace(args[i+1])
			i++
			switch arg {
			case "--provider":
				if !isAllowedWebGatherSearchProvider(value) {
					return req, searxngURL, jsonOut, fmt.Errorf("unsupported search provider: %s", value)
				}
				req.Provider = value
			case "--limit":
				n, err := strconv.Atoi(value)
				if err != nil || n <= 0 {
					return req, searxngURL, jsonOut, fmt.Errorf("invalid --limit: %s", value)
				}
				req.Limit = n
			case "--language":
				req.Language = value
			case "--freshness":
				req.Freshness = value
			case "--namespace":
				req.Namespace = value
			case "--searxng-url":
				searxngURL = value
			}
		default:
			if strings.HasPrefix(arg, "--") {
				return req, searxngURL, jsonOut, fmt.Errorf("unknown web-gather search option: %s", arg)
			}
			if querySet {
				return req, searxngURL, jsonOut, errors.New("web-gather search accepts exactly one query")
			}
			req.Query = arg
			querySet = true
		}
	}
	if strings.TrimSpace(req.Query) == "" {
		return req, searxngURL, jsonOut, errors.New("query is required")
	}
	if req.Provider == "searxng" && strings.TrimSpace(searxngURL) == "" {
		return req, searxngURL, jsonOut, errors.New("--searxng-url is required when --provider searxng")
	}
	return req, searxngURL, jsonOut, nil
}

func parseWebGatherSearchAndFetchArgs(args []string) (modulewebgather.SearchAndFetchRequest, string, bool, error) {
	req := modulewebgather.SearchAndFetchRequest{
		Provider:        modulewebgather.DefaultSearchProvider,
		Limit:           modulewebgather.DefaultSearchLimit,
		MaxFetches:      modulewebgather.DefaultMaxFetches,
		Language:        modulewebgather.DefaultSearchLanguage,
		Freshness:       modulewebgather.DefaultSearchFreshness,
		Namespace:       "kb:research",
		FetchProvider:   modulewebgather.DefaultFetchProvider,
		Extractor:       modulewebgather.DefaultExtractor,
		StoreStaging:    true,
		StoreStagingSet: true,
		Policy:          modulewebgather.DefaultFetchPolicy(),
	}
	searxngURL := ""
	jsonOut := false
	querySet := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "--json":
			jsonOut = true
		case "--refresh":
			req.Refresh = true
		case "--no-store-staging":
			req.StoreStaging = false
			req.StoreStagingSet = true
		case "--provider", "--limit", "--max-fetches", "--language", "--freshness", "--namespace", "--searxng-url", "--fetch-provider", "--extractor", "--timeout-sec", "--max-body-bytes", "--max-redirects":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return req, searxngURL, jsonOut, fmt.Errorf("%s requires a value", arg)
			}
			value := strings.TrimSpace(args[i+1])
			i++
			switch arg {
			case "--provider":
				if !isAllowedWebGatherSearchProvider(value) {
					return req, searxngURL, jsonOut, fmt.Errorf("unsupported search provider: %s", value)
				}
				req.Provider = value
			case "--limit":
				n, err := strconv.Atoi(value)
				if err != nil || n <= 0 {
					return req, searxngURL, jsonOut, fmt.Errorf("invalid --limit: %s", value)
				}
				req.Limit = n
			case "--max-fetches":
				n, err := strconv.Atoi(value)
				if err != nil || n <= 0 {
					return req, searxngURL, jsonOut, fmt.Errorf("invalid --max-fetches: %s", value)
				}
				req.MaxFetches = n
			case "--language":
				req.Language = value
			case "--freshness":
				req.Freshness = value
			case "--namespace":
				req.Namespace = value
			case "--searxng-url":
				searxngURL = value
			case "--fetch-provider":
				if value != "http" {
					return req, searxngURL, jsonOut, fmt.Errorf("unsupported fetch provider for Phase 2: %s", value)
				}
				req.FetchProvider = value
			case "--extractor":
				if !isAllowedWebGatherExtractor(value) {
					return req, searxngURL, jsonOut, fmt.Errorf("unsupported extractor: %s", value)
				}
				req.Extractor = value
			case "--timeout-sec":
				sec, err := strconv.Atoi(value)
				if err != nil || sec <= 0 {
					return req, searxngURL, jsonOut, fmt.Errorf("invalid --timeout-sec: %s", value)
				}
				req.Policy.RequestTimeout = time.Duration(sec) * time.Second
			case "--max-body-bytes":
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil || n <= 0 {
					return req, searxngURL, jsonOut, fmt.Errorf("invalid --max-body-bytes: %s", value)
				}
				req.Policy.MaxBodyBytes = n
			case "--max-redirects":
				n, err := strconv.Atoi(value)
				if err != nil || n < 0 {
					return req, searxngURL, jsonOut, fmt.Errorf("invalid --max-redirects: %s", value)
				}
				req.Policy.MaxRedirects = n
			}
		default:
			if strings.HasPrefix(arg, "--") {
				return req, searxngURL, jsonOut, fmt.Errorf("unknown web-gather search-and-fetch option: %s", arg)
			}
			if querySet {
				return req, searxngURL, jsonOut, errors.New("web-gather search-and-fetch accepts exactly one query")
			}
			req.Query = arg
			querySet = true
		}
	}
	if strings.TrimSpace(req.Query) == "" {
		return req, searxngURL, jsonOut, errors.New("query is required")
	}
	if req.Provider == "searxng" && strings.TrimSpace(searxngURL) == "" {
		return req, searxngURL, jsonOut, errors.New("--searxng-url is required when --provider searxng")
	}
	return req, searxngURL, jsonOut, nil
}

func isAllowedWebGatherSearchProvider(value string) bool {
	switch strings.TrimSpace(value) {
	case "local_cache", "searxng":
		return true
	default:
		return false
	}
}

func parseWebGatherRegisterURLArgs(args []string) (conversationpersistence.L1SourceRegistryEntry, bool, error) {
	entry := conversationpersistence.L1SourceRegistryEntry{
		Kind:          conversationpersistence.L1SourceKindWebGather,
		TrustScore:    0.5,
		FetchInterval: time.Hour,
		LicenseNote:   modulewebgather.DefaultLicenseNote,
		Enabled:       true,
		Meta:          map[string]interface{}{"namespace": modulewebgather.DefaultNamespace},
	}
	jsonOut := false
	allowLocalhost := false
	urlSet := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "--json":
			jsonOut = true
		case "--disabled":
			entry.Enabled = false
		case "--allow-localhost":
			allowLocalhost = true
			entry.Meta["allow_localhost"] = true
		case "--source-id", "--namespace", "--trust-score", "--interval-sec", "--license-note", "--extractor", "--timeout-sec", "--max-body-bytes", "--max-redirects":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return entry, jsonOut, fmt.Errorf("%s requires a value", arg)
			}
			value := strings.TrimSpace(args[i+1])
			i++
			switch arg {
			case "--source-id":
				entry.SourceID = value
			case "--namespace":
				entry.Meta["namespace"] = value
			case "--trust-score":
				n, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return entry, jsonOut, fmt.Errorf("invalid --trust-score: %s", value)
				}
				entry.TrustScore = n
			case "--interval-sec":
				n, err := strconv.Atoi(value)
				if err != nil || n <= 0 {
					return entry, jsonOut, fmt.Errorf("invalid --interval-sec: %s", value)
				}
				entry.FetchInterval = time.Duration(n) * time.Second
			case "--license-note":
				entry.LicenseNote = value
			case "--extractor":
				if !isAllowedWebGatherExtractor(value) {
					return entry, jsonOut, fmt.Errorf("unsupported extractor: %s", value)
				}
				entry.Meta["extractor"] = value
			case "--timeout-sec":
				n, err := strconv.Atoi(value)
				if err != nil || n <= 0 {
					return entry, jsonOut, fmt.Errorf("invalid --timeout-sec: %s", value)
				}
				entry.Meta["request_timeout_ms"] = int64(n) * int64(time.Second/time.Millisecond)
			case "--max-body-bytes":
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil || n <= 0 {
					return entry, jsonOut, fmt.Errorf("invalid --max-body-bytes: %s", value)
				}
				entry.Meta["max_body_bytes"] = n
			case "--max-redirects":
				n, err := strconv.Atoi(value)
				if err != nil || n < 0 {
					return entry, jsonOut, fmt.Errorf("invalid --max-redirects: %s", value)
				}
				entry.Meta["max_redirects"] = int64(n)
			}
		default:
			if strings.HasPrefix(arg, "--") {
				return entry, jsonOut, fmt.Errorf("unknown web-gather register-url option: %s", arg)
			}
			if urlSet {
				return entry, jsonOut, errors.New("web-gather register-url accepts exactly one URL")
			}
			entry.URL = arg
			urlSet = true
		}
	}
	normalizedURL, err := modulewebgather.NormalizeURL(entry.URL, allowLocalhost)
	if err != nil {
		return entry, jsonOut, err
	}
	entry.URL = normalizedURL
	if strings.TrimSpace(entry.SourceID) == "" {
		entry.SourceID = modulewebgather.SourceIDFromURL(normalizedURL)
	}
	if strings.TrimSpace(entry.LicenseNote) == "" {
		return entry, jsonOut, errors.New("license-note is required")
	}
	return entry, jsonOut, nil
}

func loadWebGatherStore(configPath string) (*conversationpersistence.L1SQLiteStore, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	p := strings.TrimSpace(cfg.Conversation.L1SQLitePath)
	if p == "" {
		return nil, errors.New("conversation.l1_sqlite_path is required for web-gather CLI")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	return conversationpersistence.NewL1SQLiteStore(p)
}
