import { describe, expect, it, vi } from "vitest";
import { createApp } from "./app";
import { MemoryEventBus } from "./events";
import { MemoryStore } from "./store";
import type { Canvas, HubEnv } from "./types";

const testToken = "test-token";
const fixedNow = "2026-04-23T19:00:00.000Z";

function testCanvas(id: string): Canvas {
  return {
    id,
    agentId: "agent-1",
    runId: "run-1",
    title: "Plan",
    summary: "Summary",
    status: "ready_for_review",
    priority: "high",
    version: 1,
    createdAt: fixedNow,
    updatedAt: fixedNow,
    blocks: [{ id: "blk-1", kind: "markdown", markdown: "hello" }]
  };
}

function makeHarness(apiKeyVerifier?: (key: string) => Promise<{ keyId?: string; workspaceId: string }>, envOverrides: Partial<HubEnv> = {}) {
  const store = new MemoryStore();
  const bus = new MemoryEventBus(store);
  const env = { HUB_TOKEN: testToken, ...envOverrides } as HubEnv;
  const app = createApp(env, { store, bus, token: testToken, now: () => fixedNow, apiKeyVerifier });
  const waits: Promise<unknown>[] = [];
  const ctx = {
    waitUntil(promise: Promise<unknown>) {
      waits.push(promise);
    },
    passThroughOnException() {}
  } as ExecutionContext;
  return {
    app,
    store,
    async fetch(path: string, init: RequestInit = {}, token = testToken) {
      const headers = new Headers(init.headers);
      if (token) headers.set("Authorization", `Bearer ${token}`);
      const response = await app.fetch(new Request(`https://hub.test${path}`, { ...init, headers }), ctx);
      await Promise.allSettled(waits.splice(0));
      return response;
    },
    async json(path: string, body: unknown, token = testToken) {
      return this.fetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }, token);
    }
  };
}

async function readJSON<T>(response: Response): Promise<T> {
  return (await response.json()) as T;
}

