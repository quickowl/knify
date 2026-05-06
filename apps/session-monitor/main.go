package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultCanvasID     = "canvas.session-monitor.local"
	defaultRunID        = "run.session-monitor.local"
	defaultAgentID      = "session-monitor"
	defaultLookback     = 24 * time.Hour
	defaultMaxSessions  = 30
	messageTextLimit    = 240
	messagesPerSession  = 3
	sessionMarkdownCap  = 3000
	recentMarkdownCap   = 12000
	recentPreviewLimit  = 180
	maxJSONLTokenBytes  = 128 * 1024 * 1024
	hubRequestTimeout   = 15 * time.Second
	likelyMatchWindow   = 48 * time.Hour
	sessionMonitorTitle = "Local session monitor"
	defaultStatusPath   = "/tmp/session-monitor-live-watch.json"
	defaultStaleAfter   = 3 * time.Minute
)

type Config struct {
	HubURL      string
	Token       string
	CanvasID    string
	RunID       string
	CodexHome   string
	ClaudeHome  string
	OutPath     string
	DryRun      bool
	Dynamic     bool
	Watch       bool
	Status      bool
	Interval    time.Duration
	StaleAfter  time.Duration
	RecentLimit int
	Lookback    time.Duration
	MaxSessions int
	Now         func() time.Time
}

type ScanResult struct {
	GeneratedAt    string             `json:"generatedAt"`
	Process        ProcessInfo        `json:"process,omitempty"`
	Lookback       string             `json:"lookback"`
	MaxSessions    int                `json:"maxSessions"`
	Sessions       []LocalSession     `json:"sessions"`
	ProviderHealth []ProviderHealth   `json:"providerHealth"`
	Errors         []string           `json:"errors,omitempty"`
	Hub            HubSummary         `json:"hub"`
	Canvas         Canvas             `json:"canvas"`
	Posted         bool               `json:"posted"`
	Dynamic        bool               `json:"dynamic"`
	EventsPosted   int                `json:"eventsPosted,omitempty"`
	PostResponse   map[string]any     `json:"postResponse,omitempty"`
	OpenAIDocs     OpenAIDocsEvidence `json:"openaiDocs"`
}

type ProcessInfo struct {
	PID      int    `json:"pid,omitempty"`
	Mode     string `json:"mode,omitempty"`
	Watch    bool   `json:"watch"`
	Dynamic  bool   `json:"dynamic"`
	Interval string `json:"interval,omitempty"`
	HubURL   string `json:"hubUrl,omitempty"`
	CanvasID string `json:"canvasId,omitempty"`
	RunID    string `json:"runId,omitempty"`
	OutPath  string `json:"outPath,omitempty"`
}

type OpenAIDocsEvidence struct {
	CodexExecResume string `json:"codexExecResume"`
	CodexAppServer  string `json:"codexAppServer"`
}

type ProviderHealth struct {
	Provider string            `json:"provider"`
	Status   string            `json:"status"`
	Details  map[string]string `json:"details,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
}

type HubSummary struct {
	URL         string `json:"url,omitempty"`
	CanvasCount int    `json:"canvasCount"`
	RunCount    int    `json:"runCount"`
}

type LocalSession struct {
	Provider       string            `json:"provider"`
	SessionID      string            `json:"sessionId"`
	Title          string            `json:"title,omitempty"`
	CWD            string            `json:"cwd,omitempty"`
	Status         string            `json:"status,omitempty"`
	SourcePath     string            `json:"sourcePath,omitempty"`
	CreatedAt      string            `json:"createdAt,omitempty"`
	UpdatedAt      string            `json:"updatedAt,omitempty"`
	LatestMessages []MessageSummary  `json:"latestMessages,omitempty"`
	ResumeHint     string            `json:"resumeHint,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Review         SessionReview     `json:"review,omitempty"`
	Match          MatchResult       `json:"match"`
}

type MessageSummary struct {
	Role      string `json:"role"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp,omitempty"`
}

type MatchResult struct {
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	CanvasID  string `json:"canvasId,omitempty"`
	RunID     string `json:"runId,omitempty"`
	Score     int    `json:"score,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
	Provider  string `json:"provider,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type SessionReview struct {
	Purpose      string   `json:"purpose,omitempty"`
	CurrentState string   `json:"currentState,omitempty"`
	NextStep     string   `json:"nextStep,omitempty"`
	Signals      []string `json:"signals,omitempty"`
}

type HubState struct {
	Canvases []HubCanvas
	Runs     []HubRun
}

type HubCanvas struct {
	ID        string `json:"id"`
	AgentID   string `json:"agentId"`
	RunID     string `json:"runId"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Status    string `json:"status"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt"`
}

type HubRun struct {
	ID             string            `json:"id"`
	Provider       string            `json:"provider"`
	AgentID        string            `json:"agentId"`
	RunID          string            `json:"runId"`
	CanvasID       string            `json:"canvasId"`
	Title          string            `json:"title"`
	Status         string            `json:"status"`
	ExternalID     string            `json:"externalId"`
	UpdatedAt      string            `json:"updatedAt"`
	FeedbackTarget FeedbackTarget    `json:"feedbackTarget"`
	Metadata       map[string]string `json:"metadata"`
}

type FeedbackTarget struct {
	Provider   string            `json:"provider"`
	Mode       string            `json:"mode"`
	ExternalID string            `json:"externalId"`
	CWD        string            `json:"cwd"`
	Metadata   map[string]string `json:"metadata"`
}

type Canvas struct {
	ID        string           `json:"id"`
	AgentID   string           `json:"agentId"`
	RunID     string           `json:"runId"`
	Title     string           `json:"title"`
	Summary   string           `json:"summary"`
	Status    string           `json:"status"`
	Mode      string           `json:"mode,omitempty"`
	Priority  string           `json:"priority"`
	CreatedAt string           `json:"createdAt"`
	UpdatedAt string           `json:"updatedAt"`
	Version   int              `json:"version"`
	Blocks    []map[string]any `json:"blocks"`
}

type CanvasLogEvent struct {
	ID                 string         `json:"id"`
	CanvasID           string         `json:"canvasId"`
	Type               string         `json:"type"`
	ExpectedVersion    *int           `json:"expectedVersion,omitempty"`
	AgentID            string         `json:"agentId,omitempty"`
	RunID              string         `json:"runId,omitempty"`
	Title              string         `json:"title,omitempty"`
	Summary            string         `json:"summary,omitempty"`
	Status             string         `json:"status,omitempty"`
	Priority           string         `json:"priority,omitempty"`
	Block              map[string]any `json:"block,omitempty"`
	BlockID            string         `json:"blockId,omitempty"`
	InsertAfterBlockID string         `json:"insertAfterBlockId,omitempty"`
	CreatedAt          string         `json:"createdAt"`
}

type codexIndexEntry struct {
	ThreadName string
	UpdatedAt  time.Time
}

func main() {
	if err := runMain(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runMain(ctx context.Context, args []string) error {
	cfg, once, err := parseConfig(args)
	if err != nil {
		return err
	}
	if cfg.Status {
		return writeStatus(ctx, cfg, os.Stdout)
	}
	if once {
		cfg.Watch = false
	}
	if !cfg.Watch {
		result, err := runOnce(ctx, cfg)
		if err != nil {
			return err
		}
		return writeResult(cfg, result)
	}
	if cfg.Interval <= 0 {
		return errors.New("--interval must be positive when --watch=true")
	}
	for {
		result, err := runOnce(ctx, cfg)
		if err != nil {
			return err
		}
		if err := writeResult(cfg, result); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cfg.Interval):
		}
	}
}

