// The markdown parser of the chat and the file viewer, from the dev page's
// markdown.js (rounds 17 and 20), unchanged apart from being an ES module:
// it only parses (text → blocks); rendering is Markdown.tsx, which builds
// React elements (no innerHTML). Keep the two copies in step until the dev
// page is retired; the tests in parse.test.ts are the dev page's tests.
const LOCAL_PREFIXES = ["/home/", "/mnt/", "/root/", "/srv/", "/tmp/", "/var/", "/opt/", "~/"];

function isAlphaNum(c) {
  return !!c && /[a-zA-Z0-9]/.test(c);
}

function isWhitespace(c) {
  return !!c && /\s/.test(c);
}

function isPunct(c) {
  return !!c && /[!"#$%&'()*+,\-./:;<=>?@[\\\]^_`{|}~]/.test(c);
}

function isStopChar(c) {
  return !c || /\s/.test(c) || c === '"' || c === "'" || c === "`" || c === ")" || c === "]" || c === ">";
}

// GitHub-style table helpers
const mdTableRow = /^\s*\|.*\|\s*$/;
const mdTableSep = /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/;
const mdTableCells = (line) =>
  line.trim().replace(/^\|/, "").replace(/\|$/, "").split("|").map((c) => c.trim());
const isMdTableStart = (lines, i) =>
  mdTableRow.test(lines[i]) && i + 1 < lines.length && mdTableSep.test(lines[i + 1]);

function sanitizeUrl(url) {
  if (!url) return null;
  const trimmed = url.trim();
  // Only allow http: and https: protocols
  if (/^https?:\/\//i.test(trimmed)) {
    return trimmed;
  }
  return null;
}

function parseInline(text) {
  if (!text) return [];
  const tokens = [];
  let curText = "";

  function flush() {
    if (curText) {
      tokens.push({ type: "text", text: curText });
      curText = "";
    }
  }

  let i = 0;
  const n = text.length;

  while (i < n) {
    // 1. Backslash escape
    if (text[i] === "\\" && i + 1 < n && /[*_`~\[\]()\\>]/.test(text[i + 1])) {
      curText += text[i + 1];
      i += 2;
      continue;
    }

    // 2. Code span: `code`
    if (text[i] === "`") {
      const closeIdx = text.indexOf("`", i + 1);
      if (closeIdx !== -1) {
        flush();
        tokens.push({ type: "code", text: text.slice(i + 1, closeIdx) });
        i = closeIdx + 1;
        continue;
      } else {
        curText += "`";
        i++;
        continue;
      }
    }

    // 3. Link or image: [text](url) or ![alt](url)
    const isImg = text[i] === "!" && i + 1 < n && text[i + 1] === "[";
    if (text[i] === "[" || isImg) {
      const startBracket = isImg ? i + 1 : i;
      let closeBracket = -1;
      let depth = 0;
      for (let j = startBracket + 1; j < n; j++) {
        if (text[j] === "\\") { j++; continue; }
        if (text[j] === "[") depth++;
        else if (text[j] === "]") {
          if (depth === 0) { closeBracket = j; break; }
          depth--;
        }
      }
      if (closeBracket !== -1 && closeBracket + 1 < n && text[closeBracket + 1] === "(") {
        let closeParen = -1;
        for (let j = closeBracket + 2; j < n; j++) {
          if (text[j] === "\\") { j++; continue; }
          if (text[j] === ")") { closeParen = j; break; }
        }
        if (closeParen !== -1) {
          const raw = text.slice(i, closeParen + 1);
          const linkText = text.slice(startBracket + 1, closeBracket);
          const linkUrl = text.slice(closeBracket + 2, closeParen).trim();

          // Check if URL is local file target
          let fileTarget = null;
          if (linkUrl.startsWith("file://")) {
            try {
              let dec = decodeURIComponent(linkUrl.slice(7));
              if (dec.startsWith("//")) dec = dec.slice(2);
              if (dec.startsWith("/") || dec.startsWith("~/")) {
                fileTarget = dec;
              }
            } catch {}
          } else {
            for (const p of LOCAL_PREFIXES) {
              if (linkUrl.startsWith(p)) {
                fileTarget = linkUrl;
                break;
              }
            }
          }

          if (fileTarget) {
            flush();
            const name = linkText || fileTarget.split("/").pop() || fileTarget;
            tokens.push({ type: "filelink", path: fileTarget, name, raw });
            i = closeParen + 1;
            continue;
          }

          // Check for safe http(s) URL
          const safeHref = sanitizeUrl(linkUrl);
          if (safeHref) {
            flush();
            if (isImg) {
              const display = linkText || safeHref;
              tokens.push({
                type: "link",
                href: safeHref,
                text: display,
                children: [{ type: "text", text: display }],
                raw
              });
            } else {
              tokens.push({
                type: "link",
                href: safeHref,
                text: linkText,
                children: parseInline(linkText),
                raw
              });
            }
            i = closeParen + 1;
            continue;
          }

          // Unsafe or unsupported link: keep verbatim as plain text
          curText += raw;
          i = closeParen + 1;
          continue;
        }
      }
    }

    // 4. Bare http(s) URL
    if (
      (text.startsWith("https://", i) || text.startsWith("http://", i)) &&
      (i === 0 || !isAlphaNum(text[i - 1]))
    ) {
      let j = i + 7;
      while (j < n && !isStopChar(text[j])) j++;
      let url = text.slice(i, j);
      const trailingPunct = url.match(/[.,;:!?)]+$/);
      let punct = "";
      if (trailingPunct) {
        punct = trailingPunct[0];
        url = url.slice(0, url.length - punct.length);
      }
      flush();
      tokens.push({
        type: "link",
        href: url,
        text: url,
        children: [{ type: "text", text: url }],
        raw: url
      });
      if (punct) curText += punct;
      i = j;
      continue;
    }

    // 5. Bare file:// URL
    if (text.startsWith("file://", i) && (i === 0 || !isAlphaNum(text[i - 1]))) {
      let j = i + 7;
      while (j < n && !isStopChar(text[j])) j++;
      let cand = text.slice(i, j);
      const trailingPunct = cand.match(/[.,;:!?]+$/);
      let punct = "";
      if (trailingPunct) {
        punct = trailingPunct[0];
        cand = cand.slice(0, cand.length - punct.length);
      }
      let lineSuffix = "";
      let startLine = null, endLine = null;
      const lineMatch = cand.match(/:(\d+)(?:-(\d+))?$/);
      if (lineMatch) {
        lineSuffix = lineMatch[0].slice(1);
        startLine = parseInt(lineMatch[1], 10);
        endLine = lineMatch[2] ? parseInt(lineMatch[2], 10) : startLine;
        cand = cand.slice(0, cand.length - lineMatch[0].length);
      }
      try {
        let dec = decodeURIComponent(cand.slice(7));
        if (dec.startsWith("//")) dec = dec.slice(2);
        if (dec.startsWith("/") || dec.startsWith("~/")) {
          flush();
          const base = dec.split("/").pop() || dec;
          const name = lineSuffix ? base + ":" + lineSuffix : base;
          tokens.push({
            type: "filelink",
            path: dec,
            name,
            lines: lineSuffix,
            startLine,
            endLine,
            raw: text.slice(i, j - punct.length)
          });
          if (punct) curText += punct;
          i = j;
          continue;
        }
      } catch {}
    }

    // 6. Bare local path
    let matchedPrefix = null;
    for (const pfx of LOCAL_PREFIXES) {
      if (text.startsWith(pfx, i)) {
        matchedPrefix = pfx;
        break;
      }
    }
    if (matchedPrefix && (i === 0 || !/[a-zA-Z0-9/\\.:]/.test(text[i - 1]))) {
      let j = i + matchedPrefix.length;
      while (j < n && !isStopChar(text[j])) j++;
      let cand = text.slice(i, j);
      const trailingPunct = cand.match(/[.,;:!?]+$/);
      let punct = "";
      if (trailingPunct) {
        punct = trailingPunct[0];
        cand = cand.slice(0, cand.length - punct.length);
      }
      let lineSuffix = "";
      let startLine = null, endLine = null;
      const lineMatch = cand.match(/:(\d+)(?:-(\d+))?$/);
      if (lineMatch) {
        lineSuffix = lineMatch[0].slice(1);
        startLine = parseInt(lineMatch[1], 10);
        endLine = lineMatch[2] ? parseInt(lineMatch[2], 10) : startLine;
        cand = cand.slice(0, cand.length - lineMatch[0].length);
      }
      if (cand.length > matchedPrefix.length) {
        flush();
        const base = cand.split("/").pop() || cand;
        const name = lineSuffix ? base + ":" + lineSuffix : base;
        tokens.push({
          type: "filelink",
          path: cand,
          name,
          lines: lineSuffix,
          startLine,
          endLine,
          raw: text.slice(i, j - punct.length)
        });
        if (punct) curText += punct;
        i = j;
        continue;
      }
    }

    // 6.5 File references: @handle:path[:line[-line]], path:line[-line], Windows paths
    // Case A: Agent-prefixed reference @handle:path[:line[-line]]
    if (text[i] === "@" && (i === 0 || !isAlphaNum(text[i - 1]))) {
      const atMatch = text.slice(i).match(/^@([a-zA-Z0-9_-]+):([^\s"`\)\]>,]+)/);
      if (atMatch) {
        const agentHandle = atMatch[1];
        let remainder = atMatch[2];
        const fullMatchLen = atMatch[0].length;
        const trailingPunct = remainder.match(/[.,;:!?]+$/);
        let punct = "";
        if (trailingPunct) {
          punct = trailingPunct[0];
          remainder = remainder.slice(0, remainder.length - punct.length);
        }
        let lineSuffix = "";
        let startLine = null, endLine = null;
        const lineMatch = remainder.match(/:(\d+)(?:-(\d+))?$/);
        let filePath = remainder;
        if (lineMatch) {
          lineSuffix = lineMatch[0].slice(1);
          startLine = parseInt(lineMatch[1], 10);
          endLine = lineMatch[2] ? parseInt(lineMatch[2], 10) : startLine;
          filePath = remainder.slice(0, remainder.length - lineMatch[0].length);
        }
        // Check that filePath is non-empty, not purely numeric, and valid path
        if (filePath && !/^\d+$/.test(filePath) && !/:\/\//.test(filePath)) {
          flush();
          const base = filePath.split(/[/\\]/).pop() || filePath;
          const name = lineSuffix ? base + ":" + lineSuffix : base;
          tokens.push({
            type: "filelink",
            agent: agentHandle,
            path: filePath,
            name,
            lines: lineSuffix,
            startLine,
            endLine,
            isRef: true,
            isRelative: true,
            raw: text.slice(i, i + fullMatchLen - punct.length)
          });
          if (punct) curText += punct;
          i += fullMatchLen;
          continue;
        }
      }
    }

    // Case B: Relative reference or Windows path with line suffix (path:line[-line])
    if ((i === 0 || !/[a-zA-Z0-9/\\.:]/.test(text[i - 1])) && /[a-zA-Z0-9.~]/.test(text[i])) {
      // Find potential token boundary (capped at 500 chars)
      let j = i;
      while (j < n && j - i < 500 && !isStopChar(text[j]) && /[a-zA-Z0-9_.~/\\:-]/.test(text[j])) j++;
      let cand = text.slice(i, j);
      if (cand.includes(":")) {
        const trailingPunct = cand.match(/[.,;:!?]+$/);
        let punct = "";
        if (trailingPunct) {
          punct = trailingPunct[0];
          cand = cand.slice(0, cand.length - punct.length);
        }
        const lineMatch = cand.match(/:(\d+)(?:-(\d+))?$/);
        if (lineMatch) {
          const lineSuffix = lineMatch[0].slice(1);
          const startLine = parseInt(lineMatch[1], 10);
          const endLine = lineMatch[2] ? parseInt(lineMatch[2], 10) : startLine;
          const filePath = cand.slice(0, cand.length - lineMatch[0].length);
          const isWinDrive = /^[a-zA-Z]:[/\\]/.test(filePath);
          const isNumeric = /^\d+$/.test(filePath);
          const hasSlash = /[/\\]/.test(filePath);
          const hasExt = /\.[a-zA-Z0-9]{1,10}$/.test(filePath);

          if (!isNumeric && !/:\/\//.test(filePath) && (hasSlash || hasExt || isWinDrive)) {
            flush();
            const base = filePath.split(/[/\\]/).pop() || filePath;
            const name = base + ":" + lineSuffix;
            tokens.push({
              type: "filelink",
              path: filePath,
              name,
              lines: lineSuffix,
              startLine,
              endLine,
              isRef: true,
              isRelative: !isWinDrive,
              raw: text.slice(i, j - punct.length)
            });
            if (punct) curText += punct;
            i = j;
            continue;
          }
        }
      }
    }

    // 7. Mention: @handle
    if (text[i] === "@" && (i === 0 || !isAlphaNum(text[i - 1]))) {
      let j = i + 1;
      while (j < n && /[a-zA-Z0-9_-]/.test(text[j])) j++;
      if (j > i + 1) {
        const handle = text.slice(i + 1, j);
        flush();
        tokens.push({ type: "mention", handle });
        i = j;
        continue;
      }
    }

    // 8. Newline within paragraph
    if (text[i] === "\n") {
      flush();
      tokens.push({ type: "br" });
      i++;
      continue;
    }

    // 9. Delimiters: **, __, *, _, ~~
    let delim = null;
    let delimLen = 0;
    if (text.startsWith("**", i)) { delim = "*"; delimLen = 2; }
    else if (text.startsWith("__", i)) { delim = "_"; delimLen = 2; }
    else if (text.startsWith("~~", i)) { delim = "~"; delimLen = 2; }
    else if (text[i] === "*") { delim = "*"; delimLen = 1; }
    else if (text[i] === "_") { delim = "_"; delimLen = 1; }

    if (delim) {
      const prev = i > 0 ? text[i - 1] : null;
      const next = i + delimLen < n ? text[i + delimLen] : null;

      // Word-boundary rule: snake_case and identifiers (surrounded by alphanumerics) are NEVER emphasis!
      if (isAlphaNum(prev) && isAlphaNum(next)) {
        curText += text.slice(i, i + delimLen);
        i += delimLen;
        continue;
      }

      const prevWs = isWhitespace(prev) || prev === null;
      const nextWs = isWhitespace(next) || next === null;
      const prevPunct = isPunct(prev);
      const nextPunct = isPunct(next);

      const leftFlanking = !nextWs && (!nextPunct || prevWs || prevPunct);
      const rightFlanking = !prevWs && (!prevPunct || nextWs || nextPunct);

      let canOpen = leftFlanking;
      let canClose = rightFlanking;

      if (delim === "_") {
        canOpen = leftFlanking && (!rightFlanking || prevPunct);
        canClose = rightFlanking && (!leftFlanking || nextPunct);
      }

      if (canOpen || canClose) {
        flush();
        tokens.push({
          type: "delim",
          delim,
          len: delimLen,
          canOpen,
          canClose,
          raw: text.slice(i, i + delimLen)
        });
        i += delimLen;
        continue;
      } else {
        curText += text.slice(i, i + delimLen);
        i += delimLen;
        continue;
      }
    }

    curText += text[i];
    i++;
  }
  flush();

  return resolveDelimiters(tokens);
}

function resolveDelimiters(tokens) {
  const stack = [];
  for (let i = 0; i < tokens.length; i++) {
    const tok = tokens[i];
    if (tok.type !== "delim") continue;

    if (tok.canClose) {
      let matchIdx = -1;
      for (let s = stack.length - 1; s >= 0; s--) {
        const op = stack[s];
        if (op.delim === tok.delim && op.len === tok.len && op.canOpen) {
          matchIdx = s;
          break;
        }
      }

      if (matchIdx !== -1) {
        const opToken = stack[matchIdx];
        const opPos = tokens.indexOf(opToken);
        const children = tokens.slice(opPos + 1, i);
        let nodeType = "em";
        if (tok.len === 2) {
          nodeType = tok.delim === "~" ? "strike" : "strong";
        }
        const formattedNode = { type: nodeType, children };
        tokens.splice(opPos, i - opPos + 1, formattedNode);
        stack.splice(matchIdx);
        i = opPos;
        continue;
      }
    }

    if (tok.canOpen) {
      stack.push(tok);
    } else {
      tokens[i] = { type: "text", text: tok.raw };
    }
  }

  // Convert unmatched delimiters to text
  for (let i = 0; i < tokens.length; i++) {
    if (tokens[i].type === "delim") {
      tokens[i] = { type: "text", text: tokens[i].raw };
    }
  }

  return mergeTextTokens(tokens);
}

function mergeTextTokens(tokens) {
  const merged = [];
  for (const t of tokens) {
    if (t.type === "text") {
      if (merged.length > 0 && merged[merged.length - 1].type === "text") {
        merged[merged.length - 1].text += t.text;
      } else {
        merged.push({ type: "text", text: t.text });
      }
    } else {
      if (t.children && Array.isArray(t.children)) {
        t.children = mergeTextTokens(t.children);
      }
      merged.push(t);
    }
  }
  return merged;
}

// --- Main AST Parser -----------------------------------------------------
function parse(text) {
  if (!text) return [];
  if (text.length > 20000) {
    return [{ type: "paragraph", children: [{ type: "text", text }] }];
  }

  const lines = text.split(/\r?\n/);
  const blocks = [];
  let i = 0;
  const n = lines.length;

  while (i < n) {
    const line = lines[i];

    // Blank line
    if (line.trim() === "") {
      i++;
      continue;
    }

    // Fenced code block: ```
    if (line.trim().startsWith("```")) {
      const lang = line.trim().slice(3).trim();
      const codeLines = [];
      i++;
      while (i < n && !lines[i].trim().startsWith("```")) {
        codeLines.push(lines[i]);
        i++;
      }
      if (i < n) i++; // skip closing ```
      blocks.push({ type: "codeblock", lang, text: codeLines.join("\n") });
      continue;
    }

    // Heading: # ... ######
    const hMatch = /^(#{1,6})\s+(.*)$/.exec(line);
    if (hMatch) {
      blocks.push({
        type: "heading",
        level: hMatch[1].length,
        children: parseInline(hMatch[2].trim())
      });
      i++;
      continue;
    }

    // Horizontal Rule: ---, ***, ___
    if (/^\s*(-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
      blocks.push({ type: "hr" });
      i++;
      continue;
    }

    // Table: GitHub style
    if (isMdTableStart(lines, i)) {
      const headerCells = mdTableCells(lines[i]).map((c) => parseInline(c));
      i += 2;
      const rows = [];
      while (i < n && mdTableRow.test(lines[i])) {
        const rowCells = mdTableCells(lines[i]).map((c) => parseInline(c));
        rows.push(rowCells);
        i++;
      }
      blocks.push({ type: "table", headers: headerCells, rows });
      continue;
    }

    // Blockquote: > ...
    if (/^\s*>\s?(.*)$/.test(line)) {
      const quoteLines = [];
      while (i < n && /^\s*>\s?(.*)$/.test(lines[i])) {
        const m = /^\s*>\s?(.*)$/.exec(lines[i]);
        quoteLines.push(m[1]);
        i++;
      }
      blocks.push({
        type: "blockquote",
        children: parseInline(quoteLines.join("\n"))
      });
      continue;
    }

    // Lists: Unordered or Ordered with one level of nesting (2+ spaces of indent)
    const ulMatch = /^\s*[-*]\s+/.test(line);
    const olMatch = /^\s*\d+\.\s+/.test(line);
    if (ulMatch || olMatch) {
      const isOrdered = olMatch;
      const listItems = [];

      while (i < n) {
        const curLine = lines[i];
        const isSubItem = /^( {2,}|\t)[-*]\s+/.test(curLine) || /^( {2,}|\t)\d+\.\s+/.test(curLine);
        const isTopItem = !isSubItem && (/^\s*[-*]\s+/.test(curLine) || /^\s*\d+\.\s+/.test(curLine));

        if (!isSubItem && !isTopItem) break;

        if (isTopItem) {
          let rawContent = curLine.replace(/^\s*[-*]\s+/, "").replace(/^\s*\d+\.\s+/, "");
          let task = null;
          const taskMatch = /^\[([ xX])\]\s+(.*)$/.exec(rawContent);
          if (taskMatch) {
            task = taskMatch[1].toLowerCase() === "x" ? "done" : "todo";
            rawContent = taskMatch[2];
          }
          listItems.push({
            children: parseInline(rawContent),
            task,
            sublist: null
          });
          i++;
        } else if (isSubItem) {
          if (listItems.length === 0) {
            let rawContent = curLine.trim().replace(/^[-*]\s+/, "").replace(/^\d+\.\s+/, "");
            listItems.push({ children: parseInline(rawContent), task: null, sublist: null });
            i++;
            continue;
          }
          const parentItem = listItems[listItems.length - 1];
          const subOrdered = /^( {2,}|\t)\d+\.\s+/.test(curLine);
          if (!parentItem.sublist) {
            parentItem.sublist = { type: "list", ordered: subOrdered, items: [] };
          }
          let rawContent = curLine.trim().replace(/^[-*]\s+/, "").replace(/^\d+\.\s+/, "");
          let task = null;
          const taskMatch = /^\[([ xX])\]\s+(.*)$/.exec(rawContent);
          if (taskMatch) {
            task = taskMatch[1].toLowerCase() === "x" ? "done" : "todo";
            rawContent = taskMatch[2];
          }
          parentItem.sublist.items.push({
            children: parseInline(rawContent),
            task
          });
          i++;
        }
      }

      blocks.push({
        type: "list",
        ordered: isOrdered,
        items: listItems
      });
      continue;
    }

    // Paragraph: single newlines kept as line breaks
    const pLines = [];
    while (
      i < n &&
      lines[i].trim() !== "" &&
      !lines[i].trim().startsWith("```") &&
      !/^(#{1,6})\s+/.test(lines[i]) &&
      !/^\s*(-{3,}|\*{3,}|_{3,})\s*$/.test(lines[i]) &&
      !isMdTableStart(lines, i) &&
      !/^\s*>\s?/.test(lines[i]) &&
      !/^\s*[-*]\s+/.test(lines[i]) &&
      !/^\s*\d+\.\s+/.test(lines[i])
    ) {
      pLines.push(lines[i]);
      i++;
    }
    blocks.push({
      type: "paragraph",
      children: parseInline(pLines.join("\n"))
    });
  }

  return blocks;
}

// --- DOM Renderer --------------------------------------------------------
function plain(text) {
  if (!text) return "";
  let s = text;
  // Fenced code blocks
  s = s.replace(/```[\s\S]*?```/g, (m) => {
    return " " + m.replace(/^```[^\n]*\n?/, "").replace(/\n?```$/, "") + " ";
  });
  // Inline code
  s = s.replace(/`([^`]+)`/g, "$1");
  // Markdown links: [text](url) -> text
  s = s.replace(/\[([^\]]+)\]\([^)]+\)/g, "$1");
  // Images: ![alt](url) -> alt
  s = s.replace(/!\[([^\]]*)\]\([^)]+\)/g, "$1");
  // Headings
  s = s.replace(/^#{1,6}\s+/gm, "");
  // Blockquotes
  s = s.replace(/^>\s?/gm, "");
  // Lists
  s = s.replace(/^\s*[-*]\s+(\[[ xX]\]\s+)?/gm, "");
  s = s.replace(/^\s*\d+\.\s+(\[[ xX]\]\s+)?/gm, "");
  // Tables: replace pipe with space
  s = s.replace(/\|/g, " ");
  // Delimiters
  s = s.replace(/\*\*([^*]+)\*\*/g, "$1");
  s = s.replace(/__([^_]+)__/g, "$1");
  s = s.replace(/(^|[^\w])\*([^*]+)\*([^\w]|$)/g, "$1$2$3");
  s = s.replace(/(^|[^\w])_([^_]+)_([^\w]|$)/g, "$1$2$3");
  s = s.replace(/~~([^~]+)~~/g, "$1");
  // Collapse whitespace
  return s.replace(/\s+/g, " ").trim();
}

// Export globally

export { parse, parseInline, plain, sanitizeUrl, LOCAL_PREFIXES };
