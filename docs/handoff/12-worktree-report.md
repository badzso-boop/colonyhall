# Handoff Report: Git worktree agentenként (A git worktree per full-access agent)

Dátum: 2026-10-16  
Ág: `feature/worktree`  
Szerző: Antigravity CLI  
Felülvizsgáló: Claude Code  

---

## 1. Változtatások commitonként

- `b2618fc` **Add worktree_repo assignment field and migration**
  - OpenAPI specifikáció: `api/openapi.yaml` bővítése a `worktree_repo` mezővel az `AssignRequest` és `HostAgent` sémákban, valamint `Worktree` struktúra (`agent_id`, `path`, `branch`) és `worktrees` tömb hozzáadása a `HostReport` sémához.
  - Adatbázis migráció: `migrations/00012_worktree.sql` hozzáadja a `worktree_repo text not null default ''` oszlopot az `agent_host` táblához.
  - SQL lekérdezések: `internal/modules/hosts/postgres/queries.sql` frissítve a `worktree_repo` mentésére és lekérdezésére.
  - Kódgenerálás: `make gen` legenerálta a típusos `apispec` és `sqlcgen` modelleket.

- `3746cb6` **Support worktree_repo in hosts module and store adapters**
  - Domain modell (`internal/modules/hosts/model.go`): `WorktreeRepo` mező hozzáadása a `Assignment` és `AssignInput` struktúrákhoz; `Worktree` modell és `Worktrees` mező a `Report` struktúrához; `ErrWorktreeDirectOnly` hiba definiálása.
  - Eseménynapló (`internal/modules/hosts/events.go`): `WorktreeRepo` rögzítése az `agent.assigned` audit eseményben.
  - Szolgáltatási réteg (`internal/modules/hosts/service.go`):
    - Ellenőrzi, hogy `worktree_repo` csak közvetlen (direct) módban adható meg (`sandbox: false`), sandbox esetén `ErrWorktreeDirectOnly` hibát ad.
    - Ha a `worktree_repo` meg van adva, de a `root_dir` üres, a `root_dir` automatikusan a `worktree_repo`-ra inicializálódik a meglévő közvetlen módú validáció kielégítésére.
    - Útvonal validáció a `ValidatePath` segédfüggvénnyel (nem lehet üres, relatív vagy control karakteres).
  - Adattár adapterek:
    - `internal/modules/hosts/postgres/store.go`: `worktree_repo` mentése és visszaolvasása az adatbázisból.
    - `internal/modules/hosts/memory/store.go`: memóriabeli hozzárendelés tárolja a mezőt.
    - `internal/modules/hosts/httpapi/handlers.go`: REST kérésből beolvasás és JSON válaszban visszaadás.
    - `internal/storetest/storetest.go`: szerződéstesztek kiegészítve a `worktree_repo` és `Report.Worktrees` perzisztálásának ellenőrzésével.

