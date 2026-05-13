package status

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func TestStatusReportsHealthyHeartbeat(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	outPath := filepath.Join(t.TempDir(), "session-monitor.json")
	writeJSON(t, outPath, types.ScanResult{
		GeneratedAt: core.FormatTime(now.Add(-30 * time.Second)),
		Process: types.ProcessInfo{
			PID:      os.Getpid(),
			Mode:     "watch",
			Watch:    true,
			Dynamic:  true,
			Interval: "1m0s",
			CanvasID: types.DefaultCanvasID,
			RunID:    types.DefaultRunID,
			OutPath:  outPath,
			Build:    types.DaemonBuildInfo{RevisionShort: "abc123", Modified: true},
		},
		Sessions: []types.LocalSession{
			{Provider: "codex", Match: types.MatchResult{Status: "unmatched"}},
			{Provider: "claude", Match: types.MatchResult{Status: "likely"}},
		},
		ProviderHealth: []types.ProviderHealth{
			{Provider: "codex", Status: "ok", Details: map[string]string{"sessions": "1"}},
			{Provider: "claude", Status: "ok", Details: map[string]string{"sessions": "1"}},
			{Provider: "cursor", Status: "health_only"},
		},
		Canvas:       types.Canvas{ID: types.DefaultCanvasID},
		Posted:       true,
		Dynamic:      true,
		EventsPosted: 3,
	})

	var out bytes.Buffer
	err := WriteStatus(testContext(t), types.Config{
		OutPath:    outPath,
		CanvasID:   types.DefaultCanvasID,
		RunID:      types.DefaultRunID,
		StaleAfter: time.Minute,
		Now:        func() time.Time { return now },
	}, &out)
	if err != nil {
		t.Fatalf("expected healthy status, got %v\n%s", err, out.String())
	}
	for _, want := range []string{"session-monitor status: ok", "heartbeat: fresh", "process: pid=", "build=abc123+dirty", "sessions: total=2", "codex: ok", "claude: ok"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status output missing %q:\n%s", want, out.String())
		}
	}
}

func TestStatusFailsStaleHeartbeat(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	outPath := filepath.Join(t.TempDir(), "session-monitor.json")
	writeJSON(t, outPath, types.ScanResult{
		GeneratedAt: core.FormatTime(now.Add(-10 * time.Minute)),
		Process:     types.ProcessInfo{PID: os.Getpid(), Mode: "watch", Watch: true},
		ProviderHealth: []types.ProviderHealth{
			{Provider: "codex", Status: "ok"},
			{Provider: "claude", Status: "ok"},
		},
		Posted: true,
	})

	var out bytes.Buffer
	err := WriteStatus(testContext(t), types.Config{
		OutPath:    outPath,
		CanvasID:   types.DefaultCanvasID,
		RunID:      types.DefaultRunID,
		StaleAfter: time.Minute,
		Now:        func() time.Time { return now },
	}, &out)
	if err == nil {
		t.Fatalf("expected stale status to fail:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "session-monitor status: not-ok") || !strings.Contains(out.String(), "heartbeat: stale") {
		t.Fatalf("unexpected stale status output:\n%s", out.String())
	}
}

func TestPrettyStatusPrintsDashboard(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	outPath := filepath.Join(t.TempDir(), "session-monitor.json")
	writeJSON(t, outPath, types.ScanResult{
		GeneratedAt: core.FormatTime(now.Add(-30 * time.Second)),
		Process: types.ProcessInfo{
			PID:      os.Getpid(),
			Mode:     "watch",
			Watch:    true,
			Dynamic:  true,
			Interval: "1m0s",
			CanvasID: types.DefaultCanvasID,
			RunID:    types.DefaultRunID,
			OutPath:  outPath,
			Build:    types.DaemonBuildInfo{RevisionShort: "abc123", Modified: true},
		},
		Sessions: []types.LocalSession{
			{
				Provider:  "codex",
				SessionID: "codex-session-1",
				Status:    "recorded",
				CWD:       "/Users/fireharp/Prog/knify/canvases",
				UpdatedAt: core.FormatTime(now.Add(-2 * time.Minute)),
				Review: types.SessionReview{
					Purpose:      "Monitor sessions",
					CurrentState: "recorded, active",
					NextStep:     "Keep watching",
				},
				Plan: types.SessionPlan{
					Key:    "plan:monitor",
					Title:  "Monitor plan",
					Source: "codex-proposed-plan",
					Status: "active",
				},
				Match: types.MatchResult{Status: "unmatched"},
			},
			{
				Provider:  "claude",
				SessionID: "claude-session-1",
				Status:    "recorded",
				CWD:       "/Users/fireharp/Prog/deacon/dogs/boot",
				UpdatedAt: core.FormatTime(now.Add(-30 * time.Minute)),
				Review: types.SessionReview{
					Purpose:      "Daemon triage",
					CurrentState: "recorded, active",
					NextStep:     "Keep watching",
				},
				Match: types.MatchResult{Status: "likely"},
			},
		},
		ProviderHealth: []types.ProviderHealth{
			{Provider: "codex", Status: "ok", Details: map[string]string{"sessions": "1", "rawFiles": "2"}},
			{Provider: "claude", Status: "ok", Details: map[string]string{"sessions": "1", "rawFiles": "2"}},
		},
		Canvas: types.Canvas{ID: types.DefaultCanvasID},
		Posted: true,
	})

	var out bytes.Buffer
	err := WriteStatus(testContext(t), types.Config{
		OutPath:     outPath,
		CanvasID:    types.DefaultCanvasID,
		RunID:       types.DefaultRunID,
		StaleAfter:  time.Minute,
		RecentLimit: 2,
		Pretty:      true,
		Now:         func() time.Time { return now },
	}, &out)
	if err != nil {
		t.Fatalf("expected healthy pretty status, got %v\n%s", err, out.String())
	}
	for _, want := range []string{"SESSION MONITOR", "[ OK ]", "HEARTBEAT", "build=abc123+dirty", "SESSIONS", "plans=1", "MATCHES", "PROVIDERS", "RECENT SESSIONS", "Monitor plan", "Monitor sessions", "Daemon triage"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("pretty status output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "purpose:") {
		t.Fatalf("pretty status should avoid verbose raw session blocks:\n%s", out.String())
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
