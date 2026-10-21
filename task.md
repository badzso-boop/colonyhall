# Feladatok és hibák

**Kiadva az Antigravitynek (2026-10-21, `docs/handoff/15-ui-round-prompt.md`, ág: `feature/ui-round`, külön worktree):** a bal sáv hibája, a „dolgozik…” jelzés, a szekciók, az elnevezett csatornák tagokkal, a részletesebb profil, az agent-adatlap, és a „Nyitott” három prompt-javítása (sandboxos mappák, hosszabb válasz, egy nyelv). Kimarad ebből a körből: markdown-megjelenítés, fájlok az üzenetekben.

Élő lista: amit a használat közben találunk. Új tétel a „Nyitott” alá kerül; ha kész, átkerül a „Kész” alá a dátummal és a commit azonosítójával. A nagyobb, tervezést igénylő dolgok a „Tervezendő” alatt vannak: ott előbb terv kell, megvalósítás csak külön döntés után.

## Nyitott

- [ ] **A sandboxos agentek a sandbox belső mappáját mondják „munkamappának”** (2026-10-21). A „melyik mappában vagytok?” kérdésre `@legy-sandbox` és `@trader-logs` a saját sandbox-könyvtárát (`~/.local/share/colonyhall/sandbox/<agent>`) írta, nem a kapott mappákat (`legymernok`, `trader`). A munkafutás promptjában meg kellene nevezni a kapott mappákat és a jogot (olvas/ír), hogy tudja, hol dolgozik.
- [ ] **A „hosszabb üzenetet” kérés ellenére is rövid a válasz** (2026-10-21). `@trader-claude` 601 karakterrel válaszolt, mert a közvetlen prompt alapból „≤600 karakter, hacsak nem kérnek többet”. Megfontolandó: a prompt mondja ki, hogy ha hosszabb összefoglalót kérnek, akár 8000 karakterig írhat.
- [ ] **Vegyes nyelv**: `@trader-logs` egy magyar válasz elejére angol mondatot írt („I am reviewing the logs now.”). A közös szabály („a csatorna nyelvén válaszolj”) nem elég erős a tömör módú (sandboxos) futásban.
- [ ] **A 10. telepítési lépés** (`docs/05-homelab-telepites.md`): az átadó fájl törlése a homelabon, a `CLAUDE.md` „Fejlesztői környezet” részének és az élő Claude Docnak a frissítése (a szerver a homelabon fut).
- [ ] **A tulajdonosi API-kulcs** a beszélgetésben szerepelt (2026-10-21): érdemes lecserélni (új kulcs, a régi visszavonása).

- [ ] **A bal oldali sávban a csatornák és a DM-ek egymás mellé kerülnek** (2026-10-21). Egymás alatt kellene lenniük, mint minden chatalkalmazásban. Ok: a sáv stílusa (`nav div { display:flex; … }`) a lista tárolójára (`#channels`) is vonatkozik, így az egy sorba rendezi az elemeit. Javítás: a szabály csak a listaelemekre vonatkozzon, a tároló legyen oszlop (`flex-direction: column`).

## Tervezendő (csak terv, megvalósítás külön döntéssel)

- [ ] **Markdown megjelenítése a felületen.** Az agentek markdownban írnak (félkövér, listák, kód, linkek), a fejlesztői oldal nyers szövegként mutatja. Terv kell: biztonságos megjelenítés (DOM-építés, nem `innerHTML`), mit támogatunk (félkövér, dőlt, kód, kódblokk, lista, link), a linkek csak `http(s)`-re, a PWA-ban ugyanez.
- [ ] **Fájlok az agentek üzeneteiben.** Az agentek a gépük helyi útvonalára linkelnek (`file:///home/…/scheduler.log`); ez a böngészőben nem nyílik meg, és a fájl tartalma nem látszik. Lehetséges irányok, el kell dönteni:
  - az agent a meglévő fájlmegosztással (`upload` MCP-eszköz) tölti fel, és a csatolmányt linkeli; a közös szabály ezt kérje;
  - az őr a helyi útvonalat felismeri, és ha a fájl az agent engedélyezett mappájában van, kérésre feltölti (méretkorláttal, titokszűréssel);
  - a felület a csatolmányt helyben megmutatja (szöveg, log, markdown, kép előnézet), letöltés gombbal.
  - Kérdések: ki láthatja (csak aki a csatornát), mekkora fájl, a titkok (`.env`, kulcsok) kiszűrése, a sandboxos agent hogyan tölt fel.

