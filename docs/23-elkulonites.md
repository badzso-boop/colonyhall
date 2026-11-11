# Ügyfelek elkülönítése (45. kör, 0. fázis): leltár és RLS-terv

> Állapot: **elfogadott terv, 2026-11-11** (a döntések a 12. pontban). A 22. dokumentum (`22-tobbberlos-terv.md`) 6. és 14. pontjának megvalósítási terve.

## 1. Összefoglaló

- **Egy pont, ahol minden lekérdezés átmegy:** a tárak kizárólag a `db.Postgres.Q(ctx)`-n át érik el az adatbázist (tranzakcióban a `pgx.Tx`, azon kívül a pool), a tranzakciót pedig a `Pool.Begin(ctx)` nyitja. Ezen kívül csak a migráció (`migrations.Up`), a `Ping` és a `Close` használja a poolt. Más közvetlen poolhasználat nincs a kódban.
- **Javaslat:** a munkaterület (vagy a rendszermód) a `context`-ben utazik (`db.WithWorkspace`), és a pool **minden kapcsolatkiadáskor** (`pgxpool.Config.PrepareConn`) beállítja a kapcsolaton: `colony.workspace_id` és `colony.system`. Ez egyformán lefedi a tranzakciós és a tranzakción kívüli lekérdezéseket, és nem kell minden lekérdezés köré rövid tranzakció.
- **A szabály minden táblán:** egy sor akkor látszik, és akkor írható, ha `workspace_id = colony_ws()` **vagy** `colony_system()`. Ha a beállítás hiányzik, `colony_ws()` NULL, tehát **semmi** nem látszik.
- **A 15 `workspace_id` nélküli tábla:** 11 kap denormalizált `workspace_id`-t (a beállításból jövő alapértékkel, így az írási utak kódja nem változik), 2 csak rendszermódban látható, 1 munkaterületenkénti gyorsítótár lesz, és a `workspace` a saját `id`-ján szűr.
- **Belépés előtti keresések** (token → munkaterület, slug → munkaterület stb.) szűk, `SECURITY DEFINER` SQL-függvényekkel mennek, amelyek csak azonosítót adnak vissza. Így kérést kiszolgáló kód **soha** nem kapcsol rendszermódba.
- **Rendszermódot** csak a háttérkód kap, egyetlen csomagon át (`internal/platform/db/system`), amit egy depguard-szabály csak a `internal/server`, az `internal/eventlog` fogyasztója és a `cmd/colonyhall` számára enged.

## 2. A beállítás útja az adatbázisig

### 2.1 A `context` hatóköre

| Függvény | Csomag | Ki hívhatja |
| --- | --- | --- |
| `db.WithWorkspace(ctx, id)` | `internal/platform/db` | bárki; a rendszermódot **kikapcsolja** (szűkít) |
| `db.Scope(ctx)` | `internal/platform/db` | a pool és a tesztek olvassák |
| `system.Context(ctx)` | `internal/platform/db/system` | **csak** `internal/server/**`, `internal/eventlog/consumer.go`, `cmd/colonyhall/**` (depguard `system-mode-is-background-only`) |

A `WithWorkspace` szűkítés, nem bővítés: rendszermódból egy munkaterületre lép, és visszafelé nincs út. Egy kérés kontextusa (`r.Context()`) a hitelesítés után kapja meg a hívó munkaterületét.

### 2.2 Hogyan jut el a Postgresig: két lehetőség

