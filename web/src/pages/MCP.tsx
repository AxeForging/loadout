import { useState } from "react";
import type { CSSProperties } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, type MCPServer } from "../api";
import { PageHeader } from "../components/Layout";
import { parseArgs, parseEnvLines, parseHeaderLines } from "./MCP.helpers";

// MCP server configs as store artifacts: define once here, sync installs the
// definition into each agent's native MCP config (claude-code, gemini-cli,
// cursor today) through the same gated flow as everything else.
export function MCPPage() {
  const queryClient = useQueryClient();
  const servers = useQuery({ queryKey: ["mcp"], queryFn: api.mcpList });

  return (
    <div>
      <PageHeader
        crate="armory / mcp"
        title="MCP servers"
        sub="One canonical definition per server — sync renders it into each agent's native MCP config (claude-code, gemini-cli, cursor). Secrets stay ${VAR} placeholders; literals never enter the store."
      />

      <AddServerForm onAdded={() => void queryClient.invalidateQueries({ queryKey: ["mcp"] })} />

      {servers.isLoading ? (
        <p className="mt-10 text-sm text-ink-faint">Loading…</p>
      ) : (servers.data ?? []).length === 0 ? (
        <p className="mt-10 max-w-[65ch] text-sm text-ink-dim">
          No MCP servers defined yet — add one above, or scaffold from the CLI with{" "}
          <code className="font-mono text-xs text-ink">loadout new mcp &lt;name&gt;</code>. After
          adding, the pending-changes bar offers the gated sync into your agents.
        </p>
      ) : (
        <ul className="mt-10 divide-y divide-line border-y border-line">
          {(servers.data ?? []).map((s, i) => (
            <ServerRow key={s.id} server={s} index={i} />
          ))}
        </ul>
      )}
    </div>
  );
}

function ServerRow({ server, index }: { server: MCPServer; index: number }) {
  const spec = server.spec;
  return (
    <li className="wipe flex items-center gap-4 py-3" style={{ "--i": index } as CSSProperties}>
      <Link
        to="/store/$artifactId"
        params={{ artifactId: server.id }}
        className="w-44 truncate font-mono text-sm text-ink transition-colors hover:text-ember"
      >
        {server.id}
      </Link>
      {server.error ? (
        <span className="min-w-0 flex-1 truncate font-mono text-xs text-danger" title={server.error}>
          ✕ {server.error}
        </span>
      ) : spec ? (
        <>
          <span className="w-14 font-mono text-[11px] text-ink-dim">{spec.transport}</span>
          <span className="min-w-0 flex-1 truncate font-mono text-xs text-ink-faint">
            {spec.transport === "stdio" ? [spec.command, ...(spec.args ?? [])].join(" ") : spec.url}
          </span>
          {Object.keys(spec.env ?? {}).length + Object.keys(spec.headers ?? {}).length > 0 ? (
            <span className="font-mono text-[11px] text-ink-faint">
              {Object.keys(spec.env ?? {}).length + Object.keys(spec.headers ?? {}).length} ${"{"}VAR{"}"}
            </span>
          ) : null}
          <span className="w-16 text-right font-mono text-[11px] text-ink-dim">{spec.scope}</span>
        </>
      ) : null}
      {server.origin && server.origin !== "local" ? (
        <span className="font-mono text-[11px] text-drift">{server.origin}</span>
      ) : null}
    </li>
  );
}

