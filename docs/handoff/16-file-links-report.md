# Handoff Report 16: Helyi fájllinkek megnyitása a felületen (File Links)

- **Dátum:** 2026-10-22
- **Ág:** `feature/file-links`
- **Worktree:** `/mnt/g/Projects/new-chat-worktrees/file-links`
- **Kapcsolódó feladat:** `docs/handoff/16-file-links-prompt.md`

---

## 1. Összefoglalás

Az agentek a válaszaikban gyakran hivatkoznak a gépükön lévő helyi fájlokra (pl. `file:///home/szmajli01/homelab/trader/logs/scheduler.log`, `/home/…/report.md`, `~/notes.txt`). Korábban ezekre kattintva a böngésző nem tudta megnyitni a homelabon vagy távoli gépen lévő fájlokat.

A 16-os feladatban megvalósítottuk a helyi fájllinkek biztonságos, teljes életciklusú kezelését:
1. **Őroldali levágási hiba javítása (`FileLinks`)**: a válaszok levágása előtt kigyűjtjük a linkeket, és a vágás miatt elveszett hivatkozásokat az őr automatikusan egy neutralis `📎` követő üzenetben küldi el `<id>-files` idempotens kliensazonosítóval.
2. **Szerveroldali fájlletöltési architektúra (`file-fetch`)**: kizárólag emberek által indítható folyamat, szigorú csatorna- és üzenettartalom-ellenőrzéssel, 2 perces seprési időtúllépéssel (`host_offline`), streaming feltöltéssel és `human_only = true` izolációval a `file` táblában.
3. **Őroldali kiszolgálás szigorú hatókörrel és tiltólistával**: az őr a `/ws/ui` WebSocketen figyeli a kérelmeket, legfeljebb 2 párhuzamos számon dolgozza fel őket a futások blokkolása nélkül; ellenőrzi a sandbox mappaengedélyeket vagy a gép gyökereit; feloldja a szimbolikus linkeket; tiltja a szenzitív könyvtárakat (`.ssh`, `.aws`, `~/.config/colonyhall`, tokenek, titokcsere-mappa) és titoknév-mintákat (`.env`, `*.pem`, `*.key`, `id_rsa*` stb.); és betartja a kép/fájl méretkorlátokat.
4. **Felületi integráció (Dev UI)**: az agent üzenetekben a fájlhivatkozások interaktív `📄 <fájlnév>` chipként jelennek meg (`textContent`, DOM-építés, `innerHTML` nélkül); kattintásra asztali gépen elegáns modál, mobilon (különösen 390px iPhone nézetben) teljes képernyős megtekintő nyílik meg; szövegeknél/kódoknál/logoknál monospace nézet 2 MB felett 1 MB fej-láb HTTP Range kapcsolóval (a logok automatikusan a legfrissebb, befejező résznél nyílnak meg); biztonságos, tisztán DOM-alapú Markdown megjelenítő (`http`/`https` linkekkel, `noopener noreferrer`-rel és nyers váltóval); képmegjelenítés; letöltési gomb; valamint adminisztrátori korlátok a Gépek panelen.

---

## 2. Commitok listája

A fejlesztés során 5 önálló, logikailag elhatárolt commit készült a `feature/file-links` ágon:

1. **`9feda29` Guard: never lose a file link when clipping**
   - Létrehozta a tiszta `FileLinks(text string) []string` parsert (`internal/guard/filelinks.go`) és annak alapos egységtesztjeit (`internal/guard/filelinks_test.go`).
   - Módosította a `reply` és `replyLong` metódusokat (`internal/guard/guard.go`), hogy a válasz vágása előtt kigyűjtse a linkeket, és a csonkoláskor elveszett linkeket egy külön követő szálüzenetben posztolja `📎` fejléccel és `<id>-files` kliensazonosítóval.

