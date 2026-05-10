import type {
  AgentEvent,
  AgentRun,
  Asset,
  Canvas,
  CanvasBundle,
  CanvasBundleAsset,
  CanvasEdit,
  CanvasImportRequest,
  CanvasImportResult,
  CanvasLogEvent,
  CanvasRestoreResponse,
  CanvasSnapshot,
  CreateCanvasSnapshotRequest,
  Feedback,
  FeedbackDelivery,
  HubEnv,
  ViewerLink,
  ViewerSession
} from "./types";
import {
  CANVAS_BUNDLE_SCHEMA_VERSION,
  SNAPSHOT_PRE_IMPORT,
  SNAPSHOT_PRE_RESTORE,
  newCanvasSnapshot,
  normalizeCanvas,
  normalizeCanvasLogEvent,
  normalizeSnapshot,
  projectCanvasLogEvent
} from "./validation";
import { AppError, base64Decode, base64Encode, badRequest, conflict, newID, notFound, nowISO, parseJSONDate, stripUndefined, validateID } from "./utils";

export interface Store {
  saveCanvas(canvas: Canvas): Promise<void>;
  getCanvas(id: string): Promise<Canvas>;
  listCanvases(): Promise<Canvas[]>;
  applyCanvasLogEvent(event: CanvasLogEvent, now?: string): Promise<{ canvas: Canvas; created: boolean; event: CanvasLogEvent }>;
  listCanvasLogEvents(canvasID: string): Promise<CanvasLogEvent[]>;
  createCanvasSnapshot(canvasID: string, request?: CreateCanvasSnapshotRequest, now?: string): Promise<CanvasSnapshot>;
  getCanvasSnapshot(canvasID: string, snapshotID: string): Promise<CanvasSnapshot>;
  listCanvasSnapshots(canvasID: string): Promise<CanvasSnapshot[]>;
  restoreCanvasSnapshot(canvasID: string, snapshotID: string, now?: string): Promise<CanvasRestoreResponse>;
  exportCanvasBundle(canvasID: string, now?: string): Promise<CanvasBundle>;
  importCanvasBundle(request: CanvasImportRequest, now?: string): Promise<CanvasImportResult>;
  saveFeedback(feedback: Feedback): Promise<void>;
  getFeedback(id: string): Promise<Feedback>;
  listFeedback(canvasID: string): Promise<Feedback[]>;
  saveEdit(edit: CanvasEdit): Promise<void>;
  getEdit(id: string): Promise<CanvasEdit>;
  listEditsForCanvas(canvasID: string): Promise<CanvasEdit[]>;
  saveAgentRun(run: AgentRun): Promise<void>;
  getAgentRun(id: string): Promise<AgentRun>;
  listAgentRuns(): Promise<AgentRun[]>;
  saveAgentEvent(event: AgentEvent): Promise<boolean>;
  getAgentEvent(id: string): Promise<AgentEvent>;
  listAgentEvents(): Promise<AgentEvent[]>;
  saveFeedbackDelivery(delivery: FeedbackDelivery): Promise<void>;
  getFeedbackDelivery(id: string): Promise<FeedbackDelivery>;
  listFeedbackDeliveries(): Promise<FeedbackDelivery[]>;
  retryFeedbackDelivery(id: string, now?: string): Promise<FeedbackDelivery>;
  saveAsset(id: string, contentType: string, body: BodyInit | ReadableStream, workspaceID?: string): Promise<Asset>;
  getAsset(id: string): Promise<{ asset: Asset; body: ReadableStream | ArrayBuffer }>;
  saveViewerLink(link: ViewerLink): Promise<void>;
  getViewerLink(id: string): Promise<ViewerLink>;
  touchViewerLink(id: string, now?: string): Promise<ViewerLink>;
  revokeViewerLink(id: string, now?: string): Promise<ViewerLink>;
  saveViewerSession(session: ViewerSession): Promise<void>;
  getViewerSession(id: string): Promise<ViewerSession>;
  touchViewerSession(id: string, now?: string): Promise<ViewerSession>;
}

