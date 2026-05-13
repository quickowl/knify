import { useEffect, useMemo, useState } from "react";
import useSWR from "swr";
import type { CanvasKind } from "./types";

type Theme = "light" | "dark";
type Severity = "green" | "amber" | "red";
type Team = "eng" | "design" | "ops" | "pm" | "infra" | "sec";

type DashboardCanvas = {
  id: string;
  ticket: string;
  title: string;
  agent: string;
  version: number;
  live: boolean;
  kind: CanvasKind;
  severity: Severity;
  team: Team;
  viewers?: number;
  source?: "hub" | "mock";
  summary?: string;
};

type Tile = {
  id: string;
  size: 4 | 6 | 8 | 12;
};

type DataSource = "hub" | "mock";

type DashboardAPIResponse = {
  source: "hub";
  hub?: string;
  canvases: DashboardCanvas[];
};

type HubCanvas = {
  id: string;
  agentId?: string;
  agentID?: string;
  title?: string;
  summary?: string;
  status?: string;
  priority?: string;
  version?: number;
  blocks?: Array<{
    kind?: string;
    title?: string;
    metadata?: Record<string, unknown>;
    payload?: Record<string, unknown>;
  }>;
};

const emptyCanvases: DashboardCanvas[] = [];

async function dashboardFetcher(url: string): Promise<DashboardAPIResponse> {
  const response = await fetch(url, { headers: { "Accept": "application/json" } });
  if (!response.ok) throw new Error(`hub returned ${response.status}`);
  const raw = await response.json() as unknown;
  if (raw && typeof raw === "object" && Array.isArray((raw as { canvases?: unknown }).canvases)) {
    const wrapped = raw as { source?: string; hub?: string; canvases: unknown[] };
    return {
      source: "hub",
      hub: wrapped.hub,
      canvases: wrapped.canvases.map((canvas) => dashboardCanvas(canvas as HubCanvas)),
    };
  }
  const canvases = Array.isArray(raw) ? raw : [];
  return {
    source: "hub",
    canvases: canvases.map((canvas) => dashboardCanvas(canvas as HubCanvas)),
  };
}

function stringValue(value: unknown, fallback = "") {
  return typeof value === "string" && value.trim() ? value : fallback;
}

function booleanValue(value: unknown, fallback = false) {
  return typeof value === "boolean" ? value : fallback;
}

