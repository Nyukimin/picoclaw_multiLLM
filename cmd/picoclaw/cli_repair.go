package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type repairCLIRunRequest struct {
	Reason      string `json:"reason"`
	Instruction string `json:"instruction"`
	Recent      int    `json:"recent"`
	TargetRoute string `json:"target_route"`
	TargetAgent string `json:"target_agent"`
}

type repairCLIRunResponse struct {
	OK      bool   `json:"ok"`
	JobID   string `json:"job_id"`
	Reason  string `json:"reason"`
	Summary string `json:"summary"`
}

func handleRepairCLISlashCommand(ctx context.Context, client *http.Client, baseURL, line string, out, errOut io.Writer) bool {
	cmd, instruction, ok := parseRepairCLISlashCommand(line)
	if !ok {
		return false
	}
	if cmd == "help" {
		fmt.Fprintln(out, "repair: /repair [修復指示]")
		return true
	}
	if instruction == "" {
		instruction = "直近ログを見て、Chat経路の異常を診断し、修復してください。"
	}
	resp, err := sendRepairCLIRun(ctx, client, baseURL, repairCLIRunRequest{
		Reason:      "user-directed-repair",
		Instruction: instruction,
		Recent:      100,
		TargetRoute: "CHAT",
		TargetAgent: "mio",
	})
	if err != nil {
		fmt.Fprintf(errOut, "repair request failed: %v\n", err)
		return true
	}
	fmt.Fprintf(out, "[repair] requested job_id=%s reason=%s\n%s\n", resp.JobID, resp.Reason, resp.Summary)
	return true
}

func parseRepairCLISlashCommand(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", false
	}
	fields := strings.Fields(line)
	if len(fields) == 0 || fields[0] != "/repair" {
		return "", "", false
	}
	if len(fields) > 1 && fields[1] == "help" {
		return "help", strings.TrimSpace(strings.TrimPrefix(line, "/repair help")), true
	}
	return "run", strings.TrimSpace(strings.TrimPrefix(line, "/repair")), true
}

func sendRepairCLIRun(ctx context.Context, client *http.Client, baseURL string, payload repairCLIRunRequest) (repairCLIRunResponse, error) {
	if client == nil {
		client = http.DefaultClient
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return repairCLIRunResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeDocsCLIBaseURL(baseURL)+"/viewer/repair/run", bytes.NewReader(body))
	if err != nil {
		return repairCLIRunResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return repairCLIRunResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		text, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return repairCLIRunResponse{}, fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(text)))
	}
	var out repairCLIRunResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 1024*1024)).Decode(&out); err != nil {
		return repairCLIRunResponse{}, fmt.Errorf("decode repair response: %w", err)
	}
	return out, nil
}
