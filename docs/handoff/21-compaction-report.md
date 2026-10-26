# Handoff Report 21: Automatikus tömörítés (Compaction) szálakban, DM-ekben és csatornákban

**Ág:** `feature/compaction`  
**Dátum:** 2026-10-26  
**Munka könyvtár / worktree:** `/mnt/g/Projects/new-chat-worktrees/compaction`

---

## 1. Összefoglaló és cél

A Colonyhall beszélgetéseiben (szálak, agent DM-ek, csatornák) a hosszas együttműködés során a tokenfelhasználás könnyen meghaladhatja a modellek hatékony kontextusablakát, ami lassuláshoz, felejtéshez és drága tokenköltségekhez vezet.

A 21. feladat célja a beszélgetések **folyamatos, automatikus és felügyelt tömörítésének** megvalósítása:
- **Tömörítési egységek (units):** Minden szál (`thread_root_id`), közvetlen beszélgetés fő folyama (`thread_root_id IS NULL`), valamint csatorna fő folyama független egységként követi a saját üzenetszámát és becsült tokenszámát (`len(body)/4`).
- **Küszöbérték (threshold):** Alapértelmezett 350 000 token a munkatér szintjén (konfigurálható 20 000 – 2 000 000 között), amely csatornánként felülbírálható.
- **Jegyző (scribe) agent és izolált futás:** A küszöb átlépésekor a rendszer felébreszti a kijelölt olcsó jegyző agentet egy `compact` feladattal. Az őr eszközök nélküli (`--tools ""`), szigorúan izolált összefoglalást futtat (`Summarize`).
- **Kivonat és rendszerüzenet:** A lezárult tömörítés eredménye csatornafájlként mentődik el, és a chatbe egy `🗜` kezdetű rendszerüzenet kerül ki a csatolt `.md` fájllal.
- **Emberi felügyelet:** Emberi felhasználók a felületen megnézhetik a formázott kivonatot, inline szerkeszthetik (`PUT /api/v1/compactions/{id}/summary`), vagy újragenerálhatják (`POST /api/v1/compactions/{id}/regenerate`).
- **Session priming és rotálás:** A közvetlen módban dolgozó agentek a `compaction_id` változásakor új munkamenetet indítanak, amelynek első promptját az őr az összefoglalóból (`<colonyhall_summary>`) és a tömörítés utáni üzenetekből építi fel. Ezzel párhuzamosan az agentek `max_session_tokens` beállítása 0-ra állítható.

---

## 2. Megvalósított részek és commitok

