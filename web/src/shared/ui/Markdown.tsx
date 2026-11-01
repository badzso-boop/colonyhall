import { Fragment, useMemo, useState, type ReactNode } from "react";
import { Check, Copy, FileText } from "lucide-react";
import { cn, parseMarkdown, sanitizeUrl, splitChips, type Block, type Chip, type FileLink, type Inline } from "@/shared/lib";

export type MarkdownProps = {
  text: string;
  /** Handles (without @) that exist; others stay plain text. */
  knownHandles?: Set<string>;
  onMention?: (handle: string) => void;
  /** Turns file references into buttons; return false to keep a reference as text. */
  onFileClick?: (link: FileLink) => void;
  canOpenFile?: (link: FileLink) => boolean;
  /**
   * Turns Q-12, D-12, [error:<id>] and [approval:<id>] in plain text into
   * whatever the caller renders (null hides the reference). Code spans are
   * never touched. Without it the references stay text.
   */
  renderChip?: (chip: Chip) => ReactNode;
  /** Chat messages: headings are small. */
  inChat?: boolean;
  copyLabel: string;
  copiedLabel: string;
  className?: string;
};

type Ctx = Omit<MarkdownProps, "text" | "className">;

function CodeBlock({ lang, text, ctx }: { lang: string; text: string; ctx: Ctx }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    void navigator.clipboard?.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };
  return (
    <div className="my-1.5 overflow-hidden rounded-md bg-terminal-bg text-terminal-fg">
      <div className="flex items-center justify-between px-2.5 pt-1 text-meta">
        <span className="font-mono lowercase opacity-70">{lang}</span>
        <button type="button" onClick={copy} className="inline-flex items-center gap-1 rounded-sm px-1.5 py-0.5 opacity-80 hover:opacity-100">
          {copied ? <Check aria-hidden className="size-3.5" /> : <Copy aria-hidden className="size-3.5" />}
          {copied ? ctx.copiedLabel : ctx.copyLabel}
        </button>
      </div>
      <pre className="overflow-x-auto px-2.5 pt-1 pb-2 font-mono text-ui leading-[1.45]">
        <code>{text}</code>
      </pre>
    </div>
  );
}

function withChips(text: string, render: (chip: Chip) => ReactNode): ReactNode {
  const parts = splitChips(text);
  if (parts.length === 1 && parts[0]!.type === "text") return text;
  return parts.map((p, i) => <Fragment key={i}>{p.type === "text" ? p.text : render(p)}</Fragment>);
}

function inline(nodes: Inline[] | undefined, ctx: Ctx): ReactNode {
  return (nodes ?? []).map((n, i) => {
    switch (n.type) {
      case "text":
        return <Fragment key={i}>{ctx.renderChip ? withChips(n.text, ctx.renderChip) : n.text}</Fragment>;
      case "code":
        return (
          <code key={i} className="rounded-sm border border-border bg-surface-sunken px-1 font-mono text-[0.9em]">
            {n.text}
          </code>
        );
      case "strong":
        return <strong key={i}>{inline(n.children, ctx)}</strong>;
      case "em":
        return <em key={i}>{inline(n.children, ctx)}</em>;
      case "strike":
        return <del key={i}>{inline(n.children, ctx)}</del>;
      case "br":
        return <br key={i} />;
      case "link": {
        const href = sanitizeUrl(n.href);
        if (!href) return <Fragment key={i}>{n.raw ?? n.text}</Fragment>;
        return (
          <a key={i} href={href} target="_blank" rel="noopener noreferrer" className="text-accent-text underline-offset-2 hover:underline">
            {inline(n.children ?? [{ type: "text", text: n.text }], ctx)}
          </a>
        );
      }
      case "mention": {
        if (!ctx.knownHandles?.has(n.handle)) return <Fragment key={i}>@{n.handle}</Fragment>;
        return (
          <button
            key={i}
            type="button"
            onClick={() => ctx.onMention?.(n.handle)}
            className="rounded-sm bg-accent-soft px-1 font-medium text-accent-text"
          >
            @{n.handle}
          </button>
        );
      }
      case "filelink": {
        if (!ctx.onFileClick || (ctx.canOpenFile && !ctx.canOpenFile(n))) return <Fragment key={i}>{n.raw ?? n.path}</Fragment>;
        return (
          <button
            key={i}
            type="button"
            title={n.path + (n.lines ? `:${n.lines}` : "")}
            onClick={() => ctx.onFileClick?.(n)}
            className="mx-0.5 inline-flex items-center gap-1 rounded-full border border-border bg-surface px-2 align-middle font-mono text-meta hover:border-border-strong"
          >
            <FileText aria-hidden className="size-3" />
            {n.name}
          </button>
        );
      }
      default:
        return null;
    }
  });
}

function list(b: Extract<Block, { type: "list" }>, ctx: Ctx, key: number): ReactNode {
  const Tag = b.ordered ? "ol" : "ul";
  return (
    <Tag key={key} className={cn("my-1 pl-5", b.ordered ? "list-decimal" : "list-disc")}>
      {b.items.map((it, i) => (
        <li key={i}>
          {it.task === "todo" ? "☐ " : it.task === "done" ? "☑ " : null}
          {inline(it.children, ctx)}
          {it.sublist ? list(it.sublist, ctx, 0) : null}
        </li>
      ))}
    </Tag>
  );
}

const headingSize = ["text-display", "text-page", "text-title", "text-body", "text-body", "text-body"];

/**
 * Safe markdown (docs/08, 10.): React elements only, links http(s) with
 * noopener, no images, raw HTML stays text. The parser is shared/lib/markdown.
 */
export function Markdown({ text, className, ...ctx }: MarkdownProps) {
  const blocks = useMemo(() => parseMarkdown(text), [text]);
  return (
    <div className={cn("break-words", className)}>
      {blocks.map((b, i) => {
        switch (b.type) {
          case "heading": {
            const size = ctx.inChat ? "text-body" : headingSize[b.level - 1];
            const Tag = ctx.inChat ? "p" : (`h${Math.min(6, Math.max(1, b.level))}` as "h1");
            return (
              <Tag key={i} className={cn("mt-2 mb-1 font-semibold", size)}>
                {inline(b.children, ctx)}
              </Tag>
            );
          }
          case "paragraph":
            return (
              <p key={i} className="my-0.5 whitespace-pre-wrap">
                {inline(b.children, ctx)}
              </p>
            );
          case "codeblock":
            return <CodeBlock key={i} lang={b.lang} text={b.text} ctx={ctx} />;
          case "blockquote":
            return (
              <blockquote key={i} className="my-1 border-l-2 border-border-strong pl-3 text-fg-muted">
                {inline(b.children, ctx)}
              </blockquote>
            );
          case "list":
            return list(b, ctx, i);
          case "table":
            return (
              <div key={i} className="my-1.5 overflow-x-auto">
                <table className="border-collapse text-ui">
                  <thead>
                    <tr>
                      {b.headers.map((c, j) => (
                        <th key={j} className="border border-border bg-surface-sunken px-2 py-1 text-left font-semibold">
                          {inline(c, ctx)}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {b.rows.map((r, j) => (
                      <tr key={j}>
                        {r.map((c, k) => (
                          <td key={k} className="border border-border px-2 py-1">
                            {inline(c, ctx)}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            );
          case "hr":
            return <hr key={i} className="my-2 border-border" />;
          default:
            return null;
        }
      })}
    </div>
  );
}
