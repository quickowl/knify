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

type statusView struct {
	OK                 bool
	Path               string
	StaleAfter         time.Duration
	Now                time.Time
	Result             types.ScanResult
	GeneratedAt        time.Time
	HeartbeatAge       time.Duration
	ProcessKnown       bool
	ProcessAlive       bool
	HubChecked         bool
	HubStatus          string
	HubCanvasID        string
	HubVersion         int
	HubUpdatedAge      time.Duration
	HubBlocks          int
	HubCollectionMode  string
	HubCollectionItems int
}

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
	processKnown := false
	processIsAlive := false
	if result.Process.PID > 0 {
		alive := processAlive(result.Process.PID)
		if !alive {
			ok = false
		}
		processKnown = true
		processIsAlive = alive
		processLine = fmt.Sprintf("process: pid=%d alive=%v mode=%s interval=%s dynamic=%v build=%s", result.Process.PID, alive, core.FirstNonEmpty(result.Process.Mode, "unknown"), result.Process.Interval, result.Process.Dynamic, daemonBuildLabel(result.Process.Build))
	}

	hubLine := "hub: not checked"
	hubChecked := false
	hubStatus := "not checked"
	hubCanvasID := ""
	hubVersion := 0
	hubUpdatedAge := time.Duration(0)
	hubBlocks := 0
	hubCollectionMode := ""
	hubCollectionItems := 0
	hubURL := core.FirstNonEmpty(cfg.HubURL, result.Hub.URL)
	if hubURL != "" {
		hubCfg := cfg
		hubCfg.HubURL = hubURL
		hubCfg.CanvasID = core.FirstNonEmpty(cfg.CanvasID, result.Canvas.ID, types.DefaultCanvasID)
		hubChecked = true
		hubCanvasID = hubCfg.CanvasID
		hubCanvas, exists, err := hub.FetchHubCanvas(ctx, hubCfg, hubCfg.CanvasID)
		if err != nil {
			ok = false
			hubStatus = "error"
			hubLine = fmt.Sprintf("hub: error %s", err)
		} else if !exists {
			ok = false
			hubStatus = "missing"
			hubLine = fmt.Sprintf("hub: missing canvas=%s", hubCfg.CanvasID)
		} else {
			mode, items, found := collectionStats(hubCanvas)
			hubAge := now.Sub(core.ParseTime(hubCanvas.UpdatedAt))
			hubStatus = "ok"
			hubCanvasID = hubCanvas.ID
			hubVersion = hubCanvas.Version
			hubUpdatedAge = hubAge
			hubBlocks = len(hubCanvas.Blocks)
			hubCollectionMode = mode
			hubCollectionItems = items
			if core.ParseTime(hubCanvas.UpdatedAt).IsZero() || hubAge > staleAfter || !found {
				ok = false
				if !found {
					hubStatus = "missing collection"
				} else {
					hubStatus = "stale"
				}
			}
			hubLine = fmt.Sprintf("hub: ok canvas=%s version=%d updated=%s ago blocks=%d collection=%s items=%d", hubCanvas.ID, hubCanvas.Version, durationLabel(hubAge), len(hubCanvas.Blocks), mode, items)
		}
	}

	view := statusView{
		OK:                 ok,
		Path:               path,
		StaleAfter:         staleAfter,
		Now:                now,
		Result:             result,
		GeneratedAt:        generatedAt,
		HeartbeatAge:       age,
		ProcessKnown:       processKnown,
		ProcessAlive:       processIsAlive,
		HubChecked:         hubChecked,
		HubStatus:          hubStatus,
		HubCanvasID:        hubCanvasID,
		HubVersion:         hubVersion,
		HubUpdatedAge:      hubUpdatedAge,
		HubBlocks:          hubBlocks,
		HubCollectionMode:  hubCollectionMode,
		HubCollectionItems: hubCollectionItems,
	}
	if cfg.Pretty {
		writePrettyStatus(w, cfg, view)
		if !ok {
			return errors.New("session-monitor status is not ok")
		}
		return nil
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

type prettyMetric struct {
	Label  string
	Value  string
	Accent string
}

func writePrettyStatus(w io.Writer, cfg types.Config, view statusView) {
	fmt.Fprintf(w, "%s  %s  %s\n",
		prettyColor("1", "SESSION MONITOR"),
		prettyStatusBadge(view.OK),
		prettyColor("2", view.Now.Format("2006-01-02 15:04:05Z")),
	)
	fmt.Fprintln(w)
	writePrettyHeartbeat(w, view)
	writePrettyProcess(w, view)
	writePrettyHub(w, view)
	fmt.Fprintln(w)
	writePrettyMetrics(w, view)
	writePrettyProviders(w, view.Result.ProviderHealth)
	if len(view.Result.Errors) > 0 {
		writePrettyErrors(w, view.Result.Errors)
	}
	if cfg.RecentLimit > 0 {
		writePrettyRecent(w, view.Result.Sessions, cfg.RecentLimit, view.Now)
	}
}

func writePrettyHeartbeat(w io.Writer, view statusView) {
	label := prettyLabel("HEARTBEAT")
	if view.GeneratedAt.IsZero() {
		fmt.Fprintf(w, "%s %s  file=%s\n", label, prettyColor("31;1", "invalid"), view.Path)
		return
	}
	freshness := freshnessWord(view.HeartbeatAge, view.StaleAfter)
	accent := "32;1"
	if freshness != "fresh" {
		accent = "31;1"
	}
	fmt.Fprintf(w, "%s %s  age=%s  generated=%s  file=%s\n",
		label,
		prettyColor(accent, freshness),
		durationLabel(view.HeartbeatAge),
		view.Result.GeneratedAt,
		view.Path,
	)
}

func writePrettyProcess(w io.Writer, view statusView) {
	label := prettyLabel("DAEMON   ")
	if !view.ProcessKnown {
		fmt.Fprintf(w, "%s %s\n", label, prettyColor("33;1", "unknown"))
		return
	}
	state := "dead"
	accent := "31;1"
	if view.ProcessAlive {
		state = "alive"
		accent = "32;1"
	}
	fmt.Fprintf(w, "%s %s  pid=%d  mode=%s  interval=%s  dynamic=%v  build=%s\n",
		label,
		prettyColor(accent, state),
		view.Result.Process.PID,
		core.FirstNonEmpty(view.Result.Process.Mode, "unknown"),
		core.FirstNonEmpty(view.Result.Process.Interval, "unknown"),
		view.Result.Process.Dynamic,
		daemonBuildLabel(view.Result.Process.Build),
	)
}

func writePrettyHub(w io.Writer, view statusView) {
	label := prettyLabel("HUB      ")
	if !view.HubChecked {
		fmt.Fprintf(w, "%s %s\n", label, prettyColor("2", "not checked"))
		return
	}
	accent := "32;1"
	if view.HubStatus != "ok" {
		accent = "31;1"
	}
	if view.HubVersion == 0 {
		fmt.Fprintf(w, "%s %s  canvas=%s\n", label, prettyColor(accent, view.HubStatus), view.HubCanvasID)
		return
	}
	fmt.Fprintf(w, "%s %s  canvas=%s  v%d  updated=%s  blocks=%d  collection=%s/%d\n",
		label,
		prettyColor(accent, view.HubStatus),
		view.HubCanvasID,
		view.HubVersion,
		durationLabel(view.HubUpdatedAge),
		view.HubBlocks,
		core.FirstNonEmpty(view.HubCollectionMode, "missing"),
		view.HubCollectionItems,
	)
}

func writePrettyMetrics(w io.Writer, view statusView) {
	sessions := view.Result.Sessions
	active, recent, idle, stale, unknown := countActivity(sessions, view.Now)
	writePrettyMetricLine(w, "SESSIONS",
		prettyMetric{Label: "total", Value: fmt.Sprint(len(sessions))},
		prettyMetric{Label: "codex", Value: fmt.Sprint(countSessions(sessions, "codex"))},
		prettyMetric{Label: "claude", Value: fmt.Sprint(countSessions(sessions, "claude"))},
		prettyMetric{Label: "plans", Value: fmt.Sprint(countSessionPlans(sessions)), Accent: "36;1"},
		prettyMetric{Label: "shared", Value: fmt.Sprint(countSharedPlans(sessions)), Accent: metricAccent(countSharedPlans(sessions), "36;1")},
		prettyMetric{Label: "active", Value: fmt.Sprint(active), Accent: "32;1"},
		prettyMetric{Label: "recent", Value: fmt.Sprint(recent)},
		prettyMetric{Label: "idle", Value: fmt.Sprint(idle), Accent: metricAccent(idle, "33;1")},
		prettyMetric{Label: "stale", Value: fmt.Sprint(stale), Accent: metricAccent(stale, "31;1")},
		prettyMetric{Label: "unknown", Value: fmt.Sprint(unknown), Accent: metricAccent(unknown, "33;1")},
	)
	writePrettyMetricLine(w, "MATCHES",
		prettyMetric{Label: "exact", Value: fmt.Sprint(countMatchStatus(sessions, "exact")), Accent: "32;1"},
		prettyMetric{Label: "likely", Value: fmt.Sprint(countMatchStatus(sessions, "likely")), Accent: "32;1"},
		prettyMetric{Label: "unmatched", Value: fmt.Sprint(countMatchStatus(sessions, "unmatched")), Accent: "33;1"},
		prettyMetric{Label: "errors", Value: fmt.Sprint(len(view.Result.Errors)), Accent: metricAccent(len(view.Result.Errors), "31;1")},
		prettyMetric{Label: "posted", Value: fmt.Sprint(view.Result.Posted), Accent: boolAccent(view.Result.Posted)},
		prettyMetric{Label: "events", Value: fmt.Sprint(view.Result.EventsPosted)},
	)
}

func writePrettyMetricLine(w io.Writer, label string, metrics ...prettyMetric) {
	fmt.Fprint(w, prettyLabel(label))
	for i, metric := range metrics {
		if i == 0 {
			fmt.Fprint(w, " ")
		} else {
			fmt.Fprint(w, " | ")
		}
		value := metric.Value
		if metric.Accent != "" {
			value = prettyColor(metric.Accent, value)
		}
		fmt.Fprintf(w, "%s=%s", metric.Label, value)
	}
	fmt.Fprintln(w)
}

func writePrettyProviders(w io.Writer, health []types.ProviderHealth) {
	if len(health) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, prettyLabel("PROVIDERS"))
	for _, item := range health {
		accent := "31;1"
		if item.Status == "ok" {
			accent = "32;1"
		} else if item.Status == "health_only" {
			accent = "33;1"
		}
		status := prettyColor(accent, fmt.Sprintf("%-11s", item.Status))
		fmt.Fprintf(w, "  %-8s %s %s\n", item.Provider, status, providerDetails(item))
	}
}

