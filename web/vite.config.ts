import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The dev server proxies API calls to the Go server (see deploy/docker-compose.yml docs).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: {
      "/healthz": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    environmentOptions: {
      jsdom: {
        // Absolute URLs (Request/fetch) need a document origin in tests too.
        url: "http://localhost:5173",
      },
    },
    setupFiles: ["./src/test-setup.ts"],
  },
});