type Kind =
  | "canvas"
  | "canvas_event"
  | "snapshot"
  | "feedback"
  | "edit"
  | "agent_run"
  | "agent_event"
  | "feedback_delivery"
  | "asset"
  | "viewer_link"
  | "viewer_session";

interface RecordMeta {
  workspaceId?: string;
  canvasId?: string;
  agentId?: string;
  runId?: string;
  provider?: string;
  status?: string;
  feedbackId?: string;
  createdAt?: string;
  updatedAt?: string;
}

abstract class BaseStore implements Store {
  protected abstract put<T extends Record<string, unknown>>(kind: Kind, id: string, value: T, meta?: RecordMeta): Promise<void>;
  protected abstract get<T>(kind: Kind, id: string): Promise<T>;
  protected abstract list<T>(kind: Kind): Promise<T[]>;
  protected abstract putAssetBody(id: string, body: ArrayBuffer, contentType: string): Promise<void>;
  protected abstract getAssetBody(id: string): Promise<ArrayBuffer>;

  async saveCanvas(canvas: Canvas): Promise<void> {
    await this.put("canvas", canvas.id, canvas as unknown as Record<string, unknown>, metaFromCanvas(canvas));
  }

  async getCanvas(id: string): Promise<Canvas> {
    validateID("canvas id", id);
    return this.get<Canvas>("canvas", id);
  }

  async listCanvases(): Promise<Canvas[]> {
    return sortAsc(await this.list<Canvas>("canvas"));
  }

  async applyCanvasLogEvent(event: CanvasLogEvent, now = nowISO()): Promise<{ canvas: Canvas; created: boolean; event: CanvasLogEvent }> {
    validateID("canvas id", event.canvasId);
    validateID("canvas event id", event.id);
    try {
      const existingEvent = await this.get<CanvasLogEvent>("canvas_event", event.id);
      const canvas = await this.getCanvas(event.canvasId);
      return { canvas, created: false, event: existingEvent };
    } catch (error) {
      if (!(error instanceof AppError) || error.status !== 404) throw error;
    }
    let existing: Canvas | undefined;
    try {
      existing = await this.getCanvas(event.canvasId);
    } catch (error) {
      if (!(error instanceof AppError) || error.status !== 404) throw error;
    }
    if (event.expectedVersion !== undefined) {
      const currentVersion = existing?.version || 0;
      if (event.expectedVersion !== currentVersion) conflict(`expected canvas version ${event.expectedVersion}, got ${currentVersion}`);
    }
    if (!existing && event.type !== "canvas.started") notFound();
    let canvas: Canvas;
    try {
      canvas = projectCanvasLogEvent(existing, event, now);
    } catch (error) {
      if (error instanceof Error && error.message.startsWith("not_found:")) notFound();
      throw error;
    }
    event.version = canvas.version;
    await this.saveCanvas(canvas);
    await this.put("canvas_event", event.id, event as unknown as Record<string, unknown>, {
      workspaceId: event.workspaceId,
      canvasId: event.canvasId,
      createdAt: event.createdAt,
      updatedAt: event.createdAt
    });
    return { canvas, created: true, event };
  }

  async listCanvasLogEvents(canvasID: string): Promise<CanvasLogEvent[]> {
    validateID("canvas id", canvasID);
    return (await this.list<CanvasLogEvent>("canvas_event"))
      .filter((event) => event.canvasId === canvasID)
      .sort((a, b) => (a.version || 0) - (b.version || 0) || compareAsc(a, b));
  }

  async createCanvasSnapshot(canvasID: string, request: CreateCanvasSnapshotRequest = {}, now = nowISO()): Promise<CanvasSnapshot> {
    const canvas = await this.getCanvas(canvasID);
    const snapshot = newCanvasSnapshot(canvas, request, now);
    await this.put("snapshot", snapshot.id, snapshot as unknown as Record<string, unknown>, {
      workspaceId: canvas.workspaceId,
      canvasId: canvasID,
      createdAt: snapshot.createdAt,
      updatedAt: snapshot.createdAt
    });
    return snapshot;
  }

