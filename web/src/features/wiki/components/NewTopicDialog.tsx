import { useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { X } from "lucide-react";
import { Button, Dialog, Input, toast } from "@/shared/ui";
import { useCreateTopic } from "../api";
import { MarkdownEditor } from "./MarkdownEditor";

/** "Új téma": title, tags (chips, suggested from the existing ones), the opening post. */
export function NewTopicDialog({ open, onOpenChange, knownTags }: { open: boolean; onOpenChange: (o: boolean) => void; knownTags: string[] }) {
  const { t } = useTranslation("wiki");
  const navigate = useNavigate();
  const create = useCreateTopic();
  const [title, setTitle] = useState("");
  const [tags, setTags] = useState<string[]>([]);
  const [tag, setTag] = useState("");
  const [body, setBody] = useState("");
  const addTag = (raw: string) => {
    const v = raw.trim().toLowerCase().replace(/^#/, "");
    if (v && !tags.includes(v)) setTags([...tags, v]);
    setTag("");
  };
  const onTagKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      addTag(tag);
    } else if (e.key === "Backspace" && !tag && tags.length) setTags(tags.slice(0, -1));
  };
  const submit = () =>
    create.mutate(
      { title: title.trim(), body, tags: tag.trim() ? [...tags, tag.trim().toLowerCase()] : tags },
      {
        onSuccess: (page) => {
          onOpenChange(false);
          navigate(`/wiki/${(page as { slug: string }).slug}`);
        },
        onError: (e) => toast(e.message, { tone: "danger" }),
      },
    );
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("newTopic")}
      size="lg"
      closeLabel={t("close")}
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
          <Button variant="primary" disabled={!title.trim() || !body.trim()} loading={create.isPending} onClick={submit}>
            {t("create")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("topicTitle")}</span>
          <Input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={200} />
        </label>
        <div className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("tagsLabel")}</span>
          <div className="flex flex-wrap items-center gap-1 rounded-md border border-border-strong px-1.5 py-1">
            {tags.map((x) => (
              <span key={x} className="flex items-center gap-0.5 rounded-full bg-surface-sunken py-0.5 pr-0.5 pl-2 text-meta">
                #{x}
                <button type="button" aria-label={t("removeTag", { tag: x })} onClick={() => setTags(tags.filter((y) => y !== x))} className="grid size-4 place-items-center rounded-full hover:bg-surface">
                  <X aria-hidden className="size-3" />
                </button>
              </span>
            ))}
            <input
              aria-label={t("tagsLabel")}
              list="colony-wiki-tags"
              value={tag}
              onChange={(e) => setTag(e.target.value)}
              onKeyDown={onTagKey}
              onBlur={() => tag.trim() && addTag(tag)}
              className="min-w-24 flex-1 bg-transparent px-1 text-ui outline-none"
            />
            <datalist id="colony-wiki-tags">
              {knownTags.filter((x) => !tags.includes(x)).map((x) => (
                <option key={x} value={x} />
              ))}
            </datalist>
          </div>
          <span className="text-meta text-fg-subtle">{t("tagsHint")}</span>
        </div>
        <MarkdownEditor value={body} onChange={setBody} label={t("body")} />
      </div>
    </Dialog>
  );
}
