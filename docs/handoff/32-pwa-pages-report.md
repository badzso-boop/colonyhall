# 32. kör jelentése: oldalak és lebegő ablakok

Claude Code, 2026-10-31.

- **Ág:** `feature/pwa-pages`, külön worktree-ben (`/mnt/g/Projects/new-chat-worktrees/pwa-pages`), a `main`-ből indult (`f34df66`), beolvasztás előtt a `ecb6770`-ra rebase-elve.
- **Prompt:** `docs/handoff/32-pwa-pages-floating-prompt.md`.
- **Élesítés:** a szervert újra kell építeni (szerveroldali API-bővítés és új PWA-build). **Migráció nincs**, az őröket nem kell újraindítani (az őr nem használja az új végpontokat).

## Commitok

| Commit | Rész | Mit |
| --- | --- | --- |
| `3abb098` | szerver | `/schedules/preview`, `/leases?state=&since=`, a `WikiPageIndex` fórummezői, 42 napos naptárablak és szélesebb gyorsítótár |
| `a24c4c2` | 1. | Lebegő ablakok (állapot, mozgatás, méretezés, áttetszőség, kicsinyítés, telefonon Sheet) |
| `dc36524` | 2.–3. | Munkaterület-oldalak az oldalsávban és a palettában, Feladatok oldal és ablak |
| `0038282` | 4. | Naptár: hónap, hét, nap (telefonon 3 nap), feliratkozások, ablak, jelvény |
| `578de6f` | 5. | Jegyzetek: oldal és a nyitott szálat követő ablak |
| `455b52e` | 6. | Ütemezőablak, Ütemezések oldal, a szerkesztő „+” menüjében „Ütemezés…” |
| `bbf2dfe` | 7. | Wiki fórumként |
| `9869acb` | 8.–9. | Foglalások, Szabályok, Döntések |
| `882f500` | javítás | Elveszett kattintás a lebegő ablakon, új ablak a nyitott mellé, összecsukott diff |
| `63e4e67` | javítás | Rövid időformátum az ütemezéseknél, az órarács első címkéje |
| utolsó | – | Dokumentáció, képek, jelentés |

Minden commit előtt a `make test-web` valódi kilépési kódját néztem; a szerveres commit előtt `make test`, `make lint`, `make gen-check` és `make test-pg` is futott.

## Mi készült

### Szerver (szerződés-első, mind additív)

- **`POST /schedules/preview`:** ugyanazokkal a szabályokkal, mint a létrehozás; `{cron, timezone, next_runs}` (≤ 5 futás, egyszerinél 1). Nincs mellékhatás, nincs esemény; hibás időzítés 400, agent 403. E2e: napi, hétköznapi, októberi óraátállítás (Europe/Budapest), hibás cron, agent-token.
  - **Eltérés a prompttól:** a kérés csak az időzítést (`at`/`cron`/`timezone`) kéri, a szöveget és a csatornát nem; a válasz mezője `cron`, nem `summary_cron`. Az előnézethez nem kell más, és így nem kell érvényes csatorna a gépeléshez.
- **`GET /leases?state=active|all&since=`:** `all` mellett minden foglalás, ami `since` óta futott vagy véget ért; alapból 24 óra, legfeljebb 7 nap. Új tárfüggvény (`ListSince`, memória és Postgres), storetest, e2e (403 agentnek, 400 rossz paraméterre).
- **`WikiPageIndex`:** `created_by`, `post_count`, `last_author_id`, `participant_ids` (≤ 5, legutóbbi elöl). Új tárfüggvény (`PostAuthors`), storetest, e2e.
- **Naptár:** `/planner/events` legfeljebb **42 nap** (a 43 napos kérés 400); a gyorsítótár ablaka **az előző hónap 1. napja … most + 120 nap** (eddig most − 1 nap … + 60 nap). **A gyorsítótár nagyobb lett:** egy sűrű naptárnál nagyjából háromszor annyi előfordulás kerül a `planner_calendar_event` táblába.

### 1. Lebegő ablakok (`shared/windows`, `layout/windows`)