  async getCanvasSnapshot(canvasID: string, snapshotID: string): Promise<CanvasSnapshot> {
    validateID("canvas id", canvasID);
    validateID("snapshot id", snapshotID);
    const snapshot = await this.get<CanvasSnapshot>("snapshot", snapshotID);
    if (snapshot.canvasId !== canvasID) notFound();
    return snapshot;
  }

  async listCanvasSnapshots(canvasID: string): Promise<CanvasSnapshot[]> {
    validateID("canvas id", canvasID);
    return (await this.list<CanvasSnapshot>("snapshot"))
      .filter((snapshot) => snapshot.canvasId === canvasID)
      .sort((a, b) => compareDesc(a, b));
  }

  async restoreCanvasSnapshot(canvasID: string, snapshotID: string, now = nowISO()): Promise<CanvasRestoreResponse> {
    const current = await this.getCanvas(canvasID);
    const snapshot = normalizeSnapshot(await this.getCanvasSnapshot(canvasID, snapshotID), canvasID, now);
    const checkpoint = newCanvasSnapshot(current, { reason: SNAPSHOT_PRE_RESTORE, source: "restore", label: `Before restoring ${snapshot.id}` }, now);
    const restored = structuredClone(snapshot.canvas);
    restored.id = canvasID;
    restored.version = current.version + 1;
    restored.updatedAt = now;
    restored.createdAt = restored.createdAt || current.createdAt || now;
    await this.put("snapshot", checkpoint.id, checkpoint as unknown as Record<string, unknown>, {
      workspaceId: current.workspaceId,
      canvasId: canvasID,
      createdAt: checkpoint.createdAt,
      updatedAt: checkpoint.createdAt
    });
    await this.saveCanvas(restored);
    return { canvas: restored, snapshot, checkpoint };
  }

  async exportCanvasBundle(canvasID: string, now = nowISO()): Promise<CanvasBundle> {
    const canvas = await this.getCanvas(canvasID);
    const snapshots = await this.listCanvasSnapshots(canvasID);
    const events = await this.listCanvasLogEvents(canvasID);
    const feedback = await this.listFeedback(canvasID);
    const edits = await this.listEditsForCanvas(canvasID);
    const runs = (await this.listAgentRuns()).filter((run) => run.canvasId === canvas.id || run.id === canvas.callback?.feedbackTargetId || run.runId === canvas.runId);
    const assetIDs = new Set<string>();
    addCanvasAssetRefs(assetIDs, canvas);
    for (const snapshot of snapshots) addCanvasAssetRefs(assetIDs, snapshot.canvas);
    for (const event of events) if (event.block) addBlockAssetRefs(assetIDs, event.block);
    for (const item of feedback) if (item.inkAssetId) assetIDs.add(item.inkAssetId);
    const assets: CanvasBundleAsset[] = [];
    for (const id of [...assetIDs].sort()) {
      const { asset, body } = await this.getAsset(id);
      const buffer = body instanceof ArrayBuffer ? body : await new Response(body).arrayBuffer();
      assets.push({ asset, bodyBase64: base64Encode(buffer) });
    }
    return stripUndefined({
      schemaVersion: CANVAS_BUNDLE_SCHEMA_VERSION,
      exportedAt: now,
      canvas,
      snapshots,
      events,
      feedback,
      edits,
      assets,
      agentRuns: runs
    });
  }

