package canvas

import (
	"encoding/json"
	"os"
	"path/filepath"
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
		CanvasID:   types.DefaultCanvasID,
		RunID:      types.DefaultRunID,
		CodexHome:  "/tmp/codex",
		ClaudeHome: "/tmp/claude",
		Build: types.DaemonBuildInfo{
			Version:          "v0.1.0",
			RevisionShort:    "abc123def456",
			Modified:         true,
			GoVersion:        "go1.24.1",
			BinaryPath:       "/tmp/session-monitor",
			BinaryModifiedAt: "2026-04-29T11:59:00Z",
			BinarySize:       12345,
		},
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
	for _, want := range []string{"Daemon abc123def456+dirty", "Daemon build", "binaryModifiedAt", "2026-04-29T11:59:00Z"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("canvas missing daemon build field %q: %s", want, raw)
		}
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
		Plan: types.SessionPlan{
			Key:    "codex-proposed-plan:abc123",
			Title:  "Review state plan",
			Source: "codex-proposed-plan",
			Status: "proposed",
			Items:  []types.PlanItem{{Text: "Add plan metadata"}},
		},
		Match: types.MatchResult{Status: "likely", CanvasID: "canvas-newer", RunID: "run-newer"},
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
	for _, want := range []string{"Purpose: Newer session", "Plan: Review state plan (proposed)", "Now: idle, active; 5m ago; match likely; evidence pending (0); assistant: newer message", "Evidence: pending (0)", "Next: Confirm the likely canvas/run link"} {
		if !strings.Contains(subtitle, want) {
			t.Fatalf("recent session subtitle missing %q:\n%s", want, subtitle)
		}
	}
	for key, want := range map[string]string{
		"sessionId":      "newer-session",
		"updatedAt":      "2026-04-29T11:55:00Z",
		"attention":      "idle",
		"purpose":        "Newer session",
		"evidenceStatus": "pending",
		"nextStep":       "Confirm the likely canvas/run link before treating this as attached.",
		"planLabel":      "Review state plan (proposed)",
	} {
		if got := core.StringValue(items[0][key]); got != want {
			t.Fatalf("collection item field %s = %q, want %q", key, got, want)
		}
	}
	if got := core.StringValue(items[0]["currentState"]); !strings.Contains(got, "idle, active") || !strings.Contains(got, "assistant: newer message") {
		t.Fatalf("unexpected currentState field: %q", got)
	}
	if got, ok := items[0]["artifactCount"].(int); !ok || got != 0 {
		t.Fatalf("unexpected artifactCount field: %#v", items[0]["artifactCount"])
	}
	badges, ok := items[0]["badges"].([]string)
	if !ok || strings.Join(badges, "|") != "claude|idle|active|likely|attention:idle|evidence:pending|plan" {
		t.Fatalf("unexpected collection badges: %#v", items[0]["badges"])
	}
	raw, err := json.Marshal(canvas)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "planTitle") || !strings.Contains(string(raw), "Review state plan") {
		t.Fatalf("canvas metadata missing plan fields: %s", raw)
	}
}

func TestBuildCanvasIncludesEvidenceBlocksAndManualNudge(t *testing.T) {
	root := t.TempDir()
	mdPath := filepath.Join(root, "review.md")
	logPath := filepath.Join(root, "cli.log")
	writeArtifactFile(t, mdPath, "# Review\n\nLooks good.\n")
	writeArtifactFile(t, logPath, "go test ./...\nPASS\n")
	ready := types.LocalSession{
		Provider:  "codex",
		SessionID: "ready-session",
		Title:     "Ready evidence",
		CWD:       root,
		Status:    "recorded",
		UpdatedAt: "2026-04-29T10:00:00Z",
		Artifacts: []types.SessionArtifact{
			{ID: "artifact-md", Kind: "markdown", Title: "review.md", Source: "assistant", Path: mdPath, ContentType: "text/markdown", Size: 20},
			{ID: "artifact-log", Kind: "terminal", Title: "cli.log", Source: "assistant", Path: logPath, ContentType: "text/plain", Size: 20},
			{ID: "artifact-img", Kind: "image", Title: "screen.png", Source: "assistant", AssetID: "asset.session-monitor.test", ContentType: "image/png", Summary: "uploaded"},
		},
		Match: types.MatchResult{Status: "unmatched"},
	}
	missing := types.LocalSession{
		Provider:   "claude",
		SessionID:  "missing-session",
		Title:      "Missing evidence",
		CWD:        root,
		Status:     "idle",
		UpdatedAt:  "2026-04-29T08:00:00Z",
		ResumeHint: "claude -p --resume missing-session <prompt>",
		Match:      types.MatchResult{Status: "unmatched"},
	}
	cfg := types.Config{
		CanvasID:    types.DefaultCanvasID,
		RunID:       types.DefaultRunID,
		CodexHome:   "/tmp/codex",
		ClaudeHome:  "/tmp/claude",
		Lookback:    time.Hour,
		MaxSessions: 10,
	}
	canvas := BuildCanvas(cfg, []types.LocalSession{ready, missing}, nil, nil, types.HubState{}, 1, time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC))
	raw, err := json.Marshal(canvas)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"evidence:ready", "artifacts:3", "session-01-artifact-01", "Looks good.", "go test ./...", "asset.session-monitor.test", "Manual nudge", "claude -p --resume missing-session"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("canvas missing %q:\n%s", want, raw)
		}
	}
}

func writeArtifactFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
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