2. **`b763f40` Server: file fetch requests, streaming upload, human-only files, and limits**
   - OpenAPI specifikáció bővítése (`api/openapi.yaml`, `internal/apispec/apispec.gen.go`).
   - Adatbázis-migráció (`migrations/00025_file_fetch.sql`): `file_fetch_request` és `workspace_file_fetch_settings` táblák, valamint `human_only` oszlop a `file` táblán.
   - Szerveroldali modul megvalósítás (`internal/modules/files/`): modell, memóriabeli és Postgres adattárak, service réteg időtúllépés-sepréssel, HTTP handlerek és streaming feltöltés `io.LimitReader`-rel.
   - HTTP Range támogatás finomhangolása a memóriabeli és fájlrendszer-tárban (`nopCloserReadSeeker`).
   - Human-only védelem az összes fájllekérdező és csatolmánylistázó végponton (agent tokenek esetén 403 / kiszűrés).
   - Szerződéstesztek (`internal/storetest/storetest.go`) és e2e tesztek (`internal/e2e/filefetch_test.go`).

3. **`1df7519` Guard: serve file fetch requests with scope, deny rules, and limits**
   - Őr REST kliens kiegészítése (`internal/guard/client.go`): `PendingFileFetches`, `UploadFetchedContent`, `FailFileFetch`.
   - Kérelmek kiszolgálása (`internal/guard/filefetch.go`): szigorú hatókör- és jogosultság-ellenőrzés (`CheckFileFetch`), szimbolikus link feloldás (`filepath.EvalSymlinks`), MIME-felismerés és méretkorlát-vizsgálat.
   - WebSocket eseményfigyelés (`internal/guard/ws.go`): `file.fetch_requested` események azonnali feldolgozása.
   - Párhuzamossági korlátozás: megosztott szemafor csatorna (`limiter := make(chan struct{}, 2)`) az őr és agentjei között; háttérben futó goroutine-ok az agent munkafutásainak blokkolása nélkül.
   - Alapos őroldali egység- és integrációs tesztek (`internal/guard/filefetch_test.go`).

4. **`31a7db2` UI: file link chips, viewer modal with head-tail range and safe markdown, limits box**
   - Kliensoldali linkfelismerés (`parseFileLinkSegments`) azonos logikával a `FileLinks` parserrel, biztonságos DOM chip-építés az agent üzenetekben (`internal/httpserver/web/index.html`).
   - Megtekintő modál: asztali nézetben lebegő dialógus, mobilon (<=640px) 100vw/100vh teljes képernyős nézet.
   - Monospace szöveg/kód/log nézet: 2 MB felett 1 MB fej-láb Range kapcsoló ("megjelenítés: eleje / vége"), a logok automatikusan a végétől töltődnek be.
   - Biztonságos DOM-alapú Markdown renderelő (`buildSafeMarkdownDOM`, `renderInlineMarkdown`): semmi `innerHTML`, szigorúan csak `http`/`https` linkek, és kapcsolható "nyers" nézet.
   - Képmegjelenítés és bináris fájl letöltés.
   - WebSocket integráció (`file.fetch_completed`, `file.fetch_failed`) és 2 másodperces tartalék lekérdezés.
   - Kétnyelvűsített hibaüzenetek és kezelőszervek (hu/en).
   - Adminisztrátori "Fájlmegnyitás" korlát-beállító kártya a Gépek panelen.

5. **`36dfff1` Guard: thread-safe agent identity and concurrency race fixes**
   - Az e2e tesztek race detector vizsgálata feltárta, hogy az `Agent.Tick()` által frissített `a.me` mezőt a háttérben futó WebSocket figyelő és a fájlkiszolgáló goroutine párhuzamosan olvashatta.
   - Bevezettük az `a.getMe()` és `a.setMe(m)` szálbiztos metódusokat (`sync.RWMutex`), valamint az egységes `a.logName()` hívást, megszüntetve a versenyhelyzeteket a guard és az e2e csomagokban.

---

## 3. Tervezési döntések

A prompt által nyitva hagyott kérdésekben az alábbi mérnöki döntéseket hoztuk:

