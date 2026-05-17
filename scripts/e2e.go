package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	baseURL := flag.String("base-url", envOrDefault("PICOCLAW_LIVE_BASE_URL", "http://127.0.0.1:18790"), "RenCrow live service base URL")
	runBrowser := flag.Bool("browser", true, "run browser Viewer E2E")
	runLive := flag.Bool("live", true, "run live health/runtime E2E")
	flag.Parse()

	trimmedBaseURL := strings.TrimRight(*baseURL, "/")
	if err := waitHealth(trimmedBaseURL, 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "live service is not ready at %s: %v\n", trimmedBaseURL, err)
		os.Exit(1)
	}

	args := []string{"test", "-v", "-count=1", "-tags=e2e", "./test/e2e"}
	env := append(os.Environ(),
		"GOCACHE="+envOrDefault("GOCACHE", "/tmp/picoclaw-gocache"),
		"PICOCLAW_LIVE_BASE_URL="+trimmedBaseURL,
	)
	if *runBrowser {
		env = append(env, "PICOCLAW_BROWSER_E2E=1")
	}
	if *runLive {
		env = append(env, "PICOCLAW_LIVE_E2E=1")
	}

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "E2E tests failed: %v\n", err)
		os.Exit(1)
	}
}

func waitHealth(baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("status=%d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return lastErr
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