- Tiszta reducer (megnyitás, fókusz, mozgatás, méretezés, kicsinyítés, áttetszőség, nézetméret), `localStorage`-ben tárolva; a Zustand-tár csak becsomagolja.
- Címsor: Halványítás (Normál / Áttetsző 55 % / Halvány 20 %), Kis méret, Teljes oldal, Bezárás; Alt+nyilak és Alt+Shift+nyilak (a súgóban is).
- Kicsinyítve 48 px-es kör jelvénnyel; telefonon a legfelső ablak alsó Sheetben.
- **Egy új ablak** a már nyitott mellé, balra nyílik (ha nincs hely, lépcsőzetesen).

### 2. Navigáció

- Az oldalsáv „Munkaterület” blokkja: Feladatok ⧉, Naptár ⧉, Jegyzetek ⧉, Ütemezések, Wiki, Döntések, Foglalások, Szabályok (csak adminnak), Design system. A ⧉ hoverre (és fókuszra) látszik. Ugyanezek a parancspalettában.
- Útvonalak: `/tasks`, `/calendar`, `/notes`, `/schedules`, `/wiki`, `/wiki/:slug`, `/leases`, `/rules`, `/decisions`. A panelek mellettük is működnek (pl. a jegyzetből nyitott szál jobbra nyílik).
- Közös oldalfejléc: `shared/ui/PageHeader`.

### 3. Feladatok

Lista (Elkezdve, Ütemezve, összecsukott Befejezve) és Tábla (három oszlop, HTML5-húzással oszlopok között és oszlopon belül), a választás megmarad. Gyors hozzáadás, állapotkör (kattintásra léptet), ↑/↓ a húzás helyett (telefon, billentyűzet), részletek Sheetben markdownnal, szerkesztés, törlés megerősítéssel. Optimista állapotváltás; `planner.task_*` eseményekre frissül.

### 4. Naptár

- Hónap (6×7, hétfővel kezdődik, a mai nap piros körben, egész napos sávok, „09:30 Cím” sorok, „+N további” napi popover, a napra kattintva a Nap nézet), Hét (egész napos sáv, órarács 07:00-tól görgetve, egymást fedő események egymás mellett, piros „most” vonal), Nap (szélesebb blokkok helyszínnel).
- Billentyűk: M, W, D, ←, →, T (ma). Esemény részletei popoverben (telefonon Sheet).
- Naptárak popover: láthatóság naptáranként (böngészőnként), hozzáadás (név, ICS-link, szín a `--color-cal-N` tokenekből), frissítés, eltávolítás megerősítéssel, maszkolt URL, utolsó letöltés vagy hiba.
- Telefonon a hónap pöttyöket mutat és a választott nap napirendjét alatta; a Hét helyett 3 nap.
- Ablak: mini hónap (pötty, ahol esemény van) és a következő 8 nap napirendje; a napra kattintva odagörget.
- **Hiba, amit közben találtam:** az októberi hat hét az óraátállítás miatt 42 nap + 1 óra, amit a szerver elutasított volna. A kliens a kérés végét 42×24 órára vágja (a 6. hét utolsó órája esik ki). A Vitest ezért fix időzónában fut (`TZ=Europe/Budapest`), hogy a teszt bármelyik gépen elkapja.

### 5. Jegyzetek

- Oldal: bal oldalt agentenként, azon belül szálanként (szál címe, jegyzetszám, utolsó jegyzet ideje), ékezet- és kisbetű-független keresés, jobb oldalt a kiválasztott szál jegyzetei, „Szál megnyitása” gomb.
- Ablak: a szálpanelen nyitott szálat követi, a szál agent-szerzőinek jegyzeteit mutatja; 📌 rögzíthető. Nincs jegyzet-esemény, ezért 30 mp-enként (csak látható lapon), fókuszkor és szálváltáskor frissül.
- **Korlát:** csak a szálpanelt követi; a fő oszlopban nyitott DM-nek nincs „szála”, így ott üres állapotot mutat.

### 6. Ütemezések

