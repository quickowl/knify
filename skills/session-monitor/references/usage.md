# Session Monitor Usage

The session monitor is a Go daemon in `apps/session-monitor`. It scans local Codex and Claude Code session stores, records Cursor CLI health metadata, matches recent sessions against hub canvases/runs, and publishes one review canvas to an AgentCanvas-compatible hub.

## Core Commands

```sh
make session-monitor-test
make session-monitor-build
make session-monitor-status
```

Dry-run:

```sh
cd apps/session-monitor
go run . --once --dry-run --out /tmp/session-monitor-result.json
```

Publish once:

```sh
cd apps/session-monitor
go run . --once --hub-url http://127.0.0.1:8787 --token agentcanvas-dev-token
```

Watch with dynamic canvas events:

```sh
cd apps/session-monitor
go run . --watch --dynamic --interval 1m --hub-url http://127.0.0.1:8787 --token agentcanvas-dev-token --out /tmp/session-monitor-live-watch.json
```

Status:

```sh
make session-monitor-status
```

## Important Flags

- `--once`: scan once and exit.
- `--watch`: scan repeatedly.
- `--status`: read the last heartbeat JSON and report daemon health.
- `--recent`: with `--status`, print newest tracked sessions.
- `--interval`: watch interval, such as `30s` or `1m`.
- `--stale-after`: maximum heartbeat age accepted by `--status`.
- `--dry-run`: build the scan result without publishing.
- `--dynamic`: publish live updates through `/v1/canvases/{canvasId}/events`.
- `--out`: write the full scan result or heartbeat JSON.
- `--hub-url`, `--token`: AgentCanvas-compatible hub endpoint and bearer token.
- `--canvas-id`, `--run-id`: review canvas/run IDs.
- `--codex-home`, `--claude-home`: provider home directories.
- `--lookback`, `--max-sessions`: cap included sessions.

## Environment

- `SESSION_MONITOR_HUB_URL`, then `HUB_BASE_URL`, then `AGENTCANVAS_HUB_URL`
- `SESSION_MONITOR_HUB_TOKEN`, then `HUB_TOKEN`, then `AGENTCANVAS_HUB_TOKEN`
- `SESSION_MONITOR_CANVAS_ID=canvas.session-monitor.local`
- `SESSION_MONITOR_RUN_ID=run.session-monitor.local`
- `SESSION_MONITOR_LOOKBACK_HOURS=24`
- `SESSION_MONITOR_MAX_SESSIONS=30`
- `SESSION_MONITOR_INTERVAL=1m`
- `SESSION_MONITOR_OUT=/tmp/session-monitor-live-watch.json`
- `SESSION_MONITOR_STALE_AFTER=3m`
- `SESSION_MONITOR_RECENT=12`
- `SESSION_MONITOR_DYNAMIC=false`

## Provider Sources

- Codex: `~/.codex/sessions/**/*.jsonl`, `~/.codex/session_index.jsonl`, and read-only SQLite access to `~/.codex/logs_2.sqlite` when `sqlite3` is available.
- Claude Code: `~/.claude/projects/**/*.jsonl`, `~/.claude/sessions/*.json`, and `~/.claude/history.jsonl`.
- Cursor: v1 checks whether `agent`, `cursor-agent`, or `cursor` is discoverable. Full transcript parsing is deferred.

The daemon does not nudge, resume, or edit local sessions in v1.
