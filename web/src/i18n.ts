import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import hu from "./locales/hu.json";
import en from "./locales/en.json";

export const languages = ["hu", "en"] as const;
export type Language = (typeof languages)[number];

const storageKey = "colony.lang";

function storedLanguage(): Language {
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

void i18n.use(initReactI18next).init({
  resources: { hu: { translation: hu }, en: { translation: en } },
  lng: storedLanguage(),
  fallbackLng: "hu",
  // React escapes text itself.
  interpolation: { escapeValue: false },
});

export default i18n;