func writePrettyErrors(w io.Writer, errors []string) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, prettyColor("31;1", "ERRORS"))
	for _, item := range errors {
		fmt.Fprintf(w, "  - %s\n", core.Truncate(strings.Join(strings.Fields(item), " "), 110))
	}
}

func writePrettyRecent(w io.Writer, sessions []types.LocalSession, limit int, now time.Time) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, prettyLabel("RECENT SESSIONS"))
	fmt.Fprintf(w, "%3s %s %s %s %s %s %s %s\n",
		"#",
		tableCell("AGE", 5, "1"),
		tableCell("AGENT", 6, "1"),
		tableCell("STATE", 8, "1"),
		tableCell("MATCH", 9, "1"),
		tableCell("PLAN", 24, "1"),
		tableCell("WORKSPACE", 20, "1"),
		tableCell("PURPOSE", 24, "1"),
	)
	if limit > len(sessions) {
		limit = len(sessions)
	}
	for i := 0; i < limit; i++ {
		fmt.Fprintln(w, prettySessionRow(i+1, sessions[i], now))
	}
}

func prettySessionRow(index int, session types.LocalSession, now time.Time) string {
	review := canvas.Review(session, now)
	updated := core.ParseTime(session.UpdatedAt)
	age := "?"
	if !updated.IsZero() {
		age = durationLabel(now.Sub(updated))
	}
	match := core.FirstNonEmpty(session.Match.Status, "unmatched")
	matchAccent := ""
	switch match {
	case "exact", "likely":
		matchAccent = "32;1"
	case "unmatched":
		matchAccent = "33;1"
	default:
		matchAccent = "31;1"
	}
	purpose := core.FirstNonEmpty(review.Purpose, session.Title, core.ShortID(session.SessionID), "untitled")
	return fmt.Sprintf("%3d %s %s %s %s %s %s %s",
		index,
		tableCell(age, 5, ""),
		tableCell(core.FirstNonEmpty(session.Provider, "unknown"), 6, ""),
		tableCell(core.FirstNonEmpty(session.Status, "unknown"), 8, ""),
		tableCell(match, 9, matchAccent),
		tableCell(planTableLabel(session.Plan), 24, "36"),
		tableCell(compactPath(session.CWD), 20, ""),
		tableCell(purpose, 24, ""),
	)
}

