package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func parseConfig(args []string) (types.Config, bool, error) {
	home, _ := os.UserHomeDir()
	cfg := types.Config{
		HubURL:      core.FirstEnv("SESSION_MONITOR_HUB_URL", "HUB_BASE_URL", "AGENTCANVAS_HUB_URL"),
		Token:       core.FirstEnv("SESSION_MONITOR_HUB_TOKEN", "HUB_TOKEN", "AGENTCANVAS_HUB_TOKEN"),
		CanvasID:    core.EnvOrDefault("SESSION_MONITOR_CANVAS_ID", types.DefaultCanvasID),
		RunID:       core.EnvOrDefault("SESSION_MONITOR_RUN_ID", types.DefaultRunID),
		CodexHome:   filepath.Join(home, ".codex"),
		ClaudeHome:  filepath.Join(home, ".claude"),
		Interval:    core.DurationEnv("SESSION_MONITOR_INTERVAL", time.Minute),
		StaleAfter:  core.DurationEnv("SESSION_MONITOR_STALE_AFTER", types.DefaultStaleAfter),
		Lookback:    core.LookbackEnv(),
		MaxSessions: core.IntEnv("SESSION_MONITOR_MAX_SESSIONS", types.DefaultMaxSessions),
		Dynamic:     core.BoolEnv("SESSION_MONITOR_DYNAMIC", false),
		Now:         func() time.Time { return time.Now().UTC() },
	}
	flags := flag.NewFlagSet("session-monitor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	once := flags.Bool("once", false, "scan once and exit")
	flags.BoolVar(&cfg.Status, "status", false, "read the last --out heartbeat and report daemon health")
	flags.BoolVar(&cfg.Watch, "watch", false, "scan repeatedly")
	flags.DurationVar(&cfg.Interval, "interval", cfg.Interval, "watch interval")
	flags.DurationVar(&cfg.StaleAfter, "stale-after", cfg.StaleAfter, "maximum acceptable heartbeat age for --status")
	flags.IntVar(&cfg.RecentLimit, "recent", core.IntEnv("SESSION_MONITOR_RECENT", 0), "with --status, print this many recent tracked sessions")
	flags.BoolVar(&cfg.DryRun, "dry-run", false, "do not publish to the hub")
	flags.BoolVar(&cfg.Dynamic, "dynamic", cfg.Dynamic, "publish through the dynamic canvas event protocol")
	flags.StringVar(&cfg.OutPath, "out", core.FirstEnv("SESSION_MONITOR_OUT"), "write scan result JSON to this path")
	flags.StringVar(&cfg.HubURL, "hub-url", cfg.HubURL, "AgentCanvas hub base URL")
	flags.StringVar(&cfg.Token, "token", cfg.Token, "AgentCanvas hub bearer token")
	flags.StringVar(&cfg.CanvasID, "canvas-id", cfg.CanvasID, "review canvas id")
	flags.StringVar(&cfg.RunID, "run-id", cfg.RunID, "review run id")
	flags.StringVar(&cfg.CodexHome, "codex-home", cfg.CodexHome, "Codex home directory")
	flags.StringVar(&cfg.ClaudeHome, "claude-home", cfg.ClaudeHome, "Claude Code home directory")
	flags.DurationVar(&cfg.Lookback, "lookback", cfg.Lookback, "session lookback duration")
	flags.IntVar(&cfg.MaxSessions, "max-sessions", cfg.MaxSessions, "maximum local sessions to include")
	if err := flags.Parse(args); err != nil {
		return types.Config{}, false, fmt.Errorf("usage: %s", usage())
	}
	if cfg.CanvasID == "" || cfg.RunID == "" {
		return types.Config{}, false, errors.New("--canvas-id and --run-id are required")
	}
	if cfg.MaxSessions <= 0 {
		return types.Config{}, false, errors.New("--max-sessions must be positive")
	}
	if cfg.Lookback <= 0 {
		return types.Config{}, false, errors.New("--lookback must be positive")
	}
	return cfg, *once, nil
}

func usage() string {
	return "session-monitor --once|--watch|--status [--dry-run] [--dynamic] [--hub-url URL] [--out FILE]"
}
