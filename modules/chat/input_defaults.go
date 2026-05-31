package chat

import "strings"

const (
	DefaultViewerChannel = "viewer"
	DefaultViewerUserID  = "viewer-user"
)

func NormalizeInput(input Input) Input {
	input.Channel = DefaultString(input.Channel, DefaultViewerChannel)
	input.UserID = DefaultString(input.UserID, DefaultViewerUserID)
	return input
}

func DefaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
