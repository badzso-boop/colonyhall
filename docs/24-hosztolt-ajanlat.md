# A hosztolt ajánlat: operátor, csomagok, titkosított kulcsok, projektek (51. kör): terv

> Állapot: **elfogadott terv, 2026-11-15** (a kapu döntései a 15. pontban). A 22. dokumentum (`22-tobbberlos-terv.md`) 1. fázisa (operátor) és 3. fázisának tárhelyrésze (csomagok), az 54. kör tárhelyére építve. Az 51. kör az **51a** és **51b** részből áll; a titkok és a projektek (2.3, 9. pont) a hosztolt őrrel jönnek.

## 0. Ami már eldőlt (2026-11-15)

| # | Kérdés | Döntés |
| --- | --- | --- |
| 1 | Operátori belépés | Jelszó + TOTP tartalék kódokkal, az első naptól. Vészhozzáférés (csak olvasás) **ebben a körben** |
| 2 | Csomagok | **Az operátor szerkeszti őket egy táblában**, nem a kódban vannak. Egyelőre csak a tárhelyet korlátozzák; a CPU-magot és a memóriát tárolják és mutatják, kikényszeríteni majd a hosztolt őr fogja |
| 3 | Hosztolt őr | **Később.** Az elkülönítés módja (Linux-felhasználó vagy konténer, lemezkvóta, hálózat) nyitott; itt csak a követelmények és a lehetőségek (11. pont) |
| 4 | AI a hosztolt őrön | Csak API-kulcs, titkosítva tárolva, csak az adott munkaterület őre kapja meg; soha nem kerül chatbe vagy naplóba |
| 5 | Projektek | GitHub-cím + kulcs (deploy key vagy hozzáférési token), titkosítva, soha nem látszik újra egészben; a tulajdonos és az adminok vehetnek fel és törölhetnek |
| 6 | A megvalósítás bontása | Két rész, mindegyik külön commitsorozat, mindegyik után `make ci`, jelentés és megállás átnézésre: **51a** operátor, **51b** vészhozzáférés és csomagok. A kulcsok és a projektek a hosztolt őrrel jönnek (15. pont, 1.) |

## 1. Áttekintés

```
admin.<domain>  ──  operátori felület (PWA, operátori süti)  ──  /api/v1/operator/*
       │ ügyfél létrehozása → meghívólink (7 nap)
       ▼
acme.<domain>   ──  ügyfél-PWA (mint ma)  ──  /api/v1/*  (+ /invites/{token})
       ▲
       │ vészhozzáférés (51b): egyszer használatos átadó link → csak olvasó munkamenet
```

- **Az operátor nem `actor`.** Külön modul (`internal/modules/operator`), külön táblák, külön süti, külön cím. Egy operátori munkamenet a munkaterületek tábláit az adatbázisban sem éri el (3. pont): csak a tartalom nélküli áttekintést kapja egy szűk SQL-függvényen át.
- **Az operátor csak többbérlős módban létezik** (`COLONY_MULTITENANT=1`). Az önálló telepítésen az `/api/v1/operator/*` nem létezik (404), a `bootstrap` és minden más változatlan.
- **Ügyféltartalmat az operátor csak a vészhozzáféréssel lát** (51b), ami ugyanazt az ügyfél-PWA-t nyitja meg, adatbázis-szinten csak olvasható módban.

## 2. Adatmodell

Új migrációk a megvalósítás kezdetekor szabad sorszámtól (ma: `00068`). Az ügyféltáblák a 10. szabály szerint `workspace_id`-t, `ENABLE` + `FORCE ROW LEVEL SECURITY`-t és `tenant` szabályt kapnak. A **platformtáblák** (operátor, csomag) indokolt kivételek: nincs `workspace_id`-jük, és új, **operátori hatókörű** szabályt kapnak (3. pont).

### 2.1 Operátor (51a)

| Tábla | Oszlopok | Szabály |
| --- | --- | --- |
| `operator` | `id`, `handle` (egyedi), `display_name`, `password_hash` (argon2id, mint az `identity`-ben), `totp_secret` (titkosítva, 5. pont), `totp_key_id`, `totp_last_step` (visszajátszás ellen), `state` (`active`, `disabled`), `created_at`, `last_login_at` | **O** |
| `operator_backup_code` | `operator_id`, `code_hash` (SHA-256; a kód 10 véletlen karakter, elég az entrópia), `used_at` | **O** |
| `operator_session` | `token_hash`, `operator_id`, `created_at`, `last_seen_at`, `expires_at`, `remote_addr`, `user_agent`, `revoked_at` | **O** |
| `operator_audit` | `id`, `operator_id`, `action`, `workspace_id` (ha érinti), `payload` (tartalom nélkül), `remote_addr`, `created_at`; csak hozzáfűzés (4. szabály) | **O** |
| `workspace_invite` | `id`, `workspace_id`, `token_hash`, `role` (`owner`), `created_by_operator`, `created_at`, `expires_at`, `used_at`, `used_by` (`actor`), `revoked_at` | **W** + **O** (csak a saját oszlopaihoz kell) |

A `workspace` bővítése: `suspended_at`, `suspended_reason` (a `state` oszlop a 45. kör óta megvan: `active`, `suspended`, `deleting`).

A belépési próbálkozások a meglévő korlátozót használják (40. kör) új hatókörrel (`operator_login`, kezenként 5, IP-nként 20 / 15 perc), munkaterület nélkül.

### 2.2 Csomag és vészhozzáférés (51b)

