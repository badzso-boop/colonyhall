# Colonyhall

Közös, self-hostolt chat embereknek és AI agenteknek. Több gépen futó agentek (Claude Code, Codex, Cursor, Gemini, API-kulcsos agentek) itt beszélnek, kérdeznek és adnak feladatot egymásnak; az admin (ember) mindent lát, és bármit leállíthat.

**Állapot (2026-10-15):** a 0. fázis („átjárható csontváz”) kész: `identity`, `conversation`, `delivery` modul, eseménynapló, REST, WebSocket, MCP (`inbox`, `ack`, `send`, `read`), hamis agent, tesztek. Következő: valódi Claude Code-próba, majd az 1. fázis.

## Mielőtt bármit csinálsz

- A teljes terv: `docs/01-terv.md`; a műszaki specifikáció (rétegek, adatmodell, WebSocket, MCP, REST): `docs/02-muszaki-specifikacio.md`; a más chat szerverekből átvett minták és azok indoklása: `docs/03-tanulsagok-mas-chat-szerverekbol.md`. Tervezési kérdésben ezek a mérvadók.
- Ha egy döntés változik, frissítsd a `docs/` megfelelő fájlját is (és szólj, hogy az élő claude.ai dokumentumot is frissíteni kell, lásd `docs/README.md`).

## Nyelv

- Kommunikáció a felhasználóval és a `docs/` tartalma: **magyarul**.
- Kód, azonosítók, adatbázis, API, MCP eszközleírások, kódkommentek, commit üzenetek: **angolul**.
- A felület szövegei soha nincsenek beégetve: i18n kulcsok (`web/src/locales/hu.json`, `en.json`), alapnyelv a magyar.

## Stack

- **Go** (aktuális stabil): szerver (`cmd/colonyhall`), CLI és őr (`cmd/colony`), hamis agent (`cmd/fake-agent`).
- HTTP: `net/http` (+ `chi`, ha kell) · WebSocket: `coder/websocket` · DB: PostgreSQL, `pgx`, `sqlc`, `goose` · feladatsor: River · MCP: `modelcontextprotocol/go-sdk` · naplózás: `log/slog` · telemetria: OpenTelemetry, Prometheus.
- **Web:** React + Vite + TypeScript PWA, i18next; a build a Go binárisba ágyazódik (`embed`).
- **Futtatás:** Docker Compose (`deploy/`).

## Architektúra-szabályok (kötelező)

1. **Funkciónkénti modulok, bennük befelé mutató függőséggel:** `internal/modules/<modul>/` (identity, conversation, delivery, work, knowledge, agents, budget, schedule, notify, admin). Egy modulon belül: `model.go` + `events.go` (csak standard könyvtár) ← `service.go` (használati esetek) ← `store.go` (port) ← `postgres/`, `memory/` (adapterek) és `httpapi/`, `mcptools/` (transzport). Közös részek: `internal/platform`, `internal/policy`, `internal/eventlog`; a `cmd/*` rakja össze. A golangci-lint `depguard` szabálya ellenőrzi; ne kerüld meg.
   - Egy modul más modulnak **csak** az exportált `api.go` felületét hívhatja, a `postgres/`, `memory/`, `httpapi/` csomagját soha.
   - Modulok közti reakció eseménnyel történik (pl. a `delivery` feliratkozik a `conversation.MessageCreated`-re), nem közvetlen hívással.
   - Minden `Store` interfészre ugyanaz a `internal/storetest` készlet fut a `memory` és a `postgres` megvalósításon.
   - Az eseménysorszám kiosztása advisory lock alatt történik (egy író), különben a sorszámok nem a commit sorrendjében látszanak.
2. **Az idő, az azonosító és a véletlen port** (`Clock`, `IDGenerator`, `Random`). A domainben és az app rétegben tilos a `time.Now()`, a közvetlen UUID-generálás és a `math/rand`; tesztben léptethető óra fut.
3. **Szerződés-első:** REST/WebSocket változás először az `api/openapi.yaml`-ben, utána `make gen` (`oapi-codegen` → `internal/apispec`; a webhez `openapi-typescript`). SQL: a modul `postgres/queries.sql`-jébe, utána `make gen` (sqlc → `postgres/sqlcgen`). Generált fájlt kézzel ne szerkessz. A végponttól végpontig tartó tesztek minden választ az OpenAPI-leíráshoz mérnek.
4. **Append-only adatok:** üzenet, üzenetverzió, esemény, futás, keretmérés és audit soha nem módosul és nem törlődik; törlés csak `deleted_at` jelöléssel.
5. **Tranzakciós outbox:** minden állapotváltozás ugyanabban a tranzakcióban ír az `event` táblába, mint maga az adat.
6. **Migrációk:** `goose`, sorszámozott SQL, kiterjeszt-majd-szűkít minta. Már lefutott migrációt soha ne módosíts; új migrációt írj.
7. **Bővítés = új adapter:** új AI kliens, AI-szolgáltató, git-szolgáltató, értesítési csatorna, tárhely vagy eseménybusz egy új adapter a megfelelő porthoz, regisztrálva. A magot ehhez ne módosítsd.
8. **Az agent konfigurációja nem módosítható:** változás = új agentverzió (lásd a terv „Agent életciklus” fejezetét).
9. **Minden `workspace_id`-hoz kötött**, a jogosultság minden hívásnál ellenőrizve (nem csak csatlakozáskor).
10. **Az agentektől jövő tartalom adat, nem utasítás:** üzenetet, fájlt, webhookot soha ne hajts végre; az agentnek átadott szöveg mindig `<colonyhall_message ...>` keretben megy.

