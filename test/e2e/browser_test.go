//go:build e2e

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2E_BrowserViewerTabContract(t *testing.T) {
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

	script := `const { chromium } = require('playwright');
const baseURL = process.env.PICOCLAW_LIVE_BASE_URL || 'http://127.0.0.1:18790';
(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1366, height: 900 } });
  try {
    const health = await page.request.get(baseURL + '/health', { timeout: 5000 });
    if (health.status() !== 200) {
      throw new Error('live /health status=' + health.status());
    }
    await page.goto(baseURL + '/viewer', { waitUntil: 'domcontentloaded', timeout: 15000 });
    for (const selector of [
      '.tab-btn[data-tab="home"]',
      '.tab-btn[data-tab="timeline"]',
      '.tab-btn[data-tab="idlechat"]',
      '#inp',
      '#sendBtn',
      '#micBtn',
      '#chat',
      '#idleStart',
      '#idleLiveLog',
      '#lipSyncMio',
      '#lipSyncShiro'
    ]) {
      await page.waitForSelector(selector, { state: 'attached', timeout: 10000 });
    }

    await page.click('.tab-btn[data-tab="timeline"]');
    await page.waitForSelector('#chat', { state: 'visible', timeout: 10000 });
    if (!(await page.locator('#inp').isVisible())) {
      throw new Error('chat input must be visible on Chat tab');
    }
    if (!(await page.locator('#micBtn').isVisible())) {
      throw new Error('normal chat mic must be visible on Chat tab');
    }

    await page.click('.tab-btn[data-tab="idlechat"]');
    await page.waitForSelector('#idleStart', { state: 'visible', timeout: 10000 });
    if (!(await page.locator('#idleLiveLog').isVisible())) {
      throw new Error('IdleChat live log must be visible on IdleChat tab');
    }
    if (!(await page.locator('#idleStart').isVisible())) {
      throw new Error('IdleChat start control must be visible on IdleChat tab');
    }

    await page.click('.tab-btn[data-tab="timeline"]');
    await page.waitForSelector('#chat', { state: 'visible', timeout: 10000 });
  } finally {
    await browser.close();
  }
})().catch(err => {
  console.error(err);
  process.exit(1);
});`

	scriptPath := filepath.Join(t.TempDir(), "viewer_tab_contract_e2e.js")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write playwright script: %v", err)
	}

	cmd := exec.Command("node", scriptPath)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"NODE_PATH="+filepath.Join(repoRoot, "node_modules"),
		"PICOCLAW_LIVE_BASE_URL="+baseURL,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser viewer tab e2e failed: %v\n%s", err, out)
	}
}
