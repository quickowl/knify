import type {
  AgentEvent,
  AgentProvider,
  AgentRun,
  AgentRunStatus,
  Block,
  Canvas,
  CanvasEdit,
  CanvasLogEvent,
  CanvasMode,
  CanvasSnapshot,
  CanvasStatus,
  CreateCanvasSnapshotRequest,
  DeliveryStatus,
  Feedback,
  FeedbackDecision,
  FeedbackDelivery,
  FeedbackTarget,
  Priority,
  ViewerLinkRequest
} from "./types";
import { badRequest, conflict, firstNonEmpty, newID, nowISO, parseJSONDate, stripUndefined, validateID } from "./utils";

export const CANVAS_BUNDLE_SCHEMA_VERSION = "agentcanvas.canvas-bundle.v1";
export const SNAPSHOT_MANUAL = "manual";
export const SNAPSHOT_STATIC_OVERWRITE = "static_overwrite";
export const SNAPSHOT_DYNAMIC_COMPLETE = "dynamic_complete";
export const SNAPSHOT_PRE_RESTORE = "pre_restore";
export const SNAPSHOT_PRE_IMPORT = "pre_import_replace";

const canvasStatuses = new Set<CanvasStatus>(["draft", "in_progress", "ready_for_review", "accepted", "needs_changes", "archived"]);
const canvasModes = new Set<CanvasMode>(["static", "dynamic"]);
const priorities = new Set<Priority>(["low", "normal", "high", "urgent"]);
const blockKinds = new Set([
  "heading",
  "markdown",
  "image",
  "terminal",
  "diff",
  "chart",
  "checklist",
  "decision",
  "link",
  "metadata",
  "video",
  "collection",
  "form",
  "orderable-list",
  "split",
  "rule-builder"
]);
const canvasEventTypes = new Set(["canvas.started", "canvas.summary.updated", "canvas.block.appended", "canvas.block.replaced", "canvas.block.removed", "canvas.completed"]);
const feedbackDecisions = new Set<FeedbackDecision>(["accepted", "needs_changes", "comment_only"]);
const agentProviders = new Set<AgentProvider>(["webhook", "generic_cloud", "cursor", "cursor_cli", "codex", "codex_exec", "claude", "claude_cli"]);
const runStatuses = new Set<AgentRunStatus>(["registered", "running", "waiting_for_feedback", "completed", "failed", "expired"]);
const deliveryStatuses = new Set<DeliveryStatus>(["pending", "queued", "retrying", "delivered", "failed", "dead_lettered"]);
const editOps = new Set(["set-field", "reorder-items", "toggle-item", "set-item-meta", "add-item", "remove-item", "set-split", "set-clauses", "submit"]);

export function validBlockKind(kind: string | undefined): boolean {
  return !!kind && blockKinds.has(kind);
}

export function validAgentProvider(provider: string | undefined): provider is AgentProvider {
  return !!provider && agentProviders.has(provider as AgentProvider);
}

export function normalizeCanvas(input: Partial<Canvas>, now = nowISO()): Canvas {
  const canvas = input as Canvas;
  if (!canvas.id) canvas.id = newID("canvas");
  validateID("canvas id", canvas.id);
  if (canvas.workspaceId) validateID("workspace id", canvas.workspaceId);
  validateID("agent id", canvas.agentId);
  validateID("run id", canvas.runId);
  if (!canvas.title?.trim()) badRequest("title is required");
  canvas.status = canvas.status || "draft";
  if (!canvasStatuses.has(canvas.status)) badRequest(`invalid status ${JSON.stringify(canvas.status)}`);
  canvas.mode = canvas.mode || "static";
  if (!canvasModes.has(canvas.mode)) badRequest(`invalid canvas mode ${JSON.stringify(canvas.mode)}`);
  canvas.priority = canvas.priority || "normal";
  if (!priorities.has(canvas.priority)) badRequest(`invalid priority ${JSON.stringify(canvas.priority)}`);
  if (!canvas.version) canvas.version = 1;
  if (canvas.version < 1) badRequest("version must be at least 1");
  if (!canvas.createdAt) canvas.createdAt = now;
  canvas.updatedAt = now;
  if (canvas.callback?.feedbackTargetId) validateID("feedback target id", canvas.callback.feedbackTargetId);
  if (canvas.lastEventId) validateID("last event id", canvas.lastEventId);
  canvas.blocks = (canvas.blocks || []).map((block, index) => normalizeBlock(block, index));
  return stripUndefined(canvas);
}

