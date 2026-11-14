// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Colonyhall service worker: caches the app shell only (docs/07, PWA basics).
// Never caches the API (/api/), the WebSocket (/ws/) or anything outside
// /app/. BUILD is filled in at build time (vite.config.ts), so every deploy
// is a new worker and the app can offer "Új verzió érhető el".
const BUILD = "__BUILD_ID__";
const CACHE = `colonyhall-shell-${BUILD}`;
const SHELL = "/app/";

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE).then((c) => c.add(SHELL)));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k.startsWith("colonyhall-shell-") && k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

// The page asks the waiting worker to take over when the user clicks "Frissítés".
self.addEventListener("message", (event) => {
  if (event.data === "skip-waiting") self.skipWaiting();
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin || !url.pathname.startsWith("/app/")) return;

  // Hashed build files never change: cache first.
  if (url.pathname.startsWith("/app/assets/")) {
    event.respondWith(
      caches.match(req).then(
        (hit) =>
          hit ||
          fetch(req).then((res) => {
            if (res.ok) {
              const copy = res.clone();
              caches.open(CACHE).then((c) => c.put(req, copy));
            }
            return res;
          }),
      ),
    );
    return;
  }

  // Page loads: the network first (a deploy shows up), the cached shell offline.
  if (req.mode === "navigate") {
    event.respondWith(
      fetch(req).catch(() => caches.match(SHELL).then((hit) => hit || new Response("Offline", { status: 503 }))),
    );
  }
});
