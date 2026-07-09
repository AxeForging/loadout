import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ExportPage } from "./Export";
import { api, type Artifact, type LibraryEntry, type LibraryGroup } from "../api";

afterEach(cleanup);

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof import("../api")>("../api");
  return {
    ...actual,
    api: {
      ...actual.api,
      artifacts: vi.fn(),
      profiles: vi.fn(),
      libraryInstructions: vi.fn(),
      libraryGroups: vi.fn(),
      bundleExportSelected: vi.fn(),
    },
  };
});

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ExportPage />
    </QueryClientProvider>,
  );
}

const sampleArtifacts: Artifact[] = [
  { id: "demo", kind: "skill", path: "skills/demo" },
  { id: "style", kind: "instruction", path: "instructions/style.md" },
];
const sampleEntries: LibraryEntry[] = [
  { id: "abc123", text: "always write tests first", createdAt: "2026-01-01T00:00:00Z" },
];
const sampleGroups: LibraryGroup[] = [
  { name: "testing-rules", entryIds: ["abc123"], createdAt: "2026-01-01T00:00:00Z" },
];

beforeEach(() => {
  vi.mocked(api.artifacts).mockResolvedValue(sampleArtifacts);
  vi.mocked(api.profiles).mockResolvedValue(["acme-backend"]);
  vi.mocked(api.libraryInstructions).mockResolvedValue(sampleEntries);
  vi.mocked(api.libraryGroups).mockResolvedValue(sampleGroups);
});

describe("ExportPage", () => {
  it("renders artifacts grouped by kind, profiles, and library entries", async () => {
    renderPage();

    expect(await screen.findByText("demo")).toBeInTheDocument();
    expect(screen.getByText("style")).toBeInTheDocument();
    expect(screen.getByText("skill")).toBeInTheDocument();
    expect(screen.getByText("instruction")).toBeInTheDocument();
    expect(screen.getByText("acme-backend")).toBeInTheDocument();
    expect(screen.getByText("always write tests first")).toBeInTheDocument();
  });

  it("the export button is disabled until something is selected", async () => {
    renderPage();
    await screen.findByText("demo");

    expect(screen.getByText("Export 0 items")).toBeDisabled();

    await userEvent.click(screen.getByText("demo"));
    expect(screen.getByText("Export 1 item")).not.toBeDisabled();
  });

  it("select all (per-kind) selects every artifact of that kind", async () => {
    renderPage();
    await screen.findByText("demo");

    const selectAllButtons = screen.getAllByText("select all");
    // First "select all" is the top-level one, per-kind ones follow.
    await userEvent.click(selectAllButtons[1]);

    expect(screen.getByText("Export 1 item")).toBeInTheDocument();
  });

  it("clicking a group chip selects every member entry", async () => {
    renderPage();
    await screen.findByText("testing-rules (1)");

    await userEvent.click(screen.getByText("testing-rules (1)"));

    expect(screen.getByText("Export 1 item")).toBeInTheDocument();
  });

  it("exports the current selection and triggers a download", async () => {
    const blob = new Blob(["fake archive bytes"], { type: "application/gzip" });
    vi.mocked(api.bundleExportSelected).mockResolvedValue(blob);
    const createObjectURL = vi.fn().mockReturnValue("blob:fake-url");
    const revokeObjectURL = vi.fn();
    vi.stubGlobal("URL", { ...URL, createObjectURL, revokeObjectURL });
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    renderPage();
    await screen.findByText("demo");
    await userEvent.click(screen.getByText("demo"));
    await userEvent.click(screen.getByText("acme-backend"));

    await userEvent.click(screen.getByText("Export 2 items"));

    expect(api.bundleExportSelected).toHaveBeenCalledWith({
      artifactIds: ["demo"],
      profileNames: ["acme-backend"],
      libraryEntryIds: [],
      sources: false,
    });
    expect(await screen.findByText("■ downloaded")).toBeInTheDocument();
    expect(createObjectURL).toHaveBeenCalledWith(blob);
    expect(clickSpy).toHaveBeenCalled();

    clickSpy.mockRestore();
    vi.unstubAllGlobals();
  });

  it("includes sources when the checkbox is checked", async () => {
    vi.mocked(api.bundleExportSelected).mockResolvedValue(new Blob());
    vi.stubGlobal("URL", { ...URL, createObjectURL: vi.fn().mockReturnValue("blob:x"), revokeObjectURL: vi.fn() });
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    renderPage();
    await screen.findByText("demo");
    await userEvent.click(screen.getByText("demo"));
    await userEvent.click(screen.getByLabelText(/Include tracked sources/));
    await userEvent.click(screen.getByText("Export 1 item"));

    expect(api.bundleExportSelected).toHaveBeenCalledWith(
      expect.objectContaining({ sources: true }),
    );

    clickSpy.mockRestore();
    vi.unstubAllGlobals();
  });
});
