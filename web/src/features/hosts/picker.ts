// The folder picker's selection logic (round 33, part 7). Paths are the
// guard's absolute paths; a root is one of the host's guard.yaml roots.

export type DirEntry = {
  name: string;
  is_git_repo?: boolean;
  has_children?: boolean;
  hidden?: boolean;
  denied?: boolean;
  heavy?: boolean;
};

export type PickMode = "single" | "multi" | "browse";
export type Access = "read" | "write";
export type Selection = Map<string, Access>;

export function joinPath(parent: string, name: string): string {
  return parent.endsWith("/") ? parent + name : `${parent}/${name}`;
}

/** Whether `ancestor` contains `path` (or is it). */
export function within(path: string, ancestor: string): boolean {
  const a = ancestor.endsWith("/") ? ancestor.slice(0, -1) : ancestor;
  return path === a || path.startsWith(`${a}/`);
}

/** The root a path is under, if any (the longest match). */
export function rootOf(path: string, roots: string[]): string | undefined {
  return roots.filter((r) => within(path, r)).sort((a, b) => b.length - a.length)[0];
}

/** Breadcrumb from the root down to `path`: [{label, path}]. */
export function breadcrumb(root: string, path: string): { label: string; path: string }[] {
  const out = [{ label: root, path: root }];
  if (!within(path, root) || path === root) return out;
  let cur = root;
  for (const part of path.slice(root.replace(/\/$/, "").length + 1).split("/").filter(Boolean)) {
    cur = joinPath(cur, part);
    out.push({ label: part, path: cur });
  }
  return out;
}

export function selectable(e: DirEntry, mode: PickMode, gitOnly = false): boolean {
  if (mode === "browse" || e.denied) return false;
  return !gitOnly || Boolean(e.is_git_repo);
}

/** Folders first as the guard sends them; hidden ones only on request; a filter over the names. */
export function visibleEntries(entries: DirEntry[], showHidden: boolean, filter: string): DirEntry[] {
  const q = filter.trim().toLowerCase();
  return entries.filter((e) => (showHidden || !e.hidden) && (!q || e.name.toLowerCase().includes(q)));
}

/** Multi mode: a click adds a folder for reading, or removes it. */
export function toggle(sel: Selection, path: string): Selection {
  const next = new Map(sel);
  if (next.has(path)) next.delete(path);
  else next.set(path, "read");
  return next;
}

export function setAccess(sel: Selection, path: string, access: Access): Selection {
  if (!sel.has(path)) return sel;
  return new Map(sel).set(path, access);
}

/** Selected folders under another selected folder whose access already covers them. */
export function covered(sel: Selection): { path: string; by: string }[] {
  const out: { path: string; by: string }[] = [];
  for (const [path, access] of sel) {
    for (const [other, otherAccess] of sel) {
      if (other === path || !within(path, other)) continue;
      if (otherAccess === "write" || access === "read") {
        out.push({ path, by: other });
        break;
      }
    }
  }
  return out.sort((a, b) => a.path.localeCompare(b.path));
}

export function summary(sel: Selection): { total: number; read: number; write: number } {
  const v = [...sel.values()];
  return { total: v.length, read: v.filter((a) => a === "read").length, write: v.filter((a) => a === "write").length };
}
