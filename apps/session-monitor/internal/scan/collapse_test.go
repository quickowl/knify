package scan

import (
	"testing"

	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func TestCollapseSessionsByProviderIDKeepsNewestRow(t *testing.T) {
	sessions := collapseSessionsByProviderID([]types.LocalSession{
		{
			Provider:       "codex",
			SessionID:      "same-session",
			Title:          "Older title",
			CWD:            "/tmp/demo",
			Status:         "recorded",
			SourcePath:     "/tmp/older.jsonl",
			UpdatedAt:      "2026-04-29T10:00:00Z",
			LatestMessages: []types.MessageSummary{{Role: "assistant", Text: "older"}},
			Metadata:       map[string]string{},
		},
		{
			Provider:       "codex",
			SessionID:      "same-session",
			Title:          "Newer title",
			CWD:            "/tmp/demo",
			Status:         "recorded",
			SourcePath:     "/tmp/newer.jsonl",
			UpdatedAt:      "2026-04-29T10:05:00Z",
			LatestMessages: []types.MessageSummary{{Role: "assistant", Text: "newer"}},
			Metadata:       map[string]string{},
		},
	})
	if len(sessions) != 1 {
		t.Fatalf("expected one collapsed session, got %#v", sessions)
	}
	got := sessions[0]
	if got.UpdatedAt != "2026-04-29T10:05:00Z" || got.LatestMessages[0].Text != "newer" || got.Metadata["collapsedFiles"] != "2" {
		t.Fatalf("unexpected collapsed session: %#v", got)
	}
}