export function normalizeBlock(input: Partial<Block>, index = 0): Block {
  const block = input as Block;
  if (!block.id) block.id = newID("block");
  validateID("block id", block.id);
  if (!validBlockKind(block.kind)) badRequest(`invalid block kind ${JSON.stringify(block.kind)} at index ${index}`);
  if (block.kind === "chart") {
    if (!block.chart) badRequest(`chart block ${JSON.stringify(block.id)} requires chart`);
    block.chart.version = block.chart.version || "1";
    if (block.chart.version !== "1") badRequest(`chart block ${JSON.stringify(block.id)} has unsupported chart version ${JSON.stringify(block.chart.version)}`);
  }
  if (block.kind === "checklist") {
    block.items = (block.items || []).map((item) => ({ ...item, checked: item.checked ?? false }));
  }
  if (block.kind === "video" && !block.sourceUrl?.trim()) badRequest(`video block ${JSON.stringify(block.id)} requires sourceUrl`);
  if (block.kind === "collection") {
    block.mode = block.mode || "paged-grid-rail";
    if (block.mode !== "paged-grid-rail" && block.mode !== "paged-list") badRequest(`collection block ${JSON.stringify(block.id)} has unsupported mode ${JSON.stringify(block.mode)}`);
    block.pageSize = block.pageSize || 12;
    if (block.pageSize > 100) badRequest(`collection block ${JSON.stringify(block.id)} pageSize must be <= 100`);
    const seen = new Set<string>();
    for (const [itemIndex, item] of (block.items || []).entries()) {
      item.id = item.id?.trim();
      if (!item.id) badRequest(`collection block ${JSON.stringify(block.id)} item at index ${itemIndex} has empty id`);
      if (seen.has(item.id)) badRequest(`collection block ${JSON.stringify(block.id)} has duplicate item id ${JSON.stringify(item.id)}`);
      seen.add(item.id);
      if (!item.label?.trim()) badRequest(`collection block ${JSON.stringify(block.id)} item ${JSON.stringify(item.id)} requires label`);
      if (!item.blockIds?.length) badRequest(`collection block ${JSON.stringify(block.id)} item ${JSON.stringify(item.id)} requires blockIds`);
      for (const blockID of item.blockIds) validateID("collection block item block id", blockID);
    }
  }
  if (block.kind === "form") normalizeFormFields(block.id, block.fields || []);
  if (block.kind === "orderable-list") {
    const seen = new Set<string>();
    for (const [itemIndex, item] of (block.orderableItems || []).entries()) {
      if (!item.id?.trim()) badRequest(`orderable-list block ${JSON.stringify(block.id)} item at index ${itemIndex} has empty id`);
      if (seen.has(item.id)) badRequest(`orderable-list block ${JSON.stringify(block.id)} has duplicate item id ${JSON.stringify(item.id)}`);
      seen.add(item.id);
      item.included = item.included ?? true;
    }
    if (block.itemEditor) normalizeFormFields(block.id, block.itemEditor.fields || []);
  }
  if (block.kind === "split") {
    block.total = block.total ?? 100;
    const seen = new Set<string>();
    for (const [sliceIndex, slice] of (block.slices || []).entries()) {
      if (!slice.id?.trim()) badRequest(`split block ${JSON.stringify(block.id)} slice at index ${sliceIndex} has empty id`);
      if (seen.has(slice.id)) badRequest(`split block ${JSON.stringify(block.id)} has duplicate slice id ${JSON.stringify(slice.id)}`);
      seen.add(slice.id);
      if (slice.weight < 0) badRequest(`split block ${JSON.stringify(block.id)} slice ${JSON.stringify(slice.id)} has negative weight`);
    }
  }
  if (block.kind === "rule-builder") {
    if (!block.ruleSchema) badRequest(`rule-builder block ${JSON.stringify(block.id)} requires ruleSchema`);
    const known = new Map<string, string[]>();
    for (const [fieldIndex, field] of block.ruleSchema.fields.entries()) {
      if (!field.name?.trim()) badRequest(`rule-builder block ${JSON.stringify(block.id)} schema field at index ${fieldIndex} has empty name`);
      if (known.has(field.name)) badRequest(`rule-builder block ${JSON.stringify(block.id)} schema has duplicate field name ${JSON.stringify(field.name)}`);
      if (!["string", "number", "enum", "multiEnum", "bool"].includes(field.valueType)) {
        badRequest(`rule-builder block ${JSON.stringify(block.id)} schema field ${JSON.stringify(field.name)} has invalid valueType ${JSON.stringify(field.valueType)}`);
      }
      known.set(field.name, field.ops || []);
    }
    for (const [clauseIndex, clause] of (block.clauses || []).entries()) {
      const ops = known.get(clause.field);
      if (!ops) badRequest(`rule-builder block ${JSON.stringify(block.id)} clause at index ${clauseIndex} references unknown field ${JSON.stringify(clause.field)}`);
      if (!ops.includes(clause.op)) {
        badRequest(`rule-builder block ${JSON.stringify(block.id)} clause at index ${clauseIndex} has op ${JSON.stringify(clause.op)} not in field ${JSON.stringify(clause.field)} ops`);
      }
    }
  }
  return stripUndefined(block);
}

