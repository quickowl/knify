package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseClaudeSessionExtractsActiveStatusAndMessages(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "claude")
	projectDir := filepath.Join(home, "projects", "-tmp-demo")
	sessionDir := filepath.Join(home, "sessions")
	mustMkdir(t, projectDir)
	mustMkdir(t, sessionDir)
	writeLines(t, filepath.Join(projectDir, "claude-session-1.jsonl"),
		`{"type":"permission-mode","permissionMode":"bypassPermissions","sessionId":"claude-session-1"}`,
		`{"type":"user","message":{"role":"user","content":"review the latest build"},"timestamp":"2026-04-29T11:00:00Z","cwd":"/tmp/demo","sessionId":"claude-session-1","gitBranch":"feature/local-monitor"}`,
		`{"type":"ai-title","aiTitle":"Claude fixture task","sessionId":"claude-session-1"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"build reviewed"}]},"timestamp":"2026-04-29T11:00:03Z","cwd":"/tmp/demo","sessionId":"claude-session-1"}`,
	)
	writeJSON(t, filepath.Join(sessionDir, "43617.json"), map[string]any{
		"sessionId": "claude-session-1",
		"cwd":       "/tmp/demo",
		"status":    "idle",
		"updatedAt": float64(time.Date(2026, 4, 29, 11, 0, 5, 0, time.UTC).UnixMilli()),
		"pid":       43617,
	})
	writeLines(t, filepath.Join(home, "history.jsonl"), `{"display":"review the latest build","timestamp":1770000000,"project":"/tmp/demo"}`)

	sessions, health, errs := scanClaude(home, time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	if health.Details["historyEntries"] != "1" {
		t.Fatalf("expected history count, got %#v", health)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(sessions))
	}
	got := sessions[0]
	if got.Provider != "claude" || got.SessionID != "claude-session-1" || got.Status != "idle" {
		t.Fatalf("unexpected session identity: %#v", got)
	}
	if got.Title != "Claude fixture task" || got.Metadata["permissionMode"] != "bypassPermissions" || got.Metadata["pid"] != "43617" {
		t.Fatalf("unexpected metadata/title: %#v", got)
	}
	if got.UpdatedAt != "2026-04-29T11:00:05Z" {
		t.Fatalf("expected active session updatedAt, got %q", got.UpdatedAt)
	}
	if len(got.LatestMessages) != 2 || !strings.Contains(got.LatestMessages[1].Text, "build reviewed") {
		t.Fatalf("unexpected messages: %#v", got.LatestMessages)
	}
}

func TestScanClaudeCollapsesSubagentFiles(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "claude")
	projectDir := filepath.Join(home, "projects", "-tmp-demo")
	subagentDir := filepath.Join(projectDir, "claude-session-1", "subagents")
	mustMkdir(t, subagentDir)
	writeLines(t, filepath.Join(projectDir, "claude-session-1.jsonl"),
		`{"type":"user","message":{"role":"user","content":"review parent"},"timestamp":"2026-04-29T11:00:00Z","cwd":"/tmp/demo","sessionId":"claude-session-1"}`,
		`{"type":"ai-title","aiTitle":"Parent session title","sessionId":"claude-session-1"}`,
	)
	writeLines(t, filepath.Join(subagentDir, "agent-a.jsonl"),
		`{"type":"assistant","message":{"role":"assistant","content":"subagent a complete"},"timestamp":"2026-04-29T11:05:00Z","cwd":"/tmp/demo","sessionId":"claude-session-1"}`,
	)
	writeLines(t, filepath.Join(subagentDir, "agent-b.jsonl"),
		`{"type":"assistant","message":{"role":"assistant","content":"subagent b complete"},"timestamp":"2026-04-29T11:06:00Z","cwd":"/tmp/demo","sessionId":"claude-session-1"}`,
	)

	sessions, health, errs := scanClaude(home, time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one collapsed session, got %#v", sessions)
	}
	got := sessions[0]
	if got.Title != "Parent session title" || strings.Contains(got.SourcePath, "/subagents/") {
		t.Fatalf("expected parent title/source path, got %#v", got)
	}
	if got.UpdatedAt != "2026-04-29T11:06:00Z" || len(got.LatestMessages) != 1 || !strings.Contains(got.LatestMessages[0].Text, "subagent b") {
		t.Fatalf("expected latest subagent update/message, got %#v", got)
	}
	if got.Metadata["collapsedFiles"] != "3" || got.Metadata["subagentFiles"] != "2" {
		t.Fatalf("missing collapse metadata: %#v", got.Metadata)
	}
	if health.Details["sessions"] != "1" || health.Details["rawFiles"] != "3" || health.Details["collapsedFiles"] != "2" {
		t.Fatalf("unexpected health details: %#v", health.Details)
	}
}

func TestScanClaudeSkipsOldProjectFilesBeforeParsing(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "claude")
	projectDir := filepath.Join(home, "projects", "-tmp-demo")
	mustMkdir(t, projectDir)
	oldPath := filepath.Join(projectDir, "old-session.jsonl")
	recentPath := filepath.Join(projectDir, "recent-session.jsonl")
	writeLines(t, oldPath,
		`{"type":"user","message":{"role":"user","content":"old but expensive"},"timestamp":"2026-04-29T11:00:00Z","cwd":"/tmp/demo","sessionId":"old-session"}`,
	)
	writeLines(t, recentPath,
		`{"type":"user","message":{"role":"user","content":"recent"},"timestamp":"2026-04-30T11:00:00Z","cwd":"/tmp/demo","sessionId":"recent-session"}`,
	)
	oldModTime := time.Date(2026, 4, 28, 11, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldPath, oldModTime, oldModTime); err != nil {
		t.Fatal(err)
	}

	sessions, health, errs := scanClaude(home, time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "recent-session" {
		t.Fatalf("expected only recent session, got %#v", sessions)
	}
	if health.Details["sessions"] != "1" {
		t.Fatalf("unexpected health details: %#v", health.Details)
	}
}

func TestParseClaudeSessionExtractsNativePlan(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "claude")
	projectDir := filepath.Join(home, "projects", "-tmp-demo")
	mustMkdir(t, projectDir)
	writeLines(t, filepath.Join(projectDir, "claude-session-plan.jsonl"),
		`{"type":"user","message":{"role":"user","content":"review plan"},"timestamp":"2026-04-29T11:00:00Z","cwd":"/tmp/demo","sessionId":"claude-session-plan"}`,
		`{"type":"assistant","message":{"role":"assistant","content":"ready"},"timestamp":"2026-04-29T11:00:03Z","cwd":"/tmp/demo","sessionId":"claude-session-plan","toolUseResult":{"plan":"# Claude implementation plan\n\n## Steps\n- Inspect scanner\n- Add plan metadata\n- Run tests\n","filePath":"/Users/fireharp/.claude/plans/shared-plan.md","planWasEdited":true}}`,
	)

	sessions, _, errs := scanClaude(home, time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC))
	if len(errs) != 0 || len(sessions) != 1 {
		t.Fatalf("unexpected scan result sessions=%#v errs=%#v", sessions, errs)
	}
	got := sessions[0].Plan
	if got.Key != "claude-plan:/Users/fireharp/.claude/plans/shared-plan.md" || got.Title != "Claude implementation plan" || got.Status != "edited" {
		t.Fatalf("unexpected claude plan: %#v", got)
	}
	if len(got.Items) != 3 || got.Items[1].Text != "Add plan metadata" {
		t.Fatalf("expected plan items, got %#v", got.Items)
	}
}
