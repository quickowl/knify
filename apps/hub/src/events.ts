import type { AuthContext, Canvas, HubEnv, HubEvent, ViewerSession } from "./types";
import type { Store } from "./store";
import { D1Store } from "./store";
import { newID, nowISO, workspaceVisible } from "./utils";
import { decorateCanvasAssetURLs, sanitizeCanvasForViewer } from "./validation";

export interface EventBus {
  publish(event: HubEvent): Promise<void>;
  stream(auth: AuthContext, session?: ViewerSession): Promise<Response>;
}

export class MemoryEventBus implements EventBus {
  private subscribers = new Set<ReadableStreamDefaultController<Uint8Array>>();

  constructor(private store: Store) {}

  async publish(event: HubEvent): Promise<void> {
    const normalized = normalizeEvent(event);
    const frame = encodeFrame(normalized);
    for (const subscriber of [...this.subscribers]) {
      try {
        subscriber.enqueue(frame);
      } catch {
        this.subscribers.delete(subscriber);
      }
    }
  }

  async stream(auth: AuthContext, session?: ViewerSession): Promise<Response> {
    const store = this.store;
    const subscribers = this.subscribers;
    const stream = new ReadableStream<Uint8Array>({
      async start(controller) {
        controller.enqueue(new TextEncoder().encode(": connected\n\n"));
        if (session) {
          const canvas = sanitizeCanvasForViewer(await store.getCanvas(session.canvasId));
          controller.enqueue(encodeFrame({ type: "canvas.updated", canvasId: canvas.id, data: canvas, createdAt: nowISO() }));
        } else {
          const canvases = (await store.listCanvases()).filter((canvas) => workspaceVisible(canvas.workspaceId, auth.workspaceId));
          for (const canvas of canvases) controller.enqueue(encodeFrame({ type: "canvas.updated", canvasId: canvas.id, data: canvas, createdAt: nowISO() }));
        }
        subscribers.add(controller);
      },
      cancel(controller) {
        subscribers.delete(controller as ReadableStreamDefaultController<Uint8Array>);
      }
    });
    return sseResponse(stream);
  }
}

export class DurableEventBus implements EventBus {
  constructor(private env: HubEnv) {}

  async publish(event: HubEvent): Promise<void> {
    const id = this.env.EVENTS.idFromName("hub-events");
    const stub = this.env.EVENTS.get(id);
    await stub.fetch("https://events.internal/publish", {
      method: "POST",
      body: JSON.stringify(normalizeEvent(event))
    });
  }

  async stream(auth: AuthContext, session?: ViewerSession): Promise<Response> {
    const id = this.env.EVENTS.idFromName("hub-events");
    const stub = this.env.EVENTS.get(id);
    const url = new URL("https://events.internal/stream");
    if (auth.workspaceId) url.searchParams.set("workspaceId", auth.workspaceId);
    if (session) {
      url.searchParams.set("canvasId", session.canvasId);
      url.searchParams.set("viewer", "1");
    }
    return stub.fetch(url);
  }
}

export class EventHub implements DurableObject {
  private subscribers = new Map<string, { controller: ReadableStreamDefaultController<Uint8Array>; workspaceId?: string; canvasId?: string; viewer: boolean }>();

  constructor(
    private state: DurableObjectState,
    private env: HubEnv
  ) {}

  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/publish") {
      const event = normalizeEvent(await request.json<HubEvent>());
      const frame = encodeFrame(event);
      for (const [id, subscriber] of [...this.subscribers.entries()]) {
        if (subscriber.canvasId && event.canvasId !== subscriber.canvasId) continue;
        if (subscriber.workspaceId && !eventVisibleToWorkspace(event, subscriber.workspaceId)) continue;
        try {
          subscriber.controller.enqueue(frame);
        } catch {
          this.subscribers.delete(id);
        }
      }
      return new Response("ok");
    }
    if (url.pathname === "/stream") {
      const workspaceId = url.searchParams.get("workspaceId") || undefined;
      const canvasId = url.searchParams.get("canvasId") || undefined;
      const viewer = url.searchParams.get("viewer") === "1";
      const store = new D1Store(this.env.DB, this.env.ASSETS_BUCKET);
      const subscribers = this.subscribers;
      const assetPublicBaseURL = this.env.AGENTCANVAS_ASSET_PUBLIC_BASE_URL;
      const stream = new ReadableStream<Uint8Array>({
        async start(controller) {
          controller.enqueue(new TextEncoder().encode(": connected\n\n"));
          if (canvasId) {
            const canvas = decorateCanvasAssetURLs(await store.getCanvas(canvasId), assetPublicBaseURL);
            controller.enqueue(encodeFrame({ type: "canvas.updated", canvasId: canvas.id, data: viewer ? sanitizeCanvasForViewer(canvas) : canvas, createdAt: nowISO() }));
          } else {
            const canvases = (await store.listCanvases()).filter((canvas) => workspaceVisible(canvas.workspaceId, workspaceId));
            for (const rawCanvas of canvases) {
              const canvas = decorateCanvasAssetURLs(rawCanvas, assetPublicBaseURL);
              controller.enqueue(encodeFrame({ type: "canvas.updated", canvasId: canvas.id, data: canvas, createdAt: nowISO() }));
            }
          }
          const id = newID("subscriber");
          subscribers.set(id, { controller, workspaceId, canvasId, viewer });
        }
      });
      return sseResponse(stream);
    }
    return new Response("not found", { status: 404 });
  }
}

export function normalizeEvent(event: HubEvent): HubEvent {
  return {
    id: event.id || newID("event"),
    type: event.type,
    canvasId: event.canvasId,
    feedbackId: event.feedbackId,
    data: event.data,
    createdAt: event.createdAt || nowISO()
  };
}

export function encodeFrame(event: HubEvent): Uint8Array {
  const normalized = normalizeEvent(event);
  return new TextEncoder().encode(`id: ${normalized.id}\nevent: ${normalized.type}\ndata: ${JSON.stringify(normalized)}\n\n`);
}

function sseResponse(stream: ReadableStream<Uint8Array>): Response {
  return new Response(stream, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive"
    }
  });
}

function eventVisibleToWorkspace(event: HubEvent, workspaceId: string): boolean {
  const data = event.data as { workspaceId?: string } | undefined;
  if (data?.workspaceId !== undefined) return data.workspaceId === workspaceId;
  return true;
}
