import { useMemo, useState, type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button, Dialog, Input, Select, Textarea, toast } from "@/shared/ui";
import { useSession, useUpdateProfile } from "../api";
import { draftOf, PROFILE_LIMITS, profileChanges, profileErrors, timeZoneOptions, type ProfileDraft } from "../model";

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-meta font-semibold text-fg-muted">{label}</span>
      {children}
      {hint ? <span className="text-meta text-fg-subtle">{hint}</span> : null}
    </label>
  );
}

/**
 * "Profilom" (round 34, part 7).
 */
export function ProfileDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation("identity");
  const me = useSession().data?.actor;
  const update = useUpdateProfile();
  const [draft, setDraft] = useState<ProfileDraft | null>(() => (me ? draftOf(me) : null));
  const zones = useMemo(() => timeZoneOptions(draft?.timezone ?? ""), [draft?.timezone]);
  if (!me || !draft) return null;
  const errors = profileErrors(draft);
  const changes = profileChanges(me, draft);
  const set = (k: keyof ProfileDraft) => (e: { target: { value: string } }) => setDraft({ ...draft, [k]: e.target.value });
  const save = (e?: FormEvent) => {
    e?.preventDefault();
    if (errors.length) return;
    if (!changes) return onClose();
    update.mutate(
      { actorId: me.id, body: changes },
      {
        onSuccess: () => {
          toast(t("profile.saved"));
          onClose();
        },
        onError: () => toast(t("profile.failed"), { tone: "danger" }),
      },
    );
  };
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("profile.title")}
      closeLabel={t("common:close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("profile.cancel")}
          </Button>
          <Button variant="primary" disabled={errors.length > 0} loading={update.isPending} onClick={() => save()}>
            {t("profile.save")}
          </Button>
        </>
      }
    >
      <form onSubmit={save} className="flex flex-col gap-3">
        <Field label={t("profile.displayName")} hint={t("profile.max", { count: PROFILE_LIMITS.displayName })}>
          <Input value={draft.display_name} onChange={set("display_name")} aria-invalid={errors.includes("display_name")} />
        </Field>
        <Field label={t("profile.avatarEmoji")} hint={t("profile.avatarEmojiHint")}>
          <Input value={draft.avatar_emoji} onChange={set("avatar_emoji")} className="w-24" aria-invalid={errors.includes("avatar_emoji")} />
        </Field>
        <Field label={t("profile.statusText")} hint={t("profile.max", { count: PROFILE_LIMITS.statusText })}>
          <Input value={draft.status_text} onChange={set("status_text")} aria-invalid={errors.includes("status_text")} />
        </Field>
        <Field label={t("profile.description")} hint={t("profile.max", { count: PROFILE_LIMITS.description })}>
          <Textarea value={draft.description} onChange={set("description")} rows={3} aria-invalid={errors.includes("description")} />
        </Field>
        <Field label={t("profile.timezone")}>
          <Select value={draft.timezone} onChange={set("timezone")}>
            {zones.map((z) => (
              <option key={z} value={z}>
                {z}
              </option>
            ))}
          </Select>
        </Field>
      </form>
    </Dialog>
  );
}
