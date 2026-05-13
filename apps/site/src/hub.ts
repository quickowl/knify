import type { AgentCanvas, CanvasBlock, CanvasEdit, CanvasSnapshot, FeedbackDeliverySummary, FeedbackRequest, HubEvent } from "./types";

export function parseCanvasIDSegment(segment: string): string {
  const decoded = decodeURIComponent(segment);
  const separator = decoded.lastIndexOf("--");
  return separator >= 0 ? decoded.slice(separator + 2) : decoded;
}

export function canvasSlug(canvas: Pick<AgentCanvas, "id" | "title">): string {
  return `${slugify(canvas.title)}--${encodeURIComponent(canvas.id)}`;
}

export function statusLabel(status: string): string {
  switch (status) {
    case "draft":
    case "queued":
      return "Queued";
    case "running":
    case "in_progress":
      return "Running";
    case "ready_for_review":
    case "needs_review":
      return "Needs Review";
    case "needs_changes":
      return "Needs Changes";
    case "accepted":
    case "completed":
      return "Completed";
    case "archived":
      return "Archived";
    case "failed":
      return "Failed";
    default:
      return status.replace(/_/g, " ");
  }
}

export function normalizeCanvases(raw: unknown): AgentCanvas[] {
  const records = Array.isArray(raw) ? raw : asRecord(raw).canvases;
  return Array.isArray(records) ? records.map(normalizeCanvas).sort((a, b) => b.updatedAtMs - a.updatedAtMs) : [];
}

export function normalizeCanvas(raw: unknown): AgentCanvas {
  const record = asRecord(raw);
  const createdAt = readString(record, "createdAt") || new Date(0).toISOString();
  const updatedAt = readString(record, "updatedAt") || createdAt;
  const status = readString(record, "status") || "draft";
  const priority = readString(record, "priority") || "normal";
  const agentID = readString(record, "agentId", "agentID") || "unknown-agent";
  const runID = readString(record, "runId", "runID") || "unknown-run";

  return {
    id: readString(record, "id") || "unknown-canvas",
    workspaceID: readString(record, "workspaceId", "workspaceID") || undefined,
    title: readString(record, "title") || "Untitled canvas",
    summary: readString(record, "summary") || "",
    agentID,
    agentName: readString(record, "agentName") || agentID,
    runID,
    status,
    mode: normalizeCanvasMode(readString(record, "mode")),
    priority,
    reviewState: normalizeReviewState(readString(record, "reviewState"), status),
    createdAt,
    updatedAt,
    updatedAtMs: Date.parse(updatedAt) || 0,
    version: readNumber(record, "version") || 1,
    lastEventId: readString(record, "lastEventId", "lastEventID") || undefined,
    tags: readStringArray(record.tags),
    blocks: Array.isArray(record.blocks) ? record.blocks.map(normalizeBlock) : [],
  };
}

export function normalizeBlock(raw: unknown): CanvasBlock {
  const record = asRecord(raw);
  const payload = asRecord(record.payload);
  const kind = readString(record, "kind") || readString(record, "type") || "metadata";
  return {
    id: readString(record, "id") || readString(payload, "id") || `block-${kind}`,
    kind,
    payload,
    raw: record,
  };
}

export async function hubJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (!headers.has("Accept")) headers.set("Accept", "application/json");
  const response = await fetch(path.startsWith("/api/hub") ? path : `/api/hub${path}`, {
    ...init,
    headers,
  });
  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(`Hub returned HTTP ${response.status}${detail ? `: ${detail}` : ""}`);
  }
  return await response.json() as T;
}

export async function fetchCanvas(canvasID: string): Promise<AgentCanvas> {
  return normalizeCanvas(await hubJSON(`/canvases/${encodeURIComponent(canvasID)}`));
}

export async function fetchCanvases(): Promise<AgentCanvas[]> {
  return normalizeCanvases(await hubJSON("/canvases"));
}

