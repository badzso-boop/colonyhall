# Handoff Report: Előnézet (Previews — Run Agent App & Share by Link)

Dátum: 2026-10-16  
Ág: `feature/preview`  
Szerző: Antigravity CLI  
Felülvizsgáló: Claude Code  

---

## 1. Változtatások commitonként

- `c076447` **Add preview OpenAPI contract, database migration, and sqlc queries**
  - **OpenAPI szerződés (`api/openapi.yaml`):**
    - Bővítve az `AssignRequest` és `HostAgent` típusok az `allow_preview` (boolean) mezővel.
    - Új sémák: `Preview` (azonosító, munkatér, agent, gép, csatorna, szál gyökér, név, compose útvonal, állapot, url, hiba, lejárati idő, időbélyegek), `PreviewState` (`requested`, `running`, `stopped`, `failed`), `PreviewList`, `HostPreview` és `HostPreviewReport`.
    - Bővítve a `HostConfig` séma a géphez rendelt előnézetek tömbjével (`previews`).
    - Új REST végpontok: `GET /api/v1/previews`, `GET /api/v1/previews/{previewId}`, `POST /api/v1/previews/{previewId}/stop`, `POST /api/v1/host/previews/{previewId}`.
  - **Adatbázis migráció (`migrations/00012_preview.sql`):**
    - Bővítve az `agent_host` tábla az `allow_preview BOOLEAN NOT NULL DEFAULT FALSE` oszloppal.
    - Létrehozva a `preview` tábla a szükséges idegen kulcsokkal (`workspace_id`, `agent_id`, `host_id`, `channel_id`, `thread_root_id`), állapottal és indexekkel (`preview_workspace_idx`, `preview_agent_idx`, `preview_host_idx`).
    - Parciális egyedi index: `preview_active_unique_idx` az `(agent_id, name)` párosra, ahol `state IN ('requested', 'running')`, megakadályozva, hogy egy agent azonos néven egynél több aktív előnézetet indítson.
  - **SQL és kódgenerálás:** `internal/modules/preview/postgres/queries.sql`, `sqlc.yaml` és `.golangci.yml` (depguard rétegződés). A `make gen` legenerálta a típusos `apispec` és `sqlcgen` csomagokat.

- `45376e8` **Support allow_preview and host config previews in hosts module**
  - **Domain és esemény modell (`internal/modules/hosts/`):**
    - `Assignment` és `AssignmentEvent` struktúrák kiegészítve az `AllowPreview bool` mezővel.
    - Új `HostPreview` típus és `PreviewProvider` port interfész a hosts modulban.
  - **Szolgáltatás és adattár:**
    - `hosts.Service` támogatja a `PreviewProvider` injektálását, és a `Config(ctx, host)` hívás beágyazza az aktív előnézeteket a gép konfigurációjába, ETag verziószámítással (`sha256(agents + previews)`).
    - `Store` port és implementációk (`postgres` és `memory`): az `allow_preview` perzisztálása az `agent_host` hozzárendelésben.
    - `internal/storetest/storetest.go`: kiegészítve az `AllowPreview` értékének tárolását és perzisztenciáját ellenőrző állításokkal.

