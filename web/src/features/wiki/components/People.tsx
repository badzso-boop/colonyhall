import type { Actor } from "@/features/identity";
import { Avatar } from "@/shared/ui";

/** An actor's avatar by id: a circle for people, a square for agents. */
export function ActorAvatar({ actor, size = "sm" }: { actor?: Actor; size?: "sm" | "md" }) {
  if (!actor) return <Avatar kind="human" name="?" size={size} />;
  return <Avatar kind={actor.kind === "agent" ? "agent" : "human"} name={actor.display_name || actor.handle} emoji={actor.avatar_emoji} size={size} />;
}