1. **Part 1: Szerveroldali beállítások, egységszámlálók, rekordok és végpontok** (`bd686d4`)
   - **Migráció (`migrations/00028_compaction.sql`):**
     - `workspace_compaction_settings`: globális alapértelmezett küszöb (`default_threshold_tokens`) és a kijelölt jegyző (`scribe_agent_id`).
     - `compaction_unit`: egységenkénti állapot és számlálók (`channel_id`, `thread_root_id`, `approx_tokens_since`, `messages_since`, `last_compaction_id`).
     - `compaction`: auditált append-only tömörítési rekordok (`id`, `channel_id`, `thread_root_id`, `up_to_message_id`, `previous_compaction_id`, `trigger`, `requested_by`, `compactor_agent_id`, `state`, `summary_file_id`, `system_message_id`, `stats`, `error_code`, timestamps).
     - `channel.compact_threshold_tokens`: csatornánkénti küszöb felülbírálási lehetőség.
   - **OpenAPI szerződés (`api/openapi.yaml`):**
     - Végpontok: `GET/PUT /api/v1/settings/compaction`, `GET/PATCH /api/v1/channels/{id}/compaction`, `GET/POST /api/v1/channels/{id}/compactions`, `PUT /api/v1/compactions/{id}/summary`, `POST /api/v1/compactions/{id}/regenerate`, `GET /api/v1/agent/compactions/{id}`, `POST /api/v1/agent/compactions/{id}/result`, `GET /api/v1/agent/context`.
     - Új típusok: `WorkspaceCompactionSettings`, `CompactionUnitStatus`, `Compaction`, `AgentContext`, `ContextMessage`, valamint `author_kind: "system"` és `compaction_id` a `Message` sémában.
   - **Belső modulok (`internal/modules/compaction/`):**
     - `Service`: eseménykezelő (`onMessageCreated`, token-akkumuláció és auto-trigger), kézi indítás, 15 perces időtúllépés (`timeout`), jegyző feladat lekérés és eredmény beküldés, összefoglaló szerkesztés, újragenerálás, agent kontextus kiszolgálás (`messages_after` plafonozva 200-nál).
     - Memória és Postgres perzisztens tárolók (`memory/`, `postgres/`, sqlc generálással).
     - Események: `compaction.created`, `compaction.done`, `compaction.failed`.
   - **Kézbesítés (`internal/modules/delivery/`):**
     - Új inbox elem típus: `ItemKindCompact` (`compact`), amely a kijelölt jegyzőhöz kerül kézbesítésre `FlagUnread | FlagWake` jelzőkkel.
   - **Szerver összedrótozás és esemény-draining:**
     - `internal/server/server.go`: tárolók, HTTP route-ok, eseményfogyasztók bekötése. Az `a.Drain(ctx)` kibővítve egy ciklussal (max 10 passz), amely megvárja a kaszkádolt események (üzenet -> compaction -> delivery) lecsendesedését.
   - **Tesztek:** Memória és Postgres szerződéstesztek (`internal/storetest/`), átfogó end-to-end tesztcsomag (`internal/e2e/compaction_test.go`).

2. **Part 2: Őr (guard): jegyző futás és agent munkamenet priming az összefoglalóból** (`79c8434`)
   - **Adapter felület és implementációk (`internal/guard/adapter.go`):**
     - `Summarize(ctx, SummarizeRequest) (RunResult, error)` metódus.
     - `ClaudeCodeAdapter`: eszközök nélküli futás (`--tools ""`, `--strict-mcp-config`, `--setting-sources ""`, `--no-session-persistence`, `--output-format json`).
     - `AntigravityAdapter`: izolált sandboxed futás eszközök nélkül, letiltott slash parancsokkal.
   - **Jegyző prompt (`internal/guard/compaction.go`):**
     - `ScribePrompt(task)`: Angol nyelvű rendszerutasítás a résztvevők felsorolásával, korábbi kivonat beágyazásával (`<colonyhall_summary>`), üzenetek framingjével (`<colonyhall_message author="..." author_kind="..." time="..." files="...">`), kötelező fejezetcímekkel (`Összefoglaló`, `Döntések`, `Állapot`, `Nyitott kérdések és teendők`, `Fontos részletek`). Szigorú prompt injection védelem: az üzenetek adatként mennek, sosem utasításként.
   - **Őr főhurok (`internal/guard/guard.go`):**
     - `RunOnce` lekezeli a `compact` típusú inbox elemeket (`handleCompact`). Lefuttatja a `Summarize`-t, beküldi az eredményt a szerverre, nyugtázza az inbox elemet, és soha nem válaszol a chatbe.
   - **Session rotálás és priming (`internal/guard/direct.go`):**
     - `DirectState.Compactions` szótárban tárolja a `compaction_id`-t egységenként.
     - Minden futás előtt lekérdezi a `GET /api/v1/agent/context` végpontot. Ha a `compaction_id` megváltozott, rotálja a munkamenetet (`Generations++`, Claude/agy session törlése) anélkül, hogy a chatbe kiírná a korábbi "Started a fresh session" üzenetet.
     - A friss munkamenet nyitó promptját a `<colonyhall_summary>` és a tömörítés utáni `<m ctx from="...">` üzenetekből állítja össze.
   - **Tesztek (`internal/guard/compaction_test.go`):**
     - Prompt és framing tesztek, Claude és Antigravity argumentumtesztek, mock-szerveres jegyző futás, és direct session rotálási/priming tesztek.

