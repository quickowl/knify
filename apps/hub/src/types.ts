export type CanvasStatus = "draft" | "in_progress" | "ready_for_review" | "accepted" | "needs_changes" | "archived";
export type CanvasMode = "static" | "dynamic";
export type Priority = "low" | "normal" | "high" | "urgent";
export type BlockKind =
  | "heading"
  | "markdown"
  | "image"
  | "terminal"
  | "diff"
  | "chart"
  | "checklist"
  | "decision"
  | "link"
  | "metadata"
  | "video"
  | "html"
  | "collection"
  | "form"
  | "orderable-list"
  | "split"
  | "rule-builder";

export type HtmlSandbox = "strict" | "relaxed";

export const HTML_BLOCK_MAX_BYTES = 256 * 1024;
export const HTML_BLOCK_MAX_HEIGHT = 1600;
export const HTML_BLOCK_DEFAULT_HEIGHT = 320;
export type FeedbackDecision = "accepted" | "needs_changes" | "comment_only";
export type DeliveryStatus = "pending" | "queued" | "retrying" | "delivered" | "failed" | "dead_lettered";
export type AgentProvider = "webhook" | "generic_cloud" | "cursor" | "cursor_cli" | "codex" | "codex_exec" | "claude" | "claude_cli";
export type AgentRunStatus = "registered" | "running" | "waiting_for_feedback" | "completed" | "failed" | "expired";
export type CanvasLogEventType =
  | "canvas.started"
  | "canvas.summary.updated"
  | "canvas.block.appended"
  | "canvas.block.replaced"
  | "canvas.block.removed"
  | "canvas.completed";

export interface Canvas {
  id: string;
  workspaceId?: string;
  agentId: string;
  runId: string;
  title: string;
  summary: string;
  status: CanvasStatus;
  mode?: CanvasMode;
  priority: Priority;
  createdAt: string;
  updatedAt: string;
  version: number;
  lastEventId?: string;
  callback?: Callback;
  blocks: Block[];
}

export interface Callback {
  webhook?: WebhookCallback;
  feedbackTargetId?: string;
}

export interface WebhookCallback {
  url: string;
  headers?: Record<string, string>;
}

export interface FeedbackTarget {
  id?: string;
  provider: AgentProvider;
  mode?: string;
  externalId?: string;
  authRef?: string;
  url?: string;
  headers?: Record<string, string>;
  command?: string[];
  cwd?: string;
  metadata?: Record<string, string>;
}

export interface AgentRun {
  id: string;
  workspaceId?: string;
  provider: AgentProvider;
  agentId: string;
  runId: string;
  canvasId?: string;
  title?: string;
  mode?: string;
  status: AgentRunStatus;
  externalId?: string;
  authRef?: string;
  webUrl?: string;
  traceUrl?: string;
  lastEventId?: string;
  error?: string;
  feedbackTarget: FeedbackTarget;
  metadata?: Record<string, string>;
  createdAt: string;
  updatedAt: string;
}

export interface AgentEvent {
  id: string;
  workspaceId?: string;
  provider: AgentProvider;
  agentRunId?: string;
  externalId?: string;
  type: string;
  status?: AgentRunStatus;
  sequence?: number;
  payload?: Record<string, unknown>;
  traceUrl?: string;
  occurredAt: string;
  createdAt: string;
}

export interface CanvasLogEvent {
  id: string;
  workspaceId?: string;
  canvasId: string;
  type: CanvasLogEventType;
  expectedVersion?: number;
  version?: number;
  agentId?: string;
  runId?: string;
  title?: string;
  summary?: string;
  status?: CanvasStatus;
  priority?: Priority;
  block?: Block;
  blockId?: string;
  insertAfterBlockId?: string;
  createdAt: string;
}

export interface CanvasSnapshot {
  id: string;
  canvasId: string;
  version: number;
  reason: string;
  source?: string;
  label?: string;
  sourceEventId?: string;
  sourceEditId?: string;
  createdAt: string;
  canvas: Canvas;
}

export interface CanvasRestoreResponse {
  canvas: Canvas;
  snapshot: CanvasSnapshot;
  checkpoint: CanvasSnapshot;
}

export interface CreateCanvasSnapshotRequest {
  reason?: string;
  source?: string;
  label?: string;
  sourceEventId?: string;
  sourceEditId?: string;
}

export interface CanvasBundle {
  schemaVersion: string;
  exportedAt: string;
  canvas: Canvas;
  snapshots?: CanvasSnapshot[];
  events?: CanvasLogEvent[];
  feedback?: Feedback[];
  edits?: CanvasEdit[];
  assets?: CanvasBundleAsset[];
  agentRuns?: AgentRun[];
}

