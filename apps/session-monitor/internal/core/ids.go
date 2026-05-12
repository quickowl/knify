package core

import (
	"path/filepath"
	"strings"
)

func ShortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func SanitizeID(value string) string {
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

func SamePath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func DisplayCWD(cwd string) string {
	if cwd == "" {
		return "no workspace"
	}
	return cwd
}

func DisplayProvider(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "Unknown"
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
}

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
