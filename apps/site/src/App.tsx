import { useEffect, useMemo, useState } from "react";
import { canvasCopy, privateCanvases, publicCanvases } from "./data";
import HubCanvasViewer from "./CanvasViewer";
import DashboardApp from "./Dashboard";
import type { CanvasExample } from "./types";

type Theme = "light" | "dark";
type GalleryTab = "public" | "live" | "private";

const allCanvases = [...publicCanvases, ...privateCanvases];

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

function CopyIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none">
      <rect x="9" y="9" width="13" height="13" rx="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  );
}

function ForkIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none">
      <circle cx="6" cy="3" r="2" />
      <circle cx="6" cy="21" r="2" />
      <circle cx="18" cy="6" r="2" />
      <path d="M6 5v6a3 3 0 0 0 3 3h6M6 19v-5" />
    </svg>
  );
}

function parseCanvasId(value: string) {
  const normalized = value.trim().replace(/^canvas:\/\//, "");
  const lastSegment = normalized.split("/").filter(Boolean).pop() ?? normalized;
  return lastSegment.split("@")[0].split("?")[0];
}

function canvasUrl(canvas: CanvasExample) {
  return `canvas://hub/c/${canvas.id}@v${canvas.version}`;
}

function Nav({
  theme,
  setTheme,
  signedIn,
  userHandle,
  onSignIn,
  onSignOut,
}: {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  signedIn: boolean;
  userHandle: string;
  onSignIn: () => void;
  onSignOut: () => void;
}) {
  return (
    <header className="nav">
      <div className="nav-inner">
        <div className="nav-left">
          <a className="mark" href="/" aria-label="Knify home">
            <span className="blade" />
            knify
            <span className="version">v0.1</span>
          </a>
          <nav className="nav-links" aria-label="Main navigation">
            <a href="/dashboard">Dashboard</a>
            <a href="#gallery">Gallery</a>
            <a href="#viewer">Viewer</a>
            <a href="#keys">Keys</a>
            <a href="#docs">Docs</a>
          </nav>
        </div>
        <div className="toolbar">
          <div className="segment" aria-label="Theme">
            <button
              className={theme === "light" ? "active" : ""}
              onClick={() => setTheme("light")}
              aria-label="Use light theme"
              title="Light theme"
            >
              <SunIcon />
            </button>
            <button
              className={theme === "dark" ? "active" : ""}
              onClick={() => setTheme("dark")}
              aria-label="Use dark theme"
              title="Dark theme"
            >
              <MoonIcon />
            </button>
          </div>
          {signedIn ? (
            <>
              <span className="connection"><span /> connected · {userHandle}</span>
              <button className="btn btn-small" onClick={onSignOut}>Sign out</button>
            </>
          ) : (
            <>
              <button className="btn btn-small" onClick={onSignIn}>Sign in</button>
              <button className="btn btn-small btn-primary" onClick={onSignIn}>
                <span className="prompt">get a hub key</span>
              </button>
            </>
          )}
        </div>
      </div>
    </header>
  );
}

function Hero({
  active,
  signedIn,
  onOpenCanvas,
}: {
  active: CanvasExample;
  signedIn: boolean;
  onOpenCanvas: (value: string) => string | null;
}) {
  const [url, setUrl] = useState(canvasUrl(active));
  const [error, setError] = useState<string | null>(null);
  const copy = canvasCopy[active.id];

  useEffect(() => {
    setUrl(canvasUrl(active));
    setError(null);
  }, [active]);

  return (
    <main className="hero" id="viewer">
      <div className="hero-grid">
        <section className="manifesto" aria-labelledby="canvas-title">
          <div className="terminal-line">
            <span className="arrow">&gt;</span> cat {active.id}.md<span className="cursor" />
          </div>
          <div className="eyebrow">{active.ticket} · {active.kind} canvas</div>
          <h1 id="canvas-title">{copy.headline}</h1>
          <p className="lede">
            <strong>What you're seeing:</strong> {copy.seeing}
          </p>
          <div className="read-card">
            <div className="read-title">How to read it</div>
            {copy.checks.map((check, index) => (
              <div className="read-row" key={check}>
                <span>{String(index + 1).padStart(2, "0")}</span>
                <p>{check}</p>
              </div>
            ))}
          </div>
          <div className="hero-actions">
            <button className="btn btn-primary btn-large">
              <span className="prompt">fork this canvas</span>
            </button>
            <button className="btn btn-ghost btn-large">knify init</button>
          </div>
          <div className="meta-strip">
            <span><strong>{active.live ? "live" : "static"}</strong> canvas</span>
            <span><strong>{signedIn ? "private" : "public"}</strong> ready</span>
            <span><strong>agent</strong>-agnostic</span>
          </div>
        </section>

        <section className="dashboard" aria-label="Canvas dashboard">
          <div className="address-bar">
            <div className="prefix">canvas://</div>
            <input
              value={url.replace(/^canvas:\/\//, "")}
              onChange={(event) => setUrl(`canvas://${event.target.value}`)}
              spellCheck={false}
              aria-label="Canvas URL"
            />
            <span className="version-pill">@v{active.version}</span>
            <button
              onClick={() => setError(onOpenCanvas(url))}
              aria-label="Open canvas URL"
            >
              open
            </button>
          </div>
          {error && <div className="address-error">{error}</div>}
          <CanvasViewer canvas={active} signedIn={signedIn} />
        </section>
      </div>
    </main>
  );
}

function CanvasViewer({ canvas, signedIn }: { canvas: CanvasExample; signedIn: boolean }) {
  return (
    <div className="viewer">
      <div className="viewer-bar">
        <div className="viewer-title">
          <span className="dots"><span /><span /><span /></span>
          <span>{canvas.ticket} · {canvas.title}</span>
        </div>
        <div className="viewer-actions">
          {canvas.live && <span className="badge badge-live">live</span>}
          {canvas.private && signedIn && <span className="badge">private</span>}
          <button>fork</button>
          <button>embed</button>
          <button>share</button>
        </div>
      </div>
      <div className="viewer-pane">
        <div className="canvas-head">
          <div>
            <h2>{canvas.title}</h2>
            <p>{canvas.agent} · v{canvas.version} · {canvas.viewers} viewers</p>
          </div>
          <div className="mini-badges">
            <span><ForkIcon /> 2 forks</span>
            <span>4 embeds</span>
          </div>
        </div>
        <CanvasBody canvas={canvas} />
      </div>
    </div>
  );
}

function CanvasBody({ canvas }: { canvas: CanvasExample }) {
  if (canvas.kind === "metric") return <MetricCanvas canvas={canvas} />;
  if (canvas.kind === "tasks") return <TasksCanvas canvas={canvas} />;
  if (canvas.kind === "incident") return <IncidentCanvas />;
  if (canvas.kind === "chart") return <ChartCanvas canvas={canvas} />;
  return <DesignCanvas />;
}

function MetricCanvas({ canvas }: { canvas: CanvasExample }) {
  const isSecurity = canvas.id === "sec-audit-jul";
  const metrics = isSecurity
    ? [
        ["Findings", "14", "+3"],
        ["Critical", "2", "+1", "warn"],
        ["Owners", "8", "assigned"],
        ["Score", "64", "baseline", "signal"],
      ]
    : [
        ["Issues", "24", "+4"],
        ["Critical", "3", "+1", "warn"],
        ["Pages", "18", "stable"],
        ["Score", "87", "+12", "signal"],
      ];

  return (
    <>
      <div className="metric-grid">
        {metrics.map(([label, value, delta, tone]) => (
          <div className="metric-cell" key={label}>
            <span>{label}</span>
            <strong className={tone ?? ""}>{value}</strong>
            <small>{delta}</small>
          </div>
        ))}
      </div>
      <div className="bars" aria-label="Severity over time">
        {[30, 48, 40, 55, 66, 90, 74, 58, 44, 62, 80, 72, 55, 38].map((height, index) => (
          <span
            className={index >= 4 && index <= 7 ? "signal" : ""}
            style={{ height: `${height}%` }}
            key={`${height}-${index}`}
          />
        ))}
      </div>
      <div className="canvas-note">severity · last 14 days</div>
    </>
  );
}

function TasksCanvas({ canvas }: { canvas: CanvasExample }) {
  const tasks = canvas.id === "launch-tracker"
    ? [
        [true, "LCH-7.1", "Marketing site meta tags", "info"],
        [true, "LCH-7.2", "Pricing page copy review", "info"],
        [false, "LCH-7.3", "Onboarding empty state", "crit"],
        [false, "LCH-7.4", "Webhook quotas: docs and UI", "warn"],
        [false, "LCH-7.5", "Status page incident template", "info"],
      ]
    : canvas.id === "mig-postgres-15"
      ? [
          [true, "INF-301.1", "Snapshot replica: verify lag", "info"],
          [true, "INF-301.2", "Test extension compatibility", "info"],
          [false, "INF-301.3", "Cutover dry-run in staging", "crit"],
          [false, "INF-301.4", "Update connection pool sizing", "warn"],
        ]
      : [
          [true, "REV-902.1", "retry.ts:144 race condition", "crit"],
          [false, "REV-902.2", "retry.ts:201 swallowed error", "crit"],
          [true, "REV-902.3", "Add 3-leg refund test", "warn"],
          [false, "REV-902.4", "Document idempotency keys", "info"],
          [true, "REV-902.5", "Redact PAN in webhook logs", "warn"],
          [false, "REV-902.6", "Add kill-switch for new path", "info"],
        ];

  return (
    <div className="task-list">
      {tasks.map(([done, id, text, severity]) => (
        <div className="task-row" key={String(id)}>
          <span className={`check ${done ? "done" : ""}`} />
          <span className="task-id">{id}</span>
          <span className="task-text">{text}</span>
          <span className={`severity ${severity}`}>{severity}</span>
        </div>
      ))}
    </div>
  );
}

function IncidentCanvas() {
  return (
    <>
      <div className="metric-grid three">
        <div className="metric-cell">
          <span>p99 (ms)</span>
          <strong className="warn">1,840</strong>
          <small>+1,200 vs baseline</small>
        </div>
        <div className="metric-cell">
          <span>Error rate</span>
          <strong className="warn">4.2%</strong>
          <small>+4.1</small>
        </div>
        <div className="metric-cell">
          <span>On-call</span>
          <strong>@a.kim</strong>
          <small>handoff 04:00 UTC</small>
        </div>
      </div>
      <div className="log-panel">
        <p><span>14:03:01</span> kubectl describe pod auth-7f9 <b>ok</b></p>
        <p><span>14:03:14</span> tail -f /var/log/auth.log</p>
        <p><span>14:03:42</span> <em>connection refused: redis-2.cache:6379</em></p>
        <p><span>14:04:11</span> kubectl rollout restart deploy/auth</p>
        <p><span>14:05:02</span> <b>p99 recovering: 880ms to 410ms</b></p>
      </div>
    </>
  );
}

function ChartCanvas({ canvas }: { canvas: CanvasExample }) {
  const rows = canvas.id === "cust-feedback-q2"
    ? [
        ["Performance", 84, true],
        ["Pricing clarity", 71, false],
        ["Mobile parity", 62, false],
        ["Onboarding", 58, false],
        ["Search quality", 44, false],
        ["Integrations", 39, false],
      ]
    : [
        ["pgvector", 88, true],
        ["Qdrant", 82, false],
        ["Weaviate", 76, false],
        ["Pinecone", 71, false],
        ["Milvus", 68, false],
        ["Vespa", 64, false],
      ];

  return (
    <div className="chart-list">
      {rows.map(([label, percent, signal]) => (
        <div className="chart-row" key={String(label)}>
          <span>{label}</span>
          <div><span className={signal ? "signal" : ""} style={{ width: `${percent}%` }} /></div>
          <strong>{percent}%</strong>
        </div>
      ))}
    </div>
  );
}

function DesignCanvas() {
  return (
    <>
      <div className="screen-grid">
        {["01 splash", "02 sign in", "03 profile", "04 home"].map((label) => (
          <div className="screen-card" key={label}>
            <span className="screen-dot" />
            <span className="line long" />
            <span className="line short" />
            <span className="block" />
            <span className="line" />
            <span className="line short" />
            <strong>{label}</strong>
          </div>
        ))}
      </div>
      <div className="task-list compact">
        <div className="task-row">
          <span className="check done" />
          <span className="task-id">DSG-71.1</span>
          <span className="task-text">CTA copy: Continue to Get started</span>
          <span className="severity info">copy</span>
        </div>
        <div className="task-row">
          <span className="check" />
          <span className="task-id">DSG-71.2</span>
          <span className="task-text">Avatar fallback color too dim</span>
          <span className="severity warn">visual</span>
        </div>
        <div className="task-row">
          <span className="check" />
          <span className="task-id">DSG-71.3</span>
          <span className="task-text">Empty-state CTA below fold</span>
          <span className="severity crit">UX</span>
        </div>
      </div>
    </>
  );
}

function Gallery({
  active,
  signedIn,
  setActive,
  onSignIn,
}: {
  active: CanvasExample;
  signedIn: boolean;
  setActive: (canvas: CanvasExample) => void;
  onSignIn: () => void;
}) {
  const [tab, setTab] = useState<GalleryTab>("public");
  const items = useMemo(() => {
    if (tab === "live") return publicCanvases.filter((canvas) => canvas.live);
    if (tab === "private") return signedIn ? privateCanvases : [];
    return publicCanvases;
  }, [signedIn, tab]);

  useEffect(() => {
    if (!signedIn && tab === "private") setTab("public");
  }, [signedIn, tab]);

  return (
    <section className="gallery section" id="gallery">
      <div className="section-head">
        <div>
          <div className="eyebrow">canvas:// gallery</div>
          <h2>Try it. Then fork it.</h2>
          <p>Public canvases load instantly. Sign in to unlock private project artifacts and API keys.</p>
        </div>
        <div className="tabs" role="tablist" aria-label="Canvas gallery filter">
          <button className={tab === "public" ? "active" : ""} onClick={() => setTab("public")}>Public</button>
          <button className={tab === "live" ? "active" : ""} onClick={() => setTab("live")}>Live</button>
          <button
            className={tab === "private" ? "active" : ""}
            onClick={() => signedIn ? setTab("private") : onSignIn()}
          >
            Private{!signedIn ? " · sign in" : ""}
          </button>
        </div>
      </div>

      {signedIn && tab === "private" && <ApiPanel />}

      <div className="canvas-cards">
        {items.map((canvas) => (
          <button
            className={`canvas-card ${active.id === canvas.id ? "active" : ""}`}
            key={canvas.id}
            onClick={() => {
              setActive(canvas);
              window.scrollTo({ top: 0, behavior: "smooth" });
            }}
          >
            <span className="card-id">
              <span>canvas://{canvas.id}</span>
              <span>@v{canvas.version}</span>
            </span>
            <strong>{canvas.title}</strong>
            <p>{canvas.description}</p>
            <span className="card-foot">
              <span>{canvas.ticket} · {canvas.agent}</span>
              <span>· {canvas.live ? "live" : `${canvas.viewers} viewers`}</span>
            </span>
          </button>
        ))}
      </div>
    </section>
  );
}

function ApiPanel() {
  return (
    <div className="api-panel" id="keys">
      <div className="api-status">
        <span />
        <strong>Connected</strong>
        <small>@ada.l · acme-marketing</small>
      </div>
      <div className="api-key">
        <span>API key</span>
        <code>knfy_live_sk_4f8c...3a1b</code>
        <button><CopyIcon /> copy</button>
      </div>
      <button className="btn btn-small">Manage keys</button>
    </div>
  );
}

function SignInModal({
  onClose,
  onConnect,
}: {
  onClose: () => void;
  onConnect: (handle: string) => void;
}) {
  const [email, setEmail] = useState("ada@acme.dev");

  function connect() {
    const name = email.split("@")[0]?.trim() || "ada";
    onConnect(`@${name.replace(/[^a-z0-9._-]/gi, "").toLowerCase() || "ada"}`);
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="sign-in-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <h2 id="sign-in-title">Connect to hub.knify.dev</h2>
        <p>Mint a mock browser session and unlock private canvases.</p>
        <label>
          Email
          <input
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            autoFocus
          />
        </label>
        <button className="btn btn-primary btn-large modal-submit" onClick={connect}>
          <span className="prompt">connect browser</span>
        </button>
        <div className="modal-foot">
          <button onClick={onClose}>Maybe later</button>
          <span>SSO · GitHub · Google</span>
        </div>
      </div>
    </div>
  );
}

function Footer() {
  return (
    <footer className="footer" id="docs">
      <div>
        <a className="mark" href="/" aria-label="Knify home">
          <span className="blade" />
          knify
        </a>
        <p>Canvas protocol for multi-agent delivery. Open spec, hosted hub, viewers everywhere.</p>
      </div>
      <nav aria-label="Footer links">
        <a href="/dashboard">Dashboard</a>
        <a href="#viewer">Viewer</a>
        <a href="#gallery">Gallery</a>
        <a href="#keys">API keys</a>
        <a href="#docs">Docs</a>
      </nav>
    </footer>
  );
}

export default function App() {
  const isDashboard = window.location.pathname.replace(/\/$/, "") === "/dashboard";
  const isCanvasRoute = window.location.pathname.startsWith("/canvases/");
  const [theme, setTheme] = useState<Theme>("light");
  const [active, setActive] = useState<CanvasExample>(publicCanvases[0]);
  const [signedIn, setSignedIn] = useState(false);
  const [userHandle, setUserHandle] = useState("@ada.l");
  const [modalOpen, setModalOpen] = useState(false);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  useEffect(() => {
    if (!signedIn && active.private) setActive(publicCanvases[0]);
  }, [active.private, signedIn]);

  function openCanvas(value: string) {
    const id = parseCanvasId(value);
    const match = allCanvases.find((canvas) => canvas.id === id);
    if (!match) return `No demo canvas found for "${id}". Pick one from the gallery.`;
    if (match.private && !signedIn) {
      setModalOpen(true);
      return "Sign in to open private canvases.";
    }
    setActive(match);
    return null;
  }

  if (isDashboard) return <DashboardApp />;
  if (isCanvasRoute) return <HubCanvasViewer />;

  return (
    <>
      <Nav
        theme={theme}
        setTheme={setTheme}
        signedIn={signedIn}
        userHandle={userHandle}
        onSignIn={() => setModalOpen(true)}
        onSignOut={() => setSignedIn(false)}
      />
      <Hero active={active} signedIn={signedIn} onOpenCanvas={openCanvas} />
      <Gallery active={active} signedIn={signedIn} setActive={setActive} onSignIn={() => setModalOpen(true)} />
      <Footer />
      {modalOpen && (
        <SignInModal
          onClose={() => setModalOpen(false)}
          onConnect={(handle) => {
            setUserHandle(handle);
            setSignedIn(true);
            setModalOpen(false);
          }}
        />
      )}
    </>
  );
}
