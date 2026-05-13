import {
  canvasPayload,
  dashboardFixtures,
  hubFetch,
  hubRequestConfig,
  sameDashboardCanvas,
} from "./dashboard-fixtures.mjs";

const { hubBaseURL } = hubRequestConfig();
const canvases = await hubFetch("/v1/canvases").then((response) => response.json());
const actualById = new Map(canvases.map((canvas) => [canvas.id, canvas]));
const failures = [];

for (const fixture of dashboardFixtures) {
  const expected = canvasPayload(fixture);
  const actual = actualById.get(expected.id);

  if (!actual) {
    failures.push(`${expected.id}: missing`);
    continue;
  }

  if (!sameDashboardCanvas(actual, expected)) {
    failures.push(`${expected.id}: shape mismatch`);
  }
}

if (failures.length) {
  for (const failure of failures) console.error(failure);
  throw new Error(`dashboard fixture check failed: ${failures.length} issue(s)`);
}

console.log(`dashboard fixture check passed: ${dashboardFixtures.length} canvases, hub=${hubBaseURL}`);
