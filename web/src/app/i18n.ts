import type { Resource } from "i18next";
import { initI18n, languages } from "@/shared/i18n";

// Every namespace: "common" from shared/i18n/locales, and one per feature
// from features/<name>/locales (docs/08-frontend-architektura.md, 7.).
const common = import.meta.glob<Record<string, unknown>>("../shared/i18n/locales/*.json", {
  import: "default",
  eager: true,
});
const layout = import.meta.glob<Record<string, unknown>>("../layout/locales/*.json", {
  import: "default",
  eager: true,
});
const features = import.meta.glob<Record<string, unknown>>("../features/*/locales/*.json", {
  import: "default",
  eager: true,
});

export function collectResources(): Resource {
  const out: Resource = Object.fromEntries(languages.map((l) => [l, {}]));
  const add = (ns: string, path: string, data: Record<string, unknown>) => {
    const lang = path.slice(path.lastIndexOf("/") + 1, -".json".length);
    const bucket = out[lang];
    if (!bucket) throw new Error(`unknown language file: ${path}`);
    bucket[ns] = data;
  };
  for (const [path, data] of Object.entries(common)) add("common", path, data);
  for (const [path, data] of Object.entries(layout)) add("layout", path, data);
  for (const [path, data] of Object.entries(features)) {
    const ns = path.split("/")[2];
    if (!ns) throw new Error(`unexpected locale path: ${path}`);
    add(ns, path, data);
  }
  return out;
}

export const i18n = initI18n(collectResources());
