import { Link, Outlet } from "@tanstack/react-router";
import { SyncDock } from "./SyncDock";

const nav = [
  { to: "/", label: "Overview", exact: true },
  { to: "/store", label: "Store" },
  { to: "/agents", label: "Agents" },
  { to: "/sources", label: "Sources" },
  { to: "/builder", label: "Builder" },
];

export function Layout() {
  return (
    <div className="min-h-screen">
      <div className="mx-auto flex max-w-6xl gap-10 px-6 pb-32 pt-8">
        <aside className="w-44 shrink-0">
          <Link to="/" className="block select-none">
            <span className="font-display text-xl font-bold tracking-wide text-ink">
              LOAD<span className="text-ember">OUT</span>
            </span>
            <span className="mt-1 block font-mono text-[11px] text-ink-faint">
              agentic gear kit
            </span>
          </Link>

          <nav className="mt-10 flex flex-col gap-1">
            {nav.map((item) => (
              <Link
                key={item.to}
                to={item.to}
                activeOptions={{ exact: item.exact ?? false }}
                className="rounded-sm px-3 py-1.5 text-sm text-ink-dim transition-colors hover:bg-surface hover:text-ink"
                activeProps={{
                  className:
                    "rounded-sm px-3 py-1.5 text-sm bg-surface text-ink font-medium",
                }}
              >
                {item.label}
              </Link>
            ))}
          </nav>

          <div className="mt-12 border-t border-line pt-4">
            <p className="font-mono text-[11px] leading-relaxed text-ink-faint">
              nothing is written
              <br />
              without asking.
            </p>
          </div>
        </aside>

        <main className="min-w-0 flex-1">
          <Outlet />
        </main>
      </div>
      <SyncDock />
    </div>
  );
}

export function PageHeader({ crate, title, sub }: { crate: string; title: string; sub?: string }) {
  return (
    <header className="rise mb-8">
      <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-ink-faint">{crate}</p>
      <h1 className="mt-1 font-display text-2xl font-semibold text-ink">{title}</h1>
      {sub ? <p className="mt-1.5 max-w-[65ch] text-sm text-ink-dim">{sub}</p> : null}
    </header>
  );
}
