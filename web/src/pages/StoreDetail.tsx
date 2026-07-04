import { useState } from "react";
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
  if (detail.isError)
    return <p className="text-sm text-danger">{String(detail.error)}</p>;

  const { artifact, files } = detail.data!;
  const paths = Object.keys(files).sort();
  const active = selected && files[selected] !== undefined ? selected : paths[0];

  return (
    <div>
      <nav className="rise font-mono text-[11px] text-ink-faint">
        <Link to="/store" className="hover:text-ink">
          store
        </Link>{" "}
        / {artifact.id}
      </nav>
      <header className="rise mb-6 mt-2 flex items-center gap-3">
        <h1 className="font-display text-2xl font-semibold text-ink">{artifact.id}</h1>
        <KindTag kind={artifact.kind} />
        {artifact.origin && artifact.origin !== "local" ? (
          <span className="font-mono text-[11px] text-drift">{artifact.origin}</span>
        ) : null}
      </header>

      {paths.length > 1 ? (
        <div className="rise mb-3 flex flex-wrap gap-1">
          {paths.map((p) => (
            <button
              key={p}
              onClick={() => setSelected(p)}
              className={
                p === active
                  ? "rounded-sm bg-surface px-2.5 py-1 font-mono text-xs text-ink"
                  : "rounded-sm px-2.5 py-1 font-mono text-xs text-ink-faint hover:text-ink"
              }
            >
              {p}
            </button>
          ))}
        </div>
      ) : (
        <p className="rise mb-3 font-mono text-xs text-ink-faint">{active}</p>
      )}

      <pre className="rise overflow-x-auto border border-line bg-surface p-5 font-mono text-[13px] leading-relaxed text-ink-dim">
        {active ? files[active] : "(empty)"}
      </pre>

      <p className="mt-4 max-w-[65ch] text-xs text-ink-faint">
        Edit this file in the store on disk ({artifact.path}) — the web UI never modifies
        artifacts silently.
      </p>
    </div>
  );
}
