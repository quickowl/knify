# Local Session Monitor

`session-monitor` is a small Go daemon that scans local Codex and Claude Code session stores, matches recent sessions against AgentCanvas hub canvases/runs, and publishes one static review canvas to the hub. Cursor is included as a v1 health check only.

The daemon reads provider state only. It never writes to `~/.codex`, `~/.claude`, or Cursor state.

## Build

```sh
go test ./...
go build -o session-monitor .
```

From the repo root:

```sh
make session-monitor-test
make session-monitor-build
```

## Run

Dry-run against local provider homes:

```sh
cd apps/session-monitor
go run . --once --dry-run --out /tmp/session-monitor-result.json
```

Publish once to a local hub:

```sh
cd apps/session-monitor
go run . \
  --once \
  --hub-url http://127.0.0.1:8787 \
  --token agentcanvas-dev-token
```

Watch mode:

```sh
cd apps/session-monitor
go run . --watch --interval 1m --hub-url http://127.0.0.1:8787
```

Live dynamic canvas mode:

```sh
cd apps/session-monitor
go run . --watch --dynamic --interval 1m --hub-url http://127.0.0.1:8787 --out /tmp/session-monitor-live-watch.json
```

Check daemon health from the latest heartbeat:

```sh
make session-monitor-status
```

or directly:

```sh
cd apps/session-monitor
go run . --status --out /tmp/session-monitor-live-watch.json --hub-url http://127.0.0.1:8787 --token agentcanvas-dev-token
```

Use `--recent 20` with `--status` to print the newest tracked sessions with provider/status/match, workspace, title, and latest capped message. `--status` exits non-zero when the heartbeat is stale, Codex/Claude health is not `ok`, the last scan had errors, the daemon PID is dead, or the hub canvas cannot be fetched. With no `--out`, status reads `/tmp/session-monitor-live-watch.json`.

## Configuration

Flags:

- `--once`: scan once and exit.
- `--watch`: scan repeatedly.
- `--status`: read the last heartbeat JSON and report whether the daemon and hub canvas look healthy.
- `--recent`: with `--status`, print this many newest tracked sessions.
- `--interval`: watch interval, such as `30s` or `1m`.
- `--stale-after`: maximum heartbeat age accepted by `--status`; default `3m`.
- `--dry-run`: build the scan result without publishing.
- `--dynamic`: publish through the AgentCanvas dynamic event protocol. In watch mode this keeps one canvas live by replacing the recent-session list and related blocks with `/v1/canvases/{canvasId}/events`.
- `--out`: write the full scan result JSON.
- `--hub-url`, `--token`: AgentCanvas hub endpoint and bearer token.
- `--canvas-id`, `--run-id`: review canvas/run IDs.
- `--codex-home`, `--claude-home`: provider home directories.
- `--lookback`, `--max-sessions`: cap included sessions.

Environment defaults:

- `SESSION_MONITOR_HUB_URL`, then `HUB_BASE_URL`, then `AGENTCANVAS_HUB_URL`
- `SESSION_MONITOR_HUB_TOKEN`, then `HUB_TOKEN`, then `AGENTCANVAS_HUB_TOKEN`
- `SESSION_MONITOR_CANVAS_ID=canvas.session-monitor.local`
- `SESSION_MONITOR_RUN_ID=run.session-monitor.local`
- `SESSION_MONITOR_LOOKBACK_HOURS=24`
- `SESSION_MONITOR_MAX_SESSIONS=30`
- `SESSION_MONITOR_INTERVAL=1m`
- `SESSION_MONITOR_OUT` for the heartbeat JSON path.
- `SESSION_MONITOR_STALE_AFTER=3m`
- `SESSION_MONITOR_RECENT` for how many newest rows `make session-monitor-status` prints; default `12`.
- `SESSION_MONITOR_DYNAMIC=false`

## Provider Sources

Codex sources:

- `~/.codex/sessions/**/*.jsonl`
- `~/.codex/session_index.jsonl`
- `~/.codex/logs_2.sqlite` when the local `sqlite3` CLI is available; it is opened through SQLite read-only URI mode.

Claude Code sources:

- `~/.claude/projects/**/*.jsonl`
- `~/.claude/sessions/*.json`
- `~/.claude/history.jsonl`

Cursor v1 checks whether a local CLI is discoverable (`agent`, `cursor-agent`, or `cursor`) and records health metadata. Full Cursor transcript parsing is deferred until there is a stable read-only local source.

Claude project scans collapse multiple JSONL files with the same session ID, including subagent files under a parent session directory, into one tracked session. The heartbeat keeps `sourceFiles`, `subagentFiles`, and `collapsedFiles` metadata so status output can explain why a row represents several local files.

## Matching

The daemon fetches existing hub state with `GET /v1/canvases` and `GET /v1/agent-runs`.

- `exact`: local session ID equals a canvas/run ID, run ID, run `externalId`, or feedback target `externalId`.
- `likely`: compatible provider family, same workspace/cwd, and updates within the recent time window.
- `unmatched`: no confident link.

The published canvas starts with a native `Recent sessions (newest first)` collection in paged-list mode instead of a large markdown transcript. Each row is structured as `Purpose`, `Now`, and `Next`:

- `Purpose`: the session title or inferred local task.
- `Now`: provider status, activity age, match state, collapse count, and latest capped message.
- `Next`: the recommended review action, such as keep watching, confirm a likely link, or decide whether an unmatched session should be linked or ignored.

Static and dynamic/watch publishes use the same list-shaped collection so the web and iPad renderers avoid horizontal-only rails and markdown line-flattening.

Scan warnings are summarized in the overview and moved into a collapsed metadata block so large parser paths do not dominate the review. The scanner accepts JSONL records up to 128 MiB; transcript-derived text is still capped before it is written to the canvas.

The published canvas also contains provider health, scan errors, capped latest-message summaries, match metadata, and resume hints. It does not nudge or resume sessions in v1. Codex `exec`/`resume` and Claude `--resume` metadata are recorded for a later explicit action path; Codex `app-server` remains experimental and is not required by core sync.

## Verification

```sh
cd apps/session-monitor && go test ./...
make session-monitor-build
make build-for-testing
scripts/sim/run-suite.sh tests/naive/M1/session-monitor-review-canvas.manifest.json
```
