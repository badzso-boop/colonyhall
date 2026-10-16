# Handoff Report: Csapat Wiki (The Team Wiki)

Dátum: 2026-10-16  
Ág: `feature/wiki`  
Szerző: Antigravity CLI  
Felülvizsgáló: Claude Code  

---

## 1. Változtatások commitonként

- `8817085` **Add wiki migration and OpenAPI contract**
  - Adatbázis migráció: `migrations/00009_wiki.sql` létrehozza a `wiki_page` (oldal metaadatok, slug, cím, címkék, létrehozó, törlési időbélyeg) és `wiki_revision` (append-only történet: `rev`, `body`, `kind`, `author_id`, `created_at`, PK: `page_id, rev`) táblákat és indexeket.
  - OpenAPI specifikáció: `api/openapi.yaml` kiegészítve a wiki REST végpontokkal (`GET/POST /api/v1/wiki`, `GET/PUT/DELETE /api/v1/wiki/{slug}`, `POST /api/v1/wiki/{slug}/append`, `POST /api/v1/wiki/{slug}/restore`, `GET /api/v1/wiki/{slug}/revisions`) és sémákkal.
  - SQL lekérdezések: `internal/modules/wiki/postgres/queries.sql`.
  - Konfiguráció: `sqlc.yaml` és `.golangci.yml` frissítve a wiki modulhoz.
  - Kódgenerálás: `make gen` legenerálta a típusos `apispec` és `sqlcgen` Go kódokat.

- `55b0ee0` **Implement server wiki module**
  - Domain modell és típusok (`internal/modules/wiki/model.go`): `Page`, `Revision`, `PageIndex`, korlátok (cím ≤ 120, törzs ≤ 50 000, hozzáfűzés ≤ 8 000, slug ≤ 80), revízió típusok, hibák, ékezetmentesítő (`FoldAccents`), slug generáló (`Slugify`) és `Excerpt` segédfüggvény.
  - Audit események (`internal/modules/wiki/events.go`): `wiki.created`, `wiki.appended`, `wiki.edited`, `wiki.deleted`, `wiki.restored` (payloadban csak `page_id`, `slug`, `title`, `rev`, `kind`, törzs nélkül).
  - Tár port és adapterek (`internal/modules/wiki/store.go`):
    - In-memory referencia implementáció (`internal/modules/wiki/memory/store.go`).
    - PostgreSQL implementáció (`internal/modules/wiki/postgres/store.go`).
  - Alkalmazási réteg (`internal/modules/wiki/service.go`): szigorú jogosultsági ellenőrzések (ember vs agent, halted állapot), egyedi slug generálás ütközéskezeléssel és 80 karakteres korláttal, hozzáfűzés automatikus fejléccel (`\n\n---\n*@handle, YYYY-MM-DD HH:MM:*\n\n`), tranzakciós eseménynaplózás és busz értesítés.
  - REST adapter (`internal/modules/wiki/httpapi/handlers.go`): HTTP végpontok regisztrálása, hibák leképezése RFC 9457 problem válaszokra (`httpx`).
  - MCP transzport (`internal/modules/wiki/mcptools/tools.go`): 4 eszköz agenteknek (`wiki_index`, `wiki_read`, `wiki_create`, `wiki_append`), 8000 karakter feletti olvasás csonkolásával; szerkesztés/törlés szándékosan hiányzik.
  - Szerver huzalozás (`internal/server/server.go`): a wiki modul bekötése a szerverbe és az MCP kiszolgálóba.
  - Szerződéstesztek (`internal/storetest/storetest.go`, `backends_test.go`): átfogó `testWiki` tesztkészlet, amely igazolja a memóriabeli és postgres adattár tökéletes felcserélhetőségét.