function normalizeFormFields(blockID: string, fields: Array<{ name: string; type: string; options?: string[] }>): void {
  const validTypes = new Set(["text", "number", "select", "multiSelect", "toggle"]);
  const seen = new Set<string>();
  for (const [index, field] of fields.entries()) {
    field.name = field.name?.trim();
    if (!field.name) badRequest(`form block ${JSON.stringify(blockID)} field at index ${index} has empty name`);
    if (seen.has(field.name)) badRequest(`form block ${JSON.stringify(blockID)} has duplicate field name ${JSON.stringify(field.name)}`);
    seen.add(field.name);
    if (!validTypes.has(field.type)) badRequest(`form block ${JSON.stringify(blockID)} field ${JSON.stringify(field.name)} has invalid type ${JSON.stringify(field.type)}`);
    if ((field.type === "select" || field.type === "multiSelect") && !field.options?.length) {
      badRequest(`form block ${JSON.stringify(blockID)} field ${JSON.stringify(field.name)} of type ${JSON.stringify(field.type)} requires non-empty options`);
    }
  }
}

export function normalizeCanvasLogEvent(input: Partial<CanvasLogEvent>, canvasID: string, now = nowISO()): CanvasLogEvent {
  const event = input as CanvasLogEvent;
  if (!event.id) event.id = newID("canvas-event");
  validateID("canvas event id", event.id);
  if (event.workspaceId) validateID("workspace id", event.workspaceId);
  validateID("canvas id", canvasID);
  if (event.canvasId && event.canvasId !== canvasID) badRequest(`canvasId ${JSON.stringify(event.canvasId)} does not match path canvas id ${JSON.stringify(canvasID)}`);
  event.canvasId = canvasID;
  if (!canvasEventTypes.has(event.type)) badRequest(`invalid canvas event type ${JSON.stringify(event.type)}`);
  if (event.expectedVersion !== undefined && event.expectedVersion < 0) badRequest("expectedVersion must be non-negative");
  if (event.agentId) validateID("agent id", event.agentId);
  if (event.runId) validateID("run id", event.runId);
  if (event.status && !canvasStatuses.has(event.status)) badRequest(`invalid status ${JSON.stringify(event.status)}`);
  if (event.priority && !priorities.has(event.priority)) badRequest(`invalid priority ${JSON.stringify(event.priority)}`);
  if (event.blockId) validateID("block id", event.blockId);
  if (event.insertAfterBlockId) validateID("insertAfterBlockId", event.insertAfterBlockId);
  if (event.block) {
    if (event.blockId && !event.block.id) event.block.id = event.blockId;
    if (event.blockId && event.block.id !== event.blockId) badRequest(`block id ${JSON.stringify(event.block.id)} does not match replacement block id ${JSON.stringify(event.blockId)}`);
    event.block = normalizeBlock(event.block, 0);
  }
  if (event.type === "canvas.started") {
    if (!event.agentId) badRequest(`agentId is required for ${event.type}`);
    if (!event.runId) badRequest(`runId is required for ${event.type}`);
    if (!event.title?.trim()) badRequest(`title is required for ${event.type}`);
  }
  if (event.type === "canvas.block.appended" && !event.block) badRequest(`block is required for ${event.type}`);
  if (event.type === "canvas.block.replaced") {
    if (!event.block) badRequest(`block is required for ${event.type}`);
    event.blockId = event.blockId || event.block.id;
  }
  if (event.type === "canvas.block.removed" && !event.blockId) badRequest(`blockId is required for ${event.type}`);
  event.createdAt = event.createdAt || now;
  return stripUndefined(event);
}