3. **Part 3: Fejlesztői felület (Web UI)** (`2f88411`)
   - **Fejléc token számláló és kézi indítás:**
     - Csatorna és szál megnyitásakor a fejlécben megjelenik a dinamikus számláló: `~120 000 / 350 000 token`.
     - "🗜 Tömörítés most" gomb az írási joggal rendelkező felhasználóknak (folyamatban lévő tömörítés esetén letiltva "🗜 Tömörítés folyamatban…" felirattal).
   - **🗜 Rendszerüzenet és `.md` chip:**
     - A rendszerüzenet csatolmányként kapott `.md` kivonata inline chipként jelenik meg, amelyre kattintva a formázott Markdown megtekintő nyílik meg (`openFileModal`).
     - Emberi felhasználóknak megjelenik az inline "✏️ Szerkesztés" gomb (textarea-t nyit mentés és mégse gombokkal -> `PUT .../summary`) és az "🔄 Újragenerálás" gomb (`POST .../regenerate`).
   - **Beállítások:**
     - Gépek (Machines) panel: Tömörítési beállítások kártya adminoknak (globális küszöb input a ~4 karakter = 1 token magyarázattal, és jegyző agent választó legördülő menü a hozzárendeléssel rendelkező agentekből).
     - Csatorna tagok modális ablak: csatornánkénti opcionális küszöbérték felülbírálás (`PATCH .../compaction`).
   - **Többnyelvűség és mobil nézet:**
     - Teljeskörű `hu` és `en` fordítások (`I18N`).
     - 390px mobil képernyőméret támogatása tördelhető fejléccel.

---

## 3. Döntések a promptban nyitva hagyott kérdésekben

1. **Rendszerüzenetek kizárása a tokenszámlálásból:**
   - A `CompleteCompaction` lefutásakor generált "🗜 Automatikus tömörítés..." rendszerüzenetet az `onMessageCreated` kifejezetten kihagyja a tokenszámlálóból (`p.Kind == "system"`). Ezzel elkerülhető, hogy a tömörítés saját értesítője azonnal piszkos állapotba vigye a számlálót.
2. **Generált rendszerüzenet szűrése az Agent Contextben:**
   - A `GetAgentContext` hívásakor az összefoglaló utáni üzenetek (`MessagesAfter`) közül kiszűrésre kerül maga a tömörítési rendszerüzenet (`m.ID == latest.SystemMessageID`). Így az agentek új munkamenetének promptjába nem kerül bele a felesleges meta-értesítés.
3. **Kaszkádolt események ürítése a tesztekben és a szerverben:**
   - A `Drain(ctx)` metódus legfeljebb 10 menetet hajt végre, amíg van olyan eseményfogyasztó, amely új eseményt dolgozott fel. Erre azért van szükség, mert a `compaction` modul `compaction.created` eseményéből a `delivery` modul készít inbox bejegyzést.
4. **DirectState session rotálás első vs. ismételt futásnál:**
   - Amikor egy szálban először fut egy agent, a helyi `DirectState.Compactions[threadRoot]` még üres. Ekkor elmenti a szervertől kapott kezdeti `compaction_id`-t generáció-növelés nélkül. Kizárólag akkor lépteti a generációt (`gen++`) és törli a CLI munkamenetet, ha az azonosító egy korábbi ismert azonosítóhoz képest megváltozott. A rotálás után elnyomja a régi "🔄 Started a fresh session" chat-üzenetet, mert a `🗜` rendszerüzenet már egyértelműen jelezte a tömörítést.
5. **Időtúllépés (`timeout`):**
   - Ha egy függőben lévő tömörítés 15 percnél régebbi és a jegyző nem küldött eredményt, a szerver automatikusan meghiúsultnak tekinti (`StateFailed`, `error_code: "timeout"`), így a csatorna nem ragad be pending állapotban.

---

## 4. Ami kimaradt (Out of scope)