| Tábla | Oszlopok | Szabály |
| --- | --- | --- |
| `plan` | `id`, `name`, `storage_bytes` (> 0), `cpu_cores` (numeric, pl. 2), `memory_bytes`, `archived_at`, `created_at`, `updated_at` | **O** írás; olvasás: **O**, vagy a munkaterület a **saját** csomagját (`id = (SELECT plan_id FROM workspace WHERE id = colony_ws())`) |
| `break_glass` | `id`, `workspace_id`, `operator_id`, `reason`, `opened_at`, `expires_at` (≤ 4 óra), `closed_at`, `closed_by` (`operator`, `expired`, `owner`), `dev_mode` (bool) | **W** + **O** |

A `workspace` bővítése: `plan_id` (NULL = nincs csomag: csak a tulajdonos 54. körös kerete él, mint ma). *(Megvalósításkor módosítva, `00070`: a csomag a `workspace_storage` sorába kerül — `plan_id`, `plan_name`, `plan_storage_bytes`, `plan_cpu_cores`, `plan_memory_bytes` másolatként —, így a `files` modul a munkaterületen belül, a platformtábla olvasása nélkül számolja a tényleges keretet; a `plan` tábla csak operátori RLS-t kap. A csomag szerkesztése a rajta lévő munkaterületek másolatát is frissíti. A `break_glass` sor `actor_id`-t, átadó-hash-t és lejáratot is tárol, a `session` pedig `break_glass_id`-t.)*

### 2.3 Titkok és projektek (a hosztolt őrrel)

| Tábla | Oszlopok | Szabály |
| --- | --- | --- |
| `workspace_secret` | `id`, `workspace_id`, `kind` (`ai_api_key`, `repo_deploy_key`, `repo_token`), `name`, `provider` (pl. `anthropic`, `openai_compatible`), `ciphertext`, `nonce`, `key_id`, `hint` (az utolsó 4 karakter vagy a nyilvános kulcs ujjlenyomata), `created_by`, `created_at`, `rotated_at`, `deleted_at` | **W** |
| `workspace_project` | `id`, `workspace_id`, `name`, `repo_url` (csak `https://github.com/<owner>/<repo>` vagy `git@github.com:…`), `default_branch`, `secret_id` → `workspace_secret`, `state` (`pending`, `ready`, `error`), `last_error_id`, `disk_bytes` (a hosztolt őr jelenti), `created_by`, `created_at`, `deleted_at` | **W** |

A `workspace_storage` majd a hosztolt őrrel kap egy `projects_bytes` oszlopot (8. pont).

## 3. Az operátori hatókör az adatbázisban

A 45. kör hatóköre (`colony.workspace_id`, `colony.system`) mellé egy harmadik jön: **`colony.operator`**.

```sql
CREATE FUNCTION colony_operator() RETURNS boolean LANGUAGE sql STABLE
  AS $$ SELECT coalesce(current_setting('colony.operator', true), '') = 'on' $$;
-- Platformtábla (O):
CREATE POLICY operator ON operator USING (colony_operator() OR colony_system())
  WITH CHECK (colony_operator() OR colony_system());
```

- `db.WithOperator(ctx)` állítja be (és a munkaterületet meg a rendszermódot kikapcsolja). Csak az `operator` modul `httpapi`-ja és a `cmd/colonyhall operator-*` parancsok hívhatják (depguard-szabály, mint a `system-mode-is-background-only`).
- **Operátori hatókörben a munkaterületi táblák üresek** (a `tenant` szabály nem ismeri az operátort). Programhiba esetén sem szivárog tartalom az operátori felületre.
- Az áttekintő számait **a `colony_operator_overview(only_ws, online_since)` függvény** adja (`00069`), az `operator.Overview` porton át (`operator/postgres`): csak `colony_operator()` mellett fut (különben `42501` hiba), a rendszermódot csak a saját lekérdezése idejére kapcsolja be és utána visszaállítja (a `00053` keresőfüggvényeinek mintája), és **csak fiókadatot és számokat** ad vissza (név, slug, állapot, tulajdonos/meghívó, felhasználók, agentek, gépek, tárhely, utolsó aktivitás). Az operátori kérésút tehát **sem rendszermódot, sem munkaterületi hatókört nem kapcsol** olvasáshoz: a `internal/server/customers.go` csak ezt a portot kapja, a modulok tárait nem, és a depguard (`operator-view-stays-in-operator-scope`) kizárja belőle a `db/system`-et. Az írások (ügyfél létrehozása, meghívó, felfüggesztés) az `identity` modul szűk metódusain át, mindig abban az egy munkaterületben futnak, amelyet módosítanak. A memóriás tárban (nincs RLS) egy külön adapter (`customers_memory.go`) ugyanezekkel a szabályokkal számol; Postgresen soha nem fut. Teszt: `TestPostgresRowLevelSecurity` (operátori hatókörben a munkaterületi táblák üresek, az áttekintő mégis helyes; munkaterületi, hatókör nélküli és rendszermódú hívás hibát kap; a hívás után nem marad rendszermód a kapcsolaton). *(Az 51a első változata munkaterületenként teljes hatókörbe lépett és rendszermódban listázott; az átnézés után javítva, 2026-11-15.)*
- A munkamenet keresése nem kér külön függvényt: az operátori szolgáltatás minden hívása eleve operátori hatókörben fut (az `admin.` hoston más nem érhető el), és a munkamenet-tábla ott látszik. *(Megvalósításkor módosítva.)*

### 3.1 Az `InTx` hatókör-csapdája (51a)

