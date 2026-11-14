import { useTranslation } from "react-i18next";
import { ShieldAlert } from "lucide-react";
import { Dialog, HoldButton, toast } from "@/shared/ui";
import { useEmergencyStop } from "../api";

/**
 * The composer's "/emergency" (round 55, part B): it only opens this
 * dialog, and the stop itself is the header's press-and-hold button, so
 * typing the command and pressing Enter never stops anything.
 */
export function EmergencyStopDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation("admin");
  const stop = useEmergencyStop();
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("emergencyStop.title")} description={t("emergencyStop.body")} closeLabel={t("close")}>
      <div className="flex justify-end">
        <HoldButton
          label={t("emergencyStop.hold")}
          hint={t("emergencyStop.hint")}
          icon={<ShieldAlert aria-hidden className="size-4" />}
          compact={false}
          onConfirm={() =>
            stop.mutate(undefined, {
              onSuccess: () => toast(t("emergencyStop.done")),
              onError: (e) => toast(e.message, { tone: "danger" }),
              onSettled: onClose,
            })
          }
        />
      </div>
    </Dialog>
  );
}
