import { useState } from "react";
import { Dialog as RDialog } from "radix-ui";
import { ChevronLeft, ChevronRight, Download, X } from "lucide-react";
import { IconButton } from "./IconButton";

export type LightboxImage = { src: string; name: string };

/**
 * A full-screen image viewer: Esc closes, arrows step through the images of
 * the message, a download button.
 */
export function Lightbox({
  images,
  start,
  onClose,
  labels,
}: {
  images: LightboxImage[];
  start: number;
  onClose: () => void;
  labels: { close: string; prev: string; next: string; download: string };
}) {
  const [i, setI] = useState(start);
  const img = images[i];
  if (!img) return null;
  const step = (d: number) => setI((x) => (x + d + images.length) % images.length);
  return (
    <RDialog.Root open onOpenChange={(o) => !o && onClose()}>
      <RDialog.Portal>
        <RDialog.Overlay className="fixed inset-0 z-(--z-overlay) bg-overlay backdrop-blur-sm" />
        <RDialog.Content
          className="fixed inset-0 z-(--z-overlay) flex flex-col items-center justify-center gap-3 p-4 outline-none"
          onKeyDown={(e) => {
            if (e.key === "ArrowRight") step(1);
            if (e.key === "ArrowLeft") step(-1);
          }}
        >
          <RDialog.Title className="sr-only">{img.name}</RDialog.Title>
          <img src={img.src} alt={img.name} className="max-h-[80dvh] max-w-full rounded-md object-contain shadow-overlay" />
          <div className="flex items-center gap-2 rounded-full bg-surface-raised px-2 py-1 shadow-overlay">
            {images.length > 1 ? <IconButton label={labels.prev} icon={<ChevronLeft className="size-4" />} onClick={() => step(-1)} /> : null}
            <span className="max-w-64 truncate px-1 text-ui">{img.name}</span>
            {images.length > 1 ? <IconButton label={labels.next} icon={<ChevronRight className="size-4" />} onClick={() => step(1)} /> : null}
            <a href={img.src} download={img.name} aria-label={labels.download} title={labels.download} className="grid size-8 place-items-center rounded-md text-fg-muted hover:bg-surface-sunken hover:text-fg">
              <Download aria-hidden className="size-4" />
            </a>
            <RDialog.Close asChild>
              <IconButton label={labels.close} icon={<X className="size-4" />} />
            </RDialog.Close>
          </div>
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}
