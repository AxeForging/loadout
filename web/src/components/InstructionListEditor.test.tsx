import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { InstructionListEditor } from "./InstructionListEditor";
import { api, type LibraryEntry } from "../api";

// This suite renders several instances of the same component; the setup
// file does not register global auto-cleanup (no `globals: true`), so each
// render must be unmounted explicitly or later tests see stale DOM from
// earlier ones.
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
    },
  };
});

function renderEditor(value: string[], onChange: (v: string[]) => void) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <InstructionListEditor value={value} onChange={onChange} />
    </QueryClientProvider>,
  );
}

const sampleLibrary: LibraryEntry[] = [
  { id: "abc123", text: "no coverage theater", createdAt: "2026-01-01T00:00:00Z" },
];

describe("InstructionListEditor", () => {
  it("adds a row when Enter is pressed and clears the draft", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue([]);
    const onChange = vi.fn();
    renderEditor([], onChange);

    const input = screen.getByPlaceholderText("type an instruction, press Enter");
    await userEvent.type(input, "always write tests first{Enter}");

    expect(onChange).toHaveBeenCalledWith(["always write tests first"]);
    expect(input).toHaveValue("");
  });

  it("removes a row when its delete control is clicked", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue([]);
    const onChange = vi.fn();
    renderEditor(["always write tests first", "no coverage theater"], onChange);

    await userEvent.click(screen.getByLabelText("remove always write tests first"));

    expect(onChange).toHaveBeenCalledWith(["no coverage theater"]);
  });

  it("saves a row to the library", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue([]);
    vi.mocked(api.libraryAdd).mockResolvedValue({
      id: "xyz789",
      text: "always write tests first",
      createdAt: "2026-01-01T00:00:00Z",
    });
    renderEditor(["always write tests first"], vi.fn());

    await userEvent.click(screen.getByText("save"));

    expect(api.libraryAdd).toHaveBeenCalledWith("always write tests first");
  });

  it("adds a library chip's text to the list on click", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue(sampleLibrary);
    const onChange = vi.fn();
    renderEditor([], onChange);

    expect(await screen.findByText("no coverage theater")).toBeInTheDocument();
    await userEvent.click(screen.getByText("no coverage theater"));

    expect(onChange).toHaveBeenCalledWith(["no coverage theater"]);
  });

  it("removes a library entry when its delete control is clicked", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue(sampleLibrary);
    vi.mocked(api.libraryRemove).mockResolvedValue({ removed: true });
    renderEditor([], vi.fn());

    await userEvent.click(await screen.findByLabelText("delete no coverage theater from library"));

    expect(api.libraryRemove).toHaveBeenCalledWith("abc123");
  });

  it("edits a row in place: click to edit, Enter to commit", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue([]);
    const onChange = vi.fn();
    renderEditor(["always write tests first"], onChange);

    await userEvent.click(screen.getByText("always write tests first"));
    const editInput = screen.getByDisplayValue("always write tests first");
    await userEvent.clear(editInput);
    await userEvent.type(editInput, "no coverage theater{Enter}");

    expect(onChange).toHaveBeenCalledWith(["no coverage theater"]);
  });

  it("cancels an edit with Escape, leaving the value unchanged", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue([]);
    const onChange = vi.fn();
    renderEditor(["always write tests first"], onChange);

    await userEvent.click(screen.getByText("always write tests first"));
    const editInput = screen.getByDisplayValue("always write tests first");
    await userEvent.type(editInput, " more{Escape}");

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByText("always write tests first")).toBeInTheDocument();
  });

  it("removes the row when an edit is committed as empty text", async () => {
    vi.mocked(api.libraryInstructions).mockResolvedValue([]);
    const onChange = vi.fn();
    renderEditor(["always write tests first", "no coverage theater"], onChange);

    await userEvent.click(screen.getByText("always write tests first"));
    const editInput = screen.getByDisplayValue("always write tests first");
    await userEvent.clear(editInput);
    await userEvent.keyboard("{Enter}");

    expect(onChange).toHaveBeenCalledWith(["no coverage theater"]);
  });
});
