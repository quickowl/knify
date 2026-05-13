import { Camera, CheckCircle2, CircleDot, Download, ExternalLink, History, Link2, Moon, RefreshCcw, RotateCcw, Send, Sun } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import ReactMarkdown from "react-markdown";
import useSWR, { useSWRConfig } from "swr";
import {
  asRecord,
  blockLabel,
  canvasSlug,
  createSnapshot,
  createViewerLink,
  exportCanvas,
  fetchCanvas,
  fetchEdits,
  fetchFeedbackDeliveries,
  fetchSnapshots,
  openHubEventSource,
  parseCanvasIDSegment,
  readBlockValue,
  restoreSnapshot,
  statusLabel,
  submitFeedback,
} from "./hub";
import type { AgentCanvas, CanvasBlock, CanvasEdit, CanvasSnapshot, FeedbackDecision } from "./types";

type Theme = "light" | "dark";
type EndpointState = "idle" | "checking" | "ok" | "error";

type CollectionItem = {
  id: string;
  label: string;
  subtitle?: string;
  status?: string;
  badges: string[];
  blockIds: string[];
};

type Collection = {
  id: string;
  title: string;
  pageSize: number;
  items: CollectionItem[];
};

const endpointList = [
  "GET /v1/canvases",
  "GET /v1/canvases/:id",
  "GET /v1/feedback-deliveries",
  "POST /v1/canvases/:id/feedback",
  "POST /v1/canvases/:id/edits",
  "GET /v1/canvases/:id/edits",
  "GET /v1/canvases/:id/snapshots",
  "POST /v1/canvases/:id/snapshots",
  "POST /v1/canvases/:id/snapshots/:snapshotId/restore",
  "GET /v1/canvases/:id/export",
  "POST /v1/canvases/import",
  "POST /v1/viewer-links",
  "GET /v1/events",
];

function parseRouteCanvasID() {
  const [, route, segment] = window.location.pathname.split("/");
  if (route !== "canvases" || !segment) return "";
  return parseCanvasIDSegment(segment);
}

function routeParams() {
  const params = new URLSearchParams(window.location.search);
  return {
    blockID: params.get("block") || undefined,
    itemID: params.get("item") || undefined,
  };
}

function makeKey(prefix: string, canvasID: string) {
  return canvasID ? `${prefix}:${canvasID}` : null;
}

