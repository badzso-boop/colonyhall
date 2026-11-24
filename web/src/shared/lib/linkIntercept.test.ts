// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { interceptLink, registerLinkHandler } from "./linkIntercept";

test("a registered handler takes its links until it is removed", () => {
  expect(interceptLink("http://192.0.2.1:41000/")).toBe(false);
  const off = registerLinkHandler((href) => href.startsWith("http://192.0.2.1:41000"));
  expect(interceptLink("http://192.0.2.1:41000/")).toBe(true);
  expect(interceptLink("https://example.com/")).toBe(false);
  off();
  expect(interceptLink("http://192.0.2.1:41000/")).toBe(false);
});
