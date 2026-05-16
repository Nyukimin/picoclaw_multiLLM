package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func sttInferViaHTTP(providerURL string, wav []byte, timeout time.Duration) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	part, err := w.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(wav); err != nil {
		return "", err
	}
	if err := w.WriteField("response_format", "json"); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, providerURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return out.Text, nil
}

func adjustAdaptiveSTTTimeout(cur, delta, minV, maxV time.Duration) time.Duration {
	next := cur + delta
	if next < minV {
		return minV
	}
	if next > maxV {
		return maxV
	}
	return next
}

func sttHTTPTimeoutFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("STT_TIMEOUT_MS"))
	if raw == "" {
		return 3000 * time.Millisecond
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 300 {
		return 3000 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

func isSTTTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "client.timeout exceeded") || strings.Contains(msg, "context deadline exceeded")
}
