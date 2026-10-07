import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import HomePage from "./HomePage";

async function renderHome() {
  const root = createRootRoute();
  const home = createRoute({ getParentRoute: () => root, path: "/", component: HomePage });
  const viewer = createRoute({
    getParentRoute: () => root,
    path: "/viewer",
    component: () => <p>viewer stub</p>,
  });
  const router = createRouter({
    routeTree: root.addChildren([home, viewer]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  await screen.findByText("Healthy");
}

describe("HomePage", () => {
  it("shows branding, API health and a link to the viewer", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response('{"status":"ok"}', {
            status: 200,
            headers: { "content-type": "application/json" },
          }),
      ),
    );

    await renderHome();

    expect(screen.getByRole("heading", { name: "Manageria" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Healthy");
    expect(screen.getByRole("link", { name: /match viewer/i })).toHaveAttribute("href", "/viewer");
  });
});
