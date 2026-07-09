import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BundleImport } from "./BundleImport";
import { api, type BundleImportPlan } from "../api";

afterEach(cleanup);

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof import("../api")>("../api");
  return {
    ...actual,
    api: {
      ...actual.api,
      bundleImportPlan: vi.fn(),
      bundleImportPlanYAML: vi.fn(),
      bundleImportApply: vi.fn(),
    },
  };
});

function renderWithClient() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <BundleImport />
    </QueryClientProvider>,
  );
}

const bundleFile = new File(["fake-archive-bytes"], "team.loadout.tar.gz", {
  type: "application/gzip",
});

const samplePlan: BundleImportPlan = {
  token: "tok-123",
  meta: { name: "team-kit", createdBy: "loadout cli", createdAt: "2026-01-01T00:00:00Z", artifacts: 2 },
  new: [{ id: "extra", kind: "skill", path: "skills/extra" }],
  conflicts: [],
  identical: [],
  sources: [],
};

describe("BundleImport", () => {
  it("renders the idle upload prompt", () => {
    renderWithClient();
    expect(screen.getByText("Choose bundle…")).toBeInTheDocument();
  });

  it("shows a pending state while the plan request is in flight", async () => {
    let resolvePlan!: (p: BundleImportPlan) => void;
    vi.mocked(api.bundleImportPlan).mockReturnValue(
      new Promise((resolve) => {
        resolvePlan = resolve;
      }),
    );
    renderWithClient();

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await userEvent.upload(input, bundleFile);

    expect(await screen.findByText("Reviewing…")).toBeInTheDocument();
    resolvePlan(samplePlan);
  });

  it("shows an error state when the plan request fails", async () => {
    vi.mocked(api.bundleImportPlan).mockRejectedValue(new Error("not a loadout bundle"));
    renderWithClient();

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await userEvent.upload(input, bundleFile);

    expect(await screen.findByText(/not a loadout bundle/)).toBeInTheDocument();
  });

  it("renders the plan review with new/conflict/source counts once resolved", async () => {
    vi.mocked(api.bundleImportPlan).mockResolvedValue(samplePlan);
    renderWithClient();

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await userEvent.upload(input, bundleFile);

    await waitFor(() => expect(screen.getByText(/team-kit/)).toBeInTheDocument());
    expect(screen.getByText("extra")).toBeInTheDocument();
    expect(screen.getByText("Import 1")).toBeInTheDocument();
  });

  it("switches to the paste-YAML tab and reviews a pasted document", async () => {
    vi.mocked(api.bundleImportPlanYAML).mockResolvedValue(samplePlan);
    renderWithClient();

    await userEvent.click(screen.getByText("paste YAML"));
    expect(screen.queryByText("Choose bundle…")).not.toBeInTheDocument();

    await userEvent.type(
      screen.getByPlaceholderText(/Paste a loadout-config\/v1 YAML document/),
      "apiVersion: loadout-config/v1",
    );
    await userEvent.click(screen.getByText("Review"));

    await waitFor(() => expect(screen.getByText(/team-kit/)).toBeInTheDocument());
    expect(api.bundleImportPlanYAML).toHaveBeenCalledWith("apiVersion: loadout-config/v1");
  });

  it("the Review button is disabled until something is pasted", async () => {
    renderWithClient();
    await userEvent.click(screen.getByText("paste YAML"));

    expect(screen.getByText("Review")).toBeDisabled();
  });
});
