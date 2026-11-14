import { defineConfig } from "vitest/config";

// Separate from vite.config.ts because that one sets root: "frontend" for dev-serving - tests
// live alongside every gem (a sibling of frontend/), so this config keeps the repo root as root.
export default defineConfig({
  test: {
    environment: "jsdom",
    include: ["gems/**/*.test.ts", "frontend/src/**/*.test.ts"],
  },
});
