# Handoff Report: Közvetlen mód (Direct Mode)

Dátum: 2026-10-15  
Ág: `feature/direct-mode`  
Szerző: Antigravity CLI  
Felülvizsgáló: Claude Code

---

## 1. Változtatások commitonként

- `92a47d3` **Sandbox: pass secrets via environment instead of bwrap arguments**
  A `bwrap` parancssori argumentumaiból kikerültek a `--setenv` kulcs-érték párok és a `--clearenv`. A `Wrapper` és a `Runner` mostantól előre szűrt környezeti változókat (`env []string`) kap és ad át közvetlenül a `cmd.Env`-en keresztül. Így a titkok (pl. API kulcsok, Claude OAuth token) nem látszódnak a processzlistában (`argv`). Frissítve a `TestSandboxArgs` és a `TestSandboxForReal` teszt.

- `2cf6b61` **Hosts: add mode, root_dir, and sandbox to agent assignment**
  Szerveroldali közvetlen mód támogatás a `hosts` modulban: új goose migráció (`00006_direct_mode.sql`), amely hozzáadja a `mode` (DEFAULT 'compact'), `root_dir` (DEFAULT ''), és `sandbox` (DEFAULT true) oszlopokat az `agent_host` táblához. OpenAPI sémafrissítés (`AssignRequest` és `HostAgent`), `make gen` kódgenerálás (`apispec` és `sqlc`), validáció (`mode` direct vagy compact; direct módban a `root_dir` kötelező és érvényes abszolút útvonal), API alapértelmezések, valamint a memória és postgres tárolótesztek és e2e szerződéstesztek frissítése.

- `d32eb53` **Guard: implement direct mode with per-thread sessions and dev page controls**
  Az őr és a felület közvetlen módjának megvalósítása:
  - Szálankénti csoportosítás (`GroupByThread`) és feldolgozási ciklus az őrben (`handleDirect`, `runDirectGroup`).
  - Determinisztikus UUIDv5 munkamenet-azonosító származtatás (`DirectSessionID`) az agent ID-ből és a szál gyökérazonosítójából.
  - Helyi, atomi írású (0600) állapotfájl (`direct_state.json`) agentenként a Claude munkamenetek és az Antigravity szál → `conversation_id` kapcsolatok követésére.
  - Új `Direct` metódus az `Adapter` interfészen, valamint a `ClaudeCode` és `Antigravity` implementációkon (`--session-id`/`--resume`, `--permission-mode`, `--mcp-config`, illetve `--conversation`).
  - Ideiglenes, 0600 jogosultságú Colonyhall MCP konfiguráció létrehozása Claude Code-hoz a `CurrentToken` tokenforrásból, futás után azonnali törléssel, a token naplózása nélkül.
  - A Supervisorban a `root_dir` ellenőrzése a gép gyökereihez (`CheckGrant`); hibás útvonal esetén elutasítás a `Rejected` listában; `sandbox: false` esetén a `bwrap` kihagyása döntési és munkafutásoknál is.
  - Fejlesztői felület („Gépek” panel): mód (`direct`/`compact`), gyökérmappa, sandbox checkbox, és permission mode kiválasztása hozzárendeléskor, valamint ezek megjelenítése minden hozzárendelt agentnél.
  - Átfogó egységtesztek (`internal/guard/direct_test.go`) és e2e teszt (`internal/e2e/supervisor_direct_test.go`).

- `0f32ae9` **Docs: mark direct mode as implemented and update schedule**
  A `docs/02-muszaki-specifikacio.md` és a `CLAUDE.md` ütemezésének frissítése a közvetlen mód és a titkok sandbox argv-ből való kivezetésének megvalósításával.

---

## 2. Hozott döntések (ahol a specifikáció nyitott volt)

