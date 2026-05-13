type Env = {
  ASSETS: {
    fetch(request: Request): Promise<Response>;
  };
  HUB_BASE_URL?: string;
  HUB_TOKEN?: string;
};

type HubCanvas = {
  id: string;
  agentId: string;
  title: string;
  summary?: string;
  status?: string;
  priority?: string;
  version: number;
  blocks?: Array<{
    kind?: string;
    title?: string;
    metadata?: Record<string, unknown>;
  }>;
};

const fallbackHubBaseURL = "https://knify-agentcanvas-hub.ctonitou.workers.dev";

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: {
      "Content-Type": "application/json",
      "Cache-Control": "no-store",
    },
  });
}

function stringValue(value: unknown, fallback = "") {
  return typeof value === "string" && value.trim() ? value : fallback;
}

function booleanValue(value: unknown, fallback = false) {
  return typeof value === "boolean" ? value : fallback;
}

function numberValue(value: unknown, fallback: number) {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function oneOf<T extends string>(value: unknown, allowed: readonly T[], fallback: T): T {
  return typeof value === "string" && allowed.includes(value as T) ? value as T : fallback;
}

function dashboardCanvas(canvas: HubCanvas) {
  const meta = canvas.blocks?.find((block) => block.kind === "metadata" && block.title === "dashboard")?.metadata || {};
  const prioritySeverity = canvas.priority === "urgent" || canvas.priority === "high" ? "red" : "green";

  return {
    id: canvas.id,
    ticket: stringValue(meta.ticket, canvas.id),
    title: canvas.title,
    agent: canvas.agentId,
    version: canvas.version,
    live: booleanValue(meta.live, canvas.status === "in_progress"),
    kind: oneOf(meta.kind, ["metric", "tasks", "incident", "chart", "design"] as const, "tasks"),
    severity: oneOf(meta.severity, ["green", "amber", "red"] as const, prioritySeverity),
    team: oneOf(meta.team, ["eng", "design", "ops", "pm", "infra", "sec"] as const, "eng"),
    viewers: numberValue(meta.viewers, 0),
    source: "hub",
    summary: stringValue(canvas.summary, ""),
  };
}

function hubBaseURL(env: Env) {
  return env.HUB_BASE_URL || fallbackHubBaseURL;
}

async function fetchHubJSON(path: string, env: Env) {
  if (!env.HUB_TOKEN) return { response: json({ error: "missing HUB_TOKEN" }, 503) };

  const hubURL = new URL(path, hubBaseURL(env));
  const response = await fetch(hubURL, {
    headers: {
      "Accept": "application/json",
      "Authorization": `Bearer ${env.HUB_TOKEN}`,
    },
  });

  if (!response.ok) {
    return { response: json({ error: "hub request failed", status: response.status }, 502) };
  }

  return { hubURL, value: await response.json() };
}

function proxyHeaders(request: Request, env: Env) {
  const headers = new Headers(request.headers);
  headers.delete("Host");
  headers.delete("Cookie");
  headers.delete("CF-Connecting-IP");
  headers.delete("CF-IPCountry");
  headers.delete("CF-Ray");
  headers.delete("CF-Visitor");
  headers.set("Authorization", `Bearer ${env.HUB_TOKEN}`);
  if (!headers.has("Accept")) headers.set("Accept", "application/json");
  return headers;
}

async function proxyHubRequest(request: Request, env: Env, sourceURL: URL) {
  if (!env.HUB_TOKEN) return json({ error: "missing HUB_TOKEN" }, 503);

  const hubPath = sourceURL.pathname === "/api/hub" ? "/v1" : sourceURL.pathname.replace(/^\/api\/hub/, "/v1");
  const hubURL = new URL(hubPath, hubBaseURL(env));
  hubURL.search = sourceURL.search;

  const response = await fetch(hubURL, {
    method: request.method,
    headers: proxyHeaders(request, env),
    body: request.method === "GET" || request.method === "HEAD" ? undefined : request.body,
  });

  const headers = new Headers(response.headers);
  headers.set("Access-Control-Allow-Origin", sourceURL.origin);
  headers.set("Cache-Control", "no-store");
  headers.delete("Set-Cookie");

  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers,
  });
}

export default {
  async fetch(request: Request, env: Env) {
    const url = new URL(request.url);
    if (url.pathname === "/api/dashboard/canvases") {
      const result = await fetchHubJSON("/v1/canvases", env);
      if (result.response) return result.response;
      const canvases = result.value as HubCanvas[];
      return json({
        source: "hub",
        hub: result.hubURL.origin,
        canvases: canvases.map(dashboardCanvas),
      });
    }

    if (url.pathname === "/api/hub" || url.pathname.startsWith("/api/hub/")) {
      return proxyHubRequest(request, env, url);
    }

    return env.ASSETS.fetch(request);
  },
};
