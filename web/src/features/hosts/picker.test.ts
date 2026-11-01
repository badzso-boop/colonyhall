import { breadcrumb, covered, rootOf, selectable, setAccess, summary, toggle, visibleEntries, within, type Selection } from "./picker";

test("paths: within, the root, the breadcrumb", () => {
  expect(within("/home/n/proj/a", "/home/n/proj")).toBe(true);
  expect(within("/home/n/project", "/home/n/proj")).toBe(false);
  expect(rootOf("/srv/data/x", ["/srv", "/srv/data", "/home"])).toBe("/srv/data");
  expect(rootOf("/etc", ["/srv"])).toBeUndefined();
  expect(breadcrumb("/home/n", "/home/n/proj/web")).toEqual([
    { label: "/home/n", path: "/home/n" },
    { label: "proj", path: "/home/n/proj" },
    { label: "web", path: "/home/n/proj/web" },
  ]);
});

test("what can be chosen in each mode", () => {
  const repo = { name: "colonyhall", is_git_repo: true };
  const plain = { name: "notes" };
  const secret = { name: ".ssh", denied: true, hidden: true };
  expect([selectable(repo, "single"), selectable(plain, "single"), selectable(secret, "multi")]).toEqual([true, true, false]);
  expect([selectable(repo, "single", true), selectable(plain, "single", true)]).toEqual([true, false]);
  expect(selectable(repo, "browse")).toBe(false);
});

test("hidden folders only on request, and the filter", () => {
  const list = [{ name: "src" }, { name: ".cache", hidden: true }, { name: "Scripts" }];
  expect(visibleEntries(list, false, "").map((e) => e.name)).toEqual(["src", "Scripts"]);
  expect(visibleEntries(list, true, "SC").map((e) => e.name)).toEqual(["Scripts"]);
});

test("multi: toggle, access, the tray's summary and the nested warning", () => {
  let sel: Selection = new Map([["/p", "read"]]); // prefilled (a copy of an agent)
  sel = toggle(sel, "/p/web");
  sel = toggle(sel, "/q");
  sel = setAccess(sel, "/q", "write");
  expect(summary(sel)).toEqual({ total: 3, read: 2, write: 1 });
  // A read child of a read parent is covered; a write child of a read parent is not.
  expect(covered(sel)).toEqual([{ path: "/p/web", by: "/p" }]);
  sel = setAccess(sel, "/p/web", "write");
  expect(covered(sel)).toEqual([]);
  sel = setAccess(sel, "/p", "write");
  expect(covered(sel)).toEqual([{ path: "/p/web", by: "/p" }]);
  sel = toggle(sel, "/p/web");
  expect([...sel.keys()]).toEqual(["/p", "/q"]);
  expect(setAccess(sel, "/nope", "write")).toBe(sel);
});