Ma, ha egy futó tranzakción belül `db.WithWorkspace` más hatókört kér, a `InTx` a külső tranzakcióhoz csatlakozik, és a kapcsolat a **külső** hatókörrel fut tovább (rendszermódból szűkítésnél a szűkítés csendben elmarad). Javítás: a tranzakció a kontextusban megjegyzi a hatókörét; a csatlakozó `InTx` összeveti a `db.Scope(ctx)`-szel, és eltérésnél hibát ad (`db: nested transaction with a different scope`). A memóriás `InTx` ugyanígy, hogy a két adapter egyformán viselkedjen. Teszt: rendszermódú tranzakcióban `WithWorkspace` → hiba; azonos hatókör → csatlakozik; a meglévő tesztek zöldek (ma nincs ilyen hívás). **Ismert korlát (az 51b átnézése, 2026-11-15):** az ellenőrzés csak a csatlakozó `InTx`-ben fut; egy futó tranzakción belüli, saját `InTx` nélküli tárhívás (`Postgres.Q`) a külső tranzakció hatókörével fut, akármit kér a kontextus. Ma nincs ilyen hívás; aki tranzakción belül hatókört vált, saját `InTx`-et nyisson (az hibát ad), vagy a `Q` kapja meg ugyanezt az ellenőrzést egy későbbi körben.

## 4. Az operátor (51a)

### 4.1 Fiók, belépés, munkamenet

- **Az első (és ebben a körben minden) operátor parancssorból:** `colonyhall operator-create -handle ops` (terminál nélkül, pl. `docker compose exec -T`, az első kód külön lépésben: `colonyhall operator-confirm -handle ops -code …`). A jelszó a stdin-ről jön; a parancs kiírja a TOTP-titkot és az `otpauth://` címet (a hitelesítő alkalmazásba), bekéri az első kódot, és csak helyes kód után aktiválja a fiókot; végül egyszer kiírja a 10 tartalék kódot. `operator-reset-totp` és `operator-disable` ugyanígy, parancssorból (a géphez fizikai hozzáférés a végső helyreállítás).
- **Belépés egy lépésben:** `POST /api/v1/operator/auth/login {handle, password, code}`, ahol `code` a 6 jegyű TOTP vagy egy tartalék kód. Nincs félig belépett állapot, amit külön kellene kezelni. Hibánál egy egységes „hibás adatok” válasz (nem árulja el, melyik rész volt rossz), a korlátozó számol.
- **TOTP:** RFC 6238, SHA-1, 6 jegy, 30 mp, ±1 lépés eltérés; egy lépés kódja csak egyszer fogadható el (`totp_last_step`). Saját, kb. 40 soros megvalósítás a standard könyvtárral, az RFC tesztvektoraival (nincs új függőség). A tartalék kód egyszer használható; elhasználásakor `operator_audit` bejegyzés, és a felület mutatja, hány maradt.
- **Munkamenet:** `__Host-colony_operator` süti (HTTPS-en; HTTP-n `colony_operator`), csak az `admin.<domain>` címre, `SameSite=Strict`, `HttpOnly`. **8 óra abszolút, 30 perc tétlenség** után lejár. CSRF ugyanúgy, mint az ügyfél-PWA-ban (`X-CSRF-Token`). Az ügyfél sütije az admin címen, az operátoré az ügyfél címén nem érvényes (más név, más host).
- **Útválasztás:** a `tenantHandler` az `admin.<base>` hostot külön ágra viszi: ott csak az operátori API, a PWA és az egészségvégpontok élnek; minden ügyfél-API 404. Az `admin` slug már foglalt (`ReservedSlugs`).
- **Engedélylista (2026-11-15, átnézés után):** a belépési korlátozó handle-enként számol, így aki ismeri az egyetlen operátor nevét, kívülről folyamatosan kizárhatná. A `COLONY_OPERATOR_ALLOWLIST` (IP-k vagy CIDR-ek vesszővel) dönti el, honnan érhető el egyáltalán az `admin.<base>`: minden más cím ugyanazt a 404-et kapja, mint egy ismeretlen host, még a belépés előtt. Üresen bárhonnan (nem ajánlott). A telepítő többbérlős módban rákérdez (Tailscale esetén `100.64.0.0/10` az alapértelmezés), és újrafuttatáskor megtartja. Teszt: `TestOperatorConsoleAnswersOnlyTheAllowlist` (kívülről 404 és tíz hibás próba után is be lehet lépni az engedélyezett címről), `TestServerOperatorAllowlist`.

### 4.2 Ügyfél létrehozása meghívóval

- `POST /operator/workspaces {name, slug, plan_id?}` → a munkaterület (`active`, tulajdonos nélkül) + egy meghívó: `{invite_url: "https://acme.<domain>/app/invite#<token>", expires_at}`. A token 32 bájt véletlen, csak a hash-e tárolódik; a link **egyszer** látszik (az operátor adja át az ügyfélnek; e-mail még nincs). A token a `#` után van, így nem kerül a szervernaplókba és a `Referer`-be.
- `POST /operator/workspaces/{id}/invite`: új link, a régi visszavonva (ha az első elveszett vagy lejárt). Csak amíg a munkaterületnek nincs tulajdonosa.
- Az ügyfél oldalán (`acme.<domain>`): `POST /api/v1/invites/lookup {token}` → `{workspace_name, expires_at}`; `POST /api/v1/invites/accept {token, handle, display_name, password}` → létrejön az `owner`, belép (munkamenet-süti), a meghívó `used_at`-et kap. A token a törzsben utazik, útvonalban soha (hozzáférési naplók). Más aldomainen, lejárt, felhasznált vagy visszavont token egységesen 410. *(Megvalósításkor módosítva: a token 256 bites véletlen, ezért a meghívókra nem került korlátozó.)*
- A `colonyhall bootstrap` megmarad az önálló telepítésnek; többbérlős módban új munkaterület csak az operátortól jön.

### 4.3 Felfüggesztés és visszaállítás

