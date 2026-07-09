import type { CSSProperties } from "react";
import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { SyncDock } from "./SyncDock";

const nav = [
  { to: "/", label: "Overview" },
  { to: "/store", label: "Store" },
  { to: "/agents", label: "Agents" },
  { to: "/sources", label: "Sources" },
  { to: "/builder", label: "Builder" },
  { to: "/library", label: "Library" },
  { to: "/export", label: "Export" },
];

const SLOT_H = 36; // px — keep in sync with the h-9 rows below

function activeIndex(pathname: string): number {
  if (pathname === "/") return 0;
  const i = nav.findIndex((n) => n.to !== "/" && pathname.startsWith(n.to));
  return i;
}

// The four-square loadout mark, stamped in one square at a time.
function Mark() {
  const cells = [
    { x: 1, y: 1, ember: true },
    { x: 12, y: 1, ember: false },
    { x: 1, y: 12, ember: false },
    { x: 12, y: 12, ember: true },
  ];
  return (
    <svg viewBox="0 0 22 22" className="h-6 w-6" aria-hidden>
      {cells.map((c, i) => (
        <rect
          key={i}
          x={c.x}
          y={c.y}
          width="9"
          height="9"
          className="stamp"
          style={{ animationDelay: `${120 + i * 90}ms` }}
          fill={c.ember ? "var(--color-ember)" : "var(--color-line-strong)"}
        />
      ))}
    </svg>
  );
}

export function Layout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const idx = activeIndex(pathname);

  return (
    <div className="min-h-screen">
      <div className="mx-auto flex max-w-6xl gap-14 px-6 pb-40 pt-10">
        <aside className="w-48 shrink-0">
          <div className="sticky top-10">
            <Link to="/" className="flex select-none items-center gap-3">
              <Mark />
              <span>
                <span className="block font-display text-xl font-bold leading-none tracking-wide text-ink">
                  LOAD<span className="text-ember">OUT</span>
                </span>
                <span className="mt-1 block font-mono text-[10px] tracking-[0.18em] text-ink-faint">
                  AGENTIC GEAR KIT
                </span>
              </span>
            </Link>

            {/* The rack: numbered slots on a rail, ember marker slides between them. */}
            <nav className="relative mt-12">
              <span aria-hidden className="absolute inset-y-1 left-0 w-px bg-line" />
              <span
                aria-hidden
                className="absolute left-0 w-[2px] bg-ember transition-transform"
                style={{
                  height: SLOT_H - 12,
                  transform: `translateY(${(idx < 0 ? 0 : idx) * SLOT_H + 6}px)`,
                  opacity: idx < 0 ? 0 : 1,
                  transitionDuration: "450ms",
                  transitionTimingFunction: "var(--ease-out-expo)",
                }}
              />
              <div className="flex flex-col">
                {nav.map((item, i) => {
                  const on = i === idx;
                  return (
                    <Link
                      key={item.to}
                      to={item.to}
                      className={`group flex h-9 items-center gap-3 pl-5 text-sm transition-colors ${
                        on ? "text-ink" : "text-ink-dim hover:text-ink"
                      }`}
                    >
                      <span
                        className={`font-mono text-[10px] tnum transition-colors ${
                          on ? "text-ember" : "text-ink-faint group-hover:text-ink-dim"
                        }`}
                      >
                        {String(i + 1).padStart(2, "0")}
                      </span>
                      <span className={on ? "font-medium" : ""}>{item.label}</span>
                    </Link>
                  );
                })}
              </div>
            </nav>

            <div className="mt-14 border-t border-line pt-4">
              <p className="font-mono text-[11px] leading-relaxed text-ink-faint">
                nothing is written
                <br />
                without asking.
              </p>
            </div>
          </div>
        </aside>

        <main className="min-w-0 flex-1">
          <div key={pathname} className="page">
            <Outlet />
          </div>
        </main>
      </div>
      <SyncDock />
    </div>
  );
}

export function PageHeader({ crate, title, sub }: { crate: string; title: string; sub?: string }) {
  return (
    <header className="mb-10">
      <p className="crate">{crate}</p>
      <h1 className="wipe mt-2 font-display text-[1.9rem] font-semibold leading-tight text-ink">
        {title}
      </h1>
      {sub ? (
        <p className="rise mt-2 max-w-[65ch] text-sm leading-relaxed text-ink-dim" style={{ "--i": 2 } as CSSProperties}>
          {sub}
        </p>
      ) : null}
    </header>
  );
}
