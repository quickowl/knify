package canvas

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func sessionArtifactBlocks(sessionIndex int, session types.LocalSession) []map[string]any {
	blocks := make([]map[string]any, 0, len(session.Artifacts))
	for i, artifact := range session.Artifacts {
		blockID := fmt.Sprintf("session-%02d-artifact-%02d", sessionIndex, i+1)
		blocks = append(blocks, artifactBlock(blockID, artifact))
	}
	return blocks
}

func artifactBlock(blockID string, artifact types.SessionArtifact) map[string]any {
	switch artifact.Kind {
	case "markdown":
		return map[string]any{
			"id":       blockID,
			"kind":     "markdown",
			"markdown": artifactMarkdown(artifact),
		}
	case "terminal":
		return map[string]any{
			"id":       blockID,
			"kind":     "terminal",
			"title":    artifact.Title,
			"command":  artifactCommand(artifact),
			"output":   artifactText(artifact),
			"exitCode": 0,
		}
	case "image":
		block := map[string]any{
			"id":      blockID,
			"kind":    "image",
			"alt":     core.FirstNonEmpty(artifact.Title, "Session evidence image"),
			"caption": artifactCaption(artifact),
		}
		if artifact.AssetID != "" {
			block["assetId"] = artifact.AssetID
			return block
		}
		if artifact.URL != "" {
			block["url"] = artifact.URL
			return block
		}
		return artifactMetadataBlock(blockID, artifact, "Image artifact is not available in Hub asset storage.")
	case "canvas":
		return map[string]any{
			"id":       blockID,
			"kind":     "markdown",
			"markdown": fmt.Sprintf("### %s\n\n%s\n", artifact.Title, core.FirstNonEmpty(artifact.Summary, "Linked canvas contains review evidence.")),
		}
	default:
		return artifactMetadataBlock(blockID, artifact, "Unsupported artifact type.")
	}
}

func manualNudgeBlock(sessionIndex int, session types.LocalSession, review types.SessionReview) map[string]any {
	if review.EvidenceStatus != "missing" || review.NudgePrompt == "" {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### Manual nudge\n\n")
	fmt.Fprintf(&b, "This session appears to have ended without reviewable artifacts.\n\n")
	if session.ResumeHint != "" {
		fmt.Fprintf(&b, "- Resume hint: `%s`\n", session.ResumeHint)
	}
	fmt.Fprintf(&b, "\n```text\n%s\n```\n", review.NudgePrompt)
	return map[string]any{
		"id":       fmt.Sprintf("session-%02d-manual-nudge", sessionIndex),
		"kind":     "markdown",
		"markdown": b.String(),
	}
}

func artifactMarkdown(artifact types.SessionArtifact) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n", artifact.Title)
	if artifact.Summary != "" {
		fmt.Fprintf(&b, "_%s_\n\n", artifact.Summary)
	}
	content := artifactText(artifact)
	if content == "" {
		content = "Artifact content could not be read."
	}
	b.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func artifactText(artifact types.SessionArtifact) string {
	if artifact.Path == "" {
		return core.FirstNonEmpty(artifact.Summary, artifact.URL)
	}
	file, err := os.Open(artifact.Path)
	if err != nil {
		return "Could not read artifact: " + err.Error()
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(types.TextArtifactCap)+1))
	if err != nil {
		return "Could not read artifact: " + err.Error()
	}
	text := string(data)
	if len(text) > types.TextArtifactCap {
		return text[:types.TextArtifactCap] + "\n\n... artifact truncated ..."
	}
	return text
}

func artifactCommand(artifact types.SessionArtifact) string {
	if artifact.Path == "" {
		return ""
	}
	return "cat " + displayArtifactPath(artifact.Path)
}

func artifactCaption(artifact types.SessionArtifact) string {
	parts := []string{}
	if artifact.Summary != "" {
		parts = append(parts, artifact.Summary)
	}
	if artifact.Path != "" {
		parts = append(parts, displayArtifactPath(artifact.Path))
	}
	return strings.Join(parts, " - ")
}

func displayArtifactPath(path string) string {
	if path == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~/" + strings.TrimPrefix(path, home+string(filepath.Separator))
	}
	return path
}

func artifactMetadataBlock(blockID string, artifact types.SessionArtifact, note string) map[string]any {
	return map[string]any{
		"id":    blockID,
		"kind":  "metadata",
		"title": artifact.Title,
		"metadata": map[string]any{
			"kind":        artifact.Kind,
			"source":      artifact.Source,
			"path":        artifact.Path,
			"url":         artifact.URL,
			"assetId":     artifact.AssetID,
			"contentType": artifact.ContentType,
			"size":        artifact.Size,
			"summary":     artifact.Summary,
			"note":        note,
		},
	}
}

func artifactTitles(artifacts []types.SessionArtifact) string {
	if len(artifacts) == 0 {
		return ""
	}
	titles := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		titles = append(titles, artifact.Kind+":"+artifact.Title)
	}
	return strings.Join(titles, "; ")
}
