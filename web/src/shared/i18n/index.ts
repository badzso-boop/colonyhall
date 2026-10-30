import i18n, { type Resource } from "i18next";
import { initReactI18next } from "react-i18next";

export const languages = ["hu", "en"] as const;
export type Language = (typeof languages)[number];

const storageKey = "colony.lang";

/** The Intl locale of a UI language (dates and numbers follow the UI, not the browser). */
export function localeOf(lang: string): string {
  return lang === "en" ? "en-GB" : "hu-HU";
}

export function storedLanguage(): Language {
  try {
    const v = localStorage.getItem(storageKey);
    if (v === "hu" || v === "en") return v;
  } catch {
    // storage blocked: fall back to the default
  }
  return "hu";
}

export function setLanguage(lang: Language): void {
  try {
    localStorage.setItem(storageKey, lang);
  } catch {
    // storage blocked: the choice lasts until reload
  }
  void i18n.changeLanguage(lang);
  document.documentElement.lang = lang;
}

// initI18n starts i18next with every namespace: "common" from shared and one
// per feature. app/i18n.ts collects them (shared does not know the features).
export function initI18n(resources: Resource): typeof i18n {
  void i18n.use(initReactI18next).init({
    resources,
    lng: storedLanguage(),
    fallbackLng: "hu",
    defaultNS: "common",
    ns: Object.keys(resources.hu ?? {}),
    // React escapes text itself.
    interpolation: { escapeValue: false },
  });
  return i18n;
}

export { i18n };
