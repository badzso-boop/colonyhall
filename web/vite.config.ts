import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { defineConfig, type Plugin } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The PWA is served by the Go server at /app/ (internal/httpserver/app.go).
// `npm run dev` proxies the API and the WebSocket to `make dev` / `make run-mem`.
// Emits sw.js with a build id derived from the bundle, so each deploy
// installs a new service worker (web/sw/sw.js).
function serviceWorker(): Plugin {
  return {
    name: "colonyhall-sw",
    apply: "build",
    generateBundle(_options, bundle) {
      const id = createHash("sha256").update(Object.keys(bundle).sort().join("\n")).digest("hex").slice(0, 12);
      const source = readFileSync(new URL("./sw/sw.js", import.meta.url), "utf8").replace('"__BUILD_ID__"', JSON.stringify(id));
      if (source.includes("__BUILD_ID__")) throw new Error("sw.js: build id placeholder not replaced");
      this.emitFile({ type: "asset", fileName: "sw.js", source });
    },
  };
}

export default defineConfig({
  base: "/app/",
  plugins: [react(), tailwindcss(), serviceWorker()],
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
    // The persistent transform cache keeps stale import.meta.glob results
    // (a new feature's locales/*.json would be missing from the tests).
    fsModuleCache: false,
    environment: "jsdom",
    // The calendar's tests need a zone with a clock change (Europe/Budapest).
    env: { TZ: "Europe/Budapest" },
    globals: true,
    include: ["src/**/*.test.{ts,tsx,mjs}"],
    setupFiles: ["src/app/test-setup.ts"],
  },
});
