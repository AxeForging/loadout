const stateColor: Record<string, string> = {
  "in-sync": "text-sync",
  missing: "text-warn",
  "drifted-local": "text-warn",
  "drifted-store": "text-drift",
  conflict: "text-danger",
  "n/a": "text-ink-faint",
};

// Square marker + label: checklist encoding, never color alone.
export function StateBadge({ state }: { state: string }) {
  const color = stateColor[state] ?? "text-ink-dim";
  return (
    <span className={`inline-flex items-center gap-1.5 font-mono text-xs ${color}`}>
      <span aria-hidden className="text-[9px] leading-none">
        {state === "n/a" ? "□" : "■"}
      </span>
      {state}
    </span>
  );
}

export function KindTag({ kind }: { kind: string }) {
  return (
    <span className="rounded-sm border border-line px-1.5 py-0.5 font-mono text-[11px] text-ink-dim">
      {kind}
    </span>
  );
}
