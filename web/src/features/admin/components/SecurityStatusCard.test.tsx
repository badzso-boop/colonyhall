// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { render, screen } from "@testing-library/react";
import type { SecurityStatus } from "../model";
import { SecurityStatusCard } from "./SecurityStatusCard";

const security = (over: Partial<SecurityStatus> = {}): SecurityStatus => ({
  lockouts_24h: 0,
  lockouts: [],
  allowlist: [],
  trusted_proxies: false,
  ...over,
});

const card = () => screen.getByRole("heading", { level: 2, name: /Biztonság/ }).parentElement!;

test("no lockouts: ok, an empty allowlist and no proxies", () => {
  render(<SecurityStatusCard security={security()} />);
  expect(card().textContent).toContain("rendben");
  expect(card().textContent).toContain("Kizárások (24 óra)0");
  expect(card().textContent).toContain("Engedélyezőlistanincs");
  expect(card().textContent).toContain("Megbízható proxyknem");
});

test("a lockout warns and lists the handle, the address and the kind", () => {
  render(
    <SecurityStatusCard
      security={security({
        lockouts_24h: 1,
        lockouts: [{ scope: "login", handle: "alice", ip: "192.0.2.1", at: "2026-11-13T09:00:00Z", until: "2026-11-13T09:15:00Z" }],
        allowlist: ["127.0.0.1/32", "::1/128"],
        trusted_proxies: true,
      })}
    />,
  );
  expect(card().textContent).toContain("figyelmeztetés");
  expect(card().textContent).toContain("@alice · 192.0.2.1 · belépés");
  expect(card().textContent).toContain("127.0.0.1/32, ::1/128");
  expect(card().textContent).toContain("Megbízható proxykigen");
});
