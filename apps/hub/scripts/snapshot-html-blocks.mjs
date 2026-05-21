#!/usr/bin/env node
// Render every html block on a canvas with headless Chrome, upload the PNG
// as an asset, and re-publish the canvas with screenshotAssetId set on the
// block. Run after the demo hub is up.
//
// Usage:
//   HUB_TOKEN=agentcanvas-dev-token HUB_BASE_URL=http://127.0.0.1:8799 \
//     node apps/hub/scripts/snapshot-html-blocks.mjs --canvas canvas-html-demo
//
// Flags:
//   --canvas <id>       canvas to walk (required)
//   --chrome <path>     chrome/chromium binary; default macOS path
//   --width <px>        viewport width, default 800
//   --height <px>       viewport height, default 600
//   --force             re-snapshot blocks that already have screenshotAssetId

import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

import { hubFetch, hubRequestConfig } from "./dashboard-fixtures.mjs";

const args = parseArgs(process.argv.slice(2));
const canvasId = args.canvas;
if (!canvasId) {
  console.error("error: --canvas <id> is required");
  process.exit(2);
}

const chromePath = args.chrome || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const width = Number(args.width || 800);
const height = Number(args.height || 600);
const force = args.force === true;

const { hubBaseURL } = hubRequestConfig();
console.log(`snapshot html blocks: canvas=${canvasId} hub=${hubBaseURL}`);

const canvasResponse = await hubFetch(`/v1/canvases/${encodeURIComponent(canvasId)}`);
const canvas = await canvasResponse.json();

const htmlBlocks = canvas.blocks.filter((block) => block.kind === "html" && block.html && (force || !block.screenshotAssetId));
if (htmlBlocks.length === 0) {
  console.log("no html blocks with inline html that need a screenshot — nothing to do.");
  process.exit(0);
}

const workdir = mkdtempSync(join(tmpdir(), "snapshot-html-"));
let changed = 0;

for (const block of htmlBlocks) {
  const htmlPath = join(workdir, `${block.id}.html`);
  const pngPath = join(workdir, `${block.id}.png`);
  writeFileSync(htmlPath, wrapHTML(block.html));

  const result = spawnSync(chromePath, [
    "--headless=new",
    "--disable-gpu",
    "--hide-scrollbars",
    "--no-sandbox",
    `--window-size=${width},${height}`,
    `--screenshot=${pngPath}`,
    `file://${htmlPath}`,
  ], { stdio: ["ignore", "pipe", "pipe"], timeout: 30_000 });

  if (result.status !== 0) {
    const stderr = (result.stderr || "").toString().trim();
    console.error(`block ${block.id}: chrome failed (${result.status}): ${stderr.slice(0, 200)}`);
    continue;
  }

  const png = readFileSync(pngPath);
  const upload = await hubFetch("/v1/assets", {
    method: "POST",
    headers: { "Content-Type": "image/png" },
    body: png,
  });
  const asset = await upload.json();
  block.screenshotAssetId = asset.assetId || asset.id;
  // Strip server-decorated url; the hub sets it on read.
  delete block.screenshotUrl;
  changed += 1;
  console.log(`block ${block.id}: rendered ${png.length}B → asset ${block.screenshotAssetId}`);
}

if (changed === 0) {
  console.log("no blocks changed.");
  process.exit(0);
}

const next = {
  ...canvas,
  version: (canvas.version || 1) + 1,
  blocks: canvas.blocks,
};
// Strip read-time decorations the hub doesn't accept on POST.
for (const block of next.blocks) {
  if (block.kind === "image" && isDecoratedAssetURL(block.url, block.assetId)) delete block.url;
  delete block.screenshotUrl;
}

const republish = await hubFetch("/v1/canvases", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(next),
});
const published = await republish.json();
console.log(`canvas re-published as v${published.version} with ${changed} screenshot(s)`);

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

function wrapHTML(body) {
  const trimmed = (body || "").trim();
  if (/<!doctype/i.test(trimmed) || /<html[\s>]/i.test(trimmed)) return trimmed;
  return `<!doctype html><meta charset="utf-8"><body style="margin:0">${trimmed}</body>`;
}

function isDecoratedAssetURL(url, assetId) {
  return typeof url === "string"
    && typeof assetId === "string"
    && url.includes(`/assets/${encodeURIComponent(assetId)}`);
}
