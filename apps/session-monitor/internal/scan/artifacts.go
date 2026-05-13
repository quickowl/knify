package scan

import (
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

var artifactMentionPattern = regexp.MustCompile(`(?i)(https?://[^\s\])}>,"']+\.(?:md|markdown|log|txt|png|jpe?g|webp)(?:\?[^\s\])}>,"']*)?|(?:~|\.{1,2}/|/|[A-Za-z0-9_.-]+/)?[A-Za-z0-9_.@%+/\-]+?\.(?:md|markdown|log|txt|png|jpe?g|webp))`)

const artifactTextScanCap = 64 * 1024

func extractArtifactsFromText(text, cwd, source string) []types.SessionArtifact {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if len(text) > artifactTextScanCap {
		text = text[:artifactTextScanCap]
	}
	var out []types.SessionArtifact
	seen := map[string]bool{}
	for _, raw := range artifactMentionPattern.FindAllString(text, -1) {
		artifact, ok := artifactFromMention(raw, cwd, source)
		if !ok {
			continue
		}
		key := artifactKey(artifact)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, artifact)
		if len(out) >= types.MaxSessionArtifacts {
			break
		}
	}
	return out
}

func artifactFromMention(raw, cwd, source string) (types.SessionArtifact, bool) {
	mention := cleanArtifactMention(raw)
	if mention == "" {
		return types.SessionArtifact{}, false
	}
	if strings.HasPrefix(strings.ToLower(mention), "http://") || strings.HasPrefix(strings.ToLower(mention), "https://") {
		parsed, err := url.Parse(mention)
		if err != nil {
			return types.SessionArtifact{}, false
		}
		kind := artifactKind(parsed.Path)
		if kind == "" {
			return types.SessionArtifact{}, false
		}
		return types.SessionArtifact{
			ID:          artifactID(source, mention),
			Kind:        kind,
			Title:       artifactTitle(parsed.Path),
			Source:      source,
			URL:         mention,
			ContentType: artifactContentType(parsed.Path),
			Summary:     "linked by agent output",
		}, true
	}

	path := mention
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	} else if !filepath.IsAbs(path) {
		if cwd == "" {
			return types.SessionArtifact{}, false
		}
		path = filepath.Join(cwd, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return types.SessionArtifact{}, false
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return types.SessionArtifact{}, false
	}
	kind := artifactKind(abs)
	if kind == "" {
		return types.SessionArtifact{}, false
	}
	return types.SessionArtifact{
		ID:          artifactID(source, abs),
		Kind:        kind,
		Title:       artifactTitle(abs),
		Source:      source,
		Path:        abs,
		ContentType: artifactContentType(abs),
		Size:        info.Size(),
		Summary:     "mentioned by agent output",
	}, true
}

func cleanArtifactMention(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.Trim(value, "`\"'()[]{}<>")
	value = strings.TrimRight(value, ".,;:!?")
	return value
}

func artifactKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return "markdown"
	case ".log", ".txt":
		return "terminal"
	case ".png", ".jpg", ".jpeg", ".webp":
		return "image"
	default:
		return ""
	}
}

func artifactContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".log", ".txt":
		return "text/plain; charset=utf-8"
	}
	if got := mime.TypeByExtension(filepath.Ext(path)); got != "" {
		return got
	}
	return "application/octet-stream"
}

func artifactTitle(path string) string {
	base := filepath.Base(path)
	if base == "." || base == "/" || base == "" {
		return "Review artifact"
	}
	return base
}

func artifactID(source, key string) string {
	return core.SanitizeID("artifact-" + source + "-" + core.ShortHash(key))
}

func mergeArtifacts(left, right []types.SessionArtifact) []types.SessionArtifact {
	out := append([]types.SessionArtifact(nil), left...)
	seen := map[string]bool{}
	for _, item := range out {
		if key := artifactKey(item); key != "" {
			seen[key] = true
		}
	}
	for _, item := range right {
		key := artifactKey(item)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
		if len(out) >= types.MaxSessionArtifacts {
			break
		}
	}
	return out
}

func artifactKey(item types.SessionArtifact) string {
	switch {
	case item.Path != "":
		return "path:" + filepath.Clean(item.Path)
	case item.URL != "":
		return "url:" + item.URL
	case item.AssetID != "":
		return "asset:" + item.AssetID
	case item.ID != "":
		return "id:" + item.ID
	default:
		return ""
	}
}
