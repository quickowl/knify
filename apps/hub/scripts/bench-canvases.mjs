#!/usr/bin/env node
// Generate varied benchmark canvases and POST them to the local hub. Each
// fixture exercises a different rendering path so the naive reviewer can
// spot regressions across the matrix.
//
// Usage:
//   HUB_TOKEN=agentcanvas-dev-token HUB_BASE_URL=http://127.0.0.1:8799 \
//     node apps/hub/scripts/bench-canvases.mjs [--scale 1]
//
// Flags:
//   --scale <n>    multiply the matrix N times with rotating variants (default 1)
//   --prefix <s>   id prefix, default "bench"

import { hubFetch } from "./dashboard-fixtures.mjs";

const args = parseArgs(process.argv.slice(2));
const scale = Math.max(1, Number(args.scale || 1));
const prefix = String(args.prefix || "bench");

const baseFixtures = buildBaseFixtures();
console.log(`bench-canvases: matrix size = ${baseFixtures.length}, scale = ${scale}`);

let published = 0;
let failed = 0;
const ids = [];

for (let seed = 0; seed < scale; seed += 1) {
  for (const fixture of baseFixtures) {
    const id = `${prefix}-${fixture.slug}-${seed}`;
    const canvas = renderFixture(id, fixture, seed);
    try {
      const response = await hubFetch("/v1/canvases", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(canvas),
      });
      await response.json();
      published += 1;
      ids.push(id);
      if ((published % 20) === 0) console.log(`  ${published} published…`);
    } catch (err) {
      failed += 1;
      console.error(`  ${id}: ${err.message?.slice(0, 200)}`);
    }
  }
}

console.log(`bench-canvases: ${published} published, ${failed} failed`);
process.stdout.write(JSON.stringify({ ids }) + "\n");

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

function buildBaseFixtures() {
  return [
    {
      slug: "html-card-light",
      title: "HTML card — light gradient",
      summary: "Single html block on a light gradient — typical agent report.",
      blocks: [headingBlock("h1", "Light gradient card"), htmlCardLight()],
    },
    {
      slug: "html-card-dark",
      title: "HTML card — dark mode",
      summary: "Same card on a dark background to check contrast.",
      blocks: [headingBlock("h1", "Dark mode card"), htmlCardDark()],
    },
    {
      slug: "html-table-wide",
      title: "Wide data table",
      summary: "html block with a 6-column table; checks horizontal overflow.",
      blocks: [headingBlock("h1", "Wide table"), htmlTableWide()],
    },
    {
      slug: "html-chart-svg",
      title: "Inline SVG chart",
      summary: "html block with an inline SVG sparkline.",
      blocks: [headingBlock("h1", "Inline SVG"), htmlChartSVG()],
    },
    {
      slug: "html-rtl",
      title: "RTL text rendering",
      summary: "html block with Arabic text and dir=rtl.",
      blocks: [headingBlock("h1", "Right-to-left"), htmlRTL()],
    },
    {
      slug: "html-cjk",
      title: "CJK text rendering",
      summary: "html block with Japanese + Chinese mixed glyphs.",
      blocks: [headingBlock("h1", "CJK"), htmlCJK()],
    },
    {
      slug: "html-emoji",
      title: "Emoji-heavy report",
      summary: "html block with status-line emoji.",
      blocks: [headingBlock("h1", "Emoji status"), htmlEmoji()],
    },
    {
      slug: "html-fallback-only",
      title: "Screenshot-only path",
      summary: "html block carrying no inline html — only screenshotAssetId.",
      blocks: [headingBlock("h1", "Screenshot only"), htmlScreenshotOnly()],
      needsFallbackAsset: true,
    },
    {
      slug: "html-mixed-markdown",
      title: "Markdown + html mix",
      summary: "Markdown narrative interleaved with two html blocks.",
      blocks: [headingBlock("h1", "Mixed surface"), markdownBlock("md-intro", "Narrative paragraph above the report."), htmlCardLight("html-a"), markdownBlock("md-mid", "**Conclusion follows.**"), htmlCardDark("html-b")],
    },
    {
      slug: "html-with-checklist",
      title: "html beside checklist",
      summary: "Common case: html report next to a status checklist.",
      blocks: [headingBlock("h1", "Status"), checklistBlock(), htmlCardLight()],
    },
    {
      slug: "html-min-height",
      title: "Tiny html block",
      summary: "html block with explicit small height.",
      blocks: [headingBlock("h1", "Small surface"), htmlMinHeight()],
    },
    {
      slug: "html-tall",
      title: "Tall html block",
      summary: "html block with a tall iframe and lots of content.",
      blocks: [headingBlock("h1", "Tall surface"), htmlTall()],
    },
    {
      slug: "html-broken",
      title: "Malformed html",
      summary: "html block with intentionally broken markup to check robustness.",
      blocks: [headingBlock("h1", "Broken markup"), htmlBroken()],
    },
    {
      slug: "html-script-strict",
      title: "Strict sandbox blocks script",
      summary: "html block with inline script under strict sandbox — should not execute.",
      blocks: [headingBlock("h1", "Strict sandbox"), htmlScriptStrict()],
    },
    {
      slug: "html-script-relaxed",
      title: "Relaxed sandbox allows script",
      summary: "html block with inline script under relaxed sandbox — script runs.",
      blocks: [headingBlock("h1", "Relaxed sandbox"), htmlScriptRelaxed()],
    },
  ];
}

