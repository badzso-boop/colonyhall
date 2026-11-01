import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Ban, Check, Info } from "lucide-react";
import { useActors } from "@/features/identity";
import { cn, hueVar, hues, type Hue } from "@/shared/lib";
import { Button, Dialog, Input, Notice, toast } from "@/shared/ui";
import { useSetHostState, useUpdateHost, type Host } from "../api";
import { AgentAvatar } from "./bits";
import { Mono, Row } from "./OverviewTab";

/** Beállítások: name and colour (live preview on the agents), the guard.yaml-only settings, revoke. */
export function SettingsTab({ host }: { host: Host }) {
  const { t } = useTranslation("hosts");
  const actors = useActors();
  const update = useUpdateHost();
  const setState = useSetHostState();
  const [name, setName] = useState(host.name);
  const [color, setColor] = useState<Hue>(host.color);
  const [confirm, setConfirm] = useState(false);
  const dirty = name.trim() !== host.name || color !== host.color;
  const r = host.report;

  const save = () => {
    const body = { ...(name.trim() !== host.name ? { name: name.trim() } : {}), ...(color !== host.color ? { color } : {}) };
    update.mutate(
      { hostId: host.id, body },
      { onSuccess: () => toast(t("settings.saved")), onError: () => toast(t("settings.saveFailed"), { tone: "danger" }) },
    );
  };
  const changeState = (state: "approved" | "revoked") =>
    setState.mutate(
      { hostId: host.id, state },
      {
        onSuccess: () => toast(t(state === "revoked" ? "settings.revoked" : "settings.approved")),
        onError: () => toast(t("settings.stateFailed"), { tone: "danger" }),
        onSettled: () => setConfirm(false),
      },
    );

  return (
    <div className="flex flex-col gap-6">
      <section className="flex flex-col gap-3">
        <label className="flex max-w-sm flex-col gap-1 text-ui">
          <span className="font-medium">{t("settings.name")}</span>
          <Input value={name} maxLength={64} onChange={(e) => setName(e.target.value)} />
        </label>
        <div className="flex flex-col gap-1">
          <span id={`hue-${host.id}`} className="text-ui font-medium">
            {t("settings.color")}
          </span>
          <div role="radiogroup" aria-labelledby={`hue-${host.id}`} className="flex flex-wrap gap-2">
            {hues.map((h) => (
              <button
                key={h}
                type="button"
                role="radio"
                aria-checked={color === h}
                aria-label={t(`hues.${h}`)}
                title={t(`hues.${h}`)}
                onClick={() => setColor(h)}
                style={{ background: hueVar(h) }}
                className={cn("grid size-8 place-items-center rounded-full border-2", color === h ? "border-fg" : "border-transparent")}
              >
                {color === h ? <Check aria-hidden className="size-4 text-bg" /> : null}
              </button>
            ))}
          </div>
        </div>
        {host.agents.length ? (
          <div className="flex flex-col gap-1">
            <span className="text-ui text-fg-muted">{t("settings.preview")}</span>
            <ul aria-label={t("settings.preview")} className="flex flex-wrap gap-3">
              {host.agents.map((a) => {
                const actor = actors.data?.get(a.agent_id);
                return (
                  <li key={a.agent_id} className="flex items-center gap-1.5 text-ui" data-own-color={actor?.color ? "true" : undefined}>
                    <AgentAvatar actor={actor} handle={a.handle} hostColor={color} size="sm" />
                    <span className="font-mono text-meta">@{a.handle}</span>
                    {actor?.color ? <span className="text-meta text-fg-muted">{t("settings.ownColor")}</span> : null}
                  </li>
                );
              })}
            </ul>
          </div>
        ) : null}
        <div>
          <Button variant="primary" disabled={!dirty || !name.trim()} loading={update.isPending} onClick={save}>
            {t("settings.save")}
          </Button>
        </div>
      </section>

      <section className="flex flex-col gap-2">
        <h3 className="text-title font-semibold">{t("settings.guardYaml")}</h3>
        <Notice icon={<Info className="size-4" />}>{t("settings.guardYamlHint")}</Notice>
        <dl className="divide-y divide-border">
          <Row label="roots">{r.roots.length ? r.roots.map((p) => <Mono key={p}>{p} </Mono>) : "–"}</Row>
          <Row label="allow_unsandboxed">{t(r.allow_unsandboxed ? "yes" : "no")}</Row>
          <Row label="exchange_dir">{r.exchange_dir ? <Mono>{r.exchange_dir}</Mono> : "–"}</Row>
          <Row label={t("overview.sandbox")}>{r.sandbox || t("overview.noSandbox")}</Row>
        </dl>
      </section>

      <section className="flex flex-col gap-2 rounded-md border border-danger/40 p-3">
        <h3 className="text-title font-semibold text-danger">{t("settings.danger")}</h3>
        {host.state === "revoked" ? (
          <>
            <p className="text-ui text-fg-muted">{t("settings.revokedNote")}</p>
            <div>
              <Button loading={setState.isPending} onClick={() => changeState("approved")}>
                {t("settings.approve")}
              </Button>
            </div>
          </>
        ) : (
          <>
            <p className="text-ui text-fg-muted">{t("settings.revokeNote")}</p>
            <div>
              <Button variant="danger" icon={<Ban className="size-4" />} onClick={() => setConfirm(true)}>
                {t("settings.revoke")}
              </Button>
            </div>
          </>
        )}
      </section>

      <Dialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t("settings.revokeTitle", { name: host.name })}
        description={t("settings.revokeBody")}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setConfirm(false)}>{t("cancel")}</Button>
            <Button variant="danger" loading={setState.isPending} onClick={() => changeState("revoked")}>
              {t("settings.revokeConfirm")}
            </Button>
          </>
        }
      >
        <p className="text-ui">{t("settings.revokeAgents", { count: host.agents.length })}</p>
      </Dialog>
    </div>
  );
}
