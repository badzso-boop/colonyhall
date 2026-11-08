import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { CheckCircle2, Copy, KeyRound, Loader2 } from "lucide-react";
import { useNow } from "@/shared/hooks";
import { Button, Dialog, IconButton, Input, Select, toast } from "@/shared/ui";
import { useCreatePairingCode, useHosts } from "../api";
import { enrollCommand, guardYaml, installCommand, mmss, newlyEnrolled, secondsLeft, type InstallPlatform } from "../model";

/** A command or file to copy, with its copy button. */
function CodeBlock({ text, label }: { text: string; label: string }) {
  const { t } = useTranslation("hosts");
  const copy = () => void navigator.clipboard?.writeText(text).then(() => toast(t("wizard.copied")), () => toast(t("wizard.copyFailed"), { tone: "danger" }));
  return (
    <div className="flex items-start gap-2 rounded-md border border-border bg-surface-sunken p-2">
      <pre aria-label={label} className="min-w-0 flex-1 overflow-x-auto font-mono text-ui whitespace-pre-wrap break-all">
        {text}
      </pre>
      <IconButton label={t("wizard.copy", { what: label })} icon={<Copy className="size-4" />} size="sm" onClick={copy} />
    </div>
  );
}

/**
 * Új gép: a pairing code with its countdown and the enroll command, then a
 * live wait for the machine (host.enrolled re-reads the list; it is also
 * polled), then the next steps.
 */
export function NewHostWizard({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation("hosts");
  const navigate = useNavigate();
  const create = useCreatePairingCode();
  const [name, setName] = useState("");
  const [platform, setPlatform] = useState<InstallPlatform>("linux");
  const [known, setKnown] = useState<Set<string> | null>(null);
  const waiting = open && known !== null;
  const hosts = useHosts(open, waiting ? 5000 : undefined);
  const now = useNow(1000);
  const server = window.location.origin;
  const code = create.data;
  const left = code ? secondsLeft(code.expires_at, now) : 0;
  const enrolled = waiting && hosts.data ? newlyEnrolled(hosts.data, known, name) : null;

  // Closing starts the wizard over.
  const change = (o: boolean) => {
    if (!o) {
      setKnown(null);
      setName("");
      create.reset();
    }
    onOpenChange(o);
  };

  const requestCode = () =>
    create.mutate(undefined, {
      onSuccess: () => setKnown(new Set((hosts.data ?? []).map((h) => h.id))),
      onError: () => toast(t("wizard.codeFailed"), { tone: "danger" }),
    });
  const done = () => {
    change(false);
    if (enrolled) navigate(`/hosts/${enrolled.id}`);
  };

  return (
    <Dialog
      open={open}
      onOpenChange={change}
      size="lg"
      title={t("wizard.title")}
      description={enrolled ? t("wizard.step3") : code ? t("wizard.step2") : t("wizard.step1")}
      closeLabel={t("close")}
      footer={
        enrolled ? (
          <Button variant="primary" onClick={done}>
            {t("wizard.done")}
          </Button>
        ) : (
          <Button onClick={() => change(false)}>{t("cancel")}</Button>
        )
      }
    >
      {enrolled ? (
        <div className="flex flex-col gap-3">
          <p role="status" className="flex items-center gap-2 font-semibold text-success">
            <CheckCircle2 aria-hidden className="size-5" />
            {t("wizard.enrolled", { name: enrolled.name })}
          </p>
          <p className="text-ui">{t("wizard.yamlHint")}</p>
          <CodeBlock text={guardYaml(server)} label="guard.yaml" />
          <p className="text-ui">{t("wizard.installHint")}</p>
          <CodeBlock text="colony guard install -config guard.yaml" label={t("wizard.installLabel")} />
          <p className="text-ui text-fg-muted">{t("wizard.colorHint")}</p>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <label className="flex flex-col gap-1 text-ui">
            <span className="font-medium">{t("wizard.name")}</span>
            <Input value={name} maxLength={64} placeholder={t("wizard.namePlaceholder")} onChange={(e) => setName(e.target.value)} />
          </label>
          {code && left > 0 ? (
            <>
              <div className="flex flex-wrap items-baseline gap-3">
                <span className="font-mono text-display tracking-wider" aria-label={t("wizard.code")}>
                  {code.code}
                </span>
                <span className="tabular text-ui text-fg-muted" aria-live="off">
                  {t("wizard.expires", { left: mmss(left) })}
                </span>
              </div>
              <div className="flex flex-wrap items-center gap-2 text-ui">
                <span>{t("wizard.platform")}</span>
                <Select aria-label={t("wizard.platform")} value={platform} onChange={(e) => setPlatform(e.target.value as InstallPlatform)} className="w-auto">
                  {(["linux", "wsl", "macos"] as const).map((p) => (
                    <option key={p} value={p}>
                      {t(`wizard.platforms.${p}`)}
                    </option>
                  ))}
                </Select>
              </div>
              <p className="text-ui">{t("wizard.installRun")}</p>
              <CodeBlock text={installCommand(server, code.code, name)} label={t("wizard.installCommand")} />
              <p className="text-meta text-fg-muted">{t(`wizard.platformNotes.${platform}`)}</p>
              <details className="text-ui">
                <summary className="cursor-pointer text-fg-muted">{t("wizard.runHint")}</summary>
                <div className="pt-2">
                  <CodeBlock text={enrollCommand(server, code.code, name)} label={t("wizard.command")} />
                </div>
              </details>
              <p role="status" className="flex items-center gap-2 text-ui text-fg-muted">
                <Loader2 aria-hidden className="size-4 motion-safe:animate-spin" />
                {t("wizard.waiting")}
              </p>
            </>
          ) : (
            <>
              {code ? <p className="text-ui text-warning">{t("wizard.expired")}</p> : null}
              <div>
                <Button variant="primary" icon={<KeyRound className="size-4" />} loading={create.isPending} disabled={!name.trim() || !hosts.isSuccess} onClick={requestCode}>
                  {t(code ? "wizard.newCode" : "wizard.request")}
                </Button>
              </div>
            </>
          )}
        </div>
      )}
    </Dialog>
  );
}