function renderFixture(id, fixture, seed) {
  return {
    id,
    agentId: `bench-${seed}`,
    runId: `run-${id}`,
    title: fixture.title,
    summary: fixture.summary,
    status: "ready_for_review",
    priority: "normal",
    version: 1,
    blocks: fixture.blocks.map((block, idx) => ({ ...block, id: block.id || `b-${idx + 1}` })),
  };
}

function headingBlock(level, text) {
  return { kind: "heading", level: level === "h1" ? 1 : 2, text };
}

function markdownBlock(id, markdown) {
  return { id, kind: "markdown", markdown };
}

function checklistBlock() {
  return {
    kind: "checklist",
    title: "Gates",
    items: [
      { id: "g1", text: "Build passes", checked: true },
      { id: "g2", text: "Lint passes", checked: true },
      { id: "g3", text: "Manual smoke", checked: false },
    ],
  };
}

function htmlCardLight(id = "html-card-light") {
  return {
    id,
    kind: "html",
    title: "Inline report",
    sandbox: "strict",
    height: 260,
    html: `<!doctype html><meta charset="utf-8"><style>body{margin:0;font-family:-apple-system,BlinkMacSystemFont,system-ui,sans-serif;background:linear-gradient(135deg,#fef9c3,#fce7f3);padding:18px;color:#1f2937}h1{margin:0 0 8px;font-size:20px}.row{display:flex;gap:8px;margin:8px 0}.chip{padding:6px 10px;border-radius:999px;background:rgba(255,255,255,.7);font-size:12px;font-family:ui-monospace,Menlo,monospace}table{width:100%;border-collapse:collapse;margin-top:10px;background:rgba(255,255,255,.6);border-radius:8px;overflow:hidden}th,td{padding:6px 10px;text-align:left;font-size:12px;border-bottom:1px solid rgba(0,0,0,.06)}th{font-weight:600;background:rgba(255,255,255,.4)}</style><h1>Agent report</h1><div class=row><span class=chip>session: alpha</span><span class=chip>artifacts: 4</span><span class=chip>evidence: ready</span></div><table><tr><th>file</th><th>kind</th><th>size</th></tr><tr><td>review.md</td><td>markdown</td><td>20 B</td></tr><tr><td>cli.log</td><td>terminal</td><td>20 B</td></tr><tr><td>screen.png</td><td>image</td><td>82 B</td></tr><tr><td>report.html</td><td>html</td><td>318 B</td></tr></table>`,
  };
}

function htmlCardDark(id = "html-card-dark") {
  return {
    id,
    kind: "html",
    title: "Inline report (dark)",
    sandbox: "strict",
    height: 260,
    html: `<!doctype html><meta charset="utf-8"><style>body{margin:0;font-family:system-ui,sans-serif;background:#0f172a;padding:18px;color:#e2e8f0}h1{margin:0 0 6px;font-size:20px;color:#fafafa}p{margin:0;color:#94a3b8}.grid{display:grid;grid-template-columns:repeat(3,1fr);gap:8px;margin-top:14px}.tile{padding:12px;border:1px solid #1e293b;border-radius:8px;background:#111827}.tile strong{font-size:18px;color:#fafafa}.tile span{font-size:11px;text-transform:uppercase;letter-spacing:.05em;color:#64748b}</style><h1>Pipeline pulse</h1><p>Last 24h, dark theme.</p><div class=grid><div class=tile><strong>318</strong><br><span>runs</span></div><div class=tile><strong>97%</strong><br><span>success</span></div><div class=tile><strong>11</strong><br><span>flakes</span></div></div>`,
  };
}

function htmlTableWide() {
  const rows = Array.from({ length: 8 }, (_, i) => `<tr><td>row-${i + 1}</td><td>codex</td><td>${i % 2 ? "running" : "idle"}</td><td>${Math.round(Math.random() * 100)} ms</td><td>v${i + 1}</td><td>—</td></tr>`).join("");
  return {
    kind: "html",
    title: "Wide table",
    sandbox: "strict",
    height: 320,
    html: `<!doctype html><meta charset="utf-8"><style>body{margin:0;padding:14px;background:#f8fafc;font-family:system-ui,sans-serif}table{width:100%;border-collapse:collapse;font-size:12px}th,td{padding:6px 10px;border-bottom:1px solid #e2e8f0;text-align:left}th{background:#e0e7ff}</style><table><tr><th>id</th><th>provider</th><th>state</th><th>latency</th><th>version</th><th>note</th></tr>${rows}</table>`,
  };
}

