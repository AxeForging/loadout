import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type SyncPlan } from "../api";

// The gated-sync ritual, docked at the bottom of every page: a persistent
// pending indicator → explicit "review" (plan + token) → explicit "apply".
export function SyncDock() {
  const queryClient = useQueryClient();
  const [plan, setPlan] = useState<SyncPlan | null>(null);
  const [applied, setApplied] = useState<number | null>(null);

  const overview = useQuery({ queryKey: ["overview"], queryFn: api.overview });

  const planMutation = useMutation({
    mutationFn: api.syncPlan,
    onSuccess: (p) => {
      setPlan(p);
      setApplied(null);
    },
  });
  const applyMutation = useMutation({
    mutationFn: (token: string) => api.syncApply(token),
    onSuccess: (res) => {
      setApplied(res.applied);
      setPlan(null);
      void queryClient.invalidateQueries();
    },
  });

  const pending = overview.data?.pending ?? 0;
  if (pending === 0 && !plan && applied === null) return null;

  return (
    <div className="fixed inset-x-0 bottom-0 z-10">
      <div className="mx-auto max-w-6xl px-6 pb-4">
        <div className="rise rounded-md border border-line-strong bg-raised shadow-[0_-8px_32px_oklch(0.08_0.01_60/0.6)]">
          {applied !== null && !plan ? (
            <div className="flex items-center justify-between px-5 py-3">
              <p className="text-sm text-sync">
                <span className="mr-2 font-mono text-[10px]">■</span>
                Applied {applied} change{applied === 1 ? "" : "s"}. Agents are up to date.
              </p>
              <button
                onClick={() => setApplied(null)}
                className="text-xs text-ink-faint hover:text-ink"
              >
                dismiss
              </button>
            </div>
          ) : plan ? (
            <div className="px-5 py-4">
              <div className="flex items-baseline justify-between">
                <p className="font-display text-sm font-semibold text-ink">
                  Review: {plan.changes.length} change{plan.changes.length === 1 ? "" : "s"}
                </p>
                <p className="font-mono text-[11px] text-ink-faint">
                  plan {plan.token.slice(0, 8)}
                </p>
              </div>
              <ul className="mt-3 max-h-44 space-y-1 overflow-y-auto">
                {plan.changes.map((c, i) => (
                  <li key={i} className="flex items-center gap-3 font-mono text-xs">
                    <span
                      className={
                        c.op === "create" ? "w-12 text-sync" : "w-12 text-warn"
                      }
                    >
                      {c.op}
                    </span>
                    <span className="text-ink-dim">{c.agent}</span>
                    <span className="min-w-0 flex-1 truncate text-ink">{c.relPath}</span>
                    <span className="text-ink-faint">{c.artifact}</span>
                  </li>
                ))}
              </ul>
              <div className="mt-4 flex items-center gap-3">
                <button
                  onClick={() => applyMutation.mutate(plan.token)}
                  disabled={applyMutation.isPending}
                  className="rounded-sm bg-ember px-4 py-1.5 font-display text-sm font-semibold text-bg transition-colors hover:bg-ember-bright disabled:opacity-50"
                >
                  {applyMutation.isPending ? "Applying…" : `Apply ${plan.changes.length}`}
                </button>
                <button
                  onClick={() => setPlan(null)}
                  className="px-2 py-1.5 text-sm text-ink-dim hover:text-ink"
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
                <span className="mr-2 font-mono text-[10px] text-ember">■</span>
                <span className="font-medium text-ink">{pending}</span> pending change
                {pending === 1 ? "" : "s"} across your agents
              </p>
              <button
                onClick={() => planMutation.mutate()}
                disabled={planMutation.isPending}
                className="rounded-sm border border-ember-dim px-4 py-1.5 font-display text-sm font-medium text-ember transition-colors hover:border-ember hover:text-ember-bright disabled:opacity-50"
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
