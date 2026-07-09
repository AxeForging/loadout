import { useState } from "react";
import type { CSSProperties } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";

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
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [editDraft, setEditDraft] = useState("");
  const queryClient = useQueryClient();
  const library = useQuery({ queryKey: ["library"], queryFn: api.libraryInstructions });

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
    const trimmed = text.trim();
    if (trimmed === "" || value.includes(trimmed)) return;
    onChange([...value, trimmed]);
  }

  function removeRow(index: number) {
    onChange(value.filter((_, i) => i !== index));
    if (editingIndex === index) setEditingIndex(null);
  }

  function startEdit(index: number) {
    setEditingIndex(index);
    setEditDraft(value[index]);
  }

  function commitEdit() {
    if (editingIndex === null) return;
    const trimmed = editDraft.trim();
    if (trimmed === "") {
      removeRow(editingIndex);
    } else {
      onChange(value.map((v, i) => (i === editingIndex ? trimmed : v)));
    }
    setEditingIndex(null);
    setEditDraft("");
  }

  function cancelEdit() {
    setEditingIndex(null);
    setEditDraft("");
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
        <ul className="mt-3 space-y-1 border-y border-line py-2">
          {value.map((item, i) => (
            <li
              key={`${item}-${i}`}
              className="rise flex items-start gap-3 py-0.5 font-mono text-sm text-ink"
              style={{ "--i": i } as CSSProperties}
            >
              {editingIndex === i ? (
                <input
                  type="text"
                  autoFocus
                  value={editDraft}
                  onChange={(e) => setEditDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      commitEdit();
                    } else if (e.key === "Escape") {
                      e.preventDefault();
                      cancelEdit();
                    }
                  }}
                  onBlur={commitEdit}
                  className="field min-w-0 flex-1"
                />
              ) : (
                <button
                  type="button"
                  onClick={() => startEdit(i)}
                  className="min-w-0 flex-1 whitespace-normal break-words text-left transition-colors hover:text-ember"
                >
                  {item}
                </button>
              )}
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
                className="shrink-0 text-xs text-ink-faint transition-colors hover:text-danger"
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
                    onClick={() => removeMutation.mutate(entry.id)}
                    aria-label={`delete ${entry.text} from library`}
                    className="text-xs text-ink-faint transition-colors hover:text-danger"
                  >
                    ×
                  </button>
                </span>
              );
            })}
          </div>
        </div>
      ) : null}
    </div>
  );
}