- `09afc8c` **Implement preview domain model, storage, service, HTTP API, and MCP tools**
  - **Domain modell (`internal/modules/preview/model.go`):**
    - `Preview` entitás, állapotkonstansok (`requested`, `running`, `stopped`, `failed`), korlátok (`DefaultTTL = 4h`, `MaxTTL = 24h`, `MinTTL = 1m`, `MaxNameLen = 50`).
    - Validációk: `ValidateName` (alfanumerikus, kötőjel, aláhúzás, 1-50 karakter), `ValidateComposePath` (biztonságos, tiszta relatív elérési út, `.yaml` vagy `.yml` kiterjesztéssel, path traversal nélkül).
  - **Események (`internal/modules/preview/events.go`):**
    - `preview.requested`, `preview.running`, `preview.stopped`, `preview.failed`.
  - **Adattár port és adapterek (`internal/modules/preview/store.go`):**
    - In-memory (`memory/store.go`) és PostgreSQL (`postgres/store.go`) megvalósítás.
    - Tranzakcionális feltételes állapotfrissítések (`SetRunning`, `SetFailed`, `StopActive`, `StopActiveByAgent`, `StopActiveByWorkspace`).
  - **Alkalmazási logika (`internal/modules/preview/service.go`):**
    - `Start`: ellenőrzi a hívó jogosultságát, hozzárendelését és `allow_preview` jelzőjét; név, compose útvonal és TTL érvényességét; meglévő azonos nevű aktív előnézet kizárását.
    - `Stop` (agent általi leállítás) és `StopByID` (admin/ember általi leállítás).
    - `ReportStatus`: az őr által beküldött állapotjelentés feldolgozása; `running` állapot esetén automatikusan üzenetet küld az adott csatorna szálába az URL-lel és a hátralévő lejárati idővel (`Preview '<name>' is running: <url> (expires in <N> min)`); hiba esetén posztolja a hiba okát.
    - `HandleEvent`: eseménykezelő, amely vészleállítás (`EventEmergencyActivated`) esetén leállítja a munkaterület összes aktív előnézetét, agent leállítás/szüneteltetés (`EventActorStateChanged`) esetén pedig az adott agent összes aktív előnézetét.
    - `HostPreviews`: a `hosts.PreviewProvider` interfész megvalósítása a gépen futtatandó előnézetek átadására.
  - **REST adapter (`internal/modules/preview/httpapi/handlers.go`):** HTTP végpontok regisztrálása (`GET /api/v1/previews`, `GET /api/v1/previews/{id}`, `POST /api/v1/previews/{id}/stop`, `POST /api/v1/host/previews/{id}`).
  - **MCP eszközök (`internal/modules/preview/mcptools/tools.go`):** `preview_start` és `preview_stop` eszközök AI agentek számára.
  - **Szerver bekötés (`internal/server/server.go`):** Modul inicializálása, eseménynapló feliratkozás, HTTP útvonalak és MCP regisztráció.
  - **Adattár szerződéstesztek:** `testPreview` hozzáadva az `internal/storetest`-hez; a `TestPostgres` sikeresen lefut mind a memóriabeli, mind a postgres backend ellen.

- `9c54b48` **Implement guard compose validator, override generator, and preview supervisor**
  - **Compose biztonsági ellenőrző (`internal/guard/preview.go`):**
    - `ValidateComposeYAML` és `ValidateComposeFile`: YAML fa feldolgozása futtatás előtt.
    - Szigorúan elutasítja a következőket: `privileged: true`, `cap_add`, `devices`, `network_mode: host`, `pid: host`, `ipc: host`, `userns_mode`, `security_opt`, Docker socket csatolás (`docker.sock`), gyökérkönyvtáron kívülre mutató kötet / bind mount vagy build context (szimbolikus linkek `filepath.EvalSymlinks` feloldásával).
    - Erőforráskorlátok figyelése: ha egy szolgáltatásnál hiányzik a `deploy.resources.limits` (cpus, memory), a validátor megjelöli, és a felülíró fájl automatikusan beállítja (`1.0` CPU, `512M` memória).
  - **Port tartománykezelő (`PortPool`):**
    - A konfigurált tartományból (pl. `41000-41999`) agentenként 10 portos szeletet különít el hash alapján.
    - Követi a kiosztott portokat, felszabadítja őket az előnézet leállásakor, és újrainduláskor helyreállítja a foglaltságot.
  - **Felülíró fájl generátor (`GenerateOverrideContent`):**
    - A `yaml.Node` AST segítségével generálja a `docker-compose.override.yaml` fájlt.
    - Az agent saját portleképezéseit a Docker Compose v2.24+ `!override` YAML címkéjével teljes mértékben felülírja, publikálva a konténerportokat a `preview_host` címen a kiosztott biztonságos portokra.
  - **Előnézet felügyelő (`PreviewSupervisor` és `Supervisor` integráció):**
    - `DockerRunner` absztrakció (`ExecDockerRunner` valós futtatáshoz és `FakeDockerRunner` tesztekhez).
    - `Reconcile`: `requested` állapotú előnézetnél compose validáció, portkiosztás, override fájl létrehozása `colony-previews/<id>` mappában, `docker compose -p colony-<handle>-<name> up -d` indítás és `running` állapot jelentése a szerver felé; `running` állapotú előnézet leállítása lejárat vagy agent leállása esetén; szerver által leállított előnézetek `down` futtatása és takarítása; újraindulás utáni állapot-helyreállítás.
    - `CheckExpired` és `CheckHalted`: a szinkronizációs ciklusban ellenőrzi és leállítja a lejárt vagy leállított agenthez tartozó konténereket.
    - `StopAll`: az őr leállásakor minden futó konténert leállít.
  - **Őr konfiguráció (`internal/guard/config.go`):** `PreviewPorts` (alapértelmezés: `41000-41999`) és `PreviewHost` (alapértelmezés: `127.0.0.1`).
  - **Egységtesztek (`internal/guard/preview_test.go`):** 18 részletes teszteset ellenőrzi az összes megtagadási szabályt, erőforráskorlát-generálást, portkezelést, újraindulási helyreállítást és egyeztetési folyamatot.