  async importCanvasBundle(request: CanvasImportRequest, now = nowISO()): Promise<CanvasImportResult> {
    const bundle = request.bundle;
    if (bundle.schemaVersion !== CANVAS_BUNDLE_SCHEMA_VERSION) badRequest(`unsupported bundle schemaVersion ${JSON.stringify(bundle.schemaVersion)}`);
    if (!bundle.canvas?.id) badRequest("bundle canvas is required");
    const policy = request.conflictPolicy || "fail";
    if (policy !== "fail" && policy !== "replace") badRequest(`unsupported conflictPolicy ${JSON.stringify(policy)}`);
    const canvas = normalizeCanvas(bundle.canvas, now);
    let current: Canvas | undefined;
    try {
      current = await this.getCanvas(canvas.id);
    } catch (error) {
      if (!(error instanceof AppError) || error.status !== 404) throw error;
    }
    if (current && policy === "fail") conflict(`canvas ${JSON.stringify(canvas.id)} already exists`);
    let checkpoint: CanvasSnapshot | undefined;
    if (current) {
      checkpoint = newCanvasSnapshot(current, { reason: SNAPSHOT_PRE_IMPORT, source: "import", label: "Before import replace" }, now);
      await this.put("snapshot", checkpoint.id, checkpoint as unknown as Record<string, unknown>, metaFromCanvas(current));
      canvas.version = current.version + 1;
      canvas.updatedAt = now;
      canvas.createdAt = canvas.createdAt || current.createdAt;
    }
    let importedAssets = 0;
    for (const item of bundle.assets || []) {
      validateID("asset id", item.asset.id);
      const bytes = base64Decode(item.bodyBase64);
      const asset = { ...item.asset, size: item.asset.size || bytes.byteLength, createdAt: item.asset.createdAt || now };
      const body = new Uint8Array(bytes);
      await this.putAssetBody(asset.id, body.buffer, asset.contentType);
      await this.put("asset", asset.id, asset as unknown as Record<string, unknown>, {
        workspaceId: asset.workspaceId,
        createdAt: asset.createdAt,
        updatedAt: asset.createdAt
      });
      importedAssets++;
    }
    for (const run of bundle.agentRuns || []) await this.saveAgentRun(run);
    for (const snapshot of bundle.snapshots || []) {
      const normalized = normalizeSnapshot(snapshot, canvas.id, now);
      await this.put("snapshot", normalized.id, normalized as unknown as Record<string, unknown>, { workspaceId: canvas.workspaceId, canvasId: canvas.id, createdAt: normalized.createdAt });
    }
    for (const event of bundle.events || []) {
      const normalized = normalizeCanvasLogEvent(event, canvas.id, now);
      await this.put("canvas_event", normalized.id, normalized as unknown as Record<string, unknown>, { workspaceId: normalized.workspaceId, canvasId: canvas.id, createdAt: normalized.createdAt });
    }
    for (const item of bundle.feedback || []) {
      validateID("feedback id", item.id);
      if (item.canvasId !== canvas.id) badRequest(`feedback ${JSON.stringify(item.id)} canvasId does not match bundle canvas`);
      await this.saveFeedback(item);
    }
    for (const edit of bundle.edits || []) {
      validateID("edit id", edit.id);
      if (edit.canvasId !== canvas.id) badRequest(`edit ${JSON.stringify(edit.id)} canvasId does not match bundle canvas`);
      await this.saveEdit(edit);
    }
    await this.saveCanvas(canvas);
    return {
      canvas,
      conflictPolicy: policy,
      checkpoint,
      importedSnapshots: bundle.snapshots?.length || 0,
      importedEvents: bundle.events?.length || 0,
      importedFeedback: bundle.feedback?.length || 0,
      importedEdits: bundle.edits?.length || 0,
      importedAssets,
      importedAgentRuns: bundle.agentRuns?.length || 0
    };
  }

  async saveFeedback(feedback: Feedback): Promise<void> {
    await this.put("feedback", feedback.id, feedback as unknown as Record<string, unknown>, {
      workspaceId: feedback.workspaceId,
      canvasId: feedback.canvasId,
      status: feedback.deliveryStatus,
      createdAt: feedback.createdAt,
      updatedAt: feedback.createdAt
    });
  }

  async getFeedback(id: string): Promise<Feedback> {
    validateID("feedback id", id);
    return this.get<Feedback>("feedback", id);
  }

  async listFeedback(canvasID: string): Promise<Feedback[]> {
    validateID("canvas id", canvasID);
    return sortAsc((await this.list<Feedback>("feedback")).filter((item) => item.canvasId === canvasID));
  }

  async saveEdit(edit: CanvasEdit): Promise<void> {
    await this.put("edit", edit.id, edit as unknown as Record<string, unknown>, { canvasId: edit.canvasId, createdAt: edit.createdAt, updatedAt: edit.createdAt });
  }

  async getEdit(id: string): Promise<CanvasEdit> {
    validateID("edit id", id);
    return this.get<CanvasEdit>("edit", id);
  }