| | **A) Kapcsolatkiadáskor (javaslat)** | B) `SET LOCAL` a tranzakcióban |
| --- | --- | --- |
| Hogyan | `PrepareConn(ctx, conn)`: `SELECT set_config('colony.workspace_id', $1, false), set_config('colony.system', $2, false)` minden kiadáskor, a ctx hatóköréből; hatókör nélkül mindkettő üres | `InTx` a `Begin` után `set_config(…, true)`; a tranzakción kívüli `Q(ctx)` minden lekérdezése egy rövid, saját tranzakcióba kerül (a `Rows` lezárásakor commit) |
| Lefedettség | Minden út, a `Begin` is (a pool ugyanazzal a ctx-szel adja ki) | Minden út, de a `Q(ctx)` burkolója új kód (Query, QueryRow, Exec; a sorok élettartama) |
| Költség | +1 körút kapcsolatkiadásonként (helyben ~0,1 ms) | +2–3 körút a tranzakción kívüli lekérdezéseknél (BEGIN, set_config, COMMIT) |
| Szivárgás | Nem lehet: minden kiadás felülírja (üresre is) | Nem lehet: tranzakció-lokális |
| Korlát | Tranzakciós módú kapcsolatgyűjtő (PgBouncer `transaction`) mögött nem működik; a 22. terv 7. pontja (NOTIFY) eleve közvetlen kapcsolatot kér | PgBouncer mögött is működik |

A javaslat az **A**: egyetlen, rövid kód a `db.Open`-ben, és a tranzakciós és nem tranzakciós út ugyanúgy viselkedik. Ha később PgBouncer jön, a B-re váltás a `db` csomagon belül marad.

### 2.3 Az SQL-oldal

```sql
-- Az aktuális munkaterület; hiányzó vagy üres beállítás: NULL (semmi nem látszik).
CREATE FUNCTION colony_ws() RETURNS uuid LANGUAGE sql STABLE
  AS $$ SELECT nullif(current_setting('colony.workspace_id', true), '')::uuid $$;
CREATE FUNCTION colony_system() RETURNS boolean LANGUAGE sql STABLE
  AS $$ SELECT coalesce(current_setting('colony.system', true), '') = 'on' $$;

-- Minden munkaterületi táblán (példa):
ALTER TABLE message ENABLE ROW LEVEL SECURITY;
ALTER TABLE message FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON message
  USING (workspace_id = colony_ws() OR colony_system())
  WITH CHECK (workspace_id = colony_ws() OR colony_system());
```

A `STABLE` függvényeket a tervező lekérdezésenként egyszer értékeli ki, így a meglévő `(workspace_id, …)` indexek használhatók maradnak. A **mért** többletköltséget a jelentés adja meg (a leglassabb e2e-utak előtte és utána).

**Fontos:** az egyéni beállításokat (`colony.*`) az alkalmazás felhasználója maga is átírhatja. Az RLS tehát **programhiba ellen** véd (elfelejtett szűrés, rossz azonosító), nem egy feltört szerverfolyamat ellen; ez megegyezik a 22. terv céljával (második védvonal).

## 3. Táblák és szabályok

Jelmagyarázat: **W** = `workspace_id = colony_ws() OR colony_system()`; **S** = csak rendszermód; **D** = denormalizált `workspace_id` kerül bele (kiterjeszt–feltölt–szűkít), utána **W**.

### 3.1 A 39 tábla, amiben már van `workspace_id`

Mind **W** szabályt kap, változtatás nélkül:

`actor`, `agent_host`, `agent_instructions`, `agent_note`, `agent_rules`, `agent_run`, `approval_request`, `audit_log`, `channel`, `compaction`, `compaction_unit`, `decision`, `delegation`, `delegation_grant`, `delegation_settings`, `delivery`, `event`, `file`, `file_fetch_request`, `folder_grant`, `git_repo_channel`, `host`, `host_pairing_code`, `lease`, `message`, `model_price`, `planner_access`, `planner_calendar`, `planner_task`, `preview`, `question`, `run_error`, `schedule`, `sidebar_section`, `terminal_event`, `wiki_page`, `workspace_compaction_settings`, `workspace_currency`, `workspace_file_fetch_settings`.

Megjegyzések:

- Az `audit_log`, `delivery` és `event` oszlopa idegen kulcs nélküli, de `NOT NULL`; a szabály ugyanaz.
- Az `event` a WebSocket utólagos küldésénél (`catchUp`) munkaterületi hatókörben olvasódik: egy kliens az RLS miatt is csak a saját munkaterülete eseményeit látja (a sorszámokban lyukak lesznek; a kurzor ezt ma is kezeli).
- Az `audit_log` hash-lánca már munkaterületenként fut (`LastEntry`, `ForwardEntries` a `workspace_id` szerint), tehát munkaterületi hatókörben is ép.

### 3.2 A 15 tábla `workspace_id` nélkül

| Tábla | Hogyan kötődik | Döntés | Indoklás |
| --- | --- | --- | --- |
| `api_token` | `actor_id → actor` | **D** | Minden kérés hitelesítése olvassa; egy `EXISTS` minden kérésnél felesleges join |
| `session` | `actor_id → actor` | **D** | Ugyanaz: minden böngészős kérés |
| `credential` | `actor_id → actor` | **D** | Kicsi, de a belépés útja; az egységes szabály egyszerűbb |
| `channel_member` | `channel_id → channel` | **D** | Forró és nagy: minden jogosultság-ellenőrzés és lista |
| `channel_mute` | `channel_id → channel` | **D** | Az oldalsáv minden betöltésnél olvassa |
| `reaction` | `message_id → message` | **D** | Nagy, üzenetlistákkal együtt olvasva |
| `thread_read` | `thread_root_id → message` | **D** | Forró írás (olvasottság), nagy |
| `message_file` | `message_id → message` | **D** | Üzenetlistákkal együtt olvasva |
| `sidebar_item` | `section_id → sidebar_section` | **D** | Az oldalsáv minden betöltésnél olvassa |
| `planner_calendar_event` | `calendar_id → planner_calendar` | **D** | A naptárfrissítés sok sort ír; egységes szabály |
| `wiki_revision` | `page_id → wiki_page` | **D** | Csak írás és előzmény; egységes szabály |
| `workspace` | — | saját: `id = colony_ws() OR colony_system()` | A slug szerinti keresés belépés előtt függvényen át megy (4. pont) |
| `consumer_offset` | — (szervertábla) | olvasás: mindenki; írás: **S** | Csak fogyasztónevek és sorszámok, ügyféladat nincs; a Rendszerállapot oldal olvassa |
| `git_webhook_delivery` | — (szervertábla) | **S** | Csak a webhook-azonosítók ismétlésszűrése; függvényen át írva (4. pont) |
| `git_link` | — (URL szerinti gyorsítótár) | **D**, kulcs `(workspace_id, url)` | Ma közös gyorsítótár: egy privát repó címe és állapota átlátszana egy másik ügyfélhez, aki ugyanazt az URL-t írja be. Munkaterületenként külön tárolva a kártya csak ott jön létre, ahol a link elhangzott |

**Miért nem `EXISTS (…)` a szülőn át:** az írási utakat nem kellene módosítani, de minden olvasás (és minden `WITH CHECK`) egy szülőkeresést kapna, a forró táblákon (`channel_member`, `reaction`, `thread_read`) éppen ott, ahol a legtöbb lekérdezés fut. A denormalizálás írási oldalát így oldom meg:

```sql
ALTER TABLE reaction ADD COLUMN workspace_id uuid
  DEFAULT colony_ws();                            -- az új sorok a beállításból kapják
UPDATE reaction r SET workspace_id = m.workspace_id FROM message m WHERE m.id = r.message_id;  -- feltöltés
ALTER TABLE reaction ALTER COLUMN workspace_id SET NOT NULL;   -- szűkítés
```

Az `INSERT`-ek nem változnak: a sor a hatókör munkaterületét kapja. Ha a hatókör hiányzik, a `NOT NULL` hangosan elbukik, tehát nem keletkezik gazdátlan sor. A memóriatárakban nincs mit változtatni.

