import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CommentaryFeed } from "./CommentaryFeed";
import { syntheticDump } from "./fixtures";

describe("CommentaryFeed", () => {
  it("lists only revealed events and resolves player names", () => {
    render(<CommentaryFeed dump={syntheticDump} tMs={0} />);

    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(1);
    expect(items[0]).toHaveTextContent("Kick-off");
    expect(screen.queryByText("Shot saved")).not.toBeInTheDocument();
  });

  it("marks spoken events and shows pattern attribution", () => {
    render(<CommentaryFeed dump={syntheticDump} tMs={45 * 60_000} />);

    const spoken = screen
      .getAllByRole("listitem")
      .filter((li) => li.getAttribute("aria-current") === "true");
    expect(spoken.length).toBe(2);
    expect(screen.getByTestId("commentary-pattern-chip")).toHaveTextContent("six_yard_darts");
    expect(spoken.find((li) => li.textContent?.includes("Miro Thane"))).toBeTruthy();
  });
});