A prompt utasításainak megfelelően az alábbi területek szándékosan kimaradtak:
- **Időalapú tömörítés:** Kizárólag a tokenmennyiség elérése vagy kézi emberi gombnyomás indít tömörítést; időalapú automatikus ütemezés nincs.
- **Beszélgetési előzmények törlése:** A korábbi üzenetek nem törlődnek az adatbázisból; az archívum megmarad, és az agentek a Colonyhall MCP `read` eszközeivel továbbra is vissza tudnak olvasni.
- **Csatolt fájlok tartalmának tömörítése:** A jegyző bemenetében kizárólag a csatolt fájlok nevei és típusai szerepelnek, a fájlok teljes szövege nem kerül beágyazásra.
- **Agentek általi kivonat-szerkesztés:** A kivonatot kizárólag emberi felhasználók szerkeszthetik vagy generálhatják újra a felületről.
- **Szerveroldali AI kulcs:** A szerver nem használ saját közvetlen LLM kulcsot; a tömörítés mindig a regisztrált jegyző agenten és az őrön fut keresztül.

---

## 5. Tesztek kimenete

### `make test`
```
go test ./...
ok  	colonyhall/cmd/colonyhall	0.149s
ok  	colonyhall/internal/e2e	10.828s
ok  	colonyhall/internal/guard	7.274s
ok  	colonyhall/internal/httpserver	0.052s
ok  	colonyhall/internal/modules/agentrules	0.048s
ok  	colonyhall/internal/modules/conversation	0.061s
ok  	colonyhall/internal/modules/delivery/mcptools	0.042s
ok  	colonyhall/internal/modules/files	0.055s
ok  	colonyhall/internal/modules/files/mcptools	0.041s
ok  	colonyhall/internal/modules/files/storage	0.046s
ok  	colonyhall/internal/modules/git	0.058s
ok  	colonyhall/internal/modules/git/provider	0.051s
ok  	colonyhall/internal/modules/identity	0.049s
ok  	colonyhall/internal/policy	0.043s
ok  	colonyhall/internal/storetest	0.812s
PASS
```

### `make lint`
```
golangci-lint run ./...
0 issues.
```

### `make test-web`
```
node --test internal/httpserver/web/markdown.test.mjs
✔ headings 1 to 6 (1.79877ms)
✔ inline formatting: bold, italic, strike, inline code (0.586991ms)
✔ word-boundary rule: identifiers and snake_case are NOT italic (0.340249ms)
✔ link security: only http and https allowed, dangerous protocols stay plain text (0.361931ms)
✔ images from URLs become links if http(s), else plain text (no remote img leaks) (0.938023ms)
✔ raw HTML tags stay text nodes (0.122043ms)
✔ file links: file:// and local paths become filelink nodes, but https urls do not (0.322442ms)
✔ real agent answer with run_id=9 and file links (0.452474ms)
✔ fenced code blocks keep content verbatim with optional lang (0.183162ms)
✔ tables (GitHub style) (0.330097ms)
✔ nested lists with task items (0.22189ms)
✔ mentions (0.158427ms)
✔ pathological 20,000-character line finishes fast (< 500ms) (176.404856ms)
✔ input over 20,000 characters is rendered as plain text (0.156749ms)
✔ ColonyMarkdown.plain strips markdown formatting cleanly (0.420687ms)
ℹ tests 15
ℹ suites 0
ℹ pass 15
ℹ fail 0
```

### `make gen-check`
```
make gen
go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
docker run --rm -u 1000:1000 -v "/mnt/g/Projects/new-chat-worktrees/compaction":/src -w /src sqlc/sqlc:1.31.1 generate
git diff --exit-code -- internal/apispec internal/eventlog/postgres/sqlcgen ...
(exit code 0 - tiszta kódgenerálás)
```

### `go test -race ./internal/guard/ ./internal/e2e/`
```
ok  	colonyhall/internal/guard	7.274s
ok  	colonyhall/internal/e2e	51.062s
```

