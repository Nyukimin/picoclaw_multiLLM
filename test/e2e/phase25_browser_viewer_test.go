//go:build e2e

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestE2E_Phase25BrowserViewerSessionContract(t *testing.T) {
	if os.Getenv("PICOCLAW_BROWSER_E2E") != "1" {
		t.Skip("set PICOCLAW_BROWSER_E2E=1 to verify Viewer in a real browser session")
	}
	repoRoot := findRepoRoot(t)
	playwrightBin := filepath.Join(repoRoot, "node_modules", ".bin", "playwright")
	if _, err := os.Stat(playwrightBin); err != nil {
		t.Skipf("playwright binary is not available at %s: %v", playwrightBin, err)
	}

	baseURL := strings.TrimRight(os.Getenv("PICOCLAW_LIVE_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:18790"
	}
	message := "Phase25 browser e2e " + time.Now().UTC().Format("20060102T150405Z")

	script := `const { chromium } = require('playwright');
const baseURL = process.env.PICOCLAW_LIVE_BASE_URL || 'http://127.0.0.1:18790';
const message = process.env.PHASE25_BROWSER_MESSAGE;
(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1366, height: 900 } });
  try {
    await page.goto(baseURL + '/viewer', { waitUntil: 'domcontentloaded', timeout: 15000 });
    await page.waitForSelector('#inp', { timeout: 10000 });
    for (const selector of ['#chat', '#opsFeedBody', '#idleLiveLog', '#ttsNowPlaying', '#lipSyncMio', '#lipSyncShiro', '#micBtn', '#idleStart']) {
      await page.waitForSelector(selector, { state: 'attached', timeout: 5000 });
    }
    const micVisible = await page.locator('#micBtn').isVisible();
    const idleVisible = await page.locator('#idleStart').isVisible();
    if (!micVisible || !idleVisible) {
      throw new Error('normal chat mic and IdleChat controls must both be visible');
    }
    const sendResponse = page.waitForResponse(resp => resp.url().includes('/viewer/send') && resp.status() === 200, { timeout: 15000 });
    await page.fill('#inp', message);
    await page.click('#sendBtn');
    await sendResponse;
    await page.waitForFunction((expected) => {
      const chat = document.querySelector('#chat')?.innerText || '';
      const ops = document.querySelector('#opsFeedBody')?.innerText || '';
      return chat.includes(expected) || ops.includes(expected);
    }, message, { timeout: 30000 });
  } finally {
    await browser.close();
  }
})().catch(err => {
  console.error(err);
  process.exit(1);
});`

	scriptPath := filepath.Join(t.TempDir(), "phase25_viewer_browser_e2e.js")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write playwright script: %v", err)
	}

	cmd := exec.Command("node", scriptPath)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"NODE_PATH="+filepath.Join(repoRoot, "node_modules"),
		"PICOCLAW_LIVE_BASE_URL="+baseURL,
		"PHASE25_BROWSER_MESSAGE="+message,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser viewer e2e failed: %v\n%s", err, out)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found from %s", wd)
		}
	}
}
