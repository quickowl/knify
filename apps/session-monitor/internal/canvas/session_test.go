package canvas

import (
	"testing"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func TestSessionAttention(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		session types.LocalSession
		want    string
	}{
		{
			name: "claude busy and recent is running",
			session: types.LocalSession{
				Provider:  "claude",
				Status:    "busy",
				UpdatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
			},
			want: "running",
		},
		{
			name: "claude busy but stale is idle",
			session: types.LocalSession{
				Provider:  "claude",
				Status:    "busy",
				UpdatedAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
			},
			want: "idle",
		},
		{
			name: "claude recorded and recent is idle",
			session: types.LocalSession{
				Provider:  "claude",
				Status:    "recorded",
				UpdatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
			},
			want: "idle",
		},
		{
			name: "codex active is running",
			session: types.LocalSession{
				Provider:  "codex",
				UpdatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
			},
			want: "running",
		},
		{
			name: "codex idle is idle",
			session: types.LocalSession{
				Provider:  "codex",
				UpdatedAt: now.Add(-3 * time.Hour).Format(time.RFC3339),
			},
			want: "idle",
		},
		{
			name: "missing updated time is idle",
			session: types.LocalSession{
				Provider: "claude",
				Status:   "busy",
			},
			want: "idle",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sessionAttention(tc.session, now)
			if got != tc.want {
				t.Fatalf("sessionAttention got %q, want %q", got, tc.want)
			}
		})
	}
}
