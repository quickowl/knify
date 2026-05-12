package types

import "time"

const (
	DefaultCanvasID     = "canvas.session-monitor.local"
	DefaultRunID        = "run.session-monitor.local"
	DefaultAgentID      = "session-monitor"
	DefaultLookback     = 24 * time.Hour
	DefaultMaxSessions  = 30
	MaxJSONLTokenBytes  = 128 * 1024 * 1024
	HubRequestTimeout   = 15 * time.Second
	LikelyMatchWindow   = 48 * time.Hour
	SessionMonitorTitle = "Local session monitor"
	DefaultStatusPath   = "/tmp/session-monitor-live-watch.json"
	DefaultStaleAfter   = 3 * time.Minute
)

type Config struct {
	HubURL      string
	Token       string
	CanvasID    string
	RunID       string
	CodexHome   string
	ClaudeHome  string
	OutPath     string
	DryRun      bool
	Dynamic     bool
	Watch       bool
	Status      bool
	Interval    time.Duration
	StaleAfter  time.Duration
	RecentLimit int
	Lookback    time.Duration
	MaxSessions int
	Now         func() time.Time
}

type ScanResult struct {
	GeneratedAt    string             `json:"generatedAt"`
	Process        ProcessInfo        `json:"process,omitempty"`
	Lookback       string             `json:"lookback"`
	MaxSessions    int                `json:"maxSessions"`
	Sessions       []LocalSession     `json:"sessions"`
	ProviderHealth []ProviderHealth   `json:"providerHealth"`
	Errors         []string           `json:"errors,omitempty"`
	Hub            HubSummary         `json:"hub"`
	Canvas         Canvas             `json:"canvas"`
	Posted         bool               `json:"posted"`
	Dynamic        bool               `json:"dynamic"`
	EventsPosted   int                `json:"eventsPosted,omitempty"`
	PostResponse   map[string]any     `json:"postResponse,omitempty"`
	OpenAIDocs     OpenAIDocsEvidence `json:"openaiDocs"`
}

type ProcessInfo struct {
	PID      int    `json:"pid,omitempty"`
	Mode     string `json:"mode,omitempty"`
	Watch    bool   `json:"watch"`
	Dynamic  bool   `json:"dynamic"`
	Interval string `json:"interval,omitempty"`
	HubURL   string `json:"hubUrl,omitempty"`
	CanvasID string `json:"canvasId,omitempty"`
	RunID    string `json:"runId,omitempty"`
	OutPath  string `json:"outPath,omitempty"`
}

type OpenAIDocsEvidence struct {
	CodexExecResume string `json:"codexExecResume"`
	CodexAppServer  string `json:"codexAppServer"`
}

type ProviderHealth struct {
	Provider string            `json:"provider"`
	Status   string            `json:"status"`
	Details  map[string]string `json:"details,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
}

type HubSummary struct {
	URL         string `json:"url,omitempty"`
	CanvasCount int    `json:"canvasCount"`
	RunCount    int    `json:"runCount"`
}

type LocalSession struct {
	Provider       string            `json:"provider"`
	SessionID      string            `json:"sessionId"`
	Title          string            `json:"title,omitempty"`
	CWD            string            `json:"cwd,omitempty"`
	Status         string            `json:"status,omitempty"`
	SourcePath     string            `json:"sourcePath,omitempty"`
	CreatedAt      string            `json:"createdAt,omitempty"`
	UpdatedAt      string            `json:"updatedAt,omitempty"`
	LatestMessages []MessageSummary  `json:"latestMessages,omitempty"`
	ResumeHint     string            `json:"resumeHint,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Review         SessionReview     `json:"review,omitempty"`
	Match          MatchResult       `json:"match"`
}

type MessageSummary struct {
	Role      string `json:"role"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp,omitempty"`
}

type MatchResult struct {
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	CanvasID  string `json:"canvasId,omitempty"`
	RunID     string `json:"runId,omitempty"`
	Score     int    `json:"score,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
	Provider  string `json:"provider,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type SessionReview struct {
	Purpose      string   `json:"purpose,omitempty"`
	CurrentState string   `json:"currentState,omitempty"`
	NextStep     string   `json:"nextStep,omitempty"`
	Signals      []string `json:"signals,omitempty"`
}

type HubState struct {
	Canvases []HubCanvas
	Runs     []HubRun
}

type HubCanvas struct {
	ID        string `json:"id"`
	AgentID   string `json:"agentId"`
	RunID     string `json:"runId"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Status    string `json:"status"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt"`
}

type HubRun struct {
	ID             string            `json:"id"`
	Provider       string            `json:"provider"`
	AgentID        string            `json:"agentId"`
	RunID          string            `json:"runId"`
	CanvasID       string            `json:"canvasId"`
	Title          string            `json:"title"`
	Status         string            `json:"status"`
	ExternalID     string            `json:"externalId"`
	UpdatedAt      string            `json:"updatedAt"`
	FeedbackTarget FeedbackTarget    `json:"feedbackTarget"`
	Metadata       map[string]string `json:"metadata"`
}

type FeedbackTarget struct {
	Provider   string            `json:"provider"`
	Mode       string            `json:"mode"`
	ExternalID string            `json:"externalId"`
	CWD        string            `json:"cwd"`
	Metadata   map[string]string `json:"metadata"`
}

type Canvas struct {
	ID        string           `json:"id"`
	AgentID   string           `json:"agentId"`
	RunID     string           `json:"runId"`
	Title     string           `json:"title"`
	Summary   string           `json:"summary"`
	Status    string           `json:"status"`
	Mode      string           `json:"mode,omitempty"`
	Priority  string           `json:"priority"`
	CreatedAt string           `json:"createdAt"`
	UpdatedAt string           `json:"updatedAt"`
	Version   int              `json:"version"`
	Blocks    []map[string]any `json:"blocks"`
}

type CanvasLogEvent struct {
	ID                 string         `json:"id"`
	CanvasID           string         `json:"canvasId"`
	Type               string         `json:"type"`
	ExpectedVersion    *int           `json:"expectedVersion,omitempty"`
	AgentID            string         `json:"agentId,omitempty"`
	RunID              string         `json:"runId,omitempty"`
	Title              string         `json:"title,omitempty"`
	Summary            string         `json:"summary,omitempty"`
	Status             string         `json:"status,omitempty"`
	Priority           string         `json:"priority,omitempty"`
	Block              map[string]any `json:"block,omitempty"`
	BlockID            string         `json:"blockId,omitempty"`
	InsertAfterBlockID string         `json:"insertAfterBlockId,omitempty"`
	CreatedAt          string         `json:"createdAt"`
}
