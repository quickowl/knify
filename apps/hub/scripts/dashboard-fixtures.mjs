export const defaultHubBaseURL = "https://hub.knify.dev";
export const defaultHubToken = "agentcanvas-dev-token";

export const dashboardFixtures = [
  { id: "pixel-audit-acme", ticket: "PIX-184", title: "Acme pixel drift", agentId: "pixelfix-agent", version: 3, live: true, kind: "metric", severity: "green", team: "design", summary: "Pixel audit tracking visual drift across launch surfaces." },
  { id: "code-review-902", ticket: "REV-902", title: "Payments review", agentId: "reviewer-agent", version: 7, live: false, kind: "tasks", severity: "amber", team: "eng", summary: "Code review checklist for payments retry behavior." },
  { id: "incident-1182", ticket: "INC-1182", title: "Live auth p99", agentId: "ops-runbook", version: 12, live: true, kind: "incident", severity: "red", team: "ops", summary: "Incident runbook for auth latency and Redis recovery." },
  { id: "spike-vec-search", ticket: "SPK-44", title: "Vector search bench", agentId: "bench-agent", version: 2, live: false, kind: "chart", severity: "green", team: "eng", summary: "Benchmark comparison across vector stores." },
  { id: "onboarding-flow", ticket: "DSG-71", title: "Onboarding flow", agentId: "design-critique", version: 4, live: false, kind: "design", severity: "green", team: "design", summary: "Design review for first-run onboarding screens." },
  { id: "launch-tracker", ticket: "LCH-7", title: "Launch readiness", agentId: "tracker-agent", version: 18, live: true, kind: "tasks", severity: "green", team: "pm", summary: "Launch readiness checklist for GA." },
  { id: "mig-postgres-15", ticket: "INF-301", title: "Postgres 15 cutover", agentId: "infra-agent", version: 6, live: false, kind: "tasks", severity: "amber", team: "infra", summary: "Cutover plan for the Postgres 15 migration." },
  { id: "cust-feedback-q2", ticket: "PRD-118", title: "Q2 feedback themes", agentId: "voc-agent", version: 3, live: true, kind: "chart", severity: "green", team: "pm", summary: "Voice-of-customer themes for Q2 planning." },
  { id: "sec-audit-jul", ticket: "SEC-22", title: "July security audit", agentId: "sec-scan", version: 2, live: true, kind: "metric", severity: "red", team: "sec", summary: "Security audit scorecard for July findings." },
  { id: "cost-cloud-may", ticket: "FIN-9", title: "May cloud cost", agentId: "finops-agent", version: 5, live: true, kind: "chart", severity: "amber", team: "infra", summary: "FinOps chart for May cloud spend." },
  { id: "api-changelog", ticket: "API-44", title: "API changelog v2", agentId: "docs-agent", version: 9, live: false, kind: "tasks", severity: "green", team: "eng", summary: "API v2 migration and changelog checklist." },
  { id: "churn-cohort", ticket: "GRW-12", title: "Churn cohort Apr", agentId: "growth-agent", version: 1, live: false, kind: "chart", severity: "green", team: "pm", summary: "April churn cohort review for growth planning." },
];

export function priorityFor(severity) {
  if (severity === "red") return "urgent";
  if (severity === "amber") return "high";
  return "normal";
}

export function canvasPayload(fixture) {
  return {
    id: fixture.id,
    agentId: fixture.agentId,
    runId: `run:${fixture.id}`,
    title: fixture.title,
    summary: fixture.summary,
    status: fixture.live ? "in_progress" : "ready_for_review",
    mode: "static",
    priority: priorityFor(fixture.severity),
    version: fixture.version,
    blocks: [
      {
        id: `${fixture.id}.heading`,
        kind: "heading",
        level: 1,
        text: fixture.title,
      },
      {
        id: `${fixture.id}.dashboard`,
        kind: "metadata",
        title: "dashboard",
        metadata: dashboardMetadata(fixture),
      },
      {
        id: `${fixture.id}.summary`,
        kind: "markdown",
        markdown: fixture.summary,
      },
    ],
  };
}

export function dashboardMetadata(fixture) {
  return {
    ticket: fixture.ticket,
    live: fixture.live,
    kind: fixture.kind,
    severity: fixture.severity,
    team: fixture.team,
    viewers: fixture.live ? 12 + fixture.version : 3 + fixture.version,
  };
}

export function dashboardBlock(canvas) {
  return canvas.blocks?.find((block) => block.kind === "metadata" && block.title === "dashboard");
}

export function sameDashboardCanvas(actual, expected) {
  const actualMeta = dashboardBlock(actual)?.metadata || {};
  const expectedMeta = dashboardBlock(expected)?.metadata || {};

  return actual.id === expected.id
    && actual.agentId === expected.agentId
    && actual.runId === expected.runId
    && actual.title === expected.title
    && actual.summary === expected.summary
    && actual.status === expected.status
    && actual.mode === expected.mode
    && actual.priority === expected.priority
    && actual.version === expected.version
    && JSON.stringify(actualMeta) === JSON.stringify(expectedMeta);
}

export function hubRequestConfig() {
  return {
    hubBaseURL: process.env.HUB_BASE_URL || defaultHubBaseURL,
    token: process.env.HUB_TOKEN || defaultHubToken,
  };
}

export async function hubFetch(path, init = {}) {
  const { hubBaseURL, token } = hubRequestConfig();
  const response = await fetch(new URL(path, hubBaseURL), {
    ...init,
    headers: {
      "Accept": "application/json",
      "Authorization": `Bearer ${token}`,
      ...init.headers,
    },
  });

  if (!response.ok) {
    const body = await response.text();
    throw new Error(`${init.method || "GET"} ${path} failed: ${response.status} ${body}`);
  }

  return response;
}