- `5918b6d` **Implement git worktree management and direct prompt in guard**
  - Új modul: `internal/guard/worktree.go`:
    - `EnsureWorktree(ctx, repoPath, handle)`: megkeresi a repó alapértelmezett ágát (`refs/remotes/origin/HEAD` vagy `init.defaultBranch` vagy fallback `main`), ellenőrzi a `colony/<handle>/work` ág meglétét, és létrehozza a `<repo>-worktrees/<handle>` könyvtárat a `git worktree add` paranccsal.
    - Ha a könyvtár már létezik és worktree, ellenőrzi és újrahasználja.
    - `IsGitRepo(ctx, repoPath)`: `git -C <repo> rev-parse --git-dir` hívással validálja a repót.
    - `gitExec`: minden git hívást `-c core.hooksPath=/dev/null` argumentummal és `GIT_CONFIG_NOSYSTEM=1` környezeti változóval futtat slice argumentumlistával (nincs shell), időkorláttal, így megelőzi a kártékony git hookok vagy konfigurációk lefutását.
  - Supervisor integráció (`internal/guard/host.go`):
    - `reconcile`: ellenőrzi a `worktree_repo`-t a gép engedélyezett gyökérkönyvtárain belül (`CheckGrant`), és ellenőrzi, hogy létező git repó-e.
    - Ha nem git repó vagy kívül esik a gyökereken, vagy a worktree létrehozása sikertelen, az agent nem indul el, és a hiba oka a visszajelentett `rejected` listába kerül.
    - Ha sikeres, a futás gyökérkönyvtára (`ha.RootDir`) átirányításra kerül a létrehozott worktree útvonalára (Claude `cwd`, Antigravity `--add-dir`).
    - `report`: az aktív worktree-k (`agent_id`, `path`, `branch`) összegyűjtése és visszaküldése a szervernek.
  - Direkt prompt frissítés (`internal/guard/direct.go`):
    - Ha az agent worktree-ben dolgozik, a prompt a következő utasítással egészül ki:
      `You work in your own worktree on branch colony/<handle>/work; commit there; never commit to the main branch; deliver through a PR when asked.`
  - Átfogó tesztek valódi git futtatásával ideiglenes könyvtárakban (`internal/guard/worktree_test.go`):
    - Worktree létrehozása és újrahasználata.
    - Megfelelő ágnév (`colony/<handle>/work`).
    - A fő munkakönyvtár érintetlen marad (main fa tiszta).
    - Hook-védelem igazolása: kártékony `post-checkout` hook sem fut le worktree létrehozásakor.
    - Nem-git könyvtár és engedélyezett gyökereken kívüli repó elutasítása.

- `17563ae` **Add worktree repo input and report display to dev page**
  - Fejlesztői felület (`internal/httpserver/web/index.html`):
    - Hozzárendelési űrlapon új mező: `Worktree repó (csak direct módban)`.
    - Agent kártyákon megjelenik a beállított worktree repó.
    - Gép kártyán és agent kártyán megjelennek a gazdagép által jelentett aktív worktree-k (`elérési út [ág]`).

- `23c14db` **Add end-to-end tests for worktree assignments and host reporting**
  - E2E tesztkészlet (`internal/e2e/hosts_test.go`):
    - `TestWorktreeAssignmentAndReport`:
      - Sandbox módban a `worktree_repo` 400 Bad Request-et ad (`ErrWorktreeDirectOnly`).
      - Közvetlen módban sikeresen hozzárendelhető, a `root_dir` automatikusan kitöltődik, ha nincs megadva.
      - A gép felügyelője jelentésben visszaküldi az aktív worktree-ket, és a szerver megfelelően továbbítja a lekérdezésekben.
  - Hibakezelés (`internal/modules/hosts/httpapi/handlers.go`): `ErrWorktreeDirectOnly` regisztrálása a validációs hibák közé (`httpx.RegisterValidation`).

---

## 2. Érintett fájlok jegyzéke

- `migrations/00012_worktree.sql` – goose adatbázis migráció (`worktree_repo` oszlop).
- `api/openapi.yaml` – `worktree_repo` és `worktrees` sémák és végpont definíciók.
- `internal/apispec/apispec.gen.go` – generált OpenAPI típusok.
- `internal/modules/hosts/model.go` – domain modellek és hibák.
- `internal/modules/hosts/events.go` – audit események.
- `internal/modules/hosts/service.go` – közvetlen módú ellenőrzés, fallback `root_dir` és validáció.
- `internal/modules/hosts/postgres/queries.sql` – SQL lekérdezések.
- `internal/modules/hosts/postgres/store.go` – Postgres perzisztencia.
- `internal/modules/hosts/memory/store.go` – memóriabeli perzisztencia.
- `internal/modules/hosts/httpapi/handlers.go` – REST kezelők és hibaregisztráció.
- `internal/storetest/storetest.go` – adattár szerződéstesztek.
- `internal/guard/client.go` – gépkliens típusok (`WorktreeRepo`, `Worktree`).
- `internal/guard/guard.go` – `Agent` struktúra worktree adatokkal.
- `internal/guard/worktree.go` – biztonságos worktree kezelés és git hívások.
- `internal/guard/worktree_test.go` – git worktree integrációs tesztek.
- `internal/guard/direct.go` – worktree instrukció a közvetlen promptban.
- `internal/guard/direct_test.go` – prompt generálás egységtesztje.
- `internal/guard/host.go` – supervisor reconcile és jelentéskészítés.
- `internal/httpserver/web/index.html` – Gépek panel felületi elemek.
- `internal/e2e/hosts_test.go` – végponttól végpontig tartó integrációs teszt.
- `CLAUDE.md`, `docs/01-terv.md`, `docs/02-muszaki-specifikacio.md` – dokumentáció frissítések.

