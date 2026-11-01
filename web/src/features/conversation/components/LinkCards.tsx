import { useTranslation } from "react-i18next";
import { BookMarked, CircleDot, GitCommitHorizontal, GitPullRequest, type LucideIcon } from "lucide-react";
import type { Schemas } from "@/shared/api";
import { sanitizeUrl } from "@/shared/lib";
import { Badge } from "@/shared/ui";
import { cardTone, repoOf } from "../messageParts";

type Card = Schemas["LinkCard"];

const icons: Record<Card["kind"], LucideIcon> = { repo: BookMarked, pull: GitPullRequest, issue: CircleDot, commit: GitCommitHorizontal };

/**
 * Gitea cards for repo, PR, issue and commit links (round 11): title, state
 * and repo, linking to the public URL the server put on the card.
 */
export function LinkCards({ cards }: { cards: Card[] }) {
  const { t } = useTranslation("conversation");
  return (
    <ul className="mt-1.5 flex max-w-xl flex-col gap-1.5">
      {cards.map((c) => {
        const href = sanitizeUrl(c.url);
        const Icon = icons[c.kind] ?? BookMarked;
        const repo = repoOf(c.url);
        const body = (
          <>
            <Icon aria-hidden className="mt-0.5 size-4 shrink-0 text-fg-subtle" />
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate font-medium text-fg">{c.title || c.url}</span>
              <span className="flex flex-wrap items-center gap-x-2 text-meta text-fg-muted">
                <span>{t(`card.${c.kind}`)}</span>
                {repo ? <span className="font-mono">{repo}</span> : null}
                {c.author ? <span>@{c.author}</span> : null}
              </span>
            </span>
            {c.state ? <Badge tone={cardTone(c.state)}>{t(`card.state.${c.state.toLowerCase()}`, { defaultValue: c.state })}</Badge> : null}
          </>
        );
        return (
          <li key={c.url}>
            {href ? (
              <a href={href} target="_blank" rel="noopener noreferrer" className="flex items-start gap-2 rounded-md border border-border bg-surface px-2.5 py-2 text-ui hover:border-border-strong">
                {body}
              </a>
            ) : (
              <div className="flex items-start gap-2 rounded-md border border-border bg-surface px-2.5 py-2 text-ui">{body}</div>
            )}
          </li>
        );
      })}
    </ul>
  );
}
