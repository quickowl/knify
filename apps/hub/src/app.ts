import type {
  AgentEvent,
  AgentRun,
  AuthContext,
  Canvas,
  CanvasEdit,
  CanvasImportRequest,
  CanvasLogEvent,
  CreateCanvasSnapshotRequest,
  Feedback,
  FeedbackDelivery,
  FeedbackTarget,
  HubEnv,
  ObserverDecision,
  ObserverIssue,
  ViewerLink,
  ViewerLinkCheck,
  ViewerLinkExchangeResponse,
  ViewerLinkRequest,
  ViewerSession
} from "./types";
import type { EventBus } from "./events";
import type { Store } from "./store";
import { DurableEventBus } from "./events";
import { storeFromEnv } from "./store";
import {
  SNAPSHOT_DYNAMIC_COMPLETE,
  SNAPSHOT_MANUAL,
  SNAPSHOT_STATIC_OVERWRITE,
  canvasReferencesAsset,
  normalizeAgentEvent,
  normalizeAgentRun,
  normalizeCanvas,
  normalizeCanvasEdit,
  normalizeCanvasLogEvent,
  normalizeFeedback,
  normalizeFeedbackDelivery,
  normalizeViewerLinkRequest,
  opRequiresBlockKind,
  sanitizeCanvasForViewer,
  validAgentProvider,
  validBlockKind
} from "./validation";
import {
  AppError,
  addSeconds,
  badRequest,
  errorResponse,
  firstNonEmpty,
  forbidden,
  hmacSHA256Hex,
  jsonResponse,
  newID,
  notFound,
  nowISO,
  parseBool,
  parseJSONDate,
  sha256Hex,
  stampWorkspace,
  stripUndefined,
  validateID,
  workspaceVisible
} from "./utils";

const defaultViewerLinkBaseURL = "https://knify.link";
const defaultCORSOrigins = [
  "http://localhost:5173",
  "http://localhost:5174",
  "http://localhost:5175",
  "http://localhost:5176",
  "http://localhost:5177",
  "http://localhost:5178",
  "http://localhost:5179",
  "http://127.0.0.1:5173",
  "http://127.0.0.1:5174",
  "http://127.0.0.1:5175",
  "http://127.0.0.1:5176",
  "http://127.0.0.1:5177",
  "http://127.0.0.1:5178",
  "http://127.0.0.1:5179",
  "http://localhost:4173",
  "http://127.0.0.1:4173",
  "https://knify.dev",
  "https://knify.link"
];

export interface AppOptions {
  store?: Store;
  bus?: EventBus;
  now?: () => string;
  token?: string;
  apiKeyVerifier?: (key: string) => Promise<{ keyId?: string; workspaceId: string }>;
}

export function createApp(env: HubEnv, options: AppOptions = {}) {
  const store = options.store || storeFromEnv(env);
  const bus = options.bus || new DurableEventBus(env);
  return {
    fetch: (request: Request, executionCtx?: ExecutionContext) => new HubApp(env, store, bus, options, executionCtx).fetch(request),
    queue: async (batch: MessageBatch<{ deliveryId: string }>, executionCtx?: ExecutionContext) => {
      const app = new HubApp(env, store, bus, options, executionCtx);
      await Promise.all(batch.messages.map((message) => app.processDelivery(message.body.deliveryId)));
    },
    store,
    bus
  };
}

class HubApp {
  private token: string;
  private now: () => string;

  constructor(
    private env: HubEnv,
    private store: Store,
    private bus: EventBus,
    options: AppOptions,
    private ctx?: ExecutionContext
  ) {
    const configuredToken = firstNonEmpty(env.AGENTCANVAS_TOKEN as string, env.CANVAS_HUB_TOKEN as string, env.HUB_TOKEN as string);
    const allowDefaultToken = parseBool(env.AGENTCANVAS_ALLOW_DEV_TOKEN as string | undefined) ?? false;
    this.token = options.token || configuredToken || (allowDefaultToken ? "agentcanvas-dev-token" : "");
    this.now = options.now || nowISO;
    this.apiKeyVerifier = options.apiKeyVerifier;
  }

  private apiKeyVerifier?: (key: string) => Promise<{ keyId?: string; workspaceId: string }>;

  async fetch(request: Request): Promise<Response> {
    try {
      const cors = this.handleCORS(request);
      if (cors) return cors;

      const url = new URL(request.url);
      if (url.pathname === "/v1/healthz") return this.withCORS(request, jsonResponse({ ok: true, service: "agentcanvas-hub" }));
      if (this.isPublicViewerExchange(url.pathname)) return this.withCORS(request, await this.handleViewerLinkExchange(request, pathParts(url.pathname)[2]));

      const auth = await this.authorize(request);
      if (auth) return this.withCORS(request, await this.handleAuthorized(request, auth));

      const session = await this.viewerSessionFromRequest(request);
      if (session) return this.withCORS(request, await this.handleViewerSession(request, session));

      return this.withCORS(request, jsonResponse({ error: "missing or invalid bearer token" }, 401));
    } catch (error) {
      return this.withCORS(request, errorResponse(error));
    }
  }

  private async handleAuthorized(request: Request, auth: AuthContext): Promise<Response> {
    const url = new URL(request.url);
    const path = url.pathname;
    if (path === "/v1/canvases") return this.handleCanvases(request, auth);
    if (path === "/v1/canvases/import") return this.handleCanvasImport(request, auth);
    if (path.startsWith("/v1/canvases/")) return this.handleCanvasByID(request, auth);
    if (path === "/v1/viewer-links/preflight") return this.handleViewerLinksPreflight(request, auth);
    if (path === "/v1/viewer-links") return this.handleViewerLinks(request, auth);
    if (path.startsWith("/v1/viewer-links/")) return this.handleViewerLinkByID(request);
    if (path === "/v1/agent-runs") return this.handleAgentRuns(request, auth);
    if (path.startsWith("/v1/agent-runs/")) return this.handleAgentRunByID(request, auth);
    if (path === "/v1/agent-events/evaluate") return this.handleAgentEventsEvaluate(request, auth);
    if (path === "/v1/agent-events") return this.handleAgentEvents(request, auth);
    if (path === "/v1/feedback-deliveries") return this.handleFeedbackDeliveries(request, auth);
    if (path.startsWith("/v1/feedback-deliveries/")) return this.handleFeedbackDeliveryByID(request, auth);
    if (path === "/v1/assets") return this.handleAssets(request, auth);
    if (path.startsWith("/v1/assets/")) return this.handleAsset(request, auth);
    if (path === "/v1/events") return this.handleEvents(request, auth);
    return jsonResponse({ error: "not found" }, 404);
  }

