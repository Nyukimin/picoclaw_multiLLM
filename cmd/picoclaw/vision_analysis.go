package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	domainattachment "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/attachment"
)

type visionEventEmitter func(eventType, content string)

type visionAnalysisResult struct {
	Filename string
	Kind     domainattachment.Kind
	OK       bool
	Summary  string
	Text     string
	Model    string
	Error    string
}

func appendVisionAnalysisToMessage(
	ctx context.Context,
	cfg config.VisionConfig,
	message string,
	attachments []domainattachment.Attachment,
	emit visionEventEmitter,
) string {
	if !cfg.Enabled || strings.TrimSpace(cfg.BaseURL) == "" || len(attachments) == 0 {
		return message
	}
	var results []visionAnalysisResult
	for _, att := range attachments {
		if att.Kind != domainattachment.KindImage && att.Kind != domainattachment.KindVideo {
			continue
		}
		if emit != nil {
			emit("vision.analysis.start", fmt.Sprintf("%s:%s", att.Kind, att.Filename))
		}
		result := analyzeAttachmentWithVision(ctx, cfg, att, message)
		results = append(results, result)
		if emit != nil {
			if result.OK {
				emit("vision.analysis.completed", fmt.Sprintf("%s:%s", att.Kind, att.Filename))
			} else {
				emit("vision.analysis.failed", fmt.Sprintf("%s:%s %s", att.Kind, att.Filename, result.Error))
			}
		}
	}
	if len(results) == 0 {
		return message
	}
	analysis := formatVisionAnalysis(results)
	if strings.TrimSpace(message) == "" {
		return analysis
	}
	return strings.TrimRight(message, "\n") + "\n\n" + analysis
}

func analyzeAttachmentWithVision(ctx context.Context, cfg config.VisionConfig, att domainattachment.Attachment, prompt string) visionAnalysisResult {
	result := visionAnalysisResult{Filename: att.Filename, Kind: att.Kind}
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, contentType, err := buildVisionAnalyzeMultipart(cfg, att, prompt)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/" + strings.TrimLeft(cfg.EndpointPath, "/")
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("vision API status=%d body=%s", resp.StatusCode, compactVisionText(string(respBody), 500))
		return result
	}

	var payload struct {
		OK        bool   `json:"ok"`
		Kind      string `json:"kind"`
		Summary   string `json:"summary"`
		Text      string `json:"text"`
		Model     string `json:"model"`
		ErrorCode string `json:"error_code"`
		Error     any    `json:"error"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		result.Error = fmt.Sprintf("decode vision response: %v", err)
		return result
	}
	if !payload.OK {
		result.Error = compactVisionError(payload.ErrorCode, payload.Error)
		if result.Error == "" {
			result.Error = "vision response ok=false"
		}
		return result
	}
	result.OK = true
	result.Summary = payload.Summary
	result.Text = payload.Text
	result.Model = payload.Model
	if payload.Kind != "" {
		result.Kind = domainattachment.Kind(payload.Kind)
	}
	return result
}

func buildVisionAnalyzeMultipart(cfg config.VisionConfig, att domainattachment.Attachment, prompt string) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	fields := map[string]string{
		"prompt":        prompt,
		"kind":          string(att.Kind),
		"model":         cfg.ModelAlias,
		"output_format": cfg.OutputFormat,
		"language":      cfg.DefaultLanguage,
		"max_frames":    strconv.Itoa(cfg.MaxFrames),
	}
	for key, value := range fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}
	data := att.Data
	if len(data) == 0 && strings.TrimSpace(att.Path) != "" {
		fileData, err := osReadFile(att.Path)
		if err != nil {
			return nil, "", err
		}
		data = fileData
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("attachment %s has no data", att.Filename)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeMultipartQuote(filepath.Base(att.Filename))))
	if strings.TrimSpace(att.ContentType) != "" {
		header.Set("Content-Type", att.ContentType)
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), writer.FormDataContentType(), nil
}

var osReadFile = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func formatVisionAnalysis(results []visionAnalysisResult) string {
	var b strings.Builder
	b.WriteString("[Vision analysis]\n")
	for i, result := range results {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "file: %s\nkind: %s\n", result.Filename, result.Kind)
		if result.OK {
			if result.Model != "" {
				fmt.Fprintf(&b, "model: %s\n", result.Model)
			}
			if strings.TrimSpace(result.Summary) != "" {
				fmt.Fprintf(&b, "summary: %s\n", strings.TrimSpace(result.Summary))
			}
			if strings.TrimSpace(result.Text) != "" && strings.TrimSpace(result.Text) != strings.TrimSpace(result.Summary) {
				fmt.Fprintf(&b, "text: %s\n", strings.TrimSpace(result.Text))
			}
		} else {
			fmt.Fprintf(&b, "error: %s\n", strings.TrimSpace(result.Error))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func compactVisionError(code string, raw any) string {
	var parts []string
	if strings.TrimSpace(code) != "" {
		parts = append(parts, strings.TrimSpace(code))
	}
	switch v := raw.(type) {
	case string:
		parts = append(parts, v)
	case map[string]any:
		if msg, _ := v["message"].(string); msg != "" {
			parts = append(parts, msg)
		}
	default:
		if raw != nil {
			if data, err := json.Marshal(raw); err == nil {
				parts = append(parts, string(data))
			}
		}
	}
	return compactVisionText(strings.Join(parts, ": "), 500)
}

func compactVisionText(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if limit > 0 && len(text) > limit {
		return strings.TrimSpace(text[:limit])
	}
	return text
}

func escapeMultipartQuote(s string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(s)
}

func logVisionAnalysisConfig(cfg config.VisionConfig) {
	if cfg.Enabled {
		log.Printf("[Vision] analysis enabled base=%s endpoint=%s", cfg.BaseURL, cfg.EndpointPath)
	}
}
