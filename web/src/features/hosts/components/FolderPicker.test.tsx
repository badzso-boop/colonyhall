// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { TooltipProvider } from "@/shared/ui";
import { FolderPicker, type FolderPickerProps } from "./FolderPicker";

const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });
const tree: Record<string, unknown[]> = {
  "": [{ name: "/home/n/projects", path: "/home/n/projects", has_children: true }],
  "/home/n/projects": [
    { name: "colonyhall", path: "/home/n/projects/colonyhall", is_git_repo: true, has_children: true },
    { name: "notes", path: "/home/n/projects/notes", has_children: false },
    { name: ".ssh", path: "/home/n/projects/.ssh", hidden: true, denied: true },
    { name: ".cache", path: "/home/n/projects/.cache", hidden: true },
  ],
  "/home/n/projects/colonyhall": [{ name: "web", path: "/home/n/projects/colonyhall/web", has_children: false }],
};

function setup(props: Partial<FolderPickerProps>, offline = false) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      if (offline) return json({ title: "Conflict", status: 409, detail: "host_offline" }, 409);
      const body = await req.json();
      return json({ op: "list_dirs", path: body.path ?? "", entries: tree[body.path ?? ""] ?? [] });
    }),
  );
  const onConfirm = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <TooltipProvider>
        <FolderPicker hostId="h1" open onOpenChange={() => {}} mode="single" onConfirm={onConfirm} hostLastSeen="2026-10-30T10:00:00Z" {...props} />
      </TooltipProvider>
    </QueryClientProvider>,
  );
  return onConfirm;
}

afterEach(() => vi.unstubAllGlobals());

test("single, git only: plain folders cannot be chosen; a repo can; Enter opens, Backspace goes up", async () => {
  const onConfirm = setup({ mode: "single", gitOnly: true });
  const list = await screen.findByRole("listbox", { name: "Mappák" });
  const notes = await within(list).findByRole("option", { name: /notes/ });
  expect(notes.getAttribute("aria-disabled")).toBe("true");
  expect(within(list).queryByText(".cache")).toBeNull(); // hidden by default
  fireEvent.click(notes);
  expect((screen.getByRole("button", { name: "Kiválasztás" }) as HTMLButtonElement).disabled).toBe(true);
  const repo = within(list).getByRole("option", { name: /colonyhall/ });
  fireEvent.click(repo);
  repo.focus();
  fireEvent.keyDown(repo, { key: "Enter" });
  expect(await within(list).findByRole("option", { name: /web/ })).toBeTruthy();
  fireEvent.keyDown(within(list).getByRole("option", { name: /web/ }), { key: "Backspace" });
  await within(list).findByRole("option", { name: /notes/ });
  fireEvent.click(screen.getByRole("button", { name: "Kiválasztás" }));
  expect([...onConfirm.mock.calls[0]![0].keys()]).toEqual(["/home/n/projects/colonyhall"]);
});

test("multi: read/write per folder, the tray's summary, the nested warning, a pre-marked selection; secrets are disabled", async () => {
  const onConfirm = setup({ mode: "multi", initial: new Map([["/home/n/projects/colonyhall", "write" as const]]) });
  const list = await screen.findByRole("listbox");
  await within(list).findByRole("option", { name: /notes/ });
  fireEvent.click(screen.getByRole("switch")); // show hidden
  const ssh = within(list).getByRole("option", { name: /\.ssh/ });
  expect(ssh.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(ssh);
  fireEvent.click(within(list).getByRole("option", { name: /notes/ }));
  expect(screen.getByText("2 mappa: 1 olvasás, 1 írás")).toBeTruthy();
  // Into colonyhall: web under a write parent is covered.
  fireEvent.click(within(list).getByRole("button", { name: "colonyhall megnyitása" }));
  fireEvent.click(await within(list).findByRole("option", { name: /web/ }));
  expect(screen.getByText(/web: a\(z\) \/home\/n\/projects\/colonyhall hozzáférése már lefedi/)).toBeTruthy();
  fireEvent.click(within(within(list).getByRole("option", { name: /web/ })).getByRole("radio", { name: "írás" }));
  fireEvent.click(screen.getByRole("button", { name: "Kiválasztás" }));
  expect(Object.fromEntries(onConfirm.mock.calls[0]![0])).toEqual({
    "/home/n/projects/colonyhall": "write",
    "/home/n/projects/notes": "read",
    "/home/n/projects/colonyhall/web": "write",
  });
});

test("offline: an empty state with the last-seen time and a retry; admins may type a path", async () => {
  setup({ mode: "multi", allowTyped: true }, true);
  expect(await screen.findByText("A gép most nem elérhető")).toBeTruthy();
  expect(screen.getByText(/Utoljára látva/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "Újra" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Útvonal beírása" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Útvonal beírása" }), { target: { value: "/srv/data" } });
  fireEvent.click(screen.getByRole("button", { name: "Hozzáadás" }));
  await waitFor(() => expect(screen.getByText(/1 mappa: 1 olvasás, 0 írás/)).toBeTruthy());
});
