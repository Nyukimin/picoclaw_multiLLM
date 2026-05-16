package tts

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseSynthesisError(body []byte) (string, string) {
	var out struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", ""
	}
	return normalizeErrorCode(out.Error.Code), strings.TrimSpace(out.Error.Message)
}

func invalidRequestError(message string) error {
	return fmt.Errorf("code=invalid_request message=%s", strings.TrimSpace(message))
}

func normalizeErrorCode(code string) string {
	code = strings.TrimSpace(strings.ToUpper(code))
	code = strings.ReplaceAll(code, "-", "_")
	return code
}
