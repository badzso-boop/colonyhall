// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// The Workshop demo on the hero monitor (docs/18, 4.10). The visitor picks
// one of three prewritten wishes; the monitor gives its screen to the demo:
// Dori asks @builder, the Builder's steps arrive, an approval card waits for
// the visitor's Allow (or Not now), and the new extension's first message
// appears, with working buttons where it has them. @builder's ant carries
// the parts to the screen. A script, not an AI: the site makes no requests.
// With reduced motion the same happens at once, without the ant.
import { data, t, text } from "./i18n.js";
import { motion, wait } from "./motion.js";
import * as colony from "./colony.js";
import { smooth } from "./colony.js";
import * as monitor from "./monitor.js";

const panel = document.getElementById("build");
const log = document.getElementById("build-log");
const closeBtn = document.getElementById("build-close");
const live = document.getElementById("live");
const wishes = [...document.querySelectorAll("[data-wish]")];
const AGENT = "builder";

let run = 0; // every new wish (or Close) cancels the one before
let ant = null;

const pace = (ms) => wait(motion.reduced ? 0 : ms);

function docRect(el) {
  const r = el.getBoundingClientRect();
  return { left: r.left, right: r.right, top: r.top + scrollY, bottom: r.bottom + scrollY, width: r.width, height: r.height };
}

/** Text with `code` and **strong**, built from text nodes (never innerHTML). */
function rich(s) {
  const frag = document.createDocumentFragment();
  for (const part of String(s).split(/(`[^`]+`|\*\*[^*]+\*\*)/)) {
    if (!part) continue;
    if (part.startsWith("`")) {
      const c = document.createElement("code"); c.textContent = part.slice(1, -1); frag.append(c);
    } else if (part.startsWith("**")) {
      const b = document.createElement("strong"); b.textContent = part.slice(2, -2); frag.append(b);
    } else frag.append(part);
  }
  return frag;
}

function el(tag, cls, content) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (content !== undefined) e.append(content);
  return e;
}

function add(node) {
  const li = el("li", "build__item", node);
  log.append(li);
  log.scrollTop = log.scrollHeight;
  return li;
}

function message(kind, initial, who, body) {
  const m = el("div", "msg");
  const av = el("span", `av av--${kind}`, initial);
  av.setAttribute("aria-hidden", "true");
  const box = el("div");
  box.append(el("p", "msg__who", who), el("p", "msg__text", body));
  m.append(av, box);
  return m;
}

function step(s) {
  return el("p", "build__step", rich(s));
}

/** The approval card; resolves true for Allow, false for Not now. */
function approval(cardText) {
  const card = el("div", "card build__card");
  const title = el("p", "card__title", t("workshop.cardTitle"));
  const p = el("p", "card__text", rich(cardText));
  const actions = el("div", "card__actions");
  const allow = el("button", "btn btn--primary btn--sm", t("workshop.allow"));
  const deny = el("button", "btn btn--sm", t("workshop.deny"));
  allow.type = deny.type = "button";
  actions.append(allow, deny);
  card.append(title, p, actions);
  const decided = new Promise((resolve) => {
    allow.addEventListener("click", () => resolve(true), { once: true });
    deny.addEventListener("click", () => resolve(false), { once: true });
  });
  return { card, allow, decided: decided.then((ok) => {
    actions.replaceWith(el("p", ok ? "card__done" : "card__hint", ok ? t("workshop.allowed") : t("workshop.deniedShort")));
    return ok;
  }) };
}

function result(w) {
  const r = w.result;
  const body = el("div", "build__result");
  body.append(el("p", "", rich(text(r.text))));
  if (r.table) {
    const table = el("table", "build__table");
    const head = el("tr");
    for (const c of r.table.columns) head.append(el("th", "", text(c)));
    table.append(el("thead", "", head));
    const tb = el("tbody");
    for (const row of r.table.rows) {
      const tr = el("tr");
      for (const c of row) tr.append(el("td", "", text(c)));
      tb.append(tr);
    }
    table.append(tb);
    body.append(table);
  }
  if (r.lines) for (const l of r.lines) body.append(el("p", "build__line", rich(text(l))));
  if (r.buttons) {
    const row = el("div", "build__buttons");
    const note = el("p", "build__line");
    for (const b of r.buttons) {
      const btn = el("button", "btn btn--sm", text(b));
      btn.type = "button";
      btn.addEventListener("click", () => {
        for (const o of row.children) o.setAttribute("aria-pressed", String(o === btn));
        note.textContent = t("workshop.voted").replace("{who}", t("workshop.you")).replace("{label}", btn.textContent);
        log.scrollTop = log.scrollHeight;
      });
      btn.setAttribute("aria-pressed", "false");
      row.append(btn);
    }
    body.append(row, note);
  }
  return message("agent", text(w.bot).replace("@", "").charAt(0), text(w.bot), body);
}