  private async handleCanvases(request: Request, auth: AuthContext): Promise<Response> {
    if (request.method === "POST") {
      const body = await readJSON<Partial<Canvas>>(request, canvasKeys);
      stampWorkspace(body, auth.workspaceId);
      const canvas = normalizeCanvas(body, this.now());
      let eventType = "canvas.created";
      try {
        const existing = await this.store.getCanvas(canvas.id);
        if (!workspaceVisible(existing.workspaceId, auth.workspaceId)) forbidden();
        eventType = "canvas.updated";
        const snapshot = await this.store.createCanvasSnapshot(canvas.id, {
          reason: SNAPSHOT_STATIC_OVERWRITE,
          source: "post:/v1/canvases",
          label: "Before static canvas overwrite"
        }, this.now());
        await this.publish({ type: "canvas.snapshot.created", canvasId: snapshot.canvasId, data: snapshot });
      } catch (error) {
        if (!(error instanceof AppError) || error.status !== 404) throw error;
      }
      await this.store.saveCanvas(canvas);
      await this.publish({ type: eventType, canvasId: canvas.id, data: canvas });
      return jsonResponse(canvas, 201);
    }
    if (request.method === "GET") {
      const url = new URL(request.url);
      let canvases = (await this.store.listCanvases()).filter((canvas) => workspaceVisible(canvas.workspaceId, auth.workspaceId));
      const agentID = firstNonEmpty(url.searchParams.get("agentId") || "", url.searchParams.get("agent_id") || "");
      const runID = firstNonEmpty(url.searchParams.get("runId") || "", url.searchParams.get("run_id") || "");
      const status = url.searchParams.get("status") || "";
      canvases = canvases.filter((canvas) => (!agentID || canvas.agentId === agentID) && (!runID || canvas.runId === runID) && (!status || canvas.status === status));
      return jsonResponse(canvases);
    }
    return methodNotAllowed();
  }

  private async handleCanvasByID(request: Request, auth: AuthContext): Promise<Response> {
    const parts = pathParts(new URL(request.url).pathname);
    const canvasID = pathID(parts[2]);
    if (parts.length === 3) {
      if (request.method !== "GET") return methodNotAllowed();
      return jsonResponse(await this.getCanvasForAuth(canvasID, auth));
    }
    const leaf = parts[3];
    if (parts.length === 4 && leaf === "feedback") {
      if (request.method === "GET") {
        await this.getCanvasForAuth(canvasID, auth);
        return jsonResponse(await this.store.listFeedback(canvasID));
      }
      if (request.method !== "POST") return methodNotAllowed();
      return this.handleFeedback(request, canvasID, auth);
    }
    if (parts.length === 4 && leaf === "events") return this.handleCanvasEvents(request, canvasID, auth);
    if (parts.length === 4 && leaf === "snapshots") return this.handleCanvasSnapshots(request, canvasID, auth);
    if (parts.length === 4 && leaf === "export") return this.handleCanvasExport(request, canvasID, auth);
    if (parts.length === 4 && leaf === "edits") return this.handleCanvasEdits(request, canvasID, auth);
    if (parts.length === 5 && leaf === "snapshots") {
      if (request.method !== "GET") return methodNotAllowed();
      await this.getCanvasForAuth(canvasID, auth);
      return jsonResponse(await this.store.getCanvasSnapshot(canvasID, pathID(parts[4])));
    }
    if (parts.length === 6 && leaf === "snapshots" && parts[5] === "restore") {
      if (request.method !== "POST") return methodNotAllowed();
      await this.getCanvasForAuth(canvasID, auth);
      const response = await this.store.restoreCanvasSnapshot(canvasID, pathID(parts[4]), this.now());
      await this.publish({ type: "canvas.snapshot.created", canvasId: canvasID, data: response.checkpoint });
      await this.publish({ type: "canvas.restored", canvasId: canvasID, data: response });
      await this.publish({ type: "canvas.updated", canvasId: canvasID, data: response.canvas });
      return jsonResponse(response);
    }
    return jsonResponse({ error: "not found" }, 404);
  }

  private async handleCanvasEvents(request: Request, canvasID: string, auth: AuthContext): Promise<Response> {
    if (request.method === "POST") {
      const body = await readJSON<Partial<CanvasLogEvent>>(request, canvasEventKeys);
      const event = normalizeCanvasLogEvent(body, canvasID, this.now());
      stampWorkspace(event, auth.workspaceId);
      try {
        const existing = await this.store.getCanvas(canvasID);
        if (!workspaceVisible(existing.workspaceId, auth.workspaceId)) forbidden();
      } catch (error) {
        if (!(error instanceof AppError) || error.status !== 404) throw error;
      }
      const { canvas, created, event: storedEvent } = await this.store.applyCanvasLogEvent(event, this.now());
      if (created) {
        await this.publish({ type: "canvas.event.created", canvasId: canvasID, data: storedEvent });
        await this.publish({ type: "canvas.updated", canvasId: canvasID, data: canvas });
        if (storedEvent.type === "canvas.completed") {
          const snapshot = await this.store.createCanvasSnapshot(canvasID, {
            reason: SNAPSHOT_DYNAMIC_COMPLETE,
            source: "canvas-event",
            label: "Completed dynamic canvas",
            sourceEventId: storedEvent.id
          }, this.now());
          await this.publish({ type: "canvas.snapshot.created", canvasId: canvasID, data: snapshot });
        }
      }
      return jsonResponse(canvas, 201);
    }
    if (request.method === "GET") {
      await this.getCanvasForAuth(canvasID, auth);
      return jsonResponse(await this.store.listCanvasLogEvents(canvasID));
    }
    return methodNotAllowed();
  }

  private async handleCanvasSnapshots(request: Request, canvasID: string, auth: AuthContext): Promise<Response> {
    if (request.method === "GET") {
      await this.getCanvasForAuth(canvasID, auth);
      return jsonResponse(await this.store.listCanvasSnapshots(canvasID));
    }
    if (request.method === "POST") {
      await this.getCanvasForAuth(canvasID, auth);
      const body = request.headers.get("Content-Length") === "0" ? {} : await readOptionalJSON<CreateCanvasSnapshotRequest>(request, snapshotKeys);
      body.reason = body.reason || SNAPSHOT_MANUAL;
      body.source = body.source || "manual";
      const snapshot = await this.store.createCanvasSnapshot(canvasID, body, this.now());
      await this.publish({ type: "canvas.snapshot.created", canvasId: canvasID, data: snapshot });
      return jsonResponse(snapshot, 201);
    }
    return methodNotAllowed();
  }

  private async handleCanvasExport(request: Request, canvasID: string, auth: AuthContext): Promise<Response> {
    if (request.method !== "GET") return methodNotAllowed();
    await this.getCanvasForAuth(canvasID, auth);
    return jsonResponse(await this.store.exportCanvasBundle(canvasID, this.now()));
  }

  private async handleCanvasImport(request: Request, auth: AuthContext): Promise<Response> {
    if (request.method !== "POST") return methodNotAllowed();
    const body = await readJSON<CanvasImportRequest>(request, canvasImportKeys);
    stampWorkspace(body.bundle.canvas, auth.workspaceId);
    for (const run of body.bundle.agentRuns || []) stampWorkspace(run, auth.workspaceId);
    const result = await this.store.importCanvasBundle(body, this.now());
    if (result.checkpoint) await this.publish({ type: "canvas.snapshot.created", canvasId: result.canvas.id, data: result.checkpoint });
    await this.publish({ type: "canvas.imported", canvasId: result.canvas.id, data: result });
    await this.publish({ type: result.checkpoint ? "canvas.updated" : "canvas.created", canvasId: result.canvas.id, data: result.canvas });
    return jsonResponse(result, 201);
  }