- `POST /operator/workspaces/{id}/suspend {reason}`, `POST …/restore`. Az indoklás csak az operátori naplóba és a `workspace.suspended_reason`-be kerül.
- Felfüggesztve az API, az MCP, a WebSocket (`/ws/ui`, `/ws/host`), az őr (`/api/v1/host/*`, párosítás) ugyanazt a 404-et kapja, mint egy ismeretlen aldomain (a 45. kör `tenantHandler`-e ezt ma is tudja). **A böngésző** (HTML-t kérő `GET`, pl. `/app/…`) egy „A munkaterület szünetel, keresd az üzemeltetőt” oldalt kap (magyarul és angolul, az `Accept-Language` szerint; a szerver beágyazott, statikus oldala, mert a PWA API-hívásai ilyenkor 404-et kapnak). Az indoklás nem jelenik meg rajta. Új: a felfüggesztés eseménye (`workspace.suspended`) **lezárja a nyitott WebSocket-kapcsolatokat** (böngészők és őrök), mert azok a kérésenkénti ellenőrzést már nem futják újra.
- Az ütemezett üzenetek várnak (az ütemező a felfüggesztett munkaterületeket kihagyja, így azok sorai a többit sem tartják fel). Az eseményfogyasztók a már bent lévő eseményeket feldolgozzák (az adat konzisztens marad); agenthez semmi nem jut el, mert az őr minden kérése elutasítva. A kérdések lejárata és a naptárfrissítés fut tovább (ártalmatlan, ügyfélnek nem látszik). Az adat megmarad, a visszaállítás után minden folytatódik. *(Megvalósításkor szűkítve: csak az ütemező hagyja ki.)* A böngésző oldala 403-as státusszal jön.
- Az ügyfél gépén futó őr a 404-re leállítja az új futásokat (mint a visszavont géptokennél); a már futó futás befejeződik, de nem tud írni.

### 4.4 Fejlesztői mód

- `COLONY_OPERATOR_DEV_ACCESS=1`: a vészhozzáférés (51b) indoklás és 4 órás korlát nélkül nyitható (legfeljebb 12 óra), és a felület **minden oldalon** kiírja: „Fejlesztői mód: az operátor minden adatot lát”. Induláskor figyelmeztetés a naplóban.
- **Élesben a szerver nem indul el vele.** Javaslat (15. pont, 3. kérdés): a kapcsoló csak `COLONY_ENV=development` mellett engedett, tehát hiányzó `COLONY_ENV` esetén is elutasítva; a telepítő `COLONY_ENV=production`-t ír.
- 51a-ban a kapcsoló, az indítási tiltás és a kiírás készül el; maga a megnyitás a vészhozzáféréssel (51b) jön, mert ugyanaz az út.

### 4.5 Az operátori áttekintés

- `GET /operator/workspaces`: munkaterületenként név, slug, állapot, csomag, tárhely (használat, keret, szint; 54. kör), felhasználók, agentek, gépek (ebből online), utolsó aktivitás (az utolsó esemény ideje), létrehozás. **Tartalom nincs** (csatornanév, üzenet, fájlnév, felhasználónév sem).
- Ide kerül a Rendszerállapot adatbázis-kártyája (50. kör): többbérlős módban az ügyféladmin nem látja, csak az operátor (`GET /operator/system/database`). Önálló telepítésen marad a tulajdonosnál.
- `GET /operator/audit`: az operátori napló.

## 5. Titkosítás és kulcskezelés

Már 51a-ban kell (a TOTP-titkokhoz), és ugyanez szolgálja majd a 2.3 titkait. Új csomag: `internal/platform/secretbox`.

- **Kulcs:** `COLONY_SECRET_KEYS` (vagy `COLONY_SECRET_KEYS_FILE`, Docker secretnek): `k2:<base64 32 bájt>,k1:<…>`; az első az aktív, a többi csak visszafejtésre. A kulcs soha nincs az adatbázisban, a naplóban, a mentésben vagy a repóban. A telepítő generálja (`colony setup`), a `.env`-be vagy a secret-fájlba írja 0600-zal.
- **Algoritmus:** AES-256-GCM, rekordonként új, véletlen 12 bájtos nonce; **AAD = tábla | sor-id | munkaterület-id | fajta**, így egy titkosított érték nem másolható át más sorba vagy más ügyfélhez (a visszafejtés elbukik).
- **Kötelező:** többbérlős módban a szerver nem indul el kulcs nélkül. Önálló telepítésen csak akkor kell, ha titkot tárolnánk (most semmit).
- **Forgatás:** új kulcs elöl a listában → `colonyhall rotate-secrets` minden sort az aktív kulccsal újratitkosít (`key_id` szerint, kötegenként, rendszermódban), utána a régi kulcs törölhető a listából. Egy teszt: forgatás után a régi kulcs nélkül is minden visszafejthető.
- **Ki mit olvashat:**

| Titok | Ki adja meg | Ki látja | Ki fejti vissza |
| --- | --- | --- | --- |
| Operátor TOTP-titka | `operator-create` | senki (a létrehozáskor egyszer) | a belépés ellenőrzése |
| AI API-kulcs | tulajdonos, admin (csak írás) | név, szolgáltató, utolsó 4 karakter, dátum | **csak** a hosztolt őr konfigjának kiszolgálása, csak a munkaterület hosztolt gépének (8. pont) |
| Projektkulcs | tulajdonos, admin (csak írás) | név, ujjlenyomat vagy utolsó 4 karakter | ugyanígy |

- Titok soha nem kerül eseménybe, auditba, naplóba vagy chatbe: az esemény csak az id-t, a nevet és a hintet viszi. A `secretscan` (meglévő) a naplóíróban is ellenőrzi a kulcsmintákat. Hibaüzenet soha nem idézi a titkot.
- **Ha a kulcs elveszik:** az operátorok TOTP-je parancssorból újraállítható, az ügyfelek kulcsait újra meg kell adni. Ezért a kulcsot a mentési útmutatóba külön tételként kell felvenni (az adatbázis-mentés nélküle nem állítja vissza a titkokat, és ez szándékos).

