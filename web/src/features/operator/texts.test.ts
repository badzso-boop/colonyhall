// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import i18n from "i18next";
import { operatorTexts } from "./texts";

const keys = (o: unknown, p = ""): string[] =>
  Object.entries(typeof o === "object" && o ? o : {}).flatMap(([k, v]) => (typeof v === "object" && v ? keys(v, `${p}${k}.`) : [`${p}${k}`]));

test("the operator texts have the same keys in Hungarian and English, and load with the chunk", () => {
  expect(keys(operatorTexts.en).sort()).toEqual(keys(operatorTexts.hu).sort());
  expect(i18n.getFixedT("hu", "operator")("title")).toBe("Üzemeltetés");
});