**Kereszthivatkozás ellen (javaslat):** a szülőkön `UNIQUE (id, workspace_id)`, a gyerekeken összetett idegen kulcs (`(message_id, workspace_id) → message (id, workspace_id)`). Az idegen kulcs ellenőrzése megkerüli az RLS-t, ezért enélkül egy programhiba A ügyfél üzenetére tehetne reakciót B hatókörében. A költsége egy-egy új egyedi index a szülőkön (`actor`, `channel`, `message`, `sidebar_section`, `planner_calendar`, `wiki_page`); a `message`-en ez a legnagyobb. (12/2: igen)

### 3.3 Nem összeolvasztott ágak

- **50. kör** (`feature/token-cleanup`): `db_size_sample` (a teljes adatbázis mérete) → **S** olvasásra is? A Rendszerállapot oldal olvassa, ami ügyféladmin-oldal; a 22. terv szerint operátori adat. Javaslat: olvasás mindenkinek (nincs benne ügyféladat), írás **S**. A karbantartó ciklus (a tokenek és munkamenetek törlése) rendszermódban fut.
- **36. kör** (`feature/rag`): `search_chunk_text`, `search_log` (van `workspace_id`) → **W**. A `rag-backfill`, `rag-reembed` és `rag-eval` parancs rendszermódban indul, és munkaterületenként szűkít. Beolvasztáskor egy-egy sor a migrációban és a teszttáblában.

## 4. Belépés előtti keresések: `SECURITY DEFINER` függvények

Ezek a keresések még nem tudják a munkaterületet, ezért egyik hatókörben sem futnának. Mindegyik egy szűk függvény, ami **csak azonosítót** ad vissza; a valódi lekérdezés utána a normál tárakon, munkaterületi hatókörben fut. A függvény belül rendszermódot kapcsol, és a végén visszaállítja (a tulajdonosra is érvényes a `FORCE`).

| Függvény | Ki hívja | Visszaad |
| --- | --- | --- |
| `colony_token_workspace(hash bytea)` | `identity.Authenticate` (Bearer, MCP, `/ws/ui` hello) | a token munkaterülete |
| `colony_session_workspace(hash bytea)` | `identity.AuthenticateSession` | a munkamenet munkaterülete |
| `colony_login_workspaces(handle text)` | belépés (egytagú módban, ahol egy név több munkaterületen is lehet) | `(id, name)` párok |
| `colony_workspace_by_slug(slug text)` | az aldomain-köztes réteg (többbérlős mód) | `(id, state)` |
| `colony_host_workspace(hash bytea)` | `hosts.AuthenticateHost` (`/api/v1/host/*`, `/ws/host`) | a gép munkaterülete |
| `colony_pairing_workspace(hash bytea)` | `POST /hosts/enroll` | a párosítókód munkaterülete |
| `colony_repo_workspaces(repo text)` | Gitea-webhook | a repót csatornához kötő munkaterületek |
| `colony_webhook_seen(id text)` | Gitea-webhook | igaz, ha már feldolgozott (és rögzíti) |
| `colony_event_head()` | `/ws/ui` hello (hitelesítés előtt), Rendszerállapot | a legnagyobb sorszám |

## 5. Hozzáférési utak

