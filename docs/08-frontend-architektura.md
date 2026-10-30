# A PWA kódbázisának felépítése (`web/`)

**Kötelező minden agentnek és embernek, aki a `web/` mappában dolgozik.** Döntés: Norbi, 2026-10-30 (funkciók a backend moduljai szerint; TanStack Query + Zustand; a határokat az `eslint-plugin-boundaries` ellenőrzi). A cél ugyanaz, mint a backendnél: rend. Funkciónként külön modul, befelé mutató függőségek, egy modul a másikból csak a nyilvános felületét használhatja, és amit lehet, gép ellenőriz, nem fegyelem.

Kapcsolódó dokumentumok:

- `docs/07-design-system.md`: hogyan néz ki (színek, betűk, komponensek, modálok). A 31. kör írja meg.
- `docs/02-muszaki-specifikacio.md` „Felület”: a stack.
- `ui/ui-mockup/`: a kinézet mintái (sima HTML + CSS).
- `docs/handoff/31…35-*-prompt.md`: a PWA körei.

Ha ez a dokumentum és egy handoff prompt ellentmond egymásnak, **ez a dokumentum az irányadó**. Az ellentmondást a kör jelentésében jelezni kell.

---

## 1. Stack

Mindenből a legfrissebb stabil kiadás, pontos verzióval és lockfile-lal (`web/package.json`, `web/package-lock.json`).

| Mire | Mi |
| --- | --- |
| Felület | React 19, TypeScript (`strict`) |
| Build és fejlesztői szerver | Vite |
| Stílus | Tailwind CSS v4 (CSS-ben megadott tokenek, `@theme`) |
| Útvonalak | React Router |
| Szerveradat | TanStack Query |
| Felület-állapot | Zustand (kis store-ok, bennük tiszta reducerrel) |
| Akadálymentes alapelemek | Radix UI |
| Ikonok | Lucide (a logó saját SVG) |
| Fordítás | i18next + react-i18next |
| Tesztek | Vitest (jsdom) + Testing Library |
| Lint | ESLint (flat config), typescript-eslint, react-hooks |
| Rétegszabályok ellenőrzése | eslint-plugin-boundaries |

A TypeScript 6.0-n marad, amíg a typescript-eslint a 7-est nem támogatja (2026-10-30: `<6.1.0`). Új csomag felvétele előtt mindig ellenőrizni kell a peer-függőségeket és a licencet. **AGPL-es vagy más copyleft licenc tilos.** Minden új futásidejű függőséget licenccel együtt fel kell sorolni a kör jelentésében.

---

## 2. Rétegek és függőségi szabályok

Négy réteg van, és minden réteg csak a nála lejjebb lévőt használhatja:

```
app/        az egész összerakása: indítás, providerek, útvonalak
  ↓
layout/     az alkalmazás kerete: fejléc, oldalsáv-keret, panelkezelő, lebegőablak-kezelő
  ↓
features/   a funkciók, a backend moduljainak tükre
  ↓
shared/     közös alap, semmilyen funkcióról nem tud
```

| Honnan | Mit importálhat | Mit **nem** |
| --- | --- | --- |
| `shared/` | csak `shared/`-et | `features/`, `layout/`, `app/` |
| `features/<x>/` | `shared/`-et; **más funkciót csak a gyökerén át** (`@/features/y`, ami a `features/y/index.ts`) | más funkció belsejét (`@/features/y/components/…`), `layout/`, `app/` |
| `layout/` | `shared/`-et; funkciót csak a gyökerén át | funkció belsejét, `app/` |
| `app/` | bármit | – |

A backend megfelelői: a „modul más modulnak csak az `api.go`-ját hívhatja” szabályból itt „csak az `index.ts`-ét”. A „modulok közti reakció eseménnyel történik” szabálynak az felel meg, hogy a funkciók a WebSocket-eseményekre maguk reagálnak (5. fejezet), nem egymás belső függvényeit hívják.

**Két funkció közti függés ritka legyen.** Ha két funkciónak ugyanaz kell, és az nem funkció-specifikus, akkor a `shared/`-be kerül. Ha egy képernyő több funkcióból áll össze (például a csatorna mellett a szál és a terminál), azt a `layout/` vagy az `app/` rakja össze, nem az egyik funkció a másikból.

Körkörös függés tilos: ha `a` importálja `b`-t, `b` nem importálhatja `a`-t.

---

## 3. Mappaszerkezet

