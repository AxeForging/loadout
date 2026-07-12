import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SyncDock } from "./SyncDock";
import { api, type Overview, type SyncPlan } from "../api";

afterEach(cleanup);

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof import("../api")>("../api");
  return {
    ...actual,
    api: {
      ...actual.api,
      overview: vi.fn(),
      syncPlan: vi.fn(),
      syncApply: vi.fn(),
      diff: vi.fn(),
    },
  };
});

function renderWithClient() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <SyncDock />
    </QueryClientProvider>,
  );
}

const overviewWithConflict: Overview = {
  store: { name: "test", root: "/store", artifacts: 1 },
  agents: [],
  pending: 0,
  conflicts: 1,
  states: { conflict: 1 },
};

const planWithConflict: SyncPlan = {
  token: "tok-abcdef12",
  changes: [],
  conflicts: [{ op: "conflict", agent: "claude-code", artifact: "plan", relPath: "commands/plan.md" }],
};

const overviewWithPending: Overview = {
  store: { name: "test", root: "/store", artifacts: 1 },
  agents: [],
  pending: 1,
  conflicts: 0,
  states: { missing: 1 },
};

const planWithUpdate: SyncPlan = {
  token: "tok-abcdef12",
  changes: [{ op: "update", agent: "claude-code", artifact: "context7", relPath: ".claude.json" }],
  conflicts: [],
};

describe("SyncDock", () => {
  it("expands a plan row into the exact diff apply would write", async () => {
    vi.mocked(api.overview).mockResolvedValue(overviewWithPending);
    vi.mocked(api.syncPlan).mockResolvedValue(planWithUpdate);
    vi.mocked(api.diff).mockResolvedValue([
      {
        agent: "claude-code",
        artifact: "context7",
        relPath: ".claude.json",
        diff: "--- a/.claude.json\n+++ b/.claude.json\n+  \"context7\": {}",
      },
    ]);
    renderWithClient();

    await userEvent.click(await screen.findByText("Review & apply"));
    await userEvent.click(await screen.findByRole("button", { name: /\.claude\.json/ }));

    expect(api.diff).toHaveBeenCalledWith({ artifact: "context7", agent: "claude-code" });
    expect(await screen.findByText('+ "context7": {}')).toBeInTheDocument();

    // Collapses again on a second click.
    await userEvent.click(screen.getByRole("button", { name: /\.claude\.json/ }));
    expect(screen.queryByText('+ "context7": {}')).not.toBeInTheDocument();
  });

  it("surfaces a conflict-only state even with zero pending changes", async () => {
    vi.mocked(api.overview).mockResolvedValue(overviewWithConflict);
    renderWithClient();

    expect(await screen.findByText(/conflicting file/)).toBeInTheDocument();
    expect(screen.getByText("Review & apply")).toBeInTheDocument();
  });

  it("keeps the apply disabled until the overwrite box is checked, and forwards the flag", async () => {
    vi.mocked(api.overview).mockResolvedValue(overviewWithConflict);
    vi.mocked(api.syncPlan).mockResolvedValue(planWithConflict);
    vi.mocked(api.syncApply).mockResolvedValue({ applied: 1 });
    renderWithClient();

    await userEvent.click(await screen.findByText("Review & apply"));

    // The conflicting file is listed; with nothing to write, apply is disabled.
    await waitFor(() => expect(screen.getByText("commands/plan.md")).toBeInTheDocument());
    expect(screen.getByText("Apply 0")).toBeDisabled();

    // Opting in promotes the conflict and forwards overwriteConflicts=true.
    await userEvent.click(screen.getByRole("checkbox"));
    const apply = screen.getByText("Apply 1");
    expect(apply).toBeEnabled();
    await userEvent.click(apply);

    expect(api.syncApply).toHaveBeenCalledWith("tok-abcdef12", true);
  });
});
