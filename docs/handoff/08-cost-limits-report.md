# Handoff Report: Költséghatárok — Munkamenet-rotáció és napi tokenkeret (Cost Limits)

Dátum: 2026-10-16  
Ág: `feature/cost-limits`  
Szerző: Antigravity CLI  
Felülvizsgáló: Claude Code  

---

## 1. Változtatások commitonként

- `15e483d` **Add cost limit fields to assignment and server daily budget endpoint**
  - **Adatbázis migráció:** `migrations/00012_cost_limits.sql` hozzáadja az `agent_host` táblához a `max_session_tokens` (`int NOT NULL DEFAULT 150000`, 0 = nincs korlát) és `daily_token_limit` (`bigint NOT NULL DEFAULT 0`, 0 = nincs korlát) oszlopokat.
  - **OpenAPI specifikáció:** `api/openapi.yaml` kiegészítve a `HostAgent` és `AssignRequest` mezőkkel (`max_session_tokens`, `daily_token_limit`), az `AgentBudget` sémával (`limit`, `used_today`, `remaining`, `resets_at`), valamint a `GET /api/v1/agent/budget` végponttal.
  - **Kódgenerálás:** `make gen` legenerálta az `apispec` és `sqlcgen` modelleket.
  - **Hosts modul:** `internal/modules/hosts/` frissítve az új mezők kezelésére és lekérdezésére (`AssignInput`, `AssignmentByAgent`).
  - **Runs modul:** `runs.Store` kibővítve `TokensSince(ctx, agentID, since)` metódussal (Postgres és memóriabeli megvalósítás), `runs.Service` lekérdezi a gép-hozzárendelés limitjét a `Hosts` interfészen keresztül, kiszámítja a budapesti nap (`Europe/Budapest`) kezdete óta felhasznált tokeneket, a fennmaradó keretet és a következő éjféli reset időpontját (`resets_at`).
  - **REST végpont:** `GET /api/v1/agent/budget` csak agent token számára engedélyezett (emberi hívó 403 Forbidden).
  - **Tesztek:** storetest kibővítve `TokensSince` ellenőrzéssel memórián és Postgresen; új E2E teszt (`internal/e2e/runs_test.go`: `TestAgentBudgetEndpoint`), amely ellenőrzi az agent-only hozzáférést, a limit- és fogyasztásszámítást, valamint az éjféli napváltást (midnight boundary).

- `c570623` **Implement session rotation in direct mode guard**
  - **Őr kliens és konfiguráció:** `HostAgent` és `AgentConfig` kiegészítve a `MaxSessionTokens` és `DailyTokenLimit` mezőkkel; felügyelő (`host.go`) átadja a konfigurációt az agent indításakor, és a gyorsítótár-kulcsba (`key`) beépítve biztosítja az élő újratöltést.
  - **Helyi állapot perzisztencia:** `internal/guard/direct.go` `DirectState` kiegészítve a `last_run_tokens` és `generations` szálankénti leképezésekkel (0600 jogosultságú atomi mentés `direct_state.json`-ba).
  - **Rotációs logika:** Ha egy szálban az előző futás tokenjei (`input + cache_read + cache_write`) elérik a `max_session_tokens` küszöböt:
    - Növeli a generációt (`generations[threadRoot]++`), és a session kulcs `threadRoot#generation` lesz.
    - Claude Code esetén új determinisztikus session ID generálódik (`UUIDv5(agent, threadRoot#generation)`).
    - Antigravity esetén a korábbi `conversation_id` törlődik a leképezésből (új beszélgetés indul).
    - A szál előző 3 üzenetét (`GET /api/v1/messages/{id}/thread`) kivonatként (`<m ctx from="@author">body…</m>`, 300 rúnára vágva) az új munkamenet promptjának elejére fűzi (`threadRecap`).
    - Egyértelmű értesítést posztol a szálba: `"🔄 Started a fresh session (the previous one reached N tokens)."`.
  - **Egységtesztek:** `TestDirectSessionRotationClaude` és `TestDirectSessionRotationAntigravity` igazolja a rotációs küszöböt, az új session/conversation azonosítókat, a szál-kivonat tartalmát és formátumát, a chat értesítést és az állapot mentését.

