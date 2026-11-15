// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import i18n from "i18next";
import en from "./locales/en.json";
import hu from "./locales/hu.json";

// The operator's texts travel in the console's lazy chunk (round 51), not
// in the app's main bundle: app/i18n.ts leaves this namespace out, and
// importing this module adds it.
export const operatorTexts: Record<string, Record<string, unknown>> = { hu, en };

export function addOperatorTexts(): void {
  for (const [lang, data] of Object.entries(operatorTexts)) {
    if (!i18n.hasResourceBundle(lang, "operator")) i18n.addResourceBundle(lang, "operator", data, true, true);
  }
}

addOperatorTexts();
