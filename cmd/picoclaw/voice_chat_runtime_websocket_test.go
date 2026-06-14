package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	modulevoicechat "github.com/Nyukimin/picoclaw_multiLLM/modules/voicechat"
	"golang.org/x/net/websocket"
)

func TestInferVoiceChatGatewayURL_PrioritizesExplicitGateway(t *testing.T) {
	t.Setenv("VOICE_CHAT_GATEWAY_URL", " ws://192.168.1.207:8081/v1/chat/audio/sessions ")
	t.Setenv("RENCROW_LLM_CHAT_WS", "ws://ignored/v1/chat/audio/sessions")
	got := inferVoiceChatGatewayURL(&config.Config{})
	want := "ws://192.168.1.207:8081/v1/chat/audio/sessions"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestInferVoiceChatGatewayURL_FallsBackToChatBaseURL(t *testing.T) {
	t.Setenv("VOICE_CHAT_GATEWAY_URL", "")
	t.Setenv("RENCROW_LLM_CHAT_WS", "")
	cfg := &config.Config{}
	cfg.LocalLLM.ChatBaseURL = "http://192.168.1.207:8081"
	got := inferVoiceChatGatewayURL(cfg)
	want := "ws://192.168.1.207:8081/v1/chat/audio/sessions"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestVoiceChatEnabledFromEnv_DefaultFalse(t *testing.T) {
	t.Setenv("VOICE_CHAT_ENABLED", "")
	if voiceChatEnabledFromEnv() {
		t.Fatal("expected voice chat disabled by default")
	}
}

func TestRegisterVoiceChatRoutes_RegistersPrimaryAndAliasPaths(t *testing.T) {
	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	registerVoiceChatRoutes(mux, handler)
	for _, path := range modulevoicechat.WebSocketRoutePaths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("path %s expected %d, got %d", path, http.StatusNoContent, rec.Code)
		}
	}
}

func TestVoiceChatWebSocketBridgeE2E_RelaysStartPCMCommitAndFinalWithoutClosing(t *testing.T) {
	pcm := rawPCM16Chunk()
	gatewayDone := make(chan error, 1)
	gateway := httptest.NewServer(websocket.Handler(func(conn *websocket.Conn) {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var start string
		if err := websocket.Message.Receive(conn, &start); err != nil {
			gatewayDone <- err
			return
		}
		if !strings.Contains(start, `"type":"session.start"`) || !strings.Contains(start, `"format":"pcm16le"`) {
			gatewayDone <- fmt.Errorf("unexpected start control: %s", start)
			return
		}
		var gotPCM []byte
		if err := websocket.Message.Receive(conn, &gotPCM); err != nil {
			gatewayDone <- err
			return
		}
		if string(gotPCM) != string(pcm) {
			gatewayDone <- fmt.Errorf("unexpected pcm chunk: got %d bytes", len(gotPCM))
			return
		}
		var commit string
		if err := websocket.Message.Receive(conn, &commit); err != nil {
			gatewayDone <- err
			return
		}
		if !strings.Contains(commit, `"type":"session.commit"`) {
			gatewayDone <- fmt.Errorf("unexpected commit control: %s", commit)
			return
		}
		if err := websocket.Message.Send(conn, `{"type":"session.ready","utterance_id":"utt-1","session_id":"sess-1"}`); err != nil {
			gatewayDone <- err
			return
		}
		if err := websocket.Message.Send(conn, `{"type":"llm.delta","utterance_id":"utt-1","session_id":"sess-1","seq":1,"text":"お"}`); err != nil {
			gatewayDone <- err
			return
		}
		if err := websocket.Message.Send(conn, `{"type":"llm.final","utterance_id":"utt-1","session_id":"sess-1","text":"おはよう"}`); err != nil {
			gatewayDone <- err
			return
		}
		gatewayDone <- nil
	}))
	defer gateway.Close()

	mux := http.NewServeMux()
	gatewayURL := "ws" + strings.TrimPrefix(gateway.URL, "http")
	registerVoiceChatRoutes(mux, handleVoiceChatWebSocketBridge(gatewayURL, nil))
	bridge := httptest.NewServer(mux)
	defer bridge.Close()

	conn, err := websocket.Dial("ws"+strings.TrimPrefix(bridge.URL, "http")+modulevoicechat.RoutePathPrimary, "", "http://localhost/")
	if err != nil {
		t.Fatalf("dial bridge websocket: %v", err)
	}
	defer conn.Close()

	if err := websocket.Message.Send(conn, `{"type":"session.start","utterance_id":"utt-1","sample_rate":16000,"channels":1,"format":"pcm16le","model":"Chat"}`); err != nil {
		t.Fatalf("send start: %v", err)
	}
	if err := websocket.Message.Send(conn, pcm); err != nil {
		t.Fatalf("send pcm: %v", err)
	}
	if err := websocket.Message.Send(conn, `{"type":"session.commit","utterance_id":"utt-1"}`); err != nil {
		t.Fatalf("send commit: %v", err)
	}

	var ready string
	if err := websocket.Message.Receive(conn, &ready); err != nil {
		t.Fatalf("receive ready: %v", err)
	}
	if !strings.Contains(ready, `"type":"session.ready"`) {
		t.Fatalf("unexpected ready event: %s", ready)
	}
	var delta string
	if err := websocket.Message.Receive(conn, &delta); err != nil {
		t.Fatalf("receive delta: %v", err)
	}
	if !strings.Contains(delta, `"type":"llm.delta"`) || !strings.Contains(delta, `"text":"お"`) {
		t.Fatalf("unexpected delta event: %s", delta)
	}
	var final string
	if err := websocket.Message.Receive(conn, &final); err != nil {
		t.Fatalf("receive final: %v", err)
	}
	if !strings.Contains(final, `"type":"llm.final"`) || !strings.Contains(final, `"text":"おはよう"`) {
		t.Fatalf("unexpected final event: %s", final)
	}
	if err := <-gatewayDone; err != nil {
		t.Fatalf("gateway relay: %v", err)
	}
}

