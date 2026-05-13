package canvas

import (
	"fmt"
	"strings"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func sessionListSubtitle(session types.LocalSession, now time.Time) string {
	review := Review(session, now)
	parts := []string{
		"Purpose: " + review.Purpose,
	}
	if session.Plan.Key != "" {
		parts = append(parts, "Plan: "+planLabel(session.Plan))
	}
	parts = append(parts, "Now: "+review.CurrentState, fmt.Sprintf("Evidence: %s (%d)", review.EvidenceStatus, len(session.Artifacts)), "Next: "+review.NextStep)
	return strings.Join(parts, " - ")
}

func sessionAttention(session types.LocalSession, now time.Time) string {
	updated := core.TimeFromRFC3339(session.UpdatedAt)
	age := now.Sub(updated)
	if !updated.IsZero() && age >= 0 && age <= 5*time.Minute && strings.EqualFold(session.Status, "busy") {
		return "running"
	}
	if strings.EqualFold(session.Provider, "codex") && sessionActivityStatus(session, now) == "active" {
		return "running"
	}
	return "idle"
}

func sessionActivityStatus(session types.LocalSession, now time.Time) string {
	updated := core.TimeFromRFC3339(session.UpdatedAt)
	if updated.IsZero() {
		return "unknown"
	}
	age := now.Sub(updated)
	if age < 0 {
		if age >= -5*time.Minute {
			return "active"
		}
		return "future"
	}
	switch {
	case age <= 15*time.Minute:
		return "active"
	case age <= 2*time.Hour:
		return "recent"
	case age <= 24*time.Hour:
		return "idle"
	default:
		return "stale"
	}
}

func sessionAgeLabel(session types.LocalSession, now time.Time) string {
	updated := core.TimeFromRFC3339(session.UpdatedAt)
	if updated.IsZero() {
		return "unknown age"
	}
	age := now.Sub(updated)
	if age < 0 {
		if age >= -5*time.Minute {
			return "just now"
		}
		return "future update"
	}
	if age < time.Minute {
		return "just now"
	}
	if age < time.Hour {
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	}
	if age < 48*time.Hour {
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(age.Hours()/24))
}

func latestMessagePreview(session types.LocalSession) string {
	if len(session.LatestMessages) == 0 {
		return ""
	}
	message := session.LatestMessages[len(session.LatestMessages)-1]
	text := strings.Join(strings.Fields(message.Text), " ")
	if text == "" {
		return ""
	}
	return fmt.Sprintf("**%s:** %s", core.FirstNonEmpty(message.Role, "message"), core.Truncate(text, recentPreviewLimit))
}

func latestMessagePlain(session types.LocalSession) string {
	if len(session.LatestMessages) == 0 {
		return ""
	}
	message := session.LatestMessages[len(session.LatestMessages)-1]
	text := strings.Join(strings.Fields(message.Text), " ")
	if text == "" {
		return ""
	}
	return fmt.Sprintf("%s: %s", core.FirstNonEmpty(message.Role, "message"), core.Truncate(text, recentPreviewLimit))
}

func scanWarningsBlock(scanErrors []string) map[string]any {
	metadata := map[string]any{"count": len(scanErrors)}
	for i, err := range scanErrors {
		metadata[fmt.Sprintf("warning%02d", i+1)] = core.Truncate(err, 500)
	}
	return map[string]any{
		"id":       "session-monitor-scan-warnings",
		"kind":     "metadata",
		"title":    "Scan warnings",
		"metadata": metadata,
	}
}

func providerHealthBlock(health []types.ProviderHealth) map[string]any {
	metadata := map[string]any{}
	for _, item := range health {
		prefix := item.Provider + "."
		metadata[prefix+"status"] = item.Status
		for key, value := range item.Details {
			metadata[prefix+key] = value
		}
		if len(item.Warnings) > 0 {
			metadata[prefix+"warnings"] = strings.Join(item.Warnings, "; ")
		}
	}
	return map[string]any{
		"id":       "session-monitor-provider-health",
		"kind":     "metadata",
		"title":    "Provider health",
		"metadata": metadata,
	}
}

func daemonBuildBlock(build types.DaemonBuildInfo) map[string]any {
	return map[string]any{
		"id":       "session-monitor-daemon-build",
		"kind":     "metadata",
		"title":    "Daemon build",
		"metadata": daemonBuildMetadata(build),
	}
}

func daemonBuildMetadata(build types.DaemonBuildInfo) map[string]any {
	return map[string]any{
		"build":            daemonBuildLabel(build),
		"version":          build.Version,
		"revision":         build.Revision,
		"revisionShort":    build.RevisionShort,
		"commitTime":       build.CommitTime,
		"modified":         build.Modified,
		"goVersion":        build.GoVersion,
		"binaryPath":       build.BinaryPath,
		"binaryModifiedAt": build.BinaryModifiedAt,
		"binarySize":       build.BinarySize,
	}
}

func daemonBuildLabel(build types.DaemonBuildInfo) string {
	label := core.FirstNonEmpty(build.RevisionShort, build.Version, "dev")
	if build.Modified && !strings.Contains(label, "dirty") {
		label += "+dirty"
	}
	return label
}

func nextStepsBlock() map[string]any {
	return map[string]any{
		"id":    "session-monitor-next-steps",
		"kind":  "checklist",
		"title": "V1 boundaries",
		"items": []map[string]any{
			{"id": "boundary-read-only", "text": "Provider stores were scanned read-only.", "checked": true},
			{"id": "boundary-no-nudge", "text": "No session was resumed or nudged by this daemon.", "checked": true},
			{"id": "boundary-cursor-v2", "text": "Cursor conversation parsing remains a v2 follow-up.", "checked": false},
		},
	}
}

func sessionMarkdown(session types.LocalSession, now time.Time) string {
	var b strings.Builder
	review := Review(session, now)
	fmt.Fprintf(&b, "## %s\n\n", session.Title)
	fmt.Fprintf(&b, "### Review shape\n\n")
	fmt.Fprintf(&b, "- Purpose: %s\n", review.Purpose)
	fmt.Fprintf(&b, "- Now: %s\n", review.CurrentState)
	fmt.Fprintf(&b, "- Evidence: %s (%d artifact%s)\n", review.EvidenceStatus, len(session.Artifacts), pluralS(len(session.Artifacts)))
	fmt.Fprintf(&b, "- Next: %s\n", review.NextStep)
	if len(review.Signals) > 0 {
		fmt.Fprintf(&b, "- Signals: %s\n", strings.Join(review.Signals, "; "))
	}
	fmt.Fprintf(&b, "\n### Session facts\n\n")
	fmt.Fprintf(&b, "- Provider: `%s`\n", session.Provider)
	fmt.Fprintf(&b, "- Session: `%s`\n", session.SessionID)
	if session.CWD != "" {
		fmt.Fprintf(&b, "- Workspace: `%s`\n", session.CWD)
	}
	fmt.Fprintf(&b, "- Updated: %s\n", core.FirstNonEmpty(session.UpdatedAt, "unknown"))
	fmt.Fprintf(&b, "- Match: **%s**", session.Match.Status)
	if session.Match.CanvasID != "" || session.Match.RunID != "" {
		fmt.Fprintf(&b, " (`%s` / `%s`)", core.FirstNonEmpty(session.Match.CanvasID, "no canvas"), core.FirstNonEmpty(session.Match.RunID, "no run"))
	}
	if session.Match.Reason != "" {
		fmt.Fprintf(&b, " - %s", session.Match.Reason)
	}
	fmt.Fprintf(&b, "\n")
	if session.ResumeHint != "" {
		fmt.Fprintf(&b, "- Resume hint: `%s`\n", session.ResumeHint)
	}
	if session.Plan.Key != "" {
		fmt.Fprintf(&b, "\n### Plan\n\n")
		fmt.Fprintf(&b, "- Plan: %s\n", planLabel(session.Plan))
		fmt.Fprintf(&b, "- Source: `%s`\n", core.FirstNonEmpty(session.Plan.Source, "unknown"))
		if session.Plan.FilePath != "" {
			fmt.Fprintf(&b, "- File: `%s`\n", session.Plan.FilePath)
		}
		if session.Plan.UpdatedAt != "" {
			fmt.Fprintf(&b, "- Updated: %s\n", session.Plan.UpdatedAt)
		}
		if session.Plan.Summary != "" {
			fmt.Fprintf(&b, "- Summary: %s\n", session.Plan.Summary)
		}
		for _, item := range session.Plan.Items {
			prefix := "-"
			if item.Status != "" {
				prefix = fmt.Sprintf("- `%s`", item.Status)
			}
			fmt.Fprintf(&b, "%s %s\n", prefix, item.Text)
		}
	}
	if outputs := outputsSection(session); outputs != "" {
		fmt.Fprintf(&b, "\n### Outputs to check\n\n%s", outputs)
	}
	if len(session.Artifacts) > 0 {
		fmt.Fprintf(&b, "\n### Review artifacts\n\n")
		for _, artifact := range session.Artifacts {
			fmt.Fprintf(&b, "- %s: %s", artifact.Kind, artifact.Title)
			if artifact.Summary != "" {
				fmt.Fprintf(&b, " - %s", artifact.Summary)
			}
			fmt.Fprintf(&b, "\n")
		}
	}
	if len(session.LatestMessages) > 0 {
		fmt.Fprintf(&b, "\n### Latest message summaries\n\n")
		for _, message := range session.LatestMessages {
			fmt.Fprintf(&b, "- **%s:** %s\n", message.Role, message.Text)
		}
	}
	return core.Truncate(b.String(), sessionMarkdownCap)
}

func pluralS(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func planLabel(plan types.SessionPlan) string {
	label := core.FirstNonEmpty(plan.Title, plan.Key, "untitled plan")
	if plan.Status != "" {
		return fmt.Sprintf("%s (%s)", label, plan.Status)
	}
	return label
}

func planItemsSummary(plan types.SessionPlan) string {
	if len(plan.Items) == 0 {
		return ""
	}
	items := make([]string, 0, len(plan.Items))
	for _, item := range plan.Items {
		if item.Status != "" {
			items = append(items, item.Status+": "+item.Text)
		} else {
			items = append(items, item.Text)
		}
	}
	return strings.Join(items, "; ")
}

func outputsSection(session types.LocalSession) string {
	var b strings.Builder
	var lastAssistant *types.MessageSummary
	for i := len(session.LatestMessages) - 1; i >= 0; i-- {
		message := session.LatestMessages[i]
		if strings.EqualFold(message.Role, "assistant") && strings.TrimSpace(message.Text) != "" {
			msg := message
			lastAssistant = &msg
			break
		}
	}
	if lastAssistant != nil {
		text := core.Truncate(strings.Join(strings.Fields(lastAssistant.Text), " "), 600)
		fmt.Fprintf(&b, "- Last assistant turn: %s\n", text)
	}
	signals := []string{}
	for _, message := range session.LatestMessages {
		text := strings.ToLower(message.Text)
		if strings.Contains(text, "error") || strings.Contains(text, "failed") || strings.Contains(text, "panic") {
			snippet := core.Truncate(strings.Join(strings.Fields(message.Text), " "), 240)
			signals = append(signals, fmt.Sprintf("- Possible error/warning (%s): %s\n", core.FirstNonEmpty(message.Role, "message"), snippet))
			if len(signals) >= 3 {
				break
			}
		}
	}
	for _, line := range signals {
		b.WriteString(line)
	}
	if collapsed := session.Metadata["collapsedFiles"]; collapsed != "" {
		fmt.Fprintf(&b, "- Collapsed local files: %s\n", collapsed)
	}
	if session.ResumeHint != "" {
		fmt.Fprintf(&b, "- Resume hint: `%s`\n", session.ResumeHint)
	}
	return b.String()
}