### 3.1. Modul elhelyezése: miért az `internal/modules/files`?
A prompt felvetette, hogy a funkció az `internal/modules/files` modulba vagy egy új `filefetch` modulba kerüljön.
- **Döntés:** Az `internal/modules/files` modul mellett döntöttünk.
- **Indoklás:**
  1. A fájlletöltési kérelem (`file_fetch_request`) sikeres teljesüléskor közvetlenül létrehoz egy `file` bejegyzést a fájltárban.
  2. A feladat megkövetelte az új `human_only` oszlop hozzáadását a meglévő `file` táblához, és ennek szigorú érvényesítését az összes létező fájllekérdező végponton (`/files/{id}`, `/files/{id}/content`, csatornacsatolmányok).
  3. Ha külön `filefetch` modult hoztunk volna létre, annak hozzá kellett volna férnie a `files` modul belső tárolójához vagy az exportált API-ján keresztül kellett volna fájlokat létrehoznia úgy, hogy közben a jogosultsági és tranzakciós határok bonyolódtak volna, és indokolatlanul növelte volna a modulok közötti csatolást.
  4. A `files` modul rétegződése (`model.go`, `service.go`, `store.go`, `postgres/`, `memory/`, `httpapi/`) így tiszta és egységes maradt, a `golangci-lint` `depguard` szabályai hiba nélkül érvényesülnek.

### 3.2. MIME-detektálás megközelítése az őrben
A prompt kérte a kép MIME felismerését a különböző méretkorlátok (`max_image_mb` vs `max_file_mb`) alkalmazásához.
- **Döntés:** Hibrid megközelítést alkalmaztunk: kiterjesztés-alapú előszűrés (`mime.TypeByExtension`) + az első 512 bájt beolvasása és vizsgálata a standard `http.DetectContentType` segítségével.
- **Indoklás:**
  - A tisztán `http.DetectContentType` alapú vizsgálat a szöveges kódokat és markdownokat gyakran egyszerűen `text/plain`-ként azonosítja, és nem ismeri fel a ritkább kiterjesztéseket (pl. `.log`, `.yaml`, `.md`).
  - Másrészt a kiterjesztés önmagában becsapható lehet, vagy hiányozhat.
  - A hibrid megoldás először a kiterjesztést vizsgálja; ha az nem határozza meg egyértelműen, vagy ha a kiterjesztés képformátumot jelez, az első 512 bájt sniffingjével igazolja a valós típusát (`image/jpeg`, `image/png`, `image/gif`, `image/webp`, `image/svg+xml`).
  - Ennek köszönhetően a méretkorlát ellenőrzése pontos: képekre a bőkezűbb `max_image_mb` vonatkozik, minden másra a `max_file_mb`.

---

## 4. Mi maradt ki (tudatosan)

1. **Nem módosult a közös agent-utasítás (`shared rules`)**: a feladatleírás kifejezetten rögzítette, hogy az agentek már természetesen linkelnek fájlokat, így nincs szükség felesleges utasításokkal növelni a promptjaikat.
2. **Nincs privát „system” identitása az őrnek**: a levágáskor elveszett linkeket az őr ugyanazzal az agent tokennel posztolja egy követő szálüzenetben, neutralis `📎` formátumban. Egy új, kiemelt rendszeridentitás bevezetése szükségtelenül bővítette volna a jogosultságokat.
3. **Agentek nem kezdeményezhetnek fájlletöltést**: agent tokenről indított kérelem 403 Forbidden hibát kap. Ez megakadályozza, hogy egy sandboxos agent a chatbe illesztett linkekkel egy másik gép teljes hozzáférésű agentjének fájljait kérhesse le.
4. **Agentek nem láthatják a letöltött fájlokat**: a letöltött fájlok `human_only = true` jelölést kapnak; nem kerülnek be az agent inboxába és nem érhetők el agent tokenekkel.

---

## 5. A megtekintő (viewer dialog) kialakítása

