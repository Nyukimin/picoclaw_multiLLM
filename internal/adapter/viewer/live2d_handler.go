package viewer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// HandleLive2DCharacter serves Live2D HTML for characters
func HandleLive2DCharacter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	characterID := r.URL.Query().Get("character_id")
	if characterID == "" {
		characterID = "mio" // default to Mio
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "normal"
	}

	// Map character IDs to file paths
	var htmlPath string
	switch strings.ToLower(characterID) {
	case "mio":
		htmlPath = "internal/adapter/viewer/assets/images/mio/Mio_透過版.html"
	case "shiro":
		htmlPath = "internal/adapter/viewer/assets/images/shiro/Shiro_透過版.html"
	default:
		http.Error(w, "Unknown character", http.StatusNotFound)
		return
	}

	// Read HTML file
	content, err := os.ReadFile(htmlPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read Live2D file: %v", err), http.StatusInternalServerError)
		return
	}

	// Get emotion from query parameter
	emotion := r.URL.Query().Get("emotion")
	if emotion == "" {
		emotion = "normal"
	}

	// Check if UI should be hidden
	hideUI := r.URL.Query().Get("hide_ui") == "true"

	// Inject mode-specific styles
	htmlStr := string(content)
	if mode == "live" {
		// Large mode for live display
		htmlStr = injectLive2DStyle(htmlStr, `
body { margin: 0; padding: 0; overflow: hidden; }
canvas { width: 100vw !important; height: 100vh !important; }
`)
	} else {
		// Normal mode - responsive
		htmlStr = injectLive2DStyle(htmlStr, `
body { margin: 0; padding: 0; overflow: hidden; }
canvas {
	max-width: 100%;
	max-height: 100vh;
	width: auto !important;
	height: auto !important;
}
@media (max-width: 768px) {
	canvas {
		width: 100vw !important;
		height: auto !important;
	}
}
`)
	}

	// Inject UI hiding style if requested
	if hideUI {
		uiHideStyle := `
/* Hide Live2D UI controls */
#panel, .panel,
#live2d-widget, .live2d-controls, .live2d-ui, .controls,
button, input[type="range"], .slider, #control-panel,
.control, .menu, .toolbar, .settings,
#exprRow, #autoRow, #hideBtn, #followToggle, #breathToggle, #autoToggle, #greenToggle {
	display: none !important;
	visibility: hidden !important;
	opacity: 0 !important;
	pointer-events: none !important;
	width: 0 !important;
	height: 0 !important;
}
/* Ensure canvas is visible and interactive */
canvas {
	display: block !important;
	visibility: visible !important;
	pointer-events: auto !important;
	position: relative;
	z-index: 1;
}
body {
	overflow: hidden !important;
}
`
		htmlStr = injectLive2DStyle(htmlStr, uiHideStyle)
	}

	// Inject emotion control script
	htmlStr = injectLive2DScript(htmlStr, BuildLive2DControlScript(EmotionType(emotion)))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(htmlStr))
}

// HandleLive2DCharacterEmbed serves embedded Live2D for chat integration
func HandleLive2DCharacterEmbed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	characterID := r.URL.Query().Get("character_id")
	if characterID == "" {
		characterID = "mio"
	}

	emotion := r.URL.Query().Get("emotion")
	if emotion == "" {
		emotion = "normal"
	}

	mode := r.URL.Query().Get("mode")

	// Get Live2D state for emotion
	emotionType := EmotionType(emotion)
	state, ok := Live2DEmotionMapping[emotionType]
	if !ok {
		state = Live2DEmotionMapping[EmotionNormal]
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>%s - %s</title>
	<style>
		body {
			margin: 0;
			padding: 0;
			overflow: hidden;
			background: transparent;
		}
		#live2d-container {
			position: relative;
			width: 100%%;
			height: 100%%;
			z-index: 10;
		}
		iframe {
			border: none;
			width: 100%%;
			height: 100%%;
			pointer-events: auto;
		}
		/* Hide UI controls in iframe */
		iframe::after {
			content: '';
			display: block;
			position: absolute;
			bottom: 0;
			left: 0;
			right: 0;
			height: 100px;
			background: transparent;
			pointer-events: none;
		}
		%s
	</style>
