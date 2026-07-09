import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { LibraryPage } from "./Library";
import { api, type LibraryEntry, type LibraryGroup } from "../api";

afterEach(cleanup);

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof import("../api")>("../api");
  return {
    ...actual,
    api: {
      ...actual.api,
      libraryInstructions: vi.fn(),
      libraryAdd: vi.fn(),
      libraryRemove: vi.fn(),
      libraryGroups: vi.fn(),
      libraryGroupSave: vi.fn(),
      libraryGroupRemove: vi.fn(),
    },
  };
});

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <LibraryPage />
    </QueryClientProvider>,
  );
}

const sampleEntries: LibraryEntry[] = [
  { id: "abc123", text: "always write tests first", createdAt: "2026-01-01T00:00:00Z" },
  { id: "def456", text: "no coverage theater", createdAt: "2026-01-01T00:00:00Z" },
];

beforeEach(() => {
  vi.mocked(api.libraryGroups).mockResolvedValue([]);
});

describe("LibraryPage", () => {
  it("renders every saved entry", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue(sampleEntries);
    renderPage();

    // Each entry renders twice by design: once in the list, once as a
    // selectable chip in "create a group".
    expect((await screen.findAllByText("always write tests first")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("no coverage theater").length).toBeGreaterThan(0);
  });

  it("filters entries by search text", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue(sampleEntries);
    renderPage();
    await screen.findAllByText("always write tests first");

    await userEvent.type(screen.getByPlaceholderText("search…"), "coverage");

    expect(screen.queryByText("always write tests first")).not.toBeInTheDocument();
    expect(screen.getAllByText("no coverage theater").length).toBeGreaterThan(0);
  });

  it("adds a new entry when Enter is pressed", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue([]);
    vi.mocked(api.libraryAdd).mockResolvedValue({
      id: "xyz",
      text: "review before merging",
      createdAt: "2026-01-01T00:00:00Z",
    });
    renderPage();

    const input = screen.getByPlaceholderText("add a new instruction, press Enter");
    await userEvent.type(input, "review before merging{Enter}");

    expect(api.libraryAdd).toHaveBeenCalledWith("review before merging");
    expect(input).toHaveValue("");
  });

  it("deletes an entry only on a second, confirming click", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue(sampleEntries);
    vi.mocked(api.libraryRemove).mockResolvedValue({ removed: true });
    renderPage();
    await screen.findAllByText("always write tests first");

    const deleteButton = screen.getByLabelText("delete always write tests first");
    await userEvent.click(deleteButton);
    expect(api.libraryRemove).not.toHaveBeenCalled();
    expect(screen.getByLabelText("confirm delete always write tests first")).toBeInTheDocument();

    await userEvent.click(screen.getByLabelText("confirm delete always write tests first"));
    expect(api.libraryRemove).toHaveBeenCalledWith("abc123");
  });

  it("creates a group from selected entries", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue(sampleEntries);
    vi.mocked(api.libraryGroupSave).mockResolvedValue({
      name: "testing-rules",
      entryIds: ["abc123", "def456"],
      createdAt: "2026-01-01T00:00:00Z",
    });
    renderPage();
    await screen.findAllByText("always write tests first");

    // The entry text renders twice: once in the list, once as a selectable
    // chip in "create a group" — the chips are the second occurrence.
    const allTests = screen.getAllByText("always write tests first");
    const allCoverage = screen.getAllByText("no coverage theater");
    await userEvent.click(allTests[allTests.length - 1]);
    await userEvent.click(allCoverage[allCoverage.length - 1]);

    await userEvent.type(screen.getByPlaceholderText("group-name"), "testing-rules");
    await userEvent.click(screen.getByText("Save group (2)"));

    expect(api.libraryGroupSave).toHaveBeenCalledWith("testing-rules", ["abc123", "def456"]);
  });

  it("renders groups with their member entries and deletes with confirmation", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue(sampleEntries);
    const testingGroup: LibraryGroup = {
      name: "testing-rules",
      entryIds: ["abc123", "def456"],
      createdAt: "2026-01-01T00:00:00Z",
    };
    vi.mocked(api.libraryGroups).mockResolvedValue([testingGroup]);
    vi.mocked(api.libraryGroupRemove).mockResolvedValue({ removed: true });
    renderPage();

    expect(await screen.findByText(/testing-rules/)).toBeInTheDocument();

    const deleteButton = screen.getByLabelText("delete group testing-rules");
    await userEvent.click(deleteButton);
    expect(api.libraryGroupRemove).not.toHaveBeenCalled();

    await userEvent.click(screen.getByLabelText("confirm delete group testing-rules"));
    expect(api.libraryGroupRemove).toHaveBeenCalledWith("testing-rules");
  });

  it("shows an empty state when there are no saved entries", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue([]);
    renderPage();

    expect(await screen.findByText("No saved instructions yet.")).toBeInTheDocument();
  });
});
