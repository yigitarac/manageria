import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StatsPanel } from "./StatsPanel";
import { syntheticDump } from "./fixtures";

describe("StatsPanel", () => {
  it("renders key stats for both sides", () => {
    render(<StatsPanel dump={syntheticDump} />);

    expect(screen.getByTestId("stat-home-Possession")).toHaveTextContent("54%");
    expect(screen.getByTestId("stat-away-Shots")).toHaveTextContent("10");
    expect(screen.getByTestId("stat-home-xG")).toHaveTextContent("1.42");
    expect(screen.getByTestId("stat-away-Turnovers")).toHaveTextContent("221");
  });

  it("shows pattern attribution chips only for sides that used patterns", () => {
    render(<StatsPanel dump={syntheticDump} />);

    expect(screen.getByTestId("pattern-chip-home")).toHaveTextContent(
      /Redvale FC: 3 pattern shots/,
    );
    expect(screen.queryByTestId("pattern-chip-away")).not.toBeInTheDocument();
  });

  it("shows only known live statistics before full time", () => {
    render(<StatsPanel dump={syntheticDump} tMs={0} />);
    expect(screen.getByTestId("stat-home-Shots")).toHaveTextContent("0");
    expect(screen.queryByTestId("stat-home-xG")).not.toBeInTheDocument();
  });
});
