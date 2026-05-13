package scan

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

type codexIndexEntry struct {
	ThreadName string
	UpdatedAt  time.Time
}

var codexIDPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func scanCodex(ctx context.Context, home string, cutoff time.Time) ([]types.LocalSession, types.ProviderHealth, []string) {
	health := types.ProviderHealth{Provider: "codex", Status: "ok", Details: map[string]string{"home": home}}
	var errorsOut []string
	info, err := os.Stat(home)
	if err != nil || !info.IsDir() {
		health.Status = "missing"
		if err != nil {
			health.Warnings = append(health.Warnings, err.Error())
		}
		return nil, health, nil
	}
	index := readCodexIndex(filepath.Join(home, "session_index.jsonl"))
	logTimes, logWarning := readCodexLogTimes(ctx, filepath.Join(home, "logs_2.sqlite"))
	if logWarning != "" {
		health.Warnings = append(health.Warnings, logWarning)
	}
	sessionRoot := filepath.Join(home, "sessions")
	var sessions []types.LocalSession
	walkErr := filepath.WalkDir(sessionRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errorsOut = append(errorsOut, fmt.Sprintf("codex walk %s: %v", path, err))
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			errorsOut = append(errorsOut, fmt.Sprintf("codex stat %s: %v", path, statErr))
			return nil
		}
		session, parseErr := parseCodexSessionFile(path, info.ModTime().UTC(), index, logTimes)
		if parseErr != nil {
			errorsOut = append(errorsOut, parseErr.Error())
			return nil
		}
		if session.SessionID == "" {
			return nil
		}
		if core.TimeFromRFC3339(session.UpdatedAt).Before(cutoff) && info.ModTime().Before(cutoff) {
			return nil
		}
		sessions = append(sessions, session)
		return nil
	})
	if walkErr != nil {
		errorsOut = append(errorsOut, walkErr.Error())
	}
	rawFiles := len(sessions)
	sessions = collapseSessionsByProviderID(sessions)
	health.Details["sessions"] = strconv.Itoa(len(sessions))
	if rawFiles != len(sessions) {
		health.Details["rawFiles"] = strconv.Itoa(rawFiles)
		health.Details["collapsedFiles"] = strconv.Itoa(rawFiles - len(sessions))
	}
	return sessions, health, errorsOut
}

