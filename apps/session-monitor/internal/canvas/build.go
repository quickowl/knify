package canvas

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

const (
	sessionMarkdownCap = 3000
	recentMarkdownCap  = 12000
	recentPreviewLimit = 180
)

func BuildCanvas(cfg types.Config, sessions []types.LocalSession, health []types.ProviderHealth, scanErrors []string, hub types.HubState, version int, now time.Time) types.Canvas {
	sessions = sortSessionsByUpdatedAt(sessions)
	FillSessionReviews(sessions, now)
	counts := countMatches(sessions)
	planCounts := countPlans(sessions)
	evidenceCounts := countEvidence(sessions, now)
	summary := fmt.Sprintf("Scanned %d local Codex/Claude sessions: %d exact, %d likely, %d unmatched, %d with plans, %d evidence-ready. Daemon %s.", len(sessions), counts["exact"], counts["likely"], counts["unmatched"], planCounts["sessions"], evidenceCounts["ready"], daemonBuildLabel(cfg.Build))
	blocks := []map[string]any{
		{
			"id":    "session-monitor-heading",
			"kind":  "heading",
			"level": 1,
			"text":  types.SessionMonitorTitle,
		},
		{
			"id":       "session-monitor-overview",
			"kind":     "markdown",
			"markdown": overviewMarkdown(cfg, sessions, health, scanErrors, hub, now),
		},
		{
			"id":    "session-monitor-scan-metadata",
			"kind":  "metadata",
			"title": "Scan metadata",
			"metadata": map[string]any{
				"lookback":     cfg.Lookback.String(),
				"maxSessions":  cfg.MaxSessions,
				"codexHome":    cfg.CodexHome,
				"claudeHome":   cfg.ClaudeHome,
				"hubUrl":       cfg.HubURL,
				"canvasCount":  len(hub.Canvases),
				"runCount":     len(hub.Runs),
				"nudgeEnabled": false,
				"daemonBuild":  daemonBuildLabel(cfg.Build),
				"daemonBinary": cfg.Build.BinaryPath,
			},
		},
	}
	items := make([]map[string]any, 0, len(sessions))
	for i, session := range sessions {
		review := Review(session, now)
		blockID := fmt.Sprintf("session-%02d-summary", i+1)
		metaID := fmt.Sprintf("session-%02d-metadata", i+1)
		label := fmt.Sprintf("%s: %s", core.DisplayProvider(session.Provider), session.Title)
		attention := sessionAttention(session, now)
		badges := []string{session.Provider, core.FirstNonEmpty(session.Status, "unknown"), sessionActivityStatus(session, now), session.Match.Status, "attention:" + attention, "evidence:" + review.EvidenceStatus}
		if collapsed := session.Metadata["collapsedFiles"]; collapsed != "" {
			badges = append(badges, "collapsed:"+collapsed)
		}
		if len(session.Artifacts) > 0 {
			badges = append(badges, fmt.Sprintf("artifacts:%d", len(session.Artifacts)))
		}
		if session.Plan.Key != "" {
			badges = append(badges, "plan")
			if shared := planCounts[session.Plan.Key]; shared > 1 {
				badges = append(badges, fmt.Sprintf("shared-plan:%d", shared))
			}
		}
		childBlocks := []map[string]any{
			{
				"id":       blockID,
				"kind":     "markdown",
				"markdown": sessionMarkdown(session, now),
			},
		}
		childBlockIDs := []string{blockID}
		for _, artifactBlock := range sessionArtifactBlocks(i+1, session) {
			childBlocks = append(childBlocks, artifactBlock)
			childBlockIDs = append(childBlockIDs, core.StringValue(artifactBlock["id"]))
		}
		if nudgeBlock := manualNudgeBlock(i+1, session, review); nudgeBlock != nil {
			childBlocks = append(childBlocks, nudgeBlock)
			childBlockIDs = append(childBlockIDs, core.StringValue(nudgeBlock["id"]))
		}
		childBlocks = append(childBlocks, map[string]any{
			"id":    metaID,
			"kind":  "metadata",
			"title": "Session metadata",
			"metadata": map[string]any{
				"provider":       session.Provider,
				"sessionId":      session.SessionID,
				"cwd":            session.CWD,
				"status":         session.Status,
				"sourcePath":     session.SourcePath,
				"createdAt":      session.CreatedAt,
				"updatedAt":      session.UpdatedAt,
				"resumeHint":     session.ResumeHint,
				"artifactCount":  len(session.Artifacts),
				"evidenceStatus": review.EvidenceStatus,
				"nudgePrompt":    review.NudgePrompt,
				"artifactTitles": artifactTitles(session.Artifacts),
				"matchStatus":    session.Match.Status,
				"matchReason":    session.Match.Reason,
				"matchCanvas":    session.Match.CanvasID,
				"matchRun":       session.Match.RunID,
				"matchScore":     session.Match.Score,
				"matchEvidence":  session.Match.Evidence,
				"purpose":        review.Purpose,
				"currentState":   review.CurrentState,
				"nextStep":       review.NextStep,
				"signals":        strings.Join(review.Signals, "; "),
				"planKey":        session.Plan.Key,
				"planTitle":      session.Plan.Title,
				"planSource":     session.Plan.Source,
				"planStatus":     session.Plan.Status,
				"planUpdatedAt":  session.Plan.UpdatedAt,
				"planFilePath":   session.Plan.FilePath,
				"planSummary":    session.Plan.Summary,
				"planItems":      planItemsSummary(session.Plan),
			},
		})
		childBlockIDs = append(childBlockIDs, metaID)
		items = append(items, map[string]any{
			"id":       fmt.Sprintf("session-%02d", i+1),
			"label":    core.Truncate(label, 120),
			"subtitle": core.Truncate(sessionListSubtitle(session, now), 420),
			"status":   session.Match.Status,
			"badges":   badges,
			"addedAt":  core.FirstNonEmpty(session.UpdatedAt, core.FormatTime(now)),
			"blockIds": childBlockIDs,
		})
		blocks = append(blocks, childBlocks...)
	}
	pageSize := cfg.MaxSessions
	if pageSize <= 0 {
		pageSize = types.DefaultMaxSessions
	}
	collection := map[string]any{
		"id":       "session-monitor-collection",
		"kind":     "collection",
		"title":    "Recent sessions (newest first)",
		"mode":     "paged-list",
		"pageSize": pageSize,
		"items":    items,
	}
	blocks = append(blocks[:1], append([]map[string]any{collection}, blocks[1:]...)...)
	if len(scanErrors) > 0 {
		blocks = append(blocks, scanWarningsBlock(scanErrors))
	}
	blocks = append(blocks, daemonBuildBlock(cfg.Build), providerHealthBlock(health), nextStepsBlock())
	return types.Canvas{
		ID:        cfg.CanvasID,
		AgentID:   types.DefaultAgentID,
		RunID:     cfg.RunID,
		Title:     types.SessionMonitorTitle,
		Summary:   summary,
		Status:    "ready_for_review",
		Mode:      "static",
		Priority:  "normal",
		CreatedAt: core.FormatTime(now),
		UpdatedAt: core.FormatTime(now),
		Version:   version,
		Blocks:    blocks,
	}
}