export function projectCanvasLogEvent(existing: Canvas | undefined, event: CanvasLogEvent, now = nowISO()): Canvas {
  let canvas: Canvas;
  if (!existing) {
    canvas = {
      id: event.canvasId,
      workspaceId: event.workspaceId,
      agentId: event.agentId || "",
      runId: event.runId || "",
      title: event.title || "",
      summary: event.summary || "",
      status: event.status || "in_progress",
      mode: "dynamic",
      priority: event.priority || "normal",
      createdAt: event.createdAt,
      updatedAt: now,
      version: 1,
      callback: { feedbackTargetId: event.runId },
      blocks: []
    };
  } else {
    canvas = structuredClone(existing);
    canvas.mode = "dynamic";
    canvas.version += 1;
    canvas.updatedAt = now;
  }
  switch (event.type) {
    case "canvas.started":
      updateCanvasMetadata(canvas, event);
      canvas.status = event.status || "in_progress";
      canvas.priority = event.priority || canvas.priority || "normal";
      if (event.runId) {
        canvas.callback = canvas.callback || {};
        canvas.callback.feedbackTargetId = canvas.callback.feedbackTargetId || event.runId;
      }
      break;
    case "canvas.summary.updated":
      updateCanvasMetadata(canvas, event);
      break;
    case "canvas.block.appended": {
      const block = event.block!;
      if (canvas.blocks.some((candidate) => candidate.id === block.id)) conflict(`block ${JSON.stringify(block.id)} already exists`);
      if (!event.insertAfterBlockId) canvas.blocks.push(block);
      else {
        const index = canvas.blocks.findIndex((candidate) => candidate.id === event.insertAfterBlockId);
        if (index < 0) badRequest(`insertAfterBlockId ${JSON.stringify(event.insertAfterBlockId)} was not found`);
        canvas.blocks.splice(index + 1, 0, block);
      }
      break;
    }
    case "canvas.block.replaced": {
      const index = canvas.blocks.findIndex((candidate) => candidate.id === event.blockId);
      if (index < 0) throw new Error("not_found:block");
      canvas.blocks[index] = event.block!;
      break;
    }
    case "canvas.block.removed": {
      const index = canvas.blocks.findIndex((candidate) => candidate.id === event.blockId);
      if (index < 0) throw new Error("not_found:block");
      canvas.blocks.splice(index, 1);
      break;
    }
    case "canvas.completed":
      updateCanvasMetadata(canvas, event);
      canvas.status = event.status || "ready_for_review";
      break;
  }
  canvas.lastEventId = event.id;
  canvas.createdAt = canvas.createdAt || event.createdAt;
  canvas.priority = canvas.priority || "normal";
  return stripUndefined(canvas);
}

