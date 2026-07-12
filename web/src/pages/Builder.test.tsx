import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BuilderPage } from "./Builder";
import { api, type WizardSchema } from "../api";

afterEach(cleanup);

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof import("../api")>("../api");
  return {
    ...actual,
    api: {
      ...actual.api,
      wizardSchema: vi.fn(),
      wizardPresets: vi.fn(),
      profiles: vi.fn(),
    },
  };
});

const schema: WizardSchema = {
  namespacePrefixQuestionId: "naming.prefix",
  reservedCommandNames: ["plan", "resume"],
  sections: [
    {
      id: "naming",
      title: "Naming",
      questions: [
        {
          id: "naming.prefix",
          prompt: "Optional namespace prefix for generated commands/skills. Leave empty for none.",
          type: "text",
          default: "",
        },
        {
          id: "sdd.planCommandName",
          prompt:
            "Name for the plan command (default: plan; the namespace prefix applies unless you set a custom name)",
          type: "text",
          default: "plan",
          namespaced: true,
        },
      ],
    },
  ],
};

beforeEach(() => {
  vi.mocked(api.wizardSchema).mockResolvedValue(schema);
  vi.mocked(api.wizardPresets).mockResolvedValue([]);
  vi.mocked(api.profiles).mockResolvedValue([]);
});

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <BuilderPage />
    </QueryClientProvider>,
  );
}

function planNameField() {
  const group = screen.getByRole("group", { name: /name for the plan command/i });
  return within(group).getByRole("textbox");
}

describe("BuilderPage name previews", () => {
  it("shows the resolved install name and warns when it shadows a built-in", async () => {
    renderPage();

    expect(await screen.findByText("/plan")).toBeInTheDocument();
    expect(screen.getByText(/shadows the \/plan built-in/)).toBeInTheDocument();
  });

  it("applies the namespace prefix to a name left at its default and clears the warning", async () => {
    const user = userEvent.setup();
    renderPage();

    const prefixGroup = await screen.findByRole("group", { name: /namespace prefix/i });
    await user.type(within(prefixGroup).getByRole("textbox"), "af");

    expect(screen.getByText("/af-plan")).toBeInTheDocument();
    expect(screen.queryByText(/shadows the/)).not.toBeInTheDocument();
  });

  it("lets an explicit rename win verbatim and flags invalid names", async () => {
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("/plan");
    const field = planNameField();
    await user.clear(field);
    await user.type(field, "blueprint");
    expect(screen.getByText("/blueprint")).toBeInTheDocument();
    expect(screen.queryByText(/shadows the/)).not.toBeInTheDocument();

    await user.clear(field);
    await user.type(field, "Bad Name!");
    expect(screen.getByText("/plan")).toBeInTheDocument();
    expect(screen.getByText(/invalid name/)).toBeInTheDocument();
  });

  it("strips the redundant default parenthetical from namespaced prompts", async () => {
    renderPage();
    await screen.findByText("/plan");
    expect(screen.queryByText(/the namespace prefix applies unless/)).not.toBeInTheDocument();
  });
});
