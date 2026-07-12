import type { CSSProperties } from "react";
import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { SyncDock } from "./SyncDock";

const nav = [
  { to: "/", label: "Overview" },
  { to: "/store", label: "Store" },
  { to: "/agents", label: "Agents" },
  { to: "/sources", label: "Sources" },
  { to: "/mcp", label: "MCP" },
  { to: "/builder", label: "Builder" },
  { to: "/library", label: "Library" },
  { to: "/export", label: "Export" },
];

function activeIndex(pathname: string): number {
  if (pathname === "/") return 0;
  const i = nav.findIndex((n) => n.to !== "/" && pathname.startsWith(n.to));
  return i;
}

// The AxeForge mark (branding kit logo.svg, inlined so the blades follow
// --af-accent): blades stamp in, the terminal cursor keeps a slow blink.
function Mark() {
  return (
    <svg viewBox="0 0 32 32" className="h-7 w-7" aria-hidden>
      <path d="M16 5 V24.5" stroke="var(--af-text)" strokeWidth="3.5" />
      <path
        fill="var(--af-accent)"
        d="M16 10 L7 4 A8.5 8.5 0 0 0 7 16 Z"
        className="stamp"
        style={{ animationDelay: "120ms" }}
      />
      <path
        fill="var(--af-accent)"
        d="M16 10 L25 4 A8.5 8.5 0 0 1 25 16 Z"
        className="stamp"
        style={{ animationDelay: "240ms" }}
      />
      <rect x="14.25" y="26.5" width="3.5" height="3.5" fill="var(--af-accent)" className="ember-pulse" />
    </svg>
  );
}

export function Layout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const idx = activeIndex(pathname);

  return (
    <div className="technical-grid flex min-h-screen flex-col">
      {/* Family topbar: edge-to-edge, sticky, one hairline — see branding kit. */}
      <header className="sticky top-0 z-40 min-h-[68px] border-b border-line bg-surface/90 py-[14px] backdrop-blur">
        <div className="flex w-full flex-wrap items-center gap-x-8 gap-y-2 px-6">
          <Link to="/" className="flex select-none items-center gap-3">
            <Mark />
            <span>
              <span className="block text-xl font-bold leading-none tracking-tight text-ink">
                load<span className="text-ember">out</span>
              </span>
              <span className="mt-1 block font-mono text-[10px] tracking-[0.18em] text-ink-faint">
                AGENTIC GEAR KIT
              </span>
            </span>
          </Link>

          {/* The rack, laid flat: numbered slots, accent marker slides in under the active one. */}
          <nav className="no-scrollbar flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
            {nav.map((item, i) => {
              const on = i === idx;
              return (
                <Link
                  key={item.to}
                  to={item.to}
                  className={`group relative flex items-center gap-2 whitespace-nowrap px-3 py-2 text-sm transition-colors ${
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
                  <span
                    aria-hidden
                    className={`absolute inset-x-3 bottom-0 h-[2px] origin-left bg-ember transition-transform duration-300 ${
                      on ? "scale-x-100" : "scale-x-0"
                    }`}
                    style={{ transitionTimingFunction: "var(--ease-out-expo)" }}
                  />
                </Link>
              );
            })}
          </nav>

          <p className="hidden font-mono text-[11px] leading-tight text-ink-faint lg:block">
            nothing is written
            <br />
            without asking.
          </p>
        </div>
      </header>

      <main className="mx-auto w-full max-w-6xl flex-1 px-6 pb-40 pt-10">
        <div key={pathname} className="page">
          <Outlet />
        </div>
      </main>
      <SyncDock />
    </div>
  );
}

export function PageHeader({ crate, title, sub }: { crate: string; title: string; sub?: string }) {
  return (
    <header className="mb-10">
      <p className="crate">{crate}</p>
      <h1 className="wipe mt-2 text-[1.9rem] font-bold leading-tight tracking-tight text-ink">
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