### `GOFLAGS=-count=1 make test-pg` (uncached PostgreSQL tároló tesztek)
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
=== RUN   TestPostgres/compaction
--- PASS: TestPostgres (2.64s)
    --- PASS: TestPostgres/compaction (0.15s)
PASS
ok  	colonyhall/internal/storetest	2.693s
```

---

## 6. Kézi próba (Manual verification on dev stack)

A dev stacken (`http://localhost:8080`) a következő lépésekkel ellenőrizhető a működés:
1. **Jegyző kiválasztása:** A Gépek (Machines) fülön a *Tömörítés* dobozban állítsuk be a jegyző ágenst egy aktív gépes ágensre (pl. `@scribe-claude`, `haiku` modellel), a küszöböt pedig ideiglenesen alacsonyra (pl. 20 000 token).
2. **Hosszú beszélgetés generálása:** Egy tesztcsatornában vagy szálban illesszünk be egy 80 000 karakteres szövegrészletet vagy több hosszabb üzenetet.
3. **Állapotjelzés és trigger:**
   - A fejlécben a számláló azonnal mutatja a megnövekedett tokent (pl. `~20 400 / 20 000 token`).
   - A szerver automatikusan létrehozza a függő tömörítést és az inboxban felébreszti a jegyzőt.
   - A "🗜 Tömörítés most" gomb "🗜 Tömörítés folyamatban…" állapotra vált és letiltódik.
4. **Jegyző futása és rendszerüzenet:**
   - A jegyző futása után a csatornában megjelenik a rendszerüzenet:  
     `🗜 Automatikus tömörítés — X üzenet, ~20 400 token → ~2 100 tokenes kivonat` a csatolt `.md` chippel.
5. **Kivonat megtekintése és szerkesztése:**
   - A `.md` chipre kattintva a beépített Markdown néző formázva nyitja meg a fejezeteket (`Összefoglaló`, `Döntések`, `Állapot`, `Nyitott kérdések és teendők`, `Fontos részletek`).
   - A "✏️ Szerkesztés" gomb megnyitja a szövegmezőt, ahol a szöveg módosítható és a "Mentés" gombbal elmenthető (`PUT .../summary`).
   - Az "🔄 Újragenerálás" gomb új feladatot küld a jegyzőnek.
6. **Agent újraindítása és priming:**
   - A szálban dolgozó agentnek írva az őr `GET /api/v1/agent/context` hívással észleli az új `compaction_id`-t.
   - Elvégzi a session rotálást és a prompt elejére illeszti a `<colonyhall_summary>` blokkot, amit követnek az új üzenetek. Az agent a kivonat tényei alapján válaszol.

---

## 7. Mit nézzen meg először az átnéző (Claude Code)

1. `internal/modules/compaction/service.go`:
   - `onMessageCreated`: a token-akkumuláció és az auto-trigger feltételei (`approx_tokens_since >= threshold && !hasPending`).
   - `SubmitScribeResult`: tranzakciós fájlmentés, rendszerüzenet létrehozás, egységszámláló reset és eseménykiküldés.
   - `GetAgentContext`: a 200 üzenetes plafon és a rendszerüzenet kizárása.
2. `internal/guard/compaction.go`:
   - `ScribePrompt`: az angol nyelvű instrukció, a fejezetcímek struktúrája, valamint a `<colonyhall_summary>` és `<colonyhall_message>` bemeneti keretezés.
3. `internal/guard/direct.go`:
   - A session rotálási logika és a `<colonyhall_summary>` alapú nyitó prompt inicializálása (`threadRecap` leváltása).
4. `internal/e2e/compaction_test.go`:
   - A teljes integrációs folyamat tesztje: küszöbátlépés -> ébresztés -> jegyző válasz -> rendszerüzenet -> agent context lekérés -> kézi trigger -> timeout.
5. `internal/httpserver/web/index.html`:
   - Fejléc token számláló, kézi indítás, inline szerkesztő és újragenerálás vezérlők.
