# 31. kör jelentése: a PWA alapja

Claude Code, 2026-10-30.

- **Ág:** `feature/pwa-foundation`, külön worktree-ben (`/mnt/g/Projects/new-chat-worktrees/pwa-foundation`).
- **Kiindulás:** a `feature/pwa-scaffold` ág, ami még nincs a `main`-ben. Beolvasztáskor előbb azt kell bevinni, utána ezt.

## Commitok

| Commit | Rész | Mit |
| --- | --- | --- |
| `c83910b` | 0. | Szerződés: WebSocket-üzenetek és eseménytartalmak sémái, TypeScript-generálás (`make gen`), típusos kliens, `reply_participant_ids` |
| `ea9acd4` | 2. | Design system: tokenek, betűtípusok, 18 komponens, `/app/design`, `docs/07` |
| `5832c05` | 3. + 1. | Logó, PWA-ikonok, manifest, service worker |
| `4e6dd9d` | 9. | Adatréteg és belépés |
| `23e4a0d` | – | Javítás: az ESLint betöltése az architektúra-teszt előtt (terhelés alatt időtúllépés volt) |
| `b3f8d15` | 4. | A keret: fejléc, csoportosított oldalsáv, beszélgetés-nézet |
| `a69e3eb` | 5. | Panelkezelő szabályai és a biztonságos markdown |
| `36f86a2` | 5. | Hírfolyam, szálak, szerkesztő, panelek |
| `5d033a1` | 6. | Terminálpanel |
| `e16d145` | 7. | Fájlböngésző-panel |
| `3c6cf6c` | 8. | Billentyűparancsok, súgó, parancspaletta |
| utolsó | – | Dokumentáció és jelentés |

**Egy hibás commit:** a `4e6dd9d` egy elbukott teszttel ment be. A parancslánc a `grep`, nem a tesztek kilépési kódját nézte. A `23e4a0d` javította, azóta minden commit előtt a `make test-web` valódi kilépési kódját ellenőrzöm.

## Mi készült

### 0. Szerződés

- **Típusgenerálás:** a `make gen` mostantól az `openapi-typescript`-et is futtatja (`web/src/shared/api/schema.d.ts`), a `make gen-check` ezt is összeveti. A kliens (`openapi-fetch`) CSRF-fejlécet küld, és 401-nél a belépő oldalra visz.
- **WebSocket-sémák:** `WsActivity`, `WsProgress`, valamint `Event*` sémák a felület által olvasott eseményekhez. A leképezés a `WsEvent` leírásában van. A szerver viselkedése nem változott.
- **`WsHello`:** a `token` nem kötelező. A böngésző nem küldi, a sütis munkamenet azonosít; így működött eddig is, csak a szerződés írta rosszul.
- **`reply_participant_ids`:** a szál legfeljebb 3 utolsó válaszolója, a legfrissebb elöl, a visszavontak nélkül. Olvasáskor számolódik, mert az üzenetsorok append-only-k.
  - új tárfüggvény: `ReplyParticipants`, memóriás és Postgres-változattal;
  - storetest és e2e-teszt tartozik hozzá.
- **Nem készült:** az e2e-teszt, ami minden eseménysémát valódi tartalommal vetne össze. A prompt csak „ha olcsó” kérte, a valódi tartalmakat az `events.go` fájlokból vettem át.

### 1. A `web/` projekt

A váz a `feature/pwa-scaffold` ágon készült. Ebben a körben jött hozzá:
- **Futásidejű függőségek:** TanStack Query, Zustand, Radix UI, Lucide, saját szerverről jövő betűtípusok, `tailwind-merge`.
- **Fejlesztői függőségek:**
  - `openapi-typescript`;
  - `@types/node` a Node-os tesztekhez (a kontraszt- és színteszt a lemezről olvas).
- **Vitest-beállítás:** a `fsModuleCache` ki van kapcsolva, mert elavult `import.meta.glob`-eredményt tartott meg, és egy új funkció nyelvi fájljait nem látták a tesztek.

