package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractArtifactsFromAssistantText(t *testing.T) {
	cwd := t.TempDir()
	mustWriteFile(t, filepath.Join(cwd, "review.md"), "# Review\n")
	mustWriteFile(t, filepath.Join(cwd, "logs", "cli.log"), "ok\n")
	mustWriteFile(t, filepath.Join(cwd, "screens", "feature.png"), "png")
	mustWriteFile(t, filepath.Join(cwd, "reports", "page.html"), "<h1>ok</h1>")

	artifacts := extractArtifactsFromText("Published `review.md`, `logs/cli.log`, screenshot screens/feature.png and report reports/page.html.", cwd, "assistant")
	if len(artifacts) != 4 {
		t.Fatalf("expected 4 artifacts, got %#v", artifacts)
	}
	if artifacts[0].Kind != "markdown" || artifacts[1].Kind != "terminal" || artifacts[2].Kind != "image" || artifacts[3].Kind != "html" {
		t.Fatalf("unexpected artifact kinds: %#v", artifacts)
	}
	if artifacts[0].Path != filepath.Join(cwd, "review.md") || artifacts[2].ContentType != "image/png" || artifacts[3].ContentType != "text/html; charset=utf-8" {
		t.Fatalf("unexpected artifact metadata: %#v", artifacts)
	}
}

func TestParseCodexSessionCollectsAssistantArtifactsOnly(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "codex")
	cwd := filepath.Join(root, "work")
	sessionDir := filepath.Join(home, "sessions", "2026", "04", "29")
	mustMkdir(t, sessionDir)
	mustWriteFile(t, filepath.Join(cwd, "user.md"), "# user\n")
	mustWriteFile(t, filepath.Join(cwd, "assistant.md"), "# assistant\n")
	writeLines(t, filepath.Join(sessionDir, "rollout-2026-04-29T10-00-00-artifact-session.jsonl"),
		`{"timestamp":"2026-04-29T10:00:00Z","type":"session_meta","payload":{"id":"artifact-session","cwd":"`+cwd+`"}}`,
		`{"timestamp":"2026-04-29T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Please inspect user.md"}]}}`,
		`{"timestamp":"2026-04-29T10:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Review artifact: assistant.md"}]}}`,
	)

	sessions, _, errs := scanCodex(testContext(t), home, time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC))
	if len(errs) != 0 || len(sessions) != 1 {
		t.Fatalf("unexpected scan result sessions=%#v errs=%#v", sessions, errs)
	}
	if len(sessions[0].Artifacts) != 1 || sessions[0].Artifacts[0].Title != "assistant.md" {
		t.Fatalf("expected only assistant artifact, got %#v", sessions[0].Artifacts)
	}
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