| Út | Hol | Melyik munkaterületért | Hogyan jut el a beállítás |
| --- | --- | --- | --- |
| REST, Bearer vagy süti | `httpx.RequireAuth` | a hívóé | függvény → `WithWorkspace(ctx, ws)` → a tárak a `r.Context()`-et kapják. Többbérlős módban: ha `ws` ≠ az aldomainé → 401 és audit A-ban |
| Belépés, kilépés, munkamenet | `identity/httpapi` | a megtalált actoré | `colony_login_workspaces` (egytagú mód) vagy az aldomain (többbérlős mód), utána `WithWorkspace` |
| `/ws/ui` (böngésző, őr ébresztése) | `gateway.serve` | a hívóé | hello: `colony_event_head` + hitelesítés, utána a kapcsolat teljes élettartamára `WithWorkspace` |
| `/ws/host` (őr) | `gateway/host.go` | a gépé | `colony_host_workspace`, utána `WithWorkspace` |
| `/api/v1/host/*` (őr REST) | `hosts/httpapi`, `preview/httpapi` | a gépé | mint fent |
| `POST /hosts/enroll` | `hosts/httpapi` | a kódé | `colony_pairing_workspace` |
| `/mcp` | `mcpserver.Handler` | a hívóé | a hitelesítő burkoló a kérés ctx-ét szűkíti, mielőtt a go-sdk megkapja. **Ellenőrizni:** az eszközkezelők a kérés ctx-ét kapják-e, vagy a munkamenetét (ha a munkamenetét, akkor azt is az első kérés ctx-e adja; ha `Background`, a teszt elbukik, és az eszközregisztrációnál kell szűkíteni) |
| Gitea-webhook | `git/httpapi` | minden érintett munkaterület | `colony_webhook_seen`, `colony_repo_workspaces`, utána munkaterületenként `WithWorkspace` |
| Fájlletöltés, fájlböngésző-relé | `files/httpapi`, `hosts/fsrelay` | a hívóé | a `RequireAuth` ctx-e |
| Eseményfogyasztók (8: delivery, audit, approval, git.unfurl, preview, lease, compaction, delegation) | `eventlog.Consumer` | **mind** | az eseménynapló olvasása és az offset írása rendszermódban; minden esemény kezelése `WithWorkspace(ctx, ev.WorkspaceID)` alatt |
| Ütemező (`schedule.Loop`), kérdések lejárata (`question.Loop`), naptárfrissítés (`planner.Loop`) | `internal/server.Run` | mind | a `Run` rendszermódú ctx-et ad; a ciklus a „mi esedékes” lekérdezés után tételenként szűkít. **Kódváltozás:** a három ciklusba egy-egy `WithWorkspace` |
| Fájlok takarítása (`Files.SweepPending`) | `internal/server.Run` | mind | rendszermódban (a függő feltöltések munkaterület nélkül is törölhetők) |
| `colonyhall bootstrap` | `cmd/colonyhall` | új munkaterület | rendszermód (új munkaterületet hoz létre) |
| `colonyhall set-password` | `cmd/colonyhall` | a név munkaterülete | `colony_login_workspaces`, utána `WithWorkspace` |
| Migrációk | `migrations.Up` | — | a feltöltő `UPDATE`-ek előtt `SET LOCAL colony.system = 'on'` a migráció elején (a `FORCE` a tulajdonosra is érvényes) |
| Fejlesztői seed (`SeedDev`) | `internal/server` | a dev munkaterület | rendszermód a létrehozáshoz, utána szűkít |

A `context.Background()` a kérést kiszolgáló kódban csak a WebSocket-írás időkorlátjához szerepel (`gateway.go`, `host.go`), adatbázis-hívás nélkül. Kérésből indított gorutin nincs; háttér-gorutint csak a `server.Run` és a `main` indít.

## 6. Adatbázis-szerepkörök

- Minden tábla `ENABLE` **és** `FORCE ROW LEVEL SECURITY`: az alkalmazás felhasználója a táblák tulajdonosa (ő futtatja a migrációkat), és a tulajdonosra az RLS csak `FORCE`-szal érvényes.
- **A superuser és a `BYPASSRLS` szerepkör a `FORCE`-t is megkerüli.** A fejlesztői stack, a CI és a telepítő ma `POSTGRES_USER=colony`-val fut, ami superuser: ott az RLS nem hatna.
  - **Indításkor ellenőrzés:** a szerver lekérdezi a saját szerepkörét (`rolsuper`, `rolbypassrls`). Többbérlős módban ilyen szerepkörrel **nem indul el**; egytagú módban figyelmeztet a naplóban.
  - **Tesztek:** a `make test-pg` és a CI `go-pg` jobja létrehoz egy `colony_app` szerepkört (`NOSUPERUSER NOBYPASSRLS`), átadja neki a teszt-adatbázis tulajdonjogát, és azzal csatlakozik.
  - **Telepítő és dev stack:** két szerepkör az új telepítéseknél (12/3).
