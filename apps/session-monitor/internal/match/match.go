package match

import (
	"strings"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func MatchSessions(sessions []types.LocalSession, hub types.HubState) {
	for i := range sessions {
		sessions[i].Match = bestMatch(sessions[i], hub)
	}
}

func bestMatch(session types.LocalSession, hub types.HubState) types.MatchResult {
	for _, run := range hub.Runs {
		if sessionIDMatchesRun(session.SessionID, run) {
			return types.MatchResult{
				Status:    "exact",
				Reason:    "session id matched an AgentCanvas agent run",
				CanvasID:  core.FirstNonEmpty(run.CanvasID, run.ID),
				RunID:     core.FirstNonEmpty(run.ID, run.RunID),
				Score:     100,
				Evidence:  "agentRun.externalId/id/runId",
				Provider:  run.Provider,
				UpdatedAt: run.UpdatedAt,
			}
		}
	}
	for _, canvas := range hub.Canvases {
		if session.SessionID != "" && (session.SessionID == canvas.ID || session.SessionID == canvas.RunID) {
			return types.MatchResult{
				Status:    "exact",
				Reason:    "session id matched an AgentCanvas canvas id or run id",
				CanvasID:  canvas.ID,
				RunID:     canvas.RunID,
				Score:     95,
				Evidence:  "canvas.id/runId",
				Provider:  canvas.AgentID,
				UpdatedAt: canvas.UpdatedAt,
			}
		}
	}
	best := types.MatchResult{Status: "unmatched", Reason: "no matching AgentCanvas run or canvas found"}
	for _, run := range hub.Runs {
		if !providerCompatible(session.Provider, run.Provider) {
			continue
		}
		runCWD := runWorkspace(run)
		if runCWD == "" || !core.SamePath(session.CWD, runCWD) {
			continue
		}
		sessionUpdated := core.TimeFromRFC3339(session.UpdatedAt)
		runUpdated := core.ParseTime(run.UpdatedAt)
		if !sessionUpdated.IsZero() && !runUpdated.IsZero() && core.AbsDuration(sessionUpdated.Sub(runUpdated)) > types.LikelyMatchWindow {
			continue
		}
		score := 70
		if !sessionUpdated.IsZero() && !runUpdated.IsZero() {
			hours := int(core.AbsDuration(sessionUpdated.Sub(runUpdated)).Hours())
			score -= core.MinInt(hours, 20)
		}
		if score > best.Score {
			best = types.MatchResult{
				Status:    "likely",
				Reason:    "provider and workspace matched within the recent update window",
				CanvasID:  run.CanvasID,
				RunID:     core.FirstNonEmpty(run.ID, run.RunID),
				Score:     score,
				Evidence:  "provider+cwd+updatedAt",
				Provider:  run.Provider,
				UpdatedAt: run.UpdatedAt,
			}
		}
	}
	return best
}

func sessionIDMatchesRun(sessionID string, run types.HubRun) bool {
	if sessionID == "" {
		return false
	}
	values := []string{run.ID, run.RunID, run.ExternalID, run.CanvasID, run.FeedbackTarget.ExternalID}
	for _, value := range values {
		if value == sessionID {
			return true
		}
	}
	return false
}

func providerCompatible(localProvider, hubProvider string) bool {
	switch localProvider {
	case "codex":
		return hubProvider == "codex" || hubProvider == "codex_exec" || hubProvider == ""
	case "claude":
		return hubProvider == "claude" || hubProvider == "claude_cli" || hubProvider == ""
	case "cursor":
		return hubProvider == "cursor" || hubProvider == "cursor_cli" || hubProvider == ""
	default:
		return localProvider == hubProvider || hubProvider == ""
	}
}

func runWorkspace(run types.HubRun) string {
	for _, value := range []string{
		run.FeedbackTarget.CWD,
		run.Metadata["cwd"],
		run.Metadata["workspace"],
		run.Metadata["sourceCwd"],
		run.FeedbackTarget.Metadata["cwd"],
		run.FeedbackTarget.Metadata["workspace"],
	} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func NextCanvasVersion(canvasID string, hub types.HubState) int {
	version := 1
	for _, canvas := range hub.Canvases {
		if canvas.ID == canvasID && canvas.Version >= version {
			version = canvas.Version + 1
		}
	}
	return version
}