export default function CanvasViewer() {
  const canvasID = parseRouteCanvasID();
  const params = useMemo(routeParams, []);
  const [theme, setTheme] = useState<Theme>("light");
  const [feedbackText, setFeedbackText] = useState("");
  const [decision, setDecision] = useState<FeedbackDecision>("comment_only");
  const [selectedBlockID, setSelectedBlockID] = useState<string | undefined>(params.blockID);
  const [collectionSelection, setCollectionSelection] = useState<Record<string, string | undefined>>({});
  const [message, setMessage] = useState<string>();
  const [shareURL, setShareURL] = useState<string>();
  const [endpointState, setEndpointState] = useState<EndpointState>("idle");
  const { mutate } = useSWRConfig();

  const canvasKey = makeKey("/api/hub/canvases", canvasID);
  const feedbackKey = makeKey("/api/hub/feedback-deliveries", canvasID);
  const editsKey = makeKey("/api/hub/edits", canvasID);
  const snapshotsKey = makeKey("/api/hub/snapshots", canvasID);

  const { data: canvas, error, isLoading } = useSWR<AgentCanvas>(canvasKey, () => fetchCanvas(canvasID), { refreshInterval: 15_000 });
  const { data: deliveries = [], mutate: mutateDeliveries } = useSWR(feedbackKey, () => fetchFeedbackDeliveries(canvasID));
  const { data: edits = [], mutate: mutateEdits } = useSWR<CanvasEdit[]>(editsKey, () => fetchEdits(canvasID));
  const { data: snapshots = [], mutate: mutateSnapshots } = useSWR<CanvasSnapshot[]>(snapshotsKey, () => fetchSnapshots(canvasID));

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  useEffect(() => {
    const source = openHubEventSource((event) => {
      if (!event.canvasId || event.canvasId === canvasID) {
        void mutate(canvasKey);
        void mutateDeliveries();
        void mutateEdits();
        void mutateSnapshots();
      }
    });
    return () => source.close();
  }, [canvasID, canvasKey, mutate, mutateDeliveries, mutateEdits, mutateSnapshots]);

  useEffect(() => {
    if (!params.blockID) return;
    const handle = window.setTimeout(() => {
      document.getElementById(`block-${params.blockID}`)?.scrollIntoView({ behavior: "smooth", block: "start" });
    }, 50);
    return () => window.clearTimeout(handle);
  }, [canvas?.id, params.blockID]);

  const blockByID = useMemo(() => new Map(canvas?.blocks.map((block) => [block.id, block]) ?? []), [canvas]);
  const collections = useMemo(() => canvas?.blocks.filter((block) => block.kind === "collection").map(readCollection) ?? [], [canvas]);
  const selectedManagedIDs = useMemo(() => {
    const ids = new Set<string>();
    for (const collection of collections) {
      const selectedItemID = collectionSelection[collection.id] || params.itemID || collection.items[0]?.id;
      const item = collection.items.find((candidate) => candidate.id === selectedItemID) || collection.items[0];
      item?.blockIds.forEach((blockID) => ids.add(blockID));
    }
    return ids;
  }, [collectionSelection, collections, params.itemID]);
  const managedIDs = useMemo(() => {
    const ids = new Set<string>();
    collections.forEach((collection) => collection.items.forEach((item) => item.blockIds.forEach((blockID) => ids.add(blockID))));
    return ids;
  }, [collections]);

  async function refreshAll() {
    await Promise.all([mutate(canvasKey), mutateDeliveries(), mutateEdits(), mutateSnapshots()]);
  }

  async function sendFeedback() {
    if (!canvas) return;
    await submitFeedback({
      id: `feedback-${Date.now()}`,
      canvasId: canvas.id,
      decision,
      text: feedbackText.trim() || decision.replace(/_/g, " "),
      targetBlockIds: selectedBlockID ? [selectedBlockID] : undefined,
      targetAnchors: selectedBlockID ? [{ blockId: selectedBlockID, blockKind: blockByID.get(selectedBlockID)?.kind, label: blockByID.get(selectedBlockID) ? blockLabel(blockByID.get(selectedBlockID)!) : selectedBlockID, x: 0.5, y: 0.5 }] : undefined,
      createdAt: new Date().toISOString(),
    });
    setFeedbackText("");
    setMessage("Feedback submitted.");
    await mutateDeliveries();
  }

  async function saveSnapshot() {
    if (!canvas) return;
    await createSnapshot(canvas.id, canvas.version);
    setMessage("Snapshot saved.");
    await mutateSnapshots();
  }

  async function restore(snapshotID: string) {
    if (!canvas) return;
    await restoreSnapshot(canvas.id, snapshotID);
    setMessage("Snapshot restored.");
    await refreshAll();
  }

  async function downloadExport() {
    if (!canvas) return;
    const bundle = await exportCanvas(canvas.id);
    const blob = new Blob([JSON.stringify(bundle, null, 2) + "\n"], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${canvas.id}-bundle.json`;
    link.click();
    URL.revokeObjectURL(url);
    setMessage("Export downloaded.");
  }

  async function makeShareLink() {
    if (!canvas) return;
    const link = await createViewerLink(canvas.id);
    setShareURL(link.url || link.code || link.id);
    setMessage("Viewer link created.");
  }

  async function checkReadParity() {
    if (!canvas) return;
    setEndpointState("checking");
    try {
      await Promise.all([
        fetch("/api/hub/canvases").then(requireOK),
        fetch(`/api/hub/canvases/${encodeURIComponent(canvas.id)}`).then(requireOK),
        fetch(`/api/hub/feedback-deliveries?canvasId=${encodeURIComponent(canvas.id)}`).then(requireOK),
        fetch(`/api/hub/canvases/${encodeURIComponent(canvas.id)}/edits`).then(requireOK),
        fetch(`/api/hub/canvases/${encodeURIComponent(canvas.id)}/snapshots`).then(requireOK),
        fetch(`/api/hub/canvases/${encodeURIComponent(canvas.id)}/export`).then(requireOK),
      ]);
      setEndpointState("ok");
    } catch {
      setEndpointState("error");
    }
  }

  if (!canvasID) {
    return <div className="canvas-page-state">Missing canvas id.</div>;
  }

  if (isLoading) {
    return <div className="canvas-page-state">Loading canvas from Hub.</div>;
  }

  if (error || !canvas) {
    return (
      <div className="canvas-page-state">
        <strong>Canvas unavailable.</strong>
        <span>{error instanceof Error ? error.message : "Hub returned no canvas."}</span>
      </div>
    );
  }

  return (
    <div className="canvas-workbench">
      <aside className="canvas-nav">
        <div className="canvas-nav-top">
          <a className="mark" href="/dashboard"><span className="blade" />knify</a>
          <div className="segment">
            <button className={theme === "light" ? "active" : ""} onClick={() => setTheme("light")} aria-label="Use light theme"><Sun size={14} /></button>
            <button className={theme === "dark" ? "active" : ""} onClick={() => setTheme("dark")} aria-label="Use dark theme"><Moon size={14} /></button>
          </div>
        </div>
        <div className="canvas-nav-meta">
          <span>{canvas.agentName}</span>
          <strong>{canvas.title}</strong>
          <small>{canvas.id}</small>
        </div>
        <div className="canvas-nav-list">
          {canvas.blocks.map((block) => (
            <button
              className={selectedBlockID === block.id ? "active" : ""}
              key={block.id}
              onClick={() => {
                setSelectedBlockID(block.id);
                document.getElementById(`block-${block.id}`)?.scrollIntoView({ behavior: "smooth", block: "start" });
              }}
            >
              <span>{block.kind}</span>
              <strong>{blockLabel(block)}</strong>
            </button>
          ))}
        </div>
        <div className="canvas-nav-foot">
          <span className="dash-connected">Hub live</span>
          <button className="btn btn-small" onClick={() => void refreshAll()}><RefreshCcw size={13} /> refresh</button>
        </div>
      </aside>

      <main className="canvas-main">
        <header className="canvas-topbar">
          <div className="canvas-crumbs">
            <a href="/dashboard">dashboard</a>
            <span>/</span>
            <span>canvases</span>
            <span>/</span>
            <strong>{canvas.id}</strong>
          </div>
          <div className="canvas-actions">
            <a className="btn btn-small" href={`/canvases/${canvasSlug(canvas)}`}>latest</a>
            <button className="btn btn-small" onClick={() => void makeShareLink()}><Link2 size={13} /> share</button>
            <button className="btn btn-small" onClick={() => void downloadExport()}><Download size={13} /> export</button>
            <button className="btn btn-small btn-primary" onClick={() => void saveSnapshot()}><Camera size={13} /> snapshot</button>
          </div>
        </header>

        <section className="canvas-hero-row">
          <div>
            <div className="detail-kicker">
              <span>{statusLabel(canvas.status)}</span>
              <span>{canvas.runID}</span>
              <span>v{canvas.version}</span>
              <span>{canvas.blocks.length} blocks</span>
            </div>
            <h1>{canvas.title}</h1>
            {canvas.summary ? <p>{canvas.summary}</p> : null}
          </div>
          <div className="canvas-read-model">
            <strong>API parity</strong>
            <span>{endpointList.length} Hub endpoints exposed through /api/hub/*</span>
            <button className={`btn btn-small parity-${endpointState}`} onClick={() => void checkReadParity()}>
              {endpointState === "checking" ? "checking" : endpointState === "ok" ? "read checks ok" : endpointState === "error" ? "read check failed" : "check read endpoints"}
            </button>
          </div>
        </section>

        {message ? <div className="canvas-message">{message}</div> : null}
        {shareURL ? <div className="canvas-message"><code>{shareURL}</code></div> : null}

        <div className="canvas-content-grid">
          <section className="canvas-blocks" aria-label="Canvas blocks">
            {canvas.blocks.map((block) => {
              if (managedIDs.has(block.id) && !selectedManagedIDs.has(block.id)) return null;
              return (
                <CanvasBlockView
                  block={block}
                  blockByID={blockByID}
                  selectedBlockID={selectedBlockID}
                  selectedItemID={collectionSelection[block.id] || params.itemID}
                  onSelectBlock={setSelectedBlockID}
                  onSelectCollectionItem={(itemID) => setCollectionSelection((current) => ({ ...current, [block.id]: itemID }))}
                  onDecision={setDecision}
                  key={block.id}
                />
              );
            })}
          </section>

          <aside className="canvas-inspector">
            <section className="inspector-panel">
              <h2>Feedback</h2>
              <div className="decision-row">
                {(["comment_only", "needs_changes", "accepted"] as FeedbackDecision[]).map((item) => (
                  <button className={decision === item ? "active" : ""} key={item} onClick={() => setDecision(item)}>{item.replace(/_/g, " ")}</button>
                ))}
              </div>
              <textarea value={feedbackText} onChange={(event) => setFeedbackText(event.target.value)} placeholder="Leave reviewer feedback..." />
              <button className="btn btn-small btn-primary" onClick={() => void sendFeedback()}><Send size={13} /> submit feedback</button>
              <small>{deliveries.length} delivery record{deliveries.length === 1 ? "" : "s"}</small>
            </section>

            <section className="inspector-panel">
              <h2>Snapshots</h2>
              <button className="btn btn-small" onClick={() => void mutateSnapshots()}><History size={13} /> refresh versions</button>
              <div className="snapshot-mini-list">
                {snapshots.slice(0, 6).map((snapshot) => (
                  <div key={snapshot.id}>
                    <span>v{snapshot.version}</span>
                    <strong>{snapshot.label || snapshot.reason}</strong>
                    <button onClick={() => void restore(snapshot.id)} aria-label={`Restore snapshot ${snapshot.version}`}><RotateCcw size={13} /></button>
                  </div>
                ))}
                {!snapshots.length ? <small>No snapshots yet.</small> : null}
              </div>
            </section>

            <section className="inspector-panel">
              <h2>Endpoint Surface</h2>
              <div className="endpoint-list">
                {endpointList.map((endpoint) => <code key={endpoint}>{endpoint}</code>)}
              </div>
            </section>

            <section className="inspector-panel">
              <h2>Edit History</h2>
              <div className="edit-list">
                {edits.slice(0, 6).map((edit) => (
                  <div key={edit.id}>
                    <span>{new Date(edit.createdAt).toLocaleString()}</span>
                    <strong>{edit.ops.length} op{edit.ops.length === 1 ? "" : "s"}</strong>
                  </div>
                ))}
                {!edits.length ? <small>No edits yet.</small> : null}
              </div>
            </section>
          </aside>
        </div>
      </main>
    </div>
  );
}

function CanvasBlockView({
  block,
  blockByID,
  selectedBlockID,
  selectedItemID,
  onSelectBlock,
  onSelectCollectionItem,
  onDecision,
}: {
  block: CanvasBlock;
  blockByID: Map<string, CanvasBlock>;
  selectedBlockID?: string;
  selectedItemID?: string;
  onSelectBlock: (id: string) => void;
  onSelectCollectionItem: (id: string) => void;
  onDecision: (decision: FeedbackDecision) => void;
}) {
  return (
    <article
      className={`hub-block hub-block-${block.kind} ${selectedBlockID === block.id ? "focused" : ""}`}
      id={`block-${block.id}`}
      data-block-id={block.id}
      onClick={() => onSelectBlock(block.id)}
    >
      <div className="hub-block-chrome">
        <span>{block.kind}</span>
        <strong>{block.id}</strong>
      </div>
      {renderBlock(block, blockByID, selectedItemID, onSelectCollectionItem, onDecision)}
    </article>
  );
}

function renderBlock(
  block: CanvasBlock,
  blockByID: Map<string, CanvasBlock>,
  selectedItemID: string | undefined,
  onSelectCollectionItem: (id: string) => void,
  onDecision: (decision: FeedbackDecision) => void,
) {
  switch (block.kind) {
    case "heading": {
      const text = readBlockValue<string>(block, "text") || readBlockValue<string>(block, "title") || "";
      return <h2 className="hub-heading">{text}</h2>;
    }
    case "markdown": {
      const markdown = readBlockValue<string>(block, "markdown") || readBlockValue<string>(block, "text") || "";
      return <div className="hub-markdown"><ReactMarkdown>{markdown}</ReactMarkdown></div>;
    }
    case "collection":
      return <CollectionBlock block={block} blockByID={blockByID} selectedItemID={selectedItemID} onSelectItem={onSelectCollectionItem} />;
    case "terminal": {
      const output = readBlockValue<string | string[]>(block, "output");
      const lines = Array.isArray(output) ? output : String(output ?? "").split("\n");
      return (
        <div className="hub-surface hub-terminal">
          {readBlockValue<string>(block, "command") ? <code>$ {readBlockValue<string>(block, "command")}</code> : null}
          <pre>{lines.join("\n")}</pre>
        </div>
      );
    }
    case "checklist": {
      const items = readBlockValue<unknown[]>(block, "items") ?? [];
      return (
        <div className="hub-surface hub-checklist">
          <strong>{readBlockValue<string>(block, "title") || "Checklist"}</strong>
          {items.map((item, index) => {
            const record = asRecord(item);
            const checked = Boolean(record.checked ?? record.isComplete);
            return <p key={String(record.id ?? index)}>{checked ? <CheckCircle2 size={15} /> : <CircleDot size={15} />}<span>{String(record.text ?? record.title ?? "")}</span></p>;
          })}
        </div>
      );
    }
    case "chart":
      return <ChartBlock block={block} />;
    case "diff":
      return <DiffBlock block={block} />;
    case "decision": {
      const options = readBlockValue<string[]>(block, "options") ?? ["comment_only", "needs_changes", "accepted"];
      return (
        <div className="hub-surface hub-decision">
          <strong>{readBlockValue<string>(block, "title") || "Decision"}</strong>
          <p>{readBlockValue<string>(block, "question") || readBlockValue<string>(block, "prompt")}</p>
          <div>
            {options.map((option) => (
              <button key={option} onClick={() => isDecision(option) && onDecision(option)}>{option.replace(/_/g, " ")}</button>
            ))}
          </div>
        </div>
      );
    }
    case "link": {
      const url = readBlockValue<string>(block, "url") || "";
      return <a className="hub-surface hub-link" href={url} target="_blank" rel="noreferrer"><strong>{readBlockValue<string>(block, "title") || url}</strong><ExternalLink size={15} /></a>;
    }
    case "image": {
      const url = resolveHubPath(readBlockValue<string>(block, "url") || "");
      const alt = readBlockValue<string>(block, "alt") || "Canvas image";
      return <figure className="hub-image">{url ? <img src={url} alt={alt} /> : <div>{alt}</div>}{readBlockValue<string>(block, "caption") ? <figcaption>{readBlockValue<string>(block, "caption")}</figcaption> : null}</figure>;
    }
    case "metadata":
      return <MetadataBlock block={block} />;
    default:
      return <UnsupportedBlock block={block} />;
  }
}

function CollectionBlock({ block, blockByID, selectedItemID, onSelectItem }: { block: CanvasBlock; blockByID: Map<string, CanvasBlock>; selectedItemID?: string; onSelectItem: (id: string) => void }) {
  const collection = readCollection(block);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("all");
  const statuses = [...new Set(collection.items.map((item) => item.status).filter(Boolean) as string[])];
  const filtered = collection.items.filter((item) => {
    const statusMatch = status === "all" || item.status === status;
    const queryMatch = !query.trim() || [item.label, item.subtitle, item.status, ...item.badges].filter(Boolean).join(" ").toLowerCase().includes(query.trim().toLowerCase());
    return statusMatch && queryMatch;
  });
  const selected = filtered.find((item) => item.id === selectedItemID) || filtered[0];

  useEffect(() => {
    if (selected && selected.id !== selectedItemID) onSelectItem(selected.id);
  }, [onSelectItem, selected, selectedItemID]);

  return (
    <section className="hub-collection">
      <div className="collection-header">
        <div>
          <strong>{collection.title}</strong>
          <span>{filtered.length} items</span>
        </div>
        <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search items" />
        {statuses.length ? (
          <select value={status} onChange={(event) => setStatus(event.target.value)}>
            <option value="all">All</option>
            {statuses.map((item) => <option value={item} key={item}>{item}</option>)}
          </select>
        ) : null}
      </div>
      <div className="collection-list">
        {filtered.slice(0, Math.max(collection.pageSize, 12)).map((item) => (
          <button className={item.id === selected?.id ? "selected" : ""} key={item.id} onClick={() => onSelectItem(item.id)}>
            <strong>{item.label}</strong>
            <span>{[item.subtitle, item.status].filter(Boolean).join(" · ")}</span>
            {item.badges.length ? <small>{item.badges.slice(0, 4).join(" · ")}</small> : null}
          </button>
        ))}
      </div>
      {selected ? (
        <div className="collection-selected">
          {selected.blockIds.map((blockID) => {
            const child = blockByID.get(blockID);
            return child ? (
              <div className="collection-child" key={blockID}>
                <strong>{blockLabel(child)}</strong>
                {renderBlock(child, blockByID, undefined, onSelectItem, () => undefined)}
              </div>
            ) : <div className="collection-child missing" key={blockID}>Missing block {blockID}</div>;
          })}
        </div>
      ) : null}
    </section>
  );
}

function ChartBlock({ block }: { block: CanvasBlock }) {
  const points = chartPoints(block);
  const max = Math.max(...points.map((point) => point.value), 1);
  return (
    <div className="hub-surface hub-chart">
      <strong>{readBlockValue<string>(block, "title") || "Chart"}</strong>
      {points.map((point) => (
        <div className="hub-chart-row" key={point.label}>
          <span>{point.label}</span>
          <div><i style={{ width: `${Math.max(3, (point.value / max) * 100)}%` }} /></div>
          <em>{point.value}</em>
        </div>
      ))}
    </div>
  );
}

function DiffBlock({ block }: { block: CanvasBlock }) {
  const rawDiff = readBlockValue<string>(block, "diff") || "";
  const lines = rawDiff.split("\n");
  return (
    <div className="hub-surface hub-diff">
      <strong>{readBlockValue<string>(block, "filePath") || readBlockValue<string>(block, "title") || "Diff"}</strong>
      <pre>{lines.map((line, index) => <span className={line.startsWith("+") ? "add" : line.startsWith("-") ? "del" : ""} key={`${line}-${index}`}>{line}</span>)}</pre>
    </div>
  );
}

function MetadataBlock({ block }: { block: CanvasBlock }) {
  const pairs = readBlockValue<unknown[]>(block, "pairs");
  const metadata = asRecord(readBlockValue(block, "metadata"));
  const rows = Array.isArray(pairs)
    ? pairs.map((pair) => {
        const record = asRecord(pair);
        return [String(record.key ?? ""), String(record.value ?? "")] as const;
      })
    : Object.entries(metadata).map(([key, value]) => [key, String(value)] as const);
  return (
    <details className="hub-surface hub-metadata">
      <summary>{readBlockValue<string>(block, "title") || "Metadata"}</summary>
      <dl>{rows.map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl>
    </details>
  );
}

function UnsupportedBlock({ block }: { block: CanvasBlock }) {
  return (
    <details className="hub-surface hub-unsupported">
      <summary>{block.kind}</summary>
      <pre>{JSON.stringify(block.raw, null, 2)}</pre>
    </details>
  );
}

function readCollection(block: CanvasBlock): Collection {
  const items = (readBlockValue<unknown[]>(block, "items") ?? []).map(readCollectionItem).filter((item): item is CollectionItem => !!item);
  return {
    id: block.id,
    title: readBlockValue<string>(block, "title") || "Collection",
    pageSize: Math.max(1, Number(readBlockValue<number>(block, "pageSize") ?? 12)),
    items,
  };
}

function readCollectionItem(raw: unknown, index: number): CollectionItem | undefined {
  const record = asRecord(raw);
  const blockIds = Array.isArray(record.blockIds) ? record.blockIds.map(String).filter(Boolean) : [];
  const id = String(record.id ?? `item-${index + 1}`);
  const label = String(record.label ?? record.title ?? blockIds[0] ?? id).trim();
  if (!label || !blockIds.length) return undefined;
  return {
    id,
    label,
    subtitle: typeof record.subtitle === "string" ? record.subtitle : undefined,
    status: typeof record.status === "string" ? record.status : undefined,
    badges: Array.isArray(record.badges) ? record.badges.map(String) : [],
    blockIds,
  };
}

function chartPoints(block: CanvasBlock) {
  const points = readBlockValue<unknown[]>(block, "points");
  if (Array.isArray(points)) {
    return points.map((point) => {
      const record = asRecord(point);
      return { label: String(record.label ?? ""), value: Number(record.value ?? 0) };
    });
  }
  const chart = asRecord(readBlockValue(block, "chart"));
  const series = Array.isArray(chart.series) ? chart.series : [];
  return series.flatMap((rawSeries) => {
    const record = asRecord(rawSeries);
    const data = Array.isArray(record.data) ? record.data : [];
    return data.map((point) => {
      const pointRecord = asRecord(point);
      return { label: String(pointRecord.label ?? ""), value: Number(pointRecord.value ?? 0) };
    });
  });
}

function isDecision(value: string): value is FeedbackDecision {
  return value === "accepted" || value === "needs_changes" || value === "comment_only";
}

function resolveHubPath(raw: string) {
  if (!raw) return "";
  if (/^https?:\/\//.test(raw) || raw.startsWith("data:")) return raw;
  if (raw.startsWith("/v1/")) return raw.replace(/^\/v1/, "/api/hub");
  if (raw.startsWith("/")) return `/api/hub${raw}`;
  return raw;
}

function requireOK(response: Response) {
  if (!response.ok) throw new Error(String(response.status));
  return response;
}