  private async handleCanvasEdits(request: Request, canvasID: string, auth: AuthContext): Promise<Response> {
    if (request.method === "POST") {
      const body = await readJSON<Partial<CanvasEdit>>(request, editKeys);
      body.canvasId = body.canvasId || canvasID;
      const edit = normalizeCanvasEdit(body, this.now());
      const canvas = await this.getCanvasForAuth(canvasID, auth);
      if (edit.expectedVersion !== canvas.version) throw new AppError(409, `conflict: expected canvas version ${edit.expectedVersion}, got ${canvas.version}`, "conflict");
      const kinds = new Map(canvas.blocks.map((block) => [block.id, block.kind]));
      for (const [index, op] of edit.ops.entries()) {
        if (op.op === "submit") continue;
        const blockID = String(op.blockId || "");
        const kind = kinds.get(blockID);
        if (!kind) badRequest(`op at index ${index} references unknown blockId ${JSON.stringify(blockID)}`);
        const required = opRequiresBlockKind(String(op.op));
        if (required && kind !== required) badRequest(`op ${JSON.stringify(op.op)} requires block kind ${JSON.stringify(required)}, but block ${JSON.stringify(blockID)} has kind ${JSON.stringify(kind)}`);
      }
      await this.store.saveEdit(edit);
      await this.publish({ type: "canvas.edit.submitted", canvasId: canvasID, data: edit });
      return jsonResponse(edit, 201);
    }
    if (request.method === "GET") {
      await this.getCanvasForAuth(canvasID, auth);
      const items = await this.store.listEditsForCanvas(canvasID);
      return jsonResponse({ items, count: items.length });
    }
    return methodNotAllowed();
  }

  private async handleFeedback(request: Request, canvasID: string, auth: AuthContext): Promise<Response> {
    const canvas = await this.getCanvasForAuth(canvasID, auth);
    const body = await readJSON<Partial<Feedback>>(request, feedbackKeys);
    const { target, run } = await this.resolveFeedbackTarget(canvas);
    const feedback = normalizeFeedback({ ...body, workspaceId: canvas.workspaceId }, canvas.id, !!target, this.now());
    await this.store.saveFeedback(feedback);
    await this.publish({ type: "feedback.created", canvasId: canvas.id, feedbackId: feedback.id, data: feedback });
    if (!target) return jsonResponse(feedback, 201);
    const delivery = normalizeFeedbackDelivery({
      workspaceId: canvas.workspaceId,
      feedbackId: feedback.id,
      canvasId: canvas.id,
      agentRunId: run?.id,
      provider: target.provider,
      mode: target.mode,
      targetId: target.id,
      target,
      status: "queued",
      maxAttempts: target.provider === "webhook" ? 1 : targetMaxAttempts(target) || 3
    }, this.now());
    await this.store.saveFeedbackDelivery(delivery);
    await this.publish({ type: "feedback.delivery.queued", canvasId: canvas.id, feedbackId: feedback.id, data: delivery });
    if (edgeDeliverable(delivery.provider)) {
      if (this.env.FEEDBACK_QUEUE) this.ctx?.waitUntil(this.env.FEEDBACK_QUEUE.send({ deliveryId: delivery.id }));
      else this.ctx?.waitUntil(this.processDelivery(delivery.id));
    }
    return jsonResponse(feedback, 202);
  }

  private async handleAssets(request: Request, auth: AuthContext): Promise<Response> {
    if (request.method !== "POST") return methodNotAllowed();
    const id = new URL(request.url).searchParams.get("id") || newID("asset");
    const asset = await this.store.saveAsset(id, request.headers.get("Content-Type") || "application/octet-stream", request.body || new Uint8Array(), auth.workspaceId || "");
    return jsonResponse({ assetId: asset.id, id: asset.id, contentType: asset.contentType, size: asset.size, url: `/v1/assets/${asset.id}`, createdAt: asset.createdAt }, 201);
  }

  private async handleAsset(request: Request, auth: AuthContext): Promise<Response> {
    const id = pathID(pathParts(new URL(request.url).pathname)[2]);
    if (request.method === "POST") {
      const asset = await this.store.saveAsset(id, request.headers.get("Content-Type") || "application/octet-stream", request.body || new Uint8Array(), auth.workspaceId || "");
      return jsonResponse(asset, 201);
    }
    if (request.method === "GET") {
      const { asset, body } = await this.store.getAsset(id);
      if (!workspaceVisible(asset.workspaceId, auth.workspaceId)) forbidden();
      return new Response(body, {
        status: 200,
        headers: {
          "Content-Type": asset.contentType,
          "Content-Length": String(asset.size)
        }
      });
    }
    return methodNotAllowed();
  }

  private async handleAgentRuns(request: Request, auth: AuthContext): Promise<Response> {
    if (request.method === "POST") {
      const body = await readJSON<Partial<AgentRun>>(request, agentRunKeys);
      stampWorkspace(body, auth.workspaceId);
      let eventType = "agent.run.created";
      if (body.id) {
        try {
          const existing = await this.store.getAgentRun(body.id);
          if (!workspaceVisible(existing.workspaceId, auth.workspaceId)) forbidden();
          eventType = "agent.run.updated";
        } catch (error) {
          if (!(error instanceof AppError) || error.status !== 404) throw error;
        }
      }
      const run = normalizeAgentRun(body, this.now());
      await this.store.saveAgentRun(run);
      await this.publish({ type: eventType, canvasId: run.canvasId, data: run });
      return jsonResponse(run, 201);
    }
    if (request.method === "GET") {
      const url = new URL(request.url);
      let runs = (await this.store.listAgentRuns()).filter((run) => workspaceVisible(run.workspaceId, auth.workspaceId));
      const provider = url.searchParams.get("provider") || "";
      const agentID = firstNonEmpty(url.searchParams.get("agentId") || "", url.searchParams.get("agent_id") || "");
      const status = url.searchParams.get("status") || "";
      runs = runs.filter((run) => (!provider || run.provider === provider) && (!agentID || run.agentId === agentID) && (!status || run.status === status));
      return jsonResponse(runs);
    }
    return methodNotAllowed();
  }

  private async handleAgentRunByID(request: Request, auth: AuthContext): Promise<Response> {
    const parts = pathParts(new URL(request.url).pathname);
    const id = pathID(parts[2]);
    if (parts.length === 3) {
      if (request.method !== "GET") return methodNotAllowed();
      return jsonResponse(await this.getAgentRunForAuth(id, auth));
    }
    if (parts.length === 4 && parts[3] === "feedback") {
      if (request.method !== "POST") return methodNotAllowed();
      const run = await this.getAgentRunForAuth(id, auth);
      const body = await readJSON<Partial<Feedback>>(request, feedbackKeys);
      const canvasID = body.canvasId || run.canvasId || "";
      const canvas = await this.getCanvasForAuth(canvasID, auth);
      return this.submitFeedbackToTarget(request, canvas, body, run.feedbackTarget, run);
    }
    return jsonResponse({ error: "not found" }, 404);
  }

  private async submitFeedbackToTarget(request: Request, canvas: Canvas, body: Partial<Feedback>, target: FeedbackTarget, run?: AgentRun): Promise<Response> {
    const feedback = normalizeFeedback({ ...body, workspaceId: canvas.workspaceId }, canvas.id, true, this.now());
    await this.store.saveFeedback(feedback);
    await this.publish({ type: "feedback.created", canvasId: canvas.id, feedbackId: feedback.id, data: feedback });
    const delivery = normalizeFeedbackDelivery({
      workspaceId: canvas.workspaceId,
      feedbackId: feedback.id,
      canvasId: canvas.id,
      agentRunId: run?.id,
      provider: target.provider,
      mode: target.mode,
      targetId: target.id || run?.id,
      target,
      status: "queued",
      maxAttempts: target.provider === "webhook" ? 1 : targetMaxAttempts(target) || 3
    }, this.now());
    await this.store.saveFeedbackDelivery(delivery);
    await this.publish({ type: "feedback.delivery.queued", canvasId: canvas.id, feedbackId: feedback.id, data: delivery });
    if (edgeDeliverable(delivery.provider)) this.ctx?.waitUntil(this.processDelivery(delivery.id));
    return jsonResponse(feedback, 202);
  }

