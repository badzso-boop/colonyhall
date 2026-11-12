// The terminal chamber (docs/18, 4.6). The terminal (always dark, as in the
// web app) types the install while it is on screen: the commands, then
// `colony setup`'s questions and answers with an invented machine name.
// When the install is done, a new little colony hatches around it and sets
// off towards the top of the page. The transcript is the page's own
// (prefilled from the i18n file), so without JavaScript, or with reduced
// motion, the finished transcript is simply there.
import { motion, wait, onVisibility } from "./motion.js";
import * as colony from "./colony.js";
import { smooth } from "./colony.js";

const term = document.getElementById("terminal");
const body = document.getElementById("terminal-body");
let visible = false;
const held = () => !visible;
const sleep = (ms) => wait(ms, held);

/** The transcript as lines of nodes (prompt, question and answer spans, text). */
function readLines() {
  const lines = [[]];
  for (const node of [...body.childNodes]) {
    if (node.nodeType === Node.TEXT_NODE) {
      const parts = node.textContent.split("\n");
      parts.forEach((part, i) => {
        if (i > 0) lines.push([]);
        if (part) lines[lines.length - 1].push(document.createTextNode(part));
      });
    } else {
      lines[lines.length - 1].push(node.cloneNode(true));
    }
  }
  return lines;
}

const cursor = document.createElement("span");
cursor.className = "term__cursor";
cursor.setAttribute("aria-hidden", "true");

async function typeInto(parent, text, speed) {
  const node = document.createTextNode("");
  parent.appendChild(node);
  for (let i = 0; i < text.length; i++) {
    node.textContent += text[i];
    await sleep(speed);
  }
}

async function play(lines) {
  body.replaceChildren(cursor);
  for (const [i, line] of lines.entries()) {
    const kind = line[0]?.className; // "p" command, "q" question, "ok" result
    const holder = document.createElement("span");
    body.insertBefore(holder, cursor);
    if (kind === "p") {
      holder.appendChild(line[0].cloneNode(true));
      await sleep(400);
      for (const n of line.slice(1)) await typeInto(holder, n.textContent, 32);
      await sleep(350);
    } else if (kind === "q") {
      for (const n of line) {
        if (n.className === "a") {
          await sleep(650);
          const a = document.createElement("span");
          a.className = "a";
          holder.appendChild(a);
          await typeInto(a, n.textContent, 60);
        } else holder.appendChild(n.cloneNode(true));
      }
      await sleep(250);
    } else {
      await sleep(320);
      for (const n of line) holder.appendChild(n.cloneNode(true));
    }
    if (i < lines.length - 1) body.insertBefore(document.createTextNode("\n"), cursor);
  }
}

/** Hatchlings come out around the terminal and head for the top of the page. */
function hatch() {
  const r = term.getBoundingClientRect();
  const { scale } = colony.limits();
  const n = innerWidth < 640 ? 2 : 4;
  for (let i = 0; i < n; i++) {
    const x = r.left + r.width * (0.15 + 0.7 * (i / Math.max(1, n - 1))) + (Math.random() - 0.5) * 20;
    const y = r.bottom + scrollY - 6;
    const path = smooth([
      { x, y },
      { x: x + (Math.random() - 0.5) * 120, y: y - r.height * 0.5 },
      { x: x + (Math.random() - 0.5) * 220, y: scrollY - 120 },
    ]);
    const ant = colony.spawn({ agent: "hatchling", path, untagged: true, speed: 0.8 + Math.random() * 0.4 });
    ant.grow = { from: 0.05, to: scale * 0.5, t: 0, dur: 0.9 };
    ant.scale = 0.05;
    ant.pauseLeft = 0.2 * i + 0.4;
  }
}

let looping = false;
async function loop(lines) {
  if (looping) return;
  looping = true;
  while (visible && !motion.reduced) {
    await play(lines);
    await sleep(300);
    hatch();
    await sleep(9000);
  }
  // leave the finished transcript
  body.replaceChildren();
  lines.forEach((line, i) => {
    for (const n of line) body.appendChild(n.cloneNode(true));
    if (i < lines.length - 1) body.appendChild(document.createTextNode("\n"));
  });
  looping = false;
}

export function start() {
  if (!term || motion.reduced) return;
  const lines = readLines();
  onVisibility(term, (v) => {
    visible = v;
    if (v && !looping && !motion.reduced) loop(lines);
  }, "0px 0px -20% 0px");
}