```
web/
├── index.html                 # a témát az első kirajzolás előtt beállító inline szkript
├── package.json, package-lock.json, tsconfig.json, vite.config.ts, eslint.config.js
├── embed.go                   # Go-csomag: a dist/ beágyazása a szerverbe
├── dist/                      # build kimenet (gitignore-olva, kivéve .keep)
├── public/brand/              # logó, favicon, PWA-ikonok
├── scripts/                   # build segédszkriptek (go.mod a node_modules-ba, dist/.keep)
└── src/
    ├── app/
    │   ├── main.tsx           # belépési pont
    │   ├── App.tsx            # providerek: router, query client, téma
    │   ├── routes.tsx         # útvonal → a funkció oldala (lusta betöltéssel)
    │   ├── i18n.ts            # összegyűjti a shared és a funkciók szövegeit
    │   └── test-setup.ts
    │
    ├── layout/
    │   ├── Shell.tsx          # fejléc + oldalsáv + fő oszlop + panelhelyek
    │   ├── header/            # Header, Breadcrumb, CommandPalette, ActivityPopover…
    │   ├── sidebar/           # az oldalsáv kerete és csoportjai
    │   ├── panes/             # panes.ts (tiszta reducer + URL-leképezés), PaneHost.tsx
    │   └── windows/           # windows.ts (tiszta reducer), FloatingWindow.tsx
    │
    ├── features/
    │   ├── conversation/      # csatornák, üzenetek, szálak, szerkesztő, csatolás, némítás, Szálaim
    │   ├── agents/            # áttekintő panel, agentlista, létrehozó és beállító ablak
    │   ├── hosts/             # gépek, mappaválasztó
    │   ├── planner/           # feladatok, naptár
    │   ├── notes/
    │   ├── schedules/
    │   ├── wiki/
    │   ├── leases/
    │   ├── rules/             # közös agent-szabályok
    │   ├── questions/         # Válaszra vár
    │   ├── approvals/         # jóváhagyások, delegációk, engedélyek
    │   ├── compaction/
    │   ├── files/             # fájlböngésző, fájlnéző
    │   ├── terminals/
    │   └── admin/             # futások, beállítások, felhasználók, tokenek, audit, rendszerállapot
    │
    └── shared/
        ├── api/
        │   ├── schema.d.ts    # GENERÁLT az api/openapi.yaml-ból; kézzel nem szerkeszthető
        │   ├── client.ts      # típusos fetch (hibakezelés, 401 → belépés)
        │   ├── ws.ts          # az egyetlen WebSocket-kliens (hello, replay, újracsatlakozás)
        │   ├── events.ts      # eseményelosztó: a funkciók itt iratkoznak fel
        │   └── queryClient.ts
        ├── ui/                # a design system komponensei (Button, Dialog, Sheet, Avatar…)
        ├── lib/               # tiszta függvények: format, markdown, mention, fuzzy, shortcuts, cn
        ├── hooks/             # általános hookok (useMediaQuery, useLocalStorage…)
        ├── i18n/              # i18next beállítás, nyelvváltás, közös szövegek (locales/)
        ├── theme/             # téma (rendszer / világos / sötét)
        └── styles/            # tokens.css, index.css
```

A váz most egyetlen helyőrző funkciót tartalmaz (`features/scaffold/`), ez mintának is szolgál; a 31. kör lecseréli a valódi kerettel.

A funkciók listája a **backend moduljait követi** (döntés: Norbi, 2026-10-30) (`internal/modules/…`), így egy API-változásnál egyértelmű, hova kell nyúlni. Új funkció csak akkor kerül be, ha a backendben is külön modul, vagy ha a felületen egyértelműen önálló terület (például `files`, `terminals`). Kétség esetén meglévő funkció bővül, és ezt a jelentés megemlíti.

---

## 4. Egy funkció belseje

Minden funkció ugyanúgy épül fel, így ha egyet ismersz, a többiben is eligazodsz:

```
features/wiki/
├── index.ts          # NYILVÁNOS FELÜLET: csak ezt importálhatja más
├── api.ts            # TanStack Query hookok: useWikiIndex(), useAppendPost()…
├── events.ts         # WebSocket-események (wiki.*) → a query cache frissítése
├── model.ts          # típusok és TISZTA logika, pl. wikiPosts(revisions)
├── model.test.ts     # a tiszta logika tesztjei, közvetlenül mellette
├── store.ts          # csak ha kell: a funkció saját felület-állapota (Zustand)
├── components/       # a funkció saját komponensei (TopicRow, PostCard…)
├── pages/            # útvonal-komponensek (WikiIndexPage, WikiTopicPage)
└── locales/
    ├── hu.json       # a funkció saját szövegei (névtér: "wiki")
    └── en.json
```

