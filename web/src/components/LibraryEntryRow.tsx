import { useState } from "react";

// Click-to-edit-in-place text: click the text to turn it into an input,
// Enter commits, Escape cancels, blur commits. Committing an empty string
// never silently deletes anything by default — it just reverts — since this
// renders persisted library entries elsewhere; callers that want "empty
// commit removes it" (the wizard's own in-progress, unsaved row list) opt in
// via onEmptyCommit.
export function LibraryEntryRow({
  text,
  onSave,
  onEmptyCommit,
}: {
  text: string;
  onSave: (newText: string) => void;
  onEmptyCommit?: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(text);

  function startEdit() {
    setDraft(text);
    setEditing(true);
  }

  function commit() {
    const trimmed = draft.trim();
    if (trimmed === "") {
      onEmptyCommit?.();
    } else if (trimmed !== text) {
      onSave(trimmed);
    }
    setEditing(false);
  }

  function cancel() {
    setEditing(false);
  }

  if (editing) {
    return (
      <input
        type="text"
        autoFocus
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            commit();
          } else if (e.key === "Escape") {
            e.preventDefault();
            cancel();
          }
        }}
        onBlur={commit}
        className="field min-w-0 flex-1"
      />
    );
  }

  return (
    <button
      type="button"
      onClick={startEdit}
      className="min-w-0 flex-1 whitespace-normal break-words text-left transition-colors hover:text-ember"
    >
      {text}
    </button>
  );
}