- `b24985d` **Enforce daily token budget in guard before runs**
  - **Őr REST kliens:** `Client.Budget(ctx)` hívja a szerver `GET /api/v1/agent/budget` végpontját.
  - **Őr budget ellenőrzés (`internal/guard/guard.go`):**
    - Minden futási fázis előtt (`decide`, `work`, `direct`) az őr meghívja a `checkBudget` segédfüggvényt.
    - Ha `limit > 0 && remaining <= 0`:
      - Nem indít futást és **nem nyugtáz** (az inbox tételek a sorban maradnak feldolgozatlanul).
      - Naponta legfeljebb egyszer bejegyzést tesz a szálba: `"⏳ My daily token budget is used up; I continue after <resets_at helyi idő>."`.
      - Rögzíti `budgetExhaustedUntil = resets_at` időpontot, és a `waitInterval()` ezen időpontig altatja a lekérdező időzítőt, megelőzve a lekérdezési vihart (polling storm).
      - Ha a gépkonfigurációban a határérték emelkedik, a felügyelő újraindítja az agentet, a friss `Agent` pedig azonnal folytatja a munkát.
    - Halt ellenőrzés (`watchHalt`): változatlanul leállítja és leöli a klienst, ha az agentet leállítják/felfüggesztik.
  - **Tesztek (`internal/guard/budget_test.go`):**
    - `TestGuardDailyTokenBudget_Direct`: manuális órával (`Now func() time.Time`) és scriptelt adapterrel igazolja: túllépéskor nincs futás és nincs ack, szálanként naponta pontosan egy értesítés megy ki; limit emelésekor azonnal újraindul a munka és nyugtáz; budapesti éjfél átlépésekor (másnap) a keret nullázódik és az inboxban maradt tétel feldolgozásra és nyugtázásra kerül.
    - `TestGuardDailyTokenBudget_Compact`: igazolja a `decide` előtti és a `work` fázis előtti keretellenőrzést és leállást.

- `fbee697` **Add cost limit inputs and budget display to dev web page**
  - **„Gépek” panel (`hostsPanel`):**
    - Az agent hozzárendelési űrlapon új beviteli mezők: `max_session_tokens` (alapértelmezett 150000) és `daily_token_limit` (alapértelmezett 0 = nincs limit).
    - A hozzárendelt agentek kártyáján megjelennek a beállított limitek (`max session: 150000 · daily limit: 100000`).
    - Új `✏️ limit` gomb az agent kártyán, amellyel a napi limit prompt ablakon át közvetlenül frissíthető.
  - **📊 Futáspanel (`runsPanel`):**
    - Minden agent kártyáján megjelenik a napi tokenkeret állapota: keret, ma felhasznált tokenek száma, hátralévő keret és a budapesti éjféli reset időpontja (`resets_at`).
  - **I18n:** Magyar és angol fordítási kulcsok (`dailyBudget`, `dailyLimit`, `remaining`, `resetsAt`, `noLimit`, `maxSession`, `editLimit`, `enterDailyLimit`).

- `Documentation, handoff report, and final checks`
  - Terv és műszaki specifikáció frissítése (`docs/01-terv.md`, `docs/02-muszaki-specifikacio.md`).
  - Ütemezés frissítése a `CLAUDE.md`-ben.
  - Handoff összefoglaló (`docs/handoff/08-cost-limits-report.md`).

---

## 2. Érintett fájlok jegyzéke

