import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// In development the UI runs on the Vite dev server and the API on the Go
// server; requests to the API are proxied so the browser sees one origin, as
// in production. Override the target with C4FORGE_DEV_API_URL.
const apiTarget = process.env.C4FORGE_DEV_API_URL ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": { target: apiTarget, ws: true },
      "/healthz": apiTarget,
      "/readyz": apiTarget,
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test-setup.ts"],
    unstubGlobals: true,
  },
});