## 6. Vészhozzáférés (51b)

- `POST /operator/workspaces/{id}/break-glass {reason (min. 20 karakter), minutes (≤ 240)}` → `break_glass` sor + egy **60 mp-es, egyszer használatos átadó link** (`https://acme.<domain>/app/break-glass#<token>`). Az ügyfél címén a link egy **csak olvasó munkamenetet** nyit egy munkaterületenkénti rendszer-actorral (`colonyhall-operator`, `EnsureSystemActor`, mint az 54. kör `colonyhall-storage`-e, jelszó és token nélkül).
- **Mit lát (döntés, 2026-11-15):** a nyilvános csatornákat és szálaikat, az agenteket, a gépeket, a futásokat és a beállításokat. **DM-eket és privát csatornákat nem**: sem az üzeneteiket, sem a fájljaikat, sem a keresési találataikat. A munkamenet a jogosultság-ellenőrzésben külön jelölést kap (nem elég a szerepkör, mert az admin a privát csatornákat is látja). Teszt: vészhozzáférési munkamenettel egy DM és egy privát csatorna üzenetei, fájljai és keresési találatai nem érhetők el. Ha később kell, bővítjük.
- **Csak olvasás, két rétegben:**
  1. a köztes réteg minden nem `GET`/`HEAD` kérést (a kilépés kivételével) és minden WebSocket-küldést 403-mal utasít el (`type: read_only_access`);
  2. a munkamenet kontextusa `db.ReadOnly`-t kap, és a pool kapcsolatkiadáskor `default_transaction_read_only = on`-t állít: ha egy `GET` mégis írna (pl. olvasottság), a Postgres utasítja el.
  Teszt: a szerződés (`api/openapi.yaml`) minden írási végpontja lefut vészhozzáférési munkamenettel, és mind 403 (új végpont automatikusan bekerül).
- **Látszik az ügyfélnél:** megnyitáskor az ügyfél `audit_log`-jába (`operator.break_glass_opened`: ki, miért, meddig) és a tulajdonos(ok) értesítései közé (új tevékenységfajta, `operator_access`, 55. kör). **Minden olvasás** (a kérés útvonala és ideje, tartalom nélkül) az ügyfél auditjába kerül, így a tulajdonos pontosan látja, mit nézett meg az operátor. Zárás (kézi, lejárat) szintén.
- **Magától zárul:** a munkamenet érvényessége a `break_glass.expires_at`; minden kérés újraellenőrzi, hogy a sor nyitott-e; a karbantartó ciklus a lejártakat `expired`-dal zárja, a nyitott WebSocketet bontja. **A tulajdonos is lezárhatja** (`DELETE /workspace/break-glass/{id}`), javaslat: igen (15. pont, 5. kérdés).
- Fejlesztői módban ugyanez indoklás nélkül, a „Fejlesztői mód” sávval; az ügyfél auditjában ilyenkor is megjelenik (`dev_mode: true`).

- **Megvalósítás (2026-11-15):** a kérésréteg (`httpx.RequireAuth`) az egyetlen hely, ami vészhozzáférési munkamenetet elfogad (`identity.AllowBreakGlass`); a WebSocket, az MCP és a bearer token elutasítja. **Engedélylista** (`identity.BreakGlassRoutes`, a regisztrált minta szerint, így egy új útvonal zárva marad, amíg fel nem veszik): a munkamenet, a saját (üres) listák, felhasználók, Rendszerállapot, csatornák, üzenetek, szál, kitűzések, fájlok, gépek, futások, költség, agent-utasítások és szabályok, beállítások, tárhely; a futásoknál a lépések és az olvasott URL-ek kimaradnak. Minden más olvasás 403 `break_glass_scope`, minden írás 403 `read_only_access` (kivéve a kilépést). Az olvasás a kiszolgálás **előtt** kerül az auditba (`operator.break_glass_read`: módszer és útvonal, lekérdezés és tartalom nélkül); ha nem írható, a kérés elbukik. A DM-et és a privát csatornát a `conversation.CanRead` és a csatornalista szűri, így a fájlok, a szál és a kitűzések is. A WebSocketen az „olvasott” események nem mennek ki (személyes eseményként a gateway-ben). Az operátor-actor (`colonyhall-operator`, admin rangú ember) nem kaphat jelszót, tokent vagy más rangot, és tokennel sem hitelesíthet. A lejárat kérésenként érvényes; a karbantartás (óránként) a sort is lezárja. Tesztek: `TestBreakGlass` és `TestPostgresBreakGlass` (a szerződés minden írási és nem engedélyezett olvasási útvonala a munkamenettel), `TestPostgresRowLevelSecurity` (csak olvasható kapcsolat), `storetest` (`break-glass`).

## 7. Csomagok (51b)

