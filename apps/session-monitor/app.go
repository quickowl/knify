package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/canvas"
	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/hub"
	"github.com/quickowl/knify/apps/session-monitor/internal/match"
	"github.com/quickowl/knify/apps/session-monitor/internal/scan"
	"github.com/quickowl/knify/apps/session-monitor/internal/status"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func runMain(ctx context.Context, args []string) error {
	cfg, once, err := parseConfig(args)
	if err != nil {
		return err
	}
	if cfg.Status {
		return status.WriteStatus(ctx, cfg, os.Stdout)
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

func runOnce(ctx context.Context, cfg types.Config) (types.ScanResult, error) {
	now := cfg.Now().UTC()
	sessions, health, scanErrors := scan.ScanLocalSessions(ctx, cfg, now)
	hubState := types.HubState{}
	var hubErrors []string
	if cfg.HubURL != "" {
		var err error
		hubState, err = hub.FetchHubState(ctx, cfg)
		if err != nil {
			hubErrors = append(hubErrors, err.Error())
		}
	}
	match.MatchSessions(sessions, hubState)
	sort.SliceStable(sessions, func(i, j int) bool {
		return core.TimeFromRFC3339(sessions[i].UpdatedAt).After(core.TimeFromRFC3339(sessions[j].UpdatedAt))
	})
	if len(sessions) > cfg.MaxSessions {
		sessions = sessions[:cfg.MaxSessions]
	}
	canvas.FillSessionReviews(sessions, now)
	version := match.NextCanvasVersion(cfg.CanvasID, hubState)
	built := canvas.BuildCanvas(cfg, sessions, health, append(scanErrors, hubErrors...), hubState, version, now)
	if cfg.Dynamic {
		built = canvas.CompactDynamicCanvas(built)
	}
	result := types.ScanResult{
		GeneratedAt:    core.FormatTime(now),
		Process:        status.ProcessInfo(cfg),
		Lookback:       cfg.Lookback.String(),
		MaxSessions:    cfg.MaxSessions,
		Sessions:       sessions,
		ProviderHealth: health,
		Errors:         append(scanErrors, hubErrors...),
		Hub: types.HubSummary{
			URL:         cfg.HubURL,
			CanvasCount: len(hubState.Canvases),
			RunCount:    len(hubState.Runs),
		},
		Canvas:  built,
		Dynamic: cfg.Dynamic,
		OpenAIDocs: types.OpenAIDocsEvidence{
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
	response, eventsPosted, err := hub.PublishCanvas(ctx, cfg, built)
	if err != nil {
		return result, err
	}
	result.Posted = true
	result.EventsPosted = eventsPosted
	result.PostResponse = response
	return result, nil
}

func writeResult(cfg types.Config, result types.ScanResult) error {
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