| Fájl | Mi lehet benne | Mi nem |
| --- | --- | --- |
| `index.ts` | újraexportok: oldalak, panelek, hookok, típusok, amit más használ | logika, JSX |
| `api.ts` | lekérdezések és módosítások a `shared/api/client.ts`-en át, query-kulcsok | megjelenítés |
| `events.ts` | feliratkozás a `shared/api/events.ts`-re, cache-frissítés | közvetlen DOM-művelet |
| `model.ts` | típusok (a `schema.d.ts`-ből levezetve), tiszta függvények | React, fetch, `Date.now()` paraméter nélkül |
| `components/` | megjelenítés, kevés helyi állapot | üzleti szabály, ami teszt nélkül maradna |
| `pages/` | egy útvonal összerakása komponensekből | máshonnan importált belső részek |

Kis funkciónál a fájlok elhagyhatók (például nincs `store.ts`), de a meglévők neve és szerepe mindig ez.

**A backend megfelelői:**

| Backend | Frontend |
| --- | --- |
| `model.go` (tiszta domain) | `model.ts` |
| `service.go` + `store.go` (port) | `api.ts` |
| modulok közti eseményreakció | `events.ts` |
| `httpapi/` (transzport) | `components/`, `pages/` |
| `api.go` (nyilvános felület) | `index.ts` |

---

## 5. Adat és állapot

### Szerverről jövő adat: TanStack Query

- Minden olvasás egy `api.ts`-beli hookon át megy. **A komponens nem hív `fetch`-et.**
- A query-kulcsok a funkció `api.ts`-ében vannak, egy helyen: `wikiKeys.index(q)`, `wikiKeys.topic(slug)`.
- Módosítás után a funkció érvényteleníti vagy frissíti a saját kulcsait. Optimista frissítés csak ott, ahol a prompt kéri, visszagörgetéssel és toasttal.

### Élő frissítés: WebSocket

- **Egyetlen kliens van:** `shared/api/ws.ts`. Ez kezeli a `hello`-t a `last_seq`-kel, a replay-t, a `resync_required`-et és az újracsatlakozást.
- Az események a `shared/api/events.ts`-be futnak be. **A funkciók a saját `events.ts`-ükben iratkoznak fel** a saját eseménytípusaikra, és a saját cache-üket frissítik. Az `app/` indításkor regisztrálja őket.
- `resync_required` után minden funkció a saját adatát kéri le újra.
- Az esemény tartalma típusos (`Event<Name>` sémák az OpenAPI-ban, 31. kör, 0. rész).

### Felület-állapot: Zustand, bennük tiszta reducerrel

- A **szabályok tiszta függvények**, például `layout/panes/panes.ts` (állapot + művelet → új állapot, állapot ↔ URL) és `layout/windows/windows.ts`. Ezeket Vitest teszteli, React nélkül.
- A Zustand store csak becsomagolja őket: tárol, értesít, perzisztál.
- **Ami megosztható, az az URL-ben van:** a megnyitott csatorna, a szál, a munkapanelek, egy beállítóablak füle. A link, az újratöltés és a vissza gomb így ugyanoda visz.
- `localStorage` csak nézőnkénti kényelemre való (összecsukott csoport, téma, lebegő ablak helye). Mindig `try/catch`-ben, és nélküle is működnie kell.

### API-típusok

- `shared/api/schema.d.ts` **generált fájl**, a `make gen` írja, a `make gen-check` ellenőrzi. Kézzel nem szerkeszthető, és kérés- vagy választípus kézzel nem írható.
- A szerződés változása itt is az `api/openapi.yaml`-ban kezdődik (CLAUDE.md 3. szabály).

---

## 6. Komponensek és stílus