function numberValue(value: unknown, fallback: number) {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function oneOf<T extends string>(value: unknown, allowed: readonly T[], fallback: T): T {
  return typeof value === "string" && allowed.includes(value as T) ? value as T : fallback;
}

function dashboardCanvas(canvas: HubCanvas): DashboardCanvas {
  const metaBlock = canvas.blocks?.find((block) => block.kind === "metadata" && (block.title === "dashboard" || block.payload?.title === "dashboard"));
  const meta = metaBlock?.metadata || (metaBlock?.payload?.metadata as Record<string, unknown> | undefined) || {};
  const prioritySeverity = canvas.priority === "urgent" || canvas.priority === "high" ? "red" : "green";

  return {
    id: canvas.id,
    ticket: stringValue(meta.ticket, canvas.id),
    title: stringValue(canvas.title, canvas.id),
    agent: stringValue(canvas.agentId || canvas.agentID, "unknown-agent"),
    version: numberValue(canvas.version, 1),
    live: booleanValue(meta.live, canvas.status === "in_progress" || canvas.status === "running"),
    kind: oneOf(meta.kind, ["metric", "tasks", "incident", "chart", "design"] as const, "tasks"),
    severity: oneOf(meta.severity, ["green", "amber", "red"] as const, prioritySeverity),
    team: oneOf(meta.team, ["eng", "design", "ops", "pm", "infra", "sec"] as const, "eng"),
    viewers: numberValue(meta.viewers, 0),
    source: "hub",
    summary: stringValue(canvas.summary, ""),
  };
}

function selectedDataSource(): DataSource {
  const params = new URLSearchParams(window.location.search);
  const selected = params.get("data") || params.get("source");
  if (selected === "mock" || params.get("mock") === "1") return "mock";
  return "hub";
}

const mockCanvases: DashboardCanvas[] = [
  { id: "pixel-audit-acme", ticket: "PIX-184", title: "Acme pixel drift", agent: "pixelfix-agent", version: 3, live: true, kind: "metric", severity: "green", team: "design" },
  { id: "code-review-902", ticket: "REV-902", title: "Payments review", agent: "reviewer-agent", version: 7, live: false, kind: "tasks", severity: "amber", team: "eng" },
  { id: "incident-1182", ticket: "INC-1182", title: "Live auth p99", agent: "ops-runbook", version: 12, live: true, kind: "incident", severity: "red", team: "ops" },
  { id: "spike-vec-search", ticket: "SPK-44", title: "Vector search bench", agent: "bench-agent", version: 2, live: false, kind: "chart", severity: "green", team: "eng" },
  { id: "onboarding-flow", ticket: "DSG-71", title: "Onboarding flow", agent: "design-critique", version: 4, live: false, kind: "design", severity: "green", team: "design" },
  { id: "launch-tracker", ticket: "LCH-7", title: "Launch readiness", agent: "tracker-agent", version: 18, live: true, kind: "tasks", severity: "green", team: "pm" },
  { id: "mig-postgres-15", ticket: "INF-301", title: "Postgres 15 cutover", agent: "infra-agent", version: 6, live: false, kind: "tasks", severity: "amber", team: "infra" },
  { id: "cust-feedback-q2", ticket: "PRD-118", title: "Q2 feedback themes", agent: "voc-agent", version: 3, live: true, kind: "chart", severity: "green", team: "pm" },
  { id: "sec-audit-jul", ticket: "SEC-22", title: "July security audit", agent: "sec-scan", version: 2, live: true, kind: "metric", severity: "red", team: "sec" },
  { id: "cost-cloud-may", ticket: "FIN-9", title: "May cloud cost", agent: "finops-agent", version: 5, live: true, kind: "chart", severity: "amber", team: "infra" },
  { id: "api-changelog", ticket: "API-44", title: "API changelog v2", agent: "docs-agent", version: 9, live: false, kind: "tasks", severity: "green", team: "eng" },
  { id: "churn-cohort", ticket: "GRW-12", title: "Churn cohort Apr", agent: "growth-agent", version: 1, live: false, kind: "chart", severity: "green", team: "pm" },
];

const starterBoards: Record<string, Tile[]> = {
  "Eng standup": [
    { id: "code-review-902", size: 6 },
    { id: "incident-1182", size: 6 },
    { id: "launch-tracker", size: 6 },
    { id: "spike-vec-search", size: 6 },
    { id: "api-changelog", size: 12 },
  ],
  "Ops this week": [
    { id: "incident-1182", size: 8 },
    { id: "sec-audit-jul", size: 4 },
    { id: "mig-postgres-15", size: 6 },
    { id: "cost-cloud-may", size: 6 },
  ],
  "Product GA": [
    { id: "launch-tracker", size: 6 },
    { id: "cust-feedback-q2", size: 6 },
    { id: "onboarding-flow", size: 6 },
    { id: "churn-cohort", size: 6 },
    { id: "pixel-audit-acme", size: 12 },
  ],
};

function SunIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none">
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1" />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none">
      <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
    </svg>
  );
}

function DragIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="currentColor" stroke="none">
      <circle cx="9" cy="6" r="1.6" />
      <circle cx="15" cy="6" r="1.6" />
      <circle cx="9" cy="12" r="1.6" />
      <circle cx="15" cy="12" r="1.6" />
      <circle cx="9" cy="18" r="1.6" />
      <circle cx="15" cy="18" r="1.6" />
    </svg>
  );
}

function XIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none">
      <path d="M6 6l12 12M18 6 6 18" />
    </svg>
  );
}

function MetricTile({ canvas }: { canvas: DashboardCanvas }) {
  const values = canvas.id === "sec-audit-jul"
    ? ["14", "2", "9", "64"]
    : ["24", "3", "18", "87"];

  return (
    <>
      <div className="dash-metric-grid">
        {[
          ["Issues", values[0], "+4"],
          ["Critical", values[1], "+1", "warn"],
          ["Pages", values[2], "stable"],
          ["Score", values[3], "+12", "signal"],
        ].map(([label, value, delta, tone]) => (
          <div className="dash-metric-cell" key={label}>
            <span>{label}</span>
            <strong className={tone ?? ""}>{value}</strong>
            <small>{delta}</small>
          </div>
        ))}
      </div>
      <div className="dash-bars">
        {[30, 48, 40, 55, 66, 90, 74, 58, 44, 62, 80, 72].map((height, index) => (
          <span className={index >= 4 && index <= 7 ? "signal" : ""} style={{ height: `${height}%` }} key={`${height}-${index}`} />
        ))}
      </div>
      <p className="dash-note">severity · last 12 runs</p>
    </>
  );
}

