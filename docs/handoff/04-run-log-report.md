# Handoff Report: Futásnapló és tokenfelhasználás a szerveren (Run Log)

Dátum: 2026-10-16  
Ág: `feature/run-log`  
Szerző: Antigravity CLI  
Felülvizsgáló: Claude Code  

---

## 1. Változtatások commitonként

- `633c709` **Add agent_run migration and OpenAPI contract for run logging**
  - Új append-only adatbázistábla migráció (`migrations/00009_agent_runs.sql`): `agent_run` tábla az összes futási mezővel (`id`, `workspace_id`, `agent_id`, `host_id`, `phase`, `client`, `model`, `session_id`, `channel_id`, `thread_root_id`, `items`, `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`, `duration_ms`, `status`, `error_id`, `created_at`) és összetett index `(workspace_id, agent_id, created_at)`.
  - OpenAPI szerződésbővítés (`api/openapi.yaml`): új végpontok (`POST /api/v1/agent/runs`, `GET /api/v1/runs`, `GET /api/v1/usage`) és sémák (`ReportRunRequest`, `RunRef`, `Run`, `RunList`, `DayUsage`, `UsageResponse`).
  - Új sqlc lekérdezések (`internal/modules/runs/postgres/queries.sql`): `InsertRun`, `ListRuns` (legújabb elöl, opcionális agent szűrés, limit), és `Usage` (napi aggregáció `Europe/Budapest` időzónában `created_at >= $2` feltétellel).
  - Kódgenerálás lefutott (`make gen`): `apispec` és `sqlcgen` szinkronban.

- `4a34832` **Implement runs service, memory and postgres stores, and HTTP endpoints**
  - Bővített `runs` modul:
    - `model.go`: konstansok (`PhaseDecide`, `PhaseWork`, `PhaseDirect`, `StatusOk`, `StatusError`, `StatusHalted`, `MaxTokens`, `MaxDurationMS`), struktúrák (`Run`, `DayUsage`), hibák (`ErrInvalidRun`, `ErrInvalidDays`) és a `Budapest` időzóna referencia (`time.LoadLocation("Europe/Budapest")`).
    - `events.go`: új audit esemény konstans és összefoglaló payload (`EventRunReported` / `RunReported`).
    - `store.go`, `postgres/store.go`, `memory/store.go`: `InsertRun`, `ListRuns`, és `Usage` implementációk mindkét tárolóban azonos rendezési és aggregációs logikával.
    - `service.go`: `ReportRun` (jogosultság-ellenőrzés, paramétervalidáció, tranzakciós beszúrás, audit esemény hozzáadása és eseménybusz értesítés), `ListRuns` (csak ember, limit 1..200), és `Usage` (csak ember, days 1..90, budapesti napi ablak kezdőpontjának kiszámítása).
    - `httpapi/handlers.go`: HTTP útvonalak regisztrálása, hitelesítés, validáció és hibakód-leképezések.
    - `storetest/storetest.go`: új tesztek `testRuns`-ban a memóriára és Postgresre, beleértve a CEST (UTC+2) éjféli határ tesztelését 21:59:59 UTC és 22:00:01 UTC között.

- `9bf6215` **Report decide, work, and direct runs from the guard to the server**
  - `Client.ReportRun` és `ReportRunInput` hozzáadása az őr klienshez (`internal/guard/client.go`).
  - `reportError` visszatérési értékének módosítása `(string, error)` párra (errorID és hiba), így a hibát generáló futások átadhatják az `error_id`-t a futásjelentésnek.
  - Az őr agent döntési (`decide`), munka (`work`) és közvetlen (`direct`) futásai után meghívja a `reportRun` segédmetódust:
    - Best-effort jelentés (ha sikertelen, csak figyelmeztetést naplóz, a futást nem akasztja meg).
    - `StatusHalted`, `StatusError`, vagy `StatusOk` megfelelő beállítása.
    - Munka- és közvetlen futás hibájánál a kapott `error_id` továbbítása.
    - Meglévő `logRun` és `logDirectRun` naplósorok érintetlenül maradtak.

- `437f703` **Add runs panel to dev page with token usage and recent runs**
  - Fejlesztői felület (`internal/httpserver/web/index.html`):
    - 📊 gomb (`runsbtn`) a fejlécben, amely csak embereknek jelenik meg (`start()`).
    - Új `runsPanel` konténer és kölcsönös panelkizárás a „Gépek” (`hostsPanel`) és az „Ütemezés” (`schedPanel`) panelekkel.
    - Napi és 7 napos aggregált táblázat agentenként (időszak, in, out, összes token, futásszám, hibaszám).
    - Legutóbbi 20 futás táblázata (időpont, agent, fázis, modell, tokenek száma, futásidő másodpercben, státusz).
    - Hibás futás esetén „Hiba részletei” gomb, amely közvetlenül megnyitja a meglévő hibamodált (`showError(error_id)`).
    - XSS-mentes megvalósítás: minden dinamikus adat kizárólag `textContent`-tel épül fel, nincs `innerHTML` adatokkal.
    - Élő frissítés WebSocket `run.reported` eseményre, ha a panel nyitva van.

