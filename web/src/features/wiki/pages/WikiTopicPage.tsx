import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router";
import { ArrowLeft, BookOpen, History, Link2, Pencil, Quote, Trash2 } from "lucide-react";
import { useActors, useSession, type Actor } from "@/features/identity";
import { ApiError } from "@/shared/api";
import { localeOf } from "@/shared/i18n";
import { fullTime, relativeTime } from "@/shared/lib";
import { Button, Dialog, DiffView, EmptyState, IconButton, Input, Markdown, PageHeader, Select, Sheet, Skeleton, toast } from "@/shared/ui";
import { markTopicSeen, useAppend, useDeleteTopic, useEditTopic, useWikiPage, useWikiRevisions, type WikiPage } from "../api";
import { MarkdownEditor } from "../components/MarkdownEditor";
import { ActorAvatar } from "../components/People";
import { quote, wikiPosts, type Post, type Revision } from "../posts";

function PostCard({ post, author, onQuote, canQuote }: { post: Extract<Post, { kind: "post" }>; author?: Actor; onQuote: () => void; canQuote: boolean }) {
  const { t, i18n } = useTranslation("wiki");
  const locale = localeOf(i18n.language);
  const id = `post-${post.rev}`;
  const copyLink = () => {
    const url = `${location.origin}${location.pathname}#${id}`;
    void navigator.clipboard?.writeText(url).then(() => toast(t("linkCopied")));
  };
  return (
    <article id={id} aria-label={`@${author?.handle ?? "?"}`} className="scroll-mt-4 rounded-lg border border-border bg-surface target:border-accent">
      <header className="flex items-center gap-2 border-b border-border px-3 py-2 text-meta text-fg-muted">
        <ActorAvatar actor={author} />
        <span className="font-semibold text-fg">{author?.display_name || author?.handle || "?"}</span>
        <span>@{author?.handle}</span>
        <time dateTime={post.at} title={fullTime(post.at, locale)}>
          {relativeTime(post.at, new Date(), locale)}
        </time>
        <span className="text-fg-subtle">· {post.opening ? t("opening") : t("rev", { rev: post.rev })}</span>
        <span className="ml-auto flex">
          <IconButton size="sm" label={t("link")} icon={<Link2 className="size-3.5" />} onClick={copyLink} />
          {canQuote ? <IconButton size="sm" label={t("quote")} icon={<Quote className="size-3.5" />} onClick={onQuote} /> : null}
        </span>
      </header>
      <Markdown text={post.body} className="px-4 py-3" copyLabel={t("copy")} copiedLabel={t("copied")} />
    </article>
  );
}

function Marker({ post, author }: { post: Exclude<Post, { kind: "post" }>; author?: Actor }) {
  const { t, i18n } = useTranslation("wiki");
  const key = post.kind === "edited" ? "edited" : post.kind === "deleted" ? "deletedBy" : "restoredBy";
  return (
    <div className="flex flex-col gap-2">
      <p className="flex items-center gap-2 text-meta text-fg-subtle before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border">
        {t(key, { handle: author?.handle ?? "?" })} · {relativeTime(post.at, new Date(), localeOf(i18n.language))}
      </p>
      {post.kind === "edited" && post.body ? (
        <div className="rounded-lg border border-dashed border-border p-3">
          <p className="mb-1 text-meta text-fg-subtle">{t("editedBody")}</p>
          <Markdown text={post.body} copyLabel={t("copy")} copiedLabel={t("copied")} />
        </div>
      ) : null}
    </div>
  );
}

function HistorySheet({ revisions, open, onOpenChange, actors }: { revisions: Revision[]; open: boolean; onOpenChange: (o: boolean) => void; actors: Map<string, Actor> }) {
  const { t, i18n } = useTranslation("wiki");
  const locale = localeOf(i18n.language);
  const sorted = useMemo(() => [...revisions].sort((a, b) => a.rev - b.rev), [revisions]);
  const last = sorted[sorted.length - 1]?.rev ?? 1;
  const [from, setFrom] = useState(Math.max(1, last - 1));
  const [to, setTo] = useState(last);
  const a = sorted.find((r) => r.rev === from)?.body ?? "";
  const b = sorted.find((r) => r.rev === to)?.body ?? "";
  const option = (r: Revision) => (
    <option key={r.rev} value={r.rev}>
      {r.rev}. {t(`kinds.${r.kind}`)} · @{actors.get(r.author_id)?.handle ?? "?"} · {new Date(r.created_at).toLocaleString(locale, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}
    </option>
  );
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title={t("history")} closeLabel={t("close")}>
      <div className="flex flex-col gap-3">
        <div className="grid grid-cols-2 gap-2">
          <label className="flex flex-col gap-1 text-meta font-semibold text-fg-muted">
            {t("from")}
            <Select value={from} onChange={(e) => setFrom(Number(e.target.value))}>
              {sorted.map(option)}
            </Select>
          </label>
          <label className="flex flex-col gap-1 text-meta font-semibold text-fg-muted">
            {t("to")}
            <Select value={to} onChange={(e) => setTo(Number(e.target.value))}>
              {sorted.map(option)}
            </Select>
          </label>
        </div>
        <DiffView a={a} b={b} label={t("history")} skipLabel={(n) => t("skip", { count: n })} />
      </div>
    </Sheet>
  );
}

