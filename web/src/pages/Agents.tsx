import { useState } from "react";
import type { CSSProperties } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api";
import { PageHeader } from "../components/Layout";
import { StateBadge, stateOf, KindGlyph } from "../components/StateBadge";

// The drift matrix as an actual matrix: artifacts down, agents across,
// every cell a state marker. Click a drifted cell to open its diff.
export function AgentsPage() {
  const status = useQuery({ queryKey: ["status"], queryFn: api.status });
  const [diffFor, setDiffFor] = useState<{ artifact: string; agent: string } | null>(null);
  const diff = useQuery({
    queryKey: ["diff", diffFor],
    queryFn: () => api.diff(diffFor!),
    enabled: diffFor !== null,
  });

  const rows = status.data?.rows ?? [];
  const skips = status.data?.skips ?? [];

  const agents = [...new Set(rows.map((r) => r.agent))];
  const artifacts: { id: string; kind: string; cells: Record<string, string> }[] = [];
  {
    const byId = new Map<string, { id: string; kind: string; cells: Record<string, string> }>();
    for (const r of rows) {
      let a = byId.get(r.artifact);
      if (!a) {
        a = { id: r.artifact, kind: r.kind, cells: {} };
        byId.set(r.artifact, a);
        artifacts.push(a);
      }
      a.cells[r.agent] = r.state;
    }
  }
  const presentStates = [...new Set(rows.map((r) => r.state))];

  const cols = `minmax(200px, 300px) repeat(${Math.max(agents.length, 1)}, minmax(104px, 148px))`;

  return (
    <div>
      <PageHeader
        crate="armory / agents"
        title="Drift matrix"
        sub="Every artifact × every installed agent. Select a drifted cell to see the exact diff before anything moves."
      />

      {status.isLoading ? (
        <p className="text-sm text-ink-faint">Loading…</p>
      ) : rows.length === 0 ? (
        <p className="max-w-[65ch] text-sm text-ink-dim">
          No rows — either the store is empty or no supported agent is installed.
        </p>
      ) : (
        <div className="overflow-x-auto">
          <div className="min-w-fit">
            {/* column heads */}
            <div
              className="grid items-end gap-x-2 border-b border-line-strong pb-2"
              style={{ gridTemplateColumns: cols }}
            >
              <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">
                artifact ▸ agent
              </span>
              {agents.map((a) => (
                <span key={a} className="text-center font-mono text-xs text-ink-dim">
                  {a}
                </span>
              ))}
            </div>

            {artifacts.map((art, i) => (
              <div
                key={art.id}
                className="wipe grid items-center gap-x-2 border-b border-line"
                style={{ gridTemplateColumns: cols, "--i": Math.min(i, 12) } as CSSProperties}
              >
                <span className="flex min-w-0 items-center gap-2.5 py-2 pr-3">
                  <KindGlyph kind={art.kind} className="flex-none text-ink-faint" />
                  <span className="truncate font-mono text-sm text-ink">{art.id}</span>
                </span>
                {agents.map((agent) => {
                  const state = art.cells[agent] ?? "n/a";
                  const s = stateOf(state);
                  const clickable = state !== "in-sync" && state !== "n/a";
                  const isOpen = diffFor?.artifact === art.id && diffFor?.agent === agent;
                  return (
                    <button
                      key={agent}
                      onClick={() =>
                        clickable
                          ? setDiffFor(isOpen ? null : { artifact: art.id, agent })
                          : undefined
                      }
                      disabled={!clickable}
                      aria-label={`${art.id} on ${agent}: ${state}`}
                      title={`${state}${clickable ? " — view diff" : ""}`}
                      data-active={isOpen}
                      className={`brk mx-auto my-1 flex h-8 w-full max-w-[9rem] items-center justify-center font-mono text-sm transition-colors ${
                        s.color
                      } ${clickable ? "cursor-pointer hover:bg-surface" : ""} ${
                        isOpen ? "bg-surface" : ""
                      }`}
                    >
                      <span aria-hidden>{s.mark}</span>
                    </button>
                  );
                })}
              </div>
            ))}

            {/* legend */}
            <div className="mt-4 flex flex-wrap gap-x-7 gap-y-2">
              {presentStates.map((s) => (
                <StateBadge key={s} state={s} />
              ))}
            </div>
          </div>
        </div>
      )}

      <div className="expander" data-open={diffFor !== null}>
        <div>
          {diffFor ? (
            <section className="mt-8 overflow-hidden rounded-2xl border border-line bg-surface/40">
              <div className="flex items-baseline justify-between border-b border-line px-4 py-2.5">
                <h2 className="font-display text-sm font-semibold text-ink">
                  {diffFor.artifact} <span className="text-ink-faint">→</span> {diffFor.agent}
                </h2>
                <button
                  onClick={() => setDiffFor(null)}
                  className="text-xs text-ink-faint transition-colors hover:text-ink"
                >
                  close
                </button>
              </div>
              <div className="px-4 py-3">
                {diff.isLoading ? (
                  <p className="text-sm text-ink-faint">Computing diff…</p>
                ) : (diff.data ?? []).length === 0 ? (
                  <p className="text-sm text-ink-dim">
                    Nothing to change — the drift is local-only (the agent file was edited; syncing
                    would not overwrite it unless the store changes too).
                  </p>
                ) : (
                  (diff.data ?? []).map((d) => (
                    <div key={d.relPath} className="mb-3 last:mb-0">
                      <p className="font-mono text-xs text-ink-faint">{d.relPath}</p>
                      <pre className="mt-1.5 overflow-x-auto rounded-lg border border-line bg-bg-deep/60 p-4 font-mono text-xs leading-relaxed">
                        {d.diff.split("\n").map((line, i) => (
                          <span
                            key={i}
                            className={
                              line.startsWith("+")
                                ? "block text-sync"
                                : line.startsWith("-")
                                  ? "block text-danger"
                                  : "block text-ink-faint"
                            }
                          >
                            {line || " "}
                          </span>
                        ))}
                      </pre>
                    </div>
                  ))
                )}
              </div>
            </section>
          ) : null}
        </div>
      </div>

      {skips.length > 0 ? (
        <p className="mt-6 font-mono text-xs text-ink-faint">
          skipped: {skips.map((s) => `${s.agent} (${s.reason})`).join(" · ")}
        </p>
      ) : null}
    </div>
  );
}
