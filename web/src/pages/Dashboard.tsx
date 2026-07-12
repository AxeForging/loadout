import type { CSSProperties } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api } from "../api";
import { BundleImport } from "../components/BundleImport";
import { PageHeader } from "../components/Layout";
import { StateBadge, stateOf, KindGlyph } from "../components/StateBadge";
import { Num } from "../components/motion";

const stateOrder = ["conflict", "drifted-local", "drifted-store", "missing", "in-sync"];

const segColor: Record<string, string> = {
  "in-sync": "var(--color-sync-dim)",
  missing: "var(--color-warn-dim)",
  "drifted-local": "var(--color-warn-dim)",
  "drifted-store": "var(--color-drift-dim)",
  conflict: "var(--color-danger-dim)",
};
const segEdge: Record<string, string> = {
  "in-sync": "var(--color-sync)",
  missing: "var(--color-warn)",
  "drifted-local": "var(--color-warn)",
  "drifted-store": "var(--color-drift)",
  conflict: "var(--color-danger)",
};

export function Dashboard() {
  const overview = useQuery({ queryKey: ["overview"], queryFn: api.overview });

  if (overview.isLoading) return <p className="text-sm text-ink-faint">Loading…</p>;
  if (overview.isError)
    return <p className="text-sm text-danger">Cannot reach loadout: {String(overview.error)}</p>;

  const data = overview.data!;
  const installed = data.agents.filter((a) => a.installed);
  const present = stateOrder.filter((s) => (data.states[s] ?? 0) > 0);
  const total = present.reduce((n, s) => n + (data.states[s] ?? 0), 0);

  return (
    <div>
      <PageHeader
        crate="armory / overview"
        title={data.store.name}
        sub={`${data.store.artifacts} artifact${data.store.artifacts === 1 ? "" : "s"} in the store · ${installed.length} of ${data.agents.length} agents installed on this machine`}
      />

      {data.store.artifacts === 0 ? (
        <div className="rise brk max-w-[65ch] rounded-lg border border-line p-6" data-active="true">
          <div className="flex items-center gap-2.5 text-ink-faint">
            {["skill", "instruction", "command", "hook", "settings", "mcp"].map((k) => (
              <KindGlyph key={k} kind={k} className="h-3.5 w-3.5" />
            ))}
          </div>
          <p className="mt-4 font-display text-lg font-medium text-ink">The rack is empty.</p>
          <p className="mt-2 text-sm leading-relaxed text-ink-dim">
            Fill it with the{" "}
            <Link to="/builder" className="text-ember transition-colors hover:text-ember-bright">
              Builder
            </Link>{" "}
            — it interviews you about your stack, conventions and testing strategy, then generates
            skills, instructions and commands for every agent. Or scaffold by hand with{" "}
            <code className="font-mono text-xs text-ink">loadout new</code>.
          </p>
        </div>
      ) : (
        <section>
          <h2 className="crate">readiness board</h2>

          {/* Segmented readiness bar: one band per sync state, in severity order. */}
          {total > 0 ? (
            <>
              <div className="mt-4 flex h-2.5 gap-[3px]" role="img" aria-label="sync state distribution">
                {present.map((s, i) => (
                  <div
                    key={s}
                    className="grow-x relative min-w-[6px]"
                    style={
                      {
                        flexGrow: data.states[s],
                        "--i": i,
                        background: segColor[s],
                      } as CSSProperties
                    }
                  >
                    <span
                      aria-hidden
                      className="absolute inset-y-0 left-0 w-[3px]"
                      style={{ background: segEdge[s] }}
                    />
                  </div>
                ))}
              </div>
              <div className="mt-4 flex flex-wrap gap-x-10 gap-y-3">
                {present.map((s, i) => (
                  <span
                    key={s}
                    className="rise flex items-baseline gap-2.5"
                    style={{ "--i": i + 2 } as CSSProperties}
                  >
                    <Num
                      value={data.states[s]}
                      className={`font-display text-[1.7rem] font-semibold leading-none ${stateOf(s).color}`}
                    />
                    <StateBadge state={s} />
                  </span>
                ))}
              </div>
            </>
          ) : (
            <p className="mt-3 text-sm text-ink-dim">No installed agents to compare against yet.</p>
          )}
        </section>
      )}

      <section className="mt-14">
        <h2 className="crate">agents on this machine</h2>
        <ul className="mt-4 divide-y divide-line border-y border-line">
          {data.agents.map((agent, i) => (
            <li
              key={agent.id}
              className="rise flex items-center gap-4 py-3"
              style={{ "--i": i } as CSSProperties}
            >
              <span
                aria-hidden
                className={`h-2 w-2 flex-none ${
                  agent.installed ? "bg-sync" : "border border-line-strong"
                }`}
              />
              <span className="w-36 font-mono text-sm text-ink">{agent.id}</span>
              <span className="w-32 text-sm text-ink-dim">{agent.name}</span>
              {agent.installed ? (
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-ink-faint">
                  {agent.home}
                </span>
              ) : (
                <span className="flex-1 text-xs text-ink-faint">not installed</span>
              )}
            </li>
          ))}
        </ul>
      </section>

      <section className="mt-14 max-w-[65ch]">
        <h2 className="crate">share</h2>
        <p className="mt-4 text-sm leading-relaxed text-ink-dim">
          Export the whole store as one bundle a teammate can import on any machine. They run{" "}
          <code className="font-mono text-xs text-ink">loadout bundle import</code> and then sync.
        </p>
        <a
          href="/api/bundle/export"
          className="btn-ghost mt-4 inline-flex items-center gap-2"
          download
        >
          <span aria-hidden className="font-mono text-ember">
            ↓
          </span>
          {data.store.name}.loadout.tar.gz
        </a>
        <Link to="/export" className="mt-2 block text-xs text-ink-faint transition-colors hover:text-ink">
          or pick exactly what to share →
        </Link>

        <BundleImport />
      </section>
    </div>
  );
}