### 5.1. Asztali nézet (Desktop)
- Elegáns lebegő modal ablak félátlátszó háttérrel (`file-viewer-modal`).
- Maximális szélessége 960px, magassága a képernyő 85%-a, flexbox elrendezéssel.
- Fejléc:
  - Teljes fájlútvonal vastagon szedve (`word-break: break-all`).
  - Hivatkozó agent neve (pl. `@norbi-bot`) és a letöltés pontos időbélyege (`állapot: 10:42`).
  - Kék „Frissítés” gomb, amely kérésre újra lekéri a fájlt a gépről (`refresh: true`).
  - Jobb felső sarokban jól látható bezáró „✕” gomb.
- Tartalom megjelenítése típus szerint:
  - **Monospace szöveg / kód / log**: fix szélességű betűtípus, szintaxis-barát sortöréssel. 2 MB feletti fájloknál egy felső kapcsolósáv jelenik meg: „Megjelenítés: [Eleje (1 MB)] [Vége (1 MB)]”. A rendszer HTTP `Range` kéréssel (`Range: bytes=0-1048575` vagy `Range: bytes=-1048576`) tölti le a kívánt szakaszt. A `.log` kiterjesztésű fájlok alapértelmezetten a végüktől töltődnek be.
  - **Biztonságos Markdown (`.md`)**: a `buildSafeMarkdownDOM` tisztán DOM-csomópontokból (`createElement`, `createTextNode`) építi fel a formázást (h1-h3 címsorok, félkövér, dőlt, kód, kódtömbök, listák). A linkek kizárólag `http://` és `https://` protokollt fogadnak el, és automatikusan megkapják a `target="_blank"` és `rel="noopener noreferrer"` attribútumokat. Egy kapcsolóval átváltható „Nyers” szöveges nézetre.
  - **Képek (`image/*`)**: reszponzív beágyazott `<img>` elem, amely automatikusan illeszkedik a nézethez.
  - **Egyéb bináris fájlok**: információs kártya fájlnévvel, MIME típussal, formázott mérettel és egy „Fájl letöltése” gombbal.
- Hibaállapotok: a gép állapota és a hibakódok (pl. `host_offline`, `not_found`, `denied`, `too_large`, `not_a_file`, `timeout`) közérthető magyar és angol üzenetekkel jelennek meg.

### 5.2. Mobil nézet (különösen 390px szélesség)
- Médialekérdezéssel (`@media (max-width: 640px)`) a modál átvált teljes képernyős nézetre:
  - `width: 100vw; height: 100vh; max-width: 100vw; max-height: 100vh; border-radius: 0;`
  - A keretek és margók eltűnnek, így a 390px széles kijelző teljes felülete a tartalomé.
  - A fejléc rugalmasan többsorosra rendeződik: a bezáró gomb és a frissítés gomb kényelmesen érinthető marad.
  - A szövegtartalom és kódnézet automatikusan tördelődik (`overflow-x: auto`), megakadályozva az oldal vízszintes elcsúszását.

---

## 6. Verifikációs és tesztkimenetek

### 6.1. `make gen-check`
```
make gen
make[1]: Entering directory '/mnt/g/Projects/new-chat-worktrees/file-links'
go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
docker run --rm -u $(id -u):$(id -g) -v "/mnt/g/Projects/new-chat-worktrees/file-links":/src -w /src sqlc/sqlc:1.31.1 generate
make[1]: Leaving directory '/mnt/g/Projects/new-chat-worktrees/file-links'
git diff --exit-code -- internal/apispec $(find internal -type d -name sqlcgen)
```
*Eredmény: 0 eltérés, a generált OpenAPI és SQLC fájlok teljesen naprakészek.*

### 6.2. `make lint`
```
golangci-lint run ./...
0 issues.
```
*Eredmény: 0 hiba, a depguard rétegződési és stílusszabályok hibátlanok.*