- `257e29e` **Add wiki instruction to direct prompt**
  - Az őr rendszerutasításának kiegészítése (`internal/guard/direct.go`: `DirectPrompt`): feladatkezdés előtt a `wiki_index` megtekintése, tartós tudás rögzítése a feladat végén `wiki_append`-del (vagy `wiki_create`-tel).
  - Egységteszt frissítés (`internal/guard/direct_test.go`: `TestDirectPrompt`).

- `243a802` **Add wiki UI to dev page**
  - Dev oldal (`internal/httpserver/web/index.html`):
    - Új gomb a fejlécben: `📖 Wiki` (minden belépett munkatér-tagnak elérhető).
    - Index nézet: keresőmező (`q`), `+ Új oldal` létrehozó doboz, oldalkártyák (cím, slug, excerpt, címkék, módosítási idő).
    - Oldal nézet: metaadatok, pre-wrap formázott plain text törzs, hozzáfűzés doboz (`wiki_append`), emberi szerkesztő doboz (`PUT` concurrency `base_rev`-vel), törlés és visszaállítás gombok.

- `a7c5b24` **Add e2e tests for wiki module**
  - `internal/e2e/wiki_test.go`:
    - `TestWikiMCPLifecycle`: agent teljes folyamata valódi MCP kapcsolaton át (`wiki_create` -> `wiki_index` -> `wiki_read` -> `wiki_append` -> `wiki_read` formátum ellenőrzés).
    - `TestWikiAuthorization`: agent REST hívással sem tud editálni/törölni/visszaállítani (403); emberi szerkesztés elavult `base_rev`-vel 409 Conflict; soft delete után az oldal eltűnik az indexből és az agentnek 404, de ember látja `?deleted=1`-gyel; visszaállítás után újra aktív.
    - `TestWikiLimitsAndValidation`: kötelező és maximális mezőhosszak validációja (400), vészleállítás (halted actor) alatti írási tiltás (403).
    - `TestWikiSlugCollisions`: magyar ékezetes karakterek ASCII alakra hajtogatása (`Árvíztűrő Tükörfúrógép` -> `arvizturo-tukorfurogep`), ütközéskor `-2` sorszámozás, hosszú címeknél 80 karakteres törzslevágás a sorszám figyelembevételével.

---

## 2. Érintett fájlok jegyzéke

- `migrations/00009_wiki.sql` – goose adatbázis migráció (`wiki_page`, `wiki_revision`).
- `api/openapi.yaml` – wiki végpontok és sémák.
- `sqlc.yaml`, `.golangci.yml` – konfigurációs frissítések.
- `internal/apispec/apispec.gen.go` – generált OpenAPI típusok.
- `internal/modules/wiki/model.go` – domain modellek, korlátok, ékezetmentesítés, slugosítás.
- `internal/modules/wiki/events.go` – audit események és típusok.
- `internal/modules/wiki/store.go` – Store port felület.
- `internal/modules/wiki/memory/store.go` – memóriabeli referencia adattár.
- `internal/modules/wiki/postgres/store.go` – Postgres adattár.
- `internal/modules/wiki/postgres/queries.sql` – SQL lekérdezések.
- `internal/modules/wiki/postgres/sqlcgen/` – sqlc által generált Go kód.
- `internal/modules/wiki/service.go` – üzleti logika és jogosultsági szabályok.
- `internal/modules/wiki/httpapi/handlers.go` – REST HTTP kezelők.
- `internal/modules/wiki/mcptools/tools.go` – agent MCP eszközök.
- `internal/server/server.go` – modulok összerakása és regisztrációja.
- `internal/storetest/storetest.go`, `backends_test.go` – közös adattár-tesztek.
- `internal/guard/direct.go`, `direct_test.go` – őr direkt prompt kiegészítés és teszt.
- `internal/httpserver/web/index.html` – fejlesztői webes felület wiki támogatással.
- `internal/e2e/wiki_test.go` – végponttól végpontig tartó szerződéstesztek.
- `docs/02-muszaki-specifikacio.md`, `CLAUDE.md` – dokumentáció és ütemezés.

