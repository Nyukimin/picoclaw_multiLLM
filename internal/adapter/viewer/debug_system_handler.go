package viewer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type DebugSystemSnapshot struct {
	UpdatedAt string             `json:"updated_at"`
	GPU       DebugGPUSnapshot   `json:"gpu"`
	Audio     DebugAudioSnapshot `json:"audio"`
}

type DebugGPUSnapshot struct {
	Available   bool              `json:"available"`
	TotalMB     int               `json:"total_mb,omitempty"`
	UsedMB      int               `json:"used_mb,omitempty"`
	FreeMB      int               `json:"free_mb,omitempty"`
	LLMUsedMB   int               `json:"llm_used_mb,omitempty"`
	STTUsedMB   int               `json:"stt_used_mb,omitempty"`
	TTSUsedMB   int               `json:"tts_used_mb,omitempty"`
	OtherUsedMB int               `json:"other_used_mb,omitempty"`
	Processes   []DebugGPUProcess `json:"processes,omitempty"`
	Note        string            `json:"note,omitempty"`
}

type DebugGPUProcess struct {
	PID         int    `json:"pid,omitempty"`
	Name        string `json:"name,omitempty"`
	Category    string `json:"category,omitempty"`
	UsedMB      int    `json:"used_mb,omitempty"`
	CommandHint string `json:"command_hint,omitempty"`
}

type DebugSystemOptions struct {
	STTBaseURL    string
	STTStreamURL  string
	TTSBaseURL    string
	TTSHealthPath string
}

type RuntimeConfig struct {
	STTStreamURL string `json:"stt_stream_url,omitempty"`
	STTBaseURL   string `json:"stt_base_url,omitempty"`
}

func HandleRuntimeConfig(opts DebugSystemOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RuntimeConfig{
			STTStreamURL: strings.TrimSpace(opts.STTStreamURL),
			STTBaseURL:   strings.TrimRight(strings.TrimSpace(opts.STTBaseURL), "/"),
		})
	}
}