func providerDetails(item types.ProviderHealth) string {
	details := []string{}
	if sessions := item.Details["sessions"]; sessions != "" {
		details = append(details, "sessions="+sessions)
	}
	if rawFiles := item.Details["rawFiles"]; rawFiles != "" {
		details = append(details, "raw="+rawFiles)
	}
	if collapsed := item.Details["collapsedFiles"]; collapsed != "" {
		details = append(details, "collapsed="+collapsed)
	}
	if len(item.Warnings) > 0 {
		details = append(details, fmt.Sprintf("warnings=%d", len(item.Warnings)))
	}
	if len(details) == 0 {
		return prettyColor("2", "no details")
	}
	return strings.Join(details, "  ")
}

func countActivity(sessions []types.LocalSession, now time.Time) (active int, recent int, idle int, stale int, unknown int) {
	for _, session := range sessions {
		updated := core.ParseTime(session.UpdatedAt)
		if updated.IsZero() {
			unknown++
			continue
		}
		age := now.Sub(updated)
		if age < 0 {
			age = 0
		}
		switch {
		case age < 15*time.Minute:
			active++
		case age < 2*time.Hour:
			recent++
		case age < 24*time.Hour:
			idle++
		default:
			stale++
		}
	}
	return active, recent, idle, stale, unknown
}

