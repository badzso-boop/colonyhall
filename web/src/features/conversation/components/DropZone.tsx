import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Paperclip } from "lucide-react";

/** Drag files anywhere over children to attach them: a full-area overlay while dragging. */
export function DropZone({ onFiles, children, className }: { onFiles: (files: File[]) => void; children: ReactNode; className?: string }) {
  const { t } = useTranslation("conversation");
  const [over, setOver] = useState(0);
  const hasFiles = (e: React.DragEvent) => [...e.dataTransfer.types].includes("Files");
  return (
    <div
      className={`relative ${className ?? ""}`}
      onDragEnter={(e) => hasFiles(e) && setOver((n) => n + 1)}
      onDragLeave={(e) => hasFiles(e) && setOver((n) => Math.max(0, n - 1))}
      onDragOver={(e) => {
        if (hasFiles(e)) e.preventDefault();
      }}
      onDrop={(e) => {
        if (!hasFiles(e)) return;
        e.preventDefault();
        setOver(0);
        onFiles([...e.dataTransfer.files]);
      }}
    >
      {children}
      {over > 0 ? (
        <div className="pointer-events-none absolute inset-2 z-(--z-pane) grid place-items-center rounded-lg border-2 border-dashed border-accent bg-accent-soft/80">
          <span className="flex items-center gap-2 text-title font-semibold text-accent-text">
            <Paperclip aria-hidden className="size-5" />
            {t("composer.drop")}
          </span>
        </div>
      ) : null}
    </div>
  );
}