export async function fetchFeedbackDeliveries(canvasID: string): Promise<FeedbackDeliverySummary[]> {
  return await hubJSON(`/feedback-deliveries?canvasId=${encodeURIComponent(canvasID)}`);
}

export async function fetchEdits(canvasID: string): Promise<CanvasEdit[]> {
  const raw = await hubJSON<unknown>(`/canvases/${encodeURIComponent(canvasID)}/edits`);
  return Array.isArray(raw) ? raw as CanvasEdit[] : Array.isArray(asRecord(raw).items) ? asRecord(raw).items as CanvasEdit[] : [];
}

export async function fetchSnapshots(canvasID: string): Promise<CanvasSnapshot[]> {
  const raw = await hubJSON<unknown>(`/canvases/${encodeURIComponent(canvasID)}/snapshots`);
  const records = Array.isArray(raw) ? raw : asRecord(raw).snapshots;
  return Array.isArray(records) ? records as CanvasSnapshot[] : [];
}

export async function submitFeedback(request: FeedbackRequest) {
  return hubJSON(`/canvases/${encodeURIComponent(request.canvasId)}/feedback`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(request),
  });
}

export async function createSnapshot(canvasID: string, version: number) {
  return hubJSON(`/canvases/${encodeURIComponent(canvasID)}/snapshots`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ reason: "manual", source: "site-dashboard", label: `Manual dashboard snapshot v${version}` }),
  });
}

export async function restoreSnapshot(canvasID: string, snapshotID: string) {
  return hubJSON(`/canvases/${encodeURIComponent(canvasID)}/snapshots/${encodeURIComponent(snapshotID)}/restore`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  });
}

export async function exportCanvas(canvasID: string) {
  return hubJSON(`/canvases/${encodeURIComponent(canvasID)}/export`);
}

export async function createViewerLink(canvasID: string) {
  return hubJSON<{ url?: string; code?: string; id?: string }>("/viewer-links", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ kind: "share", scope: "canvas", canvasId: canvasID, capabilities: ["canvas.read", "canvas.live", "asset.read", "feedback.submit"] }),
  });
}

export function openHubEventSource(onEvent: (event: HubEvent) => void) {
  const source = new EventSource("/api/hub/events");
  const handle = (event: MessageEvent<string>) => {
    try {
      onEvent(JSON.parse(event.data) as HubEvent);
    } catch {
      // Ignore malformed keepalives from dev servers.
    }
  };
  source.addEventListener("message", handle);
  [
    "canvas.created",
    "canvas.updated",
    "canvas.restored",
    "canvas.imported",
    "canvas.event.created",
    "canvas.snapshot.created",
    "feedback.created",
    "feedback.delivery.queued",
    "feedback.delivery.retrying",
    "feedback.delivered",
  ].forEach((name) => source.addEventListener(name, handle));
  return source;
}

export function readBlockValue<T = unknown>(block: CanvasBlock, key: string): T | undefined {
  return (block.payload[key] ?? block.raw[key]) as T | undefined;
}

export function blockLabel(block: CanvasBlock): string {
  const title =
    readBlockValue<string>(block, "title") ||
    readBlockValue<string>(block, "text") ||
    readBlockValue<string>(block, "prompt") ||
    readBlockValue<string>(block, "url") ||
    block.kind;
  return title.length > 80 ? `${title.slice(0, 77)}...` : title;
}

export function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function slugify(value: string): string {
  const slug = value
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
  return slug || "canvas";
}

function normalizeReviewState(value: string | undefined, status: string) {
  if (value === "approved" || value === "rejected" || value === "skipped" || value === "pending") return value;
  if (status === "accepted" || status === "completed" || status === "archived") return "approved";
  if (status === "failed") return "rejected";
  return "pending";
}

function normalizeCanvasMode(value: string | undefined) {
  return value === "dynamic" || value === "static" ? value : undefined;
}

function readString(record: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const value = record[key];
    if (typeof value === "string") return value;
  }
  return "";
}

function readNumber(record: Record<string, unknown>, key: string): number {
  const value = record[key];
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function readStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}