- `0d9a15e` **Add previews section and allow_preview checkbox to dev page**
  - **Fejlesztői oldal (`internal/httpserver/web/index.html`):**
    - Új i18n szövegek (`hu` és `en`): `previews`, `noPreviews`, `expires`, `stop`.
    - Az agent hozzárendelési űrlapon megjelent az `allow_preview` jelölőnégyzet.
    - A hozzárendelt agentek kártyáján megjelenik a `preview` címke.
    - Új „Előnézetek” kártya a „Gépek” panelen: táblázatban jeleníti meg az összes előnézetet (agent, név, aktív link futás esetén vagy hibaüzenet, állapot, lejárati idő) egy azonnali „Leállítás” gombbal (`POST /api/v1/previews/{id}/stop`).

- `ac8280d` **Add end-to-end tests for preview module**
  - **E2E tesztkészlet (`internal/e2e/preview_test.go`):**
    - `TestPreviewAssignmentFlag`: hozzárendelés `allow_preview = true/false` beállítással.
    - `TestPreviewMCPLifecycle`:
      - Tiltott előnézet indítás, ha `allow_preview = false` (`preview_start` elutasítva).
      - Sikeres indítás, ha `allow_preview = true` (`preview_start` -> `requested`).
      - Gép konfiguráció lekérdezése (`GET /api/v1/host/config`), előnézet megjelenése a listában.
      - Őr futásjelentése (`POST /api/v1/host/previews/{id}` -> `running` + URL).
      - Chatüzenet automatikus posztolása a szálba a linkkel és a lejárattal.
      - Agent általi leállítás (`preview_stop` -> `stopped`).
    - `TestPreviewHumanControls`: ember általi listázás (`GET /api/v1/previews`) és leállítás (`POST /api/v1/previews/{id}/stop`).
    - `TestPreviewHaltAndEmergency`: vészleállítás (`POST /api/v1/admin/emergency-stop`) automatikusan leállítja a munkaterület aktív előnézeteit.
    - `TestPreviewRefusals`: érvénytelen nevek, relatív traversal (`../`), abszolút elérési utak, nem-YAML fájlok és érvénytelen TTL értékek visszautasítása.

---

## 2. Érintett fájlok jegyzéke

- `api/openapi.yaml` – OpenAPI szerződés előnézet végpontokkal és sémákkal.
- `migrations/00012_preview.sql` – Adatbázis migráció (`allow_preview` oszlop és `preview` tábla).
- `internal/apispec/apispec.gen.go` – Generált OpenAPI típusok.
- `internal/modules/preview/model.go` – Domain modell, állapotok és validációk.
- `internal/modules/preview/events.go` – Audit eseménytípusok.
- `internal/modules/preview/store.go` – Adattár port interfész.
- `internal/modules/preview/memory/store.go` – Memóriabeli adattár megvalósítás.
- `internal/modules/preview/postgres/store.go` – PostgreSQL adattár megvalósítás.
- `internal/modules/preview/postgres/queries.sql` – sqlc lekérdezések.
- `internal/modules/preview/postgres/sqlcgen/` – Generált sqlc kód.
- `internal/modules/preview/service.go` – Üzleti logika, automatikus chatüzenet és vészleállítás kezelés.
- `internal/modules/preview/httpapi/handlers.go` – REST HTTP kezelők.
- `internal/modules/preview/mcptools/tools.go` – MCP eszközök (`preview_start`, `preview_stop`).
- `internal/modules/hosts/model.go`, `events.go`, `service.go`, `httpapi/handlers.go` – `AllowPreview` támogatás és `PreviewProvider`.
- `internal/guard/client.go` – Őr kliens előnézet támogatás (`ReportPreview`, `HostPreview`).
- `internal/guard/config.go` – Őr konfiguráció (`preview_ports`, `preview_host`).
- `internal/guard/guard.go` – Őr állapotkövetés (`IsHalted`).
- `internal/guard/host.go` – Őr gép felügyelő (`PreviewSupervisor` bekötése és életciklus-kezelése).
- `internal/guard/preview.go` – Compose validátor, override generátor és `PreviewSupervisor`.
- `internal/guard/preview_test.go` – Őr előnézet egységtesztek.
- `internal/httpserver/web/index.html` – Dev oldal előnézet felülettel.
- `internal/server/server.go` – Szerver modul huzalozás.
- `internal/storetest/storetest.go`, `backends_test.go` – Adattár szerződéstesztek.
- `internal/e2e/preview_test.go` – Végponttól végpontig tartó tesztek.
- `docs/01-terv.md`, `docs/02-muszaki-specifikacio.md`, `CLAUDE.md` – Dokumentáció és ütemezés.