</head>
<body>
	<div id="live2d-container">
		<iframe id="live2d-frame" src="/viewer/live2d/character?character_id=%s&mode=%s&emotion=%s&hide_ui=true" allowtransparency="true"></iframe>
	</div>
	<script>
		var currentEmotion = '%s';
		var currentState = %s;
		var frame = document.getElementById('live2d-frame');

		// Send emotion to iframe
		function setEmotion(emotion, state) {
			console.log('[Live2D Embed] Setting emotion:', emotion, state);
			currentEmotion = emotion;
			currentState = state;

			if (frame && frame.contentWindow) {
				frame.contentWindow.postMessage({
					type: 'emotion',
					emotion: emotion,
					state: state
				}, '*');
			}
		}

		// Listen for emotion changes from parent
		window.addEventListener('message', function(event) {
			if (event.data.type === 'emotion') {
				console.log('[Live2D Embed] Received emotion change from parent:', event.data);
				setEmotion(event.data.emotion, event.data.state || currentState);
			}
		});

		// Send initial emotion after iframe loads
		frame.addEventListener('load', function() {
			console.log('[Live2D Embed] iframe loaded, sending initial emotion');
			setTimeout(function() {
				setEmotion(currentEmotion, currentState);
			}, 1000); // Wait for Live2D to initialize
		});

		// Expose API
		window.live2dEmbed = {
			setEmotion: setEmotion,
			getCurrentEmotion: function() { return currentEmotion; }
		};

		console.log('[Live2D Embed] Initialized with emotion:', currentEmotion);
	</script>
</body>
</html>`, strings.ToUpper(string(characterID[0]))+characterID[1:], emotion, getModeStyle(mode), characterID, mode, emotion, emotion, toJSON(state))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// HandleLive2DAsset serves Live2D model assets
func HandleLive2DAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	characterID := r.URL.Query().Get("character_id")
	assetPath := r.URL.Query().Get("path")

	if characterID == "" || assetPath == "" {
		http.Error(w, "character_id and path are required", http.StatusBadRequest)
		return
	}

	// Security: prevent directory traversal
	if strings.Contains(assetPath, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	basePath := fmt.Sprintf("internal/adapter/viewer/assets/images/%s", characterID)
	fullPath := filepath.Join(basePath, assetPath)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, fullPath)
}

func injectLive2DStyle(html, style string) string {
	// Try to inject style before </head>
	if idx := strings.Index(html, "</head>"); idx != -1 {
		return html[:idx] + "<style>" + style + "</style>" + html[idx:]
	}
	// Fallback: inject at the beginning
	return "<style>" + style + "</style>" + html
}

func injectLive2DScript(html, script string) string {
	// Try to inject script before </body>
	if idx := strings.Index(html, "</body>"); idx != -1 {
		return html[:idx] + script + html[idx:]
	}
	// Fallback: append at the end
	return html + script
}

func getModeStyle(mode string) string {
	if mode == "live" {
		return `
#live2d-container {
	position: fixed;
	top: 0;
	left: 0;
	width: 100vw;
	height: 100vh;
	z-index: 1000;
}
iframe {
	width: 100%;
	height: 100%;
}
`
	}
	return `
#live2d-container {
	width: 300px;
	height: 400px;
}
@media (max-width: 768px) {
	#live2d-container {
		width: 200px;
		height: 300px;
	}
}
`
}

// HandleLive2DChat serves the Live2D chat UI
func HandleLive2DChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	htmlPath := "internal/adapter/viewer/assets/live2d_chat.html"
	content, err := os.ReadFile(htmlPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read chat UI: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

// HandleLive2DChatAPI handles chat API requests with emotion detection
func HandleLive2DChatAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message     string `json:"message"`
		CharacterID string `json:"character_id"`
		Mode        string `json:"mode"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.CharacterID == "" {
		req.CharacterID = "mio"
	}

	// TODO: Call actual LLM service for response
	// For now, echo back with emotion detection
	responseMessage := fmt.Sprintf("ご質問ありがとうございます。「%s」について考えてみますね。", req.Message)

	resp := BuildChatResponse(responseMessage, req.CharacterID, req.Mode)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
