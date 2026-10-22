# Feladatok és hibák

**Megvalósítva az Antigravityvel (2026-10-21, `docs/handoff/15-ui-round-report.md`, ág: `feature/ui-round`, külön worktree):** a bal sáv hibája, a „dolgozik…” jelzés, a szekciók, az elnevezett csatornák tagokkal, a részletesebb profil, az agent-adatlap, és a „Nyitott” három prompt-javítása (sandboxos mappák, hosszabb válasz, egy nyelv). Kimarad ebből a körből: markdown-megjelenítés, fájlok az üzenetekben.

Élő lista: amit a használat közben találunk. Új tétel a „Nyitott” alá kerül; ha kész, átkerül a „Kész” alá a dátummal és a commit azonosítójával. A nagyobb, tervezést igénylő dolgok a „Tervezendő” alatt vannak: ott előbb terv kell, megvalósítás csak külön döntés után.

**Kiadva az Antigravitynek (2026-10-22, `docs/handoff/16-file-links-prompt.md`, ág: `feature/file-links`, külön worktree):** az agentek helyi fájllinkjei a felületen (kattintásra az őr betölti, a szerver tárolja, csak ember nézheti, csak olvasható; kép 200 MB, más fájl 100 MB, a felületen állítható), és a levágott válasz végéről elveszett linkek külön üzenetben.

## Nyitott

- [ ] **A 10. telepítési lépés** (`docs/05-homelab-telepites.md`): az átadó fájl törlése a homelabon, a `CLAUDE.md` „Fejlesztői környezet” részének és az élő Claude Docnak a frissítése (a szerver a homelabon fut).
- [ ] **A tulajdonosi API-kulcs** a beszélgetésben szerepelt (2026-10-21): érdemes lecserélni (új kulcs, a régi visszavonása).

## Tervezendő (csak terv, megvalósítás külön döntéssel)

- [ ] **Markdown megjelenítése a felületen.** Az agentek markdownban írnak (félkövér, listák, kód, linkek), a fejlesztői oldal nyers szövegként mutatja. Terv kell: biztonságos megjelenítés (DOM-építés, nem `innerHTML`), mit támogatunk (félkövér, dőlt, kód, kódblokk, lista, link), a linkek csak `http(s)`-re, a PWA-ban ugyanez.
- [ ] **Fájlok az agentek üzeneteiben.** Az agentek a gépük helyi útvonalára linkelnek (`file:///home/…/scheduler.log`); ez a böngészőben nem nyílik meg, és a fájl tartalma nem látszik. Lehetséges irányok, el kell dönteni:
  - az agent a meglévő fájlmegosztással (`upload` MCP-eszköz) tölti fel, és a csatolmányt linkeli; a közös szabály ezt kérje;
  - az őr a helyi útvonalat felismeri, és ha a fájl az agent engedélyezett mappájában van, kérésre feltölti (méretkorláttal, titokszűréssel);
  - a felület a csatolmányt helyben megmutatja (szöveg, log, markdown, kép előnézet), letöltés gombbal.
  - Kérdések: ki láthatja (csak aki a csatornát), mekkora fájl, a titkok (`.env`, kulcsok) kiszűrése, a sandboxos agent hogyan tölt fel.

## Kész

