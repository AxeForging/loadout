import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api";
import { PageHeader } from "../components/Layout";
import { StateBadge, KindTag } from "../components/StateBadge";

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

  return (
    <div>
      <PageHeader
        crate="armory / agents"
        title="Drift matrix"
        sub="Every artifact × every installed agent. Click a drifted row to see the exact diff."
      />

      {status.isLoading ? (
        <p className="text-sm text-ink-faint">Loading…</p>
      ) : rows.length === 0 ? (
        <p className="max-w-[65ch] text-sm text-ink-dim">
          No rows — either the store is empty or no supported agent is installed.
        </p>
      ) : (
        <table className="w-full border-y border-line">
          <thead>
            <tr className="text-left font-mono text-[11px] uppercase tracking-wider text-ink-faint">
              <th className="py-2 pr-4 font-medium">artifact</th>
              <th className="py-2 pr-4 font-medium">kind</th>
              <th className="py-2 pr-4 font-medium">agent</th>
              <th className="py-2 font-medium">state</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {rows.map((r, i) => {
              const clickable = r.state !== "in-sync" && r.state !== "n/a";
              const isOpen = diffFor?.artifact === r.artifact && diffFor?.agent === r.agent;
              return (
                <tr
                  key={`${r.artifact}-${r.agent}`}
                  onClick={() =>
                    clickable
                      ? setDiffFor(isOpen ? null : { artifact: r.artifact, agent: r.agent })
                      : undefined
                  }
                  className={`rise ${clickable ? "cursor-pointer hover:bg-surface" : ""} ${isOpen ? "bg-surface" : ""}`}
                  style={{ animationDelay: `${Math.min(i * 20, 400)}ms` }}
                >
                  <td className="py-2 pr-4 font-mono text-sm text-ink">{r.artifact}</td>
                  <td className="py-2 pr-4">
                    <KindTag kind={r.kind} />
                  </td>
                  <td className="py-2 pr-4 text-sm text-ink-dim">{r.agent}</td>
                  <td className="py-2">
                    <StateBadge state={r.state} />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {diffFor ? (
        <section className="rise mt-6">
          <div className="flex items-baseline justify-between">
            <h2 className="font-display text-sm font-semibold text-ink">
              {diffFor.artifact} → {diffFor.agent}
            </h2>
            <button
              onClick={() => setDiffFor(null)}
              className="text-xs text-ink-faint hover:text-ink"
            >
              close
            </button>
          </div>
          {diff.isLoading ? (
            <p className="mt-2 text-sm text-ink-faint">Computing diff…</p>
          ) : (diff.data ?? []).length === 0 ? (
            <p className="mt-2 text-sm text-ink-dim">
              Nothing to change — the drift is local-only (the agent file was edited; syncing
              would not overwrite it unless the store changes too).
            </p>
          ) : (
            (diff.data ?? []).map((d) => (
              <div key={d.relPath} className="mt-3">
                <p className="font-mono text-xs text-ink-faint">{d.relPath}</p>
                <pre className="mt-1 overflow-x-auto border border-line bg-surface p-4 font-mono text-xs leading-relaxed">
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
        </section>
      ) : null}

      {skips.length > 0 ? (
        <p className="mt-6 font-mono text-xs text-ink-faint">
          skipped: {skips.map((s) => `${s.agent} (${s.reason})`).join(" · ")}
        </p>
      ) : null}
    </div>
  );
}
