// Per-device read/unread state for session-monitor rows.
// Backend (Hub + daemon) is intentionally unaware — read state is a frontend concern,
// modeled like chat/feed unread mechanics.
//
// Future work: cross-device sync (iPad + web) by replacing this localStorage layer with
// a site-owned BFF endpoint (e.g. /api/viewer-state/session-read) keyed by viewer identity.

const KEY = "knify:session-monitor:lastSeen";

export type ReadState = "running" | "unread" | "read";

type LastSeenMap = Record<string, string>;

function safeStorage(): Storage | null {
  try {
    return typeof window !== "undefined" ? window.localStorage : null;
  } catch {
    return null;
  }
}

export function loadLastSeen(): LastSeenMap {
  const storage = safeStorage();
  if (!storage) return {};
  try {
    const raw = storage.getItem(KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" ? (parsed as LastSeenMap) : {};
  } catch {
    return {};
  }
}

function saveLastSeen(map: LastSeenMap): void {
  const storage = safeStorage();
  if (!storage) return;
  try {
    storage.setItem(KEY, JSON.stringify(map));
  } catch {
    // storage quota or disabled — read state is best-effort, ignore.
  }
}

export function markSeen(sessionId: string, updatedAt: string): void {
  if (!sessionId) return;
  const map = loadLastSeen();
  const existing = map[sessionId];
  if (!existing || compareIso(updatedAt, existing) > 0) {
    map[sessionId] = updatedAt || new Date().toISOString();
    saveLastSeen(map);
  }
}

export function readState(
  sessionId: string | undefined,
  updatedAt: string | undefined,
  attention: string | undefined,
  lastSeen: LastSeenMap,
): ReadState {
  if (attention === "running") return "running";
  if (!sessionId || !updatedAt) return "unread";
  const seen = lastSeen[sessionId];
  if (seen && compareIso(seen, updatedAt) >= 0) return "read";
  return "unread";
}

function compareIso(a: string, b: string): number {
  const ta = Date.parse(a);
  const tb = Date.parse(b);
  if (Number.isNaN(ta) || Number.isNaN(tb)) return a.localeCompare(b);
  return ta - tb;
}
