# Handoff Report: Git linkek és Gitea integráció (Git Links & Gitea Integration)

Dátum: 2026-10-16  
Ág: `feature/gitea`  
Szerző: Antigravity CLI  
Felülvizsgáló: Claude Code  

---

## 1. Változtatások commitonként

- `d86d224` **Add git tables migration and OpenAPI contract**
  - Adatbázis migráció: `migrations/00012_git.sql` létrehozza a `git_link` (link gyorsítótár: URL, kind, cím, állapot, szerző, lekérés ideje), a `git_repo_channel` (repó-csatorna hozzárendelés munkaterületenként) és a `git_webhook_delivery` (webhook idempotencia nyilvántartás) táblákat a szükséges indexekkel.
  - OpenAPI specifikáció: `api/openapi.yaml` kiegészítve a git REST végpontokkal (`GET/POST /api/v1/git/mappings`, `DELETE /api/v1/git/mappings/{owner}/{repo}`, `POST /api/v1/hooks/gitea`), sémákkal (`GitRepoMapping`, `GitRepoMappingList`, `SetGitRepoMappingRequest`, `LinkCard`), valamint a `Message` sémában a `link_cards` mezővel.
  - SQL lekérdezések: `internal/modules/git/postgres/queries.sql`.
  - Konfiguráció: `sqlc.yaml` és `.golangci.yml` frissítve a git modulhoz.
  - Kódgenerálás: `make gen` legenerálta a típusos `apispec` és `sqlcgen` Go kódokat.

- `e8558c9` **Implement git domain model, provider port, and store adapters**
  - Domain modell és típusok (`internal/modules/git/model.go`): `LinkCard`, `RepoChannel`, `LinkKind` konstansok, `ValidateRepo` ellenőrző függvény (traversal védelem), szabványos hibák.
  - Szolgáltatói port és Gitea adapter (`internal/modules/git/provider/`):
    - `GitProvider` port interfész (`ParseURL`, `ParseRef`, `FetchResource`, `ParseWebhook`).
    - `GiteaProvider` implementáció: URL és shorthand parsing (`owner/repo#12`, `owner/repo@sha`), SSRF védelem (csak a beállított Gitea hostra fut kérés), HMAC-SHA256 aláírás-ellenőrzés konstans idejű összehasonlítással, eseményleképezés (pull_request, push, status).
  - Tár port és adapterek (`internal/modules/git/store.go`):
    - Memóriabeli referencia adattár (`internal/modules/git/memory/store.go`).
    - PostgreSQL adattár (`internal/modules/git/postgres/store.go`).
  - Szerződéstesztek (`internal/storetest/storetest.go`): `testGit` tesztkészlet memória és Postgres háttértárra.

- `9db9005` **Implement git service with unfurling, webhook handling, and bot actor**
  - Eseményalapú aszinkron link kibontás (`internal/modules/git/service.go`): a `conversation.EventMessageCreated` eseményre feliratkozó `git.unfurl` fogyasztó a háttérben bontja ki a linkeket és menti a 10 perces gyorsítótárba; a chat üzenetküldés nem blokkolódik.
  - Üzenet API linkkártyák dúsítása (`internal/modules/conversation/`): a `Message` modell `LinkCards` mezővel bővült, az üzenetolvasáskor a git modul szolgáltatja a gyorsítótárazott kártyákat.
  - Rendszer-bot actor (`@gitea`): az `identity.EnsureSystemActor` metódus biztosítja az aktív, token nélküli agent actort, amelynek nevében az automatikus értesítések érkeznek.
  - Webhook üzleti logika: idempotencia ellenőrzés (`delivery_id`), ismeretlen repók figyelmen kívül hagyása, sikeres esemény esetén üzenetküldés a konfigurált csatornába.

- `63ea67e` **Add REST handlers and MCP tools for git module**
  - REST és webhook kezelők (`internal/modules/git/httpapi/handlers.go`): hozzárendelések listázása, beállítása és törlése csak adminoknak; nyilvános webhook fogadó `X-Gitea-Signature` ellenőrzéssel.
  - Agent MCP eszköz (`internal/modules/git/mcptools/tools.go`): `git_status` (URL vagy `owner/repo#12` / `owner/repo@sha` -> egysoros tömör összefoglaló a context-kímélés érdekében).
  - Szerver huzalozás (`internal/server/server.go`): `git.Service` létrehozása, eventlog fogyasztó regisztrációja, REST és MCP végpontok bekötése.

