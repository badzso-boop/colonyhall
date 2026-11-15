// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { NavLink, Outlet, useNavigate } from "react-router";
import { LogOut, TriangleAlert } from "lucide-react";
import { cn } from "@/shared/lib";
import { BrandMark, Button } from "@/shared/ui";
import "../texts";
import { useOperatorLogout, useOperatorSession } from "../api";

const links = [
  { to: "/", key: "nav.customers", end: true },
  { to: "/audit", key: "nav.audit", end: false },
  { to: "/system", key: "nav.system", end: false },
  { to: "/account", key: "nav.account", end: false },
] as const;

/** The operator console's frame: its own header and navigation, and the developer-mode banner on every page. */
export function OperatorShell() {
  const { t } = useTranslation("operator");
  const session = useOperatorSession();
  const logout = useOperatorLogout();
  const navigate = useNavigate();
  const s = session.data;
  return (
    <div className="flex min-h-full flex-col bg-bg">
      {s?.dev_access ? <DevBanner /> : null}
      <header className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-border bg-surface px-4 py-2">
        <span className="flex items-center gap-2">
          <BrandMark className="size-6" />
          <span className="font-semibold">{t("title")}</span>
        </span>
        <nav aria-label={t("nav.label")} className="order-last -mx-1 flex w-full gap-1 overflow-x-auto sm:order-none sm:mx-0 sm:w-auto sm:min-w-0 sm:flex-1">
          {links.map((l) => (
            <NavLink
              key={l.to}
              to={l.to}
              end={l.end}
              className={({ isActive }) =>
                cn("shrink-0 rounded-md px-2 py-1 text-ui whitespace-nowrap text-fg-muted hover:bg-surface-sunken hover:text-fg", isActive && "bg-surface-sunken font-medium text-fg")
              }
            >
              {t(l.key)}
            </NavLink>
          ))}
        </nav>
        <span className="ml-auto flex items-center gap-2 text-meta text-fg-muted">
          {s ? <span>@{s.operator.handle}</span> : null}
          <Button
            size="sm"
            variant="ghost"
            icon={<LogOut className="size-4" />}
            loading={logout.isPending}
            onClick={() => logout.mutate(undefined, { onSettled: () => navigate("/login", { replace: true }) })}
          >
            {t("logout")}
          </Button>
        </span>
      </header>
      <main className="min-h-0 flex-1">
        <Outlet />
      </main>
    </div>
  );
}

/** Developer mode (COLONY_OPERATOR_DEV_ACCESS=1): shown on every page, never dismissable. */
export function DevBanner() {
  const { t } = useTranslation("operator");
  return (
    <div role="status" className="flex items-center justify-center gap-2 border-b border-warning/40 bg-warning-soft px-4 py-1.5 text-ui font-medium text-warning">
      <TriangleAlert aria-hidden className="size-4 shrink-0" />
      {t("devAccess")}
    </div>
  );
}