function updateCanvasMetadata(canvas: Canvas, event: CanvasLogEvent): void {
  if (event.agentId) canvas.agentId = event.agentId;
  if (event.runId) canvas.runId = event.runId;
  if (event.title) canvas.title = event.title;
  if (event.summary) canvas.summary = event.summary;
  if (event.status) canvas.status = event.status;
  if (event.priority) canvas.priority = event.priority;
}

export function normalizeSnapshot(input: Partial<CanvasSnapshot>, canvasID: string, now = nowISO()): CanvasSnapshot {
  const snapshot = input as CanvasSnapshot;
  if (!snapshot.id) snapshot.id = newID("snapshot");
  validateID("snapshot id", snapshot.id);
  validateID("canvas id", canvasID || snapshot.canvasId);
  if (snapshot.canvasId && snapshot.canvasId !== canvasID) badRequest(`canvasId ${JSON.stringify(snapshot.canvasId)} does not match path canvas id ${JSON.stringify(canvasID)}`);
  snapshot.canvasId = canvasID;
  if (!snapshot.canvas?.id) badRequest("snapshot canvas is required");
  if (snapshot.canvas.id !== canvasID) badRequest(`snapshot canvas id ${JSON.stringify(snapshot.canvas.id)} does not match canvas id ${JSON.stringify(canvasID)}`);
  snapshot.version = snapshot.version || snapshot.canvas.version;
  if (snapshot.version < 1) badRequest("snapshot version must be at least 1");
  snapshot.reason = snapshot.reason || SNAPSHOT_MANUAL;
  snapshot.source = snapshot.source || "hub";
  if (snapshot.sourceEventId) validateID("source event id", snapshot.sourceEventId);
  if (snapshot.sourceEditId) validateID("source edit id", snapshot.sourceEditId);
  snapshot.createdAt = snapshot.createdAt || now;
  return stripUndefined(snapshot);
}

export function newCanvasSnapshot(canvas: Canvas, request: CreateCanvasSnapshotRequest = {}, now = nowISO()): CanvasSnapshot {
  return normalizeSnapshot(
    {
      id: newID("snapshot"),
      canvasId: canvas.id,
      version: canvas.version,
      reason: firstNonEmpty(request.reason, SNAPSHOT_MANUAL),
      source: firstNonEmpty(request.source, "hub"),
      label: request.label,
      sourceEventId: request.sourceEventId,
      sourceEditId: request.sourceEditId,
      createdAt: now,
      canvas: structuredClone(canvas)
    },
    canvas.id,
    now
  );
}

export function normalizeFeedback(input: Partial<Feedback>, canvasID: string, hasDeliveryTarget: boolean, now = nowISO()): Feedback {
  const feedback = input as Feedback;
  if (!feedback.id) feedback.id = newID("feedback");
  validateID("feedback id", feedback.id);
  if (feedback.workspaceId) validateID("workspace id", feedback.workspaceId);
  validateID("canvas id", canvasID);
  feedback.canvasId = canvasID;
  if (!feedbackDecisions.has(feedback.decision)) badRequest(`invalid decision ${JSON.stringify(feedback.decision)}`);
  if (feedback.inkAssetId) validateID("ink asset id", feedback.inkAssetId);
  for (const id of feedback.targetBlockIds || []) validateID("target block id", id);
  const ids = new Set(feedback.targetBlockIds || []);
  for (const anchor of feedback.targetAnchors || []) {
    validateID("target anchor block id", anchor.blockId);
    if (anchor.x < 0 || anchor.x > 1 || anchor.y < 0 || anchor.y > 1) badRequest(`target anchor ${JSON.stringify(anchor.blockId)} coordinates must be normalized between 0 and 1`);
    if (!ids.has(anchor.blockId)) {
      feedback.targetBlockIds = [...(feedback.targetBlockIds || []), anchor.blockId];
      ids.add(anchor.blockId);
    }
  }
  feedback.createdAt = feedback.createdAt || now;
  feedback.deliveryStatus = hasDeliveryTarget ? "pending" : "delivered";
  return stripUndefined(feedback);
}