func parseConfig(args []string) (Config, bool, error) {
	home, _ := os.UserHomeDir()
	cfg := Config{
		HubURL:      firstEnv("SESSION_MONITOR_HUB_URL", "HUB_BASE_URL", "AGENTCANVAS_HUB_URL"),
		Token:       firstEnv("SESSION_MONITOR_HUB_TOKEN", "HUB_TOKEN", "AGENTCANVAS_HUB_TOKEN"),
		CanvasID:    envOrDefault("SESSION_MONITOR_CANVAS_ID", defaultCanvasID),
		RunID:       envOrDefault("SESSION_MONITOR_RUN_ID", defaultRunID),
		CodexHome:   filepath.Join(home, ".codex"),
		ClaudeHome:  filepath.Join(home, ".claude"),
		Interval:    durationEnv("SESSION_MONITOR_INTERVAL", time.Minute),
		StaleAfter:  durationEnv("SESSION_MONITOR_STALE_AFTER", defaultStaleAfter),
		Lookback:    lookbackEnv(),
		MaxSessions: intEnv("SESSION_MONITOR_MAX_SESSIONS", defaultMaxSessions),
		Dynamic:     boolEnv("SESSION_MONITOR_DYNAMIC", false),
		Now:         func() time.Time { return time.Now().UTC() },
	}
	flags := flag.NewFlagSet("session-monitor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	once := flags.Bool("once", false, "scan once and exit")
	flags.BoolVar(&cfg.Status, "status", false, "read the last --out heartbeat and report daemon health")
	flags.BoolVar(&cfg.Watch, "watch", false, "scan repeatedly")
	flags.DurationVar(&cfg.Interval, "interval", cfg.Interval, "watch interval")
	flags.DurationVar(&cfg.StaleAfter, "stale-after", cfg.StaleAfter, "maximum acceptable heartbeat age for --status")
	flags.IntVar(&cfg.RecentLimit, "recent", intEnv("SESSION_MONITOR_RECENT", 0), "with --status, print this many recent tracked sessions")
	flags.BoolVar(&cfg.DryRun, "dry-run", false, "do not publish to the hub")
	flags.BoolVar(&cfg.Dynamic, "dynamic", cfg.Dynamic, "publish through the dynamic canvas event protocol")
	flags.StringVar(&cfg.OutPath, "out", firstEnv("SESSION_MONITOR_OUT"), "write scan result JSON to this path")
	flags.StringVar(&cfg.HubURL, "hub-url", cfg.HubURL, "AgentCanvas hub base URL")
	flags.StringVar(&cfg.Token, "token", cfg.Token, "AgentCanvas hub bearer token")
	flags.StringVar(&cfg.CanvasID, "canvas-id", cfg.CanvasID, "review canvas id")
	flags.StringVar(&cfg.RunID, "run-id", cfg.RunID, "review run id")
	flags.StringVar(&cfg.CodexHome, "codex-home", cfg.CodexHome, "Codex home directory")
	flags.StringVar(&cfg.ClaudeHome, "claude-home", cfg.ClaudeHome, "Claude Code home directory")
	flags.DurationVar(&cfg.Lookback, "lookback", cfg.Lookback, "session lookback duration")
	flags.IntVar(&cfg.MaxSessions, "max-sessions", cfg.MaxSessions, "maximum local sessions to include")
	if err := flags.Parse(args); err != nil {
		return Config{}, false, fmt.Errorf("usage: %s", usage())
	}
	if cfg.CanvasID == "" || cfg.RunID == "" {
		return Config{}, false, errors.New("--canvas-id and --run-id are required")
	}
	if cfg.MaxSessions <= 0 {
		return Config{}, false, errors.New("--max-sessions must be positive")
	}
	if cfg.Lookback <= 0 {
		return Config{}, false, errors.New("--lookback must be positive")
	}
	return cfg, *once, nil
}

func usage() string {
	return "session-monitor --once|--watch|--status [--dry-run] [--dynamic] [--hub-url URL] [--out FILE]"
}

func runOnce(ctx context.Context, cfg Config) (ScanResult, error) {
	now := cfg.Now().UTC()
	sessions, health, scanErrors := scanLocalSessions(ctx, cfg, now)
	hubState := HubState{}
	var hubErrors []string
	if cfg.HubURL != "" {
		var err error
		hubState, err = fetchHubState(ctx, cfg)
		if err != nil {
			hubErrors = append(hubErrors, err.Error())
		}
	}
	matchSessions(sessions, hubState)
	sort.SliceStable(sessions, func(i, j int) bool {
		return timeFromRFC3339(sessions[i].UpdatedAt).After(timeFromRFC3339(sessions[j].UpdatedAt))
	})
	if len(sessions) > cfg.MaxSessions {
		sessions = sessions[:cfg.MaxSessions]
	}
	fillSessionReviews(sessions, now)
	version := nextCanvasVersion(cfg.CanvasID, hubState)
	canvas := buildCanvas(cfg, sessions, health, append(scanErrors, hubErrors...), hubState, version, now)
	if cfg.Dynamic {
		canvas = compactDynamicCanvas(canvas)
	}
	result := ScanResult{
		GeneratedAt:    formatTime(now),
		Process:        processInfo(cfg),
		Lookback:       cfg.Lookback.String(),
		MaxSessions:    cfg.MaxSessions,
		Sessions:       sessions,
		ProviderHealth: health,
		Errors:         append(scanErrors, hubErrors...),
		Hub: HubSummary{
			URL:         cfg.HubURL,
			CanvasCount: len(hubState.Canvases),
			RunCount:    len(hubState.Runs),
		},
		Canvas:  canvas,
		Dynamic: cfg.Dynamic,
		OpenAIDocs: OpenAIDocsEvidence{
			CodexExecResume: "OpenAI Codex CLI reference marks `codex exec` and `codex resume` stable; `codex exec` can stream JSONL and resume previous sessions.",
			CodexAppServer:  "OpenAI Codex CLI reference marks `codex app-server` experimental, so v1 records app-server readiness metadata but does not depend on it.",
		},
	}
	if cfg.DryRun {
		return result, nil
	}
	if cfg.HubURL == "" {
		return result, errors.New("--hub-url or SESSION_MONITOR_HUB_URL is required unless --dry-run is set")
	}
	response, eventsPosted, err := publishCanvas(ctx, cfg, canvas)
	if err != nil {
		return result, err
	}
	result.Posted = true
	result.EventsPosted = eventsPosted
	result.PostResponse = response
	return result, nil
}

func writeResult(cfg Config, result ScanResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if cfg.OutPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.OutPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(cfg.OutPath, data, 0o644)
	}
	if cfg.DryRun || !cfg.Watch {
		_, err := os.Stdout.Write(data)
		return err
	}
	fmt.Printf("%s posted=%v sessions=%d canvas=%s\n", result.GeneratedAt, result.Posted, len(result.Sessions), result.Canvas.ID)
	return nil
}

