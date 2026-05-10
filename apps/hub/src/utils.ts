export class AppError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly kind: "bad_request" | "not_found" | "conflict" | "forbidden" | "unauthorized" | "unavailable" | "internal" = "internal"
  ) {
    super(message);
  }
}

export function badRequest(message: string): never {
  throw new AppError(400, `bad request: ${message}`, "bad_request");
}

export function notFound(): never {
  throw new AppError(404, "not found", "not_found");
}

export function conflict(message: string): never {
  throw new AppError(409, `conflict: ${message}`, "conflict");
}

export function forbidden(message = "forbidden"): never {
  throw new AppError(403, message, "forbidden");
}

export function nowISO(): string {
  return new Date().toISOString();
}

export function addSeconds(iso: string, seconds: number): string {
  return new Date(new Date(iso).getTime() + seconds * 1000).toISOString();
}

export function newID(prefix: string): string {
  const bytes = new Uint8Array(8);
  crypto.getRandomValues(bytes);
  return `${prefix}-${Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("")}`;
}

export function validateID(label: string, id: string | undefined): string {
  if (!id) badRequest(`${label} is required`);
  if (id.includes("/") || id.includes("\\") || id.includes("..")) badRequest(`${label} contains unsafe characters`);
  for (const ch of id) {
    if (/^[A-Za-z0-9._:-]$/.test(ch)) continue;
    badRequest(`${label} contains invalid character ${JSON.stringify(ch)}`);
  }
  return id;
}

export function firstNonEmpty(...values: Array<string | undefined>): string {
  return values.find((value) => value !== undefined && value !== "") ?? "";
}

export function scoped(workspaceId: string | undefined): boolean {
  return !!workspaceId;
}

export function workspaceVisible(recordWorkspace: string | undefined, authWorkspace: string | undefined): boolean {
  return !authWorkspace || recordWorkspace === authWorkspace;
}

export function stampWorkspace<T extends { workspaceId?: string }>(value: T, workspaceId: string | undefined): T {
  if (!workspaceId) return value;
  if (!value.workspaceId) {
    value.workspaceId = workspaceId;
    return value;
  }
  if (value.workspaceId !== workspaceId) forbidden("forbidden");
  return value;
}

export function jsonResponse(value: unknown, status = 200, headers: HeadersInit = {}): Response {
  return new Response(JSON.stringify(value), {
    status,
    headers: {
      "Content-Type": "application/json",
      ...headers
    }
  });
}

export function errorResponse(error: unknown): Response {
  if (error instanceof AppError) {
    const message = error.kind === "not_found" ? "not found" : error.message;
    return jsonResponse({ error: message }, error.status);
  }
  const message = error instanceof Error ? error.message : String(error);
  return jsonResponse({ error: message }, 500);
}

export function stripUndefined<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

export async function sha256Hex(value: string): Promise<string> {
  const data = new TextEncoder().encode(value);
  const hash = await crypto.subtle.digest("SHA-256", data);
  return Array.from(new Uint8Array(hash), (b) => b.toString(16).padStart(2, "0")).join("");
}

export async function hmacSHA256Hex(secret: string, value: string): Promise<string> {
  const key = await crypto.subtle.importKey("raw", new TextEncoder().encode(secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  const sig = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(value));
  return Array.from(new Uint8Array(sig), (b) => b.toString(16).padStart(2, "0")).join("");
}

export function base64Encode(bytes: ArrayBuffer): string {
  let binary = "";
  for (const byte of new Uint8Array(bytes)) binary += String.fromCharCode(byte);
  return btoa(binary);
}

export function base64Decode(value: string): Uint8Array {
  const binary = atob(value);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

export function parseBool(raw: string | undefined): boolean | undefined {
  if (raw === undefined || raw === "") return undefined;
  switch (raw.trim().toLowerCase()) {
    case "1":
    case "true":
    case "yes":
    case "on":
    case "enabled":
      return true;
    case "0":
    case "false":
    case "no":
    case "off":
    case "disabled":
      return false;
    default:
      badRequest(`invalid boolean ${JSON.stringify(raw)}`);
  }
}

export function parseJSONDate(value: string | undefined): number {
  if (!value) return 0;
  const time = Date.parse(value);
  return Number.isFinite(time) ? time : 0;
}
