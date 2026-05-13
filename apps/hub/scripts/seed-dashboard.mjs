import {
  canvasPayload,
  dashboardFixtures,
  hubFetch,
  hubRequestConfig,
  sameDashboardCanvas,
} from "./dashboard-fixtures.mjs";

const force = process.env.SEED_FORCE === "1" || process.argv.includes("--force");
const { hubBaseURL } = hubRequestConfig();

const existing = await hubFetch("/v1/canvases").then((response) => response.json());
const existingById = new Map(existing.map((canvas) => [canvas.id, canvas]));

let published = 0;
let skipped = 0;

for (const fixture of dashboardFixtures) {
  const payload = canvasPayload(fixture);
  const current = existingById.get(payload.id);

  if (!force && current && sameDashboardCanvas(current, payload)) {
    skipped += 1;
    console.log(`naive ${payload.agentId}: skipped ${payload.id}`);
    continue;
  }

  await hubFetch("/v1/canvases", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });

  published += 1;
  console.log(`naive ${payload.agentId}: published ${payload.id}`);
}

console.log(`dashboard seed complete: ${published} published, ${skipped} unchanged, hub=${hubBaseURL}`);
