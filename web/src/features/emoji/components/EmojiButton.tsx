import { lazy, Suspense, useState, type ReactNode } from "react";
import { SmilePlus } from "lucide-react";
import { IconButton, Popover, PopoverContent, PopoverTrigger, Skeleton } from "@/shared/ui";

const EmojiPicker = lazy(() => import("./EmojiPicker"));

/**
 * A button that opens the emoji picker (round 55, part C): the "+" for
 * reactions and the composer's smiley. The picker and its data load on the
 * first open, as a chunk of their own.
 */
export function EmojiButton({ label, onPick, icon, size = "md", className }: { label: string; onPick: (char: string) => void; icon?: ReactNode; size?: "sm" | "md"; className?: string }) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <IconButton size={size} label={label} icon={icon ?? <SmilePlus className="size-4" />} className={className} />
      </PopoverTrigger>
      <PopoverContent label={label} align="end" className="w-80 p-0">
        {open ? (
          <Suspense fallback={<Skeleton className="m-3 h-72" />}>
            <EmojiPicker
              onPick={(c) => {
                onPick(c);
                setOpen(false);
              }}
            />
          </Suspense>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}
