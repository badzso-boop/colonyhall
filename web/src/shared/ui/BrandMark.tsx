import { cn } from "@/shared/lib";

// The Colonyhall mark (an ant seen from above), from public/brand/mark.svg.
export function BrandMark({ className }: { className?: string }) {
  return <img src={`${import.meta.env.BASE_URL}brand/mark.svg`} alt="" className={cn("size-6", className)} />;
}
