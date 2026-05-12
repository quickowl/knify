package scan

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func scanCursorHealth() types.ProviderHealth {
	health := types.ProviderHealth{
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
