import "@testing-library/jest-dom/vitest";
import "./lib/i18n";

// Router/scroll behaviour that jsdom does not implement (silent the noise).
if (typeof window !== "undefined") {
  window.scrollTo = () => {};
}
