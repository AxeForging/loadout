import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router";
import "./index.css";
import { Layout } from "./components/Layout";
import { Dashboard } from "./pages/Dashboard";
import { StorePage } from "./pages/Store";
import { StoreDetail } from "./pages/StoreDetail";
import { AgentsPage } from "./pages/Agents";
import { SourcesPage } from "./pages/Sources";
import { MCPPage } from "./pages/MCP";
import { BuilderPage } from "./pages/Builder";
import { LibraryPage } from "./pages/Library";
import { ExportPage } from "./pages/Export";

const rootRoute = createRootRoute({ component: Layout });

const routes = [
  createRoute({ getParentRoute: () => rootRoute, path: "/", component: Dashboard }),
  createRoute({ getParentRoute: () => rootRoute, path: "/store", component: StorePage }),
  createRoute({ getParentRoute: () => rootRoute, path: "/store/$artifactId", component: StoreDetail }),
  createRoute({ getParentRoute: () => rootRoute, path: "/agents", component: AgentsPage }),
  createRoute({ getParentRoute: () => rootRoute, path: "/sources", component: SourcesPage }),
  createRoute({ getParentRoute: () => rootRoute, path: "/mcp", component: MCPPage }),
  createRoute({ getParentRoute: () => rootRoute, path: "/builder", component: BuilderPage }),
  createRoute({ getParentRoute: () => rootRoute, path: "/library", component: LibraryPage }),
  createRoute({ getParentRoute: () => rootRoute, path: "/export", component: ExportPage }),
];

const router = createRouter({ routeTree: rootRoute.addChildren(routes) });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
