package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/canvas"
	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/hub"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func WriteStatus(ctx context.Context, cfg types.Config, w io.Writer) error {
	path := statusOutputPath(cfg)
	staleAfter := cfg.StaleAfter
	if staleAfter <= 0 {
		staleAfter = types.DefaultStaleAfter
	}
	now := cfg.Now().UTC()
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(w, "session-monitor status: down\nheartbeat: missing %s (%v)\n", path, err)
		return errors.New("session-monitor status is not ok")
	}
	var result types.ScanResult
	if err := json.Unmarshal(data, &result); err != nil {
		fmt.Fprintf(w, "session-monitor status: down\nheartbeat: unreadable %s (%v)\n", path, err)
		return errors.New("session-monitor status is not ok")
	}

	ok := true
	generatedAt := core.ParseTime(result.GeneratedAt)
	age := now.Sub(generatedAt)
	if generatedAt.IsZero() || age > staleAfter {
		ok = false
	}
	if !result.Posted {
		ok = false
	}
	if len(result.Errors) > 0 {
		ok = false
	}
	if !requiredProvidersOK(result.ProviderHealth) {
		ok = false
	}

	processLine := "process: unknown (heartbeat predates process metadata)"
	if result.Process.PID > 0 {
		alive := processAlive(result.Process.PID)
		if !alive {
			ok = false
		}
		processLine = fmt.Sprintf("process: pid=%d alive=%v mode=%s interval=%s dynamic=%v", result.Process.PID, alive, core.FirstNonEmpty(result.Process.Mode, "unknown"), result.Process.Interval, result.Process.Dynamic)
	}

	hubLine := "hub: not checked"
	hubURL := core.FirstNonEmpty(cfg.HubURL, result.Hub.URL)
	if hubURL != "" {
		hubCfg := cfg
		hubCfg.HubURL = hubURL
		hubCfg.CanvasID = core.FirstNonEmpty(cfg.CanvasID, result.Canvas.ID, types.DefaultCanvasID)
		hubCanvas, exists, err := hub.FetchHubCanvas(ctx, hubCfg, hubCfg.CanvasID)
		if err != nil {
			ok = false
			hubLine = fmt.Sprintf("hub: error %s", err)
		} else if !exists {
			ok = false
			hubLine = fmt.Sprintf("hub: missing canvas=%s", hubCfg.CanvasID)
		} else {
			mode, items, found := collectionStats(hubCanvas)
			hubAge := now.Sub(core.ParseTime(hubCanvas.UpdatedAt))
			if core.ParseTime(hubCanvas.UpdatedAt).IsZero() || hubAge > staleAfter || !found {
				ok = false
			}
			hubLine = fmt.Sprintf("hub: ok canvas=%s version=%d updated=%s ago blocks=%d collection=%s items=%d", hubCanvas.ID, hubCanvas.Version, durationLabel(hubAge), len(hubCanvas.Blocks), mode, items)
		}
	}

	fmt.Fprintf(w, "session-monitor status: %s\n", statusWord(ok))
	if generatedAt.IsZero() {
		fmt.Fprintf(w, "heartbeat: invalid generatedAt in %s\n", path)
	} else {
		fmt.Fprintf(w, "heartbeat: %s age=%s file=%s generatedAt=%s\n", freshnessWord(age, staleAfter), durationLabel(age), path, result.GeneratedAt)
	}
	fmt.Fprintln(w, processLine)
	fmt.Fprintln(w, hubLine)
	fmt.Fprintf(w, "sessions: total=%d codex=%d claude=%d exact=%d likely=%d unmatched=%d errors=%d posted=%v dynamic=%v eventsPosted=%d\n",
		len(result.Sessions),
		countSessions(result.Sessions, "codex"),
		countSessions(result.Sessions, "claude"),
		countMatchStatus(result.Sessions, "exact"),
		countMatchStatus(result.Sessions, "likely"),
		countMatchStatus(result.Sessions, "unmatched"),
		len(result.Errors),
		result.Posted,
		result.Dynamic,
		result.EventsPosted,
	)
	fmt.Fprintln(w, "providers:")
	for _, item := range result.ProviderHealth {
		fmt.Fprintf(w, "  - %s: %s", item.Provider, item.Status)
		if sessions := item.Details["sessions"]; sessions != "" {
			fmt.Fprintf(w, " sessions=%s", sessions)
		}
		if rawFiles := item.Details["rawFiles"]; rawFiles != "" {
			fmt.Fprintf(w, " rawFiles=%s", rawFiles)
		}
		if collapsed := item.Details["collapsedFiles"]; collapsed != "" {
			fmt.Fprintf(w, " collapsedFiles=%s", collapsed)
		}
		if home := item.Details["home"]; home != "" {
			fmt.Fprintf(w, " home=%s", home)
		}
		if len(item.Warnings) > 0 {
			fmt.Fprintf(w, " warnings=%d", len(item.Warnings))
		}
		fmt.Fprintln(w)
	}
	if cfg.RecentLimit > 0 {
		fmt.Fprintln(w, "recent:")
		limit := cfg.RecentLimit
		if limit > len(result.Sessions) {
			limit = len(result.Sessions)
		}
		for i := 0; i < limit; i++ {
			fmt.Fprintf(w, "  %2d. %s\n", i+1, statusSessionLine(result.Sessions[i], now))
		}
	}
	if !ok {
		return errors.New("session-monitor status is not ok")
	}
	return nil
}