### 6.3. `make test`
```
go test ./...
ok  	colonyhall/cmd/colonyhall	(cached)
ok  	colonyhall/internal/e2e	10.280s
ok  	colonyhall/internal/guard	2.477s
ok  	colonyhall/internal/modules/agentrules	(cached)
ok  	colonyhall/internal/modules/conversation	(cached)
ok  	colonyhall/internal/modules/delivery/mcptools	(cached)
ok  	colonyhall/internal/modules/files	(cached)
ok  	colonyhall/internal/modules/files/mcptools	(cached)
ok  	colonyhall/internal/modules/files/storage	(cached)
ok  	colonyhall/internal/modules/git	(cached)
ok  	colonyhall/internal/modules/git/provider	(cached)
ok  	colonyhall/internal/modules/identity	(cached)
ok  	colonyhall/internal/policy	(cached)
ok  	colonyhall/internal/storetest	(cached)
```
*Eredmény: Minden egység-, szerződés- és végponttól végpontig tartó teszt sikeresen lefutott.*

### 6.4. `go test -race -count=1 ./internal/guard/... ./internal/e2e/...`
```
ok  	colonyhall/internal/guard	3.627s
ok  	colonyhall/internal/e2e	45.506s
```
*Eredmény: Teljesen versenyhelyzet-mentes működés az őr és az e2e tesztek alatt.*

### 6.5. `make test-pg` (valódi PostgreSQL, uncached `GOFLAGS="-count=1"`)
Elindítva `make dev` segítségével, majd a tesztek után leállítva `make down`-nal:
```
COLONY_TEST_DATABASE_URL='postgres://colony:colony@localhost:5433/colony_test?sslmode=disable' go test ./internal/storetest/ -run TestPostgres -v
=== RUN   TestPostgres
=== RUN   TestPostgres/eventlog
=== RUN   TestPostgres/identity
=== RUN   TestPostgres/conversation
=== RUN   TestPostgres/delivery
=== RUN   TestPostgres/audit
=== RUN   TestPostgres/hosts
=== RUN   TestPostgres/runs
=== RUN   TestPostgres/schedule
=== RUN   TestPostgres/approval
=== RUN   TestPostgres/wiki
=== RUN   TestPostgres/question
=== RUN   TestPostgres/files
=== RUN   TestPostgres/git
=== RUN   TestPostgres/preview
=== RUN   TestPostgres/agentrules
=== RUN   TestPostgres/notes
=== RUN   TestPostgres/decisions
=== RUN   TestPostgres/leases
=== RUN   TestPostgres/sidebar
--- PASS: TestPostgres (2.61s)
    --- PASS: TestPostgres/eventlog (0.14s)
    --- PASS: TestPostgres/identity (0.15s)
    --- PASS: TestPostgres/conversation (0.15s)
    --- PASS: TestPostgres/delivery (0.11s)
    --- PASS: TestPostgres/audit (0.12s)
    --- PASS: TestPostgres/hosts (0.16s)
    --- PASS: TestPostgres/runs (0.12s)
    --- PASS: TestPostgres/schedule (0.12s)
    --- PASS: TestPostgres/approval (0.12s)
    --- PASS: TestPostgres/wiki (0.13s)
    --- PASS: TestPostgres/question (0.14s)
    --- PASS: TestPostgres/files (0.14s)
    --- PASS: TestPostgres/git (0.13s)
    --- PASS: TestPostgres/preview (0.13s)
    --- PASS: TestPostgres/agentrules (0.11s)
    --- PASS: TestPostgres/notes (0.12s)
    --- PASS: TestPostgres/decisions (0.11s)
    --- PASS: TestPostgres/leases (0.16s)
    --- PASS: TestPostgres/sidebar (0.15s)
PASS
ok  	colonyhall/internal/storetest	2.671s
```
*Eredmény: Minden Postgres adattár-szerződésteszt, beleértve a módosított `files` tárat, hiba nélkül lefutott.*

---

## 7. Bírálói útmutató (Mit érdemes először megnézni)

1. **`internal/guard/filelinks.go` & `filelinks_test.go`**:
   - A tiszta regex és URL-normalizáló logika (`FileLinks`), amely felismeri a `file://` és abszolút helyi útvonalakat.
   - A levágási tesztek a `guard_test.go`-ban: ellenőrzik, hogy ha a válasz vége csonkolódik, a hivatkozások a követő szálüzenetben megmaradnak.

