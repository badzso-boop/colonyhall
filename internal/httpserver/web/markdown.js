// ColonyMarkdown: A safe, deterministic markdown renderer for Colonyhall chat and file viewer.
// Builds DOM nodes directly without innerHTML.
(function () {
  "use strict";

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
        try {
          let dec = decodeURIComponent(cand.slice(7));
          if (dec.startsWith("//")) dec = dec.slice(2);
          if (dec.startsWith("/") || dec.startsWith("~/")) {
            flush();
            const name = dec.split("/").pop() || dec;
            tokens.push({ type: "filelink", path: dec, name, raw: cand });
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
        if (cand.length > matchedPrefix.length) {
          flush();
          const name = cand.split("/").pop() || cand;
          tokens.push({ type: "filelink", path: cand, name, raw: cand });
          if (punct) curText += punct;
          i = j;
          continue;
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
  function render(blocksOrText, options = {}) {
    if (typeof document === "undefined") {
      throw new Error("ColonyMarkdown.render requires a DOM environment (window.document)");
    }

    const blocks = typeof blocksOrText === "string" ? parse(blocksOrText) : blocksOrText || [];
    const fragment = document.createDocumentFragment();

    const knownHandles = options.knownHandles
      ? options.knownHandles instanceof Set
        ? options.knownHandles
        : new Set(options.knownHandles)
      : null;

    function renderInlineNodes(nodes, container) {
      if (!nodes) return;
      for (const node of nodes) {
        switch (node.type) {
          case "text":
            container.appendChild(document.createTextNode(node.text));
            break;
          case "code": {
            const el = document.createElement("code");
            el.textContent = node.text;
            container.appendChild(el);
            break;
          }
          case "strong": {
            const el = document.createElement("strong");
            renderInlineNodes(node.children, el);
            container.appendChild(el);
            break;
          }
          case "em": {
            const el = document.createElement("em");
            renderInlineNodes(node.children, el);
            container.appendChild(el);
            break;
          }
          case "strike": {
            const el = document.createElement("del");
            renderInlineNodes(node.children, el);
            container.appendChild(el);
            break;
          }
          case "br":
            container.appendChild(document.createElement("br"));
            break;
          case "link": {
            const safeHref = sanitizeUrl(node.href);
            if (safeHref) {
              const a = document.createElement("a");
              a.href = safeHref;
              a.target = "_blank";
              a.rel = "noopener noreferrer";
              renderInlineNodes(node.children || [{ type: "text", text: node.text }], a);
              container.appendChild(a);
            } else {
              container.appendChild(document.createTextNode(node.raw || node.text || ""));
            }
            break;
          }
          case "filelink": {
            if (options.fileChips) {
              const btn = document.createElement("button");
              btn.type = "button";
              btn.className = "file-chip";
              btn.title = node.path;
              btn.textContent = "📄 " + node.name;
              btn.onclick = (e) => {
                e.stopPropagation();
                if (typeof options.onFileClick === "function") {
                  options.onFileClick(node.path);
                }
              };
              container.appendChild(btn);
            } else {
              container.appendChild(document.createTextNode(node.raw || node.path));
            }
            break;
          }
          case "mention": {
            const isKnown = knownHandles && (knownHandles.has(node.handle) || knownHandles.has("@" + node.handle));
            if (isKnown) {
              const span = document.createElement("span");
              span.className = "mention";
              span.textContent = "@" + node.handle;
              if (typeof options.onMention === "function") {
                span.style.cursor = "pointer";
                span.onclick = (e) => {
                  e.stopPropagation();
                  options.onMention(node.handle);
                };
              }
              container.appendChild(span);
            } else {
              container.appendChild(document.createTextNode("@" + node.handle));
            }
            break;
          }
          default:
            if (node.text) {
              container.appendChild(document.createTextNode(node.text));
            }
        }
      }
    }

    function renderList(listNode, container) {
      const listEl = document.createElement(listNode.ordered ? "ol" : "ul");
      listEl.className = "md-list";
      for (const item of listNode.items) {
        const li = document.createElement("li");
        if (item.task === "todo") {
          li.appendChild(document.createTextNode("☐ "));
        } else if (item.task === "done") {
          li.appendChild(document.createTextNode("☑ "));
        }
        renderInlineNodes(item.children, li);
        if (item.sublist) {
          renderList(item.sublist, li);
        }
        listEl.appendChild(li);
      }
      container.appendChild(listEl);
    }

    for (const block of blocks) {
      switch (block.type) {
        case "heading": {
          if (options.inChat) {
            const h = document.createElement("div");
            h.className = "chat-heading chat-h" + block.level;
            renderInlineNodes(block.children, h);
            fragment.appendChild(h);
          } else {
            const h = document.createElement("h" + Math.min(6, Math.max(1, block.level)));
            renderInlineNodes(block.children, h);
            fragment.appendChild(h);
          }
          break;
        }

        case "paragraph": {
          const p = document.createElement("p");
          p.className = "md-p";
          renderInlineNodes(block.children, p);
          fragment.appendChild(p);
          break;
        }

        case "codeblock": {
          const wrap = document.createElement("div");
          wrap.className = "code-block-wrap";

          const header = document.createElement("div");
          header.className = "code-block-header";

          const langSpan = document.createElement("span");
          langSpan.className = "code-lang";
          langSpan.textContent = block.lang || "";
          header.appendChild(langSpan);

          const copyBtn = document.createElement("button");
          copyBtn.type = "button";
          copyBtn.className = "copy-btn";
          copyBtn.textContent = options.copyText || "Másolás";
          copyBtn.onclick = (e) => {
            e.stopPropagation();
            const textToCopy = block.text;
            const onCopied = () => {
              copyBtn.textContent = options.copiedText || "Másolva!";
              setTimeout(() => {
                copyBtn.textContent = options.copyText || "Másolás";
              }, 2000);
            };
            if (navigator.clipboard && navigator.clipboard.writeText) {
              navigator.clipboard.writeText(textToCopy).then(onCopied, () => {});
            } else {
              const ta = document.createElement("textarea");
              ta.value = textToCopy;
              ta.style.position = "fixed";
              ta.style.opacity = "0";
              document.body.appendChild(ta);
              ta.select();
              try { document.execCommand("copy"); } catch {}
              document.body.removeChild(ta);
              onCopied();
            }
          };
          header.appendChild(copyBtn);
          wrap.appendChild(header);

          const pre = document.createElement("pre");
          const code = document.createElement("code");
          if (block.lang) code.className = "language-" + block.lang;
          code.textContent = block.text;
          pre.appendChild(code);
          wrap.appendChild(pre);

          fragment.appendChild(wrap);
          break;
        }

        case "blockquote": {
          const bq = document.createElement("blockquote");
          bq.className = "md-quote";
          renderInlineNodes(block.children, bq);
          fragment.appendChild(bq);
          break;
        }

        case "list":
          renderList(block, fragment);
          break;

        case "table": {
          const wrap = document.createElement("div");
          wrap.className = "md-table";

          const table = document.createElement("table");
          if (block.headers && block.headers.length > 0) {
            const thead = document.createElement("thead");
            const tr = document.createElement("tr");
            for (const cell of block.headers) {
              const th = document.createElement("th");
              renderInlineNodes(cell, th);
              tr.appendChild(th);
            }
            thead.appendChild(tr);
            table.appendChild(thead);
          }

          if (block.rows && block.rows.length > 0) {
            const tbody = document.createElement("tbody");
            for (const row of block.rows) {
              const tr = document.createElement("tr");
              for (const cell of row) {
                const td = document.createElement("td");
                renderInlineNodes(cell, td);
                tr.appendChild(td);
              }
              tbody.appendChild(tr);
            }
            table.appendChild(tbody);
          }

          wrap.appendChild(table);
          fragment.appendChild(wrap);
          break;
        }

        case "hr": {
          const hr = document.createElement("hr");
          hr.className = "md-hr";
          fragment.appendChild(hr);
          break;
        }
      }
    }

    return fragment;
  }

  // --- Plain text stripper for short previews ------------------------------
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
  const ColonyMarkdown = {
    parse,
    render,
    plain,
    parseInline
  };

  if (typeof window !== "undefined") {
    window.ColonyMarkdown = ColonyMarkdown;
  }
  if (typeof module !== "undefined" && module.exports) {
    module.exports = ColonyMarkdown;
  }
  if (typeof globalThis !== "undefined") {
    globalThis.ColonyMarkdown = ColonyMarkdown;
  }
})();
