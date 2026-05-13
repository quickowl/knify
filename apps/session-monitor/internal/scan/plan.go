package scan

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

const (
	planSummaryLimit = 220
	planItemLimit    = 8
)

var (
	proposedPlanPattern = regexp.MustCompile(`(?s)<proposed_plan>\s*(.*?)\s*</proposed_plan>`)
	headingPattern      = regexp.MustCompile(`(?m)^#\s+(.+)$`)
	bulletPattern       = regexp.MustCompile(`(?m)^\s*(?:[-*]|\d+[.)])\s+(?:\[( |x|X|-)\]\s*)?(.+)$`)
)

func extractClaudePlan(record map[string]any, recordTime time.Time) types.SessionPlan {
	result := core.MapValue(record["toolUseResult"])
	if result == nil {
		return types.SessionPlan{}
	}
	body := core.StringValue(result["plan"])
	if strings.TrimSpace(body) == "" {
		return types.SessionPlan{}
	}
	filePath := core.StringValue(result["filePath"])
	status := "observed"
	if boolValue(result["planWasEdited"]) {
		status = "edited"
	}
	return buildPlan("claude-plan", body, filePath, status, recordTime)
}

func extractCodexPlanFromMessage(text string, recordTime time.Time) types.SessionPlan {
	text = strings.TrimSpace(text)
	if text == "" {
		return types.SessionPlan{}
	}
	matches := proposedPlanPattern.FindStringSubmatch(text)
	if len(matches) == 2 {
		return buildPlan("codex-proposed-plan", matches[1], "", "proposed", recordTime)
	}
	if looksLikePlan(text) {
		return buildPlan("codex-plan-text", text, "", "observed", recordTime)
	}
	return types.SessionPlan{}
}

func extractCodexPlanFromFunctionCall(payload map[string]any, recordTime time.Time) types.SessionPlan {
	name := core.StringValue(payload["name"])
	if !strings.Contains(strings.ToLower(name), "update_plan") {
		return types.SessionPlan{}
	}
	var args struct {
		Explanation string `json:"explanation"`
		Plan        []struct {
			Step   string `json:"step"`
			Status string `json:"status"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(core.StringValue(payload["arguments"])), &args); err != nil || len(args.Plan) == 0 {
		return types.SessionPlan{}
	}
	items := make([]types.PlanItem, 0, len(args.Plan))
	keyParts := make([]string, 0, len(args.Plan))
	done := 0
	inProgress := 0
	for _, item := range args.Plan {
		step := strings.Join(strings.Fields(item.Step), " ")
		if step == "" {
			continue
		}
		status := strings.Join(strings.Fields(item.Status), " ")
		items = append(items, types.PlanItem{Text: core.Truncate(step, 180), Status: status})
		keyParts = append(keyParts, strings.ToLower(step))
		switch status {
		case "completed":
			done++
		case "in_progress":
			inProgress++
		}
	}
	if len(items) == 0 {
		return types.SessionPlan{}
	}
	title := core.FirstNonEmpty(args.Explanation, items[0].Text)
	status := "planned"
	if inProgress > 0 {
		status = "active"
	} else if done == len(items) {
		status = "completed"
	}
	return types.SessionPlan{
		Key:       "codex-update-plan:" + shortHash(strings.Join(keyParts, "\n")),
		Title:     core.Truncate(title, 90),
		Source:    "codex-update-plan",
		Status:    status,
		UpdatedAt: core.FormatTime(recordTime),
		Summary:   planProgressSummary(done, inProgress, len(items)),
		Items:     capPlanItems(items),
	}
}

func mergeSessionPlan(current, candidate types.SessionPlan) types.SessionPlan {
	if candidate.Key == "" {
		return current
	}
	if current.Key == "" {
		return candidate
	}
	currentTime := core.ParseTime(current.UpdatedAt)
	candidateTime := core.ParseTime(candidate.UpdatedAt)
	if candidateTime.After(currentTime) {
		return candidate
	}
	if current.Title == "" && candidate.Title != "" {
		current.Title = candidate.Title
	}
	if current.Summary == "" && candidate.Summary != "" {
		current.Summary = candidate.Summary
	}
	if current.FilePath == "" && candidate.FilePath != "" {
		current.FilePath = candidate.FilePath
	}
	if len(current.Items) == 0 && len(candidate.Items) > 0 {
		current.Items = candidate.Items
	}
	return current
}

func buildPlan(source, body, filePath, status string, updatedAt time.Time) types.SessionPlan {
	body = strings.TrimSpace(body)
	title := planTitle(body, filePath)
	items := extractPlanItems(body)
	return types.SessionPlan{
		Key:       planKey(source, title, body, filePath),
		Title:     core.Truncate(title, 90),
		Source:    source,
		FilePath:  filePath,
		Status:    status,
		UpdatedAt: core.FormatTime(updatedAt),
		Summary:   planSummary(body, title),
		Items:     capPlanItems(items),
	}
}

func looksLikePlan(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "## test plan") ||
		strings.Contains(lower, "## implementation") ||
		strings.Contains(lower, "## key changes") ||
		strings.Contains(lower, "### test plan")
}

func planTitle(body, filePath string) string {
	if matches := headingPattern.FindStringSubmatch(body); len(matches) == 2 {
		return strings.Join(strings.Fields(strings.TrimSpace(matches[1])), " ")
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "#"))
		if line != "" {
			return strings.Join(strings.Fields(line), " ")
		}
	}
	if filePath != "" {
		base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
		return strings.ReplaceAll(base, "-", " ")
	}
	return "Untitled plan"
}

func planSummary(body, title string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || line == title {
			continue
		}
		return core.Truncate(strings.Join(strings.Fields(line), " "), planSummaryLimit)
	}
	return ""
}

func extractPlanItems(body string) []types.PlanItem {
	var items []types.PlanItem
	for _, match := range bulletPattern.FindAllStringSubmatch(body, -1) {
		text := strings.Join(strings.Fields(strings.TrimSpace(match[2])), " ")
		if text == "" || strings.HasPrefix(text, "Purpose:") || strings.HasPrefix(text, "Current state:") {
			continue
		}
		status := ""
		switch strings.ToLower(match[1]) {
		case "x":
			status = "completed"
		case "-":
			status = "in_progress"
		case " ":
			status = "pending"
		}
		items = append(items, types.PlanItem{Text: core.Truncate(text, 180), Status: status})
		if len(items) >= planItemLimit {
			break
		}
	}
	return items
}

func capPlanItems(items []types.PlanItem) []types.PlanItem {
	if len(items) <= planItemLimit {
		return items
	}
	return append([]types.PlanItem(nil), items[:planItemLimit]...)
}

func planKey(source, title, body, filePath string) string {
	if filePath != "" {
		return source + ":" + filePath
	}
	return source + ":" + shortHash(strings.ToLower(title)+"\n"+body)
}

func shortHash(value string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(value))
	return fmt.Sprintf("%012x", hash.Sum64())
}

func planProgressSummary(done, inProgress, total int) string {
	pending := total - done - inProgress
	return fmt.Sprintf("%d done, %d active, %d pending", done, inProgress, pending)
}

func boolValue(value any) bool {
	switch got := value.(type) {
	case bool:
		return got
	case string:
		return got == "true"
	default:
		return false
	}
}
