#!/usr/bin/env node
// Naive reviewer for benchmark canvases. It reads the live canvas payloads and
// the screenshot evidence written by bench-capture.mjs, then checks that every
// benchmark has a non-empty capture and structurally valid html blocks.
//
// Usage:
//   HUB_TOKEN=agentcanvas-dev-token HUB_BASE_URL=http://127.0.0.1:8799 \
//   SITE_BASE_URL=http://127.0.0.1:5173 \
//     node apps/hub/scripts/bench-review.mjs --prefix bench --out bench-out
//
// Notes:
//   - This is a "naive structural" reviewer — fast, deterministic, no LLM.
//     Pair with a vision-model pass over bench-out/*/page.png for subjective
//     UX checks.

import { existsSync, mkdirSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";

import { hubFetch } from "./dashboard-fixtures.mjs";

const args = parseArgs(process.argv.slice(2));
const prefix = String(args.prefix || "bench");
const outDir = args.out || join(process.cwd(), "bench-out");

mkdirSync(outDir, { recursive: true });

const allCanvases = await hubFetch("/v1/canvases").then((r) => r.json());
const canvases = allCanvases.filter((canvas) => canvas.id.startsWith(`${prefix}-`));
console.log(`bench-review: ${canvases.length} canvases (prefix=${prefix}), out=${outDir}`);

const rows = [];
for (const canvas of canvases) {
  const row = reviewOne(canvas);
  rows.push(row);
  const tag = row.verdict === "PASS" ? "PASS" : "FAIL";
  console.log(`  ${tag} ${canvas.id}: ${row.failed.length} fail(s), ${row.passed.length} pass(es)`);
}

rows.sort((a, b) => a.id.localeCompare(b.id));
const summary = {
  total: rows.length,
  passed: rows.filter((r) => r.verdict === "PASS").length,
  failed: rows.filter((r) => r.verdict === "FAIL").length,
  rows,
};
writeFileSync(join(outDir, "review.json"), JSON.stringify(summary, null, 2) + "\n");
writeFileSync(join(outDir, "review.md"), renderMarkdown(summary));
console.log(`bench-review: ${summary.passed}/${summary.total} PASS, ${summary.failed} FAIL — report at ${join(outDir, "review.md")}`);
process.exit(summary.failed === 0 ? 0 : 1);

function reviewOne(canvas) {
  const passed = [];
  const failed = [];

  if (typeof canvas.title === "string" && canvas.title.trim()) passed.push({ check: "title", detail: canvas.title });
  else failed.push({ check: "title", detail: "missing title" });

  const capturePath = join(outDir, canvas.id, "page.png");
  if (existsSync(capturePath) && statSync(capturePath).size > 0) {
    passed.push({ check: "capture", detail: capturePath });
  } else {
    failed.push({ check: "capture", detail: `missing non-empty ${capturePath}` });
  }

  if (Array.isArray(canvas.blocks) && canvas.blocks.length > 0) passed.push({ check: "blocks", detail: `${canvas.blocks.length} block(s)` });
  else failed.push({ check: "blocks", detail: "canvas has no blocks" });

  for (const block of canvas.blocks || []) {
    if (typeof block.id === "string" && block.id.trim()) passed.push({ check: `block-id:${block.id}`, detail: block.kind });
    else failed.push({ check: "block-id", detail: `${block.kind || "unknown"} block missing id` });
  }

  for (const block of (canvas.blocks || []).filter((b) => b.kind === "html")) {
    if (block.html) {
      passed.push({ check: `html-inline:${block.id}`, detail: `${Buffer.byteLength(block.html)} bytes` });
      if (block.sandbox === "strict" || block.sandbox === "relaxed") passed.push({ check: `sandbox:${block.id}`, detail: block.sandbox });
      else failed.push({ check: `sandbox:${block.id}`, detail: `expected strict or relaxed, got ${JSON.stringify(block.sandbox)}` });
      if (typeof block.height === "number" && block.height > 0 && block.height <= 1600) passed.push({ check: `height:${block.id}`, detail: String(block.height) });
      else failed.push({ check: `height:${block.id}`, detail: `invalid height ${JSON.stringify(block.height)}` });
    } else if (block.screenshotAssetId) {
      passed.push({ check: `html-screenshot:${block.id}`, detail: block.screenshotAssetId });
    } else {
      failed.push({ check: `html-payload:${block.id}`, detail: "html block has neither html nor screenshotAssetId" });
    }
  }

  return {
    id: canvas.id,
    title: canvas.title,
    verdict: failed.length === 0 ? "PASS" : "FAIL",
    passed,
    failed,
  };
}

function renderMarkdown(summary) {
  const lines = [];
  lines.push("# Bench review report");
  lines.push("");
  lines.push(`- Total: ${summary.total}`);
  lines.push(`- PASS: ${summary.passed}`);
  lines.push(`- FAIL: ${summary.failed}`);
  lines.push("");
  lines.push("| Canvas | Verdict | Failed checks |");
  lines.push("|---|---|---|");
  for (const row of summary.rows) {
    const fails = row.failed.length ? row.failed.map((f) => `${f.check}: ${f.detail}`).join("; ") : "—";
    lines.push(`| ${row.id} | ${row.verdict} | ${fails.replace(/\|/g, "\\|")} |`);
  }
  lines.push("");
  return lines.join("\n");
}

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
