import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The PWA is served by the Go server at /app/ (internal/httpserver/app.go).
// `npm run dev` proxies the API and the WebSocket to `make dev` / `make run-mem`.
export default defineConfig({
  base: "/app/",
  plugins: [react(), tailwindcss()],
  resolve: {
    // "@/…" is src/ (docs/08-frontend-architektura.md, 11.); also in tsconfig.json "paths".
    alias: { "@": decodeURIComponent(new URL("./src", import.meta.url).pathname) },
  },
  build: {
    outDir: "dist",
    assetsDir: "assets",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/ws": { target: "ws://localhost:8080", ws: true },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: ["src/app/test-setup.ts"],
  },
});
