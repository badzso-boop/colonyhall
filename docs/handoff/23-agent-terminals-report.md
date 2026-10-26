# Átadás-átvételi jelentés: 23. kör – Ágens terminálok (hosszú parancsok, ébresztés, élő olvasó felület)

**Dátum:** 2026-10-26  
**Ág:** `feature/agent-terminals` (bázis: `feature/file-browser`)  
**Munkakönyvtár (worktree):** `/mnt/g/Projects/new-chat-worktrees/agent-terminals`  

---

## 1. Összefoglaló és elért célok

A feladat célja a hosszú futásidejű parancsok (pl. 45 perces cross-WSL fordítások `@szakdoga-claude` agentnél) megbízható futtatása volt. A közvetlen futások korlátja (15 perces `work_timeout`) és a folyamatcsoport terminálása (`Setpgid` + `kill(-pgid)`) eddig megszakította a háttérfolyamatokat.

A megvalósított rendszer:
1. **Izolált háttérfolyamat-kezelés (tmux):** dedikált `tmux -L colonyhall` socket alatt futó munkamenetek (`colony-<handle>-<name>`), amelyek túlélik az őket indító futás lezárását.
2. **Stdio MCP szerver és eszközök:** beépített `colony terminal-mcp` alparancs, amely `colony-terminal` néven kerül a teljes hozzáférésű agentek közvetlen futási konfigurációjába (`term_start`, `term_status`, `term_output`, `term_send`, `term_kill`).
3. **Befejezési esemény és ébresztés:** a parancs lefutásakor az őr rövid szálüzenetet küld (`⚙️ … kész · exit 0 · 43:12`), és a `POST /api/v1/agent/terminal-events` végponton át új `terminal` típusú inbox elemet (`FlagWake`) hoz létre bekeretezett záró kimenettel. A következő közvetlen futás automatikusan folytatja a munkát.
4. **Emberi felület és közvetítő műveletek:** a 20. körös efemer közvetítőn (`POST /api/v1/agents/{agentId}/fs`) át elérhető `term_list`, `term_output` és `term_kill` (utóbbi adminisztrátori joghoz kötve, auditálva). A webes felületen az Agent Overview panelen dedikált „🖥 Terminálok” szekció, teljes magasságú monospace terminálnéző (`textContent`, szigorúan innerHTML nélkül), automatikus görgetés (felfelé görgetéskor szünetel), futás közben 2 másodpercenkénti frissítés, adminisztrátori Leállítás gomb megerősítéssel és másolható SSH csatlakozási parancs (`tmux -L colonyhall attach -r`).

---

## 2. Commitok listája

- `d14a4ca` – **Guard: tmux-backed terminal manager with output trimming, watcher, and halt handling**  
  Terminálmenedzser (`internal/guard/terminal.go`), lemezes állapot (`<state>/terminals/<handle>/<name>/`: `meta.json`, `output.log`, `exit`), névellenőrzés (`^[a-z0-9][a-z0-9-]{0,31}$`), határértékek (max 5 agentenként, max 20 gépenként), `CheckGrant` valós mappaellenőrzés, 5 mp watcher és 7 napos lejárati takarítás, leállítási szemantika (`killed_by: "halt"`), újrainduláskori örökbefogadás, vezérlőkarakter-szűrés (`FilterTerminalOutput`), 10 átfogó teszt.
- `06e323c` – **Agent: stdio MCP terminal tools, direct run injection, and wake on finish**  
  `colony terminal-mcp` CLI alparancs, Model Context Protocol (MCP) eszközök (`term_start`, `term_status`, `term_output` biztonsági kerettel, `term_send`, `term_kill`), közvetlen futási injektálás Claude Code és Antigravity kliensekhez, sandboxed agent tiltás, `00032_agent_terminals.sql` adatbázis-migráció, delivery modul kiegészítés (`ItemTerminal`, `FlagWake`, `POST /api/v1/agent/terminal-events`), store tesztek postgres és memóriás háttérrel.
- `50245f2` – **Web: terminal viewer in agent panel, relay ops, and SSH attach helper**  
  Efemer közvetítő kiegészítés (`term_list`, `term_output`, `term_kill` `!by.IsAdmin()` védelemmel és `audit: term kill` naplózással), OpenAPI és sqlc frissítés, `index.html` terminálmegtekintő panel, automatikus görgetés és 2s frissítés, admin leállítás in-page megerősítéssel, másolható SSH attach parancs a gép nevével, chatüzenet-linkek a befejezett terminálokhoz, e2e szerződés- és jogosultsági tesztek.
- *(Dokumentációs commit)* – **Docs: handoff report and documentation updates for task 23**  
  `docs/01-terv.md`, `docs/02-muszaki-specifikacio.md`, `docs/05-homelab-telepites.md`, `task.md`, `CLAUDE.md` és jelen átadási jelentés.

---

## 3. Nyitott kérdésekben hozott döntések