func sortSessionsByUpdatedAt(sessions []types.LocalSession) []types.LocalSession {
	out := append([]types.LocalSession(nil), sessions...)
	sort.SliceStable(out, func(i, j int) bool {
		left := core.TimeFromRFC3339(out[i].UpdatedAt)
		right := core.TimeFromRFC3339(out[j].UpdatedAt)
		if left.Equal(right) {
			return false
		}
		if left.IsZero() {
			return false
		}
		if right.IsZero() {
			return true
		}
		return left.After(right)
	})
	return out
}

func CompactDynamicCanvas(canvas types.Canvas) types.Canvas {
	keep := map[string]bool{
		"session-monitor-heading":         true,
		"session-monitor-overview":        true,
		"session-monitor-collection":      true,
		"session-monitor-scan-metadata":   true,
		"session-monitor-scan-warnings":   true,
		"session-monitor-daemon-build":    true,
		"session-monitor-provider-health": true,
		"session-monitor-next-steps":      true,
	}
	blocks := make([]map[string]any, 0, len(keep))
	for _, block := range canvas.Blocks {
		blockID := core.StringValue(block["id"])
		if keep[blockID] || strings.HasPrefix(blockID, "session-") {
			blocks = append(blocks, block)
		}
	}
	canvas.Blocks = blocks
	canvas.Mode = "dynamic"
	return canvas
}

