// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

/** A connector's short name: 2–16 characters, lower-case letters, digits and _, starting with a letter, without "__". */
export function validSlug(s: string): boolean {
  return /^[a-z][a-z0-9_]{1,15}$/.test(s) && !s.includes("__");
}

/** Turns a title into a suggested short name. */
export function slugFrom(title: string): string {
  const s = title
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/_+/g, "_")
    .replace(/^[^a-z]+/, "")
    .replace(/_$/, "")
    .slice(0, 16)
    .replace(/_$/, "");
  return s;
}

/** Whether a URL is acceptable as a connector address (the server decides what it may reach). */
export function validURL(s: string): boolean {
  try {
    const u = new URL(s);
    return (
      (u.protocol === "https:" || u.protocol === "http:") &&
      u.host !== "" &&
      !u.username &&
      s.length <= 500
    );
  } catch {
    return false;
  }
}