1. **Antigravity MCP konfiguráció:** A feladatkiírásnak megfelelően az Antigravity CLI nem kapott automatikusan generált MCP konfigurációs fájlt (szemben a Claude Code-dal). Az `agy` a saját helyi vagy globális konfigurációját használja; az őr csupán a `--conversation <id>` munkamenet-folytonosságot kezeli.
2. **UUIDv5 generálás robusztussága:** A `DirectSessionID` az agent UUID-jét használja névtérként és a szál gyökérazonosítóját (vagy DM esetén a csatornaazonosítót) névként. Nem-UUID agent azonosító esetén (például egyszerűsített mock tesztekben) először determinisztikusan érvényes UUID névteret képez az azonosítóból, megelőzve az esetleges pánikot vagy hibát.
3. **Állapotfájl perzisztencia:** A `direct_state.json` az agent saját mappájában (`sandbox_dir/<handle>/direct_state.json`) tárolódik. A mentés átmeneti fájlba történik `0600` móddal, majd `os.Rename`-mel atomi módon lép életbe, így váratlan leállás vagy vészleállító esetén sem sérülhet az állapot.
4. **Szálcsoportosítás sorrendtartása:** A `GroupByThread` megőrzi a szálak első előfordulási sorrendjét az inboxban, valamint a szálon belüli üzenetek időrendi sorrendjét is.
5. **Alapértelmezések a szerveroldali hozzárendelésnél (`Assign`):** Ha a `mode` mező hiányzik, az API alapértelmezése `direct`, `sandbox=false`, Claude-nál `permission_mode=auto`. Ha azonban a hívó expliciten `mode=compact` módot ad meg és a `sandbox` mezőt elhagyja, a `sandbox` értéke `true` lesz, biztosítva a meglévő sandboxos működés kompatibilitását.
6. **Supervisor útvonal-ellenőrzés:** Direct módban kötelező a `root_dir`, és a Supervisor a gép `roots` listájához ellenőrzi `CheckGrant`-tel. Ha nem felel meg (pl. a gép gyökérmappáin kívül esik), az őr azonnal elutasítja az indítást és felveszi a szervernek jelentett `Rejected` listába.

---

## 3. Ami kimaradt vagy szándékosan nem készült el

- **Élő CLI futtatások:** A szabályoknak megfelelően sem a valódi `claude`, sem a valódi `agy` bináris nem került futtatásra a tesztek során; kizárólag szimulált runner és szkriptelt adapterek vizsgálták a parancssor és viselkedés helyességét.
- **Chatből jóváhagyás (`--permission-prompt-tool`):** A specifikáció és az ütemezés szerint ez a későbbi feladatok része.
- **Közös titok-átadó mappa (`colonyhall-exchange`):** Szintén a következő ütemezett lépésben valósul meg.

---

## 4. Lefuttatott parancsok és eredményeik

Minden parancs a repó gyökeréből, az előírt környezeti változókkal futott le:
`export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin`

1. **`make test`**
   - Eredmény: `PASS`
   - Minden belső csomag tesztjei (e2e, guard, identity, conversation, delivery, policy, storetest) sikeresen lefutottak hibák nélkül.
2. **`make lint`**
   - Eredmény: `golangci-lint run ./...` -> `0 issues.`
3. **`make test-pg`**
   - Eredmény: `COLONY_TEST_DATABASE_URL='postgres://colony:colony@localhost:5433/colony_test?sslmode=disable' go test ./internal/storetest/ -run TestPostgres -v` -> `PASS`
   - Valamennyi PostgreSQL tárolóteszt lefutott és zölden zárt.
4. **`make gen-check`**
   - Eredmény: `git diff --exit-code -- internal/apispec $(find internal -type d -name sqlcgen)` -> 0 eltérés, a generált kód naprakész.

---

## 5. Kockázatok és átnézési javaslatok a felülvizsgálónak

1. **Ideiglenes MCP-fájl életciklusa:** Érdemes ellenőrizni az `internal/guard/direct.go` fájlban a `writeMCPConfig` hívást és a `cleanup()` defer futását, biztosítva, hogy szálankénti futás után az átmeneti tokenfájl azonnal törlődik.
2. **Munkamenet-folytatás logikája:** A `ClaudeCode.Direct` argumentum-összeállításában az első futás `--session-id <uuid>`, a további futások `--resume <uuid>` kapcsolót kapnak; ennek állapotkövetése a `direct_state.json`-ban ellenőrizendő.
3. **Supervisor bwrap elágazás:** A `Supervisor.reconcile`, `startAgent` és `policy` függvényekben a `ha.Sandbox` logikája: direct módban (`sandbox=false`) sem a döntési, sem a munkafutások nem kerülnek bwrap alá, de a `root_dir` ellenőrzése szigorúan érvényesül.