/** @builder's ant comes to the foot of the screen with a parcel. */
function bringAnt() {
  if (motion.reduced) return null;
  const s = docRect(panel);
  const stand = { x: s.left + s.width * 0.3, y: s.bottom + 6 };
  const a = colony.recruit(AGENT, { x: -50, y: stand.y });
  a.carry = true;
  a.speedMul = 1.6;
  const arrived = colony.walk(a, smooth([{ x: a.x, y: a.y }, { x: (a.x + stand.x) / 2, y: stand.y + 8 }, stand]));
  return { a, arrived };
}

/** Allowed: the ant climbs onto the screen with the new part, and leaves. */
async function deliver(mine) {
  if (!ant) return;
  const { a, arrived } = ant;
  await arrived;
  if (mine !== run) return;
  const s = docRect(log);
  const slot = { x: s.left + s.width * 0.5, y: s.bottom - 30 };
  await colony.walk(a, smooth([{ x: a.x, y: a.y }, { x: (a.x + slot.x) / 2, y: (a.y + slot.y) / 2 + 12 }, slot]));
  a.carry = false;
  await pace(400);
  const right = { x: innerWidth + 60, y: slot.y - 40 };
  colony.walk(a, smooth([{ x: a.x, y: a.y }, { x: s.right - 10, y: s.top + 10 }, right])).then(() => colony.remove(a));
  ant = null;
}

function dismissAnt() {
  if (!ant) return;
  const { a } = ant;
  ant = null;
  colony.walk(a, smooth([{ x: a.x, y: a.y }, { x: -60, y: a.y }])).then(() => colony.remove(a));
}

async function play(w) {
  const mine = ++run;
  const alive = () => mine === run;
  dismissAnt();
  monitor.hold(true);
  panel.hidden = false;
  log.replaceChildren();
  // On a phone the monitor is below the wishes: bring it into view.
  const r = panel.getBoundingClientRect();
  if (r.top < 0 || r.bottom > innerHeight) panel.closest(".monitor").scrollIntoView({ block: "center", behavior: motion.reduced ? "auto" : "smooth" });
  for (const b of wishes) b.setAttribute("aria-pressed", String(b.dataset.wish === w.id));

  add(message("human", "D", t("monitor.dori"), rich(text(w.ask))));
  await pace(700);
  if (!alive()) return;
  ant = bringAnt();
  add(message("agent", "b", t("agents.builder"), t("workshop.onIt")));
  for (const s of w.steps) {
    await pace(850);
    if (!alive()) return;
    add(step(text(s)));
  }
  await pace(500);
  if (!alive()) return;
  const { card, allow, decided } = approval(text(w.card));
  add(card);
  allow.focus({ preventScroll: true });
  const ok = await decided;
  if (!alive()) return;
  if (ok) {
    await deliver(mine);
    if (!alive()) return;
    add(result(w));
    live.textContent = t("workshop.liveDone");
  } else {
    dismissAnt();
    add(message("agent", "b", t("agents.builder"), t("workshop.denied")));
    live.textContent = t("workshop.liveDenied");
  }
}

function close() {
  run++;
  dismissAnt();
  panel.hidden = true;
  log.replaceChildren();
  for (const b of wishes) b.setAttribute("aria-pressed", "false");
  monitor.hold(false);
}

export function start() {
  if (!panel || !wishes.length) return;
  const byId = Object.fromEntries((data.workshop?.wishes || []).map((w) => [w.id, w]));
  for (const b of wishes) {
    b.addEventListener("click", () => {
      const w = byId[b.dataset.wish];
      if (!w) return;
      if (b.getAttribute("aria-pressed") === "true") { close(); return; }
      play(w);
    });
  }
  closeBtn.addEventListener("click", () => { close(); wishes[0]?.focus(); });
  addEventListener("keydown", (e) => { if (e.key === "Escape" && !panel.hidden && !motion.stopped) close(); });
}