export interface CanvasBundleAsset {
  asset: Asset;
  bodyBase64: string;
}

export interface CanvasImportRequest {
  bundle: CanvasBundle;
  conflictPolicy?: "fail" | "replace";
}

export interface CanvasImportResult {
  canvas: Canvas;
  conflictPolicy: string;
  checkpoint?: CanvasSnapshot;
  importedSnapshots: number;
  importedEvents: number;
  importedFeedback: number;
  importedEdits: number;
  importedAssets: number;
  importedAgentRuns: number;
}

export interface Asset {
  id: string;
  workspaceId?: string;
  contentType: string;
  size: number;
  createdAt: string;
}

export interface Feedback {
  id: string;
  workspaceId?: string;
  canvasId: string;
  decision: FeedbackDecision;
  text?: string;
  voiceTranscript?: string;
  inkAssetId?: string;
  targetBlockIds?: string[];
  targetAnchors?: FeedbackAnchor[];
  createdAt: string;
  deliveryStatus: DeliveryStatus;
}

export interface FeedbackAnchor {
  blockId: string;
  blockKind?: string;
  label?: string;
  x: number;
  y: number;
}

export interface FeedbackDelivery {
  id: string;
  workspaceId?: string;
  feedbackId: string;
  canvasId: string;
  agentRunId?: string;
  targetId?: string;
  provider: AgentProvider;
  mode?: string;
  status: DeliveryStatus;
  target: FeedbackTarget;
  attempts: number;
  maxAttempts: number;
  nextAttemptAt?: string;
  leaseOwner?: string;
  leaseUntil?: string;
  lastError?: string;
  receipt?: DeliveryReceipt;
  createdAt: string;
  updatedAt: string;
}

export interface DeliveryReceipt {
  providerMessageId?: string;
  url?: string;
  summary?: string;
  metadata?: Record<string, string>;
  deliveredAt: string;
}

export interface Block {
  id: string;
  kind: BlockKind;
  text?: string;
  level?: number;
  markdown?: string;
  assetId?: string;
  url?: string;
  alt?: string;
  caption?: string;
  command?: string;
  output?: string;
  exitCode?: number | null;
  language?: string;
  diff?: string;
  chart?: ChartSpec;
  items?: ChecklistItem[];
  prompt?: string;
  options?: string[];
  title?: string;
  metadata?: Record<string, unknown>;
  sourceUrl?: string;
  platform?: string;
  thumbnailUrl?: string;
  thumbnailAssetId?: string;
  status?: string;
  duration?: string;
  authorName?: string;
  addedAt?: string;
  posterAlt?: string;
  mode?: string;
  pageSize?: number;
  fields?: FormField[];
  orderableItems?: OrderableItem[];
  itemEditor?: FormSchema;
  total?: number;
  slices?: SplitSlice[];
  ruleSchema?: RuleSchema;
  clauses?: RuleClause[];
  html?: string;
  sandbox?: HtmlSandbox;
  height?: number;
  screenshotAssetId?: string;
  screenshotUrl?: string;
}

export interface ChartSpec {
  version: string;
  type: string;
  title?: string;
  xAxisLabel?: string;
  yAxisLabel?: string;
  series: ChartSeries[];
}

export interface ChartSeries {
  name: string;
  color?: string;
  data: ChartPoint[];
}

export interface ChartPoint {
  label: string;
  value: number;
}

export interface ChecklistItem {
  id: string;
  text?: string;
  checked?: boolean;
  sessionId?: string;
  label?: string;
  subtitle?: string;
  thumbnailUrl?: string;
  status?: string;
  attention?: string;
  updatedAt?: string;
  purpose?: string;
  currentState?: string;
  evidenceStatus?: string;
  nextStep?: string;
  artifactCount?: number;
  planLabel?: string;
  badges?: string[];
  addedAt?: string;
  blockIds?: string[];
}

export interface FormField {
  name: string;
  type: "text" | "number" | "select" | "multiSelect" | "toggle";
  label?: string;
  options?: string[];
  value?: unknown;
  required?: boolean;
}

export interface FormSchema {
  fields: FormField[];
}

export interface OrderableItem {
  id: string;
  label: string;
  included?: boolean;
  meta?: Record<string, unknown>;
}

export interface SplitSlice {
  id: string;
  label?: string;
  weight: number;
}

export interface RuleSchemaField {
  name: string;
  label?: string;
  ops: string[];
  valueType: "string" | "number" | "enum" | "multiEnum" | "bool";
  options?: string[];
}

