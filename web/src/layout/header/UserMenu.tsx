import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { Check, Keyboard, LogOut, Monitor, Moon, Sun, UserRound } from "lucide-react";
import { languages, setLanguage } from "@/shared/i18n";
import { applyTheme, readTheme, type Theme } from "@/shared/theme";
import { Avatar, Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/shared/ui";
import { ProfileDialog, useLogout, useSession } from "@/features/identity";
import { openOverlay } from "../overlays";

const themeIcons = { system: Monitor, light: Sun, dark: Moon } as const;

export function UserMenu() {
  const { t, i18n } = useTranslation("layout");
  const session = useSession();
  const logout = useLogout();
  const navigate = useNavigate();
  const [theme, setTheme] = useState<Theme>(readTheme);
  const [profile, setProfile] = useState(false);
  const me = session.data?.actor;
  if (!me) return null;
  const choose = (th: Theme) => {
    setTheme(th);
    applyTheme(th);
  };
  return (
    <Menu>
      <MenuTrigger asChild>
        <button type="button" aria-label={t("userMenu")} className="ml-1 rounded-full">
          <Avatar kind="human" name={me.display_name || me.handle} emoji={me.avatar_emoji} />
        </button>
      </MenuTrigger>
      <MenuContent>
        <div className="px-2 py-1.5">
          <p className="font-semibold">{me.display_name}</p>
          <p className="text-meta text-fg-muted">@{me.handle}</p>
          {me.status_text ? <p className="mt-0.5 text-meta text-fg-muted">{me.status_text}</p> : null}
        </div>
        {me.kind === "human" ? (
          <MenuItem onSelect={() => setProfile(true)} icon={<UserRound aria-hidden className="size-4" />}>
            {t("myProfile")}
          </MenuItem>
        ) : null}
        <MenuSeparator />
        <p className="px-2 pt-1 text-meta text-fg-subtle">{t("theme")}</p>
        {(["system", "light", "dark"] as const).map((th) => {
          const Icon = themeIcons[th];
          return (
            <MenuItem key={th} onSelect={() => choose(th)} icon={<Icon aria-hidden className="size-4" />}>
              <span className="flex-1">{t(`common:theme.${th}`)}</span>
              {theme === th ? <Check aria-hidden className="size-4" /> : null}
            </MenuItem>
          );
        })}
        <p className="px-2 pt-1 text-meta text-fg-subtle">{t("language")}</p>
        {languages.map((l) => (
          <MenuItem key={l} onSelect={() => setLanguage(l)}>
            <span className="flex-1">{l === "hu" ? "Magyar" : "English"}</span>
            {i18n.language === l ? <Check aria-hidden className="size-4" /> : null}
          </MenuItem>
        ))}
        <MenuSeparator />
        <MenuItem onSelect={() => openOverlay("help")} icon={<Keyboard aria-hidden className="size-4" />}>
          {t("shortcutsMenu")}
        </MenuItem>
        <MenuItem
          onSelect={() => logout.mutate(undefined, { onSettled: () => navigate("/login", { replace: true }) })}
          icon={<LogOut aria-hidden className="size-4" />}
        >
          {t("logout")}
        </MenuItem>
      </MenuContent>
      {profile ? <ProfileDialog onClose={() => setProfile(false)} /> : null}
    </Menu>
  );
}
