# Kódbázis-elemzés: tervezési elvek, tesztek, terhelés, skálázás, biztonság

Állapot: 2026-11-27, a `main` (`9658968b`) alapján; készítette @chlab-claude Norbi kérésére (#colonyhall-dev). A javítások a `fix/load-and-hardening` ágon, ugyanebben a PR-ben vannak (7. pont).

## Összefoglaló

| Szempont | Értékelés | Egy mondatban |
|---|---|---|
| SOLID, rétegek | **jó (4/5)** | A modulhatárokat és a befelé mutató függőséget a depguard gépileg kényszeríti; a gyenge pont néhány 1000+ soros fájl és a nagy `guard` csomag. |
| „Egy téma = egy fájl” | **jó, kivételekkel** | A modulokon belül témánkénti fájlok vannak; 16 nem generált fájl 800 sor fölött, ezekben több téma keveredik. |
| Közös segédfüggvények | **közepes** | Van közös réteg (`httpx`, `platform/*`), de apró segédek (`pathID`, `nullID`, `clip`) 7–7 példányban le vannak másolva. |
| Tesztlefedettség | **jó (Go: 73,5% együtt)** | Egységtesztek önmagukban 27,8%, a végponttól végpontig tartó tesztek 58,4%, a kettő együtt 73,5% utasítás-lefedettség (generált kód nélkül); a PWA-ban 122 tesztfájl 338 forrásfájlra. |
| Terhelés (több száz egyidejű felhasználó) | **közepes** | Egy példányon néhány száz felhasználó elbírható, de a WebSocket-szétosztás kapcsolatonként olvas az adatbázisból, és a kapcsolatgyűjtő alapból 4 kapcsolatos: ez az első szűk keresztmetszet (javítva a PR-ben). |
| Több backend terheléselosztó mögött | **ma nem működik helyesen** | A busz, a háttérciklusok, az eseményfogyasztók és az őr-közvetítések egy példányra vannak tervezve; két példány duplán kézbesítene. A terv megvan (`docs/22`, 7. pont), kb. 2–4 hét munka. |
| Biztonság | **jó (ASVS L2: 31/37 ✅)** | Erős alapok (RLS, CSRF, argon2id, hash-elt tokenek, CSP, sandbox); nyitott: admin MFA, kiszivárgott jelszó ellenőrzés, HSTS (javítva a PR-ben). |

## 1. SOLID és rétegzés

**Erősségek**
- **SRP / modulok:** 30 funkciómodul (`internal/modules/*`), mindegyikben `model.go`/`events.go` ← `service.go` ← `store.go` (port) ← `postgres/`, `memory/`, `httpapi/`, `mcptools/`. A nagyobb modulok szolgáltatása témánként fájlokra bomlik (pl. `conversation`: `actions.go`, `dm.go`, `forward.go`, `pins.go`, `reactions.go`, `thread.go`).
- **DIP:** a szolgáltatások interfészeken át kapják a tárat, az órát, az azonosítót és a véletlent (`Clock`, `IDGenerator`), az összerakás egy helyen van (`internal/server/server.go`). A tárak mindkét megvalósítására (`memory`, `postgres`) ugyanaz a szerződés-tesztkészlet fut (`internal/storetest`).
- **ISP:** a modulok egymásnak csak szűk `api.go` felületet adnak (pl. `conversation.Reader`, 11 metódus), a modulok közti reakció eseménnyel megy (outbox + fogyasztók).
- **OCP:** új AI kliens, git-szolgáltató, tár = új adapter (pl. `git/provider`, `search/embedhttp`, `files/storage`).
- **Gépi kényszerítés:** a `.golangci.yml` depguard-szabályai (modulmag I/O nélkül, platform nem függ modultól, rendszermód csak háttérkódban) — ez ritka és értékes.

**Gyengeségek**
- **Nagy fájlok (több téma egy fájlban):** `internal/guard/guard.go` (1660 sor, 58 függvény), `conversation/service.go` (1449, 42 metódus), `guard/direct.go` (1130), `files/service.go` (1124), `guard/host.go` (1089), `workshop/service.go` (1050), `delegation/service.go` (1044). Ezek a következő szétbontás jelöltjei (pl. a `guard.go` futás-indítás / ébresztés / állapotjelentés részekre).
- **A `guard` csomag** (~15 000 sor egy csomagban) a legkevésbé rétegzett rész: kliens-adapterek (Claude Code, Antigravity, direkt API), terminál, előnézet, fájlközvetítés és ütemezés egy csomagban, és a `guard.go`/`direct.go` futtatási ciklusa részben párhuzamos kód (az időtúllépés-üzenet szó szerint kétszer szerepelt; javítva).
- A szolgáltatás-structok nyilvános mezőkön át kapják a függőségeiket (`&conversation.Service{Store: …}`); ez egyszerű, de egy kimaradt mező csak futáskor derül ki. Konstruktor + kötelező mezők ellenőrzése erősebb lenne.

## 2. Közös segédek, ismétlődő kód

- Van közös réteg: `internal/httpx` (JSON-dekódolás méretkorláttal, hibaválaszok, CSRF), `internal/platform/*` (`ids`, `clock`, `clientip`, `egress`, `secretbox`, `secretscan`, `tenant`), `internal/policy` (tiszta függvények, 100% lefedettség).
- **Másolt apró segédek** (ugyanaz a törzs több csomagban): `pathID` 7×, `nullID` 7×, `clip` 7× (ebből 4 bájtra azonos, 3 kicsit eltérő viselkedésű), `toModel` 6×, `optID`, `notFound` 4×. A PR a bájtra azonosakat közös helperbe vonja (`httpx.PathID`, `ids` és szöveg-segéd); a viselkedésben eltérőket szándékosan meghagyja.
- A webes felület (`web/`) funkciómodulokra bontott (`docs/08`), közös `shared/` réteggel.

## 3. Tesztlefedettség

Mérés: 2026-11-27, `go test -coverprofile` (egység, csomagonként) és `go test -coverpkg=<minden nem generált csomag> ./internal/e2e/` (integráció), a profilok összevonva; a `sqlcgen` és `apispec` generált kód kimarad.

| Réteg | Lefedettség | Megjegyzés |
|---|---|---|
| Egységtesztek (csomagon belül) | **27,8%** | 212 tesztfájl, ~790 `Test`/`Fuzz` függvény, ~49 500 sor tesztkód (93 000 sor éles Go-kód mellett) |
| Végponttól végpontig (`internal/e2e`, 98 fájl, ~26 000 sor) | **58,4%** | Valódi HTTP/WebSocket/MCP a teljes szerver ellen memóriabeli tárakkal, minden válasz az OpenAPI-hoz mérve |
| **Együtt** | **73,5%** | |
| Tár-szerződéstesztek Postgres ellen (`make test-pg`) | nem mérve itt | A `postgres/` adapterek lefedettségét ez adja (Docker kell); a fenti szám nélküle értendő |
| PWA (`web/`, Vitest) | nem mérve | 122 tesztfájl / 338 forrásfájl; a `@vitest/coverage-v8` nincs telepítve, ezért százalék nincs |

Modulonként (összevont, függvényátlag): `gateway` 88%, `guard` 83%, `conversation` 68%, `delivery` 68%, `files` 67%, `hosts` 66%, `identity` 62%, `delegation` 57%.

**Értékelés:** a lefedettség jó, de **egyoldalú**: a fő üzleti modulok (`conversation`, `identity`, `files`, `delivery`) szinte csak az e2e-teszteken át vannak tesztelve (saját egységteszt-lefedettségük 0–12%). Ez működik, de egy hiba helyét nehezebb megtalálni, és az e2e-készlet lassabb. Javaslat: a szolgáltatások szabályaira (jogosultság, határesetek) célzott egységtesztek memóriabeli tárral; a `make ci`-ba lefedettségi küszöb (pl. az összevont érték ne csökkenjen), és a PWA-hoz `vitest --coverage`.

## 4. Terhelés: több száz egyidejű felhasználó

**Ami jól skálázódik:** Go + `net/http` (goroutine kapcsolatonként), a busz csak egy sorszámot jelez (nem blokkol), a bejelentkezési korlátozó és a munkamenetek Postgresben vannak, a feltöltés lemezre streamel, a JSON-kérések 1 MiB-ra korlátozottak, kérésenkénti olvasási/írási időkorlát van.

**Szűk keresztmetszetek (súlyosság szerint):**
1. **WebSocket-szétosztás (javítva a PR-ben):** minden új eseménynél minden nyitott kapcsolat külön `SELECT … FROM event WHERE seq > $1`-et futtatott (`gateway.go`, `catchUp`). 300 kapcsolatnál egy üzenet ≈ 300 lekérdezés; egy aktív csatornán (10 esemény/mp) ≈ 3000 lekérdezés/mp. A javítás munkaterületenként egyetlen közös olvasást és egy memóriabeli gyűrűpuffert használ, így eseményenként ~1 lekérdezés marad.
2. **Kapcsolatgyűjtő mérete:** a `pgxpool` alapértéke `max(4, CPU-szám)` (a homelabon 4). Konfigurálható kód nélkül a `COLONY_DATABASE_URL`-ben: `?pool_max_conns=20` (a Postgres `max_connections`-éhez és a közös Postgres többi felhasználójához igazítva).
3. **Láthatósági ellenőrzés:** a nem admin kapcsolatok csatornánként egyszer kérdezik le a `CanRead`-et (gyorsítótárazva); ez rendben van, de egy hozzáférés-változás után a teljes gyorsítótár újraépül.
4. **Eseményfogyasztók:** 12 fogyasztó, mindegyik eseményenként külön tranzakcióval és ofszet-írással. Nagy üzenetforgalomnál (100+ esemény/mp) ez a második szűk pont; kötegelt ofszet-írással gyorsítható.
5. **Egyetlen író az eseménynaplóban** (`pg_advisory_xact_lock`): szándékos (sorrend = commit-sorrend), néhány száz írás/mp-ig nem gond, de ez a rendszer írási plafonja.

**Becslés:** a javítás után egy példány (4 mag, 8 GB, közös Postgres) néhány száz egyidejű böngésző- és agentkapcsolatot, és másodpercenként néhány tíz üzenetet kényelmesen elbír; ezt **mérni kell** egy külön gépen (k6 vagy saját Go-terhelő a `/ws/ui`-ra + üzenetküldés), nem az éles homelabon. Javasolt mérőszám: p95 kézbesítési késés 300 kapcsolatnál, 20 üzenet/mp mellett.

## 5. Több backend terheléselosztó mögött (előfizetős hosztolás)

Ma `COLONY_ROLE=all`, egy példány. Két példány **hibásan** működne:

| Komponens | Gond két példánynál | Teendő |
|---|---|---|
| `bus.InProc` | A B példány nem tud az A-n írt eseményről (csak az 5 mp-es tartalék lekérdezés pótolja) | Postgres `LISTEN/NOTIFY` adapter a `bus.Bus` porthoz (tervezve) |
| 12 eseményfogyasztó | Mindkét példány feldolgozza ugyanazt az eseményt → dupla kézbesítés, dupla ébresztés | Fogyasztónként advisory lock (vezérválasztás) vagy `consumer_offset` sorzár |
| Háttérciklusok (ütemező, kérdések, mentett emlékeztetők, karbantartás, workshop-rutinok, naptár) | Duplán futnak, pl. egy ütemezés kétszer küld | Egy vezérpéldány advisory lockkal, vagy `FOR UPDATE SKIP LOCKED` a sorokon |
| Őr-kapcsolatok és közvetítések (`hosts/fsrelay.go`, `loginrelay.go`, `files/fsrelay.go`, `gateway` progress/working állapot) | A kérés rossz példányra érkezik, ahol nincs ott az őr | `host_connection` tábla + célzott NOTIFY a példánynak (tervezve) |
| Fájlok (helyi kötet) | A másik példány nem látja | S3-kompatibilis tár adapter (`files/storage` port már van) |
| Munkamenet, belépési korlát | — | Már Postgresben, rendben |

A port-alapú felépítés miatt ez **adapter-csere, nem átírás**: becslés 2–4 hét (NOTIFY-busz, vezérválasztás, példány-célzott közvetítés, S3-tár, többpéldányos e2e teszt). Sticky session nem kell. Előfizetős működéshez ezen felül: admin MFA (lásd 6.), munkaterületenkénti kvóták és mérés, mentés/visszaállítás ügyfelenként (`docs/22`, 10–12. pont).

## 6. Biztonság

Kiindulás a 40. kör auditja (`docs/14-biztonsagi-ellenorzes.md`, ASVS 4.0 L2: 31 ✅ / 3 🟡 / 2 ❌), ellenőrizve a mostani kódon:

- **Erős:** munkaterületenkénti RLS (`FORCE`) + kereszt-bérlős e2e minden útvonal-paraméterre; CSRF-token minden sütis írásnál; argon2id; tokenek csak hash-elve; tartós belépési korlát; CSP/XFO/COOP/nosniff, `no-store` az API-n; a WebSocket az `Origin`-t ellenőrzi (coder/websocket alapértelmezés); sqlc paraméteres SQL; SSRF-védelem; agenttartalom adatként keretezve; konténer csak olvasható gyökérrel, `cap_drop: ALL`.
- **Nyitott:**
  1. **Admin MFA hiányzik** (V4.3.1) — előfizetős hosztolás előtt kötelező (az operátornak már van TOTP).
  2. **Kiszivárgott jelszó ellenőrzés** (V2.1.7) — k-anonim HIBP-lekérdezés vagy helyi lista.
  3. **HSTS** a szerver nem küldte — a PR bekapcsolja, ha `COLONY_SECURE_COOKIES` aktív.
  4. Az őrök `bypassPermissions` módja (admin választhatja) teljes gépi hozzáférést ad; hosztolt őrnél ez külön elkülönítést kíván (`docs/24`, 11. pont).
  5. A beszélgetések titkosítatlanul vannak az adatbázisban (V8.3) — hosztolt ajánlatnál lemez-/mentés-titkosítás javasolt.

## 7. A PR-ben elvégzett javítások

Ág: `fix/load-and-hardening` (4 commit, a teljes Go-tesztkészlet és a `make lint` zöld):

1. **Közös eseményolvasás a WebSocket-kapcsolatoknak** (`internal/gateway/feed.go`): munkaterületenként egy „feed”; az egyidejű olvasások egy lekérdezéssé vonódnak össze, a legutóbbi 1024 esemény memóriában marad; a régebbi kurzor továbbra is az adatbázisból olvas; RLS-hatókör marad, a láthatósági szűrés kapcsolatonként változatlan; az utolsó kapcsolattal a feed felszabadul. Teszt: N kapcsolat egy esemény után **1** naplóolvasással ér utol (korábban N, illetve 2N); munkaterületek elkülönítése; régi kurzor; `-race`.
2. **HSTS** (`Strict-Transport-Security: max-age=31536000`, aldomainek nélkül), ha `COLONY_SECURE_COOKIES` be van kapcsolva (alapból igen); a `docs/14` V14.4 sora frissítve.
3. **Az őr időtúllépés-üzenete egy helyen** (`internal/guard/timeout.go`), a `guard.go` és a `direct.go` másolata helyett, táblás teszttel.
4. **Másolt segédek összevonása:** `httpx.PathID`, `ids` null-segéd, `platform/textutil.Clip` (csak a bájtra azonos példányok; az eltérő viselkedésűek maradtak), 31 fájlban.

## 8. Javasolt következő lépések (prioritás szerint)

1. A PR beolvasztása, majd terheléses mérés külön gépen (300 WS-kapcsolat, 20 üzenet/mp).
2. `pool_max_conns` beállítása az éles `COLONY_DATABASE_URL`-ben.
3. Admin MFA (TOTP, az operátor kódjára építve).
4. Többpéldányos futás: NOTIFY-busz → fogyasztó- és háttérciklus-vezérválasztás → őr-közvetítés példány-célzással → S3-tár.
5. A 1000+ soros fájlok szétbontása (először `guard/guard.go` és `conversation/service.go`), a kis lefedettségű modulok (`files`, `conversation`, `identity`) célzott egységtesztjei.
