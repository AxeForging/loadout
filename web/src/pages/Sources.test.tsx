import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRouter,
} from "@tanstack/react-router";
import { SourcesPage } from "./Sources";
import { api, type Source, type SourceUpdatePlanResponse } from "../api";

afterEach(cleanup);

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof import("../api")>("../api");
  return {
    ...actual,
    api: {
      ...actual.api,
      sources: vi.fn(),
      sourcesCheck: vi.fn(),
      sourcesAdd: vi.fn(),
      sourcesUpdatePlan: vi.fn(),
      sourcesUpdateApply: vi.fn(),
    },
  };
});

const sampleSources: Source[] = [
  { name: "team-skills", url: "git@github.com:team/skills.git", ref: "main", commit: "abc1234def5678" },
];

const planWithConflict: SourceUpdatePlanResponse = {
  token: "tok-1",
  plans: [
    {
      source: "team-skills",
      from: "abc1234def5678",
      to: "fff9999aaa0000",
      upToDate: false,
      log: ["fff9999 feat: rework alpha"],
      changed: ["beta"],
      added: [],
      locallyEdited: ["gamma"],
      conflicts: ["alpha"],
      ownedElsewhere: [],
      legacyBaseline: [],
      removed: [],
    },
  ],
};

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const rootRoute = createRootRoute({ component: SourcesPage });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  return render(
    <QueryClientProvider client={client}>
      {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
      <RouterProvider router={router as any} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.mocked(api.sources).mockResolvedValue(sampleSources);
  vi.mocked(api.sourcesUpdatePlan).mockResolvedValue(planWithConflict);
  vi.mocked(api.sourcesUpdateApply).mockResolvedValue({ applied: 1 });
});

describe("SourcesPage update flow", () => {
  it("shows the three-way classification after planning", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /plan updates/i }));

    expect(await screen.findByText(/~ update beta/)).toBeTruthy();
    expect(screen.getByText(/! gamma locally edited — left alone/)).toBeTruthy();
    expect(screen.getByText(/!! alpha — edited locally AND changed upstream/)).toBeTruthy();
  });

  it("applies without discarding unless the checkbox names the conflicts and is checked", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /plan updates/i }));
    await screen.findByText(/!! alpha/);

    // Default apply: forceDiscard stays false and the skip note is visible.
    expect(screen.getByText(/conflicted skills will be skipped/i)).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: /^apply updates$/i }));
    expect(vi.mocked(api.sourcesUpdateApply)).toHaveBeenCalledWith("tok-1", false);
  });

  it("discards only after the explicit opt-in checkbox is checked", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /plan updates/i }));
    await screen.findByText(/!! alpha/);

    const checkbox = screen.getByRole("checkbox", { name: /discard my local edits to alpha/i });
    await userEvent.click(checkbox);
    await userEvent.click(screen.getByRole("button", { name: /apply \+ discard local edits/i }));
    expect(vi.mocked(api.sourcesUpdateApply)).toHaveBeenCalledWith("tok-1", true);
  });
});
