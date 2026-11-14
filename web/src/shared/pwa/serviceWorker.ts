// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

/**
 * Registers the service worker (production builds only) and calls onUpdate
 * when a new version is installed and waiting. The page never reloads by
 * itself: activate() is called from the "Frissítés" toast action.
 */
export function registerServiceWorker(onUpdate: (activate: () => void) => void, sw: ServiceWorkerContainer | undefined = navigator.serviceWorker): void {
  if (!sw) return;
  let reloading = false;
  sw.addEventListener("controllerchange", () => {
    if (reloading) return;
    reloading = true;
    window.location.reload();
  });
  void sw.register(`${import.meta.env.BASE_URL}sw.js`, { scope: import.meta.env.BASE_URL }).then((reg) => {
    const offer = (worker: ServiceWorker) => onUpdate(() => worker.postMessage("skip-waiting"));
    // A worker already waiting from an earlier visit.
    if (reg.waiting && sw.controller) offer(reg.waiting);
    reg.addEventListener("updatefound", () => {
      const worker = reg.installing;
      if (!worker) return;
      worker.addEventListener("statechange", () => {
        // "installed" with an existing controller = an update, not the first install.
        if (worker.state === "installed" && sw.controller) offer(worker);
      });
    });
  });
}