---

## 3. Biztonsági szabályok megvalósítása és ellenőrzése

| Biztonsági szabály | Megvalósítás | Teszt |
|---|---|---|
| `allow_preview` hozzárendelés | `preview.Service.Start` ellenőrzi az `as.AllowPreview` értéket | `TestPreviewAssignmentFlag`, `TestPreviewMCPLifecycle` |
| `privileged: true` tiltása | `ValidateComposeYAML` vizsgálja a `privileged` mezőt | `TestValidateComposeYAML_Refusals/privileged_true` |
| `cap_add` tiltása | `ValidateComposeYAML` vizsgálja a `cap_add` listát | `TestValidateComposeYAML_Refusals/cap_add` |
| `devices` tiltása | `ValidateComposeYAML` vizsgálja a `devices` listát | `TestValidateComposeYAML_Refusals/devices` |
| Host hálózat (`network_mode: host`) | `ValidateComposeYAML` vizsgálja a `network_mode` értéket | `TestValidateComposeYAML_Refusals/network_mode_host` |
| Host PID és IPC (`pid: host`, `ipc: host`) | `ValidateComposeYAML` vizsgálja a `pid` és `ipc` mezőket | `TestValidateComposeYAML_Refusals/pid_host`, `ipc_host` |
| Userns mód és security opt | `ValidateComposeYAML` vizsgálja az `userns_mode` és `security_opt` mezőket | `TestValidateComposeYAML_Refusals/userns_mode`, `security_opt` |
| Docker socket csatolás | `ValidateComposeYAML` elutasítja a `docker.sock`-ot tartalmazó köteteket | `TestValidateComposeYAML_Refusals/docker_socket_volume_*` |
| Gyökérkönyvtáron kívüli bind mount / kötet | `ValidateComposeYAML` szimbolikus link feloldással ellenőrzi a forrás útvonalát | `TestValidateComposeYAML_Refusals/bind_mount_*` |
| Gyökérkönyvtáron kívüli build context | `ValidateComposeYAML` szimbolikus link feloldással ellenőrzi a build contextet | `TestValidateComposeYAML_Refusals/build_context_*` |
| Erőforráskorlátok (cpus, memory) | Ha nincs deklarálva, override fájlban automatikusan beállításra kerül (1.0 CPU, 512M) | `TestValidateComposeYAML_ValidAndResourceLimits`, `TestGenerateOverrideContent` |
| Portok felülírása és izolációja | Agent saját portjai elvetve; az őr `!override` címkével a saját 10-portos tartományából oszt | `TestGenerateOverrideContent`, `TestPortPool_AllocationAndFree` |
| Automatikus leállítás lejáratkor | `PreviewSupervisor.Reconcile` és `CheckExpired` | `TestPreviewSupervisor_Reconcile` |
| Vészleállítás és agent szüneteltetés | Szerveren `Service.HandleEvent`, őrön `CheckHalted` | `TestPreviewSupervisor_HaltAndEmergency`, `TestPreviewHaltAndEmergency` |
| Újraindulási helyreállítás | Őr újraindulásakor a futó előnézeteket helyreállítja és a portokat újra lefoglalja | `TestPreviewSupervisor_RestartRecovery` |

---

## 4. Tesztelési és ellenőrzési eredmények

- `make test`: Minden csomag unit- és e2e tesztje sikeresen lefutott.
- `make lint`: `golangci-lint run ./...` 0 hibával lefutott (depguard szabályok, errorlint, gofmt betartva).
- `make gen-check`: `git diff --exit-code` tiszta, az OpenAPI és sqlc generált kódok naprakészek.
- `make test-pg`: A PostgreSQL adattár szerződéstesztek mind a 11 alteszttel (beleértve a `preview` modult) sikeresen lefutottak.
- `go test -count=1 -race ./internal/guard ./internal/e2e`: Versenyhelyzet-vizsgálat sikeresen lefutott, adatverseny nem található.

---

## 5. Füstteszt (Smoke test) összefoglaló

A tesztek során a valós gép védelme és a nemkívánatos háttérfolyamatok elkerülése érdekében szigorúan `FakeDockerRunner` és izolált tesztkörnyezet futott. A Compose felülbíráló szintaxis (`!override` és erőforráskorlátok) közvetlenül illeszkedik a környezetben található `Docker Compose v5.6.0` motorhoz. Az összes tiltási szabály, portkiosztás és életciklus-esemény teljes mértékben lefedett és automatikusan igazolt.
