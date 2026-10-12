import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PrototypeCanvas } from "./PrototypeCanvas";
import { interpolateTrace } from "./interpolate";
import { PROTOTYPE_FIXTURE } from "./fixture";

describe("PrototypeCanvas", () => {
  it("draws the authoritative runners and ball at the queried time", () => {
    const state = interpolateTrace(PROTOTYPE_FIXTURE, 28);
    if (!state) throw new Error("interpolateTrace returned null");
    render(<PrototypeCanvas trace={PROTOTYPE_FIXTURE} state={state} label="Test pitch" />);

    expect(screen.getByTestId("prototype-canvas")).toBeInTheDocument();
    expect(screen.getByTestId("prototype-ball")).toBeInTheDocument();
    expect(screen.getByTestId("runner-0")).toBeInTheDocument();
    expect(screen.getByTestId("runner-1")).toBeInTheDocument();

    const ball = screen.getByTestId("prototype-ball");
    expect(Number(ball.getAttribute("cx"))).toBeCloseTo(0.42 * PROTOTYPE_FIXTURE.pitch.lengthM, 6);
    expect(Number(ball.getAttribute("cy"))).toBeCloseTo(0.5 * PROTOTYPE_FIXTURE.pitch.widthM, 6);
  });
});