- `7befbdb` **Add git link cards and repository mapping UI to dev page**
  - Dev felület (`internal/httpserver/web/index.html`):
    - Linkkártyák megjelenítése az üzenetek alatt (ikon típus szerint: 🔀 PR, 🟢 issue, 📦 repo, 📝 commit, külső link, állapotjelvény, szerző).
    - „Gitea tárak hozzárendelése” szekció a „Gépek” panelen az adminoknak: meglévő összerendelések listája, törlés gomb, új repó -> csatorna űrlap.
    - Szigorúan biztonságos DOM kezelés (`el()`, `textContent`), nincs XSS lehetőség.

- `4cd1a2a` **Configure gitea url, token, and webhook secret**
  - Környezeti változók támogatása (`internal/platform/config/config.go`): `COLONY_GITEA_URL`, `COLONY_GITEA_TOKEN`, `COLONY_GITEA_WEBHOOK_SECRET`.
  - Szerver indítás bekötése (`cmd/colonyhall/main.go`).

- `fb23c7b` **Add end-to-end tests for git module and gitea integration**
  - `internal/e2e/git_test.go`:
    - `TestGit_UnfurlLinkCardsAndCaching`: PR URL linkkártya kibontása aszinkron drain után, második üzenetnél gyorsítótárból kiszolgálás (nincs újabb HTTP kérés a Gitea felé).
    - `TestGit_SSRFPrevention`: idegen vagy belső host URL nem generál hálózati forgalmat és nem hoz létre linkkártyát.
    - `TestGit_RepoChannelMappingsAPI`: admin és agent jogosultsági ellenőrzés (403 agentnek), listázás, létrehozás, törlés OpenAPI szerződés szerint ellenőrizve.
    - `TestGit_WebhookVerificationAndIdempotency`: hiányzó és hibás aláírás 401 Unauthorized; érvényes HMAC esetén üzenet a csatornába `@gitea` nevében; ismételt delivery id esetén idempotens 200 duplikált üzenet nélkül; ismeretlen repó figyelmen kívül hagyása.
    - `TestGit_MCPStatusTool`: `git_status` MCP hívás teljes URL-lel és `owner/repo#12` shorthanddel.

---

## 2. Érintett fájlok jegyzéke

- `migrations/00012_git.sql` – adatbázis migráció (`git_link`, `git_repo_channel`, `git_webhook_delivery`).
- `api/openapi.yaml` – REST és webhook OpenAPI végpontok és sémák.
- `sqlc.yaml`, `.golangci.yml` – konfigurációs szabályok kiegészítése a git modulhoz.
- `internal/apispec/apispec.gen.go` – generált Go típusok.
- `internal/modules/git/model.go` – git domain típusok, hibák, validáció.
- `internal/modules/git/provider/provider.go` – `GitProvider` port interfész és típusok.
- `internal/modules/git/provider/gitea.go`, `gitea_test.go` – Gitea adapter és egységtesztek.
- `internal/modules/git/store.go` – Store port felület.
- `internal/modules/git/memory/store.go` – memóriabeli adattár.
- `internal/modules/git/postgres/store.go`, `queries.sql`, `sqlcgen/` – PostgreSQL adattár és lekérdezések.
- `internal/modules/git/service.go`, `service_test.go` – üzleti logika, linkkivonás, webhook kézbesítés.
- `internal/modules/git/httpapi/handlers.go` – REST és webhook HTTP kezelők.
- `internal/modules/git/mcptools/tools.go` – `git_status` MCP eszköz.
- `internal/modules/conversation/model.go`, `service.go`, `reactions.go`, `httpapi/handlers.go` – `LinkCard` integráció az üzenetekhez.
- `internal/modules/identity/service.go` – `EnsureSystemActor` metódus a `@gitea` bot actor létrehozásához.
- `internal/platform/config/config.go` – Gitea konfigurációs változók (`COLONY_GITEA_*`).
- `internal/server/server.go` – szerver oldali huzalozás, fogyasztók, HTTP és MCP regisztráció.
- `cmd/colonyhall/main.go` – CLI indítás paraméterezése.
- `internal/storetest/storetest.go` – közös szerződésteszt a memóriabeli és Postgres adattárakhoz.
- `internal/httpserver/web/index.html` – fejlesztői weboldal: linkkártyák és Gitea adminisztráció.
- `internal/e2e/git_test.go` – végponttól végpontig tartó integrációs tesztek fake Gitea szerverrel.
- `docs/01-terv.md`, `docs/02-muszaki-specifikacio.md`, `CLAUDE.md` – dokumentáció és ütemezés frissítése.

