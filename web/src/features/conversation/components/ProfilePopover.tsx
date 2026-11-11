import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { MessageSquare } from "lucide-react";
import { useWidth } from "@/shared/hooks";
import { cn } from "@/shared/lib";
import { Button, Popover, PopoverAnchor, PopoverContent, PopoverTrigger, Sheet, toast } from "@/shared/ui";
import { AgentProfileDetails } from "@/features/agents";
import { ProfileCard, useActors, useSession } from "@/features/identity";
import { useOpenDm } from "../api";

/** The popover's content: the card, an agent's details, and "DM". */
function ProfileBody({ actorId, onClose }: { actorId: string; onClose: () => void }) {
  const { t } = useTranslation("conversation");
  const actor = useActors().data?.get(actorId);
  const meId = useSession().data?.actor.id;
  const dm = useOpenDm();
  const navigate = useNavigate();
  if (!actor) return <p className="text-ui text-fg-muted">{t("profile.unknown")}</p>;
  const canDm = actor.id !== meId && !actor.archived_at;
  return (
    <ProfileCard
      actor={actor}
      extra={actor.kind === "agent" ? <AgentProfileDetails agentId={actor.id} onOpenOverview={onClose} /> : null}
      actions={
        canDm ? (
          <Button
            size="sm"
            variant="primary"
            icon={<MessageSquare aria-hidden className="size-3.5" />}
            loading={dm.isPending}
            onClick={() =>
              dm.mutate(actor.id, {
                onSuccess: (ch) => {
                  onClose();
                  void navigate(`/c/${ch.id}`);
                },
                onError: () => toast(t("profile.dmFailed"), { tone: "danger" }),
              })
            }
          >
            {t("profile.dm")}
          </Button>
        ) : null
      }
    />
  );
}

function useLabel(actorId: string): string {
  const { t } = useTranslation("conversation");
  const a = useActors().data?.get(actorId);
  return t("profile.label", { name: a ? a.display_name || a.handle : "?" });
}

/**
 * A button around an avatar or a name that opens the profile
 * (round 34, part 7): a popover, or a sheet on a phone.
 */
export function ProfileTrigger({ actorId, children, className }: { actorId: string; children: ReactNode; className?: string }) {
  const [open, setOpen] = useState(false);
  const narrow = useWidth() === "narrow";
  const label = useLabel(actorId);
  const button = (
    <button type="button" aria-label={label} aria-haspopup="dialog" onClick={narrow ? () => setOpen(true) : undefined} className={cn("rounded-md text-left", className)}>
      {children}
    </button>
  );
  if (narrow) {
    return (
      <>
        {button}
        <ProfileSheet actorId={actorId} open={open} onOpenChange={setOpen} />
      </>
    );
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{button}</PopoverTrigger>
      {open ? (
        <PopoverContent label={label}>
          <ProfileBody actorId={actorId} onClose={() => setOpen(false)} />
        </PopoverContent>
      ) : null}
    </Popover>
  );
}

function ProfileSheet({ actorId, open, onOpenChange }: { actorId: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const { t } = useTranslation("conversation");
  const label = useLabel(actorId);
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title={label} closeLabel={t("new.close")}>
      {open ? <ProfileBody actorId={actorId} onClose={() => onOpenChange(false)} /> : null}
    </Sheet>
  );
}

/**
 * The profile opened from elsewhere (a sidebar row's menu), anchored to
 * `children` on desktop, a sheet on a phone.
 */
export function ProfileAnchor({ actorId, open, onOpenChange, children }: { actorId: string | null; open: boolean; onOpenChange: (o: boolean) => void; children: ReactNode }) {
  const narrow = useWidth() === "narrow";
  const label = useLabel(actorId ?? "");
  if (!actorId) return <>{children}</>;
  if (narrow) {
    return (
      <>
        {children}
        <ProfileSheet actorId={actorId} open={open} onOpenChange={onOpenChange} />
      </>
    );
  }
  return (
    // Modal: the row menu that opened it hands focus back to its trigger, which must not close it.
    <Popover modal open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor asChild>{children}</PopoverAnchor>
      {open ? (
        <PopoverContent label={label}>
          <ProfileBody actorId={actorId} onClose={() => onOpenChange(false)} />
        </PopoverContent>
      ) : null}
    </Popover>
  );
}