  private async handleAgentEvents(request: Request, auth: AuthContext): Promise<Response> {
    if (request.method !== "POST") return methodNotAllowed();
    const event = await this.normalizeStampedAgentEvent(request, auth);
    await this.recordAgentEvent(event);
    return jsonResponse(event, 201);
  }

  private async handleAgentEventsEvaluate(request: Request, auth: AuthContext): Promise<Response> {
    if (request.method !== "POST") return methodNotAllowed();
    const event = await this.normalizeStampedAgentEvent(request, auth);
    const created = await this.recordAgentEvent(event);
    const decision = await this.evaluateObserver(event, created);
    await this.publish({ type: "agent.observer.decision", canvasId: decision.canvasId, data: decision });
    return jsonResponse({ event, created, decision }, 201);
  }

  private async handleFeedbackDeliveries(request: Request, auth: AuthContext): Promise<Response> {
    if (request.method !== "GET") return methodNotAllowed();
    const url = new URL(request.url);
    let deliveries = (await this.store.listFeedbackDeliveries()).filter((delivery) => workspaceVisible(delivery.workspaceId, auth.workspaceId));
    const status = url.searchParams.get("status") || "";
    const canvasID = firstNonEmpty(url.searchParams.get("canvasId") || "", url.searchParams.get("canvas_id") || "");
    const feedbackID = firstNonEmpty(url.searchParams.get("feedbackId") || "", url.searchParams.get("feedback_id") || "");
    deliveries = deliveries.filter((delivery) => (!status || delivery.status === status) && (!canvasID || delivery.canvasId === canvasID) && (!feedbackID || delivery.feedbackId === feedbackID));
    return jsonResponse(deliveries);
  }

  private async handleFeedbackDeliveryByID(request: Request, auth: AuthContext): Promise<Response> {
    const parts = pathParts(new URL(request.url).pathname);
    const id = pathID(parts[2]);
    if (parts.length === 3) {
      if (request.method !== "GET") return methodNotAllowed();
      const delivery = await this.store.getFeedbackDelivery(id);
      if (!workspaceVisible(delivery.workspaceId, auth.workspaceId)) forbidden();
      return jsonResponse(delivery);
    }
    if (parts.length === 4 && parts[3] === "retry") {
      if (request.method !== "POST") return methodNotAllowed();
      const current = await this.store.getFeedbackDelivery(id);
      if (!workspaceVisible(current.workspaceId, auth.workspaceId)) forbidden();
      const delivery = await this.store.retryFeedbackDelivery(id, this.now());
      await this.publish({ type: "feedback.delivery.queued", canvasId: delivery.canvasId, feedbackId: delivery.feedbackId, data: delivery });
      if (edgeDeliverable(delivery.provider)) this.ctx?.waitUntil(this.processDelivery(delivery.id));
      return jsonResponse(delivery, 202);
    }
    return jsonResponse({ error: "not found" }, 404);
  }

  private async handleEvents(request: Request, auth: AuthContext): Promise<Response> {
    if (request.method !== "GET") return methodNotAllowed();
    return this.bus.stream(auth);
  }

  private async handleViewerLinksPreflight(request: Request, auth: AuthContext): Promise<Response> {
    if (request.method !== "POST") return methodNotAllowed();
    const req = await readJSON<ViewerLinkRequest>(request, viewerLinkRequestKeys);
    const response = await this.preflightViewerLink(req, auth);
    return jsonResponse(response, response.status === "blocked" ? 422 : 200);
  }

  private async handleViewerLinks(request: Request, auth: AuthContext): Promise<Response> {
    if (request.method !== "POST") return methodNotAllowed();
    const raw = await readJSON<ViewerLinkRequest>(request, viewerLinkRequestKeys);
    const preflight = await this.preflightViewerLink(raw, auth);
    if (preflight.status === "blocked") return jsonResponse(preflight, 422);
    const req = normalizeViewerLinkRequest(raw, this.linkBaseURL(), this.now());
    const canvas = await this.getCanvasForAuth(req.canvasId, auth);
    const secret = newViewerSecret();
    const link: ViewerLink = {
      id: newID("viewer-link"),
      workspaceId: canvas.workspaceId,
      kind: req.kind,
      scope: "canvas",
      canvasId: req.canvasId,
      runId: req.runId || undefined,
      agentId: req.agentId || undefined,
      secretHash: await sha256Hex(secret),
      capabilities: req.capabilities,
      createdAt: this.now(),
      updatedAt: this.now(),
      expiresAt: req.expiresAt,
      useCount: 0
    };
    await this.store.saveViewerLink(stripUndefined(link));
    const code = viewerCode(link.id, secret);
    return jsonResponse({
      id: link.id,
      kind: link.kind,
      scope: link.scope,
      canvasId: link.canvasId,
      runId: link.runId,
      agentId: link.agentId,
      capabilities: link.capabilities,
      code,
      url: viewerLinkURL(req.linkBaseUrl, code),
      expiresAt: link.expiresAt
    }, 201);
  }

  private async handleViewerLinkByID(request: Request): Promise<Response> {
    const parts = pathParts(new URL(request.url).pathname);
    const id = pathID(parts[2]);
    if (parts.length === 4 && parts[3] === "self-test") return this.handleViewerLinkSelfTest(request, id);
    if (parts.length === 4 && parts[3] === "revoke") {
      if (request.method !== "POST") return methodNotAllowed();
      return jsonResponse(await this.store.revokeViewerLink(id, this.now()));
    }
    return jsonResponse({ error: "not found" }, 404);
  }

  private async handleViewerLinkSelfTest(request: Request, id: string): Promise<Response> {
    if (request.method !== "POST") return methodNotAllowed();
    const body = await readJSON<{ code?: string; secret?: string }>(request, new Set(["code", "secret"]));
    const secret = await secretFromRequest(id, body);
    const checks: ViewerLinkCheck[] = [];
    let canvasID = "";
    try {
      const link = await this.validateViewerLinkSecret(id, secret);
      canvasID = link.canvasId;
      const canvas = await this.store.getCanvas(link.canvasId);
      validateScopedCanvasAccess(link.canvasId, canvas.id);
      checks.push({ code: "exchange", status: "pass", message: "viewer link secret is valid" });
      checks.push({ code: "canvas_fetch", status: "pass", message: "scoped viewer can fetch the target canvas" });
      return jsonResponse({ status: "ready", checks, canvasId: canvasID });
    } catch (error) {
      checks.push({ code: "exchange", status: "fail", message: error instanceof Error ? error.message : String(error) });
      return jsonResponse({ status: "blocked", checks, canvasId: canvasID }, 422);
    }
  }

  private async handleViewerLinkExchange(request: Request, id: string): Promise<Response> {
    if (request.method !== "POST") return methodNotAllowed();
    const body = await readJSON<{ code?: string; secret?: string }>(request, new Set(["code", "secret"]));
    const secret = await secretFromRequest(id, body);
    const response = await this.exchangeViewerLink(id, secret);
    return jsonResponse(response, 200, viewerHeaders());
  }

