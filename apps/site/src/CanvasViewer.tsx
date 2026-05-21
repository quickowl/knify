import { Camera, CheckCircle2, CircleDot, Download, ExternalLink, History, Link2, Moon, RefreshCcw, RotateCcw, Send, Sun } from "lucide-react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import useSWR from "swr";
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
  normalizeCanvas,
  openHubEventSource,
  parseCanvasIDSegment,
  readBlockValue,
  restoreSnapshot,
  statusLabel,
  submitFeedback,
} from "./hub";
import type { AgentCanvas, CanvasBlock, CanvasEdit, CanvasSnapshot, FeedbackDecision } from "./types";
import { loadLastSeen, markSeen, readState, type ReadState } from "./sessionReadState";

type Theme = "light" | "dark";
type EndpointState = "idle" | "checking" | "ok" | "partial" | "error";

type ScrollAnchor = {
  blockID: string;
  top: number;
};

type CanvasUpdateMeta = {
  kind: "dynamic" | "static";
  previousVersion: number;
  nextVersion: number;
  changedBlockIDs: string[];
  appendedBlockIDs: string[];
  removedBlockIDs: string[];
  stagedReason?: string;
};

type UpdateNotice = {
  kind: "dynamic" | "static";
  count: number;
  version: number;
  blockIDs: string[];
  message: string;
};