export function normalizeFeedbackTarget(target: FeedbackTarget, defaultID: string): FeedbackTarget {
  target.id = target.id || defaultID;
  if (target.id) validateID("feedback target id", target.id);
  if (!validAgentProvider(target.provider)) badRequest(`invalid provider ${JSON.stringify(target.provider)}`);
  if ((target.provider === "webhook" || target.provider === "generic_cloud" || target.mode === "webhook") && !target.url?.trim()) {
    badRequest("webhook feedback target requires url");
  }
  return stripUndefined(target);
}

export function normalizeAgentRun(input: Partial<AgentRun>, now = nowISO()): AgentRun {
  const run = input as AgentRun;
  run.id = run.id || run.runId || newID("run");
  validateID("agent run id", run.id);
  if (run.workspaceId) validateID("workspace id", run.workspaceId);
  if (!validAgentProvider(run.provider)) badRequest(`invalid provider ${JSON.stringify(run.provider)}`);
  run.agentId = run.agentId || run.provider;
  validateID("agent id", run.agentId);
  run.runId = run.runId || run.id;
  validateID("run id", run.runId);
  if (run.canvasId) validateID("canvas id", run.canvasId);
  run.status = run.status || "registered";
  if (!runStatuses.has(run.status)) badRequest(`invalid agent run status ${JSON.stringify(run.status)}`);
  run.feedbackTarget = run.feedbackTarget || ({ provider: run.provider } as FeedbackTarget);
  if (run.externalId && !run.feedbackTarget.externalId) run.feedbackTarget.externalId = run.externalId;
  if (run.authRef && !run.feedbackTarget.authRef) run.feedbackTarget.authRef = run.authRef;
  run.feedbackTarget.provider = run.feedbackTarget.provider || run.provider;
  run.feedbackTarget.mode = run.feedbackTarget.mode || run.mode;
  run.feedbackTarget = normalizeFeedbackTarget(run.feedbackTarget, run.id);
  run.createdAt = run.createdAt || now;
  run.updatedAt = now;
  return stripUndefined(run);
}

export function normalizeAgentEvent(input: Partial<AgentEvent>, now = nowISO()): AgentEvent {
  const event = input as AgentEvent;
  event.id = event.id || newID("event");
  validateID("agent event id", event.id);
  if (event.workspaceId) validateID("workspace id", event.workspaceId);
  if (!validAgentProvider(event.provider)) badRequest(`invalid provider ${JSON.stringify(event.provider)}`);
  if (event.agentRunId) validateID("agent run id", event.agentRunId);
  if (!event.type?.trim()) badRequest("event type is required");
  if (event.status && !runStatuses.has(event.status)) badRequest(`invalid agent event status ${JSON.stringify(event.status)}`);
  event.occurredAt = event.occurredAt || now;
  event.createdAt = event.createdAt || now;
  return stripUndefined(event);
}

