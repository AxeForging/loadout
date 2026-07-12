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
import { MCPPage } from "./MCP";
import { api, type MCPServer } from "../api";

afterEach(cleanup);

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof import("../api")>("../api");
  return {
    ...actual,
    api: {
      ...actual.api,
      mcpList: vi.fn(),
      mcpCreate: vi.fn(),
      mcpAgents: vi.fn(),
      mcpAdopt: vi.fn(),
    },
  };
});

const sampleServers: MCPServer[] = [
  {
    id: "context7",
    path: "mcps/context7",
    origin: "local",
    spec: {
      transport: "stdio",
      command: "npx",
      args: ["-y", "@upstash/context7-mcp"],
      env: { API_TOKEN: "${CONTEXT7_TOKEN}" },
      scope: "user",
    },
  },
  {
    id: "broken",
    path: "mcps/broken",
    origin: "local",
    error: "mcp.yaml: stdio transport requires command",
  },
];

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const rootRoute = createRootRoute({ component: MCPPage });
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
  vi.mocked(api.mcpList).mockResolvedValue(sampleServers);
  vi.mocked(api.mcpCreate).mockResolvedValue({ id: "github", path: "mcps/github" });
  vi.mocked(api.mcpAgents).mockResolvedValue([]);
  vi.mocked(api.mcpAdopt).mockResolvedValue({
    id: "my-github",
    path: "mcps/my-github",
    maskedKeys: ["GITHUB_TOKEN"],
  });
});

describe("MCPPage", () => {
  it("lists servers with their command line, and surfaces parse errors instead of hiding them", async () => {
    renderPage();

    expect(await screen.findByText("context7")).toBeInTheDocument();
    expect(screen.getByText("npx -y @upstash/context7-mcp")).toBeInTheDocument();
    // "stdio" appears as the form's transport chip AND the row's cell.
    expect(screen.getAllByText("stdio").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText(/requires command/)).toBeInTheDocument();
  });

  it("creates a stdio server from the form with parsed args and env", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("context7");

    await user.type(screen.getByPlaceholderText("context7"), "github");
    await user.type(screen.getByPlaceholderText("npx"), "npx");
    await user.type(
      screen.getByPlaceholderText("-y @upstash/context7-mcp"),
      "-y @modelcontextprotocol/server-github",
    );
    // user.type treats {braces} as key syntax — paste the ${VAR} placeholder instead.
    await user.click(screen.getByPlaceholderText("API_TOKEN=${CONTEXT7_TOKEN}"));
    await user.paste("GH_TOKEN=${GH_PAT}");
    await user.click(screen.getByRole("button", { name: "Add to store" }));

    expect(api.mcpCreate).toHaveBeenCalledWith({
      name: "github",
      transport: "stdio",
      scope: "user",
      command: "npx",
      args: ["-y", "@modelcontextprotocol/server-github"],
      env: { GH_TOKEN: "${GH_PAT}" },
    });
    expect(await screen.findByText(/run a sync to install/)).toBeInTheDocument();
  });

  it("lists unmanaged agent servers and adopts one with masked secrets", async () => {
    const unmanaged = [
      {
        agent: "claude-code",
        name: "My_GitHub",
        storeName: "my-github",
        file: "/home/u/.claude.json",
        maskedKeys: ["GITHUB_TOKEN"],
        spec: {
          transport: "stdio" as const,
          command: "npx",
          args: ["-y", "@modelcontextprotocol/server-github"],
          env: { GITHUB_TOKEN: "${GITHUB_TOKEN}" },
          scope: "user" as const,
        },
      },
    ];
    vi.mocked(api.mcpAgents).mockResolvedValue(unmanaged);
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText("My_GitHub")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Adopt as my-github" }));

    expect(api.mcpAdopt).toHaveBeenCalledWith("claude-code", "My_GitHub");
    expect(await screen.findByText(/adopted/)).toBeInTheDocument();
    expect(screen.getByText("GITHUB_TOKEN")).toBeInTheDocument();
  });

  it("shows the backend's teaching error when a literal secret is rejected", async () => {
    vi.mocked(api.mcpCreate).mockRejectedValue(
      new Error('mcp.yaml: header "Authorization" looks like a literal secret; use a ${VAR} placeholder'),
    );
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("context7");

    await user.click(screen.getByRole("button", { name: "http" }));
    await user.type(screen.getByPlaceholderText("context7"), "leaky");
    await user.type(screen.getByPlaceholderText("https://mcp.example.com/v1"), "https://x.dev");
    await user.click(screen.getByRole("button", { name: "Add to store" }));

    expect(await screen.findByText(/literal secret/)).toBeInTheDocument();
  });
});
