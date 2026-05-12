package scan

import (
	"context"
	"sort"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

const (
	MessageTextLimit   = 240
	messagesPerSession = 3
)

func ScanLocalSessions(ctx context.Context, cfg types.Config, now time.Time) ([]types.LocalSession, []types.ProviderHealth, []string) {
	var sessions []types.LocalSession
	var health []types.ProviderHealth
	var errorsOut []string
	cutoff := now.Add(-cfg.Lookback)

	codexSessions, codexHealth, codexErrors := scanCodex(ctx, cfg.CodexHome, cutoff)
	sessions = append(sessions, codexSessions...)
	health = append(health, codexHealth)
	errorsOut = append(errorsOut, codexErrors...)

	claudeSessions, claudeHealth, claudeErrors := scanClaude(cfg.ClaudeHome, cutoff)
	sessions = append(sessions, claudeSessions...)
	health = append(health, claudeHealth)
	errorsOut = append(errorsOut, claudeErrors...)

	health = append(health, scanCursorHealth())

	sort.SliceStable(sessions, func(i, j int) bool {
		return core.TimeFromRFC3339(sessions[i].UpdatedAt).After(core.TimeFromRFC3339(sessions[j].UpdatedAt))
	})
	return sessions, health, errorsOut
}