  private async handleViewerSession(request: Request, session: ViewerSession): Promise<Response> {
    const path = new URL(request.url).pathname;
    if (path === "/v1/canvases") {
      if (request.method !== "GET") return methodNotAllowed();
      return jsonResponse([sanitizeCanvasForViewer(await this.store.getCanvas(session.canvasId))], 200, viewerHeaders());
    }
    if (path === "/v1/events") {
      if (!hasCapability(session.capabilities, "canvas.live")) forbidden("viewer session cannot stream live events");
      if (request.method !== "GET") return methodNotAllowed();
      return this.bus.stream({ internal: false, workspaceId: session.workspaceId }, session);
    }
    if (path === "/v1/feedback-deliveries") {
      if (request.method !== "GET") return methodNotAllowed();
      return jsonResponse([], 200, viewerHeaders());
    }
    if (path.startsWith("/v1/assets/")) {
      if (!hasCapability(session.capabilities, "asset.read")) forbidden("viewer session cannot access assets");
      const assetID = pathID(pathParts(path)[2]);
      const canvas = await this.store.getCanvas(session.canvasId);
      if (!canvasReferencesAsset(canvas, assetID)) forbidden("viewer session cannot access this asset");
      return this.handleAsset(request, { internal: false, workspaceId: session.workspaceId });
    }
    if (path.startsWith("/v1/canvases/")) {
      const parts = pathParts(path);
      const canvasID = pathID(parts[2]);
      if (canvasID !== session.canvasId) forbidden("viewer session cannot access this canvas");
      if (parts.length === 3) {
        if (request.method !== "GET") return methodNotAllowed();
        return jsonResponse(sanitizeCanvasForViewer(await this.store.getCanvas(canvasID)), 200, viewerHeaders());
      }
      if (parts.length === 4 && parts[3] === "feedback") {
        if (!hasCapability(session.capabilities, "feedback.submit")) forbidden("viewer session cannot submit feedback");
        if (request.method !== "POST") return methodNotAllowed();
        return this.handleFeedback(request, canvasID, { internal: false, workspaceId: session.workspaceId });
      }
      forbidden("viewer session cannot access this canvas resource");
    }
    forbidden("viewer session is scoped to one canvas");
  }

  async processDelivery(id: string): Promise<void> {
    let delivery = await this.store.getFeedbackDelivery(id);
    if (delivery.status === "delivered" || delivery.status === "dead_lettered") return;
    const now = this.now();
    delivery.attempts += 1;
    delivery.status = delivery.attempts > 1 ? "retrying" : delivery.status;
    delivery.updatedAt = now;
    await this.store.saveFeedbackDelivery(delivery);
    try {
      const canvas = await this.store.getCanvas(delivery.canvasId);
      const feedback = await this.store.getFeedback(delivery.feedbackId);
      const run = delivery.agentRunId ? await maybe(() => this.store.getAgentRun(delivery.agentRunId!)) : undefined;
      const receipt = await this.deliverFeedback(delivery, canvas, feedback, run);
      delivery = { ...delivery, status: "delivered", receipt, lastError: undefined, nextAttemptAt: undefined, leaseOwner: undefined, leaseUntil: undefined, updatedAt: this.now() };
      feedback.deliveryStatus = "delivered";
      await this.store.saveFeedbackDelivery(stripUndefined(delivery));
      await this.store.saveFeedback(feedback);
      await this.publish({ type: "feedback.delivered", canvasId: delivery.canvasId, feedbackId: delivery.feedbackId, data: delivery });
    } catch (error) {
      delivery.lastError = error instanceof Error ? error.message : String(error);
      delivery.updatedAt = this.now();
      if (delivery.attempts >= delivery.maxAttempts) {
        delivery.status = "dead_lettered";
        delete delivery.nextAttemptAt;
        const feedback = await maybe(() => this.store.getFeedback(delivery.feedbackId));
        if (feedback) {
          feedback.deliveryStatus = "failed";
          await this.store.saveFeedback(feedback);
        }
        await this.store.saveFeedbackDelivery(stripUndefined(delivery));
        await this.publish({ type: "feedback.dead_lettered", canvasId: delivery.canvasId, feedbackId: delivery.feedbackId, data: delivery });
        await this.publish({ type: "feedback.failed", canvasId: delivery.canvasId, feedbackId: delivery.feedbackId, data: delivery });
        return;
      }
      delivery.status = "retrying";
      delivery.nextAttemptAt = addSeconds(this.now(), Math.min(2 ** Math.max(delivery.attempts - 1, 0), 30));
      await this.store.saveFeedbackDelivery(stripUndefined(delivery));
      await this.publish({ type: "feedback.delivery.retrying", canvasId: delivery.canvasId, feedbackId: delivery.feedbackId, data: delivery });
    }
  }

  private async deliverFeedback(delivery: FeedbackDelivery, canvas: Canvas, feedback: Feedback, run?: AgentRun) {
    if (delivery.provider === "webhook" || delivery.provider === "generic_cloud") {
      if (!delivery.target.url) badRequest("webhook url is required");
      const body = JSON.stringify({ type: "feedback.created", canvas, feedback, deliveryId: delivery.id, agentRun: run, metadata: delivery.target.metadata });
      const headers = new Headers({ "Content-Type": "application/json", "Idempotency-Key": delivery.id });
      for (const [name, value] of Object.entries(delivery.target.headers || {})) headers.set(name, value);
      const secret = this.secretFromAuthRef(delivery.target.authRef);
      if (secret) {
        const ts = Math.floor(Date.now() / 1000).toString();
        headers.set("X-AgentCanvas-Timestamp", ts);
        headers.set("X-AgentCanvas-Signature", await hmacSHA256Hex(secret, `${ts}.${body}`));
      }
      const response = await fetch(delivery.target.url, { method: "POST", headers, body });
      if (!response.ok) throw new Error(`webhook returned status ${response.status}`);
      return { providerMessageId: delivery.id, url: delivery.target.url, summary: "webhook accepted feedback", deliveredAt: this.now() };
    }
    if (delivery.provider === "cursor") {
      const agentID = delivery.target.externalId || run?.externalId;
      if (!agentID) throw new Error("cursor feedback target requires external agent id");
      const base = delivery.target.url || "https://api.cursor.com";
      const url = `${base.replace(/\/$/, "")}/v0/agents/${agentID}/followup`;
      const headers = new Headers({ "Content-Type": "application/json", Accept: "application/json", "Idempotency-Key": delivery.id });
      const token = this.secretFromAuthRef(delivery.target.authRef);
      if (token) headers.set("Authorization", `Bearer ${token}`);
      for (const [name, value] of Object.entries(delivery.target.headers || {})) headers.set(name, value);
      const response = await fetch(url, { method: "POST", headers, body: JSON.stringify({ prompt: { text: formatFeedbackPrompt(canvas, feedback) } }) });
      const text = await response.text();
      if (!response.ok) throw new Error(`cursor follow-up returned status ${response.status}: ${text.trim()}`);
      return { providerMessageId: responseID(text) || delivery.id, url, summary: "cursor background agent follow-up accepted", deliveredAt: this.now() };
    }
    throw new Error(`provider ${JSON.stringify(delivery.provider)} requires a local sidecar and cannot run inside Cloudflare Workers`);
  }

