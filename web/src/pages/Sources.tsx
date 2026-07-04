import { useQuery, useMutation } from "@tanstack/react-query";
import { api, type SourceCheck } from "../api";
import { PageHeader } from "../components/Layout";

export function SourcesPage() {
  const sources = useQuery({ queryKey: ["sources"], queryFn: api.sources });
  const check = useMutation({ mutationFn: api.sourcesCheck });

  const results: Record<string, SourceCheck> = {};
  for (const r of check.data ?? []) results[r.name] = r;

  return (
    <div>
      <PageHeader
        crate="armory / sources"
        title="External sources"
        sub="Skill repos pinned in sources.lock.yaml — checking is ls-remote only; applying updates stays in the CLI (loadout update) where the diff is reviewed."
      />

      {sources.isLoading ? (
        <p className="text-sm text-ink-faint">Loading…</p>
      ) : (sources.data ?? []).length === 0 ? (
        <p className="max-w-[65ch] text-sm text-ink-dim">
          No sources registered. Track a skills repo with{" "}
          <code className="font-mono text-xs text-ink">loadout source add &lt;url&gt;</code> —
          pins land in the store so your whole team resolves the same versions.
        </p>
      ) : (
        <>
          <div className="mb-4">
            <button
              onClick={() => check.mutate()}
              disabled={check.isPending}
              className="rounded-sm border border-line-strong px-3 py-1.5 font-display text-sm text-ink-dim transition-colors hover:border-ember-dim hover:text-ink disabled:opacity-50"
            >
              {check.isPending ? "Checking remotes…" : "Check remotes"}
            </button>
            <span className="ml-3 text-xs text-ink-faint">
              read-only — nothing is fetched or applied
            </span>
          </div>

          <ul className="divide-y divide-line border-y border-line">
            {(sources.data ?? []).map((s, i) => {
              const r = results[s.name];
              return (
                <li
                  key={s.name}
                  className="rise flex items-center gap-4 py-2.5"
                  style={{ animationDelay: `${i * 30}ms` }}
                >
                  <span className="w-40 truncate font-mono text-sm text-ink">{s.name}</span>
                  <span className="min-w-0 flex-1 truncate font-mono text-xs text-ink-faint">
                    {s.url}
                  </span>
                  <span className="font-mono text-xs text-ink-dim">
                    {s.ref || "default"} @ {s.commit.slice(0, 10)}
                  </span>
                  <span className="w-44 text-right font-mono text-xs">
                    {!r ? (
                      <span className="text-ink-faint">unchecked</span>
                    ) : r.error ? (
                      <span className="text-danger" title={r.error}>
                        ■ error
                      </span>
                    ) : r.hasUpdate ? (
                      <span className="text-drift">■ update available</span>
                    ) : (
                      <span className="text-sync">■ up-to-date</span>
                    )}
                  </span>
                </li>
              );
            })}
          </ul>

          {(check.data ?? []).some((r) => r.hasUpdate) ? (
            <p className="mt-4 text-sm text-ink-dim">
              Updates available — run{" "}
              <code className="font-mono text-xs text-ember">loadout update</code> to review the
              changelog and per-skill diff before anything moves.
            </p>
          ) : null}
        </>
      )}
    </div>
  );
}