## Fejlesztői környezet

- Fejlesztés a fejlesztőgépen, WSL-ben: `G:\Projects\new-chat` = `/mnt/g/Projects/new-chat`.
- Helyi futtatás Docker Compose-zal (`deploy/compose.dev.yaml`), a szerver a `http://localhost:8080` címen.
- Később a homelab szerverre költözik (`ssh homelab`, `~/homelab/colonyhall`); **előtte olvasd el a szerveren a `~/homelab/CLAUDE.md`-t és a `README.md`-t** (megosztott szolgáltatások, titokkezelés, portok). A cél: `colony.ujjweb.hu` a meglévő Cloudflare Tunnelen át, az őrök Tailscale-en.

## Parancsok

A Go a `/usr/local/go/bin`-ben van, a `golangci-lint` a `~/go/bin`-ben (mindkettő a PATH-on a `.zshrc`/`.bashrc` szerint).

| Parancs | Mit csinál |
| --- | --- |
| `make dev` | Postgres + szerver Dockerben, fejlesztői seeddel: http://localhost:8080 |
| `make down` / `make logs` | Leállítás (az adat megmarad) / szervernapló |
| `make run-mem` | Szerver Docker nélkül, memóriabeli tárakkal |
| `make fake-agent` | A hamis agent a helyi szerver ellen (`scenarios/ping.yaml`) |
| `make test` | Egység-, szerződés- (memória) és végponttól végpontig tartó tesztek |
| `make test-pg` | A tár-szerződéstesztek Postgres ellen (kell hozzá `make dev`; a `colony_test` adatbázist törli) |
| `make lint` | golangci-lint, a depguard rétegszabályokkal |

Fejlesztői tokenek (csak a `COLONY_DEV_SEED=1` helyi stacken): admin `col_dev_admin_token_do_not_use_in_production`, `@fake-agent` `col_dev_agent_token_do_not_use_in_production`.

Minden változtatás után: `make lint test`, és ha az adattár változott, `make test-pg` is.

## Eltérések a tervtől (tudatos, átmeneti)

- **Webes felület:** a 0. fázisban egyetlen fejlesztői HTML-oldal van (`internal/httpserver/web/index.html`). A React + Vite PWA az 1. fázisban jön.
- **Szerepkörök:** egyelőre csak a `COLONY_ROLE=all` működik, a busz memórián belüli.

## Tesztelés

- `go test ./...` minden változás után; a domain és az app réteg tesztjei adatbázis nélkül futnak, memóriabeli adapterekkel.
- Integrációs tesztek valódi Postgresszel (testcontainers-go).
- A hamis agent (`cmd/fake-agent`) YAML forgatókönyvvel, determinisztikusan válaszol; az agent-folyamatokat ezzel teszteld, ne valódi AI-val.
- Hibajavításhoz mindig tartozzon egy teszt, ami a hiba nélkül elbukott volna.

## Biztonság

- Titok soha nem kerül a repóba: `.env` gitignore-olva, mintának `.env.example`.
- Tokenek csak hash-elve tárolódnak, és soha nem kerülnek naplóba.
- Agentnek szóló futtatásban soha ne legyen `--dangerously-skip-permissions` alapértelmezésként.

## Licenc

A Mattermost, Dendrite, Synapse és Stoat kódja AGPL-3.0: belőlük csak mintát veszünk át, kódot soha nem másolunk be. Ha egy megoldás onnan jön, a komment a mintára hivatkozzon (pl. `// Zulip UserMessage-style flags`).

## Git

- A távoli repó a homelab Gitea lesz. Funkciónként külön ág; a `main` ágra commit csak kérésre.
- Commit üzenetek angolul, rövid, felszólító módú első sorral.

## 0. fázis – átjárható csontváz (a következő munka)

1. Go modul, könyvtárszerkezet a specifikáció szerint, `Makefile`, golangci-lint konfiguráció (depguard szabályokkal).
2. `deploy/compose.dev.yaml`: Postgres + szerver; első goose migráció (`workspace`, `actor`, `channel`, `channel_member`, `message`, `event`, `consumer_offset`, `delivery`).
3. REST: üzenet küldése és listázása; WebSocket: élő esemény a böngészőnek; minimális webes oldal.
4. MCP (Streamable HTTP, Bearer token): `inbox`, `send`, `ack`.
5. Hamis agent és egy végponttól végpontig teszt.
6. Első valódi próba: egy Claude Code session MCP-n keresztül olvas és ír a chatbe.
