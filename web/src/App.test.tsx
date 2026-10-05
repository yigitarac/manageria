import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import App from "./App";

function renderApp() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>,
  );
}

function stubJsonResponse(body: string, status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(body, { status, headers: { "content-type": "application/json" } }),
    ),
  );
}

describe("App", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows a healthy status when the API responds", async () => {
    stubJsonResponse('{"status":"ok"}');

    renderApp();

    await screen.findByText("Healthy");
    expect(screen.getByRole("status")).toHaveTextContent("Healthy");
  });

  it("shows an error status when the API is unreachable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new TypeError("network down");
      }),
    );

    renderApp();

    await screen.findByText("Unreachable");
    expect(screen.getByRole("status")).toHaveTextContent("Unreachable");
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });
});