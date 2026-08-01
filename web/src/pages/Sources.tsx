import { useState } from "react";
import type { CSSProperties } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type SourceCheck, type SourceUpdatePlanResponse } from "../api";
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
        sub="Skill repos pinned in sources.lock.yaml — adding clones and imports once; checking is ls-remote only; updates are two-phase (plan, review the per-skill classification, apply) and never discard your local edits without an explicit opt-in."
      />

      <AddSourceForm onAdded={() => void queryClient.invalidateQueries({ queryKey: ["sources"] })} />

      {sources.isLoading ? (
        <p className="mt-10 text-sm text-ink-faint">Loading…</p>
      ) : (sources.data ?? []).length === 0 ? (
        <p className="mt-10 max-w-[65ch] text-sm text-ink-dim">
          No sources registered yet — add one above, or track a skills repo from the CLI with{" "}
          <code className="font-mono text-xs text-ink">loadout source add &lt;url&gt;</code>. Pins
          land in the store so your whole team resolves the same versions.
        </p>
      ) : (
        <>
          <div className="mb-4 mt-10 flex items-center gap-3">
            <button onClick={() => check.mutate()} disabled={check.isPending} className="btn-ghost">
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
                  className="wipe flex items-center gap-4 py-3"
                  style={{ "--i": i } as CSSProperties}
                >
                  <span className="w-40 truncate font-mono text-sm text-ink">{s.name}</span>
                  <span className="min-w-0 flex-1 truncate font-mono text-xs text-ink-faint">
                    {s.url}
                  </span>
                  <span className="font-mono text-xs text-ink-dim">
                    {s.ref || "default"} <span className="text-ink-faint">@</span>{" "}
                    {s.commit.slice(0, 10)}
                  </span>
                  <span className="w-44 text-right font-mono text-xs">
                    {!r ? (
                      <span className="text-ink-faint">unchecked</span>
                    ) : r.error ? (
                      <span className="stamp inline-block text-danger" title={r.error}>
                        ✕ error
                      </span>
                    ) : r.hasUpdate ? (
                      <span className="stamp inline-block text-drift">◨ update available</span>
                    ) : (
                      <span className="stamp inline-block text-sync">■ up-to-date</span>
                    )}
                  </span>
                </li>
              );
            })}
          </ul>

          <UpdatePlanner
            hasUpdates={(check.data ?? []).some((r) => r.hasUpdate)}
            onApplied={() => void queryClient.invalidateQueries({ queryKey: ["sources"] })}
          />
        </>
      )}
    </div>
  );
}