function TaskTile({ canvas }: { canvas: DashboardCanvas }) {
  const tasks = canvas.id === "launch-tracker"
    ? [
        [true, "LCH-7.1", "Marketing metadata", "info"],
        [true, "LCH-7.2", "Pricing copy review", "info"],
        [false, "LCH-7.3", "Onboarding empty state", "crit"],
        [false, "LCH-7.4", "Webhook quota docs", "warn"],
      ]
    : canvas.id === "mig-postgres-15"
      ? [
          [true, "INF-301.1", "Replica lag verified", "info"],
          [true, "INF-301.2", "Extension compatibility", "info"],
          [false, "INF-301.3", "Staging cutover dry-run", "crit"],
          [false, "INF-301.4", "Connection pool sizing", "warn"],
        ]
      : canvas.id === "api-changelog"
        ? [
            [true, "API-44.1", "Deprecate /v1/canvas/list", "warn"],
            [true, "API-44.2", "Add /v2/canvas/search", "info"],
            [false, "API-44.3", "OpenAPI 3.1 migration", "info"],
            [false, "API-44.4", "Python SDK return types", "warn"],
          ]
        : [
            [true, "REV-902.1", "retry.ts race condition", "crit"],
            [false, "REV-902.2", "swallowed error path", "crit"],
            [true, "REV-902.3", "refund test coverage", "warn"],
            [false, "REV-902.4", "idempotency note", "info"],
          ];

  return (
    <div className="dash-task-list">
      {tasks.map(([done, id, text, severity]) => (
        <div className="dash-task-row" key={String(id)}>
          <span className={`dash-check ${done ? "done" : ""}`} />
          <span className="dash-task-id">{id}</span>
          <span className="dash-task-text">{text}</span>
          <span className={`severity ${severity}`}>{severity}</span>
        </div>
      ))}
    </div>
  );
}

function IncidentTile() {
  return (
    <>
      <div className="dash-metric-grid three">
        <div className="dash-metric-cell"><span>p99 ms</span><strong className="warn">1,840</strong><small>+1,200</small></div>
        <div className="dash-metric-cell"><span>Error</span><strong className="warn">4.2%</strong><small>+4.1</small></div>
        <div className="dash-metric-cell"><span>On-call</span><strong>@a.kim</strong><small>handoff 04:00</small></div>
      </div>
      <div className="dash-log">
        <p><span>14:03</span> kubectl describe pod auth-7f9 <b>ok</b></p>
        <p><span>14:03</span> <em>connection refused: redis-2.cache</em></p>
        <p><span>14:04</span> kubectl rollout restart deploy/auth</p>
        <p><span>14:05</span> <b>p99 recovering: 880 to 410</b></p>
      </div>
    </>
  );
}

function ChartTile({ canvas }: { canvas: DashboardCanvas }) {
  const rows = canvas.id === "cust-feedback-q2"
    ? [["Performance", 84, true], ["Pricing clarity", 71], ["Mobile parity", 62], ["Onboarding", 58], ["Search quality", 44]]
    : canvas.id === "cost-cloud-may"
      ? [["Compute", 78, true], ["Egress", 64], ["Storage", 42], ["DB RDS", 38], ["Cache", 22]]
      : canvas.id === "churn-cohort"
        ? [["Apr trial", 34, true], ["Apr paid", 18], ["Mar trial", 31], ["Mar paid", 14], ["Feb paid", 11]]
        : [["pgvector", 88, true], ["Qdrant", 82], ["Weaviate", 76], ["Pinecone", 71], ["Milvus", 68]];

  return (
    <div className="dash-chart-list">
      {rows.map(([label, percent, signal]) => (
        <div className="dash-chart-row" key={String(label)}>
          <span>{label}</span>
          <div><span className={signal ? "signal" : ""} style={{ width: `${percent}%` }} /></div>
          <strong>{percent}%</strong>
        </div>
      ))}
    </div>
  );
}

