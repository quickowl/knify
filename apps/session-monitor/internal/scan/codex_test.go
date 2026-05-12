package scan

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCodexSessionExtractsCoreFields(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "codex")
	sessionDir := filepath.Join(home, "sessions", "2026", "04", "29")
	mustMkdir(t, sessionDir)
	sessionPath := filepath.Join(sessionDir, "rollout-2026-04-29T10-00-00-codex-session-1.jsonl")
	writeLines(t, sessionPath,
		`{"timestamp":"2026-04-29T10:00:00Z","type":"session_meta","payload":{"id":"codex-session-1","cwd":"/tmp/demo","source":"cli","model_provider":"openai","git":{"branch":"main"}}}`,
		`{"timestamp":"2026-04-29T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"please update the canvas"}]}}`,
		`{"timestamp":"2026-04-29T10:00:02Z","type":"event_msg","payload":{"type":"thread_renamed","thread_id":"codex-session-1","thread_name":"Codex fixture task"}}`,
		`{"timestamp":"2026-04-29T10:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"canvas updated"}]}}`,
	)
	writeLines(t, filepath.Join(home, "session_index.jsonl"), `{"id":"codex-session-1","thread_name":"Indexed title","updated_at":"2026-04-29T10:00:04Z"}`)

	sessions, health, errs := scanCodex(testContext(t), home, time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	if health.Status != "ok" {
		t.Fatalf("unexpected health: %#v", health)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(sessions))
	}
	got := sessions[0]
	if got.Provider != "codex" || got.SessionID != "codex-session-1" || got.CWD != "/tmp/demo" {
		t.Fatalf("unexpected session identity: %#v", got)
	}
	if got.Title != "Codex fixture task" {
		t.Fatalf("unexpected title %q", got.Title)
	}
	if got.UpdatedAt != "2026-04-29T10:00:04Z" {
		t.Fatalf("expected index updatedAt, got %q", got.UpdatedAt)
	}
	if got.Metadata["gitBranch"] != "main" {
		t.Fatalf("missing git metadata: %#v", got.Metadata)
	}
	if len(got.LatestMessages) != 2 || got.LatestMessages[0].Role != "user" || !strings.Contains(got.LatestMessages[1].Text, "canvas updated") {
		t.Fatalf("unexpected messages: %#v", got.LatestMessages)
	}
}

func TestParseCodexSessionHandlesLargeJSONLRecord(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "codex")
	sessionDir := filepath.Join(home, "sessions", "2026", "04", "29")
	mustMkdir(t, sessionDir)
	largeText := strings.Repeat("x", 9*1024*1024)
	writeLines(t, filepath.Join(sessionDir, "rollout-2026-04-29T10-00-00-large-session.jsonl"),
		`{"timestamp":"2026-04-29T10:00:00Z","type":"session_meta","payload":{"id":"large-session","cwd":"/tmp/demo"}}`,
		`{"timestamp":"2026-04-29T10:00:01Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"`+largeText+`"}]}}`,
	)

	sessions, _, errs := scanCodex(testContext(t), home, time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors for large JSONL record: %#v", errs)
	}
	if len(sessions) != 1 || len(sessions[0].LatestMessages) != 1 {
		t.Fatalf("expected one parsed large session with one capped message, got %#v", sessions)
	}
	if len(sessions[0].LatestMessages[0].Text) > MessageTextLimit {
		t.Fatalf("message summary was not capped: %d", len(sessions[0].LatestMessages[0].Text))
	}
}
