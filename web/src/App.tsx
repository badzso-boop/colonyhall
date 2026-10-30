import { useState } from "react";
import { Route, Routes } from "react-router";
import { useTranslation } from "react-i18next";
import { languages, setLanguage, type Language } from "./i18n";
import { applyTheme, readTheme, themes, type Theme } from "./theme";

function Mark() {
  return <img src={`${import.meta.env.BASE_URL}brand/mark.svg`} alt="" className="size-6" />;
}

function Scaffold() {
  const { t, i18n } = useTranslation();
  const [theme, setTheme] = useState<Theme>(readTheme);

  const chooseTheme = (next: Theme) => {
    setTheme(next);
    applyTheme(next);
  };

  return (
    <div className="flex h-full flex-col">
      <header className="flex h-12 items-center gap-2 border-b border-border bg-surface px-3">
        <Mark />
        <span className="text-base font-semibold">{t("app.name")}</span>
      </header>
      <main className="mx-auto flex w-full max-w-xl flex-1 flex-col justify-center gap-4 px-4">
        <h1 className="text-xl font-semibold">{t("scaffold.title")}</h1>
        <p className="text-fg-muted">{t("scaffold.body")}</p>
        <a className="font-medium text-accent hover:text-accent-hover" href="/">
          {t("scaffold.devPage")}
        </a>
        <div className="flex flex-wrap gap-4 text-sm">
          <label className="flex items-center gap-2">
            {t("theme.label")}
            <select
              className="rounded-md border border-border-strong bg-surface px-2 py-1"
              value={theme}
              onChange={(e) => chooseTheme(e.target.value as Theme)}
            >
              {themes.map((th) => (
                <option key={th} value={th}>
                  {t(`theme.${th}`)}
                </option>
              ))}
            </select>
          </label>
          <label className="flex items-center gap-2">
            {t("language.label")}
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

export default function App() {
  return (
    <Routes>
      <Route path="*" element={<Scaffold />} />
    </Routes>
  );
}
