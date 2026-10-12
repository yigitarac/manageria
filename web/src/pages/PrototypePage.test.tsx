import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import exportedFixture from "../features/prototype/exported-fixture.json";
import { PrototypePage } from "./PrototypePage";

function mockFetchOk() {
  const calls: string[] = [];
  const fetchMock = vi.fn((input: RequestInfo | URL) => {
    calls.push(String(input));
    return Promise.resolve({
      ok: true,
      json: () => Promise.resolve(exportedFixture),
    } as Response);
  });
  vi.stubGlobal("fetch", fetchMock);
  return { fetchMock, calls };
}

let rafQueue: FrameRequestCallback[] = [];

beforeEach(() => {
  rafQueue = [];
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
    rafQueue.push(cb);
    return rafQueue.length;
  });
  vi.stubGlobal("cancelAnimationFrame", () => {
    rafQueue = [];
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

/** Runs the queued rAF callbacks once at a given timestamp. */
function stepFrame(now: number) {
  const callbacks = rafQueue;
  rafQueue = [];
  act(() => {
    callbacks.forEach((cb) => cb(now));
  });
}

describe("PrototypePage", () => {
  it("loads a real exported fixture and renders the authoritative canvas and events", async () => {
    mockFetchOk();
    render(<PrototypePage />);
    await waitFor(() => expect(screen.getByTestId("prototype-canvas")).toBeInTheDocument());
    expect(screen.getByTestId("runner-0")).toBeInTheDocument();
    expect(screen.getAllByText(/control @/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/release @/).length).toBeGreaterThan(0);
  });

  it("shows a graceful bad-file notice for a malformed fetch and never throws", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ schemaVersion: 999, points: [{}], events: [] }),
        } as Response),
      ),
    );
    render(<PrototypePage />);
    await waitFor(() => expect(screen.getByText(/not a prototype trace/i)).toBeInTheDocument());
    expect(screen.queryByTestId("prototype-canvas")).not.toBeInTheDocument();
  });

  it("supports pause, scrub, playback stepping and changing the clip", async () => {
    const { fetchMock, calls } = mockFetchOk();
    render(<PrototypePage />);
    await waitFor(() => expect(screen.getByTestId("prototype-canvas")).toBeInTheDocument());

    // Pause toggles the control label.
    fireEvent.click(screen.getByRole("button", { name: "Pause" }));
    expect(screen.getByRole("button", { name: "Play" })).toBeInTheDocument();

    // Scrub updates the displayed time.
    const scrub = screen.getByLabelText("Timeline") as HTMLInputElement;
    fireEvent.change(scrub, { target: { value: "200" } });
    expect(screen.getByText("200 ms")).toBeInTheDocument();

    // Resume and step frames: the clock advances past the scrubbed value.
    fireEvent.click(screen.getByRole("button", { name: "Play" }));
    const base = performance.now();
    stepFrame(base + 1000);
    stepFrame(base + 1016);
    expect(screen.queryByText("200 ms")).not.toBeInTheDocument();

    // Change clip issues a second fetch for the other slug.
    fireEvent.click(screen.getByRole("button", { name: "Reachable interception" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(calls[1]).toContain("reachable-interception");
  });
});
