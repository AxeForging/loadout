import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useConfirmedAction } from "./useConfirmedAction";

describe("useConfirmedAction", () => {
  it("arms on the first call and does not confirm", () => {
    const { result } = renderHook(() => useConfirmedAction());

    let confirmed = false;
    act(() => {
      confirmed = result.current.requestConfirm("a");
    });

    expect(confirmed).toBe(false);
    expect(result.current.confirmingId).toBe("a");
  });

  it("confirms on a second call for the same id", () => {
    const { result } = renderHook(() => useConfirmedAction());

    act(() => {
      result.current.requestConfirm("a");
    });
    let confirmed = false;
    act(() => {
      confirmed = result.current.requestConfirm("a");
    });

    expect(confirmed).toBe(true);
    expect(result.current.confirmingId).toBeNull();
  });

  it("re-arms instead of confirming when a different id is requested", () => {
    const { result } = renderHook(() => useConfirmedAction());

    act(() => {
      result.current.requestConfirm("a");
    });
    let confirmed = false;
    act(() => {
      confirmed = result.current.requestConfirm("b");
    });

    expect(confirmed).toBe(false);
    expect(result.current.confirmingId).toBe("b");
  });

  it("auto-resets after the confirm window elapses", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useConfirmedAction());

    act(() => {
      result.current.requestConfirm("a");
    });
    expect(result.current.confirmingId).toBe("a");

    act(() => {
      vi.advanceTimersByTime(3000);
    });
    expect(result.current.confirmingId).toBeNull();

    vi.useRealTimers();
  });

  it("cancelIfArmed only clears the matching id", () => {
    const { result } = renderHook(() => useConfirmedAction());

    act(() => {
      result.current.requestConfirm("a");
    });
    act(() => {
      result.current.cancelIfArmed("b");
    });
    expect(result.current.confirmingId).toBe("a");

    act(() => {
      result.current.cancelIfArmed("a");
    });
    expect(result.current.confirmingId).toBeNull();
  });
});
