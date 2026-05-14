package hub

import (
	"strings"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
)

var legacyCollectionItemKeys = map[string]bool{
	"id":           true,
	"label":        true,
	"subtitle":     true,
	"status":       true,
	"badges":       true,
	"addedAt":      true,
	"blockIds":     true,
	"thumbnailUrl": true,
}

func legacyCollectionItemError(data []byte) bool {
	text := strings.ToLower(string(data))
	if strings.Contains(text, "unsupported mode") && strings.Contains(text, "paged-list") {
		return true
	}
	if !strings.Contains(text, "unknown field") {
		return false
	}
	for _, field := range []string{"sessionid", "attention", "updatedat", "purpose", "currentstate", "evidencestatus", "nextstep", "artifactcount", "planlabel"} {
		if strings.Contains(text, field) {
			return true
		}
	}
	return false
}

func stripCollectionItemExtensions(blocks []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, stripBlockCollectionItemExtensions(block))
	}
	return out
}

func stripBlockCollectionItemExtensions(block map[string]any) map[string]any {
	if core.StringValue(block["kind"]) != "collection" {
		return cloneBlock(block)
	}
	next := cloneBlock(block)
	next["mode"] = "paged-grid-rail"
	items, ok := next["items"].([]any)
	if !ok {
		return next
	}
	stripped := make([]any, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			stripped = append(stripped, raw)
			continue
		}
		legacy := map[string]any{}
		for key, value := range item {
			if legacyCollectionItemKeys[key] {
				legacy[key] = value
			}
		}
		stripped = append(stripped, legacy)
	}
	next["items"] = stripped
	return next
}
