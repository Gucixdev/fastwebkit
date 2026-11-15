import { defineConfig } from "vite";

// In dev, Vite serves the frontend and proxies API calls to the Go backend (`go run
// ./cmd/fastwebapp`, default :8080) so there's no CORS setup to hand-roll. In prod the built
// frontend is static output the Go binary can serve directly - not wired up yet, separate step.
export default defineConfig({
  // index.html/src live in frontend/, but package.json + node_modules sit at the repo root so
  // the web/ folder (a sibling of frontend/, not a child) can resolve "three"/"kiwi.js" too -
  // Node-style resolution only walks up through a file's own ancestors.
  root: "frontend",
  server: {
    fs: { allow: [".."] },
    proxy: {
      "/items": "http://localhost:8080",
      "/healthz": "http://localhost:8080",
      "/webhooks": "http://localhost:8080",
      "/register": "http://localhost:8080",
      "/login": "http://localhost:8080",
      "/logout": "http://localhost:8080",
      "/downloads": "http://localhost:8080",
    },
  },
});
