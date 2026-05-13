package scan

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func collapseSessionsByProviderID(sessions []types.LocalSession) []types.LocalSession {
	byKey := map[string]int{}
	var out []types.LocalSession
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
		return core.TimeFromRFC3339(out[i].UpdatedAt).After(core.TimeFromRFC3339(out[j].UpdatedAt))
	})
	return out
}

func mergeSessionRows(left, right types.LocalSession) types.LocalSession {
	merged := left
	merged.CreatedAt = core.FormatTime(core.Earliest(core.TimeFromRFC3339(left.CreatedAt), core.TimeFromRFC3339(right.CreatedAt)))
	if core.TimeFromRFC3339(right.UpdatedAt).After(core.TimeFromRFC3339(left.UpdatedAt)) {
		merged.UpdatedAt = right.UpdatedAt
		merged.LatestMessages = right.LatestMessages
	}
	if preferSourcePath(right.SourcePath, merged.SourcePath) {
		merged.SourcePath = right.SourcePath
	}
	if preferTitle(right.Title, merged.Title) {
		merged.Title = right.Title
	}
	merged.CWD = core.FirstNonEmpty(merged.CWD, right.CWD)
	if statusRank(right.Status) > statusRank(merged.Status) {
		merged.Status = right.Status
	}
	merged.ResumeHint = core.FirstNonEmpty(merged.ResumeHint, right.ResumeHint)
	merged.Artifacts = mergeArtifacts(merged.Artifacts, right.Artifacts)
	merged.Metadata = mergeStringMaps(merged.Metadata, right.Metadata)
	merged.Plan = mergeSessionPlan(merged.Plan, right.Plan)
	merged = normalizeCollapseMetadata(
		merged,
		intMetadata(left.Metadata, "sourceFiles")+intMetadata(right.Metadata, "sourceFiles"),
		intMetadata(left.Metadata, "subagentFiles")+intMetadata(right.Metadata, "subagentFiles"),
	)
	return merged
}

func normalizeCollapseMetadata(session types.LocalSession, sourceFiles, subagentFiles int) types.LocalSession {
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
