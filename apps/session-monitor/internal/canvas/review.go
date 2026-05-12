package canvas

import (
	"fmt"
	"strings"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func FillSessionReviews(sessions []types.LocalSession, now time.Time) {
	for i := range sessions {
		sessions[i].Review = Review(sessions[i], now)
	}
}

func Review(session types.LocalSession, now time.Time) types.SessionReview {
	if session.Review.Purpose != "" && session.Review.CurrentState != "" && session.Review.NextStep != "" {
		return session.Review
	}
	purpose := inferSessionPurpose(session)
	current := inferCurrentState(session, now)
	next := inferNextStep(session, now)
	signals := []string{
		fmt.Sprintf("provider %s, status %s, activity %s (%s)", session.Provider, core.FirstNonEmpty(session.Status, "unknown"), sessionActivityStatus(session, now), sessionAgeLabel(session, now)),
		fmt.Sprintf("match %s", core.FirstNonEmpty(session.Match.Status, "unmatched")),
	}
	if session.CWD != "" {
		signals = append(signals, "workspace "+core.DisplayCWD(session.CWD))
	}
	if collapsed := session.Metadata["collapsedFiles"]; collapsed != "" {
		signals = append(signals, "collapsed "+collapsed+" local files into this session")
	}
	if session.Match.CanvasID != "" || session.Match.RunID != "" {
		signals = append(signals, "linked candidate "+core.FirstNonEmpty(session.Match.CanvasID, "no canvas")+" / "+core.FirstNonEmpty(session.Match.RunID, "no run"))
	}
	if latest := latestMessagePlain(session); latest != "" {
		signals = append(signals, "latest "+latest)
	}
	return types.SessionReview{
		Purpose:      core.Truncate(purpose, 180),
		CurrentState: core.Truncate(current, 220),
		NextStep:     core.Truncate(next, 180),
		Signals:      capStrings(signals, 5),
	}
}

func inferSessionPurpose(session types.LocalSession) string {
	if title := strings.TrimSpace(session.Title); title != "" {
		return title
	}
	for _, message := range session.LatestMessages {
		if strings.EqualFold(message.Role, "user") && strings.TrimSpace(message.Text) != "" {
			return "Respond to: " + strings.Join(strings.Fields(message.Text), " ")
		}
	}
	if session.CWD != "" {
		return fmt.Sprintf("Track %s activity in %s", core.DisplayProvider(session.Provider), core.DisplayCWD(session.CWD))
	}
	return fmt.Sprintf("Track %s session %s", core.DisplayProvider(session.Provider), core.ShortID(session.SessionID))
}

func inferCurrentState(session types.LocalSession, now time.Time) string {
	parts := []string{
		fmt.Sprintf("%s, %s", core.FirstNonEmpty(session.Status, "unknown"), sessionActivityStatus(session, now)),
		sessionAgeLabel(session, now),
		fmt.Sprintf("match %s", core.FirstNonEmpty(session.Match.Status, "unmatched")),
	}
	if collapsed := session.Metadata["collapsedFiles"]; collapsed != "" {
		parts = append(parts, "collapsed "+collapsed+" files")
	}
	if latest := latestMessagePlain(session); latest != "" {
		parts = append(parts, latest)
	}
	return strings.Join(parts, "; ")
}

func inferNextStep(session types.LocalSession, now time.Time) string {
	match := core.FirstNonEmpty(session.Match.Status, "unmatched")
	activity := sessionActivityStatus(session, now)
	if match == "exact" {
		return "Open the linked canvas/run and review the latest session output."
	}
	if match == "likely" {
		return "Confirm the likely canvas/run link before treating this as attached."
	}
	if activity == "active" || strings.EqualFold(session.Status, "busy") {
		return "Keep watching; attach this session to a canvas if it needs reviewer attention."
	}
	if collapsed := session.Metadata["collapsedFiles"]; collapsed != "" {
		return "Review the parent row first; expand metadata only if the collapsed subagent files matter."
	}
	return "Decide whether this unmatched session should be linked, ignored, or used as resume context."
}