---

## 3. Főbb tervezési döntések és biztonsági garanciák

| Követelmény | Megoldás | Garancia / Védelem |
|---|---|---|
| Izolált munkaterület | `<repo>-worktrees/<handle>` | Nem a fő munkakönyvtárban dolgozik, a fő repó munkaterülete tiszta marad. |
| Dedikált ág | `colony/<handle>/work` | Nem commitol a `main` ágra, a változtatásait PR-ban szállítja le. |
| Kártékony git hookok kizárása | `-c core.hooksPath=/dev/null` és `GIT_CONFIG_NOSYSTEM=1` | A klónozott vagy meglévő repó `.git/hooks/` parancsfájljai nem futhatnak le az őr által kezdeményezett git műveletek közben. |
| Parancssori injekció megelőzése | `exec.CommandContext` slice argumentumokkal | Nincs shell (`sh -c`), minden argumentum közvetlenül és biztonságosan adódik át. |
| Jogosultsági határok betartása | `CheckGrant(repoPath)` | Csak a gép számára admin által engedélyezett gyökérkönyvtárakon belüli repók használhatók. |
| Érvénytelen konfiguráció kezelése | Supervisor hibaellenőrzés | Ha nem git repó vagy sikertelen a worktree nyitása, az agent nem indul el, a hiba bekerül a `rejected` okok közé. |
| Mód korlátozás | `ErrWorktreeDirectOnly` | Worktree funkció jelenleg csak közvetlen (full-access) módban engedélyezett, sandboxos konfigurációban 400 Bad Request. |

---

## 4. Lefuttatott ellenőrzések és eredmények

1. **`make test`**
   - Eredmény: `PASS`
   - Valamennyi belső modul egység- és integrációs tesztje lefutott hiba nélkül.
2. **`make lint`**
   - Eredmény: `golangci-lint run ./...` -> `0 issues.`
   - A `depguard` rétegzési szabályok és formázási követelmények maradéktalanul teljesülnek.
3. **`make test-pg`**
   - Eredmény: `COLONY_TEST_DATABASE_URL=... go test ./internal/storetest/ -run TestPostgres -v` -> `PASS`
   - A frissített `00012_worktree.sql` migráció lefutott, a Postgres és in-memory tároló tesztek hibátlanul zöldek.
4. **`make gen-check`**
   - Eredmény: `apispec` és minden `sqlcgen` csomag konzisztens az adatbázis sémával és OpenAPI leírással.
5. **Versenyhelyzet-ellenőrzés (`-race`)**
   - Parancs: `go test -count=1 -race ./internal/guard ./internal/e2e`
   - Eredmény: `PASS`, 0 adatverseny és 0 hiba.

---

## 5. Mit nézzen meg a felülvizsgáló először

1. `internal/guard/worktree.go` – a git worktree életciklus és a hook-mentes biztonságos parancsfuttatás megvalósítása.
2. `internal/guard/worktree_test.go` – valódi git környezetben futtatott tesztek, különösen a `TestEnsureWorktree_IgnoresMaliciousHooks` és `TestEnsureWorktree_BranchAndIsolation`.
3. `internal/guard/host.go` – a felügyelő `reconcile` és `startAgent` folyamata, ahol a gyökérkönyvtár a worktree mappára cserélődik.
4. `internal/modules/hosts/service.go` – a `worktree_repo` direct módhoz kötése és az alapértelmezett `RootDir` kezelése.
5. `internal/e2e/hosts_test.go` – `TestWorktreeAssignmentAndReport` end-to-end érvényesítése.
