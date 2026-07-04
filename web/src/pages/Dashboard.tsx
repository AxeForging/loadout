import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api } from "../api";
import { PageHeader } from "../components/Layout";
import { StateBadge } from "../components/StateBadge";

const stateOrder = ["conflict", "drifted-local", "drifted-store", "missing", "in-sync"];

export function Dashboard() {
  const overview = useQuery({ queryKey: ["overview"], queryFn: api.overview });

  if (overview.isLoading) return <p className="text-sm text-ink-faint">Loading…</p>;
  if (overview.isError)
    return <p className="text-sm text-danger">Cannot reach loadout: {String(overview.error)}</p>;

  const data = overview.data!;
  const installed = data.agents.filter((a) => a.installed);

  return (
    <div>
      <PageHeader
        crate="armory / overview"
        title={data.store.name}
        sub={`${data.store.artifacts} artifact${data.store.artifacts === 1 ? "" : "s"} in the store · ${installed.length} of ${data.agents.length} agents installed on this machine`}
      />

      {data.store.artifacts === 0 ? (
        <div className="rise max-w-[65ch] border border-line p-6">
          <p className="font-display text-lg font-medium text-ink">The rack is empty.</p>
          <p className="mt-2 text-sm leading-relaxed text-ink-dim">
            Fill it with the <Link to="/builder" className="text-ember hover:text-ember-bright">Builder</Link> —
            it interviews you about your stack, conventions and testing strategy, then generates
            skills, instructions and commands for every agent. Or scaffold by hand with{" "}
            <code className="font-mono text-xs text-ink">loadout new</code>.
          </p>
        </div>
      ) : (
        <section className="rise">
          <h2 className="font-mono text-[11px] uppercase tracking-[0.2em] text-ink-faint">
            sync state
          </h2>
          <div className="mt-3 flex flex-wrap gap-x-8 gap-y-2">
            {stateOrder
              .filter((s) => (data.states[s] ?? 0) > 0)
              .map((s) => (
                <span key={s} className="flex items-baseline gap-2">
                  <span className="font-display text-2xl font-semibold text-ink">
                    {data.states[s]}
                  </span>
                  <StateBadge state={s} />
                </span>
              ))}
          </div>
        </section>
      )}

      <section className="mt-10">
        <h2 className="font-mono text-[11px] uppercase tracking-[0.2em] text-ink-faint">
          agents on this machine
        </h2>
        <ul className="mt-3 divide-y divide-line border-y border-line">
          {data.agents.map((agent, i) => (
            <li
              key={agent.id}
              className="rise flex items-center gap-4 py-2.5"
              style={{ animationDelay: `${i * 40}ms` }}
            >
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

      <section className="mt-10 max-w-[65ch]">
        <h2 className="font-mono text-[11px] uppercase tracking-[0.2em] text-ink-faint">
          share
        </h2>
        <p className="mt-3 text-sm leading-relaxed text-ink-dim">
          Export the whole store as one bundle a teammate can import on any machine —{" "}
          <a href="/api/bundle/export" className="text-ember hover:text-ember-bright">
            download {data.store.name}.loadout.tar.gz
          </a>
          . They run <code className="font-mono text-xs text-ink">loadout bundle import</code>{" "}
          and then sync.
        </p>
      </section>
    </div>
  );
}
