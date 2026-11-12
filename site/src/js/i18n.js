// Strings for the scripts. The build puts the page's language (only the keys
// the scripts and data files use) into <script id="site-data">; no fetch.
const blob = JSON.parse(document.getElementById("site-data")?.textContent || "{}");

export const lang = blob.lang || document.documentElement.lang || "en";
export const data = blob.data || {};

/** The string for key, or the key itself if it is missing (the build checks it is not). */
export function t(key) {
  return blob.t?.[key] ?? key;
}

/** Resolves "@key" references in data files to strings. */
export function text(value) {
  return typeof value === "string" && value.startsWith("@") ? t(value.slice(1)) : value;
}