- [ ] **„Dolgozik…” / „gépel…” jelzés** (2026-10-21, Norbi kérése). Most semmi nem látszik, amíg egy agent dolgozik (csak a sandboxos munkafutás tesz egy 👀 reakciót), így nem tudni, történik-e valami. Terv:
  - **Az őr jelez a szervernek** a futás elején és végén (pl. `POST /api/v1/agent/activity` `{channel, thread, state: working|idle}`), hosszú futásnál 20–30 másodpercenként megújítja; ha az őr leáll, a jelzés magától lejár (kb. 60 mp).
  - **A szerver nem tárolja**, csak továbbadja: ideiglenes WebSocket-esemény (`agent.activity`), nem kerül az eseménynaplóba és az auditba, és senkit nem ébreszt.
  - **A fejlesztői oldalon** egy egyszerű sor a beviteli mező fölött: „@trader-logs dolgozik…” (több agentnél felsorolva), abban a csatornában vagy szálban, ahol a kérés volt.
  - **A PWA-ban** később rendes animáció (pötty-animáció, agentenként), ugyanerre az eseményre.
  - Kérdések: a sandboxos döntő futásnál is jelezzen-e (rövid), vagy csak a munkafutásnál és a közvetlen módban; jogosultság: csak az láthassa, aki a csatornát látja.

- [ ] **Csoportok (szekciók) a bal oldali sávban**, mint a Slack szekciói vagy a Discord kategóriái (2026-10-21). A csatornák, DM-ek és agentek saját, elnevezett csoportokba rendezhetők (pl. „Légymérnök”, „Homelab”, „Trader”), a csoport összecsukható, a sorrend állítható. Kérdések: a csoport csak a felhasználó saját nézete (személyes rendezés), vagy közös a munkatérben; egy csatorna több csoportban is lehet-e.
- [ ] **Új, elnevezett csatorna kiválasztott tagokkal** (2026-10-21), mint egy Slack- vagy Discord-csatorna létrehozása: név, leírás, privát/nyilvános, és a felületről kiválasztható, hogy mely agentek (és emberek) legyenek tagok, akár később is hozzáadhatók vagy kivehetők. Az alapok megvannak (privát csatorna, tagkezelés az API-n), a felületről hiányzik a létrehozás és a tagok kezelése. Kérdés: agent létrehozhat-e csatornát (most nem: csak ember).
- [ ] **Részletesebb felhasználói profil** (2026-10-21): avatar, megjelenített név, leírás/szerep, időzóna, elérhetőség; embernél és agentnél is.
- [ ] **Agent-adatlap jobb oldali panelben**, mint a Slack profil-panelje (2026-10-21). Az agent nevére kattintva jobb oldalt egy szélesebb oszlop nyílik, ami mindent megmutat róla:
  - alapadatok: név, kliens (Claude Code / Antigravity), modell, mód (közvetlen / sandboxos), állapot (fut / szünetel / leállítva);
  - hol dolgozik: gép, gyökérmappa, mappaengedélyek (olvas/ír), worktree és ág;
  - munkamenet és kontextus: a futó szálak munkamenetei, az utolsó futás tokenszáma a korláthoz (`max_session_tokens`) képest, mikor indul új munkamenet;
  - fogyasztás: napi tokenkeret és felhasználás, az utolsó futások (idő, eredmény, hiba-azonosító);
  - közös dolgok: a jegyzetfüzete az adott szálban, a foglalásai, a nyitott kérdései;
  - műveletek (adminnak): szüneteltetés/leállítás, beállítások szerkesztése, mappaengedélyek.
  - Kérdések: mi legyen mobilon (teljes képernyős lap a panel helyett); mit láthat egy nem-admin felhasználó.

## Kész

- [x] **Fix fejléc** (2026-10-21): a fejléc gombjai eddig kapcsolók voltak, a megnyitott panel gombja „Chat”-re váltott. Most minden fül a saját nevével látszik, külön „💬 Chat” fül van, az aktív ki van emelve; csatornára kattintva visszavált a chatre; a fejléc több sorba törve sem tolja el az elrendezést; mobilon kompaktabb. Közben javítva: a belépési űrlap belépés után is látszott (egy `display:flex` szabály felülírta a `hidden`-t).
- [x] **DM indítása `?` hibával** (2026-10-21, `5c40326`): a DM-mező a nevet kis- és nagybetűre érzékenyen, rejtett karakterekkel együtt kereste, és hiba helyett csak `?<név>`-et írt ki. Most kisbetűsít, levágja a láthatatlan karaktereket, ha nem találja, újratölti a névsort, és érthető hibát ír.
- [x] **A sandboxos agent hosszú válasza 600 karakternél levágódott** (2026-10-21, `5c40326`): a munkafutás eredményét az őr a rövid válaszként küldte. Most a közvetlen módhoz hasonlóan 8000 karakterig mehet.
- [x] **Agent létrehozása a felületről 500-as hibával** (2026-10-21, `7165604`): az űrlap `role: "agent"`-et küldött; most `member`, és a szerver 400-at ad érvénytelen szerepkörre.
- [x] **A párosító kód eltűnt a felületről** (2026-10-21, `e5df456`): az esemény utáni újrarajzolás törölte; most a lejáratig látszik.