2. **`internal/guard/filefetch.go` & `filefetch_test.go`**:
   - `CheckFileFetch`: a hatókör-ellenőrzési szabályok mindkét módra (sandbox mappaengedélyek vs teljes hozzáférésű gépgyökerek).
   - Tiltólisták: `.env`, kulcsok, tokenek, ssh mappák, csere-mappa (`colonyhall-exchange`).
   - Szimbolikus linkek feloldása (`filepath.EvalSymlinks`) és a cél újbóli ellenőrzése.
   - Párhuzamossági korlát (legfeljebb 2 szál) szemafor csatornával.

3. **`internal/modules/files/` & `internal/e2e/filefetch_test.go`**:
   - `service.go`: kérelem létrehozása és jogosultsági lánca (`CanRead`, üzenet szerzője agent, útvonal benne van az üzenetben, gép hozzárendelve).
   - `handlers.go`: `PUT /api/v1/agent/file-fetch/{id}/content` streaming feltöltés, limitellenőrzés memóriapufferelés nélkül.
   - `human_only` ellenőrzések: `TestFileFetchSecurityAndPermissions`, `TestFileFetchLifecycleAndHumanOnly`, `TestFileFetchSettings`.

4. **`internal/httpserver/web/index.html`**:
   - `parseFileLinkSegments` és `renderAgentBodyWithFileChips`: tisztán DOM-műveletekkel helyezi el a `📄 <fájlnév>` chipeket, megőrizve az eredeti szövegrészeket `textContent`-tel.
   - `buildSafeMarkdownDOM`: XSS-biztos Markdown DOM-építő, szigorú link-protokoll ellenőrzéssel.
   - HTTP Range logika (`bytes=0-1048575` és `bytes=-1048576`) 2 MB feletti fájloknál.
   - Adminisztrátori korlátok doboz a Gépek panelen.

---

## Átnézés (Claude Code, 2026-10-22)

Ellenőrizve: `make test`, `make lint`, `make gen-check`, `make test-pg` (gyorsítótár nélkül), `go test -race ./internal/guard/ ./internal/e2e/`: mind zöld. A néző böngészőben kipróbálva (memóriás szerver, az őr szerepét egy szkript játszotta): fájlgombok az üzenetben, markdown (a `javascript:` link és a `<script>` szövegként marad), tiltott fájl hibaüzenete; asztali és telefonos méretben.

Javítva:

- **Tranzakciós outbox:** a lekérés állapotváltozásai és eseményei külön, tranzakció nélkül íródtak, az esemény hibáját eldobta a kód. Most egy tranzakcióban mennek (`commit` segédfüggvény), az időtúllépés is (`expire`).
- **Állapotverseny:** a „kész” és a „hibás” frissítés feltétel nélkül írt, így az időtúllépés és egy késő feltöltés felülírhatta egymást. Most csak `pending` állapotból vált (`db.ErrConflict` különben); storetest rá.
- **Őr, növekvő log:** a feltöltés a `stat` méretét jelentette be, egy közben írt lognál a Go HTTP kliens hibával megszakítja a küldést. Most a megnyitáskori méretű pillanatképet küldi (`openSnapshot`).
- **Őr, ellenőrzés és megnyitás között:** megnyitás után újraellenőrzi, hogy ugyanazt a reguláris fájlt nyitotta-e meg (`/proc/self/fd`), így egy közben szimbolikus linkre cserélt fájl nem csúszik át.
- **Őr, nagy fájl:** a 30 mp-es kliens-időkorlát egy 100–200 MB-os feltöltést lassabb kapcsolaton elvágott volna; a feltöltésnek most 110 mp jut (a szerver 2 perc után adja fel a kérést).
- **Néző:** a hibaüzenetek `innerHTML`-lel kerültek ki (szerverszöveggel); most `textContent`.

Ismert korlát: egy 2 percnél tovább tartó feltöltést a szerver „gép nem elérhető”-ként zár le (helyi hálózaton 100–200 MB ennél jóval gyorsabb).