- **A homelab** (`colonyhall_app`, sima `LOGIN` szerepkör, az adatbázis tulajdonosa): az alapértelmezés `NOSUPERUSER NOBYPASSRLS`, tehát nem kell változtatni. Élesítés előtt csak ellenőrizni (a superuserrel, a `SHARED-POSTGRES.md` receptje szerint):

```sql
SELECT rolname, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'colonyhall_app';
-- elvárt: f | f. Ha bármelyik t:
ALTER ROLE colonyhall_app NOSUPERUSER NOBYPASSRLS;
```

## 7. Aldomain

- **Egy köztes réteg** az `internal/httpserver`-ben, az útvonalválasztó előtt, minden kérésre (REST, `/ws/ui`, `/ws/host`, `/mcp`, fájlok, webhookok, `/app/` is). Csak `COLONY_MULTITENANT=1` mellett fut; egytagú módban az egész kimarad.
- `Host` → port levágása → `<slug>.<COLONY_BASE_DOMAIN>` → `colony_workspace_by_slug`. A munkaterület a ctx-be kerül (`tenant.FromContext`).
- **Ismeretlen slug, felfüggesztett munkaterület, a csupasz alapdomain, más domain:** mindegyik ugyanaz a válasz (`404`, ugyanaz a törzs, ugyanaz az időzítés nagyságrendje), ami nem árulja el, hogy a slug létezik-e. Az `admin.<domain>` az 1. fázisé; addig ez is ez a válasz.
- **A Gitea-webhook kivétel:** több munkaterületet is érinthet, ezért az alapdomainen (`<domain>/api/v1/hooks/gitea`) is fogadjuk, és a 4. pont függvényeivel oszlik szét. Ügyfelenkénti saját Gitea: 12/4.
- **Token és munkamenet:** a hitelesítés a 4. pont függvényével megtalálja a hitelesítő adat munkaterületét. Ha ez nem az aldomainé: `401` (ugyanaz a válasz, mint egy ismeretlen tokennél), `ERROR` napló, `auth.cross_tenant` bejegyzés **A** auditjában, és a hitelesítő adat visszavonása. A válasz és a teendők: 12/5.
- **Süti:** `Domain` attribútum nélkül (csak az adott aldomainre szól); HTTPS-en (`COLONY_SECURE_COOKIES=1`) `__Host-colony_session` névvel. Egytagú módban a név marad.
- **Őr, MCP, WebSocket:** az aldomaint használják (`https://acme.<domain>/ws/host`, `/mcp`); a köztes réteg ugyanúgy feloldja. Az őrnek nem kell új beállítás: a szerver címe az aldomaines cím.
- **PWA:** semmi nem látszik másképp; a belépés többbérlős módban nem kéri a munkaterületet (a `GET /auth/config`, vagy a meglévő belépési válasz jelzi; a tesztben derül ki, melyik az egyszerűbb).
- **Helyi tesztelés:** `acme.localhost:8080`, `beta.localhost:8080` (a böngészők a `*.localhost`-ot a gépre irányítják); `COLONY_BASE_DOMAIN=localhost`. Leírás a `docs/16-telepites.md`-be.

## 8. A slug

