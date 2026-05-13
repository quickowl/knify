export type CanvasKind = "metric" | "tasks" | "incident" | "chart" | "design";

export type CanvasExample = {
  id: string;
  ticket: string;
  title: string;
  description: string;
  agent: string;
  version: number;
  live: boolean;
  viewers: number;
  kind: CanvasKind;
  private: boolean;
};

export type CanvasCopy = {
  headline: string;
  seeing: string;
  checks: string[];
};

export type CanvasStatus =
  | "draft"
  | "queued"
  | "in_progress"
  | "running"
  | "ready_for_review"
  | "needs_review"
  | "needs_changes"
  | "accepted"
  | "completed"
  | "archived"
  | "failed"
  | string;

export type CanvasPriority = "low" | "normal" | "high" | "urgent" | string;
export type CanvasMode = "static" | "dynamic";
export type CanvasReviewState = "pending" | "approved" | "rejected" | "skipped";
export type FeedbackDecision = "accepted" | "needs_changes" | "comment_only";

export interface CanvasBlock {
  id: string;
  kind: string;
  payload: Record<string, unknown>;
  raw: Record<string, unknown>;
}

export interface AgentCanvas {
  id: string;
  workspaceID?: string;
  title: string;
  summary: string;
  agentID: string;
  agentName: string;
  runID: string;
  status: CanvasStatus;
  mode?: CanvasMode;
  priority: CanvasPriority;
  reviewState: CanvasReviewState;
  createdAt: string;
  updatedAt: string;
  updatedAtMs: number;
  version: number;
  lastEventId?: string;
  tags: string[];
  blocks: CanvasBlock[];
}

export interface FeedbackTargetAnchor {
  blockId: string;
  blockKind?: string;
  label?: string;
  x: number;
  y: number;
}

export interface FeedbackRequest {
  id: string;
  canvasId: string;
  decision: FeedbackDecision;
  text?: string;
  targetBlockIds?: string[];
  targetAnchors?: FeedbackTargetAnchor[];
  createdAt: string;
}

export interface FeedbackDeliverySummary {
  id: string;
  feedbackId: string;
  canvasId: string;
  status: string;
  updatedAt?: string;
  createdAt?: string;
}

export interface CanvasSnapshot {
  id: string;
  canvasId: string;
  version: number;
  reason: string;
  source?: string;
  label?: string;
  createdAt: string;
  canvas?: AgentCanvas;
}

export interface CanvasEdit {
  id: string;
  canvasId: string;
  expectedVersion: number;
  ops: unknown[];
  submittedBy?: string;
  note?: string;
  createdAt: string;
}

export interface HubEvent {
  id?: string;
  type: string;
  canvasId?: string;
  feedbackId?: string;
  data?: unknown;
  createdAt?: string;
}
