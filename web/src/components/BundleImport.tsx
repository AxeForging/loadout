import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type BundleImportPlan, type BundleImportResult } from "../api";

// The receiving half of "share": pick a teammate's .loadout.tar.gz or paste
// a single-file YAML export, review the merge plan (new / conflicting /
// identical / sources), choose a conflict policy, then confirm — mirrors
// SyncDock's plan → apply ritual, just fed by an uploaded file or pasted
// text instead of the local diff.
export function BundleImport() {
  const queryClient = useQueryClient();
  const fileInput = useRef<HTMLInputElement>(null);
  const [fileName, setFileName] = useState<string | null>(null);
  const [plan, setPlan] = useState<BundleImportPlan | null>(null);
  const [overwrite, setOverwrite] = useState(false);
  const [result, setResult] = useState<BundleImportResult | null>(null);
  const [intake, setIntake] = useState<"file" | "paste">("file");
  const [pastedYaml, setPastedYaml] = useState("");

  const planMutation = useMutation({
    mutationFn: (file: File) => api.bundleImportPlan(file),
    onSuccess: (p) => {
      setPlan(p);
      setOverwrite(false);
      setResult(null);
    },
  });
  const planYamlMutation = useMutation({
    mutationFn: (yaml: string) => api.bundleImportPlanYAML(yaml),
    onSuccess: (p) => {
      setPlan(p);
      setOverwrite(false);
      setResult(null);
    },
  });
  const applyMutation = useMutation({
    mutationFn: () => api.bundleImportApply(plan!.token, overwrite),
    onSuccess: (res) => {
      setResult(res);
      setPlan(null);
      setFileName(null);
      setPastedYaml("");
      if (fileInput.current) fileInput.current.value = "";
      void queryClient.invalidateQueries();
    },
  });

  const changes = plan ? plan.new.length + plan.sources.length + (overwrite ? plan.conflicts.length : 0) : 0;

  function onFileChosen(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setFileName(file.name);
    setResult(null);
    planMutation.mutate(file);
  }

  function reviewPastedYaml() {
    if (!pastedYaml.trim()) return;
    setResult(null);
    planYamlMutation.mutate(pastedYaml);
  }

  function cancelPlan() {
    setPlan(null);
    setFileName(null);
    setPastedYaml("");
    if (fileInput.current) fileInput.current.value = "";
  }

  return (
    <div className="mt-6 border-t border-line pt-5">
      <p className="font-display text-xs font-semibold uppercase tracking-wider text-ink-dim">
        Import a bundle
      </p>
      <p className="mt-2 text-sm leading-relaxed text-ink-dim">
        Received a <code className="font-mono text-xs text-ink">.loadout.tar.gz</code> or a{" "}
        <code className="font-mono text-xs text-ink">.loadout.yaml</code> export from a teammate?
        Upload the file or paste the YAML here to review and merge it into this store.
      </p>

      {result ? (
        <div className="rise mt-4 flex items-center justify-between">
          <p className="stamp text-sm text-sync">
            <span className="mr-2 font-mono text-[10px]">■</span>
            Imported {result.applied.length} artifact{result.applied.length === 1 ? "" : "s"}
            {result.sourcesAdded > 0
              ? `, ${result.sourcesAdded} new source${result.sourcesAdded === 1 ? "" : "s"}`
              : ""}
            . Run a sync to install into your agents.
          </p>
          <button
            onClick={() => setResult(null)}
            className="text-xs text-ink-faint transition-colors hover:text-ink"
          >
            dismiss
          </button>
        </div>
      ) : plan ? (
        <div className="rise mt-4 border border-line-strong bg-raised p-4">
          <div className="flex items-baseline justify-between">
            <p className="font-display text-sm font-semibold uppercase tracking-wider text-ink">
              {plan.meta.name} — {changes} change{changes === 1 ? "" : "s"}
            </p>
            <p className="font-mono text-[11px] text-ink-faint">
              by {plan.meta.createdBy || "unknown"}
            </p>
          </div>

          <ul className="mt-3 max-h-44 space-y-1 overflow-y-auto pr-1 font-mono text-xs">
            {plan.new.map((a) => (
              <li key={`new-${a.id}`} className="flex items-center gap-3">
                <span className="w-20 text-sync">+ new</span>
                <span className="text-ink">{a.id}</span>
                <span className="text-ink-faint">{a.kind}</span>
              </li>
            ))}
            {plan.conflicts.map((a) => (
              <li key={`conflict-${a.id}`} className="flex items-center gap-3">
                <span className="w-20 text-warn">{overwrite ? "! overwrite" : "! skip"}</span>
                <span className="text-ink">{a.id}</span>
                <span className="text-ink-faint">{a.kind}</span>
              </li>
            ))}
            {plan.sources.map((s) => (
              <li key={`source-${s.name}`} className="flex items-center gap-3">
                <span className="w-20 text-sync">+ source</span>
                <span className="text-ink">{s.name}</span>
              </li>
            ))}
          </ul>
          {plan.identical.length > 0 ? (
            <p className="mt-2 font-mono text-xs text-ink-faint">
              = {plan.identical.length} artifact{plan.identical.length === 1 ? "" : "s"} identical,
              nothing to do
            </p>
          ) : null}

          {plan.warnings.length > 0 ? (
            <ul className="mt-3 space-y-1 text-xs text-warn">
              {plan.warnings.map((w, i) => (
                <li key={`warn-${i}`}>! {w}</li>
              ))}
            </ul>
          ) : null}

          {plan.conflicts.length > 0 ? (
            <label className="mt-3 flex items-center gap-2 text-xs text-ink-dim">
              <input
                type="checkbox"
                checked={overwrite}
                onChange={(e) => setOverwrite(e.target.checked)}
              />
              Overwrite {plan.conflicts.length} conflicting artifact
              {plan.conflicts.length === 1 ? "" : "s"} with the bundle's version
            </label>
          ) : null}

          <div className="mt-4 flex items-center gap-3 border-t border-line pt-3">
            <button
              onClick={() => applyMutation.mutate()}
              disabled={applyMutation.isPending || changes === 0}
              className="btn-ember"
            >
              {applyMutation.isPending ? "Importing…" : `Import ${changes}`}
            </button>
            <button
              onClick={cancelPlan}
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
        <div className="mt-4">
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => setIntake("file")}
              data-on={intake === "file"}
              className="chip"
            >
              choose file
            </button>
            <button
              type="button"
              onClick={() => setIntake("paste")}
              data-on={intake === "paste"}
              className="chip"
            >
              paste YAML
            </button>
          </div>

          {intake === "file" ? (
            <div className="mt-3 flex items-center gap-3">
              <label className="btn-outline inline-flex cursor-pointer items-center gap-2">
                <span aria-hidden className="font-mono text-ember">
                  ↑
                </span>
                {fileName ?? "Choose bundle or YAML…"}
                <input
                  ref={fileInput}
                  type="file"
                  accept=".tar.gz,.tgz,.yaml,.yml,application/gzip"
                  className="hidden"
                  onChange={onFileChosen}
                />
              </label>
              {planMutation.isPending ? (
                <span className="text-xs text-ink-faint">Reviewing…</span>
              ) : planMutation.isError ? (
                <span className="text-xs text-danger">{String(planMutation.error)}</span>
              ) : null}
            </div>
          ) : (
            <div className="mt-3">
              <textarea
                value={pastedYaml}
                onChange={(e) => setPastedYaml(e.target.value)}
                placeholder="Paste a loadout-config/v1 YAML document…"
                rows={8}
                className="w-full resize-y overflow-x-auto border border-line bg-bg-deep/60 p-4 font-mono text-xs leading-relaxed"
              />
              <div className="mt-3 flex items-center gap-3">
                <button
                  type="button"
                  onClick={reviewPastedYaml}
                  disabled={planYamlMutation.isPending || !pastedYaml.trim()}
                  className="btn-outline"
                >
                  {planYamlMutation.isPending ? "Reviewing…" : "Review"}
                </button>
                {planYamlMutation.isError ? (
                  <span className="text-xs text-danger">{String(planYamlMutation.error)}</span>
                ) : null}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