  private async normalizeStampedAgentEvent(request: Request, auth: AuthContext): Promise<AgentEvent> {
    const body = await readJSON<Partial<AgentEvent>>(request, agentEventKeys);
    if (body.agentRunId) {
      const run = await this.getAgentRunForAuth(body.agentRunId, auth);
      body.workspaceId = body.workspaceId || run.workspaceId;
    }
    stampWorkspace(body, auth.workspaceId);
    return normalizeAgentEvent(body, this.now());
  }

  private async recordAgentEvent(event: AgentEvent): Promise<boolean> {
    const created = await this.store.saveAgentEvent(event);
    if (event.agentRunId && event.status) {
      const run = await maybe(() => this.store.getAgentRun(event.agentRunId!));
      if (run) {
        run.status = event.status;
        run.lastEventId = event.id;
        run.traceUrl = event.traceUrl || run.traceUrl;
        run.updatedAt = this.now();
        await this.store.saveAgentRun(run);
        await this.publish({ type: "agent.run.updated", canvasId: run.canvasId, data: run });
      }
    }
    if (created) await this.publish({ type: "agent.event.created", data: event });
    return created;
  }

  private async evaluateObserver(event: AgentEvent, created: boolean): Promise<ObserverDecision> {
    const enabled = parseBool(this.env.AGENTCANVAS_OBSERVER_ENABLED as string | undefined) ?? false;
    const mode = (this.env.AGENTCANVAS_OBSERVER_ACTION_MODE as ObserverDecision["mode"] | undefined) || "off";
    const decision: ObserverDecision = {
      id: newID("observer-decision"),
      eventId: event.id,
      provider: event.provider,
      agentRunId: event.agentRunId,
      mode: enabled ? mode : "off",
      action: "allow",
      createdAt: this.now()
    };
    if (!enabled) return decision;
    const { canvasID, runID } = await this.eventCanvasAndRun(event);
    decision.canvasId = canvasID;
    decision.agentRunId = decision.agentRunId || runID;
    const issues: ObserverIssue[] = [];
    const canvas = canvasID ? await maybe(() => this.store.getCanvas(canvasID)) : undefined;
    if (isStopEvent(event.type) && !canvas) issues.push({ rule: "turn_stopped_without_canvas", severity: "error", message: "agent stopped without a visible AgentCanvas canvas", runId: runID, canvasId: canvasID });
    if (canvas?.status === "ready_for_review" && !hasVerificationEvidence(canvas)) {
      issues.push({ rule: "ready_canvas_missing_verification_evidence", severity: "warning", message: "ready-for-review canvas is missing terminal, diff, image, or checked checklist evidence", runId: runID, canvasId: canvasID });
    }
    if (issues.length) {
      decision.issues = issues;
      decision.nudgePrompt = nudgePrompt(issues, canvasID, runID);
      decision.dryRunActions = [{ type: "agent.nudge", target: decision.agentRunId, summary: "Prompt the agent to update the current AgentCanvas review artifact.", payload: { prompt: decision.nudgePrompt } }];
      decision.action = mode === "dry_run" ? "dry_run" : mode === "enforce" && isStopEvent(event.type) ? "block" : mode === "nudge" || mode === "enforce" ? "nudge" : mode === "log" ? "log" : "allow";
    }
    void created;
    return stripUndefined(decision);
  }

  private async eventCanvasAndRun(event: AgentEvent): Promise<{ canvasID: string; runID: string }> {
    let runID = firstNonEmpty(event.agentRunId, stringPayload(event.payload, "runId", "run_id", "agentRunId", "agent_run_id"));
    let canvasID = stringPayload(event.payload, "canvasId", "canvas_id");
    if (runID) {
      const run = await maybe(() => this.store.getAgentRun(runID));
      if (run) {
        canvasID = firstNonEmpty(canvasID, run.canvasId);
        runID = firstNonEmpty(runID, run.id);
      }
    }
    return { canvasID, runID };
  }

  private async resolveFeedbackTarget(canvas: Canvas): Promise<{ target?: FeedbackTarget; run?: AgentRun }> {
    if (canvas.callback?.feedbackTargetId) {
      const run = await this.store.getAgentRun(canvas.callback.feedbackTargetId);
      if (run.workspaceId !== canvas.workspaceId) forbidden("forbidden");
      const target = { ...run.feedbackTarget, id: run.feedbackTarget.id || run.id };
      return { target, run };
    }
    if (canvas.callback?.webhook?.url) {
      return {
        target: {
          id: `webhook:${canvas.id}`,
          provider: "webhook",
          mode: "webhook",
          url: canvas.callback.webhook.url,
          headers: canvas.callback.webhook.headers
        }
      };
    }
    return {};
  }

  private async getCanvasForAuth(id: string, auth: AuthContext): Promise<Canvas> {
    const canvas = await this.store.getCanvas(id);
    if (!workspaceVisible(canvas.workspaceId, auth.workspaceId)) forbidden();
    return canvas;
  }

  private async getAgentRunForAuth(id: string, auth: AuthContext): Promise<AgentRun> {
    const run = await this.store.getAgentRun(id);
    if (!workspaceVisible(run.workspaceId, auth.workspaceId)) forbidden();
    return run;
  }

  private async authorize(request: Request): Promise<AuthContext | undefined> {
    const header = request.headers.get("Authorization") || "";
    if (!header.startsWith("Bearer ")) return undefined;
    const token = header.slice("Bearer ".length).trim();
    if (!token) throw new AppError(401, "missing or invalid bearer token", "unauthorized");
    if (this.token && token === this.token) return { internal: true };
    const verifier = this.apiKeyVerifier || this.unkeyVerifier();
    if (!verifier) return undefined;
    try {
      const verified = await verifier(token);
      validateID("workspace id", verified.workspaceId);
      return { internal: false, keyId: verified.keyId, workspaceId: verified.workspaceId };
    } catch (error) {
      throw new AppError(error instanceof AppError && error.status === 503 ? 503 : 401, "missing or invalid bearer token", "unauthorized");
    }
  }