- [x] **A bal oldali sávban a csatornák és a DM-ek egymás mellé kerülnek** (2026-10-21, `e548634`): a lista tárolója oszlop elrendezésűvé vált, a flex sor kizárólag a csatorna- és DM-elemekre (`.chan-item`) korlátozódik.
- [x] **„Dolgozik…” / „gépel…” jelzés** (2026-10-21, `6595e08`): rövid életű tevékenységjelzés `POST /api/v1/agent/activity` végponttal, 60 mp-es memóriabeli lejárattal, `agent.activity` efemer WebSocket kerettel, a felületen az üzenetdoboz feletti aktivitás-sorral.
- [x] **Csoportok (szekciók) a bal oldali sávban** (2026-10-21, `ddd6702`): személyes szekciók (`sidebar_section`, `sidebar_item`), CRUD REST API (létrehozás, átnevezés, átrendezés, összecsukás, törlés, csatornák be- és kihelyezése), felületi vezérlőkkel és collapse nyilakkal.
- [x] **Új, elnevezett csatorna kiválasztott tagokkal** (2026-10-21, `7f8b0e4`): csatorna leírás (max 250 karakter, `00023_channel_description_members.sql`), `GET /channels/{id}/members`, `DELETE /channels/{id}/members/{actorId}` (soft-delete `left_at`-tal, utolsó emberi admin védelmével), felületi "+ új csatorna" modális ablak szűrhető tagválasztóval, és a fejlécben "Tagok" kezelőablak.
- [x] **Részletesebb felhasználói profil** (2026-10-21, `14a0014`): opcionális mezők (`description`, `avatar_emoji`, `timezone`, `status_text`, `00024_actor_profile.sql`), `PATCH /api/v1/actors/{id}/profile`, IANA időzóna-ellenőrzés, egyedi emoji validáció, ön- és adminisztrátori szerkesztés.
- [x] **Agent-adatlap jobb oldali panelben** (2026-10-21, `1b99600`): `GET /api/v1/agents/{id}/overview` aggregált végpont, asztali gépen jobbra nyíló slide-over panel, mobilon (≤640px) teljes képernyős sheet. Mutatja az alapadatokat és profilt, gépet és worktree-t, szálankénti munkamenetet és tokentelítettséget ("új munkamenet hamarosan"), napi keretet és legutóbbi 10 futást hibalinkkel, jegyzeteket, foglalásokat, nyitott kérdéseket, aktivitást; adminisztrátori szüneteltetés/leállítás/folytatás gombokat és mappaengedély-kezelést; embernél közös csatornákat és profilt.
- [x] **A sandboxos agentek a sandbox belső mappáját mondják „munkamappának”** (2026-10-21, `ce758c1`): a `WorkPrompt` felsorolja a kapott valós mappákat olvasási/írási joggal („Your folders: /home/…/trader (read-only). Work only in these…”).
- [x] **A „hosszabb üzenetet” kérés ellenére is rövid a válasz** (2026-10-21, `ce758c1`): a közvetlen és a munkafutás promptja is kiegészült: „when asked for a longer summary or report, you may write up to 8000 characters.”
- [x] **Vegyes nyelv** (2026-10-21, `ce758c1`): a közvetlen és a munkafutás promptja explicit rögzíti: "Answer in the language of the message you are answering; do not mix languages."
- [x] **Fix fejléc** (2026-10-21): a fejléc gombjai eddig kapcsolók voltak, a megnyitott panel gombja „Chat”-re váltott. Most minden fül a saját nevével látszik, külön „💬 Chat” fül van, az aktív ki van emelve; csatornára kattintva visszavált a chatre; a fejléc több sorba törve sem tolja el az elrendezést; mobilon kompaktabb. Közben javítva: a belépési űrlap belépés után is látszott (egy `display:flex` szabály felülírta a `hidden`-t).
- [x] **DM indítása `?` hibával** (2026-10-21, `5c40326`): a DM-mező a nevet kis- és nagybetűre érzékenyen, rejtett karakterekkel együtt kereste, és hiba helyett csak `?<név>`-et írt ki. Most kisbetűsít, levágja a láthatatlan karaktereket, ha nem találja, újratölti a névsort, és érthető hibát ír.
- [x] **A sandboxos agent hosszú válasza 600 karakternél levágódott** (2026-10-21, `5c40326`): a munkafutás eredményét az őr a rövid válaszként küldte. Most a közvetlen módhoz hasonlóan 8000 karakterig mehet.
- [x] **Agent létrehozása a felületről 500-as hibával** (2026-10-21, `7165604`): az űrlap `role: "agent"`-et küldött; most `member`, és a szerver 400-at ad érvénytelen szerepkörre.
- [x] **A párosító kód eltűnt a felületről** (2026-10-21, `e5df456`): az esemény utáni újrarajzolás törölte; most a lejáratig látszik.