1. **Vezérlőkarakterek szűrése (`FilterTerminalOutput`):**  
   A terminálkimenet ANSI escape szekvenciákat és vezérlőkaraktereket tartalmazhat (színek, kurzormozgatás, képernyőtörlés). A prompt nyitva hagyta a színek vagy szűrés kérdését.  
   *Döntés:* Szigorú szűrés mellett döntöttünk: az ANSI escape-szekvenciák (`\x1b[...]`) és a C0/C1 vezérlőkarakterek törlésre kerülnek, kizárólag a soremelés (`\n`), kocsivissza (`\r`) és tabulátor (`\t`) marad meg. Ez garantálja, hogy se az LLM promptja ne sérüljön (prompt injection / elfedés), se a webes pre-blokk ne viselkedjen rendellenesen.
2. **Antigravity stdio MCP támogatás:**  
   Az Antigravity (`agy`) a `.agents/mcp_config.json` fájlból olvassa a munkaterület MCP kiszolgálóit. A közvetlen módban futó Claude Code (`--mcp-config`) közvetlenül megkapja a `colony-terminal` stdio parancsot. Az Antigravity konfigurációba is bekerül a `colony-terminal` bejegyzés a `colony terminal-mcp` indítóparancsával. Sandboxos agenteknél a kiszolgáló egyik kliensnél sem kerül konfigurálásra, és a bináris futásidőben is elutasítja a kérést (`sandboxed agents may not run terminals`).

---

## 4. Hatókörön kívül hagyott funkciók (Out of scope)

A specifikáció és Norbi döntései alapján kimaradt:
- **Interaktív webes terminál (xterm.js / WebSocket PTY):** a felület szándékosan csak olvasható; interaktív beavatkozásra Norbi a Tailscale-en keresztüli közvetlen SSH csatlakozást használja.
- **Terminálok sandboxos agenteknek:** a sandboxos ágensek szigorú elvként nem futtatnak tetszőleges parancsokat a gazdagépen.
- **1000 sornál vagy 7 napnál hosszabb megőrzés:** az őr memóriája és lemezterülete védett (1 MB kimeneti sapka, 1000 soros lekérés, 7 napos automatikus törlés).
- **Parancsütemezés és UI-ból indított terminál:** terminált kizárólag agent indíthat az MCP eszközökön keresztül; ember csak olvashatja vagy adminisztrátorként leállíthatja.

---

## 5. Ellenőrzési és tesztelési eredmények

### `make test`
```
go test ./...
ok  	colonyhall/cmd/colonyhall	0.172s
ok  	colonyhall/internal/e2e	14.081s
ok  	colonyhall/internal/guard	(cached)
ok  	colonyhall/internal/modules/delivery	(cached)
ok  	colonyhall/internal/modules/files	(cached)
...
PASS (minden csomag zöld)
```

### `make lint`
```
golangci-lint run ./...
0 issues.
```

### `make test-web`
```
node --test internal/httpserver/web/markdown.test.mjs
✔ headings 1 to 6
✔ inline formatting: bold, italic, strike, inline code
✔ word-boundary rule: identifiers and snake_case are NOT italic
✔ link security: only http and https allowed
✔ images from URLs become links if http(s)
✔ raw HTML tags stay text nodes
✔ file links: file:// and local paths become filelink nodes
✔ real agent answer with run_id=9 and file links
✔ fenced code blocks keep content verbatim
✔ tables (GitHub style)
✔ nested lists with task items
✔ mentions
✔ pathological 20,000-character line finishes fast (< 500ms)
✔ input over 20,000 characters is rendered as plain text
✔ ColonyMarkdown.plain strips markdown formatting cleanly
✔ file references: line numbers, ranges, agent handles, Windows paths
✔ non-references: http urls with port and times are not file references
ℹ tests 17, pass 17, fail 0
```

### `make gen-check`
```
make gen
go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
docker run --rm -u 1000:1000 -v "/mnt/g/Projects/new-chat-worktrees/agent-terminals":/src -w /src sqlc/sqlc:1.31.1 generate
git diff --exit-code -- internal/apispec internal/modules/.../sqlcgen
(Exit code 0, tisztán szinkronban)
```

### `GOFLAGS=-count=1 make test-pg` (Postgres port 5433)
```
COLONY_TEST_DATABASE_URL='postgres://colony:colony@localhost:5433/colony_test?sslmode=disable' go test ./internal/storetest/ -run TestPostgres -v
=== RUN   TestPostgres
--- PASS: TestPostgres (3.07s)
    --- PASS: TestPostgres/eventlog (0.15s)
    --- PASS: TestPostgres/identity (0.16s)
    --- PASS: TestPostgres/conversation (0.17s)
    --- PASS: TestPostgres/delivery (0.13s)
    --- PASS: TestPostgres/audit (0.13s)
    --- PASS: TestPostgres/hosts (0.14s)
    --- PASS: TestPostgres/runs (0.13s)
    --- PASS: TestPostgres/schedule (0.13s)
    --- PASS: TestPostgres/approval (0.12s)
    --- PASS: TestPostgres/wiki (0.16s)
    --- PASS: TestPostgres/question (0.16s)
    --- PASS: TestPostgres/files (0.17s)
    --- PASS: TestPostgres/git (0.14s)
    --- PASS: TestPostgres/preview (0.13s)
    --- PASS: TestPostgres/agentrules (0.12s)
    --- PASS: TestPostgres/notes (0.14s)
    --- PASS: TestPostgres/decisions (0.13s)
    --- PASS: TestPostgres/leases (0.18s)
    --- PASS: TestPostgres/sidebar (0.17s)
    --- PASS: TestPostgres/compaction (0.17s)
    --- PASS: TestPostgres/delegation (0.14s)
PASS
ok  	colonyhall/internal/storetest	3.128s
```

