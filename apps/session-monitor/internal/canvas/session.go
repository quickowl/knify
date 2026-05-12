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
		"Now: " + review.CurrentState,
		"Next: " + review.NextStep,
	}
	return strings.Join(parts, " - ")
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
	if len(session.LatestMessages) > 0 {
		fmt.Fprintf(&b, "\n### Latest message summaries\n\n")
		for _, message := range session.LatestMessages {
			fmt.Fprintf(&b, "- **%s:** %s\n", message.Role, message.Text)
		}
	}
	return core.Truncate(b.String(), sessionMarkdownCap)
}

