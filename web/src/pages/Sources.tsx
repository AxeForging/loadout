import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type SourceCheck } from "../api";
import { PageHeader } from "../components/Layout";

export function SourcesPage() {
  const queryClient = useQueryClient();
  const sources = useQuery({ queryKey: ["sources"], queryFn: api.sources });
  const check = useMutation({ mutationFn: api.sourcesCheck });

  const results: Record<string, SourceCheck> = {};
  for (const r of check.data ?? []) results[r.name] = r;

  return (
    <div>
      <PageHeader
        crate="armory / sources"
        title="External sources"
        sub="Skill repos pinned in sources.lock.yaml — adding clones and imports once; checking is ls-remote only; applying updates stays in the CLI (loadout update) where the diff is reviewed."
      />

      <AddSourceForm onAdded={() => void queryClient.invalidateQueries({ queryKey: ["sources"] })} />

      {sources.isLoading ? (
        <p className="mt-8 text-sm text-ink-faint">Loading…</p>
      ) : (sources.data ?? []).length === 0 ? (
        <p className="mt-8 max-w-[65ch] text-sm text-ink-dim">
          No sources registered yet — add one above, or track a skills repo from the CLI with{" "}
          <code className="font-mono text-xs text-ink">loadout source add &lt;url&gt;</code>. Pins
          land in the store so your whole team resolves the same versions.
        </p>
      ) : (
        <>
          <div className="mb-4 mt-8 flex items-center gap-3">
            <button
              onClick={() => check.mutate()}
              disabled={check.isPending}
              className="rounded-sm border border-line-strong px-3 py-1.5 font-display text-sm text-ink-dim transition-colors hover:border-ember-dim hover:text-ink disabled:opacity-50"
            >
              {check.isPending ? "Checking remotes…" : "Check remotes"}
            </button>
            <span className="text-xs text-ink-faint">read-only — nothing is fetched or applied</span>
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

function AddSourceForm({ onAdded }: { onAdded: () => void }) {
  const [url, setUrl] = useState("");
  const [name, setName] = useState("");
  const [ref, setRef] = useState("");
  const [subdir, setSubdir] = useState("");

  const add = useMutation({
    mutationFn: () =>
      api.sourcesAdd({
        url: url.trim(),
        name: name.trim() || undefined,
        ref: ref.trim() || undefined,
        subdir: subdir.trim() || undefined,
      }),
    onSuccess: () => {
      setUrl("");
      setName("");
      setRef("");
      setSubdir("");
      onAdded();
    },
  });

  return (
    <div className="rise border border-line p-5">
      <h2 className="font-display text-sm font-semibold text-ink">Add a source</h2>
      <div className="mt-3 flex flex-wrap items-end gap-3">
        <Field label="Repository URL" value={url} onChange={setUrl} placeholder="git@github.com:team/skills.git" wide />
        <Field label="Name (optional)" value={name} onChange={setName} placeholder="repo basename" />
        <Field label="Ref (optional)" value={ref} onChange={setRef} placeholder="default branch" />
        <Field label="Subdir (optional)" value={subdir} onChange={setSubdir} placeholder="" />
        <button
          onClick={() => add.mutate()}
          disabled={add.isPending || url.trim() === ""}
          className="rounded-sm bg-ember px-4 py-1.5 font-display text-sm font-semibold text-bg transition-colors hover:bg-ember-bright disabled:opacity-50"
        >
          {add.isPending ? "Cloning…" : "Add source"}
        </button>
      </div>
      <p className="mt-2.5 text-xs text-ink-faint">
        Clones the repo, imports every skill it finds, and pins the commit — same as{" "}
        <code className="font-mono">loadout source add</code>. Private repos use your existing git
        credentials; nothing is stored.
      </p>
      {add.isError ? (
        <p className="mt-2 text-xs text-danger">{String(add.error)}</p>
      ) : add.isSuccess ? (
        <p className="mt-2 text-xs text-sync">
          ■ pinned {add.data.ref.name} @ {add.data.ref.commit.slice(0, 10)} — imported{" "}
          {(add.data.added ?? []).length} skill{(add.data.added ?? []).length === 1 ? "" : "s"}
          {(add.data.skipped ?? []).length > 0
            ? `, skipped ${(add.data.skipped ?? []).length} (already exist)`
            : ""}
          . Run <code className="font-mono">loadout sync</code> to install.
        </p>
      ) : null}
    </div>
  );
}

function Field({
  label,
  value,
  onChange,
  placeholder,
  wide,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  wide?: boolean;
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="font-mono text-[11px] uppercase tracking-[0.15em] text-ink-faint">
        {label}
      </span>
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={`${wide ? "w-72" : "w-40"} rounded-sm border border-line bg-surface px-3 py-1.5 font-mono text-sm text-ink outline-none placeholder:text-ink-faint focus:border-ember-dim`}
      />
    </label>
  );
}