func ProcessInfo(cfg types.Config) types.ProcessInfo {
	mode := "once"
	if cfg.Watch {
		mode = "watch"
	}
	return types.ProcessInfo{
		PID:      os.Getpid(),
		Mode:     mode,
		Watch:    cfg.Watch,
		Dynamic:  cfg.Dynamic,
		Interval: cfg.Interval.String(),
		HubURL:   cfg.HubURL,
		CanvasID: cfg.CanvasID,
		RunID:    cfg.RunID,
		OutPath:  cfg.OutPath,
	}
}

func statusOutputPath(cfg types.Config) string {
	if cfg.OutPath != "" {
		return cfg.OutPath
	}
	return types.DefaultStatusPath
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func requiredProvidersOK(health []types.ProviderHealth) bool {
	found := map[string]bool{}
	for _, item := range health {
		if (item.Provider == "codex" || item.Provider == "claude") && item.Status == "ok" {
			found[item.Provider] = true
		}
	}
	return found["codex"] && found["claude"]
}

func collectionStats(c types.Canvas) (string, int, bool) {
	for _, block := range c.Blocks {
		if core.StringValue(block["id"]) != "session-monitor-collection" {
			continue
		}
		switch items := block["items"].(type) {
		case []any:
			return core.FirstNonEmpty(core.StringValue(block["mode"]), "unknown"), len(items), true
		case []map[string]any:
			return core.FirstNonEmpty(core.StringValue(block["mode"]), "unknown"), len(items), true
		default:
			return core.FirstNonEmpty(core.StringValue(block["mode"]), "unknown"), 0, true
		}
	}
	return "missing", 0, false
}

func statusSessionLine(session types.LocalSession, now time.Time) string {
	review := canvas.Review(session, now)
	updated := core.ParseTime(session.UpdatedAt)
	age := "unknown"
	if !updated.IsZero() {
		age = durationLabel(now.Sub(updated)) + " ago"
	}
	title := core.FirstNonEmpty(session.Title, core.ShortID(session.SessionID), "untitled")
	workspace := compactPath(session.CWD)
	latest := ""
	if len(session.LatestMessages) > 0 {
		msg := session.LatestMessages[len(session.LatestMessages)-1]
		latest = " - " + core.Truncate(strings.TrimSpace(msg.Role+": "+msg.Text), 90)
	}
	collapsed := ""
	if count := session.Metadata["collapsedFiles"]; count != "" {
		collapsed = " collapsed=" + count
	}
	return fmt.Sprintf("%s %s/%s match=%s updated=%s workspace=%s title=%q%s%s\n      purpose: %s\n      now: %s\n      next: %s",
		core.ShortID(session.SessionID),
		session.Provider,
		core.FirstNonEmpty(session.Status, "unknown"),
		core.FirstNonEmpty(session.Match.Status, "unmatched"),
		age,
		workspace,
		core.Truncate(title, 64),
		collapsed,
		latest,
		core.Truncate(review.Purpose, 110),
		core.Truncate(review.CurrentState, 130),
		core.Truncate(review.NextStep, 130),
	)
}

func compactPath(path string) string {
	if path == "" {
		return "unknown"
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+"/") {
			path = "~/" + strings.TrimPrefix(path, home+"/")
		}
	}
	parts := strings.Split(path, "/")
	if len(parts) <= 4 {
		return path
	}
	return strings.Join(parts[len(parts)-4:], "/")
}

func countSessions(sessions []types.LocalSession, provider string) int {
	count := 0
	for _, session := range sessions {
		if session.Provider == provider {
			count++
		}
	}
	return count
}

func countMatchStatus(sessions []types.LocalSession, status string) int {
	count := 0
	for _, session := range sessions {
		if core.FirstNonEmpty(session.Match.Status, "unmatched") == status {
			count++
		}
	}
	return count
}

func statusWord(ok bool) string {
	if ok {
		return "ok"
	}
	return "not-ok"
}

func freshnessWord(age, staleAfter time.Duration) string {
	if age <= staleAfter {
		return "fresh"
	}
	return "stale"
}

func durationLabel(value time.Duration) string {
	if value < 0 {
		value = -value
	}
	if value < time.Minute {
		return fmt.Sprintf("%ds", int(value.Seconds()))
	}
	if value < time.Hour {
		return fmt.Sprintf("%dm", int(value.Minutes()))
	}
	if value < 24*time.Hour {
		return fmt.Sprintf("%dh", int(value.Hours()))
	}
	return fmt.Sprintf("%dd", int(value.Hours()/24))
}
