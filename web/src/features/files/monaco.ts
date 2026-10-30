// Loads the vendored Monaco Editor (served by the dev page at /vendor/monaco,
// round 20) once, by URL: the PWA does not bundle a second copy.

type Monaco = {
  editor: {
    create: (el: HTMLElement, opts: Record<string, unknown>) => MonacoEditor;
    createModel: (text: string, lang: string) => unknown;
    defineTheme: (name: string, theme: Record<string, unknown>) => void;
    setTheme: (name: string) => void;
  };
};
export type MonacoEditor = {
  setModel: (m: unknown) => void;
  updateOptions: (o: Record<string, unknown>) => void;
  revealLinesInCenter: (a: number, b: number) => void;
  setSelection: (r: Record<string, number>) => void;
  onDidChangeCursorSelection: (cb: (e: { selection: { startLineNumber: number; endLineNumber: number } }) => void) => { dispose: () => void };
  dispose: () => void;
};

declare global {
  interface Window {
    monaco?: Monaco;
    require?: ((deps: string[], ok: () => void, fail: (e: unknown) => void) => void) & { config: (c: unknown) => void };
    MonacoEnvironment?: unknown;
  }
}

const base = "/vendor/monaco/vs";
let loading: Promise<Monaco> | null = null;

export function loadMonaco(): Promise<Monaco> {
  if (window.monaco) return Promise.resolve(window.monaco);
  if (loading) return loading;
  loading = new Promise<Monaco>((resolve, reject) => {
    const script = document.createElement("script");
    script.src = `${base}/loader.js`;
    script.onload = () => {
      const req = window.require;
      if (!req) return reject(new Error("monaco loader missing"));
      req.config({ paths: { vs: base } });
      window.MonacoEnvironment = {
        getWorkerUrl: () =>
          `data:text/javascript;charset=utf-8,${encodeURIComponent(
            `self.MonacoEnvironment={baseUrl:'${location.origin}${base}'};importScripts('${location.origin}${base}/base/worker/workerMain.js');`,
          )}`,
      };
      req(["vs/editor/editor.main"], () => (window.monaco ? resolve(window.monaco) : reject(new Error("monaco missing"))), reject);
    };
    script.onerror = () => reject(new Error("monaco loader failed"));
    document.head.appendChild(script);
  }).catch((e) => {
    loading = null;
    throw e;
  });
  return loading;
}

/** colony-dark and colony-light, from the design tokens (read at runtime, no raw colours here). */
export function defineThemes(monaco: Monaco): string {
  const css = getComputedStyle(document.documentElement);
  const v = (name: string) => css.getPropertyValue(`--color-${name}`).trim();
  const dark = document.documentElement.classList.contains("dark");
  const name = dark ? "colony-dark" : "colony-light";
  monaco.editor.defineTheme(name, {
    base: dark ? "vs-dark" : "vs",
    inherit: true,
    rules: [],
    colors: {
      "editor.background": v("surface"),
      "editor.foreground": v("fg"),
      "editorLineNumber.foreground": v("fg-subtle"),
      "editorLineNumber.activeForeground": v("fg-muted"),
      "editor.selectionBackground": v("accent-soft"),
      "editor.lineHighlightBackground": v("surface-sunken"),
      "editorGutter.background": v("surface"),
    },
  });
  return name;
}
