import { useState } from "react";
import type { CSSProperties } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type SyncPlan } from "../api";

// The gated-sync ritual, docked at the bottom of every page: a persistent
// pending indicator → explicit "review" (plan + token) → explicit "apply".
// Staged like a requisition slip: ledger of ops, plan token as the stamp.
export function SyncDock() {
  const queryClient = useQueryClient();
  const [plan, setPlan] = useState<SyncPlan | null>(null);
  const [applied, setApplied] = useState<number | null>(null);
  const [overwrite, setOverwrite] = useState(false);

  const overview = useQuery({ queryKey: ["overview"], queryFn: api.overview });

  const planMutation = useMutation({
    mutationFn: api.syncPlan,
    onSuccess: (p) => {
      setPlan(p);
      setApplied(null);
      setOverwrite(false);
    },
  });
  const applyMutation = useMutation({
    mutationFn: ({ token, overwriteConflicts }: { token: string; overwriteConflicts: boolean }) =>
      api.syncApply(token, overwriteConflicts),
    onSuccess: (res) => {
      setApplied(res.applied);
      setPlan(null);
      void queryClient.invalidateQueries();
    },
  });

  const pending = overview.data?.pending ?? 0;
  const pendingConflicts = overview.data?.conflicts ?? 0;
  if (pending === 0 && pendingConflicts === 0 && !plan && applied === null) return null;

  const conflicts = plan?.conflicts ?? [];
  const applyCount = (plan?.changes.length ?? 0) + (overwrite ? conflicts.length : 0);

  return (
    <div className="fixed inset-x-0 bottom-0 z-10">
      <div className="mx-auto max-w-6xl px-6 pb-5">
        <div className="dock-up rounded-2xl border border-line-strong bg-raised shadow-[0_-12px_48px_rgb(0_0_0/0.6)]">
          {applied !== null && !plan ? (
            <div className="flex items-center justify-between px-5 py-3">
              <p className="stamp text-sm text-sync">
                <span className="mr-2 font-mono text-[10px]">■</span>
                Applied {applied} change{applied === 1 ? "" : "s"}. Agents are up to date.
              </p>
              <button
                onClick={() => setApplied(null)}
                className="text-xs text-ink-faint transition-colors hover:text-ink"
              >
                dismiss
              </button>
            </div>
          ) : plan ? (
            <div className="px-5 py-4">
              <div className="flex items-baseline justify-between">
                <p className="font-display text-sm font-semibold uppercase tracking-wider text-ink">
                  Requisition — {plan.changes.length} change{plan.changes.length === 1 ? "" : "s"}
                </p>
                <p className="font-mono text-[11px] text-ink-faint">
                  plan <span className="text-ember">{plan.token.slice(0, 8)}</span>
                </p>
              </div>
              <ul className="mt-3 max-h-44 space-y-1 overflow-y-auto pr-1">
                {plan.changes.map((c, i) => (
                  <li
                    key={i}
                    className="rise flex items-center gap-3 font-mono text-xs"
                    style={{ "--i": Math.min(i, 10) } as CSSProperties}
                  >
                    <span
                      className={`w-16 whitespace-nowrap ${c.op === "create" ? "text-sync" : "text-warn"}`}
                    >
                      {c.op === "create" ? "+ new" : "~ " + c.op}
                    </span>
                    <span className="text-ink-dim">{c.agent}</span>
                    <span className="min-w-0 flex-1 truncate text-ink">{c.relPath}</span>
                    <span className="text-ink-faint">{c.artifact}</span>
                  </li>
                ))}
                {conflicts.map((c, i) => (
                  <li
                    key={`conflict-${i}`}
                    className="rise flex items-center gap-3 font-mono text-xs"
                    style={{ "--i": Math.min(plan.changes.length + i, 10) } as CSSProperties}
                  >
                    <span className="w-16 whitespace-nowrap text-danger">
                      {overwrite ? "! force" : "! skip"}
                    </span>
                    <span className="text-ink-dim">{c.agent}</span>
                    <span className="min-w-0 flex-1 truncate text-ink">{c.relPath}</span>
                    <span className="text-ink-faint">{c.artifact}</span>
                  </li>
                ))}
              </ul>
              {conflicts.length > 0 ? (
                <label className="mt-3 flex items-start gap-2 border-t border-line pt-3 text-xs text-ink-dim">
                  <input
                    type="checkbox"
                    checked={overwrite}
                    onChange={(e) => setOverwrite(e.target.checked)}
                    className="mt-0.5"
                  />
                  <span>
                    Also overwrite {conflicts.length} conflicting file
                    {conflicts.length === 1 ? "" : "s"} loadout never installed. A snapshot is taken
                    first, so this stays recoverable via <span className="font-mono">loadout restore</span>.
                  </span>
                </label>
              ) : null}
              <div className="mt-4 flex items-center gap-3 border-t border-line pt-3">
                <button
                  onClick={() => applyMutation.mutate({ token: plan.token, overwriteConflicts: overwrite })}
                  disabled={applyMutation.isPending || applyCount === 0}
                  className="btn-ember"
                >
                  {applyMutation.isPending ? "Applying…" : `Apply ${applyCount}`}
                </button>
                <button
                  onClick={() => setPlan(null)}
                  className="px-2 py-1.5 text-sm text-ink-dim transition-colors hover:text-ink"
                >
                  Cancel — write nothing
                </button>
                {applyMutation.isError ? (
                  <span className="text-xs text-danger">{String(applyMutation.error)}</span>
                ) : null}
              </div>
            </div>
          ) : (
            <div className="flex items-center justify-between px-5 py-3">
              <p className="text-sm text-ink-dim">
                <span aria-hidden className="ember-pulse mr-2.5 inline-block h-2 w-2 bg-ember align-baseline" />
                {pending > 0 ? (
                  <>
                    <span className="tnum font-medium text-ink">{pending}</span> pending change
                    {pending === 1 ? "" : "s"} across your agents
                  </>
                ) : (
                  <>
                    <span className="tnum font-medium text-ink">{pendingConflicts}</span> conflicting
                    file{pendingConflicts === 1 ? "" : "s"} — never installed by loadout
                  </>
                )}
              </p>
              <button
                onClick={() => planMutation.mutate()}
                disabled={planMutation.isPending}
                className="btn-outline"
              >
                {planMutation.isPending ? "Planning…" : "Review & apply"}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
