package match

import (
	"testing"

	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func TestMatchingRanksExactAboveLikely(t *testing.T) {
	session := types.LocalSession{
		Provider:  "codex",
		SessionID: "thread-exact",
		CWD:       "/tmp/demo",
		UpdatedAt: "2026-04-29T12:00:00Z",
	}
	hub := types.HubState{
		Runs: []types.HubRun{
			{
				ID:        "run-likely",
				Provider:  "codex_exec",
				CanvasID:  "canvas-likely",
				UpdatedAt: "2026-04-29T12:00:00Z",
				FeedbackTarget: types.FeedbackTarget{
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
	got := bestMatch(session, hub)
	if got.Status != "exact" || got.CanvasID != "canvas-exact" {
		t.Fatalf("expected exact match, got %#v", got)
	}
}

func TestExactMatchDoesNotRequireProviderFamily(t *testing.T) {
	session := types.LocalSession{
		Provider:  "codex",
		SessionID: "thread-exact",
		CWD:       "/tmp/demo",
		UpdatedAt: "2026-04-29T12:00:00Z",
	}
	hub := types.HubState{
		Runs: []types.HubRun{
			{
				ID:         "run-exact",
				Provider:   "generic_cloud",
				CanvasID:   "canvas-exact",
				ExternalID: "thread-exact",
				UpdatedAt:  "2026-04-29T12:00:00Z",
			},
		},
	}
	got := bestMatch(session, hub)
	if got.Status != "exact" || got.CanvasID != "canvas-exact" {
		t.Fatalf("expected exact match across provider labels, got %#v", got)
	}
}

func TestLikelyMatchUsesProviderWorkspaceAndTime(t *testing.T) {
	session := types.LocalSession{
		Provider:  "claude",
		SessionID: "claude-local",
		CWD:       "/tmp/demo",
		UpdatedAt: "2026-04-29T12:00:00Z",
	}
	hub := types.HubState{
		Runs: []types.HubRun{
			{
				ID:        "run-claude",
				Provider:  "claude_cli",
				CanvasID:  "canvas-claude",
				UpdatedAt: "2026-04-29T11:30:00Z",
				FeedbackTarget: types.FeedbackTarget{
					CWD: "/tmp/demo",
				},
			},
		},
	}
	got := bestMatch(session, hub)
	if got.Status != "likely" || got.CanvasID != "canvas-claude" {
		t.Fatalf("expected likely match, got %#v", got)
	}
}