function DesignTile() {
  return (
    <>
      <div className="dash-screen-grid">
        {["01 splash", "02 sign in", "03 profile", "04 home"].map((label) => (
          <div className="dash-screen" key={label}>
            <span />
            <i />
            <b />
            <strong>{label}</strong>
          </div>
        ))}
      </div>
      <div className="dash-task-list compact">
        <div className="dash-task-row"><span className="dash-check done" /><span className="dash-task-id">DSG-71.1</span><span className="dash-task-text">CTA copy updated</span><span className="severity info">copy</span></div>
        <div className="dash-task-row"><span className="dash-check" /><span className="dash-task-id">DSG-71.3</span><span className="dash-task-text">CTA below fold</span><span className="severity crit">UX</span></div>
      </div>
    </>
  );
}

function TileBody({ canvas }: { canvas: DashboardCanvas }) {
  if (canvas.kind === "metric") return <MetricTile canvas={canvas} />;
  if (canvas.kind === "tasks") return <TaskTile canvas={canvas} />;
  if (canvas.kind === "incident") return <IncidentTile />;
  if (canvas.kind === "chart") return <ChartTile canvas={canvas} />;
  return <DesignTile />;
}

function DashboardTile({
  canvas,
  tile,
  index,
  dragging,
  dropTarget,
  onRemove,
  onResize,
  onDragStart,
  onDragEnd,
  onDragOver,
}: {
  canvas: DashboardCanvas | undefined;
  tile: Tile;
  index: number;
  dragging: boolean;
  dropTarget: boolean;
  onRemove: () => void;
  onResize: (size: Tile["size"]) => void;
  onDragStart: () => void;
  onDragEnd: () => void;
  onDragOver: () => void;
}) {
  if (!canvas) return null;

  return (
    <div
      className={`dash-tile size-${tile.size} ${dragging ? "dragging" : ""} ${dropTarget ? "drop-target" : ""}`}
      draggable
      onDragStart={(event) => {
        event.dataTransfer.effectAllowed = "move";
        event.dataTransfer.setData("text/plain", `tile:${index}`);
        onDragStart();
      }}
      onDragEnd={onDragEnd}
      onDragOver={(event) => {
        event.preventDefault();
        onDragOver();
      }}
    >
      <div className="dash-tile-head">
        <div className="dash-tile-left">
          <span className={`dash-sev ${canvas.severity}`} />
          <button className="dash-icon-button" title="Drag to reorder"><DragIcon /></button>
          <span className="dash-tile-title">{canvas.title}</span>
          <span className="dash-tile-sub">· {canvas.ticket} · v{canvas.version}</span>
          {canvas.live && <span className="badge badge-live">live</span>}
        </div>
        <div className="dash-tile-actions">
          <span className="dash-size-seg">
            {[4, 6, 8, 12].map((size) => (
              <button className={tile.size === size ? "active" : ""} key={size} onClick={() => onResize(size as Tile["size"])}>{size}</button>
            ))}
          </span>
          <a className="dash-icon-button" title="Open canvas" href={`/canvases/${encodeURIComponent(canvas.id)}`}>↗</a>
          <button className="dash-icon-button remove" title="Remove" onClick={onRemove}><XIcon /></button>
        </div>
      </div>
      <div className="dash-tile-body">
        <TileBody canvas={canvas} />
      </div>
    </div>
  );
}

