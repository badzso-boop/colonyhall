// Types of the parser's output (parse.js).

export type Inline =
  | { type: "text"; text: string }
  | { type: "code"; text: string }
  | { type: "strong" | "em" | "strike"; children: Inline[] }
  | { type: "br" }
  | { type: "link"; href: string; text: string; children?: Inline[]; raw?: string }
  | { type: "mention"; handle: string }
  | {
      type: "filelink";
      path: string;
      name: string;
      raw?: string;
      lines?: string;
      startLine?: number;
      endLine?: number;
      agent?: string;
      isRef?: boolean;
      isRelative?: boolean;
    };

export type ListItem = { children: Inline[]; task: "todo" | "done" | null; sublist: ListBlock | null };
export type ListBlock = { type: "list"; ordered: boolean; items: ListItem[] };

export type Block =
  | { type: "heading"; level: number; children: Inline[] }
  | { type: "paragraph"; children: Inline[] }
  | { type: "codeblock"; lang: string; text: string }
  | { type: "blockquote"; children: Inline[] }
  | ListBlock
  | { type: "table"; headers: Inline[][]; rows: Inline[][][] }
  | { type: "hr" };

export type FileLink = Extract<Inline, { type: "filelink" }>;

export function parse(text: string): Block[];
export function parseInline(text: string): Inline[];
export function plain(text: string): string;
export function sanitizeUrl(url: string | null | undefined): string | null;
export const LOCAL_PREFIXES: string[];