- **A design system komponensei a `shared/ui/`-ban vannak**, és csak a design tokenekkel dolgoznak. Akadálymentességet igénylő elemeknél (Dialog, Menu, Popover, Tooltip, Tabs) Radix az alap, Tailwinddel stílusozva. Kész kinézetű komponenskészlet nem kerülhet be.
- Egy funkció nem gyárt magának gombot vagy dialógust: a `shared/ui`-t használja, és ha onnan hiányzik valami, ott bővíti.
- **Csak szemantikus tokenek:** `bg-surface`, `text-fg-muted`, `border-border`, `bg-accent`. Nyers szín (`#e5383b`, `rgb(…)`) vagy a Tailwind alappalettája (`bg-red-500`, `text-zinc-400`) tilos a `src/`-ben. Kivétel a `shared/styles/tokens.css`, ahol a tokenek értékei vannak.
- A sötét mód a `.dark` osztályt követi a `<html>`-en (`dark:` variáns). A tokenek maguk váltanak, ezért `dark:` ritkán kell.
- Feltételes osztályok a `shared/lib/cn.ts` segédfüggvénnyel. Változatoknál egy egyszerű objektum-térkép kell, nem hosszú feltételsor.
- Inline `style` csak dinamikus értékre (pozíció, méret, százalék), színre soha.
- **Akadálymentesség:**
  - ikon-gombnak kötelező az `aria-label`;
  - a fókusz látható;
  - a billentyűzetes használat működik;
  - a szín soha nem az egyetlen jelzés;
  - a `prefers-reduced-motion` be van tartva.
- **Billentyűparancsok:** csak a közös regiszteren át (`shared/lib/shortcuts.ts`, 31. kör), külön `keydown` figyelő nem írható.

---

## 7. Szövegek (i18n)

- **Szöveg nem lehet beégetve** a komponensben (CLAUDE.md, Nyelv). A felhasználó minden szövege i18n kulcs.
- **Névterek:**
  - közös szövegek (gombok, általános hibák): `shared/i18n/locales/{hu,en}.json`, névtér `common`;
  - funkció szövegei: `features/<x>/locales/{hu,en}.json`, a névtér a funkció neve (`wiki`, `agents`…).
  - Az `app/i18n.ts` gyűjti össze őket (`import.meta.glob`), a funkciónak csak a saját JSON-ját kell létrehoznia.
- Használat: `useTranslation("wiki")` és `t("topic.reply")`, közös szövegnél `t("common:save")`.
- Alapnyelv a magyar. **A magyar és az angol kulcsok egyezését teszt ellenőrzi**, minden névtérben.
- Dátum, idő és szám a felület nyelvét követi (`hu-HU` / `en-GB`), a `shared/lib/format.ts`-en át.

---

## 8. Útvonalak

- Az útvonalakat az `app/routes.tsx` sorolja fel. A funkció oldalait a funkció `index.ts`-éből veszi, **lustán betöltve** (`React.lazy`), így minden funkció külön kódcsomag.
- Az alapútvonal `/app` (`BrowserRouter basename="/app"`). A szerver minden `/app/…` útvonalra az `index.html`-t adja (`internal/httpserver/app.go`).
- A paneleket és ablakokat leíró URL-paraméterek formáját a `layout/panes/panes.ts` határozza meg, a funkciók nem találnak ki sajátot.

---

## 9. Tesztek

| Mit | Hol | Mivel |
| --- | --- | --- |
| tiszta logika (`model.ts`, `shared/lib`, `layout/*/…ts` reducerek) | a fájl mellett, `*.test.ts` | Vitest, React nélkül |
| komponens viselkedése (megjelenik, kattintásra mit csinál, akadálymentes név) | a komponens mellett, `*.test.tsx` | Vitest + Testing Library |
| útvonal-összeállítás | `app/` | Testing Library + `MemoryRouter` |
| szabálytesztek (nyelvi kulcsok egyezése, nyers színek tilalma, tokenek kontrasztja) | `app/` vagy `shared/` | Vitest |

- Minden üzleti szabálynak tiszta függvény a helye, teszttel. Ha egy szabály csak komponensben él, azt át kell tenni.
- Hibajavításhoz tartozzon egy teszt, ami a javítás nélkül elbukott volna (CLAUDE.md).
- A tesztek a választ a generált típusokkal építik fel, nem kézzel írt alakkal.
- Minden képernyőt böngészőben is meg kell nézni, három szélességben (1440, 1024, 390), mindkét témában, konzolhiba nélkül (a körök prompjai szerint).

---

## 10. Biztonság

- **`dangerouslySetInnerHTML` tilos**, ezt lintszabály ellenőrzi. Markdown csak a közös, biztonságos megjelenítőn át jelenhet meg (`shared/lib/markdown`), amely React-elemeket épít, és nem enged távoli képet.
- **Az agentektől jövő szöveg adat, nem utasítás** (CLAUDE.md 10. szabály): nem futtatjuk, nem értelmezzük HTML-ként, nem követünk benne linket automatikusan.
- Titok (token, jelszó, ICS-link) nem kerülhet `localStorage`-ba, URL-be vagy naplóba. Az egyszer megmutatott titok csak a memóriában él.
- A jogosultság mindig a szerveren dől el. A felület csak elrejti, amit a szerver úgyis megtagadna, de nem helyettesíti a szerver ellenőrzését.