type CollectionItem = {
  id: string;
  sessionId?: string;
  label: string;
  subtitle?: string;
  status?: string;
  attention?: string;
  updatedAt?: string;
  purpose?: string;
  currentState?: string;
  evidenceStatus?: string;
  nextStep?: string;
  artifactCount?: number;
  planLabel?: string;
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
  const [displayCanvas, setDisplayCanvas] = useState<AgentCanvas>();
  const [stagedCanvas, setStagedCanvas] = useState<AgentCanvas>();
  const [updateNotice, setUpdateNotice] = useState<UpdateNotice>();
  const [highlightedBlockIDs, setHighlightedBlockIDs] = useState<Set<string>>(() => new Set());
  const pendingAnchorRef = useRef<ScrollAnchor | undefined>(undefined);
  const pendingScrollToBottomRef = useRef(false);

  const canvasKey = makeKey("/api/hub/canvases", canvasID);
  const feedbackKey = makeKey("/api/hub/feedback-deliveries", canvasID);
  const editsKey = makeKey("/api/hub/edits", canvasID);
  const snapshotsKey = makeKey("/api/hub/snapshots", canvasID);

  const { data: latestCanvas, error, isLoading, mutate: mutateCanvas } = useSWR<AgentCanvas>(canvasKey, () => fetchCanvas(canvasID), { refreshInterval: 15_000 });
  const { data: deliveries = [], mutate: mutateDeliveries } = useSWR(feedbackKey, () => fetchFeedbackDeliveries(canvasID));
  const { data: edits = [], mutate: mutateEdits } = useSWR<CanvasEdit[]>(editsKey, () => fetchEdits(canvasID));
  const { data: snapshots = [], mutate: mutateSnapshots } = useSWR<CanvasSnapshot[]>(snapshotsKey, () => fetchSnapshots(canvasID));

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  useEffect(() => {
    const source = openHubEventSource((event) => {
      if (!event.canvasId || event.canvasId === canvasID) {
        if ((event.type === "canvas.created" || event.type === "canvas.updated") && event.data) {
          void mutateCanvas(normalizeCanvas(event.data), { revalidate: false });
        } else {
          void mutateCanvas();
        }
        void mutateDeliveries();
        void mutateEdits();
        void mutateSnapshots();
      }
    });
    return () => source.close();
  }, [canvasID, mutateCanvas, mutateDeliveries, mutateEdits, mutateSnapshots]);

  useEffect(() => {
    setDisplayCanvas(undefined);
    setStagedCanvas(undefined);
    setUpdateNotice(undefined);
    setHighlightedBlockIDs(new Set());
    pendingAnchorRef.current = undefined;
    pendingScrollToBottomRef.current = false;
  }, [canvasID]);

  useEffect(() => {
    if (!latestCanvas) return;
    if (stagedCanvas && sameCanvasVersion(stagedCanvas, latestCanvas)) return;
    if (!displayCanvas || displayCanvas.id !== latestCanvas.id) {
      setDisplayCanvas(latestCanvas);
      setStagedCanvas(undefined);
      setUpdateNotice(undefined);
      return;
    }
    if (sameCanvasVersion(displayCanvas, latestCanvas)) return;

    const anchor = captureScrollAnchor();
    const meta = describeCanvasUpdate(displayCanvas, latestCanvas);
    const anchorSurvives = !anchor || latestCanvas.blocks.some((block) => block.id === anchor.blockID);

    if (meta.kind === "static" && !anchorSurvives) {
      setStagedCanvas(latestCanvas);
      setUpdateNotice({
        kind: "static",
        count: 1,
        version: latestCanvas.version,
        blockIDs: meta.changedBlockIDs,
        message: `New version v${latestCanvas.version} available.`,
      });
      return;
    }

    const atLiveEdge = isAtLiveEdge();
    pendingAnchorRef.current = anchor;
    pendingScrollToBottomRef.current = meta.kind === "dynamic" && atLiveEdge && meta.appendedBlockIDs.length > 0;
    setDisplayCanvas(latestCanvas);
    setStagedCanvas(undefined);
    setHighlightedBlockIDs(new Set(meta.changedBlockIDs));

    if (meta.kind === "dynamic" && !atLiveEdge && meta.changedBlockIDs.length > 0) {
      setUpdateNotice({
        kind: "dynamic",
        count: meta.changedBlockIDs.length,
        version: latestCanvas.version,
        blockIDs: meta.changedBlockIDs,
        message: `${meta.changedBlockIDs.length} live update${meta.changedBlockIDs.length === 1 ? "" : "s"} applied.`,
      });
    } else {
      setUpdateNotice(undefined);
    }
  }, [displayCanvas, latestCanvas, stagedCanvas]);

  const canvas = displayCanvas ?? latestCanvas;

  useLayoutEffect(() => {
    if (!canvas) return;
    if (pendingScrollToBottomRef.current) {
      pendingScrollToBottomRef.current = false;
      window.scrollTo({ top: document.documentElement.scrollHeight, behavior: "auto" });
      return;
    }
    const anchor = pendingAnchorRef.current;
    if (!anchor) return;
    pendingAnchorRef.current = undefined;
    const element = document.getElementById(`block-${anchor.blockID}`);
    if (!element) return;
    const delta = element.getBoundingClientRect().top - anchor.top;
    if (Math.abs(delta) > 1) window.scrollBy(0, delta);
  }, [canvas?.id, canvas?.lastEventId, canvas?.version]);

  useEffect(() => {
    if (!highlightedBlockIDs.size) return;
    const handle = window.setTimeout(() => setHighlightedBlockIDs(new Set()), 2200);
    return () => window.clearTimeout(handle);
  }, [highlightedBlockIDs]);

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
    await Promise.all([mutateCanvas(), mutateDeliveries(), mutateEdits(), mutateSnapshots()]);
  }

  function applyStagedCanvas() {
    if (!canvas || !stagedCanvas) return;
    const meta = describeCanvasUpdate(canvas, stagedCanvas);
    pendingAnchorRef.current = captureScrollAnchor();
    setDisplayCanvas(stagedCanvas);
    setStagedCanvas(undefined);
    setUpdateNotice(undefined);
    setHighlightedBlockIDs(new Set(meta.changedBlockIDs));
  }

  function jumpToLatestUpdate() {
    const targetID = updateNotice?.blockIDs.find((id) => document.getElementById(`block-${id}`)) || (canvas?.blocks.length ? canvas.blocks[canvas.blocks.length - 1]?.id : undefined);
    if (targetID) document.getElementById(`block-${targetID}`)?.scrollIntoView({ behavior: "smooth", block: "start" });
    setUpdateNotice(undefined);
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
    try {
      await createSnapshot(canvas.id, canvas.version);
      setMessage("Snapshot saved.");
      await mutateSnapshots();
    } catch (err) {
      setMessage(`Snapshot endpoint unavailable: ${err instanceof Error ? err.message : String(err)}`);
    }
  }

  async function restore(snapshotID: string) {
    if (!canvas) return;
    try {
      await restoreSnapshot(canvas.id, snapshotID);
      setMessage("Snapshot restored.");
      await refreshAll();
    } catch (err) {
      setMessage(`Restore endpoint unavailable: ${err instanceof Error ? err.message : String(err)}`);
    }
  }

  async function downloadExport() {
    if (!canvas) return;
    try {
      const bundle = await exportCanvas(canvas.id);
      const blob = new Blob([JSON.stringify(bundle, null, 2) + "\n"], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `${canvas.id}-bundle.json`;
      link.click();
      URL.revokeObjectURL(url);
      setMessage("Export downloaded.");
    } catch (err) {
      setMessage(`Export endpoint unavailable: ${err instanceof Error ? err.message : String(err)}`);
    }
  }

  async function makeShareLink() {
    if (!canvas) return;
    try {
      const link = await createViewerLink(canvas.id);
      setShareURL(link.url || link.code || link.id);
      setMessage("Viewer link created.");
    } catch (err) {
      setMessage(`Viewer link endpoint unavailable: ${err instanceof Error ? err.message : String(err)}`);
    }
  }

  async function checkReadParity() {
    if (!canvas) return;
    setEndpointState("checking");
    try {
      const core = await Promise.all([
        fetch("/api/hub/canvases").then(requireOK),
        fetch(`/api/hub/canvases/${encodeURIComponent(canvas.id)}`).then(requireOK),
        fetch(`/api/hub/feedback-deliveries?canvasId=${encodeURIComponent(canvas.id)}`).then(requireOK),
        fetch(`/api/hub/canvases/${encodeURIComponent(canvas.id)}/edits`).then(requireOK),
      ]);
      const optional = await Promise.allSettled([
        fetch(`/api/hub/canvases/${encodeURIComponent(canvas.id)}/snapshots`).then(requireOK),
        fetch(`/api/hub/canvases/${encodeURIComponent(canvas.id)}/export`).then(requireOK),
      ]);
      const unavailable = optional.filter((result) => result.status === "rejected").length;
      setEndpointState(core.length === 4 && unavailable ? "partial" : "ok");
      setMessage(unavailable ? "Core Hub reads are ok. This local Hub binary returns 404 for snapshots/export." : "All checked Hub reads are ok.");
    } catch {
      setEndpointState("error");
      setMessage("Core Hub read check failed.");
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
              {endpointState === "checking" ? "checking" : endpointState === "ok" ? "read checks ok" : endpointState === "partial" ? "core reads ok" : endpointState === "error" ? "read check failed" : "check read endpoints"}
            </button>
          </div>
        </section>

        {message ? <div className="canvas-message">{message}</div> : null}
        {shareURL ? <div className="canvas-message"><code>{shareURL}</code></div> : null}
        <div className="sr-only" aria-live="polite">{updateNotice?.message || ""}</div>
        {updateNotice ? (
          <div className={`canvas-update-notice update-${updateNotice.kind}`}>
            <span>{updateNotice.message}</span>
            {stagedCanvas ? (
              <button className="btn btn-small" onClick={applyStagedCanvas}>show latest</button>
            ) : (
              <button className="btn btn-small" onClick={jumpToLatestUpdate}>jump to update</button>
            )}
          </div>
        ) : null}

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
                  highlighted={highlightedBlockIDs.has(block.id)}
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
  highlighted,
}: {
  block: CanvasBlock;
  blockByID: Map<string, CanvasBlock>;
  selectedBlockID?: string;
  selectedItemID?: string;
  onSelectBlock: (id: string) => void;
  onSelectCollectionItem: (id: string) => void;
  onDecision: (decision: FeedbackDecision) => void;
  highlighted: boolean;
}) {
  return (
    <article
      className={`hub-block hub-block-${block.kind} ${selectedBlockID === block.id ? "focused" : ""} ${highlighted ? "updated" : ""}`}
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
      const url = resolveAssetSrc(readBlockValue<string>(block, "url"), readBlockValue<string>(block, "assetId"));
      const alt = readBlockValue<string>(block, "alt") || "Canvas image";
      return <figure className="hub-image">{url ? <img src={url} alt={alt} /> : <div className="hub-image-placeholder">{alt}</div>}{readBlockValue<string>(block, "caption") ? <figcaption>{readBlockValue<string>(block, "caption")}</figcaption> : null}</figure>;
    }
    case "html":
      return <HtmlBlock block={block} />;
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
  const [readFilter, setReadFilter] = useState<"all" | "unread" | "running">("all");
  const [lastSeen, setLastSeen] = useState(() => loadLastSeen());
  const hasReadable = collection.items.some((item) => item.sessionId);

  const readStateById = useMemo(() => {
    const map = new Map<string, ReadState>();
    for (const item of collection.items) {
      map.set(item.id, readState(item.sessionId, item.updatedAt, item.attention, lastSeen));
    }
    return map;
  }, [collection.items, lastSeen]);

  const statuses = [...new Set(collection.items.map((item) => item.status).filter(Boolean) as string[])];
  const filtered = collection.items.filter((item) => {
    const statusMatch = status === "all" || item.status === status;
    const rs = readStateById.get(item.id);
    const readMatch =
      readFilter === "all" ||
      (readFilter === "unread" && (rs === "unread" || rs === "running")) ||
      (readFilter === "running" && rs === "running");
    const queryMatch = !query.trim() || [item.label, item.subtitle, item.status, ...item.badges].filter(Boolean).join(" ").toLowerCase().includes(query.trim().toLowerCase());
    return statusMatch && readMatch && queryMatch;
  });

  const ordered = hasReadable
    ? [...filtered].sort((a, b) => readStateRank(readStateById.get(a.id)) - readStateRank(readStateById.get(b.id)))
    : filtered;
  const grouped = groupCollectionItems(ordered);

  const selected = ordered.find((item) => item.id === selectedItemID) || ordered[0];

  useEffect(() => {
    if (selected && selected.id !== selectedItemID) onSelectItem(selected.id);
  }, [onSelectItem, selected, selectedItemID]);

  const handleSelect = (item: CollectionItem) => {
    onSelectItem(item.id);
    if (item.sessionId && item.updatedAt) {
      markSeen(item.sessionId, item.updatedAt);
      setLastSeen((current) => ({ ...current, [item.sessionId!]: item.updatedAt! }));
    }
  };

  return (
    <section className="hub-collection">
      <div className="collection-header">
        <div>
          <strong>{collection.title}</strong>
          <span>{ordered.length} items</span>
        </div>
        <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search items" />
        {statuses.length ? (
          <select value={status} onChange={(event) => setStatus(event.target.value)}>
            <option value="all">All</option>
            {statuses.map((item) => <option value={item} key={item}>{item}</option>)}
          </select>
        ) : null}
        {hasReadable ? (
          <select value={readFilter} onChange={(event) => setReadFilter(event.target.value as typeof readFilter)} aria-label="Filter by read state">
            <option value="all">Read &amp; unread</option>
            <option value="unread">Only unread</option>
            <option value="running">Only running</option>
          </select>
        ) : null}
      </div>
      <div className="collection-list">
        {grouped.map((group) => (
          <div className="collection-priority-group" key={group.id}>
            <div className={`collection-group-heading group-${group.id}`}>
              <strong>{groupHeadingLabel(group)}</strong>
              <span>{group.items.length}</span>
            </div>
            {group.items.slice(0, Math.max(collection.pageSize, 12)).map((item) => {
              const rs = readStateById.get(item.id);
              const sessionTriage = isSessionTriageItem(item);
              return (
                <button
                  className={`collection-card ${sessionTriage ? `session-triage-card ${priorityClass(item)}` : ""} ${item.id === selected?.id ? "selected" : ""} read-${rs ?? "read"}`}
                  key={item.id}
                  onClick={() => handleSelect(item)}
                >
                  {sessionTriage ? (
                    <SessionTriageCard item={item} readState={rs} />
                  ) : (
                    <>
                      {rs ? <span className={`read-dot read-dot-${rs}`} aria-label={rs} /> : null}
                      <strong>{item.label}</strong>
                      <span>{[item.subtitle, item.status].filter(Boolean).join(" | ")}</span>
                      {item.badges.length ? <small>{item.badges.slice(0, 4).join(" | ")}</small> : null}
                    </>
                  )}
                </button>
              );
            })}
          </div>
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

type CollectionGroup = {
  id: "decision" | "active" | "nudge" | "other";
  label: string;
  items: CollectionItem[];
};

function groupCollectionItems(items: CollectionItem[]): CollectionGroup[] {
  const groups: CollectionGroup[] = [
    { id: "decision", label: "Needs decision", items: [] },
    { id: "active", label: "Active / watching", items: [] },
    { id: "nudge", label: "Needs nudge", items: [] },
    { id: "other", label: "Other sessions", items: [] },
  ];
  for (const item of items) {
    groups.find((group) => group.id === priorityBucket(item))?.items.push(item);
  }
  return groups.filter((group) => group.items.length > 0);
}

function SessionTriageCard({ item, readState }: { item: CollectionItem; readState?: ReadState }) {
  const provider = providerLabel(item);
  const title = item.label.startsWith(`${provider}: `) ? item.label.slice(provider.length + 2) : item.purpose || item.label;
  const evidenceText = item.artifactCount && item.artifactCount > 0 ? `${item.evidenceStatus || "evidence"} (${item.artifactCount})` : item.evidenceStatus || "unknown";
  const action = actionLabel(item.nextStep);
  const bucket = priorityBucket(item);

  return (
    <>
      <div className="session-card-head">
        {readState ? <span className={`read-dot read-dot-${readState}`} aria-label={readState} /> : null}
        <div>
          <span>{provider}</span>
          <strong>{title}</strong>
        </div>
        <time className={`state-${bucket}`}>{stateLabel(item)}</time>
      </div>
      <div className="session-chip-row">
        <span className={`session-chip evidence-${cleanClass(item.evidenceStatus)}`}>evidence {evidenceText}</span>
        <span className={`session-chip status-${cleanClass(item.status)}`}>{statusText(item.status)}</span>
      </div>
      <div className={`session-primary-action action-${cleanClass(action)}`}>{primaryActionText(action)}</div>
      {bucket === "decision" ? (
        <div className="session-choice-row" aria-label="Decision choices">
          <span>Link session</span>
          <span>Ignore</span>
          <span>Resume context</span>
        </div>
      ) : null}
      {item.planLabel ? <div className="session-plan-line">{item.planLabel}</div> : null}
      <div className="session-card-reason">{decisionReason(item)}</div>
      <div className="session-card-facts">
        <span>{compactCardFact(item)}</span>
      </div>
    </>
  );
}

function groupHeadingLabel(group: CollectionGroup): string {
  if (group.id === "decision") return `${group.items.length} decision${group.items.length === 1 ? "" : "s"} needed`;
  if (group.id === "active") return `${group.items.length} active / watching`;
  if (group.id === "nudge") return `${group.items.length} need nudge`;
  return `${group.items.length} other session${group.items.length === 1 ? "" : "s"}`;
}

function priorityBucket(item: CollectionItem): CollectionGroup["id"] {
  const action = actionLabel(item.nextStep);
  if (action === "decide" || action === "confirm") return "decision";
  if (item.attention === "running" || item.currentState?.includes(", active")) return "active";
  if (action === "nudge" || item.evidenceStatus === "missing") return "nudge";
  return "other";
}

function priorityClass(item: CollectionItem): string {
  return `priority-${priorityBucket(item)}`;
}

function isSessionTriageItem(item: CollectionItem): boolean {
  return Boolean(item.sessionId && (item.currentState || item.nextStep || item.evidenceStatus));
}

function providerLabel(item: CollectionItem): string {
  const providerBadge = item.badges.find((badge) => badge === "codex" || badge === "claude" || badge === "cursor");
  if (providerBadge) return providerBadge.charAt(0).toUpperCase() + providerBadge.slice(1);
  const separator = item.label.indexOf(":");
  return separator > 0 ? item.label.slice(0, separator) : "Session";
}

function evidenceSummary(item: CollectionItem): string {
  const count = item.artifactCount ?? 0;
  if (count > 0) return `${item.evidenceStatus || "ready"} with ${count} artifact${count === 1 ? "" : "s"}`;
  if (item.evidenceStatus === "missing") return "missing; likely needs a nudge";
  if (item.evidenceStatus === "pending") return "pending while session remains active";
  return item.evidenceStatus || "unknown";
}

function statusText(status: string | undefined): string {
  if (status === "unmatched") return "not linked";
  if (status === "likely") return "likely link";
  return status || "unknown";
}

function stateLabel(item: CollectionItem): string {
  const age = shortTimeLabel(item.updatedAt);
  if (item.attention === "running") return "Live now";
  if (item.currentState?.includes(", active")) return `Active ${age}`;
  if (actionLabel(item.nextStep) === "nudge") return `Idle ${age}`;
  return age;
}

function decisionReason(item: CollectionItem): string {
  const action = actionLabel(item.nextStep);
  if (action === "decide") return "Needs matching decision.";
  if (action === "confirm") return "Confirm likely match.";
  if (action === "nudge") return "Waiting for evidence.";
  if (action === "watch") return item.evidenceStatus === "ready" ? "Live with evidence ready." : "Watching for output.";
  if (action === "review") return "Review detail before deciding next step.";
  return item.nextStep || "Choose the next action for this session.";
}

function compactCardFact(item: CollectionItem): string {
  const current = item.currentState || item.subtitle || "No current state recorded.";
  const state = current
    .replace(/; assistant:.*/i, "")
    .split(";")
    .map((part) => part.trim())
    .filter(Boolean)
    .slice(0, 2)
    .join("; ");
  return [evidenceSummary(item), state].filter(Boolean).join(" | ");
}

function actionLabel(nextStep: string | undefined): string {
  const text = (nextStep || "").toLowerCase();
  if (text.startsWith("keep watching")) return "watch";
  if (text.startsWith("confirm")) return "confirm";
  if (text.startsWith("send")) return "nudge";
  if (text.startsWith("review")) return "review";
  if (text.startsWith("decide")) return "decide";
  return "action";
}

function primaryActionText(action: string): string {
  switch (action) {
    case "decide":
      return "Decide now";
    case "confirm":
      return "Confirm link";
    case "nudge":
      return "Send nudge";
    case "review":
      return "Review details";
    case "watch":
      return "Keep watching";
    default:
      return "Choose action";
  }
}

function shortTimeLabel(value: string | undefined): string {
  if (!value) return "unknown";
  const then = Date.parse(value);
  if (Number.isNaN(then)) return value.slice(0, 10);
  const deltaMs = Date.now() - then;
  if (deltaMs < -60_000) return "future";
  if (deltaMs < 60_000) return "now";
  const minutes = Math.floor(deltaMs / 60_000);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}

function cleanClass(value: string | undefined): string {
  return (value || "unknown").toLowerCase().replace(/[^a-z0-9-]/g, "-");
}

function readStateRank(state: ReadState | undefined): number {
  switch (state) {
    case "running":
      return 0;
    case "unread":
      return 1;
    case "read":
      return 2;
    default:
      return 3;
  }
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

function HtmlBlock({ block }: { block: CanvasBlock }) {
  const html = readBlockValue<string>(block, "html");
  const screenshotUrl = resolveAssetSrc(readBlockValue<string>(block, "screenshotUrl"), readBlockValue<string>(block, "screenshotAssetId"));
  const title = readBlockValue<string>(block, "title");
  const caption = readBlockValue<string>(block, "caption");
  const sandbox = readBlockValue<string>(block, "sandbox") === "relaxed" ? "allow-scripts" : "";
  const requestedHeight = Number(readBlockValue<number>(block, "height") ?? 0);
  const height = Number.isFinite(requestedHeight) && requestedHeight > 0 ? Math.min(Math.max(requestedHeight, 80), 1600) : 320;

  return (
    <figure className="hub-surface hub-html">
      {title ? <strong>{title}</strong> : null}
      {html ? (
        <iframe
          className="hub-html-frame"
          title={title || `html block ${block.id}`}
          srcDoc={html}
          sandbox={sandbox}
          loading="lazy"
          referrerPolicy="no-referrer"
          style={{ width: "100%", height, border: 0 }}
        />
      ) : screenshotUrl ? (
        <img className="hub-html-screenshot" src={screenshotUrl} alt={title || "HTML screenshot"} />
      ) : (
        <div className="hub-image-placeholder">HTML block has no body or screenshot.</div>
      )}
      {caption ? <figcaption>{caption}</figcaption> : null}
    </figure>
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
  const subtitle = typeof record.subtitle === "string" ? record.subtitle : undefined;
  const badges = Array.isArray(record.badges) ? record.badges.map(String) : [];
  const parsed = parseSessionSubtitle(subtitle);
  const evidenceStatus = typeof record.evidenceStatus === "string" ? record.evidenceStatus : evidenceStatusFrom(parsed.evidence || badgeValue(badges, "evidence"));
  const artifactCount = typeof record.artifactCount === "number" ? record.artifactCount : numberFromBadge(badges, "artifacts");
  return {
    id,
    sessionId: typeof record.sessionId === "string" ? record.sessionId : id.startsWith("session-") ? id : undefined,
    label,
    subtitle,
    status: typeof record.status === "string" ? record.status : undefined,
    attention: typeof record.attention === "string" ? record.attention : badgeValue(badges, "attention"),
    updatedAt: typeof record.updatedAt === "string" ? record.updatedAt : typeof record.addedAt === "string" ? record.addedAt : undefined,
    purpose: typeof record.purpose === "string" ? record.purpose : parsed.purpose,
    currentState: typeof record.currentState === "string" ? record.currentState : parsed.currentState,
    evidenceStatus,
    nextStep: typeof record.nextStep === "string" ? record.nextStep : parsed.nextStep,
    artifactCount,
    planLabel: typeof record.planLabel === "string" ? record.planLabel : parsed.planLabel,
    badges,
    blockIds,
  };
}

function parseSessionSubtitle(subtitle: string | undefined): Partial<Pick<CollectionItem, "purpose" | "currentState" | "nextStep" | "planLabel">> & { evidence?: string } {
  if (!subtitle) return {};
  return {
    purpose: readSubtitleSection(subtitle, "Purpose", ["Plan", "Now", "Evidence", "Next"]),
    planLabel: readSubtitleSection(subtitle, "Plan", ["Now", "Evidence", "Next"]),
    currentState: readSubtitleSection(subtitle, "Now", ["Evidence", "Next"]),
    evidence: readSubtitleSection(subtitle, "Evidence", ["Next"]),
    nextStep: readSubtitleSection(subtitle, "Next", []),
  };
}

function readSubtitleSection(text: string, label: string, nextLabels: string[]): string | undefined {
  const startToken = `${label}: `;
  const start = text.indexOf(startToken);
  if (start < 0) return undefined;
  const valueStart = start + startToken.length;
  const ends = nextLabels
    .map((next) => text.indexOf(` - ${next}: `, valueStart))
    .filter((position) => position >= 0);
  const end = ends.length ? Math.min(...ends) : text.length;
  const value = text.slice(valueStart, end).trim();
  return value || undefined;
}

function badgeValue(badges: string[], prefix: string): string | undefined {
  const value = badges.find((badge) => badge.startsWith(`${prefix}:`));
  return value ? value.slice(prefix.length + 1) : undefined;
}

function numberFromBadge(badges: string[], prefix: string): number | undefined {
  const value = badgeValue(badges, prefix);
  if (!value) return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function evidenceStatusFrom(value: string | undefined): string | undefined {
  if (!value) return undefined;
  const first = value.trim().split(/\s+/)[0];
  return first || undefined;
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

function absoluteImageURL(raw: string | undefined) {
  if (!raw) return "";
  try {
    const url = new URL(raw.trim());
    return url.protocol === "http:" || url.protocol === "https:" ? url.toString() : "";
  } catch {
    return "";
  }
}

function resolveAssetSrc(rawUrl: string | undefined, assetId: string | undefined) {
  const absolute = absoluteImageURL(rawUrl);
  if (absolute) return absolute;
  const id = (assetId || "").trim();
  if (!id) return "";
  return `/api/hub/assets/${encodeURIComponent(id)}`;
}

function requireOK(response: Response) {
  if (!response.ok) throw new Error(String(response.status));
  return response;
}

function sameCanvasVersion(first: AgentCanvas, second: AgentCanvas) {
  return first.version === second.version && first.lastEventId === second.lastEventId && first.updatedAt === second.updatedAt;
}

function describeCanvasUpdate(previous: AgentCanvas, next: AgentCanvas): CanvasUpdateMeta {
  const previousBlocks = new Map(previous.blocks.map((block) => [block.id, stableBlockSignature(block)]));
  const nextBlocks = new Map(next.blocks.map((block) => [block.id, stableBlockSignature(block)]));
  const appendedBlockIDs = next.blocks.filter((block) => !previousBlocks.has(block.id)).map((block) => block.id);
  const removedBlockIDs = previous.blocks.filter((block) => !nextBlocks.has(block.id)).map((block) => block.id);
  const replacedBlockIDs = next.blocks
    .filter((block) => previousBlocks.has(block.id) && previousBlocks.get(block.id) !== nextBlocks.get(block.id))
    .map((block) => block.id);
  const changedBlockIDs = [...new Set([...appendedBlockIDs, ...replacedBlockIDs])];
  return {
    kind: next.mode === "dynamic" || previous.mode === "dynamic" || next.lastEventId !== previous.lastEventId ? "dynamic" : "static",
    previousVersion: previous.version,
    nextVersion: next.version,
    changedBlockIDs,
    appendedBlockIDs,
    removedBlockIDs,
  };
}

function stableBlockSignature(block: CanvasBlock) {
  return JSON.stringify({ kind: block.kind, payload: block.payload, raw: block.raw });
}

function captureScrollAnchor(): ScrollAnchor | undefined {
  const blocks = [...document.querySelectorAll<HTMLElement>(".canvas-blocks [data-block-id]")];
  const candidate = blocks.find((block) => block.getBoundingClientRect().bottom > 8);
  if (!candidate?.dataset.blockId) return undefined;
  return { blockID: candidate.dataset.blockId, top: candidate.getBoundingClientRect().top };
}

function isAtLiveEdge() {
  const doc = document.documentElement;
  return window.scrollY + window.innerHeight >= doc.scrollHeight - 140;
}
