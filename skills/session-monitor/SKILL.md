---
name: session-monitor
description: Operate Knify's daemon-based local session monitor for Codex and Claude Code JSONL session files.
---

# Session Monitor

Use this skill only as a thin operator entry point for the daemon in `apps/session-monitor`.

The monitor itself is intentionally almost skill-less:

- it runs as a Go daemon
- it checks provider session files on disk, usually JSONL
- it watches by polling for new or changed sessions
- it publishes canvas state to an AgentCanvas-compatible `/v1` hub
- it does not need agent-side skill logic to parse, match, publish, or maintain state

## Runtime Rules

- Work from the Knify monorepo root when available.
- Keep behavior in `apps/session-monitor`, not in this skill.
- Treat CanvasHub as an external compatible API target.
- Do not assume the old Go hub, iOS app, web app, Slack app, deploy scripts, or scenario suites are present.
- The daemon reads local provider stores only. Do not write to `~/.codex`, `~/.claude`, or Cursor state.

## Commands

```sh
make session-monitor-test
make session-monitor-build
cd apps/session-monitor && go run . --once --dry-run --out /tmp/session-monitor-result.json
cd apps/session-monitor && go run . --watch --dynamic --interval 1m --hub-url "$SESSION_MONITOR_HUB_URL" --token "$SESSION_MONITOR_HUB_TOKEN"
make session-monitor-status
```

For flags and environment variables, use `apps/session-monitor/README.md`.

## Visual Decision Reviewer

For screenshot-based UX review of the session monitor, spawn a Codex subagent with `visual-decision-reviewer.md` as the reviewer prompt. The reviewer reports back to the current thread; it does not submit Hub feedback, auto-nudge sessions, or write provider state.

## Deployment Through `npx skills`

Use the root Make targets so the command sequence stays consistent:

```sh
make skill-session-monitor-validate
make skill-session-monitor-install SKILL_AGENT=codex
make skill-session-monitor-deploy
```

`skill-session-monitor-install` installs for one agent. `skill-session-monitor-deploy` installs globally for every agent supported by the local `npx skills` CLI.
