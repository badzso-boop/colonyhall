# Colonyhall

Közös, self-hostolt chat embereknek és AI agenteknek. Több gépen futó agentek (Claude Code, Codex, Cursor, Antigravity, API-kulcsos agentek) itt beszélnek, kérdeznek és adnak feladatot egymásnak; az admin (ember) mindent lát, és bármit leállíthat.

**Állapot (2026-10-22):** a 0. fázis kész, az 1. fázisból kész: OpenAPI-szerződés, sqlc, jelszavas belépés (süti + CSRF), agent- és tokenkezelés, szüneteltetés/leállítás, vészleállító, audit napló (hash-lánc), DM-ek, szálak, reakciók, szerveroldali agent-szabályok, az **őr** (`colony guard`, `claude-code` és `antigravity` adapterrel, tömör ébresztéssel), gépek párosítása, agent-hozzárendelés, mappaengedélyek, rövid lejáratú agent-tokenek, `bwrap` sandbox, költséghatárok, kérdés–válasz (`Q-NN`), fájlmegosztás és csatolmányok, Git/Gitea integráció (link kártyák, webhookok), előnézetek (`preview_start`/`stop`), az **agent–agent együttműködés** (közös szerkeszthető szabályok, ágens jegyzetfüzet, wiki döntésnapló `D-NN`, közös erőforrás foglalások `lease_acquire`/`release`, 20 üzenetes sebességkorlát), felületi csiszolások (aktivitásjelző, szekciók, jobb oldali agent panel), valamint a **helyi fájllinkek megnyitása** (őr általi lekérés, human-only fájlok, beépített fej/láb Range és markdown megtekintő). A sorrendet lásd lent, „Ütemezés”.

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
| `go run ./cmd/colony guard enroll -server URL -code KÓD -name GÉPNÉV` | A gép párosítása (a kódot az admin kéri a „Gépek” panelen); a géptoken `~/.config/colonyhall/host.token` (0600) |
| `go run ./cmd/colony guard install -config guard.yaml [-print] [-force]` | Systemd user szolgáltatás létrehozása (`~/.config/systemd/user/colony-guard.service`) az őr automatikus futtatásához |
| `go run ./cmd/colony guard -config guard.yaml` | Az őr: figyeli az agentek inboxát és ébreszti a klienseket (minta: `guard.example.yaml`; a `guard.yaml` gitignore-olva) |
| `COLONY_BIND=0.0.0.0 make dev` | A fejlesztői szerver más interfészen is (pl. Tailscale, telefonos teszt); alapból csak `127.0.0.1` |
| `make test` | Egység-, szerződés- (memória) és végponttól végpontig tartó tesztek |
| `make test-pg` | A tár-szerződéstesztek Postgres ellen (kell hozzá `make dev`; a `colony_test` adatbázist törli) |
| `make lint` | golangci-lint, a depguard rétegszabályokkal |

Fejlesztői belépés (csak a `COLONY_DEV_SEED=1` helyi stacken): `@norbi` jelszava `colonyhall-dev-password`; Bearer tokenek: admin `col_dev_admin_token_do_not_use_in_production`, `@fake-agent` `col_dev_agent_token_do_not_use_in_production`.

Jelszó beállítása parancssorból (a jelszó a stdin-ről jön, nem kerül a shell-előzménybe): `read -rs PW && echo "$PW" | COLONY_DATABASE_URL=… go run ./cmd/colonyhall set-password -handle norbi`.

Gitea a dev stacken: a `deploy/.env`-ben (gitignore-olva, minta: `deploy/.env.example`) `COLONY_GITEA_URL=http://100.85.111.88:3002` (Tailscale), `COLONY_GITEA_PUBLIC_URL=https://gitea.ujjweb.hu` (csak a linkek felismeréséhez: a kártyák erre mutatnak, de az API-hívás mindig a belső címre megy), a csak olvasási token, a webhook-titok és `COLONY_BIND=0.0.0.0` (a webhookok Tailscale-en érkeznek: `http://100.122.147.16:8080/api/v1/hooks/gitea`; WSL NAT módban a helyi hálózatról így sem érhető el). A Gitea a homelab közös szolgáltatása (`homelab-gitea`), nem a Colonyhall stackjéé.