- **Az operátor szerkeszti:** `GET/POST /operator/plans`, `PATCH /operator/plans/{id}` (név, tárhely, CPU, memória), archiválás (használatban lévő csomag nem törölhető; az archivált nem rendelhető új ügyfélhez).
- **Hozzárendelés:** `PUT /operator/workspaces/{id}/plan {plan_id, confirm_over_quota}`.
- **A tényleges keret** = a csomag tárhelye, vagy a tulajdonos 54. körös kerete, ha az kisebb (és nem 0). A tulajdonos `PUT /workspace/storage/quota`-ja a csomag fölé 422 (`quota_above_plan`). A figyelmeztetés (80%, 95%) és a teli állapot (feltöltés 507, az őr szünetel) pontosan az 54. kör szerint, a tényleges keretre számolva. Csomag nélkül (`plan_id` NULL) minden a mai.
- **Kisebb csomagra váltás:** a felület előbb lekéri `GET /operator/workspaces/{id}/plan-check?plan_id=…` → `{used_bytes, new_quota_bytes, fits, level_after}`, és mutatja: „Jelenlegi használat 18,2 GB, az új csomag 15 GB: a munkaterület **megtelik**, az agentek szünetelnek, amíg a használat 15 GB alá nem csökken.”
  - **Javaslat: engedni, kifejezett megerősítéssel** (`confirm_over_quota: true`), és a munkaterület „teli” állapotba kerül; a tulajdonos megkapja az 54. kör DM-jét. Indok: a visszautasítás az operátort arra kényszerítené, hogy a drágább csomagon tartsa az ügyfelet, vagy maga töröljön ügyféladatot (amit nem tehet); a teli állapot adatvesztés nélkül, érthetően jelzi a helyzetet, és az ügyfél maga dönt, mit töröl. Megerősítés nélkül a kérés 409 (`plan_over_quota`).
- **CPU és memória:** csak tárolva és mutatva (operátori lista, az ügyfél Beállítások → Tárhely kártyája: „Csomag: Közepes · 20 GB · 4 mag · 4 GB memória”). A hosztolt őr fogja kikényszeríteni (systemd slice vagy konténerkorlát).
- Ügyféloldalon: `GET /workspace/plan` (tulajdonos és adminok). *(Megvalósításkor módosítva: a csomag a `GET /workspace/storage` válaszában jön — `plan` és a tényleges keret, `limit_bytes` —, a Tárhely kártya ezt mutatja; külön végpont nem kell.)*

## 8. A tárhely és a hosztolt őr projektjei (a hosztolt őrrel)

- `workspace_storage.projects_bytes`: a hosztolt őr a gépjelentésben küldi a projektmappák méretét; a `Used()` = fájlok + adatbázis + projektek. Az 54. kör állapotai és szünete változatlanul érvényesek.
- **Kemény korlát** a lemezen is (11. pont), mert a jelentés késhet. Javaslat: a projektek kemény korlátja = a tényleges keret − (fájlok + adatbázis) a kiosztáskor, óránként frissítve, és sosem kisebb a már elfoglaltnál (különben a git félbeszakadt írásokkal sérülne); így a teli állapot a szokásos úton (szünet) áll be, a kemény korlát csak a túlfutást fogja meg.
- Saját gépen futó őrnél a projektek nem számítanak bele (mint ma).

## 9. Projektek és titkok: API és képernyők (a hosztolt őrrel)

- **API (tulajdonos és adminok, emberek):**
  - `GET /workspace/secrets` (csak metaadat), `POST /workspace/secrets {kind, name, provider, value}` (a válaszban nincs `value`), `PUT /workspace/secrets/{id} {value}` (csere = forgatás), `DELETE /workspace/secrets/{id}`.
  - `GET /workspace/projects`, `POST /workspace/projects {name, repo_url, default_branch, secret_id | generate_deploy_key: true}`, `DELETE /workspace/projects/{id}`.
  - Agentnek és MCP-nek nincs ilyen eszköze; a titok értéke semmilyen `GET`-ben nem jön vissza.
- **Javaslat a kulcsra (pontosítás az 5. döntéshez):** alapból **mi generálunk** egy ed25519 kulcspárt projektenként, és a felület a **nyilvános** kulcsot mutatja, amit a tulajdonos deploy keyként felvesz a GitHubon („Allow write access”). Így a titkos kulcsot ember soha nem másolja (nem kerülhet chatbe, vágólapra), és csak egy repóhoz ad hozzáférést. A meglévő tokent (fine-grained PAT, egy repóra korlátozva) megadni a második út, ha a szervezetben a deploy key tiltott.
- **Képernyők (PWA, `features/projects`):** Beállítások → **Projektek** (lista: név, repó, ág, állapot, méret; „Új projekt” párbeszéd két úttal; törlés megerősítéssel) és Beállítások → **AI-kulcsok** (név, szolgáltató, „…a1b2”, dátum; „Csere”, „Törlés”; csak írható mező, a mentés után üres). Mindkettő csak a hosztolt őrrel rendelkező munkaterületen jelenik meg.

## 10. Képernyők (51a, 51b)

- **Operátori PWA az `admin.<domain>`-en:** ugyanaz a csomag (`web/`), új funkciómodul `features/operator`; az `App` a hostnévből dönt (`admin.` → operátori útvonalak és keret, minden más → a mai). Az operátori oldalak **külön, lustán töltött darabban** vannak (`React.lazy` + dinamikus `import()`), így az ügyfelek felülete nem tölti be őket. Oldalak: **Belépés** (név, jelszó, kód egy űrlapon), **Ügyfelek** (táblázat a 4.5 adataival; állapot- és tárhelyjelölés; „Új ügyfél” párbeszéd → a meghívólink másolható, egyszer), **Ügyfél részletei** (adatok, csomag + váltás az ellenőrzéssel, felfüggesztés/visszaállítás indoklással, új meghívó, 51b-től vészhozzáférés indoklással és időtartammal), **Csomagok** (szerkeszthető táblázat), **Rendszer** (az 50. kör adatbázis-kártyája), **Napló**, **Fiókom** (tartalék kódok száma, újragenerálás jelszóval + kóddal).
- **Ügyfél-PWA:** **Meghívó elfogadása** (`/app/invite`: munkaterület neve, név, felhasználónév, jelszó kétszer); a vészhozzáférés alatt felül egy nem zárható sáv („Üzemeltetői hozzáférés, csak olvasás, lejár: 14:30”), a szerkesztők letiltva; a tulajdonosnál értesítés és az auditban a bejegyzések; Beállítások → Tárhely kártyán a csomag.
- Minden szöveg i18n-kulcs (hu, en), a `docs/08` rétegszabályai szerint.