- `35a5a8b` **Add e2e contract and permission tests for run logging and usage**
  - Új végponttól végpontig tartó tesztcsomag (`internal/e2e/runs_test.go`):
    - `TestRunLoggingFromGuard`: szkriptelt őr futás, döntési fázis tokenjeinek lejelentése, ember általi listázás (`GET /runs`), szűrés `agent_id` szerint, napi használat (`GET /usage?days=7`).
    - `TestRunLoggingWorkError`: sikertelen munkafutás esetén az `error_id` bekötése és elérhetősége a `/api/v1/errors/{id}` végponton, hibaszám növekedése a használati statisztikában.
    - `TestRunLoggingPermissions`: agent nem olvashatja a futásokat vagy a használatot (403), ember nem jelenthet be futást az agent végponton (403).
    - `TestRunLoggingValidation`: negatív tokenek, túl nagy tokenek (>10^9), ismeretlen fázis, ismeretlen státusz, 24 óránál hosszabb futás, valamint érvénytelen `days` paraméter (<1 vagy >90) mind 400 Bad Request választ adnak.

- (jelenlegi) **Docs: mark run log as implemented and add handoff report**
  - Műszaki leírás (`docs/02-muszaki-specifikacio.md`) és ütemezés (`CLAUDE.md`) frissítése a futásnapló és tokenhasználat megvalósításával.

---

## 2. Hozott döntések (ahol a specifikáció nyitott volt)

1. **Időzóna-kezelés és éjféli határ:** A napi aggregáció `Europe/Budapest` időzónában történik mind az SQL lekérdezésben (`TO_CHAR(created_at AT TIME ZONE 'Europe/Budapest', 'YYYY-MM-DD')`), mind a memóriatárolóban (`runs.Budapest`). A `storetest` expliciten ellenőrzi a CEST (UTC+2) éjféli határon átívelő futásokat (21:59:59 UTC = 23:59:59 CEST előző nap, míg 22:00:01 UTC = 00:00:01 CEST következő nap).
2. **Hívó identitásának kényszerítése:** A `POST /api/v1/agent/runs` végponton az `agent_id` nem adható meg a kérés törzsében: a szerver a hitelesített `Actor` azonosítóját használja fel. Agent nem jelenthet le futást másik agent nevében.
3. **Best-effort lejelentés az őrben:** A futás befejeztével a futáslejelentés egy 5 másodperces időkorláttal ellátott háttérkontextussal fut le. Hiba esetén a hiba részlete figyelmeztetésként bekerül az őr naplójába, de a guard ciklus zavartalanul folytatódik (nem dob hibát a munkafutásra).
4. **Hibakapcsolat:** Amikor egy munka- vagy közvetlen futás hibával zárul, az őr először elmenti a részleteket a `reportError` hívással, majd a kapott `error_id`-t csatolja a `reportRun` kéréshez, így a futásnaplóból közvetlenül elérhető a hiba összefoglalója és részlete.
5. **Megjelenítés a fejlesztői felületen:** A felületen egy agentenkénti összefoglaló kártya jeleníti meg a mai nap és az elmúlt 7 nap aggregált tokenjeit (be, ki, összes), futás- és hibaszámait, alatta pedig a legutóbbi 20 futás részletes táblázata látható.

---

## 3. Ami kimaradt vagy szándékosan nem készült el

- **Élő AI CLI futtatások:** A szabályoknak megfelelően a tesztek kizárólag szimulált runnerrel és determinisztikus adapterekkel futottak.
- **Költségkorlátok és büdzsé (budget limits):** A specifikációnak megfelelően a futásnapló az alapja a későbbi korlátoknak, de korlátozási logika nem került beépítésre.

---

## 4. Lefuttatott parancsok és eredményeik

1. **`make test`**
   - Eredmény: `PASS`
   - Minden belső csomag tesztje lefutott hibák nélkül.
2. **`make test-pg`**
   - Eredmény: `PASS`
   - Valódi PostgreSQL adatbázison a `storetest` teljes készlete lefutott, a `runs` tároló összes metódusával és a budapesti éjféli határral együtt.
3. **`make lint`**
   - Eredmény: `0 issues`
   - `golangci-lint run ./...` hibamentes.
4. **`make gen-check`**
   - Eredmény: `0 diff`
   - Az `apispec` és `sqlcgen` generált fájlok teljesen naprakészek az `openapi.yaml` és az SQL migrációk alapján.
5. **`go test -count=1 -race ./internal/guard ./internal/e2e`**
   - Eredmény: `PASS`
   - A race detector mind az őr csomagon, mind a teljes e2e csomagon tiszta futást igazolt.