func readCodexIndex(path string) map[string]codexIndexEntry {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	index := map[string]codexIndexEntry{}
	scanner := newJSONLScanner(file)
	for scanner.Scan() {
		var entry struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
			UpdatedAt  string `json:"updated_at"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || entry.ID == "" {
			continue
		}
		index[entry.ID] = codexIndexEntry{ThreadName: entry.ThreadName, UpdatedAt: core.ParseTime(entry.UpdatedAt)}
	}
	return index
}

func readCodexLogTimes(ctx context.Context, path string) (map[string]time.Time, string) {
	if _, err := os.Stat(path); err != nil {
		return nil, ""
	}
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		return nil, "sqlite3 not found; Codex logs_2.sqlite enrichment skipped"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	uri := "file:" + path + "?mode=ro"
	cmd := exec.CommandContext(ctx, sqlite, uri, "select thread_id, max(ts) from logs where thread_id is not null group by thread_id;")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Sprintf("Codex logs_2.sqlite enrichment skipped: %v", err)
	}
	times := map[string]time.Time{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			continue
		}
		sec, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		times[parts[0]] = time.Unix(sec, 0).UTC()
	}
	return times, ""
}

func parseCodexSessionFile(path string, modTime time.Time, index map[string]codexIndexEntry, logTimes map[string]time.Time) (types.LocalSession, error) {
	file, err := os.Open(path)
	if err != nil {
		return types.LocalSession{}, fmt.Errorf("codex open %s: %w", path, err)
	}
	defer file.Close()
	session := types.LocalSession{
		Provider:   "codex",
		SourcePath: path,
		Status:     "recorded",
		Metadata:   map[string]string{},
	}
	var created, updated time.Time
	var messages []types.MessageSummary
	scanner := newJSONLScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		recordTime := core.ParseRecordTime(record)
		created = core.Earliest(created, recordTime)
		updated = core.Latest(updated, recordTime)
		recordType := core.StringValue(record["type"])
		payload := core.MapValue(record["payload"])
		switch recordType {
		case "session_meta":
			session.SessionID = core.FirstNonEmpty(core.StringValue(payload["id"]), session.SessionID)
			session.CWD = core.FirstNonEmpty(core.StringValue(payload["cwd"]), session.CWD)
			if source := core.StringValue(payload["source"]); source != "" {
				session.Metadata["source"] = source
			}
			if modelProvider := core.StringValue(payload["model_provider"]); modelProvider != "" {
				session.Metadata["modelProvider"] = modelProvider
			}
			if git := core.MapValue(payload["git"]); git != nil {
				if branch := core.StringValue(git["branch"]); branch != "" {
					session.Metadata["gitBranch"] = branch
				}
			}
		case "event_msg":
			if title := core.StringValue(payload["thread_name"]); title != "" {
				session.Title = title
			}
			if threadID := core.StringValue(payload["thread_id"]); threadID != "" {
				session.SessionID = core.FirstNonEmpty(session.SessionID, threadID)
			}
		case "response_item":
			if core.StringValue(payload["type"]) == "function_call" {
				if strings.Contains(strings.ToLower(core.StringValue(payload["name"])), "update_plan") {
					session.Plan = mergeSessionPlan(session.Plan, extractCodexPlanFromFunctionCall(payload, recordTime))
				}
			}
			role := core.StringValue(payload["role"])
			if role == "user" || role == "assistant" {
				rawText := core.ExtractText(payload["content"])
				if role == "assistant" {
					session.Plan = mergeSessionPlan(session.Plan, extractCodexPlanFromMessage(rawText, recordTime))
					session.Artifacts = mergeArtifacts(session.Artifacts, extractArtifactsFromText(rawText, session.CWD, "assistant"))
				}
				text := core.SummarizeText(rawText, MessageTextLimit)
				if text != "" {
					messages = append(messages, types.MessageSummary{Role: role, Text: text, Timestamp: core.FormatTime(recordTime)})
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return types.LocalSession{}, fmt.Errorf("codex scan %s: %w", path, err)
	}
	if session.SessionID == "" {
		session.SessionID = codexIDFromPath(path)
	}
	if entry, ok := index[session.SessionID]; ok {
		session.Title = core.FirstNonEmpty(session.Title, entry.ThreadName)
		updated = core.Latest(updated, entry.UpdatedAt)
	}
	if logTime, ok := logTimes[session.SessionID]; ok {
		updated = core.Latest(updated, logTime)
	}
	if updated.IsZero() {
		updated = modTime
	}
	if created.IsZero() {
		created = updated
	}
	session.CreatedAt = core.FormatTime(created)
	session.UpdatedAt = core.FormatTime(updated)
	if session.Title == "" {
		session.Title = "Codex " + core.ShortID(session.SessionID)
	}
	session.LatestMessages = core.LastMessages(messages, messagesPerSession)
	if session.SessionID != "" {
		session.ResumeHint = "codex exec resume " + session.SessionID + " <prompt>"
	}
	return session, nil
}

func codexIDFromPath(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	matches := codexIDPattern.FindAllString(base, -1)
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	if idx := strings.LastIndex(base, "-"); idx >= 0 && idx+1 < len(base) {
		return base[idx+1:]
	}
	return base
}

func countJSONLLines(path string) (int, string) {
	file, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer file.Close()
	count := 0
	scanner := newJSONLScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return count, err.Error()
	}
	return count, ""
}

func newJSONLScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), types.MaxJSONLTokenBytes)
	return scanner
}
