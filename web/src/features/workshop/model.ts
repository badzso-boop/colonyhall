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

/** A starting manifest for a new extension (one code tool). */
export const manifestTemplate = `{
  "egress": [],
  "store": [],
  "tools": [
    {
      "name": "hello",
      "kind": "js",
      "risk": "read",
      "description": "Greets someone",
      "input": { "type": "object", "properties": { "name": { "type": "string" } } }
    }
  ]
}`;

/** A starting source for the template's tool. */
export const sourceTemplate = `// Every exported function is a tool; ctx has fetch, store, colony, call and log.
export async function hello(input: { name?: string }, ctx: any) {
  ctx.log("hello", input.name);
  return "Hello " + (input.name ?? "world");
}
`;

/** Parses a JSON object from a text field; null when it is not one. */
export function parseObject(text: string): Record<string, unknown> | null {
  try {
    const v: unknown = JSON.parse(text);
    return v && typeof v === "object" && !Array.isArray(v)
      ? (v as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

/** Whether a diff adds anything a person should look at. */
export function diffIsEmpty(d: {
  new_connectors?: string[];
  new_egress?: string[];
  new_store?: string[];
  new_tools?: string[];
  new_routines?: string[];
  raised_risks?: Record<string, string>;
  code_changed: boolean;
}): boolean {
  return (
    !d.code_changed &&
    !d.new_connectors?.length &&
    !d.new_egress?.length &&
    !d.new_store?.length &&
    !d.new_tools?.length &&
    !d.new_routines?.length &&
    !Object.keys(d.raised_risks ?? {}).length
  );
}