function EditDialog({ page, open, onOpenChange }: { page: WikiPage; open: boolean; onOpenChange: (o: boolean) => void }) {
  const { t } = useTranslation("wiki");
  const edit = useEditTopic(page.slug);
  const [title, setTitle] = useState(page.title);
  const [tags, setTags] = useState(page.tags.join(", "));
  const [body, setBody] = useState(page.body);
  const save = () =>
    edit.mutate(
      { base_rev: page.rev, title: title.trim(), body, tags: tags.split(",").map((x) => x.trim().toLowerCase()).filter(Boolean) },
      {
        onSuccess: () => onOpenChange(false),
        onError: (e) => toast(e instanceof ApiError && e.status === 409 ? t("conflict") : e.message, { tone: "danger" }),
      },
    );
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("edit")}
      description={t("editWarn")}
      size="xl"
      closeLabel={t("close")}
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
          <Button variant="primary" loading={edit.isPending} disabled={!title.trim() || !body.trim()} onClick={save}>
            {t("save")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("topicTitle")}</span>
          <Input value={title} onChange={(e) => setTitle(e.target.value)} />
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("tagsLabel")}</span>
          <Input value={tags} onChange={(e) => setTags(e.target.value)} />
        </label>
        <MarkdownEditor value={body} onChange={setBody} label={t("body")} rows={16} />
      </div>
    </Dialog>
  );
}

/** A wiki page as a forum topic (round 32, part 7). */
export default function WikiTopicPage() {
  const { slug = "" } = useParams();
  const { t } = useTranslation("wiki");
  const page = useWikiPage(slug);
  const revisions = useWikiRevisions(slug);
  const actors = useActors();
  const session = useSession();
  const human = session.data?.actor.kind === "human";
  const append = useAppend(slug);
  const del = useDeleteTopic(slug);
  const [reply, setReply] = useState("");
  const [dialog, setDialog] = useState<"edit" | "history" | "delete" | null>(null);
  const posts = useMemo(() => wikiPosts(revisions.data ?? []), [revisions.data]);

  const updated = page.data?.updated_at;
  useEffect(() => {
    if (updated) markTopicSeen(slug, updated);
  }, [slug, updated]);

  if (page.isError)
    return (
      <div className="p-6">
        <EmptyState icon={<BookOpen className="size-8" />} title={t("notFound")} action={<Link to="/wiki">{t("back")}</Link>} />
      </div>
    );
  const p = page.data;
  const deleted = Boolean(p?.deleted_at);
  const send = () => append.mutate(reply.trim(), { onSuccess: () => setReply(""), onError: (e) => toast(e.message, { tone: "danger" }) });

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        icon={<BookOpen className="size-5" />}
        title={p?.title ?? "…"}
        actions={
          <>
            <Link to="/wiki" className="inline-flex h-7 items-center gap-1 rounded-md px-2 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg">
              <ArrowLeft aria-hidden className="size-4" />
              <span className="max-sm:sr-only">{t("back")}</span>
            </Link>
            {human && p ? (
              <>
                <Button size="sm" icon={<History className="size-4" />} onClick={() => setDialog("history")}>
                  {t("history")}
                </Button>
                {!deleted ? (
                  <>
                    <Button size="sm" icon={<Pencil className="size-4" />} onClick={() => setDialog("edit")}>
                      {t("edit")}
                    </Button>
                    <IconButton size="sm" label={t("delete")} icon={<Trash2 className="size-4" />} onClick={() => setDialog("delete")} />
                  </>
                ) : null}
              </>
            ) : null}
          </>
        }
      />
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex max-w-3xl flex-col gap-3 p-4">
          {p?.tags.length ? (
            <p className="flex flex-wrap gap-2 text-meta text-fg-subtle">
              {p.tags.map((x) => (
                <span key={x}>#{x}</span>
              ))}
            </p>
          ) : null}
          {deleted ? <p className="rounded-md bg-danger-soft px-3 py-2 text-ui text-danger">{t("isDeleted")}</p> : null}
          {revisions.isPending ? <Skeleton className="h-32 w-full" /> : null}
          {posts.map((x) =>
            x.kind === "post" ? (
              <PostCard
                key={x.rev}
                post={x}
                author={actors.data?.get(x.authorId)}
                canQuote={!deleted}
                onQuote={() => setReply((r) => r + quote(x.body, actors.data?.get(x.authorId)?.handle ?? "?"))}
              />
            ) : (
              <Marker key={x.rev} post={x} author={actors.data?.get(x.authorId)} />
            ),
          )}
          {p && !deleted ? (
            <section aria-label={t("reply")} className="flex flex-col gap-2 rounded-lg border border-border p-3">
              <MarkdownEditor value={reply} onChange={setReply} label={t("reply")} rows={4} placeholder={t("replyPlaceholder")} />
              <Button variant="primary" size="sm" className="self-end" disabled={!reply.trim()} loading={append.isPending} onClick={send}>
                {t("send")}
              </Button>
            </section>
          ) : null}
        </div>
      </div>
      {p && dialog === "edit" ? <EditDialog page={p} open onOpenChange={(o) => !o && setDialog(null)} /> : null}
      {dialog === "history" && revisions.data && actors.data ? (
        <HistorySheet revisions={revisions.data} actors={actors.data} open onOpenChange={(o) => !o && setDialog(null)} />
      ) : null}
      <Dialog
        open={dialog === "delete"}
        onOpenChange={(o) => !o && setDialog(null)}
        title={t("deleteTitle")}
        description={t("deleteBody", { title: p?.title ?? "" })}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setDialog(null)}>{t("cancel")}</Button>
            <Button variant="danger" loading={del.isPending} onClick={() => del.mutate(undefined, { onSuccess: () => setDialog(null) })}>
              {t("delete")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
    </div>
  );
}