---

## 3. Jogosultsági szabályok megvalósítása és tesztelése

| Szabály | Megvalósítás | Teszt |
|---|---|---|
| Olvasás (bárki a munkatérből) | `service.Get`, `service.List`, `service.Revisions` | `TestWikiMCPLifecycle`, `TestWikiAuthorization` |
| Törölt oldal elrejtése agent elől | `GET /wiki/{slug}` agentnek 404; `?deleted=1` paramétert agentnél figyelmen kívül hagyja a szerver | `TestWikiAuthorization` |
| Létrehozás (ember és agent) | `service.Create`: cím (≤120), törzs (≤50000), kezdő rev: 1 | `TestWikiMCPLifecycle`, `TestWikiLimitsAndValidation` |
| Hozzáfűzés (ember és agent) | `service.Append`: szöveg (≤8000), törzs méretlimit (≤50000), automatikus `@handle, YYYY-MM-DD HH:MM` fejléc | `TestWikiMCPLifecycle` |
| Teljes szerkesztés (csak ember) | `service.Edit`: `by.Kind != KindHuman` -> 403 Forbidden; elavult `base_rev` -> 409 Conflict | `TestWikiAuthorization` |
| Törlés és visszaállítás (csak ember) | `service.Delete`, `service.Restore`: `by.Kind != KindHuman` -> 403 Forbidden | `TestWikiAuthorization` |
| Halted állapotú actor írási tiltása | `by.Halted` -> 403 Forbidden (`identity.ErrHalted`) | `TestWikiLimitsAndValidation` |
| MCP korlátozás | MCP toolként csak `wiki_index`, `wiki_read`, `wiki_create`, `wiki_append` létezik | `TestWikiMCPLifecycle`, `internal/modules/wiki/mcptools/tools.go` |

---

## 4. Lefuttatott ellenőrzések és eredmények

1. **`make test`**
   - Eredmény: `PASS`
   - Valamennyi belső csomag tesztjei (e2e, guard, identity, conversation, delivery, policy, storetest, wiki) hiba nélkül lefutottak.
2. **`make lint`**
   - Eredmény: `golangci-lint run ./...` -> `0 issues.`
   - A `depguard` rétegzési szabályok és a `gofmt` maradéktalanul teljesülnek.
3. **`make test-pg`**
   - Eredmény: `COLONY_TEST_DATABASE_URL=... go test ./internal/storetest/ -run TestPostgres -v` -> `PASS`
   - Valamennyi PostgreSQL adattár-teszt (beleértve a `TestPostgres/wiki`-t) hibátlanul zöld.
4. **`make gen-check`**
   - Eredmény: `git diff --exit-code -- internal/apispec $(find internal -type d -name sqlcgen)` -> 0 eltérés, a generált kód naprakész.
5. **Versenyhelyzet-ellenőrzés (`-race`)**
   - Parancs: `go test -count=1 -race ./internal/guard ./internal/e2e`
   - Eredmény: `ok colonyhall/internal/guard`, `ok colonyhall/internal/e2e` (0 adatverseny).

---

## 5. Megjegyzések a felülvizsgálónak

1. **Összeférhetőség a párhuzamos ágakkal:**
   - A `migrations/00009_wiki.sql` a `main` ághoz képest a 9-es sorszámú migráció.
   - Ha a korábbi feature ágak (pl. `feature/approvals`) szintén a 9-es sorszámot használták a saját águkon, a `main`-re való összevonáskor az egyik migrációt át kell majd sorszámozni (pl. `00010_...`-re).
2. **Szerződés-első érvényesítés az e2e tesztekben:**
   - Minden e2e HTTP hívás automatikusan átmegy az `openapi3filter.ValidateResponse` ellenőrzőn, így a wiki végpontok válaszai szigorúan egyeznek az `api/openapi.yaml` leírással.