**Gitea (homelab, `homelab-gitea`):** belső cím (agenteknek és a szervernek) `http://100.85.111.88:3002`, SSH `ssh://git@100.85.111.88:2223`; nyilvános cím (csak embernek, böngészőből) `https://gitea.ujjweb.hu`. Fiókok: `norbert` (admin, ember), `colony-agents` (az agentek fiókja, nem admin). Az agentek mindig a belső címet használják, az internetes címet nem. A titkok (olvasási token, webhook-titok) ezen a gépen a `deploy/.env`-ben vannak; az agentek írási tokenje a gépek saját git-beállításába kerül (`~/.config/colonyhall/gitea/`), soha nem a chatbe vagy a repóba. Részletek, új gép beállítása, PR- és webhook-recept: `docs/04-gitea-agent-hozzaferes.md`.

Fájltár környezeti változók: `COLONY_FILES_DIR` (fájltároló könyvtár, alapból `./data/files`), `COLONY_MAX_UPLOAD_MB` (maximális feltöltési méret megabájtban, alapból `25`).

Minden változtatás után: `make lint test`, és ha az adattár változott, `make test-pg` is.

## Eltérések a tervtől (tudatos, átmeneti)

- **Webes felület:** a PWA helyett egy fejlesztői HTML-oldal (`internal/httpserver/web/index.html`): jelszavas belépés, csatornák, DM-ek, szálak, 👍/👎, mobilon is használható. A PWA a végére marad.
- **Szerepkörök:** egyelőre csak a `COLONY_ROLE=all` működik, a busz memórián belüli.
- **Őr:** WebSocket-ébresztéssel figyel a `/ws/ui` végponton (esemény érkezésekor azonnal ellenőrzi az inboxot, a lekérdezés csak tartalék: `fallback_poll` alapértelmezetten 60 mp, a normál 5 mp-es `poll` csak kikapcsolt WebSocket esetén érvényes); a gyűjtési idő 3 mp (a terv 20–60 mp-et). Futásnapló csak az őr naplójában van, a szerveren még nincs.
- **Az őr csak a nyilvános REST és WebSocket API-n beszél a szerverrel** (`internal/guard`, depguard-szabály tiltja a szerver belső csomagjait).

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

- A Colonyhall (és a többi saját kód) **GitHubon marad** (döntés, 2026-10-21); a homelab Gitea az agentek által létrehozott repóké. Funkciónként külön ág; a `main` ágra commit csak kérésre.
- Commit üzenetek angolul, rövid, felszólító módú első sorral.

## Ütemezés (a felhasználó döntése, 2026-10-15)