func TestVoiceChatInputAudioBridgeE2E_PostsWAVAndReturnsFinal(t *testing.T) {
	pcm := rawPCM16Chunk()
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		rawMessages, _ := payload["messages"].([]any)
		if len(rawMessages) != 1 {
			t.Fatalf("messages = %#v", payload["messages"])
		}
		msg, _ := rawMessages[0].(map[string]any)
		content, _ := msg["content"].([]any)
		if len(content) != 2 {
			t.Fatalf("content = %#v", msg["content"])
		}
		audioPart, _ := content[1].(map[string]any)
		inputAudio, _ := audioPart["input_audio"].(map[string]any)
		data, _ := inputAudio["data"].(string)
		if data == "" {
			t.Fatal("missing input_audio data")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"音声を確認しました"}}]}`))
	}))
	defer llm.Close()

	mux := http.NewServeMux()
	registerVoiceChatRoutes(mux, handleVoiceChatInputAudioBridge("ws"+strings.TrimPrefix(llm.URL, "http")+"/v1/chat/audio/sessions", nil))
	bridge := httptest.NewServer(mux)
	defer bridge.Close()

	conn, err := websocket.Dial("ws"+strings.TrimPrefix(bridge.URL, "http")+modulevoicechat.RoutePathPrimary, "", "http://localhost/")
	if err != nil {
		t.Fatalf("dial bridge websocket: %v", err)
	}
	defer conn.Close()

	if err := websocket.Message.Send(conn, `{"type":"session.start","utterance_id":"utt-1","sample_rate":16000,"channels":1,"format":"pcm16le","channel":"viewer","prompt":"短く確認"}`); err != nil {
		t.Fatalf("send start: %v", err)
	}
	var ready string
	if err := websocket.Message.Receive(conn, &ready); err != nil {
		t.Fatalf("receive ready: %v", err)
	}
	if !strings.Contains(ready, `"type":"session.ready"`) {
		t.Fatalf("unexpected ready event: %s", ready)
	}
	if err := websocket.Message.Send(conn, pcm); err != nil {
		t.Fatalf("send pcm: %v", err)
	}
	if err := websocket.Message.Send(conn, `{"type":"session.commit","utterance_id":"utt-1"}`); err != nil {
		t.Fatalf("send commit: %v", err)
	}
	var delta string
	if err := websocket.Message.Receive(conn, &delta); err != nil {
		t.Fatalf("receive delta: %v", err)
	}
	if !strings.Contains(delta, `"type":"llm.delta"`) || !strings.Contains(delta, `"text":"音声を確認しました"`) {
		t.Fatalf("unexpected delta event: %s", delta)
	}
	var final string
	if err := websocket.Message.Receive(conn, &final); err != nil {
		t.Fatalf("receive final: %v", err)
	}
	if !strings.Contains(final, `"type":"llm.final"`) || !strings.Contains(final, `"text":"音声を確認しました"`) {
		t.Fatalf("unexpected final event: %s", final)
	}
}

func TestVoiceChatDisabledHandlerReturnsErrorFrame(t *testing.T) {
	mux := http.NewServeMux()
	registerVoiceChatRoutes(mux, handleVoiceChatDisabled())
	server := httptest.NewServer(mux)
	defer server.Close()

	conn, err := websocket.Dial("ws"+strings.TrimPrefix(server.URL, "http")+modulevoicechat.RoutePathPrimary, "", "http://localhost/")
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	var msg string
	if err := websocket.Message.Receive(conn, &msg); err != nil {
		t.Fatalf("receive error frame: %v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(msg), &payload); err != nil {
		t.Fatalf("decode error frame: %v", err)
	}
	if payload["type"] != modulevoicechat.EventError || payload["error_code"] != modulevoicechat.ErrorVoiceChatDisabled {
		t.Fatalf("unexpected error frame: %s", msg)
	}
}

func TestIsVoiceChatTextFramePayload(t *testing.T) {
	if !isVoiceChatTextFramePayload([]byte(`{"type":"session.ready"}`)) {
		t.Fatal("json object should be relayed as text")
	}
	if isVoiceChatTextFramePayload(rawPCM16Chunk()) {
		t.Fatal("pcm16 audio should be relayed as binary")
	}
}
