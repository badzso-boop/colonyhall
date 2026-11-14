import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Button, Dialog, toast } from "@/shared/ui";
import { useActors } from "@/features/identity";
import { useChangeAgentState } from "../api";

/**
 * The composer's "/stop @agent" (round 55, part B): the agent's stop (its
 * running work is interrupted, its tokens stop working), after the same
 * confirmation as on the agent's page. An unknown handle closes with a hint.
 */
export function StopAgentDialog({ handle, onClose }: { handle: string; onClose: () => void }) {
  const { t } = useTranslation("agents");
  const actors = useActors();
  const change = useChangeAgentState();
  const agent = [...(actors.data?.values() ?? [])].find((a) => a.kind === "agent" && a.handle === handle);
  const unknown = Boolean(actors.data) && !agent;
  useEffect(() => {
    if (!unknown) return;
    toast(t("stopCommand.unknown", { handle }), { tone: "danger" });
    onClose();
  }, [unknown, handle, onClose, t]);
  if (!agent) return null;
  const stop = () =>
    change.mutate(
      { id: agent.id, state: "stopped" },
      {
        onSuccess: () => toast(t("overview.actions.done")),
        onError: () => toast(t("overview.actions.failed"), { tone: "danger" }),
        onSettled: onClose,
      },
    );
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("overview.actions.stopTitle", { handle: agent.handle })}
      description={t("overview.actions.stopBody")}
      closeLabel={t("overview.actions.cancel")}
      footer={
        <>
          <Button onClick={onClose}>{t("overview.actions.cancel")}</Button>
          <Button variant="danger" loading={change.isPending} onClick={stop}>
            {t("overview.actions.stop")}
          </Button>
        </>
      }
    >
      {null}
    </Dialog>
  );
}