  private unkeyVerifier(): ((key: string) => Promise<{ keyId?: string; workspaceId: string }>) | undefined {
    const rootKey = firstNonEmpty(this.env.AGENTCANVAS_UNKEY_ROOT_KEY as string, this.env.UNKEY_ROOT_KEY as string);
    if (!rootKey) return undefined;
    const endpoint = firstNonEmpty(this.env.AGENTCANVAS_UNKEY_VERIFY_URL as string, "https://api.unkey.com/v2/keys.verifyKey");
    return async (key: string) => {
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { Authorization: `Bearer ${rootKey}`, "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ key })
      });
      if (!response.ok) throw new AppError(503, `api key verifier unavailable`, "unavailable");
      const parsed = await response.json<{ data?: { valid?: boolean; keyId?: string; id?: string; metadata?: Record<string, unknown>; meta?: Record<string, unknown> } }>();
      if (!parsed.data?.valid) throw new AppError(401, "invalid api key", "unauthorized");
      const metadata = { ...(parsed.data.metadata || {}), ...(parsed.data.meta || {}) };
      const keyID = parsed.data.keyId || parsed.data.id || "";
      const workspaceId = firstNonEmpty(
        stringValue(metadata.workspaceId),
        stringValue(metadata.workspace_id),
        stringValue(metadata.tenantId),
        stringValue(metadata.tenant_id),
        stringValue(metadata.orgId),
        stringValue(metadata.organizationId),
        keyID
      );
      if (!workspaceId) throw new AppError(401, "invalid api key", "unauthorized");
      return { keyId: keyID, workspaceId };
    };
  }

  private async viewerSessionFromRequest(request: Request): Promise<ViewerSession | undefined> {
    const header = request.headers.get("Authorization") || "";
    if (!header) return undefined;
    if (!header.startsWith("Bearer ")) throw new AppError(401, "missing or invalid viewer session", "unauthorized");
    try {
      const [id, secret] = splitViewerCode(header.slice("Bearer ".length));
      const session = await this.store.getViewerSession(id);
      if (parseJSONDate(session.expiresAt) <= parseJSONDate(this.now())) notFound();
      if (session.secretHash !== (await sha256Hex(secret))) notFound();
      return this.store.touchViewerSession(id, this.now());
    } catch {
      throw new AppError(401, "missing or invalid viewer session", "unauthorized");
    }
  }

  private async validateViewerLinkSecret(id: string, secret: string): Promise<ViewerLink> {
    validateViewerSecret(secret);
    const link = await this.store.getViewerLink(id);
    if (link.revokedAt) notFound();
    if (link.expiresAt && parseJSONDate(link.expiresAt) <= parseJSONDate(this.now())) notFound();
    if (link.secretHash !== (await sha256Hex(secret))) notFound();
    return link;
  }

  private async exchangeViewerLink(id: string, secret: string): Promise<ViewerLinkExchangeResponse> {
    const link = await this.validateViewerLinkSecret(id, secret);
    const canvas = await this.store.getCanvas(link.canvasId);
    if (canvas.workspaceId !== link.workspaceId) notFound();
    const sessionSecret = newViewerSecret();
    const now = this.now();
    const session: ViewerSession = stripUndefined({
      id: newID("viewer-session"),
      workspaceId: link.workspaceId,
      linkId: link.id,
      kind: link.kind,
      scope: link.scope,
      canvasId: link.canvasId,
      runId: link.runId,
      agentId: link.agentId,
      secretHash: await sha256Hex(sessionSecret),
      capabilities: link.capabilities,
      createdAt: now,
      updatedAt: now,
      expiresAt: addSeconds(now, 12 * 60 * 60),
      useCount: 0
    });
    await this.store.saveViewerSession(session);
    await this.store.touchViewerLink(link.id, now);
    return {
      linkId: link.id,
      kind: link.kind,
      scope: link.scope,
      canvasId: link.canvasId,
      runId: link.runId,
      agentId: link.agentId,
      capabilities: link.capabilities,
      sessionToken: viewerCode(session.id, sessionSecret),
      expiresAt: session.expiresAt,
      canvas: sanitizeCanvasForViewer(canvas)
    };
  }

  private async preflightViewerLink(raw: ViewerLinkRequest, auth: AuthContext) {
    const req = normalizeViewerLinkRequest(raw, this.linkBaseURL(), this.now());
    const checks: ViewerLinkCheck[] = [];
    const add = (code: string, status: "pass" | "fail" | "warn", message: string) => checks.push({ code, status, message });
    if (req.kind !== "configuration" && req.kind !== "share") add("kind", "fail", "kind must be configuration or share");
    if (req.scope !== "canvas") add("scope", "fail", "scope must be canvas");
    try {
      validateID("canvas id", req.canvasId);
      if (req.runId) validateID("run id", req.runId);
      if (req.agentId) validateID("agent id", req.agentId);
      validateViewerBaseURL(req.linkBaseUrl);
      if (req.expiresAt && parseJSONDate(req.expiresAt) <= parseJSONDate(this.now())) add("expiry", "fail", "expiresAt must be in the future");
      for (const capability of req.capabilities) if (!["canvas.read", "canvas.live", "asset.read", "feedback.submit"].includes(capability)) add("capabilities", "fail", `unsupported capability ${capability}`);
      const canvas = await this.getCanvasForAuth(req.canvasId, auth);
      add("canvas_fetch", "pass", "target canvas is readable");
      if (req.runId && canvas.runId !== req.runId) add("run_match", "fail", `canvas runId ${JSON.stringify(canvas.runId)} does not match requested runId ${JSON.stringify(req.runId)}`);
      if (req.agentId && canvas.agentId !== req.agentId) add("agent_match", "fail", `canvas agentId ${JSON.stringify(canvas.agentId)} does not match requested agentId ${JSON.stringify(req.agentId)}`);
      const size = JSON.stringify(canvas).length;
      add("payload_size", size > 1 << 20 ? "fail" : "pass", size > 1 << 20 ? `canvas payload is too large for viewer links: ${size} bytes` : `canvas payload is ${size} bytes`);
      canvas.blocks.forEach((block, index) => {
        if (!validBlockKind(block.kind)) add("block_kind", "fail", `block ${index} has unsupported kind ${JSON.stringify(block.kind)}`);
      });
      if (req.capabilities.includes("feedback.submit")) {
        const target = await this.resolveFeedbackTarget(canvas);
        add("feedback_target", target.target ? "pass" : "warn", target.target ? "feedback can be submitted and delivered" : "feedback can be stored, but no delivery target is configured");
      }
    } catch (error) {
      add("canvas_fetch", "fail", error instanceof Error ? error.message : "target canvas must exist before creating a viewer link");
    }
    return {
      status: checks.some((check) => check.status === "fail") ? "blocked" : "ready",
      checks,
      kind: req.kind,
      scope: req.scope,
      canvasId: req.canvasId,
      runId: req.runId || undefined,
      agentId: req.agentId || undefined,
      capabilities: req.capabilities,
      linkBaseUrl: req.linkBaseUrl
    };
  }

  private handleCORS(request: Request): Response | undefined {
    const origin = request.headers.get("Origin");
    if (!origin) return undefined;
    if (!this.corsOrigins().has(origin)) {
      if (request.method === "OPTIONS") return jsonResponse({ error: "cors origin is not allowed" }, 403, corsVaryHeaders());
      return undefined;
    }
    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: corsHeaders(origin, request) });
    }
    return undefined;
  }

  private corsOrigins(): Set<string> {
    const raw = this.env.AGENTCANVAS_CORS_ORIGINS as string | undefined;
    return new Set((raw ? raw.split(",") : defaultCORSOrigins).map((origin) => origin.trim()).filter(Boolean));
  }

  private withCORS(request: Request, response: Response): Response {
    const origin = request.headers.get("Origin");
    if (!origin || !this.corsOrigins().has(origin)) return response;
    const next = new Response(response.body, response);
    for (const [key, value] of Object.entries(corsHeaders(origin, request))) next.headers.set(key, value);
    return next;
  }

  private linkBaseURL(): string {
    return (this.env.AGENTCANVAS_LINK_BASE_URL as string | undefined) || defaultViewerLinkBaseURL;
  }

  private isPublicViewerExchange(path: string): boolean {
    const parts = pathParts(path);
    return parts.length === 4 && parts[0] === "v1" && parts[1] === "viewer-links" && parts[3] === "exchange";
  }

  private async publish(event: Parameters<EventBus["publish"]>[0]): Promise<void> {
    await this.bus.publish({ ...event, createdAt: event.createdAt || this.now() });
  }

  private secretFromAuthRef(authRef: string | undefined): string {
    if (!authRef) return "";
    const value = this.env[authRef];
    return typeof value === "string" ? value : "";
  }
}

async function readJSON<T>(request: Request, allowedKeys?: Set<string>): Promise<T> {
  let raw: unknown;
  try {
    raw = await request.json();
  } catch (error) {
    badRequest(`invalid json: ${error instanceof Error ? error.message : String(error)}`);
  }
  if (allowedKeys && raw && typeof raw === "object" && !Array.isArray(raw)) rejectUnknown(raw as Record<string, unknown>, allowedKeys);
  return raw as T;
}