- Ütemezőablak: Mit? (szöveg, csatorna; szálból indítva a szál chipként, levehető), Mikor? (Egyszer gyorsgombokkal: Ma 18:00, Holnap 09:00, Hétfő 09:00, 1 óra múlva; Naponta + csak munkanapokon; Hetente napválasztóval; Havonta 1–31; Haladó cron), időzóna (kereshető lista), „Pótolja egyszer / Kihagyja”. Alul magyar összefoglaló, a szerver előnézetéből az 5 következő futás és a cron.
- `buildCron`, `parseCron` (a visszaolvasás az ablak saját formáira), `describeRule`, `quickChips`: tiszta függvények tesztekkel (`features/schedules/rule.ts`).
- A szerkesztő „+” menüjében „Ütemezés…”: a szöveggel, csatornával és szállal előtöltve nyílik, mentés után a szerkesztő kiürül. Hogy a beszélgetés és az ütemezés funkció ne függjön egymástól körben, a kérés a `shared/lib/scheduleDraft` tárolón megy át, az ablakot a keret rajzolja ki.
- Oldal: táblázat (üzenet, hova, ismétlés szavakkal vagy a nyers cron, következő, utolsó, futások, állapot, hiba), Aktív / Kész / Törölt szűrő, adminnak „Mindenkié”, Megszakítás megerősítéssel, „Másolás új ütemezésbe” (szerkesztés nincs, ezt egy tipp is mondja).
- **Eltérés:** „utolsó nap” nincs, mert a `robfig/cron` nem ismeri az `L`-t; a 29–31. napnál a felület figyelmeztet, hogy rövidebb hónapban kimarad. Az üzenetmezőben nincs `@`-kiegészítés (sima szövegmező).

### 7. Wiki fórumként

- Index: címkék mint kategóriák (számmal, „Összes” elöl; telefonon chipek), témasorok (cím, két soros kivonat, címkék, szerző avatarja, bejegyzésszám, legfeljebb 3 résztvevő, utolsó aktivitás), rendezés (aktivitás, létrehozás, cím), keresés (`q`), „Új” jelölés, „Törölt” szűrő visszaállítással (csak embernek), „Új téma” ablak (cím, címkék chipként ajánlással, markdown írás/előnézet).
- Téma: bejegyzéskártyák (avatar, név, `@handle`, idő, változat, markdown), „Link” és „Idézet”, válaszmező, emberi műveletek: Szerkesztés (figyelmeztet, hogy az egész témát átírja; ütközésnél érthető hiba), Előzmények (két változat különbsége), Törlés.
- `wikiPosts(revisions)` tiszta függvény, a prompt mind a négy esetével tesztelve.
- „Új” jelölés: amit a néző már megnyitott és azóta változott, vagy amit még nem nyitott meg és egy hétnél fiatalabb (különben az első látogatáskor minden „Új” lenne).
- **Nem készült:** a `D-12` / `Q-12` chipek. A chatben sincsenek még meg a PWA-ban; a 34. körben (chat) érdemes egyszerre, a közös markdown-megjelenítőben.

### 8. Foglalások

Rendezhető táblázat ragadós fejléccel (erőforrás monóban másolással, tartó négyzet avatarral és az agent-áttekintő panelre mutató linkkel, megjegyzés, megszerezve, élő visszaszámlálás, 5 percen belül borostyán, lejárt piros, hátralévő idő sávja, állapot a feloldóval), Aktív / Előzmények (7 nap), tartó szerinti szűrő, szabad szöveges szűrő, Felszabadítás megerősítéssel (erőforrás és tartó a szövegben), az új foglalás egyszer felvillan (csökkentett mozgásnál átmenet nélkül). Telefonon kártyák.

### 9. Szabályok

Olvasó nézet (a `{handle}`, `{admins}`, `{workspace}` súgós chipként), adminnak osztott szerkesztő (telefonon fülek) helyőrző-beszúró sávval és 8000-es számlálóval (90 %-tól borostyán), „Mentés új verzióként” különbséggel és a terv szerinti megjegyzéssel, verzió-idővonal jobbra (telefonon Sheet), bármelyik régi verzió összevetése a mostanival, „Előnézet @agentként”.