## 11. A hosztolt őr elkülönítése: követelmények és lehetőségek (nyitott, később)

**Követelmények:** egy munkaterület őre és agentjei nem olvashatják, írhatják, érhetik el és meríthetik ki egy másik munkaterület (és a gazdagép) erőforrásait: fájlok, folyamatok, hálózat, CPU, memória, folyamatszám, lemez. Csak a saját munkaterületéhez párosított őr; minden agent sandboxban (`bwrap`, saját worktree, a 18. kör hálózati engedélylistája), teljes hozzáférésű agent nincs; csak internet (nincs LAN, Tailscale, a gazdagép szolgáltatásai, más ügyfél, az üzemeltető privát hálózata); a titkok csak a futás környezetébe kerülnek, lemezre nem (vagy csak a felhasználó 0600-as fájljába).

| Kérdés | Lehetőségek | Megjegyzés |
| --- | --- | --- |
| Elkülönítés | **A)** munkaterületenként Linux-felhasználó + systemd slice, minden agent `bwrap`-ban · **B)** munkaterületenként konténer (rootless Podman/Docker), benne az őr és a `bwrap` | A) egyszerűbb, a CI-runner mintája (`gh-runner`, user-slice, ufw a felhasználóra); B) erősebb határ, könnyebb költöztetni, de `bwrap` konténerben külön jogokat kér |
| Lemezkvóta | **1)** munkaterületenként ext4-képfájl fix mérettel, loop-mountolva · **2)** XFS projektkvóta · **3)** konténerkötet méretkorláttal | 1) bárhol működik, de átméretezés kell csomagváltáskor; 2) a legtisztább, ha a bérelt szerveren XFS lesz; 3) a tárolómeghajtótól függ |
| Hálózat | ufw/nftables szabály a felhasználóra (`owner` match) vagy a konténerhálózatra | Mint a CI-runnernél: kifelé csak internet |
| CPU, memória | systemd slice (`CPUQuota`, `MemoryMax`, `TasksMax`) vagy konténerkorlát | A csomag `cpu_cores` és `memory_bytes` mezőiből |

Ez a bérelt szerver kiválasztása után dől el; a homelab próbája külön lépés.

## 12. Mit épít ez a kör, és mi vár a hosztolt őrre

| Rész | Tartalom | Kész, ha |
| --- | --- | --- |
| **51a** | `secretbox` és kulcskezelés; operátori táblák és hatókör (`colony.operator`, depguard); `operator-create`, `operator-reset-totp`, `operator-disable`; belépés jelszó + TOTP + tartalék kód, munkamenet; `admin.` útválasztás; ügyfél létrehozása meghívóval, meghívó elfogadása; felfüggesztés és visszaállítás (WebSocket-bontással, háttérmunka kihagyásával); `COLONY_ENV` és a fejlesztői mód tiltása élesben, sáv; operátori áttekintés és adatbázis-kártya; operátori napló; az `InTx` hatókör-csapdája; operátori PWA (belépés, ügyfelek, részletek, rendszer, napló, fiókom) és a meghívó oldala | A meghívóval új ügyfél be tud lépni; a 14. pont 51a-tesztjei zöldek; `make ci` |
| **51b** | Vészhozzáférés (átadó link, csak olvasó munkamenet két rétegben, ügyfél-audit minden olvasásról, értesítés, lejárat, tulajdonosi zárás), fejlesztői módú megnyitás; `plan` tábla és operátori szerkesztése, hozzárendelés, tényleges keret, a tulajdonosi keret felső határa, kisebb csomag ellenőrzése és megerősítése; a csomag az ügyfél Beállításaiban | A vészhozzáférés látszik az ügyfél auditjában és értesítései között; a csomag kikényszeríti a tárhelyet; `make ci` |
| Később | **Titkok és projektek** (2.3, 9. pont): a hosztolt őrrel együtt (15. pont, 1.) | — |
| Hosztolt őr (51c) | Elkülönítés (11. pont), az önfejlesztés agentes részei (Építő, agentes rutinok, stdio-s MCP-szerverek; 25. dokumentum), lemezkorlát, `projects_bytes`, a titkok kézbesítése a hosztolt gépnek, klónozás és worktree-k, a CPU/memória kikényszerítése | Később, a szerver kiválasztása után |

## 13. Migráció a meglévő munkaterületekre

- Minden új oszlop kiterjesztő (NULL vagy alapérték), az új táblák üresek. Meglévő munkaterület: `plan_id` NULL (csak a tulajdonosi keret él, mint ma), nincs meghívó, `active`.
- **Önálló telepítés** (`COLONY_MULTITENANT=0`): semmi nem változik; az operátori végpontok és parancsok nem érhetők el; titkosítási kulcs nem kell.
- **Többbérlős telepítés** (a homelab tesztje): a frissítés előtt `COLONY_SECRET_KEYS` kell (a telepítő generálja, `colony upgrade` figyelmeztet, ha hiányzik), utána `colonyhall operator-create`. A meglévő ügyfelek ugyanúgy működnek; csomagot az operátor rendel hozzájuk.

## 14. Tesztelés