async function readOptionalJSON<T>(request: Request, allowedKeys?: Set<string>): Promise<T> {
  const text = await request.text();
  if (!text.trim()) return {} as T;
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch (error) {
    badRequest(`invalid json: ${error instanceof Error ? error.message : String(error)}`);
  }
  if (allowedKeys && raw && typeof raw === "object" && !Array.isArray(raw)) rejectUnknown(raw as Record<string, unknown>, allowedKeys);
  return raw as T;
}

function rejectUnknown(raw: Record<string, unknown>, allowed: Set<string>): void {
  for (const key of Object.keys(raw)) if (!allowed.has(key)) badRequest(`invalid json: unknown field ${JSON.stringify(key)}`);
}

function pathParts(path: string): string[] {
  return path.split("/").filter(Boolean);
}

function pathID(raw: string | undefined): string {
  if (!raw) badRequest("invalid id");
  let decoded: string;
  try {
    decoded = decodeURIComponent(raw);
  } catch {
    badRequest("invalid escaped id");
  }
  return validateID("id", decoded);
}

function methodNotAllowed(): Response {
  return jsonResponse({ error: "method not allowed" }, 405);
}

function corsHeaders(origin: string, request: Request): Record<string, string> {
  return {
    ...corsVaryHeaders(),
    "Access-Control-Allow-Origin": origin,
    "Access-Control-Allow-Methods": "GET,POST,OPTIONS",
    "Access-Control-Allow-Headers": request.headers.get("Access-Control-Request-Headers") || "Authorization, Content-Type",
    "Access-Control-Max-Age": "86400"
  };
}

function corsVaryHeaders(): Record<string, string> {
  return {
    Vary: "Origin, Access-Control-Request-Method, Access-Control-Request-Headers"
  };
}

function viewerHeaders(): Record<string, string> {
  return { "Cache-Control": "no-store" };
}

function hasCapability(capabilities: string[], target: string): boolean {
  return capabilities.includes(target);
}

function validateScopedCanvasAccess(expected: string, actual: string): void {
  if (expected !== actual) forbidden("viewer session cannot access this canvas");
}

function viewerLinkURL(baseURL: string, code: string): string {
  return `${baseURL.replace(/\/$/, "")}/c/${code}`;
}

function viewerCode(id: string, secret: string): string {
  return `${id}.${secret}`;
}

function splitViewerCode(code: string): [string, string] {
  const [id, secret, extra] = code.trim().split(".");
  if (!id || !secret || extra !== undefined) badRequest("viewer link code must be <id>.<secret>");
  validateID("viewer link id", id);
  validateViewerSecret(secret);
  return [id, secret];
}

async function secretFromRequest(id: string, body: { code?: string; secret?: string }): Promise<string> {
  let secret = body.secret || "";
  if (body.code) {
    const [codeID, codeSecret] = splitViewerCode(body.code);
    if (codeID !== id) badRequest("viewer link code does not match path id");
    secret = codeSecret;
  }
  return secret;
}

function validateViewerSecret(secret: string): void {
  if (!secret) badRequest("viewer secret is required");
  for (const ch of secret) if (!/^[A-Za-z0-9_-]$/.test(ch)) badRequest(`viewer secret contains invalid character ${JSON.stringify(ch)}`);
}

function validateViewerBaseURL(raw: string): void {
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    badRequest("linkBaseUrl must be an absolute URL");
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") badRequest("linkBaseUrl must use http or https");
}

function newViewerSecret(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function targetMaxAttempts(target: FeedbackTarget): number {
  const raw = target.metadata?.maxAttempts;
  const parsed = raw ? Number.parseInt(raw, 10) : 0;
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
}

function edgeDeliverable(provider: string): boolean {
  return provider === "webhook" || provider === "generic_cloud" || provider === "cursor";
}

function formatFeedbackPrompt(canvas: Canvas, feedback: Feedback): string {
  return [`Feedback on AgentCanvas "${canvas.title}" (${canvas.id})`, `Decision: ${feedback.decision}`, feedback.text || feedback.voiceTranscript || "", feedback.targetBlockIds?.length ? `Target blocks: ${feedback.targetBlockIds.join(", ")}` : ""].filter(Boolean).join("\n\n");
}

function responseID(text: string): string {
  try {
    const parsed = JSON.parse(text) as { id?: string; requestId?: string };
    return parsed.id || parsed.requestId || "";
  } catch {
    return "";
  }
}

async function maybe<T>(fn: () => Promise<T>): Promise<T | undefined> {
  try {
    return await fn();
  } catch {
    return undefined;
  }
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function stringPayload(payload: Record<string, unknown> | undefined, ...keys: string[]): string {
  for (const key of keys) {
    const value = payload?.[key];
    if (typeof value === "string" && value) return value;
  }
  return "";
}

function isStopEvent(eventType: string): boolean {
  return eventType.includes(".Stop") || eventType.includes("turn.completed") || eventType.includes("turn.finished");
}

function hasVerificationEvidence(canvas: Canvas): boolean {
  return canvas.blocks.some((block) => block.kind === "terminal" || block.kind === "diff" || block.kind === "image" || (block.kind === "checklist" && block.items?.some((item) => item.checked)));
}

function nudgePrompt(issues: ObserverIssue[], canvasID: string, runID: string): string {
  return ["AgentCanvas lifecycle observer found missing review evidence.", `Target: canvas=${canvasID || "<unknown>"} run=${runID || "<unknown>"}`, ...issues.map((issue) => `- ${issue.message}`), "Update the existing AgentCanvas canvas with the missing status, evidence, or acknowledgement before stopping."].join("\n");
}

const canvasKeys = new Set(["id", "workspaceId", "agentId", "runId", "title", "summary", "status", "mode", "priority", "createdAt", "updatedAt", "version", "lastEventId", "callback", "blocks"]);
const canvasEventKeys = new Set(["id", "workspaceId", "canvasId", "type", "expectedVersion", "version", "agentId", "runId", "title", "summary", "status", "priority", "block", "blockId", "insertAfterBlockId", "createdAt"]);
const snapshotKeys = new Set(["reason", "source", "label", "sourceEventId", "sourceEditId"]);
const canvasImportKeys = new Set(["bundle", "conflictPolicy"]);
const editKeys = new Set(["id", "canvasId", "expectedVersion", "ops", "submittedBy", "note", "createdAt"]);
const feedbackKeys = new Set(["id", "workspaceId", "canvasId", "decision", "text", "voiceTranscript", "inkAssetId", "targetBlockIds", "targetAnchors", "createdAt", "deliveryStatus"]);
const agentRunKeys = new Set(["id", "workspaceId", "provider", "agentId", "runId", "canvasId", "title", "mode", "status", "externalId", "authRef", "webUrl", "traceUrl", "lastEventId", "error", "feedbackTarget", "metadata", "createdAt", "updatedAt"]);
const agentEventKeys = new Set(["id", "workspaceId", "provider", "agentRunId", "externalId", "type", "status", "sequence", "payload", "traceUrl", "occurredAt", "createdAt"]);
const viewerLinkRequestKeys = new Set(["kind", "scope", "canvasId", "runId", "agentId", "capabilities", "linkBaseUrl", "expiresAt", "ttlSeconds"]);