type DebugAudioSnapshot struct {
	STTBaseURL string `json:"stt_base_url,omitempty"`
	TTSBaseURL string `json:"tts_base_url,omitempty"`
	STTOK      bool   `json:"stt_ok"`
	TTSLiveOK  bool   `json:"tts_live_ok"`
	TTSReadyOK bool   `json:"tts_ready_ok"`
	STTHealth  string `json:"stt_health,omitempty"`
	TTSLive    string `json:"tts_live,omitempty"`
	TTSReady   string `json:"tts_ready,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}

func HandleDebugSystemSnapshot(opts DebugSystemOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s := DebugSystemSnapshot{
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			GPU:       collectGPUSnapshot(),
			Audio:     collectAudioSnapshot(opts),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s)
	}
}

func collectAudioSnapshot(opts DebugSystemOptions) DebugAudioSnapshot {
	out := DebugAudioSnapshot{
		STTBaseURL: strings.TrimRight(strings.TrimSpace(opts.STTBaseURL), "/"),
		TTSBaseURL: strings.TrimRight(strings.TrimSpace(opts.TTSBaseURL), "/"),
	}
	client := &http.Client{Timeout: 2 * time.Second}

	if out.STTBaseURL != "" {
		if body, ok, err := fetchEndpoint(client, out.STTBaseURL+"/health"); err != nil {
			out.LastError = appendError(out.LastError, "stt:"+err.Error())
		} else {
			out.STTHealth = body
			out.STTOK = ok
		}
	}
	if out.TTSBaseURL != "" {
		if strings.TrimSpace(opts.TTSHealthPath) != "" {
			body, ok, err := fetchEndpoint(client, out.TTSBaseURL+"/"+strings.TrimLeft(strings.TrimSpace(opts.TTSHealthPath), "/"))
			if err != nil {
				out.LastError = appendError(out.LastError, "tts:"+err.Error())
			} else {
				out.TTSLive = body
				out.TTSReady = body
				out.TTSLiveOK = ok
				out.TTSReadyOK = ok
			}
			return out
		}
		if body, ok, err := fetchEndpoint(client, out.TTSBaseURL+"/health/live"); err != nil {
			out.LastError = appendError(out.LastError, "tts_live:"+err.Error())
		} else {
			out.TTSLive = body
			out.TTSLiveOK = ok
		}
		if body, ok, err := fetchEndpoint(client, out.TTSBaseURL+"/health/ready"); err != nil {
			out.LastError = appendError(out.LastError, "tts_ready:"+err.Error())
		} else {
			out.TTSReady = body
			out.TTSReadyOK = ok
		}
	}
	return out
}

func fetchEndpoint(client *http.Client, endpoint string) (string, bool, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	body := strings.TrimSpace(string(bodyBytes))
	return body, resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

func appendError(cur, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return cur
	}
	if strings.TrimSpace(cur) == "" {
		return next
	}
	return cur + "; " + next
}

func collectGPUSnapshot() DebugGPUSnapshot {
	base, err := queryGPUMemoryTotals()
	if err != nil {
		return DebugGPUSnapshot{
			Available: false,
			Note:      err.Error(),
		}
	}

	procs, err := queryGPUProcesses()
	if err != nil {
		base.Available = true
		base.Note = err.Error()
		return base
	}
	base.Available = true
	base.Processes = procs
	for _, p := range procs {
		switch p.Category {
		case "llm":
			base.LLMUsedMB += p.UsedMB
		case "stt":
			base.STTUsedMB += p.UsedMB
		case "tts":
			base.TTSUsedMB += p.UsedMB
		default:
			base.OtherUsedMB += p.UsedMB
		}
	}
	return base
}

func queryGPUMemoryTotals() (DebugGPUSnapshot, error) {
	out, err := runCmd(2*time.Second, "nvidia-smi",
		"--query-gpu=memory.total,memory.used,memory.free",
		"--format=csv,noheader,nounits")
	if err != nil {
		return DebugGPUSnapshot{}, fmt.Errorf("nvidia-smi unavailable: %w", err)
	}
	line := firstNonEmptyLine(out)
	if line == "" {
		return DebugGPUSnapshot{}, fmt.Errorf("nvidia-smi returned empty output")
	}
	parts := splitCSVLine(line)
	if len(parts) < 3 {
		return DebugGPUSnapshot{}, fmt.Errorf("nvidia-smi parse error: %q", line)
	}
	total, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	used, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	free, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
	return DebugGPUSnapshot{
		TotalMB: total,
		UsedMB:  used,
		FreeMB:  free,
	}, nil
}

func queryGPUProcesses() ([]DebugGPUProcess, error) {
	out, err := runCmd(2*time.Second, "nvidia-smi",
		"--query-compute-apps=pid,process_name,used_gpu_memory",
		"--format=csv,noheader,nounits")
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi process query failed: %w", err)
	}
	lines := strings.Split(out, "\n")
	items := make([]DebugGPUProcess, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		parts := splitCSVLine(line)
		if len(parts) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		name := strings.TrimSpace(parts[1])
		usedMB, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
		hint := processCommandHint(pid)
		items = append(items, DebugGPUProcess{
			PID:         pid,
			Name:        name,
			UsedMB:      usedMB,
			Category:    classifyGPUProcess(name, hint),
			CommandHint: hint,
		})
	}
	return items, nil
}

func processCommandHint(pid int) string {
	if pid <= 0 {
		return ""
	}
	path := fmt.Sprintf("/proc/%d/cmdline", pid)
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return ""
	}
	raw := strings.ReplaceAll(string(b), "\x00", " ")
	return strings.TrimSpace(raw)
}

func classifyGPUProcess(name, hint string) string {
	text := strings.ToLower(strings.TrimSpace(name + " " + hint))
	switch {
	case strings.Contains(text, "ollama"), strings.Contains(text, "llama"), strings.Contains(text, "deepseek"), strings.Contains(text, "openai"), strings.Contains(text, "claude"):
		return "llm"
	case strings.Contains(text, "whisper"), strings.Contains(text, "stt"), strings.Contains(text, "speech-to-text"):
		return "stt"
	case strings.Contains(text, "tts"), strings.Contains(text, "vits"), strings.Contains(text, "sbv2"), strings.Contains(text, "style-bert"), strings.Contains(text, "voicevox"):
		return "tts"
	default:
		return "other"
	}
}

func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func firstNonEmptyLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			return t
		}
	}
	return ""
}

func splitCSVLine(line string) []string {
	parts := strings.Split(line, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