### `go test -race ./internal/guard/ ./internal/e2e/`
```
ok  	colonyhall/internal/guard	15.283s
ok  	colonyhall/internal/e2e	72.928s
(Nulla adatverseny, tiszta lefutás)
```

---

## 6. Kézi ellenőrzési forgatókönyv (Manual Check)

Az éles működés az alábbi lépésekkel ellenőrizhető:

1. **Parancs indítása agent által:**
   Közvetlen Claude Code agentnek küldjünk feladatot:
   `"Indíts el egy háttérterminált 'ros-build' néven a 'sleep 120; echo done' paranccsal, és várd meg a befejezést."`
   - Az agent meghívja a `term_start(name: "ros-build", command: "sleep 120; echo done")` eszközt.
   - Az eszköz leírása miatt az agent nem vár ciklusban, a futás véget ér.
2. **Folyamat túlélése:**
   - Ellenőrizzük a gépen: `tmux -L colonyhall ls` mutatja a `colony-<handle>-ros-build` munkamenetet, a háttérben fut a `sleep`.
3. **Webes olvasó felület:**
   - Nyissuk meg a jobb oldali Agent Overview panelt: a **"🖥 Terminálok"** szekcióban látható a `ros-build` `fut ⏳` jelvénnyel.
   - Kattintsunk rá: megnyílik a teljes magasságú terminálnéző, a fejlécben látható a "frissítés 2 mp-enként", az SSH dobozban a másolható parancs (`tmux -L colonyhall attach -r -t colony-...`).
   - Adminisztrátorként megjelenik a piros "Leállítás" gomb.
4. **Befejezés és ébresztés:**
   - A 120 mp leteltével az őr megfigyelője észleli a kilépést.
   - A chat szálba bekerül a rövid státuszüzenet: `⚙️ ros-build kész · exit 0 · 02:00`.
   - Az üzenet mellett megjelenik a `🖥 Terminál megtekintése` gomb, amelyre kattintva a nézőben látható a `done` kimenet, a jelvény `kész ✅ exit 0`-ra vált, a 2 mp-es frissítés leáll.
   - A szerver létrehozza az ébresztő inbox elemet, az agent automatikusan felébred, megkapja a promptban a bekeretezett záró kimenetet, és válaszol a feladat folytatásáról.
5. **SSH csatlakozási ellenőrzés:**
   - Terminálból futtassuk a felület által kínált parancsot:
     `tmux -L colonyhall attach -r -t colony-<handle>-ros-build`
   - Csak olvasható módban rácsatlakozik, `Ctrl+B`, majd `D` gombokkal lecsatlakozhatunk.

---

## 7. Mit nézzen meg a bíráló (reviewer) először?

1. **`internal/guard/terminal.go`**:
   - `TerminalManager`: a különálló `tmux -L colonyhall` socket kezelése, a lemezes könyvtárszerkezet (`terminalsDir`, `meta.json`, `output.log`, `exit`).
   - `Start`: `trap 'echo $? > exit' EXIT; (...)` parancscsomagolás a pontos kilépési kód garantálására; a gépenkénti (20) és agentenkénti (5) korlátok és a `CheckGrant` valós mappaellenőrzés.
   - `WatchTick` és `AdoptExisting`: 5 mp-es életciklus-kezelés, örökbefogadás újrainduláskor és 7 napos takarítás.
   - `FilterTerminalOutput`: ANSI és C0/C1 szűrés.
2. **`internal/guard/terminal_mcp.go`**:
   - Az stdio MCP kiszolgáló és eszközei (`term_start`, `term_status`, `term_output`, `term_send`, `term_kill`), kimeneti keretezés (`<terminal_output>`), a belső lezáró címkék maszkolása.
3. **`internal/modules/files/service.go` & `fsrelay.go`**:
   - `RequestAgentFs`: a közvetítő bővítése a terminálműveletekre, az adminisztrátori ellenőrzés (`!by.IsAdmin()`), `audit: term kill` naplózás.
4. **`internal/httpserver/web/index.html`**:
   - `#terminalViewerPanel`: a teljes magasságú / mobilon teljes képernyős néző, auto-scroll szüneteltetéssel, `textContent` kizárólagos használata, admin Leállítás megerősítéssel és SSH csatlakozási segéd.
5. **`internal/e2e/terminal_relay_test.go`**:
   - A teljes közvetítői folyamat és jogosultsági határok e2e tesztjei.
