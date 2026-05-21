package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

type AssetUploadResponse struct {
	AssetID     string `json:"assetId"`
	ID          string `json:"id"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
	CreatedAt   string `json:"createdAt"`
}

func FetchHubState(ctx context.Context, cfg types.Config) (types.HubState, error) {
	var state types.HubState
	var errs []string
	if err := hubGET(ctx, cfg, "/v1/canvases", &state.Canvases); err != nil {
		errs = append(errs, err.Error())
	}
	if err := hubGET(ctx, cfg, "/v1/agent-runs", &state.Runs); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return state, fmt.Errorf("hub fetch: %s", strings.Join(errs, "; "))
	}
	return state, nil
}

func hubGET(ctx context.Context, cfg types.Config, path string, target any) error {
	url := strings.TrimRight(cfg.HubURL, "/") + path
	ctx, cancel := context.WithTimeout(ctx, types.HubRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("GET %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func PublishCanvas(ctx context.Context, cfg types.Config, canvas types.Canvas) (map[string]any, int, error) {
	if cfg.Dynamic {
		response, eventsPosted, err := postCanvasDynamic(ctx, cfg, canvas)
		return response, eventsPosted, err
	}
	response, err := postCanvas(ctx, cfg, canvas)
	if err != nil {
		return nil, 0, err
	}
	return response, 0, nil
}

func postCanvas(ctx context.Context, cfg types.Config, canvas types.Canvas) (map[string]any, error) {
	body, err := json.Marshal(canvas)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, types.HubRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.HubURL, "/")+"/v1/canvases", bytes.NewReader(body))
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
		if legacyCollectionItemError(data) {
			return postCanvasLegacy(ctx, cfg, canvas)
		}
		return nil, fmt.Errorf("POST /v1/canvases returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var response map[string]any
	if len(data) > 0 {
		_ = json.Unmarshal(data, &response)
	}
	return response, nil
}

func postCanvasLegacy(ctx context.Context, cfg types.Config, canvas types.Canvas) (map[string]any, error) {
	legacy := canvas
	legacy.Blocks = stripCollectionItemExtensions(canvas.Blocks)
	body, err := json.Marshal(legacy)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, types.HubRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.HubURL, "/")+"/v1/canvases", bytes.NewReader(body))
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
		return nil, fmt.Errorf("POST /v1/canvases returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var response map[string]any
	if len(data) > 0 {
		_ = json.Unmarshal(data, &response)
	}
	return response, nil
}

func FetchHubCanvas(ctx context.Context, cfg types.Config, canvasID string) (types.Canvas, bool, error) {
	url := strings.TrimRight(cfg.HubURL, "/") + "/v1/canvases/" + canvasID
	ctx, cancel := context.WithTimeout(ctx, types.HubRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.Canvas{}, false, err
	}
	req.Header.Set("Accept", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.Canvas{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return types.Canvas{}, false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return types.Canvas{}, false, fmt.Errorf("GET /v1/canvases/%s returned %d: %s", canvasID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var canvas types.Canvas
	if err := json.NewDecoder(resp.Body).Decode(&canvas); err != nil {
		return types.Canvas{}, false, err
	}
	return canvas, true, nil
}

func UploadAsset(ctx context.Context, cfg types.Config, contentType string, body io.Reader) (AssetUploadResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, types.HubRequestTimeout)
	defer cancel()
	url := strings.TrimRight(cfg.HubURL, "/") + "/v1/assets"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return AssetUploadResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", contentType)
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return AssetUploadResponse{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AssetUploadResponse{}, fmt.Errorf("POST /v1/assets returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var response AssetUploadResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return AssetUploadResponse{}, err
	}
	if response.AssetID == "" {
		response.AssetID = response.ID
	}
	if response.ID == "" {
		response.ID = response.AssetID
	}
	return response, nil
}
