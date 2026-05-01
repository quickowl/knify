package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	if len(sessions[0].LatestMessages[0].Text) > messageTextLimit {
		t.Fatalf("message summary was not capped: %d", len(sessions[0].LatestMessages[0].Text))
	}
}

func TestCollapseSessionsByProviderIDKeepsNewestRow(t *testing.T) {
	sessions := collapseSessionsByProviderID([]LocalSession{
		{
			Provider:       "codex",
			SessionID:      "same-session",
			Title:          "Older title",
			CWD:            "/tmp/demo",
			Status:         "recorded",
			SourcePath:     "/tmp/older.jsonl",
			UpdatedAt:      "2026-04-29T10:00:00Z",
			LatestMessages: []MessageSummary{{Role: "assistant", Text: "older"}},
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
			LatestMessages: []MessageSummary{{Role: "assistant", Text: "newer"}},
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

func TestMatchingRanksExactAboveLikely(t *testing.T) {
	session := LocalSession{
		Provider:  "codex",
		SessionID: "thread-exact",
		CWD:       "/tmp/demo",
		UpdatedAt: "2026-04-29T12:00:00Z",
	}
	hub := HubState{
		Runs: []HubRun{
			{
				ID:        "run-likely",
				Provider:  "codex_exec",
				CanvasID:  "canvas-likely",
				UpdatedAt: "2026-04-29T12:00:00Z",
				FeedbackTarget: FeedbackTarget{
					CWD: "/tmp/demo",
				},
			},
			{
				ID:         "run-exact",
				Provider:   "codex_exec",
				CanvasID:   "canvas-exact",
				ExternalID: "thread-exact",
				UpdatedAt:  "2026-04-29T12:00:00Z",
			},
		},
	}
	match := bestMatch(session, hub)
	if match.Status != "exact" || match.CanvasID != "canvas-exact" {
		t.Fatalf("expected exact match, got %#v", match)
	}
}

func TestExactMatchDoesNotRequireProviderFamily(t *testing.T) {
	session := LocalSession{
		Provider:  "codex",
		SessionID: "thread-exact",
		CWD:       "/tmp/demo",
		UpdatedAt: "2026-04-29T12:00:00Z",
	}
	hub := HubState{
		Runs: []HubRun{
			{
				ID:         "run-exact",
				Provider:   "generic_cloud",
				CanvasID:   "canvas-exact",
				ExternalID: "thread-exact",
				UpdatedAt:  "2026-04-29T12:00:00Z",
			},
		},
	}
	match := bestMatch(session, hub)
	if match.Status != "exact" || match.CanvasID != "canvas-exact" {
		t.Fatalf("expected exact match across provider labels, got %#v", match)
	}
}

func TestLikelyMatchUsesProviderWorkspaceAndTime(t *testing.T) {
	session := LocalSession{
		Provider:  "claude",
		SessionID: "claude-local",
		CWD:       "/tmp/demo",
		UpdatedAt: "2026-04-29T12:00:00Z",
	}
	hub := HubState{
		Runs: []HubRun{
			{
				ID:        "run-claude",
				Provider:  "claude_cli",
				CanvasID:  "canvas-claude",
				UpdatedAt: "2026-04-29T11:30:00Z",
				FeedbackTarget: FeedbackTarget{
					CWD: "/tmp/demo",
				},
			},
		},
	}
	match := bestMatch(session, hub)
	if match.Status != "likely" || match.CanvasID != "canvas-claude" {
		t.Fatalf("expected likely match, got %#v", match)
	}
}

func TestBuildCanvasCapsTranscriptText(t *testing.T) {
	longText := strings.Repeat("x", messageTextLimit*3)
	session := LocalSession{
		Provider:  "codex",
		SessionID: "thread-1",
		Title:     "Long transcript",
		CWD:       "/tmp/demo",
		Status:    "recorded",
		UpdatedAt: "2026-04-29T12:00:00Z",
		LatestMessages: []MessageSummary{
			{Role: "user", Text: summarizeText(longText, messageTextLimit)},
		},
		Match: MatchResult{Status: "unmatched", Reason: "no match"},
	}
	cfg := Config{
		CanvasID:    defaultCanvasID,
		RunID:       defaultRunID,
		CodexHome:   "/tmp/codex",
		ClaudeHome:  "/tmp/claude",
		Lookback:    time.Hour,
		MaxSessions: 10,
	}
	canvas := buildCanvas(cfg, []LocalSession{session}, nil, nil, HubState{}, 1, time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC))
	raw, err := json.Marshal(canvas)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), strings.Repeat("x", messageTextLimit+20)) {
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
	older := LocalSession{
		Provider:       "codex",
		SessionID:      "older-session",
		Title:          "Older session",
		CWD:            "/tmp/demo",
		Status:         "recorded",
		UpdatedAt:      "2026-04-29T10:00:00Z",
		LatestMessages: []MessageSummary{{Role: "assistant", Text: "older message"}},
		Match:          MatchResult{Status: "unmatched"},
	}
	newer := LocalSession{
		Provider:       "claude",
		SessionID:      "newer-session",
		Title:          "Newer session",
		CWD:            "/tmp/demo",
		Status:         "idle",
		UpdatedAt:      "2026-04-29T11:55:00Z",
		LatestMessages: []MessageSummary{{Role: "assistant", Text: "newer message"}},
		Match:          MatchResult{Status: "likely", CanvasID: "canvas-newer", RunID: "run-newer"},
	}
	cfg := Config{
		CanvasID:    defaultCanvasID,
		RunID:       defaultRunID,
		CodexHome:   "/tmp/codex",
		ClaudeHome:  "/tmp/claude",
		Lookback:    time.Hour,
		MaxSessions: 10,
	}
	canvas := buildCanvas(cfg, []LocalSession{older, newer}, nil, nil, HubState{}, 1, time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC))
	collection := blockByID(t, canvas, "session-monitor-collection")
	if collection["mode"] != "paged-list" {
		t.Fatalf("expected paged-list collection mode, got %#v", collection["mode"])
	}
	items, ok := collection["items"].([]map[string]any)
	if !ok {
		t.Fatalf("collection items have unexpected shape: %#v", collection["items"])
	}
	if len(items) != 2 || !strings.Contains(stringValue(items[0]["label"]), "Newer session") || !strings.Contains(stringValue(items[1]["label"]), "Older session") {
		t.Fatalf("recent session collection not newest-first: %#v", items)
	}
	subtitle := stringValue(items[0]["subtitle"])
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

func TestSessionReviewAddsPurposeNowNext(t *testing.T) {
	session := LocalSession{
		Provider:  "claude",
		SessionID: "session-1",
		Title:     "Review product server status",
		CWD:       "/tmp/demo",
		Status:    "busy",
		UpdatedAt: "2026-04-29T11:58:00Z",
		LatestMessages: []MessageSummary{
			{Role: "assistant", Text: "server is still booting"},
		},
		Metadata: map[string]string{"collapsedFiles": "4"},
		Match:    MatchResult{Status: "unmatched"},
	}
	review := sessionReview(session, time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC))
	if review.Purpose != "Review product server status" {
		t.Fatalf("unexpected purpose: %#v", review)
	}
	if !strings.Contains(review.CurrentState, "busy, active") || !strings.Contains(review.CurrentState, "collapsed 4 files") {
		t.Fatalf("unexpected current state: %#v", review)
	}
	if !strings.Contains(review.NextStep, "Keep watching") {
		t.Fatalf("unexpected next step: %#v", review)
	}
	if len(review.Signals) == 0 {
		t.Fatalf("expected signals: %#v", review)
	}
}

func TestRunOncePublishesCanvasToHub(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex")
	sessionDir := filepath.Join(codexHome, "sessions", "2026", "04", "29")
	mustMkdir(t, sessionDir)
	writeLines(t, filepath.Join(sessionDir, "rollout-2026-04-29T10-00-00-thread-1.jsonl"),
		`{"timestamp":"2026-04-29T10:00:00Z","type":"session_meta","payload":{"id":"thread-1","cwd":"/tmp/demo"}}`,
		`{"timestamp":"2026-04-29T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"text":"publish me"}]}}`,
	)
	claudeHome := filepath.Join(root, "claude")
	mustMkdir(t, filepath.Join(claudeHome, "projects"))

	var posted Canvas
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/canvases":
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode([]HubCanvas{})
				return
			}
			if r.Method == http.MethodPost {
				if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
					t.Fatalf("decode posted canvas: %v", err)
				}
				_ = json.NewEncoder(w).Encode(posted)
				return
			}
		case "/v1/agent-runs":
			_ = json.NewEncoder(w).Encode([]HubRun{})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cfg := Config{
		HubURL:      server.URL,
		Token:       "token",
		CanvasID:    defaultCanvasID,
		RunID:       defaultRunID,
		CodexHome:   codexHome,
		ClaudeHome:  claudeHome,
		Lookback:    24 * time.Hour,
		MaxSessions: 10,
		Now:         func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) },
	}
	result, err := runOnce(testContext(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Posted || posted.ID != defaultCanvasID || len(posted.Blocks) == 0 {
		t.Fatalf("expected posted canvas, result=%#v posted=%#v", result, posted)
	}
}

func TestRunOncePublishesDynamicCanvasEvents(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex")
	sessionDir := filepath.Join(codexHome, "sessions", "2026", "04", "29")
	mustMkdir(t, sessionDir)
	writeLines(t, filepath.Join(sessionDir, "rollout-2026-04-29T10-00-00-thread-1.jsonl"),
		`{"timestamp":"2026-04-29T10:00:00Z","type":"session_meta","payload":{"id":"thread-1","cwd":"/tmp/demo"}}`,
		`{"timestamp":"2026-04-29T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"text":"publish me"}]}}`,
	)
	claudeHome := filepath.Join(root, "claude")
	mustMkdir(t, filepath.Join(claudeHome, "projects"))

	var events []CanvasLogEvent
	staticPosts := 0
	version := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/canvases":
			_ = json.NewEncoder(w).Encode([]HubCanvas{})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/canvases":
			staticPosts++
			http.Error(w, "static post should not be used", http.StatusBadRequest)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agent-runs":
			_ = json.NewEncoder(w).Encode([]HubRun{})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/canvases/"+defaultCanvasID:
			http.NotFound(w, r)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/canvases/"+defaultCanvasID+"/events":
			var event CanvasLogEvent
			if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
				t.Fatalf("decode canvas event: %v", err)
			}
			events = append(events, event)
			version++
			_ = json.NewEncoder(w).Encode(map[string]any{"id": defaultCanvasID, "version": version})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cfg := Config{
		HubURL:      server.URL,
		Token:       "token",
		CanvasID:    defaultCanvasID,
		RunID:       defaultRunID,
		CodexHome:   codexHome,
		ClaudeHome:  claudeHome,
		Lookback:    24 * time.Hour,
		MaxSessions: 10,
		Dynamic:     true,
		Now:         func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) },
	}
	result, err := runOnce(testContext(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Posted || !result.Dynamic || result.EventsPosted == 0 || staticPosts != 0 {
		t.Fatalf("expected dynamic event publish, result=%#v staticPosts=%d", result, staticPosts)
	}
	if len(events) == 0 || events[0].Type != "canvas.started" {
		t.Fatalf("expected first dynamic event to start canvas, got %#v", events)
	}
	foundRecentList := false
	for _, event := range events {
		if event.Type == "canvas.block.appended" && event.BlockID == "" && event.Block["id"] == "session-monitor-collection" && event.Block["mode"] == "paged-list" {
			foundRecentList = true
		}
	}
	if !foundRecentList {
		t.Fatalf("dynamic publish did not append recent sessions block: %#v", events)
	}
}

func TestStatusReportsHealthyHeartbeat(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	outPath := filepath.Join(t.TempDir(), "session-monitor.json")
	writeJSON(t, outPath, ScanResult{
		GeneratedAt: formatTime(now.Add(-30 * time.Second)),
		Process: ProcessInfo{
			PID:      os.Getpid(),
			Mode:     "watch",
			Watch:    true,
			Dynamic:  true,
			Interval: "1m0s",
			CanvasID: defaultCanvasID,
			RunID:    defaultRunID,
			OutPath:  outPath,
		},
		Sessions: []LocalSession{
			{Provider: "codex", Match: MatchResult{Status: "unmatched"}},
			{Provider: "claude", Match: MatchResult{Status: "likely"}},
		},
		ProviderHealth: []ProviderHealth{
			{Provider: "codex", Status: "ok", Details: map[string]string{"sessions": "1"}},
			{Provider: "claude", Status: "ok", Details: map[string]string{"sessions": "1"}},
			{Provider: "cursor", Status: "health_only"},
		},
		Canvas:       Canvas{ID: defaultCanvasID},
		Posted:       true,
		Dynamic:      true,
		EventsPosted: 3,
	})

	var out bytes.Buffer
	err := writeStatus(testContext(t), Config{
		OutPath:    outPath,
		CanvasID:   defaultCanvasID,
		RunID:      defaultRunID,
		StaleAfter: time.Minute,
		Now:        func() time.Time { return now },
	}, &out)
	if err != nil {
		t.Fatalf("expected healthy status, got %v\n%s", err, out.String())
	}
	for _, want := range []string{"session-monitor status: ok", "heartbeat: fresh", "process: pid=", "sessions: total=2", "codex: ok", "claude: ok"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status output missing %q:\n%s", want, out.String())
		}
	}
}

