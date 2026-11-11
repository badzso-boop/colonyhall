import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "@/shared/api";
import { Button, Dialog, Input, Notice, toast } from "@/shared/ui";
import { useEmergencyClear } from "../api";

/** "Feloldás": the stop is only released with the password again; errors stay in the dialog. */
export function EmergencyReleaseDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation("admin");
  const clear = useEmergencyClear();
  const [password, setPassword] = useState("");
  const close = (o: boolean) => {
    if (!o) {
      setPassword("");
      clear.reset();
    }
    onOpenChange(o);
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!password) return;
    clear.mutate(password, {
      onSuccess: () => {
        toast(t("emergency.released"));
        close(false);
      },
      onError: () => setPassword(""),
    });
  };
  return (
    <Dialog open={open} onOpenChange={close} title={t("emergency.title")} description={t("emergency.body")} closeLabel={t("close")}>
      {/* A passkey option comes here later (round 35, decision 1). */}
      <form onSubmit={submit} className="flex flex-col gap-3">
        {clear.error ? <Notice tone="danger">{clear.error instanceof ApiError && clear.error.status === 401 ? t("emergency.wrong") : clear.error.message}</Notice> : null}
        <label className="flex flex-col gap-1 text-ui">
          {t("emergency.password")}
          <Input type="password" autoComplete="current-password" autoFocus value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={() => close(false)}>
            {t("cancel")}
          </Button>
          <Button type="submit" variant="primary" disabled={!password} loading={clear.isPending}>
            {t("emergency.release")}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
