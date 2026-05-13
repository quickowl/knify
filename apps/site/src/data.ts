import type { CanvasCopy, CanvasExample } from "./types";

export const publicCanvases: CanvasExample[] = [
  {
    id: "pixel-audit-acme",
    ticket: "PIX-184",
    title: "Acme pixel drift report",
    description: "Visual fidelity audit across 18 pages with severity buckets and forkable fixes.",
    agent: "pixelfix-agent",
    version: 3,
    live: true,
    viewers: 4,
    kind: "metric",
    private: false,
  },
  {
    id: "code-review-902",
    ticket: "REV-902",
    title: "Payments service review",
    description: "Inline diff summary, six review threads, two blocking issues.",
    agent: "reviewer-agent",
    version: 7,
    live: false,
    viewers: 2,
    kind: "tasks",
    private: false,
  },
  {
    id: "incident-1182",
    ticket: "INC-1182",
    title: "Live incident: auth p99",
    description: "Streaming runbook with timeline, command log, and on-call handoff.",
    agent: "ops-runbook",
    version: 12,
    live: true,
    viewers: 9,
    kind: "incident",
    private: false,
  },
  {
    id: "spike-vec-search",
    ticket: "SPK-44",
    title: "Vector search benchmark",
    description: "Latency and recall comparison across six vector stores.",
    agent: "bench-agent",
    version: 2,
    live: false,
    viewers: 3,
    kind: "chart",
    private: false,
  },
  {
    id: "onboarding-flow",
    ticket: "DSG-71",
    title: "Onboarding flow review",
    description: "Annotated screens, contrast notes, and copy fixes pinned by frame.",
    agent: "design-critique",
    version: 4,
    live: false,
    viewers: 6,
    kind: "design",
    private: false,
  },
  {
    id: "launch-tracker",
    ticket: "LCH-7",
    title: "Launch readiness tracker",
    description: "Cross-team checklist with owners, blockers, and viewer-ready status.",
    agent: "tracker-agent",
    version: 18,
    live: true,
    viewers: 22,
    kind: "tasks",
    private: false,
  },
];

export const privateCanvases: CanvasExample[] = [
  {
    id: "mig-postgres-15",
    ticket: "INF-301",
    title: "Postgres 15 migration plan",
    description: "Pre-flight checks, rollback strategy, and downtime budget.",
    agent: "infra-agent",
    version: 6,
    live: false,
    viewers: 3,
    kind: "tasks",
    private: true,
  },
  {
    id: "cust-feedback-q2",
    ticket: "PRD-118",
    title: "Q2 feedback themes",
    description: "1,204 tickets clustered into six product themes with source quotes.",
    agent: "voc-agent",
    version: 3,
    live: true,
    viewers: 5,
    kind: "chart",
    private: true,
  },
  {
    id: "sec-audit-jul",
    ticket: "SEC-22",
    title: "July security audit",
    description: "Fourteen findings, two critical, with owners and due dates.",
    agent: "sec-scan",
    version: 2,
    live: true,
    viewers: 8,
    kind: "metric",
    private: true,
  },
];

export const canvasCopy: Record<string, CanvasCopy> = {
  "pixel-audit-acme": {
    headline: "24 visual drifts, 3 critical, on 18 pages.",
    seeing:
      "A pixel-audit canvas. The agent diff-rendered Acme's site against the latest design system and clustered every drift by severity.",
    checks: [
      "Start with Critical: 3 issues need a human or agent fix before release.",
      "The bar strip shows when drift spiked; days 5-7 are the regression window.",
      "Score 87/100 is the latest run. Fork this canvas to scope fixes into a sprint.",
    ],
  },
  "code-review-902": {
    headline: "Payments service: 6 threads, 2 blocking.",
    seeing:
      "A review canvas pinned to REV-902. Reviewer-agent annotated the diff and surfaced the threads with the highest blast radius.",
    checks: [
      "Two critical rows block merge: retry race and swallowed error path.",
      "Completed rows are already addressed; skim them for regression risk.",
      "Fork the canvas to split review ownership across people and agents.",
    ],
  },
  "incident-1182": {
    headline: "Auth p99 spiked to 1,840ms and is recovering.",
    seeing:
      "A live runbook canvas. It keeps incident context, command output, and recovery state together under one shareable link.",
    checks: [
      "p99 and error rate confirm the user-visible blast radius.",
      "The command log shows the root cause: redis-2.cache refused connections.",
      "The latest event shows recovery, so this is ready for handoff.",
    ],
  },
  "spike-vec-search": {
    headline: "pgvector leads six stores on recall@10.",
    seeing:
      "A benchmark canvas. Six vector stores ran the same workload, and each row can link back to raw runs or reproducible scripts.",
    checks: [
      "pgvector leads at 88%, with the lowest operational surface for this project.",
      "Qdrant and Weaviate are close enough to revisit if filters matter more.",
      "Fork v2 and rerun with your own dataset before committing.",
    ],
  },
  "onboarding-flow": {
    headline: "4 screens, 3 issues, 1 below the fold.",
    seeing:
      "A design review canvas with screen thumbnails and task rows pinned to exact frames.",
    checks: [
      "Screen 02 already has a copy fix ready to ship.",
      "Screen 03 needs contrast remediation for avatar fallback color.",
      "Screen 04 has the blocking UX issue: empty-state CTA below the fold.",
    ],
  },
  "launch-tracker": {
    headline: "5 of 28 launch readiness items remain open.",
    seeing:
      "A delivery canvas for launch coordination. It gives every owner one live artifact rather than a status thread.",
    checks: [
      "Two rows are complete: marketing metadata and pricing copy are locked.",
      "The empty-state task is critical and blocks GA.",
      "Twenty-two viewers are using the same artifact for the readout.",
    ],
  },
  "mig-postgres-15": {
    headline: "Postgres 15 cutover: 4 of 6 checks green.",
    seeing:
      "A private migration canvas with pre-flight checks, rollback notes, and downtime budget pinned to INF-301.",
    checks: [
      "Replica lag and extension compatibility are verified.",
      "Staging cutover dry-run remains critical; schedule it before production.",
      "Connection pool sizing is still a warning and should be bumped.",
    ],
  },
  "cust-feedback-q2": {
    headline: "1,204 tickets became 6 product themes.",
    seeing:
      "A voice-of-customer canvas. The agent clustered raw tickets, ranked themes, and kept sample quotes behind each row.",
    checks: [
      "Performance leads the cluster, so a focused sprint is justified.",
      "Pricing clarity and mobile parity trail close behind.",
      "Open any row in a real viewer to inspect source tickets and quotes.",
    ],
  },
  "sec-audit-jul": {
    headline: "July audit: 14 findings, 2 critical.",
    seeing:
      "A private security canvas with severity counts, owners, and due dates under a stable internal link.",
    checks: [
      "Two critical findings already have owners assigned.",
      "Score 64/100 is the remediation baseline for weekly reruns.",
      "Live rescans append events so viewers always get current state.",
    ],
  },
};