- **Diff:** saját sor szintű LCS (`shared/lib/diff.ts`, függőség nélkül); a hosszú változatlan szakaszok „… N változatlan sor …” sorrá csukódnak (`shared/ui/DiffView`, a wiki is ezt használja).
- **Korlátok:** a `{workspace}` előnézetben „Colonyhall” (a munkaterület nevét nem adja vissza API); a mentetlen változtatásnál a lap bezárása és a „Mégse” kérdez rá, az alkalmazáson belüli navigáció nem (a `BrowserRouter` mellett nincs `useBlocker`). Ha egy bekezdés helyőrzőt tartalmaz, az olvasó nézet azt sima szövegként mutatja (a chipek miatt), a többi bekezdés markdown.

### Döntések

Egyszerű, csak olvasható lista (`/decisions`): szám, szerző, idő, felülírja / felülírta jelzés, szöveg és indoklás markdownnal, link a beszélgetésre, keresés, „Továbbiak betöltése”.

## Hibák, amiket a valódi böngészőben találtam

- **Elveszett kattintás a lebegő ablakon:** ha nem a legfelső ablak gombjára kattintottál, az első kattintás csak előrehozta az ablakot. Ok: a fókusz z szerint újrarendezte a DOM-ot, és a böngésző ilyenkor elnyeli a kattintást. Javítás: fix DOM-sorrend, a rétegzés csak `z-index`. Teszt: `FloatingWindows.test.tsx` (a javítás nélkül elbukik).
- **Egymásra nyíló ablakok:** a második ablak 52 px-lel eltolva szinte teljesen eltakarta az elsőt. Most mellé nyílik (`placeFree`, teszttel).
- **Worker-időtúllépés a Vitestben** terhelt WSL-en: `maxWorkers: "50%"`.

## Tesztek

- `make test-web`: 47 fájl, 241 teszt (a 31. kör végén 33 fájl, 198 teszt), típusellenőrzés, ESLint (rétegszabályokkal).
- `make test`, `make lint`, `make gen-check`, `make test-pg` (a saját Postgresen) a szerveres résznél.

## Képek (`docs/handoff/32-screens/`)

A naptár és a foglalások képein az adatot a Playwright adja a böngészőben (a fejlesztői stack nem tud nyilvános https-ICS-t letölteni, foglalást pedig csak agent szerez MCP-n). A többi képen valódi, API-n át feltöltött adat van.

| Kép | Mit mutat |
| --- | --- |
| `windows-normal-dark.png`, `windows-auto-dark.png`, `windows-dim-light.png` | Két lebegő ablak a chat fölött, Normál / Áttetsző / Halvány módban |
| `windows-bubbles-dark.png` | Kicsinyített körök |
| `tasks-list-dark.png`, `tasks-board-light.png` | Feladatok: lista és tábla |
| `calendar-month-dark.png`, `calendar-week-light.png`, `calendar-day-dark.png`, `calendar-event-dark.png` | Naptár: hónap, hét, nap, esemény részletei |
| `calendar-month-phone.png` | Naptár telefonon (pöttyök és napirend) |
| `builder-Egyszer.png` … `builder-Haladó.png` | Az ütemezőablak mind az öt módban, a következő futásokkal |
| `schedules-dark.png` | Ütemezések táblázat |
| `wiki-index-dark.png`, `wiki-topic-light.png` | Wiki index és egy téma válaszokkal és „szerkesztette” sorral |
| `leases-dark.png`, `leases-phone.png` | Foglalások lejáró és lejárt foglalással, telefonon kártyák |
| `rules-diff-dark.png`, `rules-edit-light.png` | Szabályok: két verzió különbsége, szerkesztő előnézettel @agentként |

## Nyitva maradt

- `D-12`/`Q-12` chipek (34. kör, a chattel együtt).
- `@`-kiegészítés az ütemezőablak szövegmezőjében.
- Mentetlen szabályok alkalmazáson belüli navigációnál (adatrouterre váltással lenne `useBlocker`).
- A lebegő jegyzetablak a fő oszlop DM-jét nem követi.
