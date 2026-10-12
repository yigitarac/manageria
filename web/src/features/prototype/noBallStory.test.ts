import { describe, expect, it } from "vitest";
// Raw source strings: the prototype renderer must not borrow legacy ballStory
// elastic retiming. Reading the sources keeps this guard next to the code.
import interpolateSrc from "./interpolate.ts?raw";
import canvasSrc from "./PrototypeCanvas.tsx?raw";
import pageSrc from "../../pages/PrototypePage.tsx?raw";

describe("same-clock prototype independence", () => {
  it("never imports or references ballStory retiming", () => {
    for (const src of [interpolateSrc, canvasSrc, pageSrc]) {
      expect(src).not.toMatch(/(?:import|from)\s*[^;\n]*ballStory/);
    }
  });

  it("interpolates only from emitted knots (no physics/retiming helpers)", () => {
    expect(interpolateSrc).toMatch(/interpolateTrace/);
    expect(interpolateSrc).not.toMatch(/requestAnimationFrame|performance\.now/);
  });
});
