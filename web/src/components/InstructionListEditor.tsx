import { useState } from "react";
import type { CSSProperties } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useConfirmedAction } from "../hooks/useConfirmedAction";
import { LibraryEntryRow } from "./LibraryEntryRow";

// Row-based editor for the "list" question type: type an instruction, press
// Enter to add it as a row; each row can be deleted or saved to a
// store-wide library for reuse across future wizard runs. Library entries
// render as chips below — click one to re-add its text without retyping.
export function InstructionListEditor({
  value,
  onChange,
}: {
  value: string[];
  onChange: (v: string[]) => void;
}) {
  const [draft, setDraft] = useState("");
  const { confirmingId: confirmingDeleteId, requestConfirm: requestDeleteConfirm, cancelIfArmed } = useConfirmedAction();
  const queryClient = useQueryClient();
  const library = useQuery({ queryKey: ["library"], queryFn: api.libraryInstructions });
  const groups = useQuery({ queryKey: ["library-groups"], queryFn: api.libraryGroups });

  const saveMutation = useMutation({
    mutationFn: (text: string) => api.libraryAdd(text),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["library"] }),
  });
  const removeMutation = useMutation({
    mutationFn: (id: string) => api.libraryRemove(id),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["library"] }),
  });

  const savedTexts = new Set((library.data ?? []).map((e) => e.text));

  function addRow(text: string) {
    addRows([text]);
  }

  // Adds every text not already present in one onChange call. Looping
  // addRow per item would have each call close over the same stale `value`
  // prop and clobber the others instead of accumulating — this computes the
  // full next array once.
  function addRows(texts: string[]) {
    const seen = new Set(value);
    const toAdd: string[] = [];
    for (const raw of texts) {
      const trimmed = raw.trim();
      if (trimmed !== "" && !seen.has(trimmed)) {
        seen.add(trimmed);
        toAdd.push(trimmed);
      }
    }
    if (toAdd.length > 0) onChange([...value, ...toAdd]);
  }

  function removeRow(index: number) {
    onChange(value.filter((_, i) => i !== index));
  }

  // Deleting a library entry is permanent (unlike removing a row from the
  // in-progress list above, which only edits local state until the wizard
  // is generated) — arm on the first click, require a second click within
  // the confirm window to actually delete, so a stray click never silently
  // destroys it.
  function requestDeleteFromLibrary(id: string) {
    if (requestDeleteConfirm(id)) removeMutation.mutate(id);
  }

  return (
    <div>
      <div className="flex items-center gap-2">
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== "Enter") return;
            e.preventDefault();
            addRow(draft);
            setDraft("");
          }}
          placeholder="type an instruction, press Enter"
          className="field w-full max-w-md"
        />
        <button
          type="button"
          onClick={() => {
            addRow(draft);
            setDraft("");
          }}
          disabled={draft.trim() === ""}
          className="btn-ghost"
        >
          Add
        </button>
      </div>

      {value.length > 0 ? (
        <ul className="mt-3 divide-y divide-line border-y border-line">
          {value.map((item, i) => (
            <li
              key={`${item}-${i}`}
              className="rise flex items-start gap-3 py-2 font-mono text-sm text-ink"
              style={{ "--i": i } as CSSProperties}
            >
              <LibraryEntryRow
                text={item}
                onSave={(newText) => onChange(value.map((v, idx) => (idx === i ? newText : v)))}
                onEmptyCommit={() => removeRow(i)}
              />
              <button
                type="button"
                onClick={() => saveMutation.mutate(item)}
                disabled={saveMutation.isPending || savedTexts.has(item)}
                className="shrink-0 text-xs text-ink-faint transition-colors hover:text-ink"
              >
                {savedTexts.has(item) ? "■ saved" : "save"}
              </button>
              <button
                type="button"
                onClick={() => removeRow(i)}
                aria-label={`remove ${item}`}
                className="btn-x"
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      ) : null}

      {(library.data ?? []).length > 0 ? (
        <div className="mt-4">
          <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">
            from your library
          </p>
          <div className="mt-2 flex flex-wrap gap-2">
            {(library.data ?? []).map((entry) => {
              const active = value.includes(entry.text);
              return (
                <span key={entry.id} className="inline-flex items-center gap-1">
                  <button type="button" onClick={() => addRow(entry.text)} data-on={active} className="chip">
                    {active ? (
                      <span aria-hidden className="mr-1.5 align-middle font-mono text-[9px]">
                        ■
                      </span>
                    ) : null}
                    {entry.text}
                  </button>
                  <button
                    type="button"
                    onClick={() => requestDeleteFromLibrary(entry.id)}
                    onBlur={() => cancelIfArmed(entry.id)}
                    aria-label={
                      confirmingDeleteId === entry.id
                        ? `confirm delete ${entry.text} from library`
                        : `delete ${entry.text} from library`
                    }
                    data-armed={confirmingDeleteId === entry.id}
                    className="btn-x"
                  >
                    {confirmingDeleteId === entry.id ? "confirm ×" : "×"}
                  </button>
                </span>
              );
            })}
          </div>
        </div>
      ) : null}

      {(groups.data ?? []).length > 0 ? (
        <div className="mt-3">
          <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">groups</p>
          <div className="mt-2 flex flex-wrap gap-2">
            {(groups.data ?? []).map((group) => {
              const entryTexts = group.entryIds
                .map((id) => (library.data ?? []).find((e) => e.id === id)?.text)
                .filter((t): t is string => Boolean(t));
              const allActive = entryTexts.length > 0 && entryTexts.every((t) => value.includes(t));
              return (
                <button
                  key={group.name}
                  type="button"
                  onClick={() => addRows(entryTexts)}
                  data-on={allActive}
                  className="chip"
                >
                  {allActive ? (
                    <span aria-hidden className="mr-1.5 align-middle font-mono text-[9px]">
                      ■
                    </span>
                  ) : null}
                  {group.name} ({entryTexts.length})
                </button>
              );
            })}
          </div>
        </div>
      ) : null}
    </div>
  );
}