function AddServerForm({ onAdded }: { onAdded: () => void }) {
  const [name, setName] = useState("");
  const [transport, setTransport] = useState<"stdio" | "http" | "sse">("stdio");
  const [command, setCommand] = useState("");
  const [args, setArgs] = useState("");
  const [url, setUrl] = useState("");
  const [env, setEnv] = useState("");
  const [headers, setHeaders] = useState("");
  const [scope, setScope] = useState<"user" | "project">("user");

  const add = useMutation({
    mutationFn: () =>
      api.mcpCreate({
        name: name.trim(),
        transport,
        scope,
        ...(transport === "stdio"
          ? { command: command.trim(), args: parseArgs(args), env: parseEnvLines(env) }
          : { url: url.trim(), headers: parseHeaderLines(headers) }),
      }),
    onSuccess: () => {
      setName("");
      setCommand("");
      setArgs("");
      setUrl("");
      setEnv("");
      setHeaders("");
      onAdded();
    },
  });

  const incomplete =
    name.trim() === "" || (transport === "stdio" ? command.trim() === "" : url.trim() === "");

  return (
    <div className="rise brk rounded-lg border border-line p-5" data-active="false">
      <h2 className="font-display text-sm font-semibold uppercase tracking-wider text-ink">
        Add an MCP server
      </h2>

      <div className="mt-3.5 flex flex-wrap items-end gap-3">
        <Field label="Name" hint="artifact id" value={name} onChange={setName} placeholder="context7" />
        <div className="flex flex-col gap-1.5">
          <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">
            Transport
          </span>
          <div className="flex gap-2">
            {(["stdio", "http", "sse"] as const).map((t) => (
              <button key={t} type="button" onClick={() => setTransport(t)} data-on={transport === t} className="chip font-mono text-xs">
                {t}
              </button>
            ))}
          </div>
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">
            Scope
          </span>
          <div className="flex gap-2">
            {(["user", "project"] as const).map((sc) => (
              <button key={sc} type="button" onClick={() => setScope(sc)} data-on={scope === sc} className="chip font-mono text-xs">
                {sc}
              </button>
            ))}
          </div>
        </div>
      </div>

      {transport === "stdio" ? (
        <div className="mt-3 flex flex-wrap items-end gap-3">
          <Field label="Command" value={command} onChange={setCommand} placeholder="npx" />
          <Field
            label="Args"
            hint="space-separated"
            value={args}
            onChange={setArgs}
            placeholder="-y @upstash/context7-mcp"
            wide
          />
        </div>
      ) : (
        <div className="mt-3">
          <Field label="URL" value={url} onChange={setUrl} placeholder="https://mcp.example.com/v1" wide />
        </div>
      )}

      <div className="mt-3">
        <label className="flex flex-col gap-1.5">
          <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">
            {transport === "stdio" ? "Env" : "Headers"}{" "}
            <span className="normal-case tracking-normal">
              (one per line — secrets as {"${VAR}"} placeholders only)
            </span>
          </span>
          <textarea
            value={transport === "stdio" ? env : headers}
            onChange={(e) => (transport === "stdio" ? setEnv(e.target.value) : setHeaders(e.target.value))}
            placeholder={
              transport === "stdio" ? "API_TOKEN=${CONTEXT7_TOKEN}" : "Authorization: Bearer ${MCP_KEY}"
            }
            rows={2}
            className="field w-full max-w-xl resize-y"
          />
        </label>
      </div>

      <div className="mt-4 flex items-center gap-3">
        <button onClick={() => add.mutate()} disabled={add.isPending || incomplete} className="btn-ember">
          {add.isPending ? "Adding…" : "Add to store"}
        </button>
        <span className="text-xs text-ink-faint">
          writes to the store only — installing into agents stays behind the sync gate
        </span>
      </div>
      {add.isError ? (
        <p className="stamp mt-2 text-xs text-danger">{String(add.error)}</p>
      ) : add.isSuccess ? (
        <p className="stamp mt-2 text-xs text-sync">
          ■ added <span className="font-mono">{add.data.id}</span> — run a sync to install it into
          your agents
        </p>
      ) : null}
    </div>
  );
}

function Field({
  label,
  hint,
  value,
  onChange,
  placeholder,
  wide = false,
}: {
  label: string;
  hint?: string;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  wide?: boolean;
}) {
  return (
    <label className={`flex flex-col gap-1.5 ${wide ? "min-w-0 flex-1" : ""}`}>
      <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">
        {label}
        {hint ? <span className="normal-case tracking-normal"> ({hint})</span> : null}
      </span>
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={`field ${wide ? "w-full" : "w-44"}`}
      />
    </label>
  );
}
