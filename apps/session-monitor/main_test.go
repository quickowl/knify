package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func TestRunOncePublishesCanvasToHub(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex")
	cwd := filepath.Join(root, "work")
	sessionDir := filepath.Join(codexHome, "sessions", "2026", "04", "29")
	mustMkdir(t, sessionDir)
	writeFile(t, filepath.Join(cwd, "screen.png"), "png")
	writeLines(t, filepath.Join(sessionDir, "rollout-2026-04-29T10-00-00-thread-1.jsonl"),
		`{"timestamp":"2026-04-29T10:00:00Z","type":"session_meta","payload":{"id":"thread-1","cwd":"`+cwd+`"}}`,
		`{"timestamp":"2026-04-29T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"text":"publish me"}]}}`,
		`{"timestamp":"2026-04-29T10:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"text":"Published screenshot screen.png"}]}}`,
	)
	claudeHome := filepath.Join(root, "claude")
	mustMkdir(t, filepath.Join(claudeHome, "projects"))

	var posted types.Canvas
	assetPosts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/canvases":
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode([]types.HubCanvas{})
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
			_ = json.NewEncoder(w).Encode([]types.HubRun{})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/assets" {
			assetPosts++
			id := "asset-0123456789abcdef0123456789abcdef"
			_ = json.NewEncoder(w).Encode(map[string]any{"assetId": id, "id": id, "contentType": r.Header.Get("Content-Type"), "url": "https://pub.example.r2.dev/assets/" + id})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cfg := types.Config{
		HubURL:      server.URL,
		Token:       "token",
		CanvasID:    types.DefaultCanvasID,
		RunID:       types.DefaultRunID,
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
	if !result.Posted || posted.ID != types.DefaultCanvasID || len(posted.Blocks) == 0 {
		t.Fatalf("expected posted canvas, result=%#v posted=%#v", result, posted)
	}
	raw, err := json.Marshal(posted)
	if err != nil {
		t.Fatal(err)
	}
	rawCanvas := string(raw)
	if assetPosts != 1 || !strings.Contains(rawCanvas, `"assetId"`) || !strings.Contains(rawCanvas, `"url":"https://pub.example.r2.dev/assets/asset-0123456789abcdef0123456789abcdef"`) || strings.Contains(rawCanvas, "/v1/assets/") || !strings.Contains(rawCanvas, "evidence:ready") {
		t.Fatalf("expected uploaded image evidence, assetPosts=%d canvas=%s", assetPosts, raw)
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

	var events []types.CanvasLogEvent
	staticPosts := 0
	version := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/canvases":
			_ = json.NewEncoder(w).Encode([]types.HubCanvas{})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/canvases":
			staticPosts++
			http.Error(w, "static post should not be used", http.StatusBadRequest)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agent-runs":
			_ = json.NewEncoder(w).Encode([]types.HubRun{})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/canvases/"+types.DefaultCanvasID:
			http.NotFound(w, r)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/canvases/"+types.DefaultCanvasID+"/events":
			var event types.CanvasLogEvent
			if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
				t.Fatalf("decode canvas event: %v", err)
			}
			events = append(events, event)
			version++
			_ = json.NewEncoder(w).Encode(map[string]any{"id": types.DefaultCanvasID, "version": version})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cfg := types.Config{
		HubURL:      server.URL,
		Token:       "token",
		CanvasID:    types.DefaultCanvasID,
		RunID:       types.DefaultRunID,
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

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