- Szabály: `^[a-z0-9]([a-z0-9-]{1,38}[a-z0-9])$` (3–40 karakter, kisbetű, számjegy, kötőjel; nem kezdődik és nem végződik kötőjellel; nincs két kötőjel egymás után).
- Foglalt: `admin`, `api`, `app`, `www`, `mail`, `smtp`, `imap`, `pop`, `ftp`, `ns1`, `ns2`, `static`, `assets`, `cdn`, `status`, `docs`, `help`, `support`, `billing`, `auth`, `login`, `sso`, `id`, `operator`, `root`, `system`, `colonyhall`, `localhost`, `test`, `dev`.
- A migráció a kiterjeszt–feltölt–szűkít mintát követi: `slug` oszlop (nullable) → feltöltés (minden munkaterület a nevéből képzett slugot kap; ha az érvénytelen, foglalt vagy ütközik, `workspace-<n>`-t) → `NOT NULL` és egyedi index. A `state` alapértéke `active`.
- A migráció nem ír be konkrét slugot (minden önálló telepítésen ugyanaz futna le); egy meglévő munkaterület slugját a `colonyhall set-slug -workspace <név> -slug <slug>` parancs állítja át (12/1).
- `bootstrap --slug`: ha nincs megadva, a névből képzett slug.

## 9. Tesztek

- **Storetest Postgresen, RLS-sel:** a `make test-pg` és a `go-pg` job a `colony_app` (nem tulajdonos, `NOBYPASSRLS`) szerepkörrel csatlakozik. A storetest esetei ma `context.Background()`-ot használnak: mindegyik a fixture munkaterületére szűkített ctx-et kap (`db.WithWorkspace(ctx, f.ws.ID)`), a több munkaterületes esetek esetenként. Egy elfelejtett hatókör így elbukik, nem csendben megy át. Egy külön eset ellenőrzi, hogy **hatókör nélkül semmi** nem látszik és semmi nem írható.
- **Ügyfélszétválasztási készlet** (`internal/e2e/tenant_test.go`): két munkaterület (`acme`, `beta`) a promptban felsorolt adatokkal; a 171 REST-művelet, a 30 MCP-eszköz, a `/ws/ui` és a `/ws/host` `beta` azonosítóival `acme` objektumaira, és `acme` aldomainjén. Egy táblavezérelt teszt; egy ellenőrzés elbukik, ha az `api/openapi.yaml` egy `operationId`-ja vagy egy regisztrált MCP-eszköz nincs a táblában. Memóriatárral és Postgresszel (RLS) is lefut.

## 10. Kockázatok

1. **Elfelejtett hatókör egy ritka úton** (pl. egy hibaág, ami `context.Background()`-dal hív): éles üzemben „semmi nem látszik” hibaként jelentkezik, nem szivárgásként. A storetest RLS-sel és a szétválasztási készlet a legtöbbet elkapja; a maradékra egy napló-figyelmeztetés: ha a pool hatókör nélkül ad ki kapcsolatot, `WARN` a hívó helyével (csak fejlesztői módban, hogy ne legyen zajos).
2. **A go-sdk MCP-munkamenet ctx-e** (5. pont): ha a munkamenet `Background`-ból indul, az eszközregisztrációnál kell szűkíteni.
3. **A `/ws/ui` hello előtti `Head`** ma hatókör nélkül fut; RLS-sel 0-t adna, és a kliens a napló elejéről kapná az eseményeket. Ezért kell a `colony_event_head()`.
4. **Superuserrel futó telepítések** (6. pont): ott az RLS csendben nem hat. Az indítási ellenőrzés figyelmeztet.
5. **Teljesítmény:** kapcsolatkiadásonként +1 körút, és a szabályok kifejezése minden lekérdezésben. A jelentés méri; ha zavaró, a `PrepareConn` gyorsítótárazhat (ha a kapcsolat beállítása már ugyanaz, nem küld semmit).
6. **Nem összeolvasztott ágak** (RAG, 50. kör): az új tábláiknak szabály és tesztsor kell; a `CLAUDE.md` új szabálya ezt mindenkinek előírja.
7. **Migrációs sorszám:** az `00048` a RAG-é, az `00049`–`00050` az 50. köré; ez a kör `00051`-től számoz.

