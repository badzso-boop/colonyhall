import { useState } from "react";
import { useTranslation } from "react-i18next";
import { languages, setLanguage, type Language } from "@/shared/i18n";
import { applyTheme, readTheme, themes, type Theme } from "@/shared/theme";
import { BrandMark } from "@/shared/ui";

// Placeholder page of the scaffold; round 31 replaces it with the shell.
export default function ScaffoldPage() {
  const { t, i18n } = useTranslation("scaffold");
  const [theme, setTheme] = useState<Theme>(readTheme);

  const chooseTheme = (next: Theme) => {
    setTheme(next);
    applyTheme(next);
  };

  return (
    <div className="flex h-full flex-col">
      <header className="flex h-12 items-center gap-2 border-b border-border bg-surface px-3">
        <BrandMark />
        <span className="text-base font-semibold">{t("common:appName")}</span>
      </header>
      <main className="mx-auto flex w-full max-w-xl flex-1 flex-col justify-center gap-4 px-4">
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-fg-muted">{t("body")}</p>
        <a className="font-medium text-accent hover:text-accent-hover" href="/">
          {t("devPage")}
        </a>
        <div className="flex flex-wrap gap-4 text-sm">
          <label className="flex items-center gap-2">
            {t("common:theme.label")}
            <select
              className="rounded-md border border-border-strong bg-surface px-2 py-1"
              value={theme}
              onChange={(e) => chooseTheme(e.target.value as Theme)}
            >
              {themes.map((th) => (
                <option key={th} value={th}>
                  {t(`common:theme.${th}`)}
                </option>
              ))}
            </select>
          </label>
          <label className="flex items-center gap-2">
            {t("common:language.label")}
            <select
              className="rounded-md border border-border-strong bg-surface px-2 py-1"
              value={i18n.language}
              onChange={(e) => setLanguage(e.target.value as Language)}
            >
              {languages.map((l) => (
                <option key={l} value={l}>
                  {l.toUpperCase()}
                </option>
              ))}
            </select>
          </label>
        </div>
      </main>
    </div>
  );
}
