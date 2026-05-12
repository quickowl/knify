package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/core"
	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func postCanvasDynamic(ctx context.Context, cfg types.Config, canvas types.Canvas) (map[string]any, int, error) {
	now := core.ParseTime(canvas.UpdatedAt)
	if now.IsZero() {
		now = cfg.Now().UTC()
	}
	existing, exists, err := FetchHubCanvas(ctx, cfg, canvas.ID)
	if err != nil {
		return nil, 0, err
	}
	currentVersion := existing.Version
	existingBlocks := mapBlocksByID(existing.Blocks)
	eventsPosted := 0
	var response map[string]any

	if !exists {
		status := canvas.Status
		if cfg.Watch {
			status = "in_progress"
		}
		event := types.CanvasLogEvent{
			ID:        dynamicEventID(canvas.ID, "start", now, eventsPosted+1),
			CanvasID:  canvas.ID,
			Type:      "canvas.started",
			AgentID:   canvas.AgentID,
			RunID:     canvas.RunID,
			Title:     canvas.Title,
			Summary:   canvas.Summary,
			Status:    status,
			Priority:  canvas.Priority,
			CreatedAt: core.FormatTime(now),
		}
		response, err = postCanvasEvent(ctx, cfg, event)
		if err != nil {
			return response, eventsPosted, err
		}
		eventsPosted++
		currentVersion = responseVersion(response, currentVersion+1)
	} else {
		status := canvas.Status
		if cfg.Watch {
			status = "in_progress"
		}
		event := types.CanvasLogEvent{
			ID:              dynamicEventID(canvas.ID, "summary", now, eventsPosted+1),
			CanvasID:        canvas.ID,
			Type:            "canvas.summary.updated",
			ExpectedVersion: &currentVersion,
			Title:           canvas.Title,
			Summary:         canvas.Summary,
			Status:          status,
			Priority:        canvas.Priority,
			CreatedAt:       core.FormatTime(now),
		}
		response, err = postCanvasEvent(ctx, cfg, event)
		if err != nil {
			return response, eventsPosted, err
		}
		eventsPosted++
		currentVersion = responseVersion(response, currentVersion+1)
	}

	desiredIDs := map[string]bool{}
	previousBlockID := ""
	for _, block := range canvas.Blocks {
		blockID := core.StringValue(block["id"])
		if blockID == "" {
			continue
		}
		desiredIDs[blockID] = true
		existingBlock, hadBlock := existingBlocks[blockID]
		if hadBlock && blocksEqual(existingBlock, block) {
			previousBlockID = blockID
			continue
		}
		event := types.CanvasLogEvent{
			ID:              dynamicEventID(canvas.ID, "block", now, eventsPosted+1),
			CanvasID:        canvas.ID,
			ExpectedVersion: &currentVersion,
			Block:           cloneBlock(block),
			CreatedAt:       core.FormatTime(now),
		}
		if hadBlock {
			event.Type = "canvas.block.replaced"
			event.BlockID = blockID
		} else {
			event.Type = "canvas.block.appended"
			event.InsertAfterBlockID = previousBlockID
		}
		response, err = postCanvasEvent(ctx, cfg, event)
		if err != nil {
			return response, eventsPosted, err
		}
		eventsPosted++
		currentVersion = responseVersion(response, currentVersion+1)
		previousBlockID = blockID
	}

	for _, block := range existing.Blocks {
		blockID := core.StringValue(block["id"])
		if blockID == "" || desiredIDs[blockID] || !isSessionMonitorBlockID(blockID) {
			continue
		}
		event := types.CanvasLogEvent{
			ID:              dynamicEventID(canvas.ID, "remove", now, eventsPosted+1),
			CanvasID:        canvas.ID,
			Type:            "canvas.block.removed",
			ExpectedVersion: &currentVersion,
			BlockID:         blockID,
			CreatedAt:       core.FormatTime(now),
		}
		response, err = postCanvasEvent(ctx, cfg, event)
		if err != nil {
			return response, eventsPosted, err
		}
		eventsPosted++
		currentVersion = responseVersion(response, currentVersion+1)
	}

	if response == nil {
		response = map[string]any{"id": canvas.ID, "version": currentVersion}
	}
	return response, eventsPosted, nil
}

func postCanvasEvent(ctx context.Context, cfg types.Config, event types.CanvasLogEvent) (map[string]any, error) {
	body, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, types.HubRequestTimeout)
	defer cancel()
	url := strings.TrimRight(cfg.HubURL, "/") + "/v1/canvases/" + event.CanvasID + "/events"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST /v1/canvases/%s/events returned %d: %s", event.CanvasID, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var response map[string]any
	if len(data) > 0 {
		_ = json.Unmarshal(data, &response)
	}
	return response, nil
}

func mapBlocksByID(blocks []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, block := range blocks {
		if id := core.StringValue(block["id"]); id != "" {
			out[id] = block
		}
	}
	return out
}

func blocksEqual(left, right map[string]any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func cloneBlock(block map[string]any) map[string]any {
	raw, err := json.Marshal(block)
	if err != nil {
		return block
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return block
	}
	return cloned
}

func responseVersion(response map[string]any, fallback int) int {
	if response == nil {
		return fallback
	}
	switch value := response["version"].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
	}
	return fallback
}

func dynamicEventID(canvasID, kind string, now time.Time, sequence int) string {
	base := core.SanitizeID(canvasID)
	if base == "" {
		base = "session-monitor"
	}
	return fmt.Sprintf("%s-event-%s-%d-%03d", base, kind, now.UnixNano(), sequence)
}

func isSessionMonitorBlockID(id string) bool {
	switch id {
	case "session-monitor-heading", "session-monitor-overview", "session-monitor-recent-sessions", "session-monitor-session-list", "session-monitor-scan-metadata", "session-monitor-collection", "session-monitor-provider-health", "session-monitor-next-steps", "session-monitor-scan-warnings":
		return true
	default:
		return strings.HasPrefix(id, "session-")
	}
}