  async listEditsForCanvas(canvasID: string): Promise<CanvasEdit[]> {
    validateID("canvas id", canvasID);
    return sortAsc((await this.list<CanvasEdit>("edit")).filter((edit) => edit.canvasId === canvasID));
  }

  async saveAgentRun(run: AgentRun): Promise<void> {
    await this.put("agent_run", run.id, run as unknown as Record<string, unknown>, {
      workspaceId: run.workspaceId,
      canvasId: run.canvasId,
      agentId: run.agentId,
      runId: run.runId,
      provider: run.provider,
      status: run.status,
      createdAt: run.createdAt,
      updatedAt: run.updatedAt
    });
  }

  async getAgentRun(id: string): Promise<AgentRun> {
    validateID("agent run id", id);
    return this.get<AgentRun>("agent_run", id);
  }

  async listAgentRuns(): Promise<AgentRun[]> {
    return sortAsc(await this.list<AgentRun>("agent_run"));
  }

  async saveAgentEvent(event: AgentEvent): Promise<boolean> {
    try {
      await this.getAgentEvent(event.id);
      return false;
    } catch (error) {
      if (!(error instanceof AppError) || error.status !== 404) throw error;
    }
    await this.put("agent_event", event.id, event as unknown as Record<string, unknown>, {
      workspaceId: event.workspaceId,
      provider: event.provider,
      status: event.status,
      createdAt: event.occurredAt,
      updatedAt: event.createdAt
    });
    return true;
  }

  async getAgentEvent(id: string): Promise<AgentEvent> {
    validateID("agent event id", id);
    return this.get<AgentEvent>("agent_event", id);
  }

  async listAgentEvents(): Promise<AgentEvent[]> {
    return (await this.list<AgentEvent>("agent_event")).sort((a, b) => parseJSONDate(a.occurredAt) - parseJSONDate(b.occurredAt) || a.id.localeCompare(b.id));
  }

  async saveFeedbackDelivery(delivery: FeedbackDelivery): Promise<void> {
    await this.put("feedback_delivery", delivery.id, delivery as unknown as Record<string, unknown>, {
      workspaceId: delivery.workspaceId,
      canvasId: delivery.canvasId,
      feedbackId: delivery.feedbackId,
      provider: delivery.provider,
      status: delivery.status,
      createdAt: delivery.createdAt,
      updatedAt: delivery.updatedAt
    });
  }

  async getFeedbackDelivery(id: string): Promise<FeedbackDelivery> {
    validateID("feedback delivery id", id);
    return this.get<FeedbackDelivery>("feedback_delivery", id);
  }

  async listFeedbackDeliveries(): Promise<FeedbackDelivery[]> {
    return sortAsc(await this.list<FeedbackDelivery>("feedback_delivery"));
  }

  async retryFeedbackDelivery(id: string, now = nowISO()): Promise<FeedbackDelivery> {
    const delivery = await this.getFeedbackDelivery(id);
    delivery.status = "queued";
    delivery.attempts = 0;
    delete delivery.nextAttemptAt;
    delete delivery.leaseOwner;
    delete delivery.leaseUntil;
    delete delivery.lastError;
    delivery.updatedAt = now;
    await this.saveFeedbackDelivery(delivery);
    return delivery;
  }

  async saveAsset(id: string, contentType: string, body: BodyInit | ReadableStream, workspaceID = ""): Promise<Asset> {
    validateID("asset id", id);
    if (workspaceID) validateID("workspace id", workspaceID);
    const normalizedContentType = contentType || "application/octet-stream";
    const buffer = await new Response(body).arrayBuffer();
    await this.putAssetBody(id, buffer, normalizedContentType);
    const asset: Asset = stripUndefined({
      id,
      workspaceId: workspaceID || undefined,
      contentType: normalizedContentType,
      size: buffer.byteLength,
      createdAt: nowISO()
    });
    await this.put("asset", id, asset as unknown as Record<string, unknown>, { workspaceId: workspaceID, createdAt: asset.createdAt, updatedAt: asset.createdAt });
    return asset;
  }

