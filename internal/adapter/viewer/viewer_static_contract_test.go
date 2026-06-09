package viewer

import (
	"os"
	"strings"
	"testing"
)

func TestViewerStaticContractSeparatesDisplayAudioLipsyncAndLogs(t *testing.T) {
	data, err := os.ReadFile("viewer.html")
	if err != nil {
		t.Fatalf("read viewer.html: %v", err)
	}
	html := string(data)

	required := map[string]string{
		`id="chat"`:                      "normal chat timeline display",
		`id="idleLiveLog"`:               "IdleChat live display",
		`id="idleSummaryReview"`:         "IdleChat summary/review display",
		`id="ttsNowPlaying"`:             "TTS playback status",
		`id="lipSyncMio"`:                "Mio lipsync state",
		`id="lipSyncShiro"`:              "Shiro lipsync state",
		`id="opsFeedBody"`:               "ops/event log",
		`id="toolHarnessBody"`:           "Tool Harness mediation event log",
		`id="dciTraceBody"`:              "DCI search trace log",
		`id="debugSttTrace"`:             "STT trace log",
		`id="sourceRegistryBody"`:        "Source Registry panel",
		`id="sourceRegistryStagingBody"`: "Source Registry staging review panel",
		`id="memoryLayerBody"`:           "Memory layer panel",
		`id="micBtn"`:                    "normal chat voice input control",
		`id="idleStart"`:                 "IdleChat control separated from mic input",
		`id="audioBtn"`:                  "browser audio enable control",
		`id="liveAudioBtn"`:              "live audio enable control",
		`id="sourceRegistrySaveBtn"`:     "Source Registry save control",
	}
	for needle, purpose := range required {
		if !strings.Contains(html, needle) {
			t.Fatalf("viewer.html missing %s (%s)", needle, purpose)
		}
	}

	micIndex := strings.Index(html, `id="micBtn"`)
	idleIndex := strings.Index(html, `id="idleStart"`)
	headerEnd := strings.Index(html, `</header>`)
	lipsyncIndex := strings.Index(html, `class="lipsync-stage"`)
	if micIndex < 0 || idleIndex < 0 {
		t.Fatal("mic and IdleChat controls must both be present")
	}
	if micIndex > idleIndex {
		t.Fatal("normal chat mic control should be in the normal input controls before IdleChat controls")
	}
	if headerEnd < 0 || lipsyncIndex < 0 || lipsyncIndex > headerEnd {
		t.Fatal("Mio/Shiro lipsync mini icons must be placed inside the top header band")
	}
}

func TestViewerStaticContractDailyDeskTabs(t *testing.T) {
	data, err := os.ReadFile("viewer.html")
	if err != nil {
		t.Fatalf("read viewer.html: %v", err)
	}
	html := string(data)

	required := map[string]string{
		`data-tab="home"`:                        "Home tab",
		`data-tab="develop"`:                     "Develop tab",
		`data-tab="instructions"`:                "Instructions tab",
		`data-tab="reports"`:                     "Reports tab",
		`data-tab="movie-db"`:                    "Movie Database tab",
		`id="panel-home" class="panel active"`:   "Home is the initial active panel",
		`id="panel-develop"`:                     "Develop panel",
		`id="panel-instructions"`:                "Instructions panel",
		`id="panel-reports"`:                     "Reports panel",
		`id="panel-movie-db"`:                    "Movie Database panel",
		`id="movieDbFetchKind"`:                  "Movie Database fetch kind selector",
		`id="movieDbFetchQuery"`:                 "Movie Database fetch query input",
		`id="movieDbFetchBtn"`:                   "Movie Database fetch action",
		`/viewer/assets/css/tabs/desk.css`:       "Daily Desk CSS",
		`/viewer/assets/js/tabs/home.js`:         "Home tab JavaScript",
		`/viewer/assets/js/tabs/develop.js`:      "Develop tab JavaScript",
		`/viewer/assets/js/tabs/instructions.js`: "Instructions tab JavaScript",
		`/viewer/assets/js/tabs/reports.js`:      "Reports tab JavaScript",
		`/viewer/assets/js/tabs/movie-db.js`:     "Movie Database tab JavaScript",
	}
	for needle, purpose := range required {
		if !strings.Contains(html, needle) {
			t.Fatalf("viewer.html missing %s (%s)", needle, purpose)
		}
	}

	if strings.Contains(html, `id="panel-overview" class="panel active"`) {
		t.Fatal("overview must not remain the initial active panel after Daily Desk addition")
	}
}