describe("AgentCanvas Worker hub", () => {
  it("requires auth and handles canvases, assets, and feedback", async () => {
    const h = makeHarness();
    expect((await h.fetch("/v1/canvases", {}, "")).status).toBe(401);

    const created = await h.json("/v1/canvases", testCanvas("canvas-http"));
    expect(created.status).toBe(201);
    expect((await readJSON<Canvas>(created)).id).toBe("canvas-http");

    const got = await readJSON<Canvas>(await h.fetch("/v1/canvases/canvas-http"));
    expect(got.title).toBe("Plan");

    const assetCreate = await h.fetch("/v1/assets/ink-1", { method: "POST", headers: { "Content-Type": "image/png" }, body: "ink" });
    expect(assetCreate.status).toBe(201);
    const asset = await h.fetch("/v1/assets/ink-1");
    expect(asset.status).toBe(200);
    expect(asset.headers.get("Content-Type")).toBe("image/png");
    expect(await asset.text()).toBe("ink");

    const feedback = await h.json("/v1/canvases/canvas-http/feedback", { id: "feedback-http", decision: "accepted", text: "ship it" });
    expect(feedback.status).toBe(201);
    expect(await readJSON<unknown>(feedback)).toMatchObject({ canvasId: "canvas-http", deliveryStatus: "delivered" });
  });

  it("accepts html blocks, validates them, and decorates screenshot asset urls", async () => {
    const h = makeHarness(undefined, { AGENTCANVAS_ASSET_PUBLIC_BASE_URL: "https://pub.example.r2.dev/" } as Partial<HubEnv>);

    const inline = testCanvas("canvas-html-inline");
    inline.blocks = [{ id: "html-1", kind: "html", html: "<p>hi</p>", sandbox: "strict", height: 240, title: "Snippet" }];
    expect((await h.json("/v1/canvases", inline)).status).toBe(201);

    const shot = await readJSON<{ assetId: string; url: string }>(await h.fetch("/v1/assets", { method: "POST", headers: { "Content-Type": "image/png" }, body: "png" }));
    const screenshot = testCanvas("canvas-html-screenshot");
    screenshot.blocks = [{ id: "html-2", kind: "html", screenshotAssetId: shot.assetId, caption: "Rendered HTML" }];
    expect((await h.json("/v1/canvases", screenshot)).status).toBe(201);

    const got = await readJSON<Canvas>(await h.fetch("/v1/canvases/canvas-html-screenshot"));
    expect(got.blocks[0]).toMatchObject({ screenshotAssetId: shot.assetId, screenshotUrl: shot.url });
    expect((await h.store.getCanvas("canvas-html-screenshot")).blocks[0].screenshotUrl).toBeUndefined();

    const missing = testCanvas("canvas-html-missing");
    missing.blocks = [{ id: "html-bad", kind: "html" }];
    expect((await h.json("/v1/canvases", missing)).status).toBe(400);

    const tooBig = testCanvas("canvas-html-big");
    tooBig.blocks = [{ id: "html-big", kind: "html", html: "x".repeat(300_000) }];
    expect((await h.json("/v1/canvases", tooBig)).status).toBe(400);

    const badSandbox = testCanvas("canvas-html-sandbox");
    badSandbox.blocks = [{ id: "html-sb", kind: "html", html: "<p>hi</p>", sandbox: "wide-open" as never }];
    expect((await h.json("/v1/canvases", badSandbox)).status).toBe(400);
  });

  it("returns opaque public asset URLs and decorates image blocks on read", async () => {
    const h = makeHarness(undefined, { AGENTCANVAS_ASSET_PUBLIC_BASE_URL: "https://pub.example.r2.dev/" } as Partial<HubEnv>);
    const uploaded = await readJSON<{ assetId: string; id: string; url: string }>(await h.fetch("/v1/assets", { method: "POST", headers: { "Content-Type": "image/png" }, body: "png" }));
    expect(uploaded.assetId).toMatch(/^asset-[a-f0-9]{32}$/);
    expect(uploaded.id).toBe(uploaded.assetId);
    expect(uploaded.url).toBe(`https://pub.example.r2.dev/assets/${uploaded.assetId}`);

    const canvas = testCanvas("canvas-image");
    canvas.blocks = [{ id: "img-1", kind: "image", assetId: uploaded.assetId, alt: "screenshot" }];
    expect((await h.json("/v1/canvases", canvas)).status).toBe(201);

    const got = await readJSON<Canvas>(await h.fetch("/v1/canvases/canvas-image"));
    expect(got.blocks[0]).toMatchObject({ assetId: uploaded.assetId, url: uploaded.url });
    expect((await h.store.getCanvas("canvas-image")).blocks[0].url).toBeUndefined();

    const asset = await h.fetch(`/v1/assets/${uploaded.assetId}`);
    expect(asset.status).toBe(200);
    expect(await asset.text()).toBe("png");
  });

  it("scopes API keys by workspace while internal token sees all", async () => {
    const h = makeHarness(async (key) => {
      if (key === "knify-key-a") return { keyId: "key-a", workspaceId: "ws-a" };
      if (key === "knify-key-b") return { keyId: "key-b", workspaceId: "ws-b" };
      throw new Error("bad key");
    });
    expect((await h.json("/v1/canvases", testCanvas("canvas-a"), "knify-key-a")).status).toBe(201);
    expect((await h.json("/v1/canvases", testCanvas("canvas-b"), "knify-key-b")).status).toBe(201);
    expect((await h.fetch("/v1/canvases/canvas-a", {}, "knify-key-b")).status).toBe(403);
    expect((await readJSON<Canvas[]>(await h.fetch("/v1/canvases", {}, "knify-key-a"))).map((canvas) => canvas.id)).toEqual(["canvas-a"]);
    expect((await readJSON<Canvas[]>(await h.fetch("/v1/canvases"))).map((canvas) => canvas.id)).toEqual(["canvas-a", "canvas-b"]);
  });

  it("honors configured CORS origins on normal responses", async () => {
    const h = makeHarness(undefined, { AGENTCANVAS_CORS_ORIGINS: "https://custom.example" } as Partial<HubEnv>);
    const response = await h.fetch("/v1/healthz", { headers: { Origin: "https://custom.example" } });
    expect(response.status).toBe(200);
    expect(response.headers.get("Access-Control-Allow-Origin")).toBe("https://custom.example");
  });

  it("replays canvases on SSE connect", async () => {
    const h = makeHarness();
    await h.json("/v1/canvases", testCanvas("canvas-replay"));
    const response = await h.fetch("/v1/events");
    expect(response.status).toBe(200);
    const reader = response.body!.getReader();
    const first = new TextDecoder().decode((await reader.read()).value);
    const second = new TextDecoder().decode((await reader.read()).value);
    expect(first + second).toContain("event: canvas.updated");
    expect(first + second).toContain("canvas-replay");
    await reader.cancel();
  });

  it("handles viewer links, exchange, scoped canvas access, assets, and feedback", async () => {
    const h = makeHarness();
    const canvas = testCanvas("canvas-viewer");
    canvas.callback = { webhook: { url: "https://feedback.test/submit", headers: { Authorization: "secret" } } };
    canvas.blocks.push({ id: "img-1", kind: "image", assetId: "asset-viewer", alt: "diagram" });
    await h.json("/v1/canvases", canvas);
    await h.json("/v1/canvases", testCanvas("canvas-other"));
    await h.fetch("/v1/assets/asset-viewer", { method: "POST", headers: { "Content-Type": "image/png" }, body: "png" });

    const request = { kind: "configuration", canvasId: "canvas-viewer", runId: "run-1", agentId: "agent-1", linkBaseUrl: "http://127.0.0.1:5173" };
    expect((await h.json("/v1/viewer-links/preflight", request)).status).toBe(200);
    const created = await readJSON<{ id: string; code: string; url: string }>(await h.json("/v1/viewer-links", request));
    expect(created.url).toContain("/c/viewer-link-");
    expect((await h.json(`/v1/viewer-links/${created.id}/self-test`, { code: created.code })).status).toBe(200);
    const exchanged = await readJSON<{ sessionToken: string; canvas: Canvas }>(await h.json(`/v1/viewer-links/${created.id}/exchange`, { code: created.code }, ""));
    expect(exchanged.canvas.callback).toBeUndefined();

    expect((await readJSON<Canvas[]>(await h.fetch("/v1/canvases", {}, exchanged.sessionToken))).map((item) => item.id)).toEqual(["canvas-viewer"]);
    expect((await h.fetch("/v1/canvases/canvas-other", {}, exchanged.sessionToken)).status).toBe(403);
    expect(await (await h.fetch("/v1/assets/asset-viewer", {}, exchanged.sessionToken)).text()).toBe("png");
    vi.stubGlobal("fetch", vi.fn(async () => new Response("ok")));
    expect((await h.json("/v1/canvases/canvas-viewer/feedback", { decision: "comment_only", text: "looks good" }, exchanged.sessionToken)).status).toBe(202);
    vi.unstubAllGlobals();
  });

  it("projects dynamic canvas events and rejects stale versions", async () => {
    const h = makeHarness();
    const start = await h.json("/v1/canvases/canvas-dynamic/events", { id: "event-1", type: "canvas.started", agentId: "agent-live", runId: "run-live", title: "Dynamic review" });
    expect(start.status).toBe(201);
    expect(await readJSON<Canvas>(start)).toMatchObject({ id: "canvas-dynamic", mode: "dynamic", version: 1, status: "in_progress" });
    const append = await h.json("/v1/canvases/canvas-dynamic/events", { id: "event-2", type: "canvas.block.appended", expectedVersion: 1, block: { id: "block-live", kind: "markdown", markdown: "live block" } });
    expect(append.status).toBe(201);
    const canvas = await readJSON<Canvas>(append);
    expect(canvas.version).toBe(2);
    expect(canvas.blocks[0].id).toBe("block-live");
    const stale = await h.json("/v1/canvases/canvas-dynamic/events", { id: "event-3", type: "canvas.block.appended", expectedVersion: 1, block: { id: "block-live-2", kind: "markdown", markdown: "bad" } });
    expect(stale.status).toBe(409);
  });

  it("creates snapshots, auto-snapshots static overwrites, and restores", async () => {
    const h = makeHarness();
    await h.json("/v1/canvases", testCanvas("canvas-snap"));
    const baseline = await readJSON<{ id: string; version: number }>(await h.json("/v1/canvases/canvas-snap/snapshots", { label: "Baseline", reason: "manual", source: "test" }));
    expect(baseline.version).toBe(1);
    const updated = testCanvas("canvas-snap");
    updated.version = 2;
    updated.blocks[0].markdown = "changed";
    await h.json("/v1/canvases", updated);
    expect((await readJSON<unknown[]>(await h.fetch("/v1/canvases/canvas-snap/snapshots"))).length).toBe(2);
    const restored = await readJSON<{ canvas: Canvas; checkpoint: { reason: string } }>(await h.json(`/v1/canvases/canvas-snap/snapshots/${baseline.id}/restore`, {}));
    expect(restored.canvas.version).toBe(3);
    expect(restored.canvas.blocks[0].markdown).toBe("hello");
    expect(restored.checkpoint.reason).toBe("pre_restore");
  });

  it("validates canvas edits", async () => {
    const h = makeHarness();
    const canvas = testCanvas("canvas-edit");
    canvas.blocks = [
      { id: "form-blk-1", kind: "form", fields: [{ name: "test-name", type: "text", label: "Test Name" }] },
      { id: "heading-blk-1", kind: "heading", text: "Hello" }
    ];
    await h.json("/v1/canvases", canvas);
    expect((await h.json("/v1/canvases/canvas-edit/edits", { expectedVersion: 1, ops: [{ op: "set-field", blockId: "form-blk-1", name: "test-name", value: "x" }] })).status).toBe(201);
    expect((await h.json("/v1/canvases/canvas-edit/edits", { expectedVersion: 99, ops: [{ op: "set-field", blockId: "form-blk-1", value: "x" }] })).status).toBe(409);
    expect((await h.json("/v1/canvases/canvas-edit/edits", { expectedVersion: 1, ops: [{ op: "set-field", blockId: "heading-blk-1", value: "x" }] })).status).toBe(400);
    expect(await readJSON<unknown>(await h.fetch("/v1/canvases/canvas-edit/edits"))).toMatchObject({ count: 1 });
  });

  it("rejects unknown request fields", async () => {
    const h = makeHarness();
    const response = await h.json("/v1/canvases", { ...testCanvas("canvas-bad"), primary: true });
    expect(response.status).toBe(400);
    expect(await readJSON<unknown>(response)).toMatchObject({ error: expect.stringContaining("unknown field") });
  });
});
