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
          prompt: "Namespace prefix for generated commands/skills. Clear it for bare names.",
          type: "text",
          default: "ld",
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

function planGroup() {
  return screen.getByRole("group", { name: /name for the plan command/i });
}

function planNameField() {
  return within(planGroup()).getByRole("textbox");
}

async function findPlanPreview(text: string) {
  const group = await screen.findByRole("group", { name: /name for the plan command/i });
  return within(group).findByText(text);
}

describe("BuilderPage name previews", () => {
  it("is safe by default: an untouched form previews the ld- prefixed name, no shadow warning", async () => {
    renderPage();

    expect(await findPlanPreview("/ld-plan")).toBeInTheDocument();
    expect(screen.queryByText(/shadows the/)).not.toBeInTheDocument();
    // The prefix field itself shows its effect too.
    const prefixGroup = screen.getByRole("group", { name: /namespace prefix/i });
    expect(within(prefixGroup).getByText("/ld-spec")).toBeInTheDocument();
  });

  it("clearing the prefix reveals the bare name and its built-in shadow warning", async () => {
    const user = userEvent.setup();
    renderPage();

    const prefixGroup = await screen.findByRole("group", { name: /namespace prefix/i });
    await user.clear(within(prefixGroup).getByRole("textbox"));

    expect(within(planGroup()).getByText("/plan")).toBeInTheDocument();
    expect(screen.getByText(/shadows the \/plan built-in/)).toBeInTheDocument();
  });

  it("applies a custom prefix to names left at their default", async () => {
    const user = userEvent.setup();
    renderPage();

    const prefixGroup = await screen.findByRole("group", { name: /namespace prefix/i });
    const prefixField = within(prefixGroup).getByRole("textbox");
    await user.clear(prefixField);
    await user.type(prefixField, "af");

    expect(within(planGroup()).getByText("/af-plan")).toBeInTheDocument();
    expect(screen.queryByText(/shadows the/)).not.toBeInTheDocument();
  });

  it("lets an explicit rename win verbatim (no prefix) and flags invalid names", async () => {
    const user = userEvent.setup();
    renderPage();

    await findPlanPreview("/ld-plan");
    const field = planNameField();
    await user.clear(field);
    await user.type(field, "blueprint");
    expect(within(planGroup()).getByText("/blueprint")).toBeInTheDocument();
    expect(screen.queryByText(/shadows the/)).not.toBeInTheDocument();

    await user.clear(field);
    await user.type(field, "Bad Name!");
    expect(within(planGroup()).getByText("/ld-plan")).toBeInTheDocument();
    expect(screen.getByText(/invalid name/)).toBeInTheDocument();
  });

  it("strips the redundant default parenthetical from namespaced prompts", async () => {
    renderPage();
    await findPlanPreview("/ld-plan");
    expect(screen.queryByText(/the namespace prefix applies unless/)).not.toBeInTheDocument();
  });
});