func countSessionPlans(sessions []types.LocalSession) int {
	count := 0
	for _, session := range sessions {
		if session.Plan.Key != "" {
			count++
		}
	}
	return count
}

func countSharedPlans(sessions []types.LocalSession) int {
	keys := map[string]int{}
	for _, session := range sessions {
		if session.Plan.Key != "" {
			keys[session.Plan.Key]++
		}
	}
	count := 0
	for _, sessionsForPlan := range keys {
		if sessionsForPlan > 1 {
			count++
		}
	}
	return count
}

func planTableLabel(plan types.SessionPlan) string {
	if plan.Key == "" {
		return "-"
	}
	status := plan.Status
	if status != "" {
		status = " " + status
	}
	return core.FirstNonEmpty(plan.Title, "plan") + status
}

func metricAccent(value int, accent string) string {
	if value == 0 {
		return ""
	}
	return accent
}

func boolAccent(value bool) string {
	if value {
		return "32;1"
	}
	return "31;1"
}

func prettyLabel(label string) string {
	return prettyColor("36;1", fmt.Sprintf("%-10s", label))
}

func prettyStatusBadge(ok bool) string {
	if ok {
		return prettyColor("32;1", "[ OK ]")
	}
	return prettyColor("31;1", "[FAIL]")
}

func tableCell(value string, width int, accent string) string {
	text := fmt.Sprintf("%-*s", width, fitCell(value, width))
	if accent == "" {
		return text
	}
	return prettyColor(accent, text)
}

func fitCell(value string, width int) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		value = "-"
	}
	return core.Truncate(value, width)
}

func prettyColor(code string, value string) string {
	if code == "" || os.Getenv("NO_COLOR") != "" || os.Getenv("SESSION_MONITOR_NO_COLOR") != "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
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
		Build:    cfg.Build,
	}
}

func daemonBuildLabel(build types.DaemonBuildInfo) string {
	label := core.FirstNonEmpty(build.RevisionShort, build.Version, "dev")
	if build.Modified && !strings.Contains(label, "dirty") {
		label += "+dirty"
	}
	return label
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
	planLine := ""
	if session.Plan.Key != "" {
		planLine = fmt.Sprintf("\n      plan: %s source=%s key=%s", core.Truncate(planTableLabel(session.Plan), 110), core.FirstNonEmpty(session.Plan.Source, "unknown"), core.Truncate(session.Plan.Key, 80))
	}
	return fmt.Sprintf("%s %s/%s match=%s updated=%s workspace=%s title=%q%s%s\n      purpose: %s%s\n      now: %s\n      next: %s",
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
		planLine,
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