### 2–3. Design system és logó

- **Tokenek, mindkét témában:** sötét témában a piros ketté vált: `accent` a fehér szöveg alatti kitöltéshez, `accent-text` a piros szöveghez, mert egyetlen piros nem lehet mindkettőre jó.
- **Kontraszt:** a `contrast.test.ts` minden párt ellenőriz. A táblázat lent van.
- **Betűtípusok:** Inter és JetBrains Mono saját szerverről, Google Fonts nélkül.
- **Komponensek** (`shared/ui`), mindegyik teszttel:
  - Button, IconButton (kötelező címkével), mezők, Badge;
  - Avatar: az ember kör, az agent négyzet színes gyűrűvel;
  - Tooltip, Menu, Tabs;
  - Dialog (telefonon alulról felcsúszó lap), Sheet, Toast;
  - EmptyState, Skeleton, Kbd, HoldButton, Markdown, Lightbox.
- **Élő stílusoldal:** `/app/design`.
- **Logó:** a `web/scripts/brand.mjs` egyetlen geometriából állítja elő az SVG-ket és a PNG-ket. A PNG-kódolás a Node `zlib`-jével megy, mert a használható képkönyvtárak MPL- vagy LGPL-licencűek.
- **PWA:**
  - manifest;
  - service worker, ami csak az alkalmazás keretét cache-eli, az API-t soha;
  - új verziónál „Új verzió érhető el” értesítés „Frissítés” gombbal, magától nem tölt újra.

### 4. A keret

- **Fejléc:**
  - oldalsáv-kapcsoló, logó, morzsasor;
  - keresőmező, ami a palettát nyitja;
  - Válaszra vár számláló, „⚡ N dolgozik”;
  - adminnak Claude-keret gyűrű és vészleállító gomb (2 mp nyomva tartás);
  - felhasználói menü: téma, nyelv, billentyűparancsok, kijelentkezés.
  - Vészleállításkor az egész fejléc piros.
- **Oldalsáv:**
  - felül a saját szekciók, utána Csatornák, Közvetlen üzenetek;
  - AI-beszélgetések két alcsoporttal: „Te és az agentek”, „Agentek egymás közt”.
  - A besorolás tiszta, tesztelt függvény (`classifyConversation`).
- **Egyéb:**
  - újracsatlakozás-sáv 3 másodperc után;
  - megemlítésszám a fülcímben, piros pötty a faviconon;
  - telefonon alsó sáv, a beszélgetéslista a kezdőképernyő.

### 5. Hírfolyam, szálak, szerkesztő, panelek

- **Panelkezelő:** a `shared/panes` mind a 9 szabálya tesztelve, az állapot az URL-ben, a vissza gomb bezárja a panelt.
- **Hírfolyam:**
  - markdown (a fejlesztői oldal elemzője változatlanul, a 17 régi teszttel), megemlítés-kiemelés;
  - hosszú üzenet összecsukása, képnagyító, fájl-chipek, reakciók;
  - szál-összesítő sor;
  - régebbi üzenetek felgörgetéskor, „Új üzenetek” elválasztó, „↓ N új üzenet” gomb;
  - olvasottnak jelölés, ha alul vagy.
- **Szál-panel:** saját szerkesztővel.
- **Szerkesztő:**
  - `@` kiegészítés, `/` menü;
  - `/schedule`, `/daily`, `/cron`, `/answer`;
  - csatolás „+” menüből (telefonon kamerával), húzással vagy beillesztéssel; tálca folyamatjelzővel és újrapróbálással;
  - „Elküldve” értesítés „Visszavonás” gombbal;
  - csak-olvasás sáv.
- **„@agent dolgozik…” sor:** az élő lépéssel és az eltelt idővel.

### 6–7. Terminál és fájlböngésző

- **Terminálpanel:**
  - lista állapotjelvényekkel;
  - kimenet csak szövegként, futás közben 2 másodpercenként frissítve, automatikus görgetéssel;
  - másolható kimenet és SSH-sor;
  - adminnak leállítás megerősítéssel.
