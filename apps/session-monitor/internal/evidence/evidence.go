package evidence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/hub"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func EnrichSessions(ctx context.Context, cfg types.Config, sessions []types.LocalSession) []string {
	var warnings []string
	fullCanvases := map[string]types.Canvas{}
	for i := range sessions {
		sessions[i].Artifacts = capArtifacts(dedupeArtifacts(sessions[i].Artifacts))
		if cfg.HubURL != "" && sessions[i].Match.CanvasID != "" {
			canvas, ok := fullCanvases[sessions[i].Match.CanvasID]
			if !ok {
				fetched, found, err := hub.FetchHubCanvas(ctx, cfg, sessions[i].Match.CanvasID)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("evidence fetch canvas %s: %v", sessions[i].Match.CanvasID, err))
				} else if found {
					canvas = fetched
					ok = true
					fullCanvases[sessions[i].Match.CanvasID] = fetched
				}
			}
			if ok {
				sessions[i].Artifacts = appendHubCanvasEvidence(sessions[i].Artifacts, canvas)
			}
		}
		sessions[i].Artifacts = capArtifacts(dedupeArtifacts(sessions[i].Artifacts))
		if cfg.HubURL != "" && !cfg.DryRun {
			uploadImageArtifacts(ctx, cfg, &sessions[i], &warnings)
		}
		sessions[i].ArtifactCount = len(sessions[i].Artifacts)
	}
	return warnings
}

func appendHubCanvasEvidence(artifacts []types.SessionArtifact, canvas types.Canvas) []types.SessionArtifact {
	kinds := canvasEvidenceKinds(canvas)
	if len(kinds) == 0 {
		return artifacts
	}
	item := types.SessionArtifact{
		ID:      core.SanitizeID("artifact-hub-canvas-" + core.ShortHash(canvas.ID)),
		Kind:    "canvas",
		Title:   core.FirstNonEmpty(canvas.Title, canvas.ID),
		Source:  "hub-canvas",
		Summary: fmt.Sprintf("Linked canvas %s has review evidence: %s", canvas.ID, strings.Join(kinds, ", ")),
	}
	return capArtifacts(appendArtifact(artifacts, item))
}

func canvasEvidenceKinds(canvas types.Canvas) []string {
	seen := map[string]bool{}
	var kinds []string
	for _, block := range canvas.Blocks {
		kind := core.StringValue(block["kind"])
		hasEvidence := false
		switch kind {
		case "terminal", "diff", "image", "chart":
			hasEvidence = true
		case "checklist":
			hasEvidence = checklistHasCheckedItem(block["items"])
		}
		if hasEvidence && !seen[kind] {
			seen[kind] = true
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func checklistHasCheckedItem(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if checked, ok := item["checked"].(bool); ok && checked {
			return true
		}
		if checked, ok := item["isComplete"].(bool); ok && checked {
			return true
		}
	}
	return false
}

func uploadImageArtifacts(ctx context.Context, cfg types.Config, session *types.LocalSession, warnings *[]string) {
	for i := range session.Artifacts {
		artifact := &session.Artifacts[i]
		if artifact.Kind != "image" || artifact.Path == "" || artifact.AssetID != "" {
			continue
		}
		if artifact.Size > types.ImageArtifactCap {
			artifact.Summary = fmt.Sprintf("image exceeds %d byte upload cap", types.ImageArtifactCap)
			continue
		}
		file, err := os.Open(artifact.Path)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("evidence open image %s: %v", artifact.Path, err))
			continue
		}
		response, err := hub.UploadAsset(ctx, cfg, core.FirstNonEmpty(artifact.ContentType, "application/octet-stream"), file)
		closeErr := file.Close()
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("evidence upload image %s: %v", artifact.Path, err))
			continue
		}
		if closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			*warnings = append(*warnings, fmt.Sprintf("evidence close image %s: %v", artifact.Path, closeErr))
		}
		artifact.AssetID = core.FirstNonEmpty(response.AssetID, response.ID)
		artifact.URL = response.URL
		artifact.Summary = "uploaded to Hub asset storage"
	}
}

func dedupeArtifacts(items []types.SessionArtifact) []types.SessionArtifact {
	var out []types.SessionArtifact
	seen := map[string]bool{}
	for _, item := range items {
		key := artifactKey(item)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func appendArtifact(items []types.SessionArtifact, item types.SessionArtifact) []types.SessionArtifact {
	key := artifactKey(item)
	if key == "" {
		return items
	}
	for _, existing := range items {
		if artifactKey(existing) == key {
			return items
		}
	}
	return append(items, item)
}

func capArtifacts(items []types.SessionArtifact) []types.SessionArtifact {
	if len(items) <= types.MaxSessionArtifacts {
		return items
	}
	return append([]types.SessionArtifact(nil), items[:types.MaxSessionArtifacts]...)
}

func artifactKey(item types.SessionArtifact) string {
	switch {
	case item.Path != "":
		return "path:" + filepath.Clean(item.Path)
	case item.URL != "":
		return "url:" + item.URL
	case item.AssetID != "":
		return "asset:" + item.AssetID
	case item.Source == "hub-canvas" && item.ID != "":
		return "id:" + item.ID
	default:
		return ""
	}
}