- `migrations/00012_cost_limits.sql` – adatbázis migráció az új limit mezőkhöz.
- `api/openapi.yaml` – OpenAPI szerződés (`AgentBudget`, `/agent/budget`, `HostAgent` limitek).
- `internal/apispec/apispec.gen.go` – generált OpenAPI kód.
- `internal/modules/hosts/model.go`, `service.go`, `httpapi/handlers.go` – gép-hozzárendelés limitek kezelése.
- `internal/modules/hosts/postgres/queries.sql`, `store.go`, `sqlcgen/` – SQL lekérdezések és generált kód.
- `internal/modules/runs/model.go`, `service.go`, `store.go`, `httpapi/handlers.go` – `TokensSince` és `AgentBudget` számítás.
- `internal/modules/runs/postgres/`, `internal/modules/runs/memory/` – Postgres és in-memory `TokensSince` implementációk.
- `internal/server/server.go` – hosts és runs modulok összekötése.
- `internal/storetest/storetest.go` – store szerződéstesztek bővítése.
- `internal/guard/client.go` – őr kliens `AgentBudget` és `Budget` metódusa, `HostAgent` limitek.
- `internal/guard/config.go` – `AgentConfig` limitek.
- `internal/guard/host.go` – felügyelő agent indítás és konfiguráció újratöltés kulcsa.
- `internal/guard/guard.go` – `checkBudget` ellenőrzés decide/work előtt, `budgetExhaustedUntil`, `waitInterval`, `Now` mock óra.
- `internal/guard/direct.go` – session rotáció, `LastRunTokens`, `Generations`, szál-kivonat, szál értesítés, budget ellenőrzés direct előtt.
- `internal/guard/direct_test.go` – Claude és Antigravity rotációs egységtesztek.
- `internal/guard/budget_test.go` – napi keret őr egységtesztek direct és compact módban.
- `internal/e2e/runs_test.go` – végponttól végpontig tartó budget tesztek.
- `internal/httpserver/web/index.html` – fejlesztői webes felület limitekkel és budget kijelzéssel.
- `docs/01-terv.md`, `docs/02-muszaki-specifikacio.md`, `CLAUDE.md` – dokumentáció.

---

## 3. Megvalósított szabályok és ellenőrzésük

| Szabály | Megvalósítás | Teszt |
|---|---|---|
| `max_session_tokens` rotáció közvetlen módban | `runDirectGroup`: ha `lastTokens >= maxSessionTokens`, új generáció, új session ID, recap prepended, szál-értesítés | `TestDirectSessionRotationClaude`, `TestDirectSessionRotationAntigravity` |
| Antigravity conversation törlés rotációkor | `delete(state.Conversations, threadRoot)`, promptban nincs korábbi `--conversation` | `TestDirectSessionRotationAntigravity` |
| Napi tokenkeret számítás budapesti nap szerint | `runs.Service.AgentBudget`: `TokensSince` az aktuális `Europe/Budapest` nap kezdetétől, reset budapesti éjfélkor | `TestAgentBudgetEndpoint` |
| `GET /api/v1/agent/budget` hozzáférés-védelem | Csak `KindAgent` token hívhatja, ember 403 Forbidden | `TestAgentBudgetEndpoint` |
| Őr futásmegtagadás keret elfogyásakor | `checkBudget`: ha `limit > 0 && remaining <= 0`, futás nem indul, nincs ack, tételek inboxban maradnak | `TestGuardDailyTokenBudget_Direct`, `TestGuardDailyTokenBudget_Compact` |
| Napi egy szál-értesítés az őrtől kerethiánykor | `checkBudget`: naponta (`dayKey == resets_at.Format(...)`) legfeljebb egyszer posztol szálanként | `TestGuardDailyTokenBudget_Direct`, `TestGuardDailyTokenBudget_Compact` |
| Lekérdezési vihar elkerülése | `waitInterval()` a `resets_at` időpontig alszik, ha a keret elfogyott | `TestGuardDailyTokenBudget_Direct` |
| Limit emelésekor / másnap azonnali folytatás | Keret emelésekor vagy éjfél után a következő ticknél a munka automatikusan és hibátlanul folytatódik | `TestGuardDailyTokenBudget_Direct`, `TestGuardDailyTokenBudget_Compact` |
| Felügyelő élő konfiguráció-újratöltés | `Supervisor.reconcile`: a limit változása új `key`-t generál, leállítja a régi és elindítja az új agentet | `internal/guard/host.go` |

---

## 4. Lefuttatott minőségbiztosítási parancsok

1. **`make gen-check`**
   - OpenAPI és sqlc generált kódok ellenőrzése.
2. **`make test`**
   - Minden egység-, szerződés- és E2E teszt lefutott és sikeres (`PASS`).
3. **`make test-pg`**
   - Valamennyi PostgreSQL adattárteszt (storetest memory + Postgres) hibátlanul zöld (`PASS`).
4. **`make lint`**
   - `golangci-lint run ./...` -> `0 issues.`
5. **Race detector ellenőrzés**
   - `go test -race ./internal/guard ./internal/e2e` -> `PASS`, adatverseny mentes működés.