func writeStatus(ctx context.Context, cfg Config, w io.Writer) error {
	path := statusOutputPath(cfg)
	staleAfter := cfg.StaleAfter
	if staleAfter <= 0 {
		staleAfter = defaultStaleAfter
	}
	now := cfg.Now().UTC()
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(w, "session-monitor status: down\nheartbeat: missing %s (%v)\n", path, err)
		return errors.New("session-monitor status is not ok")
	}
	var result ScanResult
	if err := json.Unmarshal(data, &result); err != nil {
		fmt.Fprintf(w, "session-monitor status: down\nheartbeat: unreadable %s (%v)\n", path, err)
		return errors.New("session-monitor status is not ok")
	}

	ok := true
	generatedAt := parseTime(result.GeneratedAt)
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
		processLine = fmt.Sprintf("process: pid=%d alive=%v mode=%s interval=%s dynamic=%v", result.Process.PID, alive, firstNonEmpty(result.Process.Mode, "unknown"), result.Process.Interval, result.Process.Dynamic)
	}

	hubLine := "hub: not checked"
	hubURL := firstNonEmpty(cfg.HubURL, result.Hub.URL)
	if hubURL != "" {
		hubCfg := cfg
		hubCfg.HubURL = hubURL
		hubCfg.CanvasID = firstNonEmpty(cfg.CanvasID, result.Canvas.ID, defaultCanvasID)
		canvas, exists, err := fetchHubCanvas(ctx, hubCfg, hubCfg.CanvasID)
		if err != nil {
			ok = false
			hubLine = fmt.Sprintf("hub: error %s", err)
		} else if !exists {
			ok = false
			hubLine = fmt.Sprintf("hub: missing canvas=%s", hubCfg.CanvasID)
		} else {
			mode, items, found := collectionStats(canvas)
			hubAge := now.Sub(parseTime(canvas.UpdatedAt))
			if parseTime(canvas.UpdatedAt).IsZero() || hubAge > staleAfter || !found {
				ok = false
			}
			hubLine = fmt.Sprintf("hub: ok canvas=%s version=%d updated=%s ago blocks=%d collection=%s items=%d", canvas.ID, canvas.Version, durationLabel(hubAge), len(canvas.Blocks), mode, items)
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

func processInfo(cfg Config) ProcessInfo {
	mode := "once"
	if cfg.Watch {
		mode = "watch"
	}
	return ProcessInfo{
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

func statusOutputPath(cfg Config) string {
	if cfg.OutPath != "" {
		return cfg.OutPath
	}
	return defaultStatusPath
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

func requiredProvidersOK(health []ProviderHealth) bool {
	found := map[string]bool{}
	for _, item := range health {
		if (item.Provider == "codex" || item.Provider == "claude") && item.Status == "ok" {
			found[item.Provider] = true
		}
	}
	return found["codex"] && found["claude"]
}

func collectionStats(canvas Canvas) (string, int, bool) {
	for _, block := range canvas.Blocks {
		if stringValue(block["id"]) != "session-monitor-collection" {
			continue
		}
		switch items := block["items"].(type) {
		case []any:
			return firstNonEmpty(stringValue(block["mode"]), "unknown"), len(items), true
		case []map[string]any:
			return firstNonEmpty(stringValue(block["mode"]), "unknown"), len(items), true
		default:
			return firstNonEmpty(stringValue(block["mode"]), "unknown"), 0, true
		}
	}
	return "missing", 0, false
}

func statusSessionLine(session LocalSession, now time.Time) string {
	review := sessionReview(session, now)
	updated := parseTime(session.UpdatedAt)
	age := "unknown"
	if !updated.IsZero() {
		age = durationLabel(now.Sub(updated)) + " ago"
	}
	title := firstNonEmpty(session.Title, shortID(session.SessionID), "untitled")
	workspace := compactPath(session.CWD)
	latest := ""
	if len(session.LatestMessages) > 0 {
		msg := session.LatestMessages[len(session.LatestMessages)-1]
		latest = " - " + truncate(strings.TrimSpace(msg.Role+": "+msg.Text), 90)
	}
	collapsed := ""
	if count := session.Metadata["collapsedFiles"]; count != "" {
		collapsed = " collapsed=" + count
	}
	return fmt.Sprintf("%s %s/%s match=%s updated=%s workspace=%s title=%q%s%s\n      purpose: %s\n      now: %s\n      next: %s",
		shortID(session.SessionID),
		session.Provider,
		firstNonEmpty(session.Status, "unknown"),
		firstNonEmpty(session.Match.Status, "unmatched"),
		age,
		workspace,
		truncate(title, 64),
		collapsed,
		latest,
		truncate(review.Purpose, 110),
		truncate(review.CurrentState, 130),
		truncate(review.NextStep, 130),
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

func countSessions(sessions []LocalSession, provider string) int {
	count := 0
	for _, session := range sessions {
		if session.Provider == provider {
			count++
		}
	}
	return count
}

func countMatchStatus(sessions []LocalSession, status string) int {
	count := 0
	for _, session := range sessions {
		if firstNonEmpty(session.Match.Status, "unmatched") == status {
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

func scanLocalSessions(ctx context.Context, cfg Config, now time.Time) ([]LocalSession, []ProviderHealth, []string) {
	var sessions []LocalSession
	var health []ProviderHealth
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
		return timeFromRFC3339(sessions[i].UpdatedAt).After(timeFromRFC3339(sessions[j].UpdatedAt))
	})
	return sessions, health, errorsOut
}

func scanCodex(ctx context.Context, home string, cutoff time.Time) ([]LocalSession, ProviderHealth, []string) {
	health := ProviderHealth{Provider: "codex", Status: "ok", Details: map[string]string{"home": home}}
	var errorsOut []string
	info, err := os.Stat(home)
	if err != nil || !info.IsDir() {
		health.Status = "missing"
		if err != nil {
			health.Warnings = append(health.Warnings, err.Error())
		}
		return nil, health, nil
	}
	index := readCodexIndex(filepath.Join(home, "session_index.jsonl"))
	logTimes, logWarning := readCodexLogTimes(ctx, filepath.Join(home, "logs_2.sqlite"))
	if logWarning != "" {
		health.Warnings = append(health.Warnings, logWarning)
	}
	sessionRoot := filepath.Join(home, "sessions")
	var sessions []LocalSession
	walkErr := filepath.WalkDir(sessionRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errorsOut = append(errorsOut, fmt.Sprintf("codex walk %s: %v", path, err))
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			errorsOut = append(errorsOut, fmt.Sprintf("codex stat %s: %v", path, statErr))
			return nil
		}
		session, parseErr := parseCodexSessionFile(path, info.ModTime().UTC(), index, logTimes)
		if parseErr != nil {
			errorsOut = append(errorsOut, parseErr.Error())
			return nil
		}
		if session.SessionID == "" {
			return nil
		}
		if timeFromRFC3339(session.UpdatedAt).Before(cutoff) && info.ModTime().Before(cutoff) {
			return nil
		}
		sessions = append(sessions, session)
		return nil
	})
	if walkErr != nil {
		errorsOut = append(errorsOut, walkErr.Error())
	}
	rawFiles := len(sessions)
	sessions = collapseSessionsByProviderID(sessions)
	health.Details["sessions"] = strconv.Itoa(len(sessions))
	if rawFiles != len(sessions) {
		health.Details["rawFiles"] = strconv.Itoa(rawFiles)
		health.Details["collapsedFiles"] = strconv.Itoa(rawFiles - len(sessions))
	}
	return sessions, health, errorsOut
}

func readCodexIndex(path string) map[string]codexIndexEntry {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	index := map[string]codexIndexEntry{}
	scanner := newJSONLScanner(file)
	for scanner.Scan() {
		var entry struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
			UpdatedAt  string `json:"updated_at"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || entry.ID == "" {
			continue
		}
		index[entry.ID] = codexIndexEntry{ThreadName: entry.ThreadName, UpdatedAt: parseTime(entry.UpdatedAt)}
	}
	return index
}

func readCodexLogTimes(ctx context.Context, path string) (map[string]time.Time, string) {
	if _, err := os.Stat(path); err != nil {
		return nil, ""
	}
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		return nil, "sqlite3 not found; Codex logs_2.sqlite enrichment skipped"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	uri := "file:" + path + "?mode=ro"
	cmd := exec.CommandContext(ctx, sqlite, uri, "select thread_id, max(ts) from logs where thread_id is not null group by thread_id;")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Sprintf("Codex logs_2.sqlite enrichment skipped: %v", err)
	}
	times := map[string]time.Time{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			continue
		}
		sec, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		times[parts[0]] = time.Unix(sec, 0).UTC()
	}
	return times, ""
}

func parseCodexSessionFile(path string, modTime time.Time, index map[string]codexIndexEntry, logTimes map[string]time.Time) (LocalSession, error) {
	file, err := os.Open(path)
	if err != nil {
		return LocalSession{}, fmt.Errorf("codex open %s: %w", path, err)
	}
	defer file.Close()
	session := LocalSession{
		Provider:   "codex",
		SourcePath: path,
		Status:     "recorded",
		Metadata:   map[string]string{},
	}
	var created, updated time.Time
	var messages []MessageSummary
	scanner := newJSONLScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		recordTime := parseRecordTime(record)
		created = earliest(created, recordTime)
		updated = latest(updated, recordTime)
		recordType := stringValue(record["type"])
		payload := mapValue(record["payload"])
		switch recordType {
		case "session_meta":
			session.SessionID = firstNonEmpty(stringValue(payload["id"]), session.SessionID)
			session.CWD = firstNonEmpty(stringValue(payload["cwd"]), session.CWD)
			if source := stringValue(payload["source"]); source != "" {
				session.Metadata["source"] = source
			}
			if modelProvider := stringValue(payload["model_provider"]); modelProvider != "" {
				session.Metadata["modelProvider"] = modelProvider
			}
			if git := mapValue(payload["git"]); git != nil {
				if branch := stringValue(git["branch"]); branch != "" {
					session.Metadata["gitBranch"] = branch
				}
			}
		case "event_msg":
			if title := stringValue(payload["thread_name"]); title != "" {
				session.Title = title
			}
			if threadID := stringValue(payload["thread_id"]); threadID != "" {
				session.SessionID = firstNonEmpty(session.SessionID, threadID)
			}
		case "response_item":
			role := stringValue(payload["role"])
			if role == "user" || role == "assistant" {
				text := summarizeText(extractText(payload["content"]), messageTextLimit)
				if text != "" {
					messages = append(messages, MessageSummary{Role: role, Text: text, Timestamp: formatTime(recordTime)})
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return LocalSession{}, fmt.Errorf("codex scan %s: %w", path, err)
	}
	if session.SessionID == "" {
		session.SessionID = codexIDFromPath(path)
	}
	if entry, ok := index[session.SessionID]; ok {
		session.Title = firstNonEmpty(session.Title, entry.ThreadName)
		updated = latest(updated, entry.UpdatedAt)
	}
	if logTime, ok := logTimes[session.SessionID]; ok {
		updated = latest(updated, logTime)
	}
	if updated.IsZero() {
		updated = modTime
	}
	if created.IsZero() {
		created = updated
	}
	session.CreatedAt = formatTime(created)
	session.UpdatedAt = formatTime(updated)
	if session.Title == "" {
		session.Title = "Codex " + shortID(session.SessionID)
	}
	session.LatestMessages = lastMessages(messages, messagesPerSession)
	if session.SessionID != "" {
		session.ResumeHint = "codex exec resume " + session.SessionID + " <prompt>"
	}
	return session, nil
}

func scanClaude(home string, cutoff time.Time) ([]LocalSession, ProviderHealth, []string) {
	health := ProviderHealth{Provider: "claude", Status: "ok", Details: map[string]string{"home": home}}
	var errorsOut []string
	info, err := os.Stat(home)
	if err != nil || !info.IsDir() {
		health.Status = "missing"
		if err != nil {
			health.Warnings = append(health.Warnings, err.Error())
		}
		return nil, health, nil
	}
	active := readClaudeActiveSessions(filepath.Join(home, "sessions"))
	var sessions []LocalSession
	projectRoot := filepath.Join(home, "projects")
	walkErr := filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errorsOut = append(errorsOut, fmt.Sprintf("claude walk %s: %v", path, err))
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			errorsOut = append(errorsOut, fmt.Sprintf("claude stat %s: %v", path, statErr))
			return nil
		}
		session, parseErr := parseClaudeProjectFile(path, info.ModTime().UTC(), active)
		if parseErr != nil {
			errorsOut = append(errorsOut, parseErr.Error())
			return nil
		}
		if session.SessionID == "" {
			return nil
		}
		if timeFromRFC3339(session.UpdatedAt).Before(cutoff) && info.ModTime().Before(cutoff) {
			return nil
		}
		sessions = append(sessions, session)
		return nil
	})
	if walkErr != nil {
		errorsOut = append(errorsOut, walkErr.Error())
	}
	if historyCount, historyWarning := countJSONLLines(filepath.Join(home, "history.jsonl")); historyWarning != "" {
		health.Warnings = append(health.Warnings, historyWarning)
	} else {
		health.Details["historyEntries"] = strconv.Itoa(historyCount)
	}
	rawFiles := len(sessions)
	sessions = collapseSessionsByProviderID(sessions)
	health.Details["sessions"] = strconv.Itoa(len(sessions))
	if rawFiles != len(sessions) {
		health.Details["rawFiles"] = strconv.Itoa(rawFiles)
		health.Details["collapsedFiles"] = strconv.Itoa(rawFiles - len(sessions))
	}
	return sessions, health, errorsOut
}

func collapseSessionsByProviderID(sessions []LocalSession) []LocalSession {
	byKey := map[string]int{}
	var out []LocalSession
	for _, session := range sessions {
		session = normalizeCollapseMetadata(session, 1, boolInt(isSubagentSource(session.SourcePath)))
		key := session.Provider + "\x00" + session.SessionID
		if idx, ok := byKey[key]; ok {
			out[idx] = mergeSessionRows(out[idx], session)
			continue
		}
		byKey[key] = len(out)
		out = append(out, session)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return timeFromRFC3339(out[i].UpdatedAt).After(timeFromRFC3339(out[j].UpdatedAt))
	})
	return out
}

func mergeSessionRows(left, right LocalSession) LocalSession {
	merged := left
	merged.CreatedAt = formatTime(earliest(timeFromRFC3339(left.CreatedAt), timeFromRFC3339(right.CreatedAt)))
	if timeFromRFC3339(right.UpdatedAt).After(timeFromRFC3339(left.UpdatedAt)) {
		merged.UpdatedAt = right.UpdatedAt
		merged.LatestMessages = right.LatestMessages
	}
	if preferSourcePath(right.SourcePath, merged.SourcePath) {
		merged.SourcePath = right.SourcePath
	}
	if preferTitle(right.Title, merged.Title) {
		merged.Title = right.Title
	}
	merged.CWD = firstNonEmpty(merged.CWD, right.CWD)
	if statusRank(right.Status) > statusRank(merged.Status) {
		merged.Status = right.Status
	}
	merged.ResumeHint = firstNonEmpty(merged.ResumeHint, right.ResumeHint)
	merged.Metadata = mergeStringMaps(merged.Metadata, right.Metadata)
	merged = normalizeCollapseMetadata(
		merged,
		intMetadata(left.Metadata, "sourceFiles")+intMetadata(right.Metadata, "sourceFiles"),
		intMetadata(left.Metadata, "subagentFiles")+intMetadata(right.Metadata, "subagentFiles"),
	)
	return merged
}

func normalizeCollapseMetadata(session LocalSession, sourceFiles, subagentFiles int) LocalSession {
	if session.Metadata == nil {
		session.Metadata = map[string]string{}
	}
	session.Metadata["sourceFiles"] = strconv.Itoa(maxInt(sourceFiles, 1))
	if subagentFiles > 0 {
		session.Metadata["subagentFiles"] = strconv.Itoa(subagentFiles)
	}
	if sourceFiles > 1 {
		session.Metadata["collapsedFiles"] = strconv.Itoa(sourceFiles)
	}
	return session
}

func preferSourcePath(candidate, current string) bool {
	if candidate == "" {
		return false
	}
	if current == "" {
		return true
	}
	return isSubagentSource(current) && !isSubagentSource(candidate)
}

func preferTitle(candidate, current string) bool {
	if candidate == "" {
		return false
	}
	if current == "" {
		return true
	}
	return strings.HasPrefix(current, "Claude ") && !strings.HasPrefix(candidate, "Claude ")
}

func isSubagentSource(path string) bool {
	return strings.Contains(path, string(filepath.Separator)+"subagents"+string(filepath.Separator))
}

func statusRank(status string) int {
	switch strings.ToLower(status) {
	case "busy", "running", "active":
		return 3
	case "idle":
		return 2
	case "recorded":
		return 1
	default:
		return 0
	}
}

func mergeStringMaps(left, right map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range left {
		out[key] = value
	}
	for key, value := range right {
		if _, exists := out[key]; !exists && value != "" {
			out[key] = value
		}
	}
	return out
}

func intMetadata(metadata map[string]string, key string) int {
	if metadata == nil {
		return 0
	}
	value, _ := strconv.Atoi(metadata[key])
	return value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

type claudeActiveSession struct {
	SessionID string
	CWD       string
	Status    string
	UpdatedAt time.Time
	PID       string
}

func readClaudeActiveSessions(root string) map[string]claudeActiveSession {
	out := map[string]claudeActiveSession{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		id := stringValue(raw["sessionId"])
		if id == "" {
			continue
		}
		active := claudeActiveSession{
			SessionID: id,
			CWD:       stringValue(raw["cwd"]),
			Status:    stringValue(raw["status"]),
			UpdatedAt: timeFromMillis(raw["updatedAt"]),
			PID:       numberString(raw["pid"]),
		}
		out[id] = active
	}
	return out
}

func parseClaudeProjectFile(path string, modTime time.Time, active map[string]claudeActiveSession) (LocalSession, error) {
	file, err := os.Open(path)
	if err != nil {
		return LocalSession{}, fmt.Errorf("claude open %s: %w", path, err)
	}
	defer file.Close()
	session := LocalSession{
		Provider:   "claude",
		SourcePath: path,
		Status:     "recorded",
		Metadata:   map[string]string{},
	}
	var created, updated time.Time
	var messages []MessageSummary
	scanner := newJSONLScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		recordType := stringValue(record["type"])
		recordTime := parseRecordTime(record)
		created = earliest(created, recordTime)
		updated = latest(updated, recordTime)
		session.SessionID = firstNonEmpty(session.SessionID, stringValue(record["sessionId"]))
		session.CWD = firstNonEmpty(session.CWD, stringValue(record["cwd"]))
		if branch := stringValue(record["gitBranch"]); branch != "" {
			session.Metadata["gitBranch"] = branch
		}
		if version := stringValue(record["version"]); version != "" {
			session.Metadata["version"] = version
		}
		if recordType == "ai-title" {
			session.Title = firstNonEmpty(session.Title, stringValue(record["aiTitle"]))
			continue
		}
		if recordType == "permission-mode" {
			if mode := stringValue(record["permissionMode"]); mode != "" {
				session.Metadata["permissionMode"] = mode
			}
			continue
		}
		if recordType == "user" || recordType == "assistant" {
			message := mapValue(record["message"])
			role := firstNonEmpty(stringValue(message["role"]), recordType)
			text := summarizeText(extractText(message["content"]), messageTextLimit)
			if text != "" {
				messages = append(messages, MessageSummary{Role: role, Text: text, Timestamp: formatTime(recordTime)})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return LocalSession{}, fmt.Errorf("claude scan %s: %w", path, err)
	}
	if session.SessionID == "" {
		session.SessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if got, ok := active[session.SessionID]; ok {
		session.CWD = firstNonEmpty(session.CWD, got.CWD)
		session.Status = firstNonEmpty(got.Status, session.Status)
		updated = latest(updated, got.UpdatedAt)
		if got.PID != "" {
			session.Metadata["pid"] = got.PID
		}
	}
	if updated.IsZero() {
		updated = modTime
	}
	if created.IsZero() {
		created = updated
	}
	session.CreatedAt = formatTime(created)
	session.UpdatedAt = formatTime(updated)
	if session.Title == "" {
		session.Title = "Claude " + shortID(session.SessionID)
	}
	session.LatestMessages = lastMessages(messages, messagesPerSession)
	if session.SessionID != "" {
		session.ResumeHint = "claude -p --resume " + session.SessionID + " <prompt>"
	}
	return session, nil
}

func scanCursorHealth() ProviderHealth {
	health := ProviderHealth{
		Provider: "cursor",
		Status:   "health_only",
		Details:  map[string]string{"sessionParsing": "deferred"},
		Warnings: []string{"Cursor v1 is limited to CLI health because local conversation storage is not stable enough for read-only parsing."},
	}
	for _, name := range []string{"agent", "cursor-agent", "cursor"} {
		if path := lookPathWithFallback(name); path != "" {
			health.Details[name] = path
		}
	}
	if len(health.Details) == 1 {
		health.Status = "missing"
		health.Warnings = append(health.Warnings, "no Cursor CLI binary found")
	}
	return health
}

func lookPathWithFallback(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", name),
	}
	if name == "cursor" {
		candidates = append(candidates, "/Applications/Cursor.app/Contents/Resources/app/bin/cursor")
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path
		}
	}
	return ""
}

func fetchHubState(ctx context.Context, cfg Config) (HubState, error) {
	var state HubState
	var errs []string
	if err := hubGET(ctx, cfg, "/v1/canvases", &state.Canvases); err != nil {
		errs = append(errs, err.Error())
	}
	if err := hubGET(ctx, cfg, "/v1/agent-runs", &state.Runs); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return state, fmt.Errorf("hub fetch: %s", strings.Join(errs, "; "))
	}
	return state, nil
}

func hubGET(ctx context.Context, cfg Config, path string, target any) error {
	url := strings.TrimRight(cfg.HubURL, "/") + path
	ctx, cancel := context.WithTimeout(ctx, hubRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("GET %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func publishCanvas(ctx context.Context, cfg Config, canvas Canvas) (map[string]any, int, error) {
	if cfg.Dynamic {
		response, eventsPosted, err := postCanvasDynamic(ctx, cfg, canvas)
		return response, eventsPosted, err
	}
	response, err := postCanvas(ctx, cfg, canvas)
	if err != nil {
		return nil, 0, err
	}
	return response, 0, nil
}

func postCanvas(ctx context.Context, cfg Config, canvas Canvas) (map[string]any, error) {
	body, err := json.Marshal(canvas)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, hubRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.HubURL, "/")+"/v1/canvases", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST /v1/canvases returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var response map[string]any
	if len(data) > 0 {
		_ = json.Unmarshal(data, &response)
	}
	return response, nil
}

func postCanvasDynamic(ctx context.Context, cfg Config, canvas Canvas) (map[string]any, int, error) {
	now := parseTime(canvas.UpdatedAt)
	if now.IsZero() {
		now = cfg.Now().UTC()
	}
	existing, exists, err := fetchHubCanvas(ctx, cfg, canvas.ID)
	if err != nil {
		return nil, 0, err
	}
	currentVersion := existing.Version
	existingBlocks := mapBlocksByID(existing.Blocks)
	eventsPosted := 0
	var response map[string]any

	if !exists {
		status := canvas.Status
		if cfg.Watch {
			status = "in_progress"
		}
		event := CanvasLogEvent{
			ID:        dynamicEventID(canvas.ID, "start", now, eventsPosted+1),
			CanvasID:  canvas.ID,
			Type:      "canvas.started",
			AgentID:   canvas.AgentID,
			RunID:     canvas.RunID,
			Title:     canvas.Title,
			Summary:   canvas.Summary,
			Status:    status,
			Priority:  canvas.Priority,
			CreatedAt: formatTime(now),
		}
		response, err = postCanvasEvent(ctx, cfg, event)
		if err != nil {
			return response, eventsPosted, err
		}
		eventsPosted++
		currentVersion = responseVersion(response, currentVersion+1)
	} else {
		status := canvas.Status
		if cfg.Watch {
			status = "in_progress"
		}
		event := CanvasLogEvent{
			ID:              dynamicEventID(canvas.ID, "summary", now, eventsPosted+1),
			CanvasID:        canvas.ID,
			Type:            "canvas.summary.updated",
			ExpectedVersion: &currentVersion,
			Title:           canvas.Title,
			Summary:         canvas.Summary,
			Status:          status,
			Priority:        canvas.Priority,
			CreatedAt:       formatTime(now),
		}
		response, err = postCanvasEvent(ctx, cfg, event)
		if err != nil {
			return response, eventsPosted, err
		}
		eventsPosted++
		currentVersion = responseVersion(response, currentVersion+1)
	}

	desiredIDs := map[string]bool{}
	previousBlockID := ""
	for _, block := range canvas.Blocks {
		blockID := stringValue(block["id"])
		if blockID == "" {
			continue
		}
		desiredIDs[blockID] = true
		existingBlock, hadBlock := existingBlocks[blockID]
		if hadBlock && blocksEqual(existingBlock, block) {
			previousBlockID = blockID
			continue
		}
		event := CanvasLogEvent{
			ID:              dynamicEventID(canvas.ID, "block", now, eventsPosted+1),
			CanvasID:        canvas.ID,
			ExpectedVersion: &currentVersion,
			Block:           cloneBlock(block),
			CreatedAt:       formatTime(now),
		}
		if hadBlock {
			event.Type = "canvas.block.replaced"
			event.BlockID = blockID
		} else {
			event.Type = "canvas.block.appended"
			event.InsertAfterBlockID = previousBlockID
		}
		response, err = postCanvasEvent(ctx, cfg, event)
		if err != nil {
			return response, eventsPosted, err
		}
		eventsPosted++
		currentVersion = responseVersion(response, currentVersion+1)
		previousBlockID = blockID
	}

	for _, block := range existing.Blocks {
		blockID := stringValue(block["id"])
		if blockID == "" || desiredIDs[blockID] || !isSessionMonitorBlockID(blockID) {
			continue
		}
		event := CanvasLogEvent{
			ID:              dynamicEventID(canvas.ID, "remove", now, eventsPosted+1),
			CanvasID:        canvas.ID,
			Type:            "canvas.block.removed",
			ExpectedVersion: &currentVersion,
			BlockID:         blockID,
			CreatedAt:       formatTime(now),
		}
		response, err = postCanvasEvent(ctx, cfg, event)
		if err != nil {
			return response, eventsPosted, err
		}
		eventsPosted++
		currentVersion = responseVersion(response, currentVersion+1)
	}

	if response == nil {
		response = map[string]any{"id": canvas.ID, "version": currentVersion}
	}
	return response, eventsPosted, nil
}

func fetchHubCanvas(ctx context.Context, cfg Config, canvasID string) (Canvas, bool, error) {
	url := strings.TrimRight(cfg.HubURL, "/") + "/v1/canvases/" + canvasID
	ctx, cancel := context.WithTimeout(ctx, hubRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Canvas{}, false, err
	}
	req.Header.Set("Accept", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Canvas{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Canvas{}, false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Canvas{}, false, fmt.Errorf("GET /v1/canvases/%s returned %d: %s", canvasID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var canvas Canvas
	if err := json.NewDecoder(resp.Body).Decode(&canvas); err != nil {
		return Canvas{}, false, err
	}
	return canvas, true, nil
}

func postCanvasEvent(ctx context.Context, cfg Config, event CanvasLogEvent) (map[string]any, error) {
	body, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, hubRequestTimeout)
	defer cancel()
	url := strings.TrimRight(cfg.HubURL, "/") + "/v1/canvases/" + event.CanvasID + "/events"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST /v1/canvases/%s/events returned %d: %s", event.CanvasID, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var response map[string]any
	if len(data) > 0 {
		_ = json.Unmarshal(data, &response)
	}
	return response, nil
}

func mapBlocksByID(blocks []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, block := range blocks {
		if id := stringValue(block["id"]); id != "" {
			out[id] = block
		}
	}
	return out
}

func blocksEqual(left, right map[string]any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func cloneBlock(block map[string]any) map[string]any {
	raw, err := json.Marshal(block)
	if err != nil {
		return block
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return block
	}
	return cloned
}

func responseVersion(response map[string]any, fallback int) int {
	if response == nil {
		return fallback
	}
	switch value := response["version"].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
	}
	return fallback
}

func dynamicEventID(canvasID, kind string, now time.Time, sequence int) string {
	base := sanitizeID(canvasID)
	if base == "" {
		base = "session-monitor"
	}
	return fmt.Sprintf("%s-event-%s-%d-%03d", base, kind, now.UnixNano(), sequence)
}

func sanitizeID(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func isSessionMonitorBlockID(id string) bool {
	switch id {
	case "session-monitor-heading", "session-monitor-overview", "session-monitor-recent-sessions", "session-monitor-session-list", "session-monitor-scan-metadata", "session-monitor-collection", "session-monitor-provider-health", "session-monitor-next-steps", "session-monitor-scan-warnings":
		return true
	default:
		return strings.HasPrefix(id, "session-")
	}
}

func matchSessions(sessions []LocalSession, hub HubState) {
	for i := range sessions {
		sessions[i].Match = bestMatch(sessions[i], hub)
	}
}

func bestMatch(session LocalSession, hub HubState) MatchResult {
	for _, run := range hub.Runs {
		if sessionIDMatchesRun(session.SessionID, run) {
			return MatchResult{
				Status:    "exact",
				Reason:    "session id matched an AgentCanvas agent run",
				CanvasID:  firstNonEmpty(run.CanvasID, run.ID),
				RunID:     firstNonEmpty(run.ID, run.RunID),
				Score:     100,
				Evidence:  "agentRun.externalId/id/runId",
				Provider:  run.Provider,
				UpdatedAt: run.UpdatedAt,
			}
		}
	}
	for _, canvas := range hub.Canvases {
		if session.SessionID != "" && (session.SessionID == canvas.ID || session.SessionID == canvas.RunID) {
			return MatchResult{
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
	best := MatchResult{Status: "unmatched", Reason: "no matching AgentCanvas run or canvas found"}
	for _, run := range hub.Runs {
		if !providerCompatible(session.Provider, run.Provider) {
			continue
		}
		runCWD := runWorkspace(run)
		if runCWD == "" || !samePath(session.CWD, runCWD) {
			continue
		}
		sessionUpdated := timeFromRFC3339(session.UpdatedAt)
		runUpdated := parseTime(run.UpdatedAt)
		if !sessionUpdated.IsZero() && !runUpdated.IsZero() && absDuration(sessionUpdated.Sub(runUpdated)) > likelyMatchWindow {
			continue
		}
		score := 70
		if !sessionUpdated.IsZero() && !runUpdated.IsZero() {
			hours := int(absDuration(sessionUpdated.Sub(runUpdated)).Hours())
			score -= minInt(hours, 20)
		}
		if score > best.Score {
			best = MatchResult{
				Status:    "likely",
				Reason:    "provider and workspace matched within the recent update window",
				CanvasID:  run.CanvasID,
				RunID:     firstNonEmpty(run.ID, run.RunID),
				Score:     score,
				Evidence:  "provider+cwd+updatedAt",
				Provider:  run.Provider,
				UpdatedAt: run.UpdatedAt,
			}
		}
	}
	return best
}

func sessionIDMatchesRun(sessionID string, run HubRun) bool {
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

func runWorkspace(run HubRun) string {
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

func nextCanvasVersion(canvasID string, hub HubState) int {
	version := 1
	for _, canvas := range hub.Canvases {
		if canvas.ID == canvasID && canvas.Version >= version {
			version = canvas.Version + 1
		}
	}
	return version
}

func fillSessionReviews(sessions []LocalSession, now time.Time) {
	for i := range sessions {
		sessions[i].Review = sessionReview(sessions[i], now)
	}
}

func buildCanvas(cfg Config, sessions []LocalSession, health []ProviderHealth, scanErrors []string, hub HubState, version int, now time.Time) Canvas {
	sessions = sortSessionsByUpdatedAt(sessions)
	fillSessionReviews(sessions, now)
	counts := countMatches(sessions)
	summary := fmt.Sprintf("Scanned %d local Codex/Claude sessions: %d exact, %d likely, %d unmatched.", len(sessions), counts["exact"], counts["likely"], counts["unmatched"])
	blocks := []map[string]any{
		{
			"id":    "session-monitor-heading",
			"kind":  "heading",
			"level": 1,
			"text":  sessionMonitorTitle,
		},
		{
			"id":       "session-monitor-overview",
			"kind":     "markdown",
			"markdown": overviewMarkdown(sessions, health, scanErrors, hub, now),
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
			},
		},
	}
	items := make([]map[string]any, 0, len(sessions))
	for i, session := range sessions {
		review := sessionReview(session, now)
		blockID := fmt.Sprintf("session-%02d-summary", i+1)
		metaID := fmt.Sprintf("session-%02d-metadata", i+1)
		label := fmt.Sprintf("%s: %s", displayProvider(session.Provider), session.Title)
		badges := []string{session.Provider, firstNonEmpty(session.Status, "unknown"), sessionActivityStatus(session, now), session.Match.Status}
		if collapsed := session.Metadata["collapsedFiles"]; collapsed != "" {
			badges = append(badges, "collapsed:"+collapsed)
		}
		items = append(items, map[string]any{
			"id":       fmt.Sprintf("session-%02d", i+1),
			"label":    truncate(label, 120),
			"subtitle": truncate(sessionListSubtitle(session, now), 420),
			"status":   session.Match.Status,
			"badges":   badges,
			"addedAt":  firstNonEmpty(session.UpdatedAt, formatTime(now)),
			"blockIds": []string{blockID, metaID},
		})
		blocks = append(blocks,
			map[string]any{
				"id":       blockID,
				"kind":     "markdown",
				"markdown": sessionMarkdown(session, now),
			},
			map[string]any{
				"id":    metaID,
				"kind":  "metadata",
				"title": "Session metadata",
				"metadata": map[string]any{
					"provider":      session.Provider,
					"sessionId":     session.SessionID,
					"cwd":           session.CWD,
					"status":        session.Status,
					"sourcePath":    session.SourcePath,
					"createdAt":     session.CreatedAt,
					"updatedAt":     session.UpdatedAt,
					"resumeHint":    session.ResumeHint,
					"matchStatus":   session.Match.Status,
					"matchReason":   session.Match.Reason,
					"matchCanvas":   session.Match.CanvasID,
					"matchRun":      session.Match.RunID,
					"matchScore":    session.Match.Score,
					"matchEvidence": session.Match.Evidence,
					"purpose":       review.Purpose,
					"currentState":  review.CurrentState,
					"nextStep":      review.NextStep,
					"signals":       strings.Join(review.Signals, "; "),
				},
			},
		)
	}
	pageSize := cfg.MaxSessions
	if pageSize <= 0 {
		pageSize = defaultMaxSessions
	}
	collection := map[string]any{
		"id":       "session-monitor-collection",
		"kind":     "collection",
		"title":    "Recent sessions (newest first)",
		"mode":     "paged-list",
		"pageSize": pageSize,
		"items":    items,
	}
	blocks = append(blocks[:2], append([]map[string]any{collection}, blocks[2:]...)...)
	if len(scanErrors) > 0 {
		blocks = append(blocks, scanWarningsBlock(scanErrors))
	}
	blocks = append(blocks, providerHealthBlock(health), nextStepsBlock())
	return Canvas{
		ID:        cfg.CanvasID,
		AgentID:   defaultAgentID,
		RunID:     cfg.RunID,
		Title:     sessionMonitorTitle,
		Summary:   summary,
		Status:    "ready_for_review",
		Mode:      "static",
		Priority:  "normal",
		CreatedAt: formatTime(now),
		UpdatedAt: formatTime(now),
		Version:   version,
		Blocks:    blocks,
	}
}

func sortSessionsByUpdatedAt(sessions []LocalSession) []LocalSession {
	out := append([]LocalSession(nil), sessions...)
	sort.SliceStable(out, func(i, j int) bool {
		left := timeFromRFC3339(out[i].UpdatedAt)
		right := timeFromRFC3339(out[j].UpdatedAt)
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

func compactDynamicCanvas(canvas Canvas) Canvas {
	keep := map[string]bool{
		"session-monitor-heading":         true,
		"session-monitor-overview":        true,
		"session-monitor-collection":      true,
		"session-monitor-scan-metadata":   true,
		"session-monitor-scan-warnings":   true,
		"session-monitor-provider-health": true,
		"session-monitor-next-steps":      true,
	}
	blocks := make([]map[string]any, 0, len(keep))
	for _, block := range canvas.Blocks {
		blockID := stringValue(block["id"])
		if keep[blockID] || strings.HasPrefix(blockID, "session-") {
			blocks = append(blocks, block)
		}
	}
	canvas.Blocks = blocks
	canvas.Mode = "dynamic"
	return canvas
}

func overviewMarkdown(sessions []LocalSession, health []ProviderHealth, scanErrors []string, hub HubState, now time.Time) string {
	counts := countMatches(sessions)
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
	fmt.Fprintf(&b, "- Next decision: open rows that are active, unmatched, or collapsed; link useful sessions to canvases/runs later. V1 still does not nudge or resume sessions.\n\n")
	fmt.Fprintf(&b, "## Scan summary\n\n")
	fmt.Fprintf(&b, "- Sessions: %d\n", len(sessions))
	fmt.Fprintf(&b, "- Codex sessions: %d\n", providers["codex"])
	fmt.Fprintf(&b, "- Claude sessions: %d\n", providers["claude"])
	fmt.Fprintf(&b, "- Activity: active %d, recent %d, idle %d, stale %d\n", activity["active"], activity["recent"], activity["idle"], activity["stale"])
	fmt.Fprintf(&b, "- Matches: exact %d, likely %d, unmatched %d\n", counts["exact"], counts["likely"], counts["unmatched"])
	fmt.Fprintf(&b, "- Hub inventory: %d canvases, %d agent runs\n", len(hub.Canvases), len(hub.Runs))
	fmt.Fprintf(&b, "- Nudge delivery: disabled in v1; resume hints are recorded for a later explicit action path.\n")
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

func recentSessionsMarkdown(sessions []LocalSession, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Recent sessions (newest first)\n\n")
	if len(sessions) == 0 {
		fmt.Fprintf(&b, "No Codex or Claude sessions matched this scan window.\n")
		return b.String()
	}
	for i, session := range sessions {
		fmt.Fprintf(&b, "%d. **%s**\n", i+1, firstNonEmpty(session.Title, displayProvider(session.Provider)+" "+shortID(session.SessionID)))
		fmt.Fprintf(
			&b,
			"   - Labels: `provider:%s` `status:%s` `activity:%s` `match:%s`\n",
			session.Provider,
			firstNonEmpty(session.Status, "unknown"),
			sessionActivityStatus(session, now),
			firstNonEmpty(session.Match.Status, "unmatched"),
		)
		fmt.Fprintf(
			&b,
			"   - Provider: `%s` - status: `%s` - activity: `%s` (%s) - match: **%s**\n",
			session.Provider,
			firstNonEmpty(session.Status, "unknown"),
			sessionActivityStatus(session, now),
			sessionAgeLabel(session, now),
			firstNonEmpty(session.Match.Status, "unmatched"),
		)
		fmt.Fprintf(&b, "   - Updated: `%s`", firstNonEmpty(session.UpdatedAt, "unknown"))
		if session.CWD != "" {
			fmt.Fprintf(&b, " - workspace: `%s`", truncate(session.CWD, 140))
		}
		fmt.Fprintf(&b, "\n")
		if latest := latestMessagePreview(session); latest != "" {
			fmt.Fprintf(&b, "   - Latest: %s\n", latest)
		}
		if session.Match.CanvasID != "" || session.Match.RunID != "" {
			fmt.Fprintf(&b, "   - Link: `%s` / `%s`\n", firstNonEmpty(session.Match.CanvasID, "no canvas"), firstNonEmpty(session.Match.RunID, "no run"))
		}
	}
	return truncate(b.String(), recentMarkdownCap)
}

func sessionListSubtitle(session LocalSession, now time.Time) string {
	review := sessionReview(session, now)
	parts := []string{
		"Purpose: " + review.Purpose,
		"Now: " + review.CurrentState,
		"Next: " + review.NextStep,
	}
	return strings.Join(parts, " - ")
}

func sessionReview(session LocalSession, now time.Time) SessionReview {
	if session.Review.Purpose != "" && session.Review.CurrentState != "" && session.Review.NextStep != "" {
		return session.Review
	}
	purpose := inferSessionPurpose(session)
	current := inferCurrentState(session, now)
	next := inferNextStep(session, now)
	signals := []string{
		fmt.Sprintf("provider %s, status %s, activity %s (%s)", session.Provider, firstNonEmpty(session.Status, "unknown"), sessionActivityStatus(session, now), sessionAgeLabel(session, now)),
		fmt.Sprintf("match %s", firstNonEmpty(session.Match.Status, "unmatched")),
	}
	if session.CWD != "" {
		signals = append(signals, "workspace "+displayCWD(session.CWD))
	}
	if collapsed := session.Metadata["collapsedFiles"]; collapsed != "" {
		signals = append(signals, "collapsed "+collapsed+" local files into this session")
	}
	if session.Match.CanvasID != "" || session.Match.RunID != "" {
		signals = append(signals, "linked candidate "+firstNonEmpty(session.Match.CanvasID, "no canvas")+" / "+firstNonEmpty(session.Match.RunID, "no run"))
	}
	if latest := latestMessagePlain(session); latest != "" {
		signals = append(signals, "latest "+latest)
	}
	return SessionReview{
		Purpose:      truncate(purpose, 180),
		CurrentState: truncate(current, 220),
		NextStep:     truncate(next, 180),
		Signals:      capStrings(signals, 5),
	}
}

func inferSessionPurpose(session LocalSession) string {
	if title := strings.TrimSpace(session.Title); title != "" {
		return title
	}
	for _, message := range session.LatestMessages {
		if strings.EqualFold(message.Role, "user") && strings.TrimSpace(message.Text) != "" {
			return "Respond to: " + strings.Join(strings.Fields(message.Text), " ")
		}
	}
	if session.CWD != "" {
		return fmt.Sprintf("Track %s activity in %s", displayProvider(session.Provider), displayCWD(session.CWD))
	}
	return fmt.Sprintf("Track %s session %s", displayProvider(session.Provider), shortID(session.SessionID))
}

func inferCurrentState(session LocalSession, now time.Time) string {
	parts := []string{
		fmt.Sprintf("%s, %s", firstNonEmpty(session.Status, "unknown"), sessionActivityStatus(session, now)),
		sessionAgeLabel(session, now),
		fmt.Sprintf("match %s", firstNonEmpty(session.Match.Status, "unmatched")),
	}
	if collapsed := session.Metadata["collapsedFiles"]; collapsed != "" {
		parts = append(parts, "collapsed "+collapsed+" files")
	}
	if latest := latestMessagePlain(session); latest != "" {
		parts = append(parts, latest)
	}
	return strings.Join(parts, "; ")
}

func inferNextStep(session LocalSession, now time.Time) string {
	match := firstNonEmpty(session.Match.Status, "unmatched")
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

func capStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return append([]string(nil), values[:limit]...)
}

func sessionActivityStatus(session LocalSession, now time.Time) string {
	updated := timeFromRFC3339(session.UpdatedAt)
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

func sessionAgeLabel(session LocalSession, now time.Time) string {
	updated := timeFromRFC3339(session.UpdatedAt)
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

func latestMessagePreview(session LocalSession) string {
	if len(session.LatestMessages) == 0 {
		return ""
	}
	message := session.LatestMessages[len(session.LatestMessages)-1]
	text := strings.Join(strings.Fields(message.Text), " ")
	if text == "" {
		return ""
	}
	return fmt.Sprintf("**%s:** %s", firstNonEmpty(message.Role, "message"), truncate(text, recentPreviewLimit))
}

func latestMessagePlain(session LocalSession) string {
	if len(session.LatestMessages) == 0 {
		return ""
	}
	message := session.LatestMessages[len(session.LatestMessages)-1]
	text := strings.Join(strings.Fields(message.Text), " ")
	if text == "" {
		return ""
	}
	return fmt.Sprintf("%s: %s", firstNonEmpty(message.Role, "message"), truncate(text, recentPreviewLimit))
}

func scanWarningsBlock(scanErrors []string) map[string]any {
	metadata := map[string]any{"count": len(scanErrors)}
	for i, err := range scanErrors {
		metadata[fmt.Sprintf("warning%02d", i+1)] = truncate(err, 500)
	}
	return map[string]any{
		"id":       "session-monitor-scan-warnings",
		"kind":     "metadata",
		"title":    "Scan warnings",
		"metadata": metadata,
	}
}

func providerHealthBlock(health []ProviderHealth) map[string]any {
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

func sessionMarkdown(session LocalSession, now time.Time) string {
	var b strings.Builder
	review := sessionReview(session, now)
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
	fmt.Fprintf(&b, "- Updated: %s\n", firstNonEmpty(session.UpdatedAt, "unknown"))
	fmt.Fprintf(&b, "- Match: **%s**", session.Match.Status)
	if session.Match.CanvasID != "" || session.Match.RunID != "" {
		fmt.Fprintf(&b, " (`%s` / `%s`)", firstNonEmpty(session.Match.CanvasID, "no canvas"), firstNonEmpty(session.Match.RunID, "no run"))
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
	return truncate(b.String(), sessionMarkdownCap)
}

func countMatches(sessions []LocalSession) map[string]int {
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

func parseRecordTime(record map[string]any) time.Time {
	if got := parseTime(stringValue(record["timestamp"])); !got.IsZero() {
		return got
	}
	if got := timeFromMillis(record["timestamp"]); !got.IsZero() {
		return got
	}
	payload := mapValue(record["payload"])
	if payload != nil {
		if got := parseTime(stringValue(payload["timestamp"])); !got.IsZero() {
			return got
		}
		if got := timeFromSeconds(payload["started_at"]); !got.IsZero() {
			return got
		}
	}
	return time.Time{}
}

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z07:00"} {
		if got, err := time.Parse(layout, raw); err == nil {
			return got.UTC()
		}
	}
	return time.Time{}
}

func timeFromRFC3339(raw string) time.Time {
	return parseTime(raw)
}

func timeFromMillis(value any) time.Time {
	switch got := value.(type) {
	case float64:
		if got > 0 {
			return time.UnixMilli(int64(got)).UTC()
		}
	case json.Number:
		if n, err := got.Int64(); err == nil && n > 0 {
			return time.UnixMilli(n).UTC()
		}
	}
	return time.Time{}
}

func timeFromSeconds(value any) time.Time {
	switch got := value.(type) {
	case float64:
		if got > 0 {
			sec := int64(got)
			nsec := int64((got - float64(sec)) * 1e9)
			return time.Unix(sec, nsec).UTC()
		}
	}
	return time.Time{}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func earliest(current, candidate time.Time) time.Time {
	if candidate.IsZero() {
		return current
	}
	if current.IsZero() || candidate.Before(current) {
		return candidate
	}
	return current
}

func latest(current, candidate time.Time) time.Time {
	if candidate.IsZero() {
		return current
	}
	if current.IsZero() || candidate.After(current) {
		return candidate
	}
	return current
}

func lastMessages(messages []MessageSummary, limit int) []MessageSummary {
	if len(messages) <= limit {
		return messages
	}
	return append([]MessageSummary(nil), messages[len(messages)-limit:]...)
}

func extractText(value any) string {
	switch got := value.(type) {
	case nil:
		return ""
	case string:
		return got
	case []any:
		parts := make([]string, 0, len(got))
		for _, item := range got {
			if text := extractText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	case map[string]any:
		if typ := stringValue(got["type"]); typ == "tool_use" || typ == "tool_result" || typ == "function_call" {
			return ""
		}
		for _, key := range []string{"text", "message"} {
			if text := stringValue(got[key]); text != "" {
				return text
			}
		}
		if content, ok := got["content"]; ok {
			return extractText(content)
		}
	}
	return ""
}

func summarizeText(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	return truncate(text, limit)
}

func truncate(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
}

func mapValue(value any) map[string]any {
	if got, ok := value.(map[string]any); ok {
		return got
	}
	return nil
}

func stringValue(value any) string {
	switch got := value.(type) {
	case string:
		return got
	case json.Number:
		return got.String()
	default:
		return ""
	}
}

func numberString(value any) string {
	switch got := value.(type) {
	case float64:
		return strconv.FormatInt(int64(got), 10)
	case json.Number:
		return got.String()
	case string:
		return got
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func boolEnv(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func lookbackEnv() time.Duration {
	if value := os.Getenv("SESSION_MONITOR_LOOKBACK_HOURS"); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed > 0 {
			return time.Duration(parsed * float64(time.Hour))
		}
	}
	return defaultLookback
}

var codexIDPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func codexIDFromPath(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	matches := codexIDPattern.FindAllString(base, -1)
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	if idx := strings.LastIndex(base, "-"); idx >= 0 && idx+1 < len(base) {
		return base[idx+1:]
	}
	return base
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func samePath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func displayCWD(cwd string) string {
	if cwd == "" {
		return "no workspace"
	}
	return cwd
}

func displayProvider(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "Unknown"
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func countJSONLLines(path string) (int, string) {
	file, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer file.Close()
	count := 0
	scanner := newJSONLScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return count, err.Error()
	}
	return count, ""
}

func newJSONLScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), maxJSONLTokenBytes)
	return scanner
}
