// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { collectResources } from "./i18n";

const keys = (o: unknown, p = ""): string[] =>
  Object.entries(typeof o === "object" && o ? o : {}).flatMap(([k, v]) => (typeof v === "object" && v ? keys(v, `${p}${k}.`) : [`${p}${k}`]));

test("every namespace has the same keys in Hungarian and English", () => {
  const r = collectResources();
  const hu = r.hu ?? {};
  const en = r.en ?? {};
  expect(Object.keys(en).sort()).toEqual(Object.keys(hu).sort());
  expect(Object.keys(hu)).toContain("common");
  for (const ns of Object.keys(hu)) {
    expect({ ns, keys: keys(en[ns] ?? {}).sort() }).toEqual({ ns, keys: keys(hu[ns] ?? {}).sort() });
  }
});
