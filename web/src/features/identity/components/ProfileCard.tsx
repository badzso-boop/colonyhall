import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Clock } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { useNow } from "@/shared/hooks";
import { hueVar } from "@/shared/lib";
import { Avatar, type AvatarStatus } from "@/shared/ui";
import type { Actor } from "../api";
import { localTime } from "../model";

/**
 * A person's or an agent's profile (round 34, part 7): avatar, names,
 * description, status text and local time. Other features add their part
 * through `extra` (an agent's state and machine) and `actions` (DM).
 */
export function ProfileCard({ actor, status, extra, actions }: { actor: Actor; status?: AvatarStatus; extra?: ReactNode; actions?: ReactNode }) {
  const { t, i18n } = useTranslation("identity");
  const now = useNow();
  const agent = actor.kind === "agent";
  const time = localTime(actor.timezone, now, localeOf(i18n.language));
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-start gap-3">
        <Avatar
          kind={agent ? "agent" : "human"}
          name={actor.display_name || actor.handle}
          emoji={actor.avatar_emoji}
          size="lg"
          status={status}
          ringColor={agent ? hueVar(actor.effective_color) : undefined}
          className="size-12 text-page"
        />
        <div className="min-w-0 flex-1">
          <p className="truncate text-title font-semibold">{actor.display_name || actor.handle}</p>
          <p className="truncate font-mono text-meta text-fg-muted">@{actor.handle}</p>
          {actor.status_text ? <p className="mt-1 text-ui">{actor.status_text}</p> : null}
        </div>
      </div>
      {actor.description ? <p className="text-ui whitespace-pre-wrap text-fg-muted">{actor.description}</p> : null}
      {time ? (
        <p className="flex items-center gap-1.5 text-meta text-fg-muted">
          <Clock aria-hidden className="size-3.5" />
          {t("profile.localTime", { time, zone: actor.timezone })}
        </p>
      ) : null}
      {extra}
      {actions ? <div className="flex flex-wrap gap-2 border-t border-border pt-3">{actions}</div> : null}
    </div>
  );
}