  async getAsset(id: string): Promise<{ asset: Asset; body: ArrayBuffer }> {
    validateID("asset id", id);
    return { asset: await this.get<Asset>("asset", id), body: await this.getAssetBody(id) };
  }

  async saveViewerLink(link: ViewerLink): Promise<void> {
    await this.put("viewer_link", link.id, link as unknown as Record<string, unknown>, {
      workspaceId: link.workspaceId,
      canvasId: link.canvasId,
      agentId: link.agentId,
      runId: link.runId,
      createdAt: link.createdAt,
      updatedAt: link.updatedAt
    });
  }

  async getViewerLink(id: string): Promise<ViewerLink> {
    validateID("viewer link id", id);
    return this.get<ViewerLink>("viewer_link", id);
  }

  async touchViewerLink(id: string, now = nowISO()): Promise<ViewerLink> {
    const link = await this.getViewerLink(id);
    link.lastUsedAt = now;
    link.updatedAt = now;
    link.useCount = (link.useCount || 0) + 1;
    await this.saveViewerLink(link);
    return link;
  }

  async revokeViewerLink(id: string, now = nowISO()): Promise<ViewerLink> {
    const link = await this.getViewerLink(id);
    link.revokedAt = now;
    link.updatedAt = now;
    await this.saveViewerLink(link);
    return link;
  }

  async saveViewerSession(session: ViewerSession): Promise<void> {
    await this.put("viewer_session", session.id, session as unknown as Record<string, unknown>, {
      workspaceId: session.workspaceId,
      canvasId: session.canvasId,
      agentId: session.agentId,
      runId: session.runId,
      createdAt: session.createdAt,
      updatedAt: session.updatedAt
    });
  }

  async getViewerSession(id: string): Promise<ViewerSession> {
    validateID("viewer session id", id);
    return this.get<ViewerSession>("viewer_session", id);
  }

  async touchViewerSession(id: string, now = nowISO()): Promise<ViewerSession> {
    const session = await this.getViewerSession(id);
    session.lastUsedAt = now;
    session.updatedAt = now;
    session.useCount = (session.useCount || 0) + 1;
    await this.saveViewerSession(session);
    return session;
  }
}

export class MemoryStore extends BaseStore {
  private records = new Map<string, unknown>();
  private assets = new Map<string, ArrayBuffer>();

  protected async put<T extends Record<string, unknown>>(kind: Kind, id: string, value: T): Promise<void> {
    this.records.set(`${kind}:${id}`, structuredClone(value));
  }

  protected async get<T>(kind: Kind, id: string): Promise<T> {
    const value = this.records.get(`${kind}:${id}`);
    if (value === undefined) notFound();
    return structuredClone(value) as T;
  }

  protected async list<T>(kind: Kind): Promise<T[]> {
    const prefix = `${kind}:`;
    return [...this.records.entries()].filter(([key]) => key.startsWith(prefix)).map(([, value]) => structuredClone(value) as T);
  }

  protected async putAssetBody(id: string, body: ArrayBuffer): Promise<void> {
    this.assets.set(id, body.slice(0));
  }

  protected async getAssetBody(id: string): Promise<ArrayBuffer> {
    const body = this.assets.get(id);
    if (!body) notFound();
    return body.slice(0);
  }
}

export class D1Store extends BaseStore {
  private ensured?: Promise<void>;

  constructor(
    private db: D1Database,
    private bucket: R2Bucket
  ) {
    super();
  }

  private ensure(): Promise<void> {
    this.ensured ||= this.db
      .batch(
        [
          `CREATE TABLE IF NOT EXISTS records (
            kind TEXT NOT NULL,
            id TEXT NOT NULL,
            workspace_id TEXT NOT NULL DEFAULT '',
            canvas_id TEXT NOT NULL DEFAULT '',
            agent_id TEXT NOT NULL DEFAULT '',
            run_id TEXT NOT NULL DEFAULT '',
            provider TEXT NOT NULL DEFAULT '',
            status TEXT NOT NULL DEFAULT '',
            feedback_id TEXT NOT NULL DEFAULT '',
            created_at TEXT NOT NULL DEFAULT '',
            updated_at TEXT NOT NULL DEFAULT '',
            payload_json TEXT NOT NULL,
            PRIMARY KEY (kind, id)
          )`,
          "CREATE INDEX IF NOT EXISTS records_kind_created_idx ON records(kind, created_at, id)",
          "CREATE INDEX IF NOT EXISTS records_kind_canvas_idx ON records(kind, canvas_id, created_at, id)",
          "CREATE INDEX IF NOT EXISTS records_kind_workspace_idx ON records(kind, workspace_id, created_at, id)",
          "CREATE INDEX IF NOT EXISTS records_kind_feedback_idx ON records(kind, feedback_id, created_at, id)"
        ].map((statement) => this.db.prepare(statement))
      )
      .then(() => undefined);
    return this.ensured;
  }