- **Fájlböngésző:**
  - lustán töltődő fa git-jelvényekkel, ág és ahead/behind, keresés;
  - Monaco a beépített példányból, témája a tokenekből;
  - markdown formázva vagy nyersen, képek, 2 MB felett eleje/vége nézet;
  - sorkijelölésből „Hivatkozás beszúrása” és „Link másolása”;
  - telefonon előbb a fa, utána a néző.
- **Közös csatorna:** a műveletekhez kell egy csatorna, amelyben a néző és az agent is tag. Ezt az agent-áttekintő `common_channels` mezője adja, így a fülek URL-formátuma nem változott.

### 8. Billentyűparancsok

- **Regiszter:** egyetlen regiszter (`shared/lib/shortcuts.ts`). Ugyanazon a hatókörön belül két azonos parancs nem regisztrálható, és gépelés közben nem sülnek el.
- **Súgó** (`?`) és **parancspaletta** (Ctrl K).
- **A Ctrl Shift T helyett Ctrl Shift X nyitja a terminálokat**, mert a Ctrl Shift T a böngésző „bezárt fül visszanyitása” parancsa, az oldal nem kaphatja meg.
- **Az `e` (reakció) parancs kimaradt**, mert a reakció-felület a 34. kör része.

## Döntések, ahol a feladat nyitott volt

- **A panelkezelő a `shared/panes`-ben van**, nem a `layout/panes`-ben. Funkciók is nyitnak panelt, a funkciók pedig nem importálhatnak a `layout`-ból. A kirajzolás a `layout`-ban maradt. A `docs/08` ezt rögzíti.
- **Állapotkezelés:** TanStack Query és Zustand, ahogy Norbi döntött.
- **Közepes szélesség (768–1279 px):** az oldalsáv 1024 px alatt fiók, nem ikon-sín.
- **Méretezhető oszlopok:** az oldalsáv és a jobb panel szélessége rögzített (260 / 440 px), nem húzható.
- **Fejléc „dolgozik” jelzése:** a lista tooltipben jelenik meg, nem felugró ablakban.
- **Témaváltó a stílusoldalon:** a két téma nem egymás mellett látszik, hanem váltani lehet köztük, mert a `.dark` osztály az egész oldalra vonatkozik.
- **A gép által generált fájlok** (`schema.d.ts`, ikonok) commitolva vannak, ahogy az `apispec` is.

## Ami kimaradt, és miért

- **A Docker-image buildje nem futott**, mert ezen a gépen nincs Docker. A `node:22-alpine` fázist egyszer meg kell építeni.
- **A `make test-pg` nem futott** ugyanezért. A `ReplyParticipants` Postgres-lekérdezését csak a memóriás tár szerződéstesztje ellenőrizte; a Postgres-ágat le kell futtatni.
- **A terminál- és a fájlpanelt valódi őrrel nem próbáltam ki**, mert ehhez a géphez nem csatlakozik őr. Komponens-tesztek fedik őket, álkiszolgálóval.
- **Kimaradt még:** a méretezhető oszlopok, az ikon-sín, a reakció-parancs, és az eseménysémák e2e-ellenőrzése (lásd fent).

## Eredmények

- `make test-web`: 179 Vitest-teszt és 34 Node-teszt, mind zöld. Ebben benne van a tipusellenőrzés és a lint is.
- `make test`: zöld.
- `go test -race ./internal/guard/ ./internal/e2e/`: ok.
- `make lint`: 0 hiba.
- `make gen-check` (helyi sqlc 1.31.1): nincs eltérés.
- `make test-pg`: nem futott (nincs Docker).

## Függőségek és licencek (futásidő)

