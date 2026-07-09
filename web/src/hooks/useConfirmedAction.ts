import { useEffect, useRef, useState } from "react";

const CONFIRM_WINDOW_MS = 3000;

// Arms a one-click-away destructive action (e.g. deleting something with no
// undo). The first call to requestConfirm(id) arms it and returns false —
// the caller should not act yet, just show a "confirm?" state. A second
// call for the SAME id within the window confirms it (returns true, caller
// performs the action) and disarms. Armed state auto-resets after the
// window so a stray click on stale UI can never trigger the action.
export function useConfirmedAction() {
  const [confirmingId, setConfirmingId] = useState<string | null>(null);
  const timeout = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timeout.current) clearTimeout(timeout.current);
    };
  }, []);

  function requestConfirm(id: string): boolean {
    if (timeout.current) clearTimeout(timeout.current);
    if (confirmingId === id) {
      setConfirmingId(null);
      return true;
    }
    setConfirmingId(id);
    timeout.current = setTimeout(() => setConfirmingId(null), CONFIRM_WINDOW_MS);
    return false;
  }

  // Disarms id only if it's the one currently armed — safe to call from an
  // unrelated element's blur handler without clobbering a different id.
  function cancelIfArmed(id: string) {
    setConfirmingId((current) => (current === id ? null : current));
  }

  return { confirmingId, requestConfirm, cancelIfArmed };
}
