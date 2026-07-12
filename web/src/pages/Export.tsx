import { useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { api, type Artifact } from "../api";
import { PageHeader } from "../components/Layout";

// Selective export: pick exactly which artifacts, profiles and custom
// instructions go into a downloadable .loadout.tar.gz. The Dashboard's
// one-click "export everything" link stays the fast path; this page is for
// when you want to share less than the whole store.
export function ExportPage() {
  const artifacts = useQuery({ queryKey: ["artifacts"], queryFn: api.artifacts });
  const profiles = useQuery({ queryKey: ["profiles"], queryFn: api.profiles });
  const library = useQuery({ queryKey: ["library"], queryFn: api.libraryInstructions });
  const groups = useQuery({ queryKey: ["library-groups"], queryFn: api.libraryGroups });

  const [selectedArtifacts, setSelectedArtifacts] = useState<Set<string>>(new Set());
  const [selectedProfiles, setSelectedProfiles] = useState<Set<string>>(new Set());
  const [selectedLibrary, setSelectedLibrary] = useState<Set<string>>(new Set());
  const [includeSources, setIncludeSources] = useState(false);
  const [format, setFormat] = useState<"archive" | "flat">("archive");
  const [flatYaml, setFlatYaml] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const byKind = useMemo(() => {
    const map = new Map<string, Artifact[]>();
    for (const a of artifacts.data ?? []) {
      const list = map.get(a.kind) ?? [];
      list.push(a);
      map.set(a.kind, list);
    }
    return map;
  }, [artifacts.data]);

  function toggle(set: Set<string>, setSet: (s: Set<string>) => void, id: string) {
    const next = new Set(set);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setSet(next);
  }

  function currentSelection() {
    return {
      artifactIds: [...selectedArtifacts],
      profileNames: [...selectedProfiles],
      libraryEntryIds: [...selectedLibrary],
      sources: includeSources,
    };
  }

  const archiveMutation = useMutation({
    mutationFn: () => api.bundleExportSelected(currentSelection()),
    onSuccess: (blob) => {
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = "export.loadout.tar.gz";
      link.click();
      URL.revokeObjectURL(url);
    },
  });

  const flatMutation = useMutation({
    mutationFn: () => api.bundleExportFlat(currentSelection()),
    onSuccess: (res) => {
      setFlatYaml(res.yaml);
      setCopied(false);
    },
  });

  const exportMutation = format === "flat" ? flatMutation : archiveMutation;

  async function copyFlatYaml() {
    if (!flatYaml) return;
    await navigator.clipboard.writeText(flatYaml);
    setCopied(true);
  }

  function downloadFlatYaml() {
    if (!flatYaml) return;
    const url = URL.createObjectURL(new Blob([flatYaml], { type: "text/yaml" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = "export.loadout.yaml";
    link.click();
    URL.revokeObjectURL(url);
  }

  const totalSelected = selectedArtifacts.size + selectedProfiles.size + selectedLibrary.size;

  return (
    <div>
      <PageHeader
        crate="armory / export"
        title="Export a bundle"
        sub="Pick exactly what to share — artifacts, profiles, custom instructions — then download a .loadout.tar.gz a teammate can import."
      />

      <section>
        <div className="flex items-center justify-between">
          <h2 className="crate">artifacts</h2>
          {(artifacts.data ?? []).length > 0 ? (
            <button
              type="button"
              onClick={() => setSelectedArtifacts(new Set((artifacts.data ?? []).map((a) => a.id)))}
              className="text-xs text-ink-faint transition-colors hover:text-ink"
            >
              select all
            </button>
          ) : null}
        </div>
        {[...byKind.entries()].map(([kind, items]) => (
          <div key={kind} className="mt-3">
            <div className="flex items-center justify-between">
              <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">{kind}</p>
              <button
                type="button"
                onClick={() => {
                  const next = new Set(selectedArtifacts);
                  items.forEach((a) => next.add(a.id));
                  setSelectedArtifacts(next);
                }}
                className="font-mono text-[10px] text-ink-faint transition-colors hover:text-ink"
              >
                select all
              </button>
            </div>
            <div className="mt-2 flex flex-wrap gap-2">
              {items.map((a) => (
                <button
                  key={a.id}
                  type="button"
                  onClick={() => toggle(selectedArtifacts, setSelectedArtifacts, a.id)}
                  data-on={selectedArtifacts.has(a.id)}
                  className="chip"
                >
                  {selectedArtifacts.has(a.id) ? (
                    <span aria-hidden className="mr-1.5 align-middle font-mono text-[9px]">
                      ■
                    </span>
                  ) : null}
                  {a.id}
                </button>
              ))}
            </div>
          </div>
        ))}
        {artifacts.data && artifacts.data.length === 0 ? (
          <p className="mt-3 text-sm text-ink-faint">No artifacts in the store.</p>
        ) : null}
      </section>

      <section className="mt-8 border-t border-line pt-6">
        <h2 className="crate">profiles</h2>
        {(profiles.data ?? []).length === 0 ? (
          <p className="mt-2 text-sm text-ink-faint">No saved profiles.</p>
        ) : (
          <div className="mt-3 flex flex-wrap gap-2">
            {(profiles.data ?? []).map((name) => (
              <button
                key={name}
                type="button"
                onClick={() => toggle(selectedProfiles, setSelectedProfiles, name)}
                data-on={selectedProfiles.has(name)}
                className="chip"
              >
                {selectedProfiles.has(name) ? (
                  <span aria-hidden className="mr-1.5 align-middle font-mono text-[9px]">
                    ■
                  </span>
                ) : null}
                {name}
              </button>
            ))}
          </div>
        )}
      </section>

      <section className="mt-8 border-t border-line pt-6">
        <h2 className="crate">custom instructions</h2>
        {(library.data ?? []).length === 0 ? (
          <p className="mt-2 text-sm text-ink-faint">No saved instructions.</p>
        ) : (
          <>
            <div className="mt-3 flex flex-wrap gap-2">
              {(library.data ?? []).map((entry) => (
                <button
                  key={entry.id}
                  type="button"
                  onClick={() => toggle(selectedLibrary, setSelectedLibrary, entry.id)}
                  data-on={selectedLibrary.has(entry.id)}
                  className="chip"
                >
                  {selectedLibrary.has(entry.id) ? (
                    <span aria-hidden className="mr-1.5 align-middle font-mono text-[9px]">
                      ■
                    </span>
                  ) : null}
                  {entry.text}
                </button>
              ))}
            </div>
            {(groups.data ?? []).length > 0 ? (
              <div className="mt-3">
                <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">groups</p>
                <div className="mt-2 flex flex-wrap gap-2">
                  {(groups.data ?? []).map((group) => (
                    <button
                      key={group.name}
                      type="button"
                      onClick={() => {
                        const next = new Set(selectedLibrary);
                        group.entryIds.forEach((id) => next.add(id));
                        setSelectedLibrary(next);
                      }}
                      className="chip"
                    >
                      {group.name} ({group.entryIds.length})
                    </button>
                  ))}
                </div>
              </div>
            ) : null}
          </>
        )}
      </section>

      <section className="mt-8 border-t border-line pt-6">
        <label className="flex items-center gap-2 text-sm text-ink-dim">
          <input type="checkbox" checked={includeSources} onChange={(e) => setIncludeSources(e.target.checked)} />
          Include tracked sources (sources.lock.yaml)
        </label>
      </section>

      <section className="mt-8 border-t border-line pt-6">
        <h2 className="crate">format</h2>
        <div className="mt-3 flex flex-wrap gap-2">
          <button
            type="button"
            onClick={() => {
              setFormat("archive");
              setFlatYaml(null);
            }}
            data-on={format === "archive"}
            className="chip"
          >
            .tar.gz archive
          </button>
          <button
            type="button"
            onClick={() => {
              setFormat("flat");
              setFlatYaml(null);
            }}
            data-on={format === "flat"}
            className="chip"
          >
            single YAML file
          </button>
        </div>
        {format === "flat" ? (
          <p className="mt-2 text-sm leading-relaxed text-ink-dim">
            One self-contained document — small enough to paste into a gist, a doc, or a chat.
          </p>
        ) : null}
      </section>

      <div className="mt-8 flex items-center gap-3">
        <button
          type="button"
          onClick={() => exportMutation.mutate()}
          disabled={exportMutation.isPending || totalSelected === 0}
          className="btn-ember"
        >
          {exportMutation.isPending ? "Exporting…" : `Export ${totalSelected} item${totalSelected === 1 ? "" : "s"}`}
        </button>
        {exportMutation.isError ? <span className="text-xs text-danger">{String(exportMutation.error)}</span> : null}
        {format === "archive" && exportMutation.isSuccess ? (
          <span className="stamp text-xs text-sync">■ downloaded</span>
        ) : null}
      </div>

      {format === "flat" && flatYaml ? (
        <div className="rise mt-4 rounded-2xl border border-line-strong bg-raised p-4">
          <div className="flex items-center justify-between">
            <p className="font-display text-xs font-semibold uppercase tracking-wider text-ink-dim">
              export.loadout.yaml
            </p>
            <div className="flex items-center gap-3">
              <button
                type="button"
                onClick={() => void copyFlatYaml()}
                className="text-xs text-ink-faint transition-colors hover:text-ink"
              >
                {copied ? "copied ✓" : "copy"}
              </button>
              <button
                type="button"
                onClick={downloadFlatYaml}
                className="text-xs text-ink-faint transition-colors hover:text-ink"
              >
                download
              </button>
            </div>
          </div>
          <textarea
            readOnly
            value={flatYaml}
            rows={16}
            className="mt-3 w-full resize-y overflow-x-auto rounded-lg border border-line bg-bg-deep/60 p-4 font-mono text-xs leading-relaxed"
          />
        </div>
      ) : null}
    </div>
  );
}
