// Sync-state encoding: every state gets a color AND a distinct marker glyph,
// so state is never communicated by color alone (checklist, not traffic lights).
export const stateSpec: Record<string, { color: string; mark: string; dim: string }> = {
  "in-sync": { color: "text-sync", mark: "■", dim: "bg-sync" },
  missing: { color: "text-warn", mark: "□", dim: "bg-warn" },
  "drifted-local": { color: "text-warn", mark: "◧", dim: "bg-warn" },
  "drifted-store": { color: "text-drift", mark: "◨", dim: "bg-drift" },
  conflict: { color: "text-danger", mark: "✕", dim: "bg-danger" },
  "n/a": { color: "text-ink-faint", mark: "·", dim: "bg-ink-faint" },
};

export function stateOf(state: string) {
  return stateSpec[state] ?? { color: "text-ink-dim", mark: "■", dim: "bg-ink-dim" };
}

export function StateBadge({ state }: { state: string }) {
  const s = stateOf(state);
  return (
    <span className={`inline-flex items-center gap-1.5 font-mono text-xs ${s.color}`}>
      <span aria-hidden className="text-[10px] leading-none">
        {s.mark}
      </span>
      {state}
    </span>
  );
}

// Artifact kinds get small stamped glyphs, like markings on gear.
const kindPaths: Record<string, string> = {
  skill: "M6 1 L7.4 4.6 L11 6 L7.4 7.4 L6 11 L4.6 7.4 L1 6 L4.6 4.6 Z", // spark
  instruction: "M1 2.5 H11 M1 6 H8.5 M1 9.5 H10", // ruled lines
  command: "M2 2.5 L6 6 L2 9.5 M7 9.5 H11", // prompt chevron
  hook: "M8.5 1 V6 A3 3 0 0 1 2.5 6 V4.5", // J-hook
  settings: "M1 4 H11 M1 8 H11 M4 2.5 V5.5 M8 6.5 V9.5", // slider rails
  mcp: "M6 1.5 V4 M6 8 V10.5 M1.5 6 H4 M8 6 H10.5 M6 4 A2 2 0 1 1 6 8 A2 2 0 1 1 6 4", // socket node
};

export function KindGlyph({ kind, className = "" }: { kind: string; className?: string }) {
  const d = kindPaths[kind];
  if (!d)
    return (
      <svg viewBox="0 0 12 12" className={`h-3 w-3 ${className}`} aria-hidden>
        <rect x="2" y="2" width="8" height="8" fill="none" stroke="currentColor" />
      </svg>
    );
  return (
    <svg viewBox="0 0 12 12" className={`h-3 w-3 ${className}`} aria-hidden>
      <path
        d={d}
        fill={kind === "skill" ? "currentColor" : "none"}
        stroke="currentColor"
        strokeWidth="1.2"
        strokeLinecap="square"
      />
    </svg>
  );
}

export function KindTag({ kind }: { kind: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-xs border border-line px-1.5 py-0.5 font-mono text-[11px] text-ink-dim">
      <KindGlyph kind={kind} className="text-ink-faint" />
      {kind}
    </span>
  );
}
