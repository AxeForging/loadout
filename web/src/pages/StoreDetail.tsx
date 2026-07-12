import { useState } from "react";
import type { CSSProperties } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { api } from "../api";
import { KindTag } from "../components/StateBadge";

export function StoreDetail() {
  const { artifactId } = useParams({ from: "/store/$artifactId" });
  const detail = useQuery({
    queryKey: ["artifact", artifactId],
    queryFn: () => api.artifact(artifactId),
  });
  const [selected, setSelected] = useState<string | null>(null);

  if (detail.isLoading) return <p className="text-sm text-ink-faint">Loading…</p>;
  if (detail.isError) return <p className="text-sm text-danger">{String(detail.error)}</p>;

  const { artifact, files } = detail.data!;
  const paths = Object.keys(files).sort();
  const active = selected && files[selected] !== undefined ? selected : paths[0];
  const lines = active ? files[active].split("\n").length : 0;

  return (
    <div>
      <nav className="crate">
        <Link to="/store" className="transition-colors hover:text-ink">
          store
        </Link>
        <span aria-hidden className="text-ink-faint">
          /
        </span>
        <span className="normal-case tracking-normal text-ink-dim">{artifact.id}</span>
      </nav>
      <header className="wipe mb-7 mt-3 flex items-center gap-3">
        <h1 className="font-display text-[1.9rem] font-semibold leading-tight text-ink">
          {artifact.id}
        </h1>
        <KindTag kind={artifact.kind} />
        {artifact.origin && artifact.origin !== "local" ? (
          <span className="font-mono text-[11px] text-drift">{artifact.origin}</span>
        ) : null}
      </header>

      <div className="rise overflow-hidden rounded-2xl border border-line" style={{ "--i": 2 } as CSSProperties}>
        {/* file rail: folder-tab strip on top of the sheet */}
        <div className="flex flex-wrap items-center border-b border-line bg-surface/60">
          {paths.map((p) => (
            <button
              key={p}
              onClick={() => setSelected(p)}
              className={`relative px-3.5 py-2 font-mono text-xs transition-colors ${
                p === active ? "text-ink" : "text-ink-faint hover:text-ink-dim"
              }`}
            >
              {p}
              <span
                aria-hidden
                className="absolute inset-x-2 bottom-0 h-[2px] bg-ember transition-transform duration-200"
                style={{
                  transform: p === active ? "scaleX(1)" : "scaleX(0)",
                  transformOrigin: "left center",
                  transitionTimingFunction: "var(--ease-out-quart)",
                }}
              />
            </button>
          ))}
          <span className="ml-auto px-3.5 font-mono text-[11px] text-ink-faint tnum">
            {lines} lines
          </span>
        </div>
        <pre
          key={active}
          className="page max-h-[65vh] overflow-auto p-5 font-mono text-[13px] leading-relaxed text-ink-dim"
        >
          {active ? files[active] : "(empty)"}
        </pre>
      </div>

      <p className="mt-4 max-w-[65ch] text-xs text-ink-faint">
        Edit this file in the store on disk (
        <span className="font-mono">{artifact.path}</span>) — the web UI never modifies artifacts
        silently.
      </p>
    </div>
  );
}
