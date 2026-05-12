package core

import (
	"strings"

	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func SummarizeText(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	return Truncate(text, limit)
}

func Truncate(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func LastMessages(messages []types.MessageSummary, limit int) []types.MessageSummary {
	if len(messages) <= limit {
		return messages
	}
	return append([]types.MessageSummary(nil), messages[len(messages)-limit:]...)
}
