package main

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

var (
	buildVersion = ""
	buildCommit  = ""
	buildDate    = ""
)

func daemonBuildInfo() types.DaemonBuildInfo {
	info := types.DaemonBuildInfo{Version: "dev"}
	if buildVersion != "" {
		info.Version = buildVersion
	}
	if buildCommit != "" {
		info.Revision = buildCommit
		info.RevisionShort = shortRevision(buildCommit)
	}
	if buildDate != "" {
		info.CommitTime = buildDate
	}

	if build, ok := debug.ReadBuildInfo(); ok {
		if build.Main.Version != "" && build.Main.Version != "(devel)" && buildVersion == "" {
			info.Version = build.Main.Version
		}
		info.GoVersion = build.GoVersion
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				if info.Revision == "" {
					info.Revision = setting.Value
					info.RevisionShort = shortRevision(setting.Value)
				}
			case "vcs.time":
				if info.CommitTime == "" {
					info.CommitTime = setting.Value
				}
			case "vcs.modified":
				info.Modified = setting.Value == "true"
			}
		}
	}

	if executable, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(executable); err == nil {
			executable = resolved
		}
		info.BinaryPath = executable
		if stat, err := os.Stat(executable); err == nil {
			info.BinaryModifiedAt = core.FormatTime(stat.ModTime().UTC())
			info.BinarySize = stat.Size()
		}
	}
	return info
}

func shortRevision(revision string) string {
	revision = strings.TrimSpace(revision)
	if len(revision) <= 12 {
		return revision
	}
	return revision[:12]
}
