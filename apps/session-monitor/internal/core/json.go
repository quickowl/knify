package core

import (
	"encoding/json"
	"strconv"
	"strings"
)

func MapValue(value any) map[string]any {
	if got, ok := value.(map[string]any); ok {
		return got
	}
	return nil
}

func StringValue(value any) string {
	switch got := value.(type) {
	case string:
		return got
	case json.Number:
		return got.String()
	default:
		return ""
	}
}

func NumberString(value any) string {
	switch got := value.(type) {
	case float64:
		return strconv.FormatInt(int64(got), 10)
	case json.Number:
		return got.String()
	case string:
		return got
	default:
		return ""
	}
}

func ExtractText(value any) string {
	switch got := value.(type) {
	case nil:
		return ""
	case string:
		return got
	case []any:
		parts := make([]string, 0, len(got))
		for _, item := range got {
			if text := ExtractText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	case map[string]any:
		if typ := StringValue(got["type"]); typ == "tool_use" || typ == "tool_result" || typ == "function_call" {
			return ""
		}
		for _, key := range []string{"text", "message"} {
			if text := StringValue(got[key]); text != "" {
				return text
			}
		}
		if content, ok := got["content"]; ok {
			return ExtractText(content)
		}
	}
	return ""
}