func TestStatusFailsStaleHeartbeat(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	outPath := filepath.Join(t.TempDir(), "session-monitor.json")
	writeJSON(t, outPath, ScanResult{
		GeneratedAt: formatTime(now.Add(-10 * time.Minute)),
		Process:     ProcessInfo{PID: os.Getpid(), Mode: "watch", Watch: true},
		ProviderHealth: []ProviderHealth{
			{Provider: "codex", Status: "ok"},
			{Provider: "claude", Status: "ok"},
		},
		Posted: true,
	})

	var out bytes.Buffer
	err := writeStatus(testContext(t), Config{
		OutPath:    outPath,
		CanvasID:   defaultCanvasID,
		RunID:      defaultRunID,
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

func markdownBlock(t *testing.T, canvas Canvas, id string) string {
	t.Helper()
	for _, block := range canvas.Blocks {
		if block["id"] == id {
			markdown, ok := block["markdown"].(string)
			if !ok {
				t.Fatalf("block %s is not markdown: %#v", id, block)
			}
			return markdown
		}
	}
	t.Fatalf("markdown block %s not found", id)
	return ""
}

func blockByID(t *testing.T, canvas Canvas, id string) map[string]any {
	t.Helper()
	for _, block := range canvas.Blocks {
		if block["id"] == id {
			return block
		}
	}
	t.Fatalf("block %s not found", id)
	return nil
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
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
