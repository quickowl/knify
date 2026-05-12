package canvas

import (
	"strings"
	"testing"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func TestSessionReviewAddsPurposeNowNext(t *testing.T) {
	session := types.LocalSession{
		Provider:  "claude",
		SessionID: "session-1",
		Title:     "Review product server status",
		CWD:       "/tmp/demo",
		Status:    "busy",
		UpdatedAt: "2026-04-29T11:58:00Z",
		LatestMessages: []types.MessageSummary{
			{Role: "assistant", Text: "server is still booting"},
		},
		Metadata: map[string]string{"collapsedFiles": "4"},
		Match:    types.MatchResult{Status: "unmatched"},
	}
	review := Review(session, time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC))
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
