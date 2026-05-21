#!/usr/bin/env node
// Walk every benchmark canvas on the hub, headless-Chrome the viewer page,
// and dump PNGs under bench/<id>/page.png. Use as input to bench-review.mjs.
//
// Usage:
//   HUB_TOKEN=agentcanvas-dev-token HUB_BASE_URL=http://127.0.0.1:8799 \
//   SITE_BASE_URL=http://127.0.0.1:5173 \
//     node apps/hub/scripts/bench-capture.mjs [--prefix bench]

import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

import { hubFetch } from "./dashboard-fixtures.mjs";

const args = parseArgs(process.argv.slice(2));
const siteBaseURL = (process.env.SITE_BASE_URL || "http://127.0.0.1:5173").replace(/\/+$/, "");
const chromePath = args.chrome || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const prefix = String(args.prefix || "bench");
const outDir = args.out || join(process.cwd(), "bench-out");
const concurrency = Math.max(1, Number(args.concurrency || 3));

mkdirSync(outDir, { recursive: true });

const canvases = await hubFetch("/v1/canvases").then((r) => r.json());
const targets = canvases.filter((canvas) => canvas.id.startsWith(`${prefix}-`));
console.log(`bench-capture: ${targets.length} canvases match prefix=${prefix}, out=${outDir}, site=${siteBaseURL}`);

let captured = 0;
let failed = 0;
const summaries = [];

const queue = [...targets];
const workers = Array.from({ length: concurrency }, async () => {
  while (queue.length) {
    const canvas = queue.shift();
    if (!canvas) return;
    const url = `${siteBaseURL}/canvases/${encodeURIComponent(canvas.id)}`;
    const dir = join(outDir, canvas.id);
    mkdirSync(dir, { recursive: true });
    const pngPath = join(dir, "page.png");
    const result = spawnSync(chromePath, [
      "--headless=new",
      "--disable-gpu",
      "--hide-scrollbars",
      "--no-sandbox",
      "--window-size=1200,1600",
      `--screenshot=${pngPath}`,
      url,
    ], { stdio: ["ignore", "pipe", "pipe"], timeout: 30_000 });
    if (result.status !== 0) {
      failed += 1;
      const stderr = (result.stderr || "").toString().trim().slice(0, 200);
      console.error(`  ${canvas.id}: chrome failed (${result.status}): ${stderr}`);
      summaries.push({ id: canvas.id, ok: false, error: stderr });
      continue;
    }
    captured += 1;
    summaries.push({ id: canvas.id, ok: true, png: pngPath, title: canvas.title });
    if ((captured % 5) === 0) console.log(`  ${captured} captured…`);
  }
});

await Promise.all(workers);

writeFileSync(join(outDir, "captures.json"), JSON.stringify(summaries, null, 2) + "\n");
console.log(`bench-capture: ${captured} captured, ${failed} failed; index at ${join(outDir, "captures.json")}`);

function parseArgs(list) {
  const out = {};
  for (let i = 0; i < list.length; i += 1) {
    const token = list[i];
    if (!token.startsWith("--")) continue;
    const name = token.slice(2);
    const next = list[i + 1];
    if (next === undefined || next.startsWith("--")) {
      out[name] = true;
    } else {
      out[name] = next;
      i += 1;
    }
  }
  return out;
}
