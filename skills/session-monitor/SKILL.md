---
name: session-monitor
description: Run and operate Knify's AgentCanvas-compatible local session monitor for Codex and Claude Code sessions, including dry-runs, hub publishes, live dynamic updates, and health checks.
---

# Session Monitor

Use this skill when the user wants to inspect local agent session state, publish a session review canvas, check monitor health, or wire Knify's session monitor to an AgentCanvas-compatible `/v1` hub.

## Scope

- Work from the Knify monorepo root when available.
- Use `apps/session-monitor` as the daemon source.
- Treat CanvasHub as an external compatible `/v1` API target.
- Do not import or assume the old Go hub, iOS app, web app, Slack app, deploy scripts, or scenario suites.
- The monitor reads local provider stores only. Do not write to `~/.codex`, `~/.claude`, or Cursor state.

## Standard Workflow

1. Verify the monitor builds and tests:

   ```sh
   make session-monitor-test
   make session-monitor-build
   ```

2. Run a read-only dry-run before publishing:

   ```sh
   cd apps/session-monitor
   go run . --once --dry-run --out /tmp/session-monitor-result.json
   ```

3. Publish only when a hub URL and token are known:

   ```sh
   cd apps/session-monitor
   go run . --once --hub-url "$SESSION_MONITOR_HUB_URL" --token "$SESSION_MONITOR_HUB_TOKEN"
   ```

4. For a live review canvas, use watch mode with dynamic events:

   ```sh
   cd apps/session-monitor
   go run . --watch --dynamic --interval 1m --hub-url "$SESSION_MONITOR_HUB_URL" --token "$SESSION_MONITOR_HUB_TOKEN" --out /tmp/session-monitor-live-watch.json
   ```

5. Check daemon health:

   ```sh
   make session-monitor-status
   ```

## Deployment Through `npx skills`

Use the root Make targets so the command sequence stays consistent:

```sh
make skill-session-monitor-validate
make skill-session-monitor-install SKILL_AGENT=codex
make skill-session-monitor-deploy
```

`skill-session-monitor-install` installs for one agent. `skill-session-monitor-deploy` installs globally for every agent supported by the local `npx skills` CLI.

See `references/usage.md` for flags, environment variables, and common operating modes.