func overviewMarkdown(cfg types.Config, sessions []types.LocalSession, health []types.ProviderHealth, scanErrors []string, hub types.HubState, now time.Time) string {
	counts := countMatches(sessions)
	planCounts := countPlans(sessions)
	evidenceCounts := countEvidence(sessions, now)
	providers := map[string]int{}
	activity := map[string]int{}
	for _, session := range sessions {
		providers[session.Provider]++
		activity[sessionActivityStatus(session, now)]++
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## What this view is for\n\n")
	fmt.Fprintf(&b, "- Purpose: turn scattered local Codex and Claude work into a review queue with enough context to decide what needs attention.\n")
	fmt.Fprintf(&b, "- Current state: newest sessions are sorted by update time, grouped by provider/session id, and labelled with match status against AgentCanvas hub runs/canvases.\n")
	fmt.Fprintf(&b, "- Next decision: open rows that are active, unmatched, missing evidence, or collapsed; link useful sessions to canvases/runs later. V1 still does not auto-nudge or auto-resume sessions.\n\n")
	fmt.Fprintf(&b, "## Scan summary\n\n")
	fmt.Fprintf(&b, "- Sessions: %d\n", len(sessions))
	fmt.Fprintf(&b, "- Codex sessions: %d\n", providers["codex"])
	fmt.Fprintf(&b, "- Claude sessions: %d\n", providers["claude"])
	fmt.Fprintf(&b, "- Activity: active %d, recent %d, idle %d, stale %d\n", activity["active"], activity["recent"], activity["idle"], activity["stale"])
	fmt.Fprintf(&b, "- Matches: exact %d, likely %d, unmatched %d\n", counts["exact"], counts["likely"], counts["unmatched"])
	fmt.Fprintf(&b, "- Plans: %d sessions with plans, %d distinct plans, %d shared plans\n", planCounts["sessions"], planCounts["distinct"], planCounts["shared"])
	fmt.Fprintf(&b, "- Evidence: ready %d, pending %d, missing %d\n", evidenceCounts["ready"], evidenceCounts["pending"], evidenceCounts["missing"])
	fmt.Fprintf(&b, "- Daemon: %s", daemonBuildLabel(cfg.Build))
	if cfg.Build.BinaryModifiedAt != "" {
		fmt.Fprintf(&b, "; binary %s", cfg.Build.BinaryModifiedAt)
	}
	if cfg.Build.GoVersion != "" {
		fmt.Fprintf(&b, "; %s", cfg.Build.GoVersion)
	}
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "- Hub inventory: %d canvases, %d agent runs\n", len(hub.Canvases), len(hub.Runs))
	fmt.Fprintf(&b, "- Nudge delivery: manual only in v1; resume hints are recorded but never executed by the daemon.\n")
	if len(scanErrors) > 0 {
		fmt.Fprintf(&b, "- Scan warnings: %d, collapsed in the scan warning metadata block.\n", len(scanErrors))
	}
	if len(health) > 0 {
		fmt.Fprintf(&b, "\n### Provider health\n\n")
		for _, item := range health {
			fmt.Fprintf(&b, "- %s: %s\n", item.Provider, item.Status)
		}
	}
	return b.String()
}

func countEvidence(sessions []types.LocalSession, now time.Time) map[string]int {
	counts := map[string]int{"ready": 0, "pending": 0, "missing": 0}
	for _, session := range sessions {
		counts[inferEvidenceStatus(session, now)]++
	}
	return counts
}

func countPlans(sessions []types.LocalSession) map[string]int {
	counts := map[string]int{"sessions": 0, "distinct": 0, "shared": 0}
	keys := map[string]int{}
	for _, session := range sessions {
		if session.Plan.Key == "" {
			continue
		}
		counts["sessions"]++
		keys[session.Plan.Key]++
	}
	counts["distinct"] = len(keys)
	for _, count := range keys {
		if count > 1 {
			counts["shared"]++
		}
	}
	for key, count := range keys {
		counts[key] = count
	}
	return counts
}

func countMatches(sessions []types.LocalSession) map[string]int {
	counts := map[string]int{"exact": 0, "likely": 0, "unmatched": 0}
	for _, session := range sessions {
		status := session.Match.Status
		if status == "" {
			status = "unmatched"
		}
		counts[status]++
	}
	return counts
}

func capStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return append([]string(nil), values[:limit]...)
}
