#!/usr/bin/env node
import { spawn } from "node:child_process";
import { mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const addr = process.env.AGENTCANVAS_ADDR || "127.0.0.1:8787";
const [host, rawPort] = addr.includes(":") ? addr.split(":") : ["127.0.0.1", addr];
const port = process.env.PORT || rawPort || "8787";
const dataDir = resolve(process.env.AGENTCANVAS_DATA || ".data-worker");
const hubToken = process.env.HUB_TOKEN || process.env.AGENTCANVAS_TOKEN || "agentcanvas-dev-token";
mkdirSync(dirname(dataDir), { recursive: true });

const scriptDir = dirname(fileURLToPath(import.meta.url));
const wranglerBin = resolve(scriptDir, "../node_modules/wrangler/bin/wrangler.js");
const nodeBin = process.env.AGENTCANVAS_NODE_BIN || process.execPath;

const args = [
  wranglerBin,
  "dev",
  "--local",
  "--ip",
  host || "127.0.0.1",
  "--port",
  port,
  "--persist-to",
  dataDir,
  "--var",
  `HUB_TOKEN:${hubToken}`
];

const child = spawn(nodeBin, args, {
  stdio: "inherit",
  env: {
    ...process.env,
    HUB_TOKEN: hubToken
  }
});

child.on("error", (error) => {
  console.error(`failed to start local wrangler: ${error.message}`);
  process.exit(1);
});

child.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  process.exit(code ?? 0);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    child.kill(signal);
  });
}