export default function DashboardApp() {
  const [theme, setTheme] = useState<Theme>("light");
  const [boards, setBoards] = useState<Record<string, Tile[]>>(starterBoards);
  const [activeBoard, setActiveBoard] = useState("Eng standup");
  const [boardName, setBoardName] = useState("Eng standup");
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<Team | "all" | "live">("all");
  const [dragCanvasId, setDragCanvasId] = useState<string | null>(null);
  const [tileDragIndex, setTileDragIndex] = useState<number | null>(null);
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);
  const [dropActive, setDropActive] = useState(false);
  const dataSource = useMemo(selectedDataSource, []);
  const mockMode = dataSource === "mock";
  const {
    data: hubResponse,
    error: hubError,
    isLoading: hubLoading,
  } = useSWR<DashboardAPIResponse>(mockMode ? null : "/api/hub/canvases", dashboardFetcher, {
    refreshInterval: 10_000,
    revalidateOnFocus: true,
  });

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  useEffect(() => {
    setBoardName(activeBoard);
  }, [activeBoard]);

  const hubCanvases = hubResponse?.canvases ?? emptyCanvases;
  const canvases = useMemo(() => mockMode ? mockCanvases : hubCanvases, [hubCanvases, mockMode]);
  const canvasById = useMemo(() => new Map(canvases.map((canvas) => [canvas.id, canvas])), [canvases]);
  const tiles = boards[activeBoard] ?? [];
  const placedIds = useMemo(() => new Set(tiles.map((tile) => tile.id)), [tiles]);
  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return canvases.filter((canvas) => {
      if (filter === "live" && !canvas.live) return false;
      if (filter !== "all" && filter !== "live" && canvas.team !== filter) return false;
      if (!query) return true;
      return `${canvas.title} ${canvas.ticket} ${canvas.agent}`.toLowerCase().includes(query);
    });
  }, [canvases, filter, search]);

  const liveCount = tiles.filter((tile) => canvasById.get(tile.id)?.live).length;
  const criticalCount = tiles.filter((tile) => canvasById.get(tile.id)?.severity === "red").length;
  const sourceLabel = mockMode ? "mock mode" : hubError ? "hub error" : hubLoading ? "syncing hub" : "hub live";
  const sourceWarn = mockMode || !!hubError || hubLoading;

  function updateBoard(updater: (tiles: Tile[]) => Tile[]) {
    setBoards((current) => ({
      ...current,
      [activeBoard]: updater(current[activeBoard] ?? []),
    }));
  }

  function addCanvas(id: string, atIndex = tiles.length) {
    if (placedIds.has(id)) return;
    updateBoard((current) => {
      const next = [...current];
      next.splice(Math.min(atIndex, next.length), 0, { id, size: 6 });
      return next;
    });
  }

  function moveTile(from: number, to: number) {
    if (from === to) return;
    updateBoard((current) => {
      const next = [...current];
      const [tile] = next.splice(from, 1);
      next.splice(to > from ? to - 1 : to, 0, tile);
      return next;
    });
  }

  function renameBoard() {
    const trimmed = boardName.trim();
    if (!trimmed || trimmed === activeBoard) {
      setBoardName(activeBoard);
      return;
    }
    setBoards((current) => {
      const { [activeBoard]: activeTiles, ...rest } = current;
      return { ...rest, [trimmed]: activeTiles };
    });
    setActiveBoard(trimmed);
  }

  function newBoard() {
    let name = "New dashboard";
    let index = 1;
    while (boards[name]) {
      index += 1;
      name = `New dashboard ${index}`;
    }
    setBoards((current) => ({ ...current, [name]: [] }));
    setActiveBoard(name);
  }

  return (
    <div className="dash-shell">
      <aside className="dash-side">
        <div className="dash-side-top">
          <a className="mark" href="/"><span className="blade" />knify</a>
          <div className="segment">
            <button className={theme === "light" ? "active" : ""} onClick={() => setTheme("light")} aria-label="Use light theme"><SunIcon /></button>
            <button className={theme === "dark" ? "active" : ""} onClick={() => setTheme("dark")} aria-label="Use dark theme"><MoonIcon /></button>
          </div>
        </div>
        <div className="dash-side-search">
          <input placeholder="search canvases..." value={search} onChange={(event) => setSearch(event.target.value)} spellCheck={false} />
        </div>
        <div className="dash-side-label">Filters</div>
        <div className="dash-filter-row">
          {["all", "live", "eng", "design", "ops", "pm", "infra", "sec"].map((item) => (
            <button className={filter === item ? "active" : ""} key={item} onClick={() => setFilter(item as Team | "all" | "live")}>{item}</button>
          ))}
        </div>
        <div className="dash-side-label">
          <span>Canvases</span>
          <span>{filtered.length}</span>
        </div>
        <div className="dash-side-list">
          {filtered.map((canvas) => {
            const placed = placedIds.has(canvas.id);
            return (
              <button
                className={`dash-side-item ${placed ? "placed" : ""}`}
                draggable={!placed}
                key={canvas.id}
                onClick={() => !placed && addCanvas(canvas.id)}
                onDragStart={(event) => {
                  event.dataTransfer.effectAllowed = "copy";
                  event.dataTransfer.setData("text/plain", `canvas:${canvas.id}`);
                  setDragCanvasId(canvas.id);
                }}
                onDragEnd={() => setDragCanvasId(null)}
                title={placed ? "Already on this dashboard" : "Click or drag to add"}
              >
                <span className={`dash-sev ${canvas.severity}`} />
                <span>{canvas.title}</span>
                <small>{canvas.ticket}</small>
              </button>
            );
          })}
        </div>
        <div className="dash-side-bottom">
          <span className={`dash-connected ${sourceWarn ? "warn" : ""}`}>{sourceLabel}</span>
          <span>@ada.l</span>
        </div>
      </aside>

      <main className="dash-main">
        <header className="dash-main-top">
          <div className="dash-crumbs">
            <span>workspace</span>
            <span>/</span>
            <span>dashboards</span>
            <span>/</span>
            <input
              value={boardName}
              onChange={(event) => setBoardName(event.target.value)}
              onBlur={renameBoard}
              onKeyDown={(event) => {
                if (event.key === "Enter") event.currentTarget.blur();
              }}
            />
          </div>
          <div className="dash-top-actions">
            <span className="dash-meta"><strong>{tiles.length}</strong> tiles · <strong>{liveCount}</strong> live · <strong className={criticalCount ? "critical" : ""}>{criticalCount}</strong> critical</span>
            <div className="dash-board-tabs">
              {Object.keys(boards).map((name) => (
                <button className={activeBoard === name ? "active" : ""} key={name} onClick={() => setActiveBoard(name)}>{name}</button>
              ))}
              <button className="add" onClick={newBoard}>+</button>
            </div>
            <button className="btn btn-small">share</button>
            <button className="btn btn-small btn-primary"><span className="prompt">save</span></button>
          </div>
        </header>

        <div
          className="dash-board"
          onDragOver={(event) => {
            event.preventDefault();
            setDropActive(true);
          }}
          onDragLeave={(event) => {
            if (event.currentTarget === event.target) setDropActive(false);
          }}
          onDrop={(event) => {
            event.preventDefault();
            const data = event.dataTransfer.getData("text/plain");
            if (data.startsWith("canvas:")) addCanvas(data.replace("canvas:", ""), hoverIndex ?? tiles.length);
            if (tileDragIndex !== null && hoverIndex !== null) moveTile(tileDragIndex, hoverIndex);
            setDropActive(false);
            setDragCanvasId(null);
            setTileDragIndex(null);
            setHoverIndex(null);
          }}
        >
          <div className={`dash-grid ${dropActive || dragCanvasId ? "drop-active" : ""}`}>
            {!mockMode && hubLoading && canvases.length === 0 ? (
              <div className="dash-empty">
                <span>...</span>
                <h1>Loading Hub canvases.</h1>
                <p>The dashboard is waiting for the Hub API. Mock canvases are only shown when mock mode is explicitly selected.</p>
              </div>
            ) : !mockMode && hubError && canvases.length === 0 ? (
              <div className="dash-empty">
                <span>!</span>
                <h1>Hub API unavailable.</h1>
                <p>The dashboard did not fall back to mock data. Open with ?data=mock when you intentionally want mock canvases.</p>
              </div>
            ) : !mockMode && canvases.length === 0 ? (
              <div className="dash-empty">
                <span>0</span>
                <h1>No Hub canvases yet.</h1>
                <p>Publish canvases to the Hub or open with ?data=mock when you intentionally want mock canvases.</p>
              </div>
            ) : tiles.length === 0 ? (
              <div className="dash-empty">
                <span>▸▸</span>
                <h1>This dashboard is empty.</h1>
                <p>Drag a canvas from the sidebar or click it to add it here. Resize tiles with 4 / 6 / 8 / 12, drag headers to reorder, and remove tiles with the x button.</p>
              </div>
            ) : (
              tiles.map((tile, index) => (
                <DashboardTile
                  key={`${tile.id}-${index}`}
                  canvas={canvasById.get(tile.id)}
                  tile={tile}
                  index={index}
                  dragging={tileDragIndex === index}
                  dropTarget={hoverIndex === index && tileDragIndex !== null && tileDragIndex !== index}
                  onRemove={() => updateBoard((current) => current.filter((_, tileIndex) => tileIndex !== index))}
                  onResize={(size) => updateBoard((current) => current.map((item, tileIndex) => tileIndex === index ? { ...item, size } : item))}
                  onDragStart={() => setTileDragIndex(index)}
                  onDragEnd={() => {
                    setTileDragIndex(null);
                    setHoverIndex(null);
                  }}
                  onDragOver={() => setHoverIndex(index)}
                />
              ))
            )}
          </div>
        </div>
      </main>
    </div>
  );
}
