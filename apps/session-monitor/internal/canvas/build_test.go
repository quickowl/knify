package canvas

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/scan"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func TestBuildCanvasCapsTranscriptText(t *testing.T) {
	longText := strings.Repeat("x", scan.MessageTextLimit*3)
	session := types.LocalSession{
		Provider:  "codex",
		SessionID: "thread-1",
		Title:     "Long transcript",
		CWD:       "/tmp/demo",
		Status:    "recorded",
		UpdatedAt: "2026-04-29T12:00:00Z",
		LatestMessages: []types.MessageSummary{
			{Role: "user", Text: core.SummarizeText(longText, scan.MessageTextLimit)},
		},
		Match: types.MatchResult{Status: "unmatched", Reason: "no match"},
	}
	cfg := types.Config{
		CanvasID:    types.DefaultCanvasID,
		RunID:       types.DefaultRunID,
		CodexHome:   "/tmp/codex",
		ClaudeHome:  "/tmp/claude",
		Lookback:    time.Hour,
		MaxSessions: 10,
	}
	canvas := BuildCanvas(cfg, []types.LocalSession{session}, nil, nil, types.HubState{}, 1, time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC))
	raw, err := json.Marshal(canvas)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), strings.Repeat("x", scan.MessageTextLimit+20)) {
		t.Fatalf("canvas leaked uncapped transcript text")
	}
	if !strings.Contains(string(raw), "Recent sessions (newest first)") || !strings.Contains(string(raw), "Long transcript") {
		t.Fatalf("canvas missing expected collection content: %s", raw)
	}
	if !strings.Contains(string(raw), "Purpose: Long transcript") || !strings.Contains(string(raw), "match unmatched") || !strings.Contains(string(raw), "Next:") {
		t.Fatalf("canvas missing recent session list metadata: %s", raw)
	}
}

func TestBuildCanvasRecentSessionsNewestFirst(t *testing.T) {
	older := types.LocalSession{
		Provider:       "codex",
		SessionID:      "older-session",
		Title:          "Older session",
		CWD:            "/tmp/demo",
		Status:         "recorded",
		UpdatedAt:      "2026-04-29T10:00:00Z",
		LatestMessages: []types.MessageSummary{{Role: "assistant", Text: "older message"}},
		Match:          types.MatchResult{Status: "unmatched"},
	}
	newer := types.LocalSession{
		Provider:       "claude",
		SessionID:      "newer-session",
		Title:          "Newer session",
		CWD:            "/tmp/demo",
		Status:         "idle",
		UpdatedAt:      "2026-04-29T11:55:00Z",
		LatestMessages: []types.MessageSummary{{Role: "assistant", Text: "newer message"}},
		Match:          types.MatchResult{Status: "likely", CanvasID: "canvas-newer", RunID: "run-newer"},
	}
	cfg := types.Config{
		CanvasID:    types.DefaultCanvasID,
		RunID:       types.DefaultRunID,
		CodexHome:   "/tmp/codex",
		ClaudeHome:  "/tmp/claude",
		Lookback:    time.Hour,
		MaxSessions: 10,
	}
	canvas := BuildCanvas(cfg, []types.LocalSession{older, newer}, nil, nil, types.HubState{}, 1, time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC))
	collection := blockByID(t, canvas, "session-monitor-collection")
	if collection["mode"] != "paged-list" {
		t.Fatalf("expected paged-list collection mode, got %#v", collection["mode"])
	}
	items, ok := collection["items"].([]map[string]any)
	if !ok {
		t.Fatalf("collection items have unexpected shape: %#v", collection["items"])
	}
	if len(items) != 2 || !strings.Contains(core.StringValue(items[0]["label"]), "Newer session") || !strings.Contains(core.StringValue(items[1]["label"]), "Older session") {
		t.Fatalf("recent session collection not newest-first: %#v", items)
	}
	subtitle := core.StringValue(items[0]["subtitle"])
	for _, want := range []string{"Purpose: Newer session", "Now: idle, active; 5m ago; match likely; assistant: newer message", "Next: Confirm the likely canvas/run link"} {
		if !strings.Contains(subtitle, want) {
			t.Fatalf("recent session subtitle missing %q:\n%s", want, subtitle)
		}
	}
	badges, ok := items[0]["badges"].([]string)
	if !ok || strings.Join(badges, "|") != "claude|idle|active|likely" {
		t.Fatalf("unexpected collection badges: %#v", items[0]["badges"])
	}
}

func blockByID(t *testing.T, canvas types.Canvas, id string) map[string]any {
	t.Helper()
	for _, block := range canvas.Blocks {
		if block["id"] == id {
			return block
		}
	}
	t.Fatalf("block %s not found", id)
	return nil
}