// UpdatePlanner is the web counterpart of `loadout update`: plan first, show
// the three-way classification with the CLI's vocabulary, and only apply
// after review. Discarding locally edited skills is a separate, explicit
// checkbox naming exactly what would be lost.
function UpdatePlanner({ hasUpdates, onApplied }: { hasUpdates: boolean; onApplied: () => void }) {
  const [planData, setPlanData] = useState<SourceUpdatePlanResponse | null>(null);
  const [discard, setDiscard] = useState(false);

  const plan = useMutation({
    mutationFn: api.sourcesUpdatePlan,
    onSuccess: (d) => {
      setPlanData(d);
      setDiscard(false);
    },
  });
  const apply = useMutation({
    mutationFn: () => api.sourcesUpdateApply(planData?.token ?? "", discard),
    onSuccess: () => {
      setPlanData(null);
      onApplied();
    },
  });

  const conflictNames = (planData?.plans ?? []).flatMap((p) => p.conflicts);
  const writes = (planData?.plans ?? []).some(
    (p) => p.changed.length > 0 || p.added.length > 0 || p.removed.length > 0 || !p.upToDate,
  );

  return (
    <div className="mt-6">
      <div className="flex items-center gap-3">
        <button onClick={() => plan.mutate()} disabled={plan.isPending} className="btn-ember">
          {plan.isPending ? "Planning…" : "Plan updates"}
        </button>
        {hasUpdates && !planData ? (
          <span className="text-xs text-ink-faint">
            fetches each source and classifies every skill before anything moves
          </span>
        ) : null}
      </div>
      {plan.isError ? <p className="stamp mt-2 text-xs text-danger">{String(plan.error)}</p> : null}

      {planData ? (
        <div className="rise mt-4 rounded-lg border border-line p-5">
          {planData.plans.map((p) => (
            <div key={p.source} className="mb-4 last:mb-0">
              <p className="font-mono text-sm text-ink">
                {p.source}{" "}
                <span className="text-ink-faint">
                  {p.upToDate
                    ? `pin up-to-date @ ${p.from.slice(0, 10)}`
                    : `${p.from.slice(0, 10)} → ${p.to.slice(0, 10)}`}
                </span>
              </p>
              <ul className="mt-2 space-y-1 font-mono text-xs">
                {p.changed.map((n) => (
                  <li key={`c-${n}`} className="text-sync">
                    ~ update {n}{" "}
                    {p.legacyBaseline.includes(n) ? "(no baseline — edits undetectable)" : "(upstream changed)"}
                  </li>
                ))}
                {p.added.map((n) => (
                  <li key={`a-${n}`} className="text-sync">
                    + new skill {n}
                  </li>
                ))}
                {p.locallyEdited.map((n) => (
                  <li key={`e-${n}`} className="text-drift">
                    ! {n} locally edited — left alone
                  </li>
                ))}
                {p.conflicts.map((n) => (
                  <li key={`x-${n}`} className="text-danger">
                    !! {n} — edited locally AND changed upstream
                  </li>
                ))}
                {p.ownedElsewhere.map((n) => (
                  <li key={`o-${n}`} className="text-ink-faint">
                    . {n} exists locally with a different origin — not touched
                  </li>
                ))}
                {p.removed.map((n) => (
                  <li key={`r-${n}`} className="text-ink-dim">
                    - {n} no longer upstream (kept, relabeled local)
                  </li>
                ))}
                {p.changed.length +
                  p.added.length +
                  p.locallyEdited.length +
                  p.conflicts.length +
                  p.ownedElsewhere.length +
                  p.removed.length ===
                0 ? (
                  <li className="text-ink-faint">nothing to change</li>
                ) : null}
              </ul>
            </div>
          ))}

          {conflictNames.length > 0 ? (
            <label className="mt-3 flex items-start gap-2 text-xs text-danger">
              <input
                type="checkbox"
                checked={discard}
                onChange={(e) => setDiscard(e.target.checked)}
                className="mt-0.5"
              />
              <span>
                Discard my local edits to{" "}
                <span className="font-mono">{conflictNames.join(", ")}</span> and take upstream's
                version — this cannot be undone from here (a store snapshot is taken first;{" "}
                <code className="font-mono">loadout restore</code> can bring them back).
              </span>
            </label>
          ) : null}

          <div className="mt-4 flex items-center gap-3">
            <button
              onClick={() => apply.mutate()}
              disabled={apply.isPending || (!writes && !(discard && conflictNames.length > 0))}
              className="btn-ember"
            >
              {apply.isPending
                ? "Applying…"
                : discard && conflictNames.length > 0
                  ? "Apply + discard local edits"
                  : "Apply updates"}
            </button>
            <button onClick={() => setPlanData(null)} className="btn-ghost">
              Cancel
            </button>
            {conflictNames.length > 0 && !discard ? (
              <span className="text-xs text-ink-faint">conflicted skills will be skipped</span>
            ) : null}
          </div>
          {apply.isError ? (
            <p className="stamp mt-2 text-xs text-danger">{String(apply.error)}</p>
          ) : null}
        </div>
      ) : null}

      {apply.isSuccess && !planData ? (
        <p className="stamp mt-3 text-xs text-sync">
          ■ store updated — run <code className="font-mono">loadout sync</code> (or the Sync dock) to
          roll changes out to your agents.
        </p>
      ) : null}
    </div>
  );
}

function AddSourceForm({ onAdded }: { onAdded: () => void }) {
  const [url, setUrl] = useState("");
  const [name, setName] = useState("");
  const [ref, setRef] = useState("");
  const [subdir, setSubdir] = useState("");
  const [more, setMore] = useState(false);

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
    <div className="rise brk rounded-lg border border-line p-5" data-active="false">
      <h2 className="font-display text-sm font-semibold uppercase tracking-wider text-ink">
        Add a source
      </h2>

      <div className="mt-3.5 flex flex-wrap items-center gap-3">
        <input
          type="text"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && url.trim() !== "" && !add.isPending) add.mutate();
          }}
          placeholder="git@github.com:team/skills.git"
          aria-label="Repository URL"
          className="field w-full max-w-md flex-1"
        />
        <button
          onClick={() => add.mutate()}
          disabled={add.isPending || url.trim() === ""}
          className="btn-ember"
        >
          {add.isPending ? "Cloning…" : "Add source"}
        </button>
        <button
          onClick={() => setMore((m) => !m)}
          aria-expanded={more}
          className="font-mono text-xs text-ink-faint transition-colors hover:text-ink"
        >
          {more ? "− options" : "+ options"}
        </button>
      </div>

      <div className="expander" data-open={more}>
        <div>
          <div className="flex flex-wrap items-end gap-3 pt-4">
            <Field label="Name" value={name} onChange={setName} placeholder="repo basename" />
            <Field label="Ref" value={ref} onChange={setRef} placeholder="default branch" />
            <Field label="Subdir" value={subdir} onChange={setSubdir} placeholder="whole repo" />
          </div>
        </div>
      </div>

      <p className="mt-3 text-xs text-ink-faint">
        Clones the repo, imports every skill it finds, and pins the commit — same as{" "}
        <code className="font-mono">loadout source add</code>. Private repos use your existing git
        credentials; nothing is stored.
      </p>
      {add.isError ? (
        <p className="stamp mt-2 text-xs text-danger">{String(add.error)}</p>
      ) : add.isSuccess ? (
        <p className="stamp mt-2 text-xs text-sync">
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
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
}) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">
        {label} <span className="normal-case tracking-normal">(optional)</span>
      </span>
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="field w-44"
      />
    </label>
  );
}
