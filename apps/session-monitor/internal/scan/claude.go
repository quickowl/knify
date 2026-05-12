package scan

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

type claudeActiveSession struct {
	SessionID string
	CWD       string
	Status    string
	UpdatedAt time.Time
	PID       string
}

func scanClaude(home string, cutoff time.Time) ([]types.LocalSession, types.ProviderHealth, []string) {
	health := types.ProviderHealth{Provider: "claude", Status: "ok", Details: map[string]string{"home": home}}
	var errorsOut []string
	info, err := os.Stat(home)
	if err != nil || !info.IsDir() {
		health.Status = "missing"
		if err != nil {
			health.Warnings = append(health.Warnings, err.Error())
		}
		return nil, health, nil
	}
	active := readClaudeActiveSessions(filepath.Join(home, "sessions"))
	var sessions []types.LocalSession
	projectRoot := filepath.Join(home, "projects")
	walkErr := filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errorsOut = append(errorsOut, fmt.Sprintf("claude walk %s: %v", path, err))
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			errorsOut = append(errorsOut, fmt.Sprintf("claude stat %s: %v", path, statErr))
			return nil
		}
		session, parseErr := parseClaudeProjectFile(path, info.ModTime().UTC(), active)
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
	if historyCount, historyWarning := countJSONLLines(filepath.Join(home, "history.jsonl")); historyWarning != "" {
		health.Warnings = append(health.Warnings, historyWarning)
	} else {
		health.Details["historyEntries"] = strconv.Itoa(historyCount)
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

func readClaudeActiveSessions(root string) map[string]claudeActiveSession {
	out := map[string]claudeActiveSession{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		id := core.StringValue(raw["sessionId"])
		if id == "" {
			continue
		}
		active := claudeActiveSession{
			SessionID: id,
			CWD:       core.StringValue(raw["cwd"]),
			Status:    core.StringValue(raw["status"]),
			UpdatedAt: core.TimeFromMillis(raw["updatedAt"]),
			PID:       core.NumberString(raw["pid"]),
		}
		out[id] = active
	}
	return out
}

func parseClaudeProjectFile(path string, modTime time.Time, active map[string]claudeActiveSession) (types.LocalSession, error) {
	file, err := os.Open(path)
	if err != nil {
		return types.LocalSession{}, fmt.Errorf("claude open %s: %w", path, err)
	}
	defer file.Close()
	session := types.LocalSession{
		Provider:   "claude",
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
		recordType := core.StringValue(record["type"])
		recordTime := core.ParseRecordTime(record)
		created = core.Earliest(created, recordTime)
		updated = core.Latest(updated, recordTime)
		session.SessionID = core.FirstNonEmpty(session.SessionID, core.StringValue(record["sessionId"]))
		session.CWD = core.FirstNonEmpty(session.CWD, core.StringValue(record["cwd"]))
		if branch := core.StringValue(record["gitBranch"]); branch != "" {
			session.Metadata["gitBranch"] = branch
		}
		if version := core.StringValue(record["version"]); version != "" {
			session.Metadata["version"] = version
		}
		if recordType == "ai-title" {
			session.Title = core.FirstNonEmpty(session.Title, core.StringValue(record["aiTitle"]))
			continue
		}
		if recordType == "permission-mode" {
			if mode := core.StringValue(record["permissionMode"]); mode != "" {
				session.Metadata["permissionMode"] = mode
			}
			continue
		}
		if recordType == "user" || recordType == "assistant" {
			message := core.MapValue(record["message"])
			role := core.FirstNonEmpty(core.StringValue(message["role"]), recordType)
			text := core.SummarizeText(core.ExtractText(message["content"]), MessageTextLimit)
			if text != "" {
				messages = append(messages, types.MessageSummary{Role: role, Text: text, Timestamp: core.FormatTime(recordTime)})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return types.LocalSession{}, fmt.Errorf("claude scan %s: %w", path, err)
	}
	if session.SessionID == "" {
		session.SessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if got, ok := active[session.SessionID]; ok {
		session.CWD = core.FirstNonEmpty(session.CWD, got.CWD)
		session.Status = core.FirstNonEmpty(got.Status, session.Status)
		updated = core.Latest(updated, got.UpdatedAt)
		if got.PID != "" {
			session.Metadata["pid"] = got.PID
		}
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
		session.Title = "Claude " + core.ShortID(session.SessionID)
	}
	session.LatestMessages = core.LastMessages(messages, messagesPerSession)
	if session.SessionID != "" {
		session.ResumeHint = "claude -p --resume " + session.SessionID + " <prompt>"
	}
	return session, nil
}
