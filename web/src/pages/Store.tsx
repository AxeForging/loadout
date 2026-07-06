import { useState } from "react";
import type { CSSProperties } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api } from "../api";
import { PageHeader } from "../components/Layout";
import { KindGlyph } from "../components/StateBadge";

export function StorePage() {
  const artifacts = useQuery({ queryKey: ["artifacts"], queryFn: api.artifacts });
  const [kind, setKind] = useState<string | null>(null);

  const all = artifacts.data ?? [];
  const kinds = [...new Set(all.map((a) => a.kind))];
  const shown = kind ? all.filter((a) => a.kind === kind) : all;

  return (
    <div>
      <PageHeader
        crate="armory / store"
        title="Canonical store"
        sub="Every artifact here is the single source of truth — agents receive rendered copies on sync."
      />

      {artifacts.isLoading ? (
        <p className="text-sm text-ink-faint">Loading…</p>
      ) : all.length === 0 ? (
        <p className="max-w-[65ch] text-sm text-ink-dim">
          Nothing here yet. The{" "}
          <Link to="/builder" className="text-ember transition-colors hover:text-ember-bright">
            Builder
          </Link>{" "}
          generates a full kit from your conventions.
        </p>
      ) : (
        <>
          {kinds.length > 1 ? (
            <div className="rise mb-5 flex flex-wrap items-center gap-2">
              <button className="chip font-mono text-xs" data-on={kind === null} onClick={() => setKind(null)}>
                all <span className="tnum text-ink-faint">{all.length}</span>
              </button>
              {kinds.map((k) => (
                <button
                  key={k}
                  className="chip inline-flex items-center gap-1.5 font-mono text-xs"
                  data-on={kind === k}
                  onClick={() => setKind(kind === k ? null : k)}
                >
                  <KindGlyph kind={k} />
                  {k} <span className="tnum text-ink-faint">{all.filter((a) => a.kind === k).length}</span>
                </button>
              ))}
            </div>
          ) : null}

          <ul key={kind ?? "all"} className="divide-y divide-line border-y border-line">
            {shown.map((a, i) => (
              <li key={a.id} className="wipe" style={{ "--i": Math.min(i, 12) } as CSSProperties}>
                <Link
                  to="/store/$artifactId"
                  params={{ artifactId: a.id }}
                  className="brk group flex items-center gap-4 py-3 pl-1 pr-2 transition-colors hover:bg-surface/50"
                >
                  <KindGlyph kind={a.kind} className="flex-none text-ink-faint transition-colors group-hover:text-ember" />
                  <span className="w-56 truncate font-mono text-sm text-ink transition-colors group-hover:text-ember-bright">
                    {a.id}
                  </span>
                  <span className="font-mono text-[11px] text-ink-dim">{a.kind}</span>
                  <span className="min-w-0 flex-1 truncate font-mono text-xs text-ink-faint">
                    {a.path}
                  </span>
                  {a.origin?.startsWith("source:") ? (
                    <span className="font-mono text-[11px] text-drift">{a.origin}</span>
                  ) : null}
                  <span
                    aria-hidden
                    className="font-mono text-xs text-ink-faint opacity-0 transition-all duration-200 group-hover:translate-x-0.5 group-hover:text-ember group-hover:opacity-100"
                  >
                    ▸
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
