import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api } from "../api";
import { PageHeader } from "../components/Layout";
import { KindTag } from "../components/StateBadge";

export function StorePage() {
  const artifacts = useQuery({ queryKey: ["artifacts"], queryFn: api.artifacts });

  return (
    <div>
      <PageHeader
        crate="armory / store"
        title="Canonical store"
        sub="Every artifact here is the single source of truth — agents receive rendered copies on sync."
      />

      {artifacts.isLoading ? (
        <p className="text-sm text-ink-faint">Loading…</p>
      ) : (artifacts.data ?? []).length === 0 ? (
        <p className="max-w-[65ch] text-sm text-ink-dim">
          Nothing here yet. The{" "}
          <Link to="/builder" className="text-ember hover:text-ember-bright">
            Builder
          </Link>{" "}
          generates a full kit from your conventions.
        </p>
      ) : (
        <ul className="divide-y divide-line border-y border-line">
          {(artifacts.data ?? []).map((a, i) => (
            <li key={a.id} className="rise" style={{ animationDelay: `${i * 25}ms` }}>
              <Link
                to="/store/$artifactId"
                params={{ artifactId: a.id }}
                className="group flex items-center gap-4 py-2.5"
              >
                <span className="w-56 truncate font-mono text-sm text-ink group-hover:text-ember-bright">
                  {a.id}
                </span>
                <KindTag kind={a.kind} />
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-ink-faint">
                  {a.path}
                </span>
                <span className="font-mono text-[11px] text-ink-faint">
                  {a.origin?.startsWith("source:") ? a.origin : ""}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