## 11. A commitok sorrendje

1. `docs/23-elkulonites.md` (ez a dokumentum, a válaszokkal kiegészítve).
2. Slug és állapot (migráció `00051`), `COLONY_MULTITENANT`, `COLONY_BASE_DOMAIN`, `bootstrap --slug`, a slug szabályai és tesztjei.
3. Hatókör a `context`-ben, a pool `PrepareConn`-ja, a `system` csomag és a depguard-szabály; a hitelesítés, a gateway, az MCP, az őr, a webhook és a háttérciklusok szűkítése (a viselkedés még változatlan, RLS nélkül).
4. A denormalizált oszlopok (`00052`): kiterjeszt, feltölt, szűkít; összetett idegen kulcsok (12/2).
5. RLS-függvények, szabályok, `FORCE` (`00053`), a `SECURITY DEFINER` függvények; a storetest hatókörrel és a `colony_app` szerepkörrel; az indítási szerepkör-ellenőrzés.
6. Aldomain-köztes réteg, süti, a keresztbérlős `401` és audit; `docs/16` helyi tesztelés.
7. Az ügyfélszétválasztási készlet és a lefedettség-ellenőrzés.
8. Dokumentáció: `docs/02` „Többbérlős működés”, `docs/22` 0. fázis kész, `CLAUDE.md` új szabály; a jelentés RLS-mérése.

## 12. Döntések a kapunál (Norbi, 2026-11-11)

1. **A meglévő munkaterület slugja:** Norbi választotta (a karbantartó privát jegyzeteiben); a telepítés után a `colonyhall set-slug` parancs állítja be. Egytagú módban nem használt.
2. **Összetett idegen kulcsok: igen.** A szülőkön `UNIQUE (id, workspace_id)`, a denormalizált gyerekeken `(…_id, workspace_id) → szülő (id, workspace_id)`.
3. **Superuser:** az **új** telepítések, a dev stack és a CI két szerepkört hoznak létre (karbantartó superuser és egy `NOSUPERUSER NOBYPASSRLS` alkalmazás-szerepkör, amivel a szerver csatlakozik). A Postgres init-szkriptje csak üres köteten fut, így a meglévő telepítések nem változnak; ők indításkor figyelmeztetést kapnak. Többbérlős módban superuserrel vagy `BYPASSRLS`-szel a szerver nem indul el.
4. **Gitea ügyfelenként, kapcsolhatóan:** alapból a közös Gitea hookja a csupasz alapdomainre jön, és a szerver osztja szét a munkaterületek között. Egy munkaterület kaphat saját Giteát (cím, token, webhook-titok); ha be van kapcsolva, a hookja az ő aldomainjére jön, csak az ő titkával fogadjuk, és a közös hook őt nem éri el. **Ebben a körben:** tábla, útválasztás, kapcsoló, tesztek és egy parancssori beállító (`colonyhall gitea-set`); a felület és a token titkosított tárolása a titoktárral jön (22. terv, 5. fázis). Addig a token a szerver adatbázisában van, mint a közös Gitea tokenje a környezeti változóban: csak olvasási jogú tokent szabad megadni.
5. **Másik ügyfél tokenje vagy munkamenete:** kifelé ugyanaz a `401`, mint egy ismeretlen tokennél (nem árulja el, hogy a token valódi). Befelé: `ERROR` szintű `SECURITY: cross-tenant credential use` napló (előtag, a két munkaterület, IP, User-Agent; a titok soha), bejegyzés a token munkaterületének auditjában, és **a token (vagy munkamenet) azonnali visszavonása**, mert kiszivárgott.
6. **Mechanizmus:** a munkaterület kapcsolatkiadáskor kerül a kapcsolatra (2.2 A).