export function normalizeFeedbackDelivery(input: Partial<FeedbackDelivery>, now = nowISO()): FeedbackDelivery {
  const delivery = input as FeedbackDelivery;
  delivery.id = delivery.id || newID("delivery");
  validateID("feedback delivery id", delivery.id);
  if (delivery.workspaceId) validateID("workspace id", delivery.workspaceId);
  validateID("feedback id", delivery.feedbackId);
  validateID("canvas id", delivery.canvasId);
  if (delivery.agentRunId) validateID("agent run id", delivery.agentRunId);
  if (delivery.targetId) validateID("feedback target id", delivery.targetId);
  if (!validAgentProvider(delivery.provider)) badRequest(`invalid delivery provider ${JSON.stringify(delivery.provider)}`);
  if (!deliveryStatuses.has(delivery.status)) badRequest(`invalid delivery status ${JSON.stringify(delivery.status)}`);
  delivery.target = normalizeFeedbackTarget(delivery.target, delivery.targetId || delivery.agentRunId || delivery.id);
  delivery.attempts = delivery.attempts || 0;
  delivery.maxAttempts = delivery.maxAttempts || 3;
  delivery.createdAt = delivery.createdAt || now;
  delivery.updatedAt = now;
  return stripUndefined(delivery);
}

export function normalizeCanvasEdit(input: Partial<CanvasEdit>, now = nowISO()): CanvasEdit {
  const edit = input as CanvasEdit;
  edit.id = edit.id || newID("edit");
  validateID("edit id", edit.id);
  validateID("canvas id", edit.canvasId);
  if (edit.expectedVersion < 1) badRequest("expectedVersion must be at least 1");
  if (!edit.ops?.length) badRequest("at least one op is required");
  for (const [index, op] of edit.ops.entries()) {
    const name = op.op;
    if (typeof name !== "string" || name === "") badRequest(`op at index ${index} has non-string or empty "op" value`);
    if (!editOps.has(name)) badRequest(`op at index ${index} has unknown op name ${JSON.stringify(name)}`);
    if (name !== "submit" && (typeof op.blockId !== "string" || !op.blockId.trim())) badRequest(`op at index ${index} (${JSON.stringify(name)}) requires a non-empty "blockId"`);
  }
  edit.createdAt = edit.createdAt || now;
  return stripUndefined(edit);
}

export function opRequiresBlockKind(op: string): string | undefined {
  switch (op) {
    case "set-field":
      return "form";
    case "reorder-items":
    case "toggle-item":
    case "set-item-meta":
    case "add-item":
    case "remove-item":
      return "orderable-list";
    case "set-split":
      return "split";
    case "set-clauses":
      return "rule-builder";
    default:
      return undefined;
  }
}

export function normalizeViewerLinkRequest(req: ViewerLinkRequest, defaultBaseURL: string, now = nowISO()): Required<ViewerLinkRequest> {
  const kind = req.kind || "configuration";
  const scope = req.scope || "canvas";
  const capabilities = req.capabilities?.length
    ? req.capabilities
    : kind === "configuration"
      ? ["canvas.read", "canvas.live", "asset.read", "feedback.submit"]
      : ["canvas.read", "asset.read"];
  const expiresAt = req.expiresAt || (req.ttlSeconds && req.ttlSeconds > 0 ? new Date(parseJSONDate(now) + req.ttlSeconds * 1000).toISOString() : new Date(parseJSONDate(now) + 7 * 24 * 60 * 60 * 1000).toISOString());
  return {
    kind,
    scope,
    canvasId: req.canvasId,
    runId: req.runId || "",
    agentId: req.agentId || "",
    capabilities,
    linkBaseUrl: req.linkBaseUrl || defaultBaseURL,
    expiresAt,
    ttlSeconds: req.ttlSeconds || 0
  };
}

export function canvasReferencesAsset(canvas: Canvas, assetID: string): boolean {
  return canvas.blocks.some((block) => block.assetId === assetID || block.thumbnailAssetId === assetID);
}

export function sanitizeCanvasForViewer(canvas: Canvas): Canvas {
  const clone = structuredClone(canvas);
  delete clone.callback;
  return clone;
}
