---
type: Note
status: Research
related_to: "[[session-monitor]]"
---

# Mobile pairing for Codex and Claude Code

Research captured 2026-05-21. The goal was to figure out whether our `apps/session-monitor` (which tails JSONL files off disk) is the only path, or whether either provider exposes a sanctioned event stream we could subscribe to and feed into the hub.

## TL;DR

Both providers do have sanctioned realtime surfaces — neither of them is the mobile pairing relay itself, which is closed and host-restricted in both cases.

- **Codex**: drive `codex app-server` directly over stdio (or experimental WebSocket with signed-bearer auth). It is documented, third-party clients already do it (`nshkrdotcom/codex_sdk`), and the notifications match the granularity we need.
- **Claude Code**: ship a Claude Code Hooks plugin that POSTs `SessionStart`/`Stop`/`SubagentStop`/`PostToolUse`/`PreCompact` events (with `session_id` + `transcript_path`) to the hub. This is Anthropic's documented integration point and avoids needing a claude.ai OAuth subscription.

JSONL scraping in `apps/session-monitor` should stay as the safety net, not the primary signal path.

## Codex

- Codex CLI ships [`codex app-server`](https://developers.openai.com/codex/app-server) exposing the full session lifecycle (threads, turns, item streams, approval prompts) over bidirectional JSON-RPC 2.0.
- Transports: `stdio` (default), Unix socket, and experimental WebSocket via `codex app-server --listen ws://IP:PORT`; see [Codex CLI remote app-server mode](https://developers.openai.com/codex/cli/features#connect-the-tui-to-a-remote-app-server).
  - WS is explicitly marked "experimental and unsupported" but supports `--ws-auth capability-token` or `--ws-auth signed-bearer-token` with token files / sha256 fingerprints, plus optional issuer/audience claims.
- Notifications include: `item/agentMessage/delta`, `item/commandExecution/outputDelta`, `item/plan/delta`, `item/reasoning/textDelta`, plus `item/fileChange/requestApproval`. JSON-RPC methods: `initialize`, `thread/start`, `thread/resume`, `thread/list`, `turn/start`, `turn/interrupt`, `account/login/start`, `account/read`. Health: `GET /readyz`, `GET /healthz`.
- Mobile pairing is **separate and not third-party-attachable**. Per [OpenAI Codex Remote Connections](https://developers.openai.com/codex/remote-connections), Codex shows a QR code that opens ChatGPT; the phone connects through an OpenAI-hosted secure relay to the macOS Codex desktop app (not the CLI). The relay endpoints and token exchange are undocumented.
- [openai/codex#11166](https://github.com/openai/codex/issues/11166) is the open community ask to network-enable `app-server` for remote/mobile attach. No maintainer commitment yet — worth tracking.

## Claude Code

- Claude Code has an official Remote Control feature pairing claude.ai/code and the mobile app to a local session (`claude remote-control`, `claude --remote-control`/`--rc`, or `/rc` in-session; v2.1.51+).
- Architecture: local CLI makes **outbound HTTPS** to the Anthropic API and polls/streams over TLS — no inbound ports.
- Pairing requires a **claude.ai OAuth subscription token** (Pro/Max/Team/Enterprise), explicitly **not** an `ANTHROPIC_API_KEY` and not the inference-only token from `claude setup-token`. Short-lived, purpose-scoped credentials are minted per session.
- There is **no documented protocol, endpoint, or SDK for third-party clients** to subscribe to the Remote Control stream; the relay is closed and the feature explicitly requires a full-scope claude.ai login.
- The documented and consumable surface for our hub is **Claude Code Hooks** (`SessionStart`, `SessionEnd`, `UserPromptSubmit`, `Stop`, `SubagentStop`, `PreToolUse`, `PostToolUse`, `PreCompact`, etc.). Hooks emit JSON to stdin and pass `session_id` + `transcript_path`. See [code.claude.com/docs/en/hooks](https://code.claude.com/docs/en/hooks).

## Community prior art

- [siteboon/claudecodeui (CloudCLI)](https://github.com/siteboon/claudecodeui) — open-source web/mobile UI that already proxies Claude Code, Cursor, and Codex sessions. Closest prior art to Knify Canvas.
- [es6kr/claude-session-manager-mcp](https://github.com/es6kr/claude-session-manager-mcp) — MCP server exposing Claude Code conversation sessions with a GUI.
- [steipete/claude-code-mcp](https://github.com/steipete/claude-code-mcp) — wraps Claude Code as a one-shot MCP server.
- [nshkrdotcom/codex_sdk](https://github.com/nshkrdotcom/codex_sdk) — Elixir SDK driving `codex app-server` over stdio and experimental WS with `--ws-auth`. Proves the protocol is usable third-party.
- [Daniel Vaughan: Codex App-Server JSON-RPC walkthrough](https://codex.danielvaughan.com/2026/03/28/codex-app-server-json-rpc-protocol/) — independent protocol reference.

## Recommendation

- Keep `apps/session-monitor`'s JSONL tail as the always-on safety net.
- Add a Claude Code Hooks plugin that POSTs lifecycle events to the hub. Each event includes `session_id`, `transcript_path`, and tool metadata — enough to project a live canvas without reading the JSONL ourselves.
- Add a Codex app-server adapter that connects to `codex app-server` over stdio (or experimental WS with signed-bearer auth) and projects the `item/*` notifications into canvas log events.
- Do not depend on either mobile pairing relay; both are closed.