export interface RuleSchema {
  fields: RuleSchemaField[];
}

export interface RuleClause {
  field: string;
  op: string;
  value?: unknown;
}

export interface CanvasEdit {
  id: string;
  canvasId: string;
  expectedVersion: number;
  ops: Record<string, unknown>[];
  submittedBy?: string;
  note?: string;
  createdAt: string;
}

export interface ViewerLink {
  id: string;
  workspaceId?: string;
  kind: "configuration" | "share";
  scope: "canvas";
  canvasId: string;
  runId?: string;
  agentId?: string;
  secretHash: string;
  capabilities: string[];
  createdAt: string;
  updatedAt: string;
  expiresAt?: string;
  revokedAt?: string;
  lastUsedAt?: string;
  useCount: number;
}

export interface ViewerSession {
  id: string;
  workspaceId?: string;
  linkId: string;
  kind: "configuration" | "share";
  scope: "canvas";
  canvasId: string;
  runId?: string;
  agentId?: string;
  secretHash: string;
  capabilities: string[];
  createdAt: string;
  updatedAt: string;
  expiresAt: string;
  lastUsedAt?: string;
  useCount: number;
}

export interface ViewerLinkRequest {
  kind?: "configuration" | "share";
  scope?: "canvas";
  canvasId: string;
  runId?: string;
  agentId?: string;
  capabilities?: string[];
  linkBaseUrl?: string;
  expiresAt?: string;
  ttlSeconds?: number;
}

export interface ViewerLinkCheck {
  code: string;
  status: "pass" | "fail" | "warn";
  message: string;
}

export interface ViewerLinkPreflightResponse {
  status: "ready" | "blocked";
  checks: ViewerLinkCheck[];
  kind: "configuration" | "share";
  scope: "canvas";
  canvasId: string;
  runId?: string;
  agentId?: string;
  capabilities: string[];
  linkBaseUrl: string;
}

export interface ViewerLinkCreateResponse {
  id: string;
  kind: "configuration" | "share";
  scope: "canvas";
  canvasId: string;
  runId?: string;
  agentId?: string;
  capabilities: string[];
  code: string;
  url: string;
  expiresAt?: string;
}

export interface ViewerLinkExchangeResponse {
  linkId: string;
  kind: "configuration" | "share";
  scope: "canvas";
  canvasId: string;
  runId?: string;
  agentId?: string;
  capabilities: string[];
  sessionToken: string;
  expiresAt: string;
  canvas: Canvas;
}

export interface HubEvent {
  id?: string;
  type: string;
  canvasId?: string;
  feedbackId?: string;
  data?: unknown;
  createdAt?: string;
}

export interface AuthContext {
  internal: boolean;
  keyId?: string;
  workspaceId?: string;
}

export interface ObserverDecision {
  id: string;
  eventId: string;
  provider: AgentProvider;
  agentRunId?: string;
  canvasId?: string;
  mode: "off" | "log" | "dry_run" | "nudge" | "enforce";
  action: "allow" | "log" | "dry_run" | "nudge" | "block";
  issues?: ObserverIssue[];
  dryRunActions?: ObserverDryRunAction[];
  nudgePrompt?: string;
  createdAt: string;
}

export interface ObserverIssue {
  rule: string;
  severity: string;
  message: string;
  runId?: string;
  canvasId?: string;
}

export interface ObserverDryRunAction {
  type: string;
  target?: string;
  summary: string;
  payload?: Record<string, unknown>;
}

export interface HubEnv {
  DB: D1Database;
  ASSETS_BUCKET: R2Bucket;
  EVENTS: DurableObjectNamespace;
  FEEDBACK_QUEUE?: Queue<{ deliveryId: string }>;
  HUB_TOKEN?: string;
  AGENTCANVAS_TOKEN?: string;
  CANVAS_HUB_TOKEN?: string;
  AGENTCANVAS_ASSET_PUBLIC_BASE_URL?: string;
  AGENTCANVAS_LINK_BASE_URL?: string;
  AGENTCANVAS_CORS_ORIGINS?: string;
  AGENTCANVAS_UNKEY_ROOT_KEY?: string;
  UNKEY_ROOT_KEY?: string;
  AGENTCANVAS_UNKEY_VERIFY_URL?: string;
  AGENTCANVAS_OBSERVER_ENABLED?: string;
  AGENTCANVAS_OBSERVER_PROVIDERS?: string;
  AGENTCANVAS_OBSERVER_ACTION_MODE?: string;
  AGENTCANVAS_OBSERVER_CANVAS_TELEMETRY?: string;
  [key: string]: unknown;
}
