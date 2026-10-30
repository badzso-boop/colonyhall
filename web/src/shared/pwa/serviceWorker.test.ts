import { registerServiceWorker } from "./serviceWorker";

// A minimal fake of the registration API: enough to drive the update flow.
function fakeContainer(hasController: boolean) {
  const listeners: Record<string, () => void> = {};
  const regListeners: Record<string, () => void> = {};
  const workerListeners: Record<string, () => void> = {};
  const worker = {
    state: "installing",
    postMessage: vi.fn(),
    addEventListener: (t: string, f: () => void) => (workerListeners[t] = f),
  };
  const reg = {
    waiting: null,
    installing: worker,
    addEventListener: (t: string, f: () => void) => (regListeners[t] = f),
  };
  const container = {
    controller: hasController ? {} : null,
    register: vi.fn(async () => reg),
    addEventListener: (t: string, f: () => void) => (listeners[t] = f),
  };
  return { container, worker, fire: { updatefound: () => regListeners.updatefound?.(), state: () => workerListeners.statechange?.() } };
}

test("offers an update when a new worker is installed over a running one", async () => {
  const { container, worker, fire } = fakeContainer(true);
  const onUpdate = vi.fn();
  registerServiceWorker(onUpdate, container as unknown as ServiceWorkerContainer);
  await Promise.resolve();
  await Promise.resolve();
  const base = import.meta.env.BASE_URL; // "/app/" in the build
  expect(container.register).toHaveBeenCalledWith(`${base}sw.js`, { scope: base });
  fire.updatefound();
  worker.state = "installed";
  fire.state();
  expect(onUpdate).toHaveBeenCalledOnce();
  onUpdate.mock.calls[0]![0]();
  expect(worker.postMessage).toHaveBeenCalledWith("skip-waiting");
});

test("the first install is not an update", async () => {
  const { container, worker, fire } = fakeContainer(false);
  const onUpdate = vi.fn();
  registerServiceWorker(onUpdate, container as unknown as ServiceWorkerContainer);
  await Promise.resolve();
  await Promise.resolve();
  fire.updatefound();
  worker.state = "installed";
  fire.state();
  expect(onUpdate).not.toHaveBeenCalled();
});