  protected async put<T extends Record<string, unknown>>(kind: Kind, id: string, value: T, meta: RecordMeta = {}): Promise<void> {
    await this.ensure();
    await this.db
      .prepare(
        `INSERT OR REPLACE INTO records
        (kind, id, workspace_id, canvas_id, agent_id, run_id, provider, status, feedback_id, created_at, updated_at, payload_json)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
      )
      .bind(
        kind,
        id,
        meta.workspaceId || "",
        meta.canvasId || "",
        meta.agentId || "",
        meta.runId || "",
        meta.provider || "",
        meta.status || "",
        meta.feedbackId || "",
        meta.createdAt || "",
        meta.updatedAt || meta.createdAt || "",
        JSON.stringify(stripUndefined(value))
      )
      .run();
  }

  protected async get<T>(kind: Kind, id: string): Promise<T> {
    await this.ensure();
    const row = await this.db.prepare("SELECT payload_json FROM records WHERE kind = ? AND id = ?").bind(kind, id).first<{ payload_json: string }>();
    if (!row) notFound();
    return JSON.parse(row.payload_json) as T;
  }

  protected async list<T>(kind: Kind): Promise<T[]> {
    await this.ensure();
    const result = await this.db.prepare("SELECT payload_json FROM records WHERE kind = ? ORDER BY created_at, id").bind(kind).all<{ payload_json: string }>();
    return (result.results || []).map((row) => JSON.parse(row.payload_json) as T);
  }

  protected async putAssetBody(id: string, body: ArrayBuffer, contentType: string): Promise<void> {
    await this.bucket.put(assetKey(id), body, { httpMetadata: { contentType } });
  }

  protected async getAssetBody(id: string): Promise<ArrayBuffer> {
    const object = await this.bucket.get(assetKey(id));
    if (!object) notFound();
    return object.arrayBuffer();
  }
}

export function storeFromEnv(env: HubEnv): Store {
  return new D1Store(env.DB, env.ASSETS_BUCKET);
}

function metaFromCanvas(canvas: Canvas): RecordMeta {
  return {
    workspaceId: canvas.workspaceId,
    canvasId: canvas.id,
    agentId: canvas.agentId,
    runId: canvas.runId,
    status: canvas.status,
    createdAt: canvas.createdAt,
    updatedAt: canvas.updatedAt
  };
}

function sortAsc<T extends { id: string; createdAt?: string }>(items: T[]): T[] {
  return items.sort(compareAsc);
}

function compareAsc(a: { id: string; createdAt?: string }, b: { id: string; createdAt?: string }): number {
  return parseJSONDate(a.createdAt) - parseJSONDate(b.createdAt) || a.id.localeCompare(b.id);
}

function compareDesc(a: { id: string; createdAt?: string }, b: { id: string; createdAt?: string }): number {
  return parseJSONDate(b.createdAt) - parseJSONDate(a.createdAt) || b.id.localeCompare(a.id);
}

function assetKey(id: string): string {
  return `assets/${id}`;
}

function addCanvasAssetRefs(ids: Set<string>, canvas: Canvas): void {
  for (const block of canvas.blocks) addBlockAssetRefs(ids, block);
}

function addBlockAssetRefs(ids: Set<string>, block: { assetId?: string; thumbnailAssetId?: string }): void {
  if (block.assetId) ids.add(block.assetId);
  if (block.thumbnailAssetId) ids.add(block.thumbnailAssetId);
}