function htmlChartSVG() {
  return {
    kind: "html",
    title: "Inline SVG sparkline",
    sandbox: "strict",
    height: 220,
    html: `<!doctype html><meta charset="utf-8"><style>body{margin:0;padding:16px;background:#fff7ed;font-family:system-ui,sans-serif}svg{width:100%;height:140px;background:#fff;border:1px solid #fed7aa;border-radius:8px}</style><h2 style="margin:0 0 8px;font-size:14px">Throughput, last hour</h2><svg viewBox="0 0 200 60"><polyline fill="none" stroke="#ea580c" stroke-width="2" points="0,40 20,28 40,32 60,18 80,24 100,12 120,22 140,14 160,8 180,18 200,10"/><circle cx="200" cy="10" r="3" fill="#ea580c"/></svg>`,
  };
}

function htmlRTL() {
  return {
    kind: "html",
    title: "Right-to-left",
    sandbox: "strict",
    height: 180,
    html: `<!doctype html><meta charset="utf-8"><body dir="rtl" style="margin:0;padding:18px;font-family:system-ui,sans-serif;background:#ecfeff"><h2 style="margin:0 0 6px">تقرير الجلسة</h2><p style="margin:0">تم تشغيل الجلسة بنجاح. لا توجد أخطاء.</p><ul><li>الملف الأول</li><li>الملف الثاني</li></ul></body>`,
  };
}

function htmlCJK() {
  return {
    kind: "html",
    title: "CJK rendering",
    sandbox: "strict",
    height: 180,
    html: `<!doctype html><meta charset="utf-8"><body style="margin:0;padding:18px;font-family:system-ui,sans-serif;background:#fdf2f8"><h2 style="margin:0 0 6px">セッションレポート / 会话报告</h2><p>すべてのテストが合格しました。所有测试均已通过。</p></body>`,
  };
}

function htmlEmoji() {
  return {
    kind: "html",
    title: "Emoji status",
    sandbox: "strict",
    height: 160,
    html: `<!doctype html><meta charset="utf-8"><body style="margin:0;padding:18px;font-family:system-ui,sans-serif;background:#f0fdf4"><h2 style="margin:0 0 8px">Run summary</h2><p style="font-size:22px;margin:0">✅ 12 &nbsp; ⚠️ 2 &nbsp; ❌ 0 &nbsp; ⏱ 4m12s</p></body>`,
  };
}

function htmlScreenshotOnly() {
  // We attach a placeholder screenshotAssetId; the screenshot script can
  // later overwrite it with a real rendered PNG.
  return {
    kind: "html",
    title: "Screenshot only",
    caption: "No inline html. Reviewer sees only the rendered PNG.",
    screenshotAssetId: "asset-placeholder-bench",
  };
}

function htmlMinHeight() {
  return {
    kind: "html",
    title: "Tiny",
    sandbox: "strict",
    height: 80,
    html: `<!doctype html><meta charset="utf-8"><body style="margin:0;padding:6px;font-family:system-ui;background:#fff5f5;font-size:12px">Single-line html block.</body>`,
  };
}

function htmlTall() {
  const lines = Array.from({ length: 28 }, (_, i) => `<p style="margin:4px 0">Line ${i + 1} — agent observation about something interesting that happened.</p>`).join("");
  return {
    kind: "html",
    title: "Tall surface",
    sandbox: "strict",
    height: 600,
    html: `<!doctype html><meta charset="utf-8"><body style="margin:0;padding:14px;background:#eff6ff;font-family:system-ui,sans-serif;font-size:13px;line-height:1.5">${lines}</body>`,
  };
}

function htmlBroken() {
  return {
    kind: "html",
    title: "Malformed",
    sandbox: "strict",
    height: 200,
    html: `<!doctype html><body style="background:#fef2f2;padding:14px;font-family:system-ui"><h2>Broken markup test</h2><p>Unclosed div below <div>Still readable<p>And another paragraph.`,
  };
}

function htmlScriptStrict() {
  return {
    kind: "html",
    title: "Strict sandbox + script",
    sandbox: "strict",
    height: 180,
    html: `<!doctype html><body style="margin:0;padding:14px;background:#f8fafc;font-family:system-ui"><h2 id="hdr">Should stay STATIC</h2><script>document.getElementById('hdr').textContent='SCRIPT RAN';</script></body>`,
  };
}

function htmlScriptRelaxed() {
  return {
    kind: "html",
    title: "Relaxed sandbox + script",
    sandbox: "relaxed",
    height: 180,
    html: `<!doctype html><body style="margin:0;padding:14px;background:#fef9c3;font-family:system-ui"><h2 id="hdr">Should be replaced</h2><script>document.getElementById('hdr').textContent='SCRIPT RAN';</script></body>`,
  };
}
