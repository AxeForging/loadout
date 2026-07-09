import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { PageHeader } from "../components/Layout";
import { useConfirmedAction } from "../hooks/useConfirmedAction";

// Browsing/managing every saved custom instruction and its groups. Entries
// are content-addressed (id = hash of text), so this page never offers
// in-place text editing — renaming would mint a new id and silently orphan
// any group referencing the old one. Delete + re-add is the honest way to
// change an entry's wording.
export function LibraryPage() {
  const queryClient = useQueryClient();
  const library = useQuery({ queryKey: ["library"], queryFn: api.libraryInstructions });
  const groups = useQuery({ queryKey: ["library-groups"], queryFn: api.libraryGroups });

  const [draft, setDraft] = useState("");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [groupName, setGroupName] = useState("");

  const addMutation = useMutation({
    mutationFn: (text: string) => api.libraryAdd(text),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["library"] }),
  });
  const removeMutation = useMutation({
    mutationFn: (id: string) => api.libraryRemove(id),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["library"] }),
  });
  const saveGroupMutation = useMutation({
    mutationFn: () => api.libraryGroupSave(groupName, [...selected]),
    onSuccess: () => {
      setGroupName("");
      setSelected(new Set());
      void queryClient.invalidateQueries({ queryKey: ["library-groups"] });
    },
  });
  const removeGroupMutation = useMutation({
    mutationFn: (name: string) => api.libraryGroupRemove(name),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["library-groups"] }),
  });

  const entryDelete = useConfirmedAction();
  const groupDelete = useConfirmedAction();

  const entries = library.data ?? [];
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return entries;
    return entries.filter((e) => e.text.toLowerCase().includes(q));
  }, [entries, search]);

  function toggleSelected(id: string) {
    setSelected((s) => {
      const next = new Set(s);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function textFor(id: string) {
    return entries.find((e) => e.id === id)?.text ?? id;
  }

  function addDraft() {
    const text = draft.trim();
    if (text === "") return;
    addMutation.mutate(text);
    setDraft("");
  }

  return (
    <div>
      <PageHeader
        crate="armory / library"
        title="Instruction library"
        sub="Every custom instruction you've saved, in one place — search it, delete what's stale, and group entries so a whole set can be added to a wizard run in one click."
      />

      <div className="flex flex-wrap items-center gap-2">
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== "Enter") return;
            e.preventDefault();
            addDraft();
          }}
          placeholder="add a new instruction, press Enter"
          className="field w-full max-w-md"
        />
        <button
          type="button"
          onClick={addDraft}
          disabled={draft.trim() === ""}
          className="btn-ghost"
        >
          Add
        </button>
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="search…"
          className="field w-full max-w-xs"
        />
      </div>

      {entries.length === 0 ? (
        <p className="mt-6 text-sm text-ink-faint">No saved instructions yet.</p>
      ) : filtered.length === 0 ? (
        <p className="mt-6 text-sm text-ink-faint">Nothing matches "{search}".</p>
      ) : (
        <ul className="mt-4 space-y-1 border-y border-line py-2">
          {filtered.map((entry) => (
            <li key={entry.id} className="flex items-start gap-3 py-1 font-mono text-sm text-ink">
              <span className="min-w-0 flex-1 whitespace-normal break-words">{entry.text}</span>
              <button
                type="button"
                onClick={() => {
                  if (entryDelete.requestConfirm(entry.id)) removeMutation.mutate(entry.id);
                }}
                onBlur={() => entryDelete.cancelIfArmed(entry.id)}
                aria-label={
                  entryDelete.confirmingId === entry.id ? `confirm delete ${entry.text}` : `delete ${entry.text}`
                }
                className={
                  entryDelete.confirmingId === entry.id
                    ? "shrink-0 font-medium text-danger"
                    : "shrink-0 text-xs text-ink-faint transition-colors hover:text-danger"
                }
              >
                {entryDelete.confirmingId === entry.id ? "confirm ×" : "×"}
              </button>
            </li>
          ))}
        </ul>
      )}

      <section className="mt-10 border-t border-line pt-6">
        <h2 className="crate">create a group</h2>
        <p className="mt-2 text-sm leading-relaxed text-ink-dim">
          Select entries below, name the group, save it — then apply the whole set from the wizard in one click.
        </p>
        {entries.length === 0 ? (
          <p className="mt-3 text-sm text-ink-faint">Add some instructions first.</p>
        ) : (
          <div className="mt-3 flex flex-wrap gap-2">
            {filtered.map((entry) => (
              <button
                key={entry.id}
                type="button"
                onClick={() => toggleSelected(entry.id)}
                data-on={selected.has(entry.id)}
                className="chip"
              >
                {selected.has(entry.id) ? (
                  <span aria-hidden className="mr-1.5 align-middle font-mono text-[9px]">
                    ■
                  </span>
                ) : null}
                {entry.text}
              </button>
            ))}
          </div>
        )}
        <div className="mt-3 flex items-center gap-2">
          <input
            type="text"
            value={groupName}
            onChange={(e) => setGroupName(e.target.value)}
            placeholder="group-name"
            className="field w-48"
          />
          <button
            type="button"
            onClick={() => saveGroupMutation.mutate()}
            disabled={saveGroupMutation.isPending || groupName.trim() === "" || selected.size === 0}
            className="btn-ghost"
          >
            {saveGroupMutation.isPending ? "Saving…" : `Save group (${selected.size})`}
          </button>
          {saveGroupMutation.isError ? (
            <span className="text-xs text-danger">{String(saveGroupMutation.error)}</span>
          ) : null}
        </div>
      </section>

      <section className="mt-10 border-t border-line pt-6">
        <h2 className="crate">groups</h2>
        {(groups.data ?? []).length === 0 ? (
          <p className="mt-2 text-sm text-ink-faint">No groups yet.</p>
        ) : (
          <ul className="mt-3 space-y-3">
            {(groups.data ?? []).map((group) => (
              <li key={group.name} className="border border-line-strong bg-raised p-3">
                <div className="flex items-center justify-between gap-3">
                  <span className="font-mono text-sm text-ink">
                    {group.name} <span className="text-ink-faint">({group.entryIds.length})</span>
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      if (groupDelete.requestConfirm(group.name)) removeGroupMutation.mutate(group.name);
                    }}
                    onBlur={() => groupDelete.cancelIfArmed(group.name)}
                    aria-label={
                      groupDelete.confirmingId === group.name
                        ? `confirm delete group ${group.name}`
                        : `delete group ${group.name}`
                    }
                    className={
                      groupDelete.confirmingId === group.name
                        ? "shrink-0 font-medium text-danger"
                        : "shrink-0 text-xs text-ink-faint transition-colors hover:text-danger"
                    }
                  >
                    {groupDelete.confirmingId === group.name ? "confirm ×" : "×"}
                  </button>
                </div>
                <ul className="mt-2 space-y-1 font-mono text-xs text-ink-dim">
                  {group.entryIds.map((id) => (
                    <li key={id} className="break-words">
                      {textFor(id)}
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