---

## 11. Elnevezések és importok

| Mi | Forma | Példa |
| --- | --- | --- |
| komponens | `PascalCase.tsx` | `MessageList.tsx` |
| hook | `useValami.ts` | `useChannel.ts` |
| egyéb modul | `camelCase.ts` | `panes.ts`, `model.ts` |
| teszt | a fájl mellett, `*.test.ts(x)` | `panes.test.ts` |
| mappa | `kebab-case` | `features/conversation/components/thread-pane/` |
| i18n kulcs | `pont.szerinti.camelCase` | `composer.attachFile` |

- **Importnál az `@/` alias** az `src/`-re mutat: `import { Button } from "@/shared/ui"`. Mappán belül relatív import (`./model`) is használható; több szinttel feljebb mutató relatív import (`../../`) nem.
- Egy mappának akkor van `index.ts`-e, ha mások a mappán kívülről használják (minden funkció, `shared/ui`, `shared/lib`).
- Alapértelmezett export csak az útvonal-oldalaknál van (a lusta betöltés miatt), máshol nevesített export.

---

## 12. Gépi ellenőrzés

A `make test-web` blokkol, ahogy a backendnél a `make lint`. Részei:

| Ellenőrzés | Hol |
| --- | --- |
| rétegszabályok (2. fejezet) | `web/eslint.config.js`, `eslint-plugin-boundaries`: `boundaries/dependencies` |
| funkció belsejébe importálás tilalma (csak `index.ts`) | ugyanott (`fileInternalPath: "index.ts"`) |
| minden forrásfájl egy rétegbe tartozik | ugyanott, `boundaries/no-unknown-files` |
| `../../` importok tilalma | ugyanott, `no-restricted-imports` |
| hogy a fenti szabályok tényleg elkapják a hibát | `src/app/architecture.test.ts` (szándékos szabálysértésekkel lintel) |
| `dangerouslySetInnerHTML` és `innerHTML` tilalma | ugyanott, `no-restricted-syntax` |
| hookszabályok | `eslint-plugin-react-hooks` |
| típusok | `tsc --noEmit` (`strict`, `noUncheckedIndexedAccess`) |
| nyelvi kulcsok egyezése minden névtérben | `src/app/i18n.test.ts` |
| nyers színek tilalma | `src/shared/styles/no-raw-colors.test.ts` |
| tokenek kontrasztja (WCAG AA) | a 31. kör adja hozzá |
| generált API-típusok frissessége | `make gen-check` (a 31. kör adja hozzá) |

Szabályt megkerülni (`eslint-disable`, `@ts-ignore`, `as any`) csak indoklással szabad, egy soros kommentben. Minden ilyet a jelentésben is fel kell sorolni.

---

## 13. Új funkció felvétele: ellenőrzőlista

1. Létezik-e már a backendben a modul? A funkció neve egyezzen vele.
2. A mappa a 4. fejezet szerint: `index.ts`, `api.ts`, `model.ts`, `components/`, `pages/`, `locales/{hu,en}.json`; szükség szerint `events.ts`, `store.ts`.
3. A szerződés az `api/openapi.yaml`-ban, majd `make gen`. A típusok a `schema.d.ts`-ből jönnek.
4. Az oldal bekerül az `app/routes.tsx`-be, lusta betöltéssel. Az oldalsáv-bejegyzés a `layout/sidebar`-ba kerül.
5. Az eseményekre az `events.ts` iratkozik fel, az `app/` regisztrálja.
6. A tiszta logikához teszt, a fontos komponensekhez viselkedésteszt.
7. `make test-web` zöld, a képernyők böngészőben megnézve.

## 14. Amit soha

- komponensben `fetch`, vagy kézzel írt API-típus;
- import egy másik funkció belsejéből;
- `shared/` → `features/` import;
- beégetett felhasználói szöveg;
- nyers szín vagy Tailwind alappaletta;
- `dangerouslySetInnerHTML`, `innerHTML`;
- saját `keydown` figyelő a parancsregiszter helyett;
- titok `localStorage`-ban vagy URL-ben;
- generált fájl kézi szerkesztése;
- copyleft licencű függőség.