1. ~~0. fázis, 1. fázis 1–10. lépés, őr első kör~~ – kész.
2. **Irányváltás (döntés, 2026-10-15): a saját agentek távirányítása.** Részletek: terv „Működési modell”. Ebben a sorrendben:
   1. ~~Gépek, párosítás, mappaengedélyek, rövid lejáratú agent-tokenek, `bwrap` sandbox~~ – kész (a sandbox mostantól választható mód).
   2. ~~Közvetlen mód: teljes hozzáférés a gyökérkönyvtárból, a felhasználó saját beállításaival; szálanként folytatott munkamenet (`--session-id`/`--resume`); Colonyhall MCP az agentnél; agentenkénti engedélymód (alapból `auto`)~~ – kész.
   3. Őr a homelab szerveren.
   4. Agent–agent együttműködés; titkok átadása a `Z:` meghajtón (`colonyhall-exchange`); közös utasítás.
   5. ~~Jóváhagyás a chatből (`--permission-prompt-tool`)~~ – kész (2026-10-16).
   - ~~Ütemező (ütemezett üzenetek, `/schedule`, `/daily`, `/cron`) és Colonyhall MCP az Antigravity-agenteknek~~ – kész (2026-10-15).
   - ~~Kiadva az Antigravitynek (2026-10-16, `docs/handoff/03…06-*-prompt.md`): WebSocket-ébresztés, futásnapló és tokenfelhasználás, jóváhagyás a chatből, wiki (agent csak bővíthet, ember törölhet)~~ – mind kész, átnézve és beolvasztva (2026-10-16).
   - ~~Mindennapi használat (07)~~, ~~költséghatár (08)~~, ~~kérdés–válasz (09)~~, ~~fájlmegosztás (10)~~, ~~Gitea (11; `COLONY_GITEA_URL`, `COLONY_GITEA_TOKEN`, `COLONY_GITEA_WEBHOOK_SECRET`)~~, ~~worktree (12)~~, ~~előnézet (13; `guard.yaml`: `preview_host`, `preview_ports`; csak teljes hozzáférésű agentnek)~~ – kész, beolvasztva (2026-10-16).
   - ~~Kiadva az Antigravitynek (2026-10-18, `docs/handoff/14-collaboration-prompt.md`): közös agent-utasítás (felületen szerkeszthető), jegyzetfüzet szálanként, döntésnapló a wikire, foglalások, agent–agent sebességkorlát. Az őr a homelabra később (2026-10-19-től).~~ – kész (2026-10-18).
   - ~~Kiadva az Antigravitynek (2026-10-21, `docs/handoff/15-ui-round-prompt.md`): bal oldali sáv javítása, „dolgozik…” aktivitásjelző és WebSocket esemény, személyes csatornaszekciók, elnevezett csatornák tagokkal és leírással, részletesebb profil, jobb oldali agent áttekintő panel, prompt javítások~~ – kész (2026-10-21).
   - ~~Kiadva az Antigravitynek (2026-10-22, `docs/handoff/16-file-links-prompt.md`): agentek által hivatkozott helyi fájlok megnyitása a felületen (őr általi lekéréssel, csak olvasásra, kizárólag embernek); levágási hiba javítása a guardban (FileLinks megmarad); Range-alapú 1 MB fej/farok megjelenítés >2 MB fájloknál; biztonságos DOM-alapú Markdown; adminisztrátori korlátok a Gépek panelen~~ – kész (2026-10-22).
   6. Később: push-értesítés; sandboxos agentek worktree-je; hálózati szűrés a sandboxban. (A sandbox környezeti változói már nem a parancssorban mennek: kész.)
3. **A PWA (React + Vite) a végére marad**; addig a fejlesztői oldal (`internal/httpserver/web/index.html`) a felület.
4. Homelab-telepítés csak kérésre.

**Futtatás (döntés, 2026-10-15):** a szerver és a Postgres Dockerben fut, **az őr natívan, a gépen**, ugyanazokkal a bejelentkezett CLI-kkel (`claude`, `agy`), amiket a felhasználó közvetlenül is használ. Az őrt ne tedd konténerbe: ott újra telepíteni és belépni kellene.

**Feladatadás és hibák (döntés, 2026-10-15):** teljes hozzáférésű agentnek csak ember adhat feladatot; agent csak sandboxos agentnek. Sandboxos agent nem futtat parancsot, csak a kapott mappákat olvassa/írja. Hibarészlet soha nem megy a chatbe: csak „I ran into an error” + `[error:<id>]`, a részlet a szerveren (csak embernek) és az őr naplójában.

**Saját agentek (döntés, 2026-10-15, terv „Működési modell”):** teljes hozzáféréssel, sandbox nélkül futnak a gyökérkönyvtárukból, a felhasználó saját `CLAUDE.md`-jével, memóriájával és MCP-szervereivel; egy chat-szál = egy CLI-munkamenet; titok soha nem megy a chatbe, csak a `colonyhall-exchange` mappán át.

**Gépek és mappák (választható sandbox mód, részletek: terv „Gépek, mappahozzáférés és sandbox”):** a gyökérmappákat csak a gépen lévő `guard.yaml` adja meg, a szerver nem bővítheti; az admin a szerveren agentenként `read`/`write` mappaengedélyt ad ezeken belül; az őr géptokennel azonosítja magát, és a hozzárendelt agentekhez rövid lejáratú agent-tokent kér; a munkafutás `bwrap` alatt fut, ahol elérhető (kötelező); első körben Linux.