- **Egység:** TOTP az RFC 6238 vektoraival, ±1 lépés, visszajátszás; tartalék kód egyszer; `secretbox` (rossz AAD, rossz kulcs, forgatás); `InTx` hatókör-ütközés (memória és Postgres).
- **Tár:** minden új `Store` a `storetest`-ben memóriával és Postgresszel; az új táblák a `TRUNCATE` listában; az `rls_test.go` a platformtáblák operátori szabályát is ellenőrzi, és hogy operátori hatókörben a munkaterületi táblák üresek.
- **Végponttól végpontig (`internal/e2e`, a válaszok a szerződéshez mérve):**
  - ügyféltokennel és ügyfélsütivel az operátori végpontok elérhetetlenek (más hoston 404, az admin hoston 401); operátori sütivel az ügyfél címén semmi nem érhető el; az admin hoston az ügyfél-API 404;
  - belépés rossz jelszóval, rossz kóddal, újrahasznált kóddal, elhasznált tartalék kóddal → egységes hiba; korlátozás; lejárat (abszolút és tétlenség), léptethető órával;
  - meghívó: egyszer, 7 nap, más aldomainen 404, visszavont 410, csak tulajdonos nélküli munkaterületnek;
  - felfüggesztés: REST, MCP, `/ws/ui`, `/ws/host`, `/api/v1/host/*`, párosítás mind elutasítva; a nyitott WebSocket bezárul; visszaállítás után minden működik, az adat megvan;
  - fejlesztői mód + `COLONY_ENV` ≠ `development` → a szerver nem indul;
  - 51b: vészhozzáférés minden írási végpontot elutasít (a szerződésből generált lista), egy író `GET` az adatbázisban bukik; lejárat után 401; az ügyfél auditja és a tulajdonos értesítése; csomag: a tulajdonos nem emelhet a csomag fölé, kisebb csomag megerősítés nélkül 409, vele teli állapot és szünet.
- **Keresztbérlős készlet (`crosstenant_test.go`):** az új paraméterfajták (`invite token`, `break-glass id`, `secret id`, `project id`) egy-egy acme-objektumhoz rendelve.
- **PWA:** Vitest a funkciómodulra; az architektúra-teszt az új `features/operator`-ra is fut; képernyőképek memóriás szerverrel, kitalált adatokkal.
- Minden szabályhoz egy teszt, ami nélküle elbukik.

## 15. A kapu döntései (Norbi, 2026-11-15)

| # | Kérdés | Döntés |
| --- | --- | --- |
| 1 | Titkok és projektek | **A hosztolt őrrel együtt.** A kör 51b-vel kész. A titkosítás alapja (`secretbox`, kulcsforgatás) 51a-ban elkészül a TOTP-titkokhoz |
| 2 | Kisebb csomag a használat alá | Kifejezett megerősítéssel engedve; a munkaterület megtelik, az agentek szünetelnek; megerősítés nélkül 409 (7. pont) |
| 3 | `COLONY_ENV` | A fejlesztői mód csak `COLONY_ENV=development` mellett indul; hiányzó változó = éles; a telepítő `production`-t ír |
| 4 | Mit lát a vészhozzáférés | Nyilvános csatornák, szálak, agentek, gépek, futások, beállítások, csak olvasva; **DM-ek és privát csatornák nélkül** (6. pont), teszttel |
| 5 | A tulajdonos lezárhatja | Igen |
| 6 | Felfüggesztett ügyfél | A böngésző „A munkaterület szünetel, keresd az üzemeltetőt” oldalt kap (hu, en); az API, az MCP, a WebSocket és az őr ugyanazt a 404-et, mint egy ismeretlen aldomain (4.3) |
| 7 | További operátorok | Ebben a körben csak parancssorból |
| 8 | Operátori felület | Ugyanaz a PWA az `admin.<domain>`-en, hostnév szerinti útvonalakkal; az operátori oldalak külön, lustán töltött darabban |
| 9 | Projektkulcs (hosztolt őr) | Alapból általunk generált deploy key, a felület csak a nyilvános kulcsot mutatja; második út egy repóra szűkített token |
| 10 | Lemezkorlát (hosztolt őr) | A tényleges keret mínusz a már elfoglalt egyéb tárhely, óránként frissítve (8. pont) |
| 11 | Ügyfél törlése | Külön körben, **de az első valódi ügyfél előtt** (GDPR) |

## 16. Nyitott tételek

- **51a kész (2026-11-15):** operátor, TOTP, munkamenet, ügyfél létrehozása meghívóval, felfüggesztés, `COLONY_ENV` és a fejlesztői mód tiltása élesben, áttekintő, adatbázis-kártya, operátori napló, `InTx`. Következik: **51b** (vészhozzáférés, csomagok).

- **Ügyfél törlése (az első valódi ügyfél előtt kötelező, GDPR):** a `deleting` állapot megvan; kell hozzá az adatok, a fájlok és a mentésekből való kikerülés folyamata, az export a tulajdonosnak (22. terv 12. pont), és az adatfeldolgozási szerződés szerinti határidők (46. kör).
- **A hosztolt őr elkülönítése** (11. pont): a bérelt szerver kiválasztása után.
- **Titkok és projektek** (2.3, 5., 9. pont): a hosztolt őr körében.
- **51c: a hosztolt őr futtatja a hosztolt ügyfelek Építő- és rutinagentjeit** (Norbi, 2026-11-17; `25-onfejlesztes-terv.md`, a Műhely az 56. kör, az 51c előtt). A hosztolt ügyfélnek nincs saját őre, ezért az önfejlesztés agentes részei (az Építő, az agentes rutinok, a stdio-s MCP-szerverek) a szerveren futó hosztolt őrön, sandboxban futnak, **az ügyfél saját API-kulcsával** (2.3). A szerveroldali részek (MCP-átjáró, kapcsolatok, deklaratív és JS-eszközök, agent nélküli rutinok) őr nélkül is működnek.
- **Operátori meghívó a felületen** (több operátor): később.
- **A vészhozzáférés bővítése** DM-ekre és privát csatornákra: csak ha szükség lesz rá.