func TestViewerStaticContractChatAndIdleChatDeskRedesign(t *testing.T) {
	data, err := os.ReadFile("viewer.html")
	if err != nil {
		t.Fatalf("read viewer.html: %v", err)
	}
	html := string(data)

	required := map[string]string{
		`id="panel-timeline" class="panel chat-desk-panel"`: "Chat panel keeps tab contract",
		`class="chat-desk-shell"`:                           "Chat uses desk shell",
		`class="chat-character-pane"`:                       "Chat has large character pane",
		`class="chat-main-pane"`:                            "Chat has main conversation pane",
		`class="chat-intent-strip"`:                         "Chat shows compact routing/action guidance",
		`id="chat"`:                                        "Chat message render target remains stable",
		`id="panel-idlechat" class="panel idle-desk-panel"`: "IdleChat panel uses redesigned shell",
		`class="idle-desk-shell"`:                           "IdleChat uses desk shell",
		`class="idle-character-pane"`:                       "IdleChat has character/status pane",
		`class="idle-mode-board"`:                           "IdleChat mode controls are first-class controls",
		`id="idleLiveLog"`:                                  "IdleChat live render target remains stable",
		`id="idleSummaryReview"`:                            "IdleChat summary review remains stable",
		`id="idlechatBody"`:                                 "IdleChat history body remains stable",
	}
	for needle, purpose := range required {
		if !strings.Contains(html, needle) {
			t.Fatalf("viewer.html missing %s (%s)", needle, purpose)
		}
	}

	if strings.Contains(html, `<section id="panel-idlechat" class="panel">
    <div class="grid">`) {
		t.Fatal("IdleChat must not use the old generic grid-first layout")
	}
}

func TestViewerStaticContractMovieDatabaseTabSwitchMapping(t *testing.T) {
	data, err := os.ReadFile("assets/js/viewer.js")
	if err != nil {
		t.Fatalf("read viewer.js: %v", err)
	}
	js := string(data)
	if !strings.Contains(js, `'movie-db': document.getElementById('panel-movie-db')`) {
		t.Fatal("viewer.js missing Movie Database panel switch mapping")
	}
}

func TestViewerStaticContractChatScrollUsesConversationContainer(t *testing.T) {
	data, err := os.ReadFile("assets/js/viewer.js")
	if err != nil {
		t.Fatalf("read viewer.js: %v", err)
	}
	js := string(data)
	if !strings.Contains(js, "const target = chat || mainEl") {
		t.Fatal("Chat timeline auto-follow must scroll the chat container, not the whole main viewport")
	}
	if !strings.Contains(js, "if (mainEl) mainEl.scrollTop = 0") {
		t.Fatal("Tab switching must reset the main viewport to avoid clipped Viewer panels")
	}
}

func TestViewerStaticContractNowPlayingFloatsAboveComposer(t *testing.T) {
	data, err := os.ReadFile("assets/css/viewer.css")
	if err != nil {
		t.Fatalf("read viewer.css: %v", err)
	}
	css := string(data)

	required := []string{
		`.tts-now-playing{`,
		`bottom:calc(148px + var(--safe-bottom));z-index:41;pointer-events:none;`,
		`@media (orientation: landscape) and (max-height: 520px){`,
		`.tts-now-playing{max-width:min(76vw,760px);font-size:13px;bottom:calc(180px + var(--safe-bottom))}`,
		`@media (max-width: 640px){`,
		`.tts-now-playing{bottom:calc(180px + var(--safe-bottom))}`,
	}
	for _, needle := range required {
		if !strings.Contains(css, needle) {
			t.Fatalf("viewer.css missing %q", needle)
		}
	}
	if strings.Contains(css, `bottom:calc(96px + var(--safe-bottom));z-index:35;pointer-events:none;`) {
		t.Fatal("legacy Now Playing offset must not remain after the composer overlap fix")
	}
}

func TestViewerStaticContractHobbyGraphOpsOverview(t *testing.T) {
	viewerJS, err := os.ReadFile("assets/js/viewer.js")
	if err != nil {
		t.Fatalf("read viewer.js: %v", err)
	}
	opsJS, err := os.ReadFile("assets/js/tabs/ops.js")
	if err != nil {
		t.Fatalf("read ops.js: %v", err)
	}
	if !strings.Contains(string(viewerJS), "function refreshHobbyGraphOverviewData()") {
		t.Fatal("viewer.js missing Hobby Graph overview refresh function")
	}
	if !strings.Contains(string(viewerJS), "/viewer/hobby-graph?action=overview&limit=5") {
		t.Fatal("viewer.js missing Hobby Graph overview endpoint fetch")
	}
	if !strings.Contains(string(opsJS), "function hobbyGraphOpsCard()") {
		t.Fatal("ops.js missing Hobby Graph Ops card")
	}
	if !strings.Contains(string(opsJS), "hobbyGraphOpsCard()") {
		t.Fatal("ops.js missing Hobby Graph Ops card registration")
	}
}
