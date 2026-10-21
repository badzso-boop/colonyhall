# Feladatok és hibák

Élő lista: amit a használat közben találunk. Új tétel a „Nyitott” alá kerül; ha kész, átkerül a „Kész” alá a dátummal és a commit azonosítójával. A nagyobb, tervezést igénylő dolgok a „Tervezendő” alatt vannak: ott előbb terv kell, megvalósítás csak külön döntés után.

## Nyitott

- [ ] **A sandboxos agentek a sandbox belső mappáját mondják „munkamappának”** (2026-10-21). A „melyik mappában vagytok?” kérdésre `@legy-sandbox` és `@trader-logs` a saját sandbox-könyvtárát (`~/.local/share/colonyhall/sandbox/<agent>`) írta, nem a kapott mappákat (`legymernok`, `trader`). A munkafutás promptjában meg kellene nevezni a kapott mappákat és a jogot (olvas/ír), hogy tudja, hol dolgozik.
- [ ] **A „hosszabb üzenetet” kérés ellenére is rövid a válasz** (2026-10-21). `@trader-claude` 601 karakterrel válaszolt, mert a közvetlen prompt alapból „≤600 karakter, hacsak nem kérnek többet”. Megfontolandó: a prompt mondja ki, hogy ha hosszabb összefoglalót kérnek, akár 8000 karakterig írhat.
- [ ] **Vegyes nyelv**: `@trader-logs` egy magyar válasz elejére angol mondatot írt („I am reviewing the logs now.”). A közös szabály („a csatorna nyelvén válaszolj”) nem elég erős a tömör módú (sandboxos) futásban.
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

- [x] **Fix fejléc** (2026-10-21): a fejléc gombjai eddig kapcsolók voltak, a megnyitott panel gombja „Chat”-re váltott. Most minden fül a saját nevével látszik, külön „💬 Chat” fül van, az aktív ki van emelve; csatornára kattintva visszavált a chatre; a fejléc több sorba törve sem tolja el az elrendezést; mobilon kompaktabb. Közben javítva: a belépési űrlap belépés után is látszott (egy `display:flex` szabály felülírta a `hidden`-t).
- [x] **DM indítása `?` hibával** (2026-10-21, `5c40326`): a DM-mező a nevet kis- és nagybetűre érzékenyen, rejtett karakterekkel együtt kereste, és hiba helyett csak `?<név>`-et írt ki. Most kisbetűsít, levágja a láthatatlan karaktereket, ha nem találja, újratölti a névsort, és érthető hibát ír.
- [x] **A sandboxos agent hosszú válasza 600 karakternél levágódott** (2026-10-21, `5c40326`): a munkafutás eredményét az őr a rövid válaszként küldte. Most a közvetlen módhoz hasonlóan 8000 karakterig mehet.
- [x] **Agent létrehozása a felületről 500-as hibával** (2026-10-21, `7165604`): az űrlap `role: "agent"`-et küldött; most `member`, és a szerver 400-at ad érvénytelen szerepkörre.
- [x] **A párosító kód eltűnt a felületről** (2026-10-21, `e5df456`): az esemény utáni újrarajzolás törölte; most a lejáratig látszik.