---

## 3. Biztonsági és működési szabályok megvalósítása

| Követelmény | Megvalósítás | Teszt |
|---|---|---|
| **SSRF védelem** | A szerver kizárólag a `COLONY_GITEA_URL`-ben beállított hostnevet hajlandó lekérdezni; minden más host figyelmen kívül marad. 5s kliens timeout, max 1MB válaszméret. | `TestGit_SSRFPrevention`, `TestGiteaProvider_ParseURL` |
| **HMAC aláírás ellenőrzés** | `X-Gitea-Signature` (vagy `X-Hub-Signature-256`) ellenőrzése konstans idejű összehasonlítással (`subtle.ConstantTimeCompare`). Érvénytelen aláírásnál 401 Unauthorized. | `TestGit_WebhookVerificationAndIdempotency`, `TestGiteaProvider_ParseWebhook` |
| **Webhook idempotencia** | Kézbesítéskor a delivery azonosító bekerül a `git_webhook_delivery` táblába. Replay esetén a szerver 200 OK-val nyugtáz, de nem küld duplikált chat üzenetet. | `TestGit_WebhookVerificationAndIdempotency`, `TestService_WebhookAndIdempotency` |
| **Aszinkron link kibontás** | Üzenetküldéskor azonnali mentés és válasz. A linkeket a `git.unfurl` eseményfigyelő bontja ki a háttérben, és a `git_link` táblában 10 perces gyorsítótárazással rögzíti. | `TestGit_UnfurlLinkCardsAndCaching`, `TestService_HandleEvent_Unfurl` |
| **Rendszer-bot actor** | `@gitea` nevű, `agent` típusú actor token és ébresztési jogosultság nélkül. A chatben tisztán elkülönül az emberi és normál AI felhasználóktól. | `TestGit_WebhookVerificationAndIdempotency` |
| **Adminisztrációs jogosultság** | A repó–csatorna hozzárendelést kizárólag emberi adminisztrátorok (`by.IsAdmin()`) módosíthatják; agenteknek 403 Forbidden. | `TestGit_RepoChannelMappingsAPI` |
| **Tömör MCP válasz** | `git_status` egysoros kimenetet ad: `#12 Cím [állapot] by @szerző`, minimalizálva az agent context terhelését. | `TestGit_MCPStatusTool` |

---

## 4. Lefuttatott ellenőrzések és eredmények

1. **`make test`**
   - Eredmény: `PASS`
   - Minden egységteszt, adattár szerződésteszt és végponttól végpontig tartó teszt lefutott.
2. **`make lint`**
   - Eredmény: `golangci-lint run ./...` -> `0 issues.`
   - A `depguard` rétegszabályok (modul magban nincs hálózati I/O), a `gofmt` és a hibakezelési ellenőrzések maradéktalanul rendben vannak.
3. **`make gen-check`**
   - Eredmény: `git status --porcelain` üres a `make gen` után; a generált fájlok szinkronban vannak az OpenAPI specifikációval és az SQL lekérdezésekkel.
4. **`go test -race ./internal/guard ./internal/e2e`**
   - Eredmény: `PASS`, nincs adatseregtörés (race condition).
5. **`make test-pg`**
   - Eredmény: `PASS`, a PostgreSQL háttértár tesztek (beleértve a `git` táblákat) hibátlanul futnak le.