| Csomag | Verzió | Licenc |
| --- | --- | --- |
| react, react-dom | 19.3.1 | MIT |
| react-router | 8.5.1 | MIT |
| @tanstack/react-query | 5.105.0 | MIT |
| zustand | 5.0.16 | MIT |
| radix-ui | 1.8.1 | MIT |
| lucide-react | 1.65.0 | ISC |
| i18next / react-i18next | 26.5.12 / 17.1.9 | MIT |
| openapi-fetch | 0.17.0 | MIT |
| tailwind-merge | 3.7.0 | MIT |
| @fontsource-variable/inter, jetbrains-mono | 5.3.0 | OFL-1.1 |

**Fejlesztői függőségek** (mind MIT, ISC vagy Apache): Vite 8, Vitest 5, TypeScript 6, ESLint 10, typescript-eslint, eslint-plugin-boundaries, openapi-typescript (TS 5-ös peer-függőséggel, `overrides`-szal fut TS 6-on).

Copyleft licencű függőség nincs.

## Buildméret (első betöltés, gzip)

- **Összesen:** körülbelül 214 KB.
- **Részletek:** JS 102 + 74 + 26 KB, CSS 11 KB.
- **Igény szerint töltődik:** a betűtípusok, és az oldalak (beszélgetés, stílusoldal), amelyek külön csomagban jönnek.

## Kontraszt (a `contrast.test.ts` értékei)

| Pár | Minimum | Világos | Sötét |
| --- | --- | --- | --- |
| `fg` / `surface` | 4.5:1 | 18.78 | 16.21 |
| `fg-muted` / `surface` | 4.5:1 | 5.92 | 6.80 |
| `fg-muted` / `surface-sunken` | 4.5:1 | 5.18 | 7.06 |
| `fg-subtle` / `surface` | 3:1 | 3.84 | 4.55 |
| `border-strong` / `surface` | 3:1 | 3.59 | 3.27 |
| `accent-text` / `surface` | 4.5:1 | 6.22 | 6.74 |
| `accent-text` / `accent-soft` | 4.5:1 | 5.32 | 5.87 |
| `accent-fg` / `accent` | 4.5:1 | 6.22 | 5.04 |
| `accent-fg` / `accent-hover` | 4.5:1 | 7.89 | 6.05 |
| `warning` / `warning-soft` | 4.5:1 | 5.22 | 6.74 |
| `success` / `success-soft` | 4.5:1 | 5.27 | 7.00 |
| `info` / `info-soft` | 4.5:1 | 5.53 | 6.37 |
| `terminal-fg` / `terminal-bg` | 4.5:1 | 15.38 | 15.38 |

## Képernyőképek (`docs/handoff/31-screens/`)

A képek a memóriás szerverről készültek, a fejlesztői adatokkal, belépve. Egyik oldalon sem volt konzolhiba.

| Kép | Méret és téma |
| --- | --- |
| `shell-*` | 1440 sötét és világos, 1024 sötét, 390 világos |
| `thread-work-*` | 1440 sötét és világos: szál jobbra, agent-áttekintő a bal helyen |
| `thread-*` | 1024 világos, 390 sötét |
| `home-390-dark` | telefonon a beszélgetéslista |
| `login-*` | belépés |
| `design-*` | a stílusoldal mindkét témában, egész oldal |
| `palette-1440-dark` | parancspaletta |
| `help-1440-light` | billentyűparancsok súgója |

## Mit nézzen meg először az átnéző

1. **`web/src/shared/panes/panes.ts` és a tesztjei:** ezek a 31. kör szabályai, minden későbbi felület erre épül.
2. **`web/src/shared/ui/Markdown.tsx`:** a biztonsági szabályok (linkek, képek, HTML) és a hozzájuk tartozó tesztek.
3. **`internal/modules/conversation`:** a `ReplyParticipants` mindkét tárban, és a Postgres-lekérdezés (a `make test-pg` még hátravan).
4. **`web/src/shared/styles/tokens.css` és `contrast.test.ts`:** a sötét téma két pirosa.
5. **`web/src/features/files/components/FileViewer.tsx`:** a Monaco betöltése és a téma; valódi őrrel még nem próbáltam ki.
