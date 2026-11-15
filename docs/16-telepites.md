# Telepítési útmutató

Állapot: 2026-11-05, 41. kör. A terv és a döntések: `15-telepites-terv.md`. Angol rövid változat: a repó `README.md`-je.

## Röviden

> **Még nincs kiadás (2026-11-15).** Az `install.sh`, a `colony setup` és a `colony upgrade` a `colony` programot és a szerver image-ét GitHub-kiadásból tölti le, ezért az első `v*` kiadástól működik. Addig forrásból lehet kipróbálni: `git clone …`, majd `make dev` (Docker kell; fejlesztői seeddel, `http://localhost:8080/app/`), lásd a `README.md` „Trying it from source” részét.

```sh
gh release download -R badzso-boop/colonyhall -p install.sh -O install.sh   # amíg a repó privát (előtte: gh auth login)
less install.sh
sh install.sh            # letölti és ellenőrzi a colony-t, majd elindítja a `colony setup`-ot
```

A `colony setup` megkérdezi, mi legyen a gép: **szerver**, **őr** vagy **mindkettő**. Minden lépés előtt megmutatja, mit futtatna; a `sudo`-t és a telepítést megerősítés után.

## A szerver

- Mappa: `~/.local/share/colonyhall/server/` (`compose.yaml`, `.env` `0600`-val; macOS-en a `backup.sh` is).
- Adatbázis: alapból saját Postgres a compose-ban; vagy meglévő Postgres kapcsolati URL-lel (a kérdés rejtett, mert jelszót tartalmaz). A konténerből a `localhost` maga a konténer: a gép címét vagy egy Docker-hálózat nevét kell megadni. A saját Postgres a `pgvector/pgvector:pg17` kép (a keresés a `vector` kiterjesztést használja, 36. kör). **Meglévő Postgresnél** a `vector` kiterjesztésnek elérhetőnek kell lennie, és ha a Colonyhall felhasználója nem superuser, egyszer létre kell hozni: `CREATE EXTENSION vector;` az adatbázisban (különben az első indításkor a migráció ezzel a tanáccsal áll meg).
- Cím: alapból `127.0.0.1:8080`. Választható a helyi hálózati vagy a Tailscale-cím. `0.0.0.0` csak külön megerősítéssel, felügyelet nélküli módban csak `allow_all_interfaces: true`-val.
- Linuxon a Docker-parancsok **`sudo`-val** futnak (döntés: a `docker` csoport tagsága root-egyenértékű).
- Első tulajdonos: felhasználónév, név, jelszó (rejtve, kétszer; üresen generál egyet, és egyszer kiírja). A jelszó a `colonyhall bootstrap`-nak stdin-en megy, parancssorba nem kerül.
- Mentés (opció): éjjel 3:30-kor `pg_dump` és a fájlkötet archívuma, 14 nap megőrzéssel.
  - **Linuxon** rendszerszintű systemd-időzítő rootként fut (mert a Docker `sudo`-val fut), ezért minden, amit használ, root tulajdonú: a szkript `/usr/local/lib/colonyhall/backup.sh`, a mentések helye `/var/backups/colonyhall/` (`0700`). A felhasználó által írható szkriptet vagy mappát a root nem futtathat és nem írhat, különben a felhasználó (vagy bármi, ami a nevében fut, pl. egy agent) rootot kapna. A szkript a felhasználó `compose.yaml`-jét és `.env`-jét sem olvassa: az adatbázis-konténert a compose címkéi alapján találja meg.
  - **macOS-en** launchd ügynök a felhasználó nevében, a mentések a `~/colonyhall-backups/`-ba.
  - Visszaállítás: `sudo docker compose … exec -T db pg_restore -U colony -d colonyhall --clean < colonyhall-ÉÉÉÉHHNN-ÓÓPP.dump`, a fájlok: `sudo docker run --rm -v colonyhall_files:/data -v /var/backups/colonyhall:/backup busybox tar xzf /backup/files-….tar.gz -C /data` (macOS-en `~/colonyhall-backups` és `sudo` nélkül).
- Adatbázis-szerepkör (45. kör): a saját Postgresszel készülő **új** telepítésnél a szerver egy külön `colony_app` szerepkörrel csatlakozik (nem superuser, `NOBYPASSRLS`, így a sorszintű védelem rá is vonatkozik); a jelszava az `.env`-ben (`COLONY_APP_DB_PASSWORD`), a telepítő stdin-en adja át a `psql`-nek. A korábbi telepítések frissítéskor megtartják a superuser-kapcsolatukat; a szerver ilyenkor figyelmeztet a naplóban.
- Frissítés: `colony setup --server --upgrade` (az új képcímke az `.env`-be, `pull`, `up -d`; a migrációk induláskor futnak). Előbb a szerver, utána az őrök.
- Forráskód-link (AGPL 13. pont): ha módosított változatot üzemeltetsz, a `.env`-ben `COLONY_SOURCE_URL=<a módosított forrás címe>`; a felület és a `GET /api/v1/version` erre mutat. Üresen a nyilvános repó.
- Bejelentkezési korlát (40. kör): handle-ként 5, IP-címenként 20 hibás próba után 15 percre kizár (a vészleállítás feloldásánál és a jelszócserénél külön számlálóval), a választ `429` és `Retry-After` kíséri, a kizárás az auditba kerül, és a Rendszerállapot oldal „Biztonság” kártyája mutatja. **Ha kizártad magad:** várd ki a `Retry-After` idejét, vagy vedd fel a saját címedet a `.env`-ben a `COLONY_LOGIN_ALLOWLIST`-be (IP-k vagy CIDR-ek vesszővel; a hibák így is naplózódnak), majd `docker compose up -d`. Fordított proxy mögött a `COLONY_TRUSTED_PROXIES` sorolja fel a proxy címét; különben az `X-Forwarded-For` fejlécet a szerver figyelmen kívül hagyja.
- A szerver konténere csak olvasható gyökérrel, képességek nélkül, `no-new-privileges`-szel és memória- (alap 1 GB) és folyamatkorláttal (256) fut; állítható: `COLONY_MEM_LIMIT`, `COLONY_GOMEMLIMIT`, `COLONY_PIDS_LIMIT`. A `colony setup --server --upgrade` a régi telepítések compose-fájlját is kiegészíti.
- Eltávolítás: `colony uninstall --server` (az adat marad), `--purge`-dzsel az adat is törlődik.

### Tárhely és keret (54. kör)

- **Fájljaim** (a felhasználói menüben és a profilban): mindenki a saját és az agentjei által feltöltött fájlokat látja, és ezeket törölheti. Más fájlját senki nem törölheti, a tulajdonos sem. A törölt fájl helyén az üzenetekben „Fájl törölve” áll; a tartalom az óránkénti karbantartáskor tűnik el a fájlkötetből, ha más nem hivatkozik rá.
- **Tárhely kártya** (Rendszerállapot oldal és Beállítások → Fájlok): a munkaterület foglaltsága (csatolmányok, chat- és keresési adatok, egyéb adat), az utolsó adatbázis-becslés ideje és az állapot. Ki mit töltött fel, azt a tulajdonos sem látja.
- **Keret:** a tulajdonos ugyanitt állítja be GB-ban; üres vagy 0 = korlátlan (az alapértelmezés, így frissítéskor nem változik semmi). 80% és 95% fölött a tulajdonos DM-et kap a „Colonyhall” rendszerfióktól, a tulajdonos és az adminok a fejléc alatt sávot látnak. Ha megtelik, a feltöltés érthető hibát ad, és az őrök nem ébresztik az agenteket, amíg nincs hely; üzenetet írni továbbra is lehet. Helyet a fájlok törlésével (mindenki a sajátjáét) vagy a keret emelésével lehet csinálni.
- **Frissítés:** a `00067` migráció a `file` táblát két oszloppal és két indexszel bővíti, és létrehozza a `workspace_storage` táblát. A használat az első óránkénti karbantartáskor (induláskor is lefut) számolódik ki; addig a kártya 0 adatbázisrészt mutat.

### Több ügyfél egy szerveren (többbérlős mód)

Alapból kikapcsolva. Bekapcsolva minden munkaterület a saját aldomainjén érhető el (`<slug>.<alapdomain>`); részletek: `docs/02-muszaki-specifikacio.md`, „Többbérlős működés”, és `docs/23-elkulonites.md`.

- `.env`: `COLONY_MULTITENANT=1`, `COLONY_BASE_DOMAIN=colonyhall.example.com`; az aldomainekhez wildcard DNS (`*.colonyhall.example.com`) és HTTPS-en wildcard tanúsítvány kell. A szerver adatbázis-szerepköre nem lehet superuser és nem lehet `BYPASSRLS` (különben nem indul).
- Munkaterület slugja: `colonyhall bootstrap -slug acme …`, utólag `colonyhall set-slug -workspace <név> -slug acme`.
- **Az operátor (51. kör):** a `.env`-be `COLONY_SECRET_KEYS` kell (`colonyhall secret-key`; a telepítő magától generál egyet, és a `COLONY_ENV=production`-t is beírja). A kulcsot tartsd az adatbázis-mentéstől külön: nélküle a titkosított értékek (az operátorok TOTP-titkai) nem nyílnak. Az első operátor (a szerver mappájában, `sudo docker compose … exec` alakban, mint a mentésnél): `read -rs PW && echo "$PW" | docker compose exec -T server /colonyhall operator-create -handle ops -name "Ops"` kiírja a hitelesítő alkalmazásba felveendő titkot (és az `otpauth://` címet), majd `docker compose exec -T server /colonyhall operator-confirm -handle ops -code <6 jegy>` aktiválja a fiókot, és egyszer kiírja a tartalék kódokat. A konzol: `https://admin.<alapdomain>/app/`. **Ajánlott:** a `.env`-ben a `COLONY_OPERATOR_ALLOWLIST` (IP-k vagy CIDR-ek vesszővel, pl. a Tailscale-hálózat `100.64.0.0/10`) korlátozza, honnan érhető el a konzol; máshonnan az `admin.` cím nem létezik (404), így az operátor belépését kívülről senki nem próbálgathatja és nem zárhatja ki. A telepítő többbérlős módban rákérdez. Ott lehet ügyfelet létrehozni (a tulajdonos meghívólinkje egyszer jelenik meg, 7 napig érvényes), felfüggeszteni és visszaállítani. Elveszett telefon: `/colonyhall operator-reset-totp -handle ops`, utána `operator-confirm` az új kóddal. Kulcsforgatás: az új kulcs elöl, a régi utána (`COLONY_SECRET_KEYS=k2:…,k1:…`), `colonyhall rotate-secrets`, majd a régi törölhető. Részletek: `docs/24-hosztolt-ajanlat.md`.
- Saját Gitea egy ügyfélnek: `printf '%s\n%s\n' "$TOKEN" "$SECRET" | colonyhall gitea-set -workspace <név> -url https://git.acme.example` (csak olvasási token); a Gitea webhookja ilyenkor `https://acme.<alapdomain>/api/v1/hooks/gitea`. Kikapcsolás: `colonyhall gitea-set -workspace <név> -off`.
- **Helyi kipróbálás domain nélkül:** a böngészők a `*.localhost` címeket a gépre irányítják. `COLONY_MULTITENANT=1 COLONY_BASE_DOMAIN=localhost`, két munkaterület (`acme`, `beta` slugokkal), és a böngészőben `http://acme.localhost:8080/app/` és `http://beta.localhost:8080/app/`. Parancssorból: `curl -H 'Host: acme.localhost' http://127.0.0.1:8080/api/v1/me`.

### HTTPS (útmutató, nincs beépítve)

A telepítő nem állít be TLS-t. Ha a szervernek az otthoni hálózaton vagy a Tailscale-en kívülről is elérhetőnek kell lennie, tedd fordított proxy mögé, például Caddyvel (automatikus tanúsítvánnyal):

```
colony.pelda.hu {
    reverse_proxy 127.0.0.1:8080
}
```

Utána a `.env`-ben `COLONY_SECURE_COOKIES=1`, és `colony setup --server` (vagy `sudo docker compose … up -d`). A szerver maradjon `127.0.0.1`-en, csak a proxy legyen kint.

## Az őr

- A gép párosítása: a webes felületen **Gépek → Új gép** ad egy párosító kódot és a telepítési parancsot; „mindkettő” módban a telepítő maga kér kódot a frissen létrehozott tulajdonos nevében.
- `~/.config/colonyhall/guard.yaml`, kommentekkel: a gyökérmappák (alapból `~/projects`; **WSL-en a `/mnt/*` Windows-meghajtókat nem ajánlja fel**), sandbox (Linuxon bubblewrap), `allow_unsandboxed`, `max_concurrent_runs: 2`, `preview_ports`, `claude_token_file`.
- Szolgáltatás:
  - Linux és WSL: systemd user unit (`~/.config/systemd/user/colony-guard.service`), `loginctl enable-linger` (megerősítés, `sudo`), hogy kijelentkezve is fusson.
  - WSL systemd nélkül: megerősítés után `[boot] systemd=true` az `/etc/wsl.conf`-ba; utána Windowsban `wsl --shutdown`, a terminál újranyitása, és a `colony setup` újrafuttatása folytatja.
  - WSL keepalive (opció): egy Windows-feladat bejelentkezéskor (`conhost.exe --headless wsl.exe -d <disztró> -- sleep infinity`). Nélküle az őr csak addig fut, amíg egy WSL-terminál nyitva van.
  - macOS: LaunchAgent (`~/Library/LaunchAgents/hu.colonyhall.guard.plist`), naplók: `~/Library/Logs/colonyhall/`. macOS-en nincs sandbox: csak teljes hozzáférésű agent futhat, a szerver a sandboxosat elutasítja.
- AI-kliensek: a telepítő a választott kliens hivatalos telepítőjét letölti, kiírja a címét, méretét és SHA-256-ját, kérésre megmutatja, és csak megerősítés után futtatja. **Belépni a felhasználónak kell** (`claude`, `agy`), vagy a Claude-tokent a webes felületen megadni (Gépek → Claude-belépés). Az Antigravity telepítési módja még nincs a telepítőben (helyőrző).
- Kolléga ugyanazon a gépen: külön Linux-felhasználó, azon `colony setup --guard` (saját őr, saját előfizetés).
- Frissítés: `colony upgrade` (letöltés, ellenőrzés, csere, az előző `colony.prev`, újraindítás, doctor). Eltávolítás: `colony uninstall` (a konfig marad), `--purge`.

## Felügyelet nélküli telepítés

```sh
colony setup --guard --yes --server-url http://100.85.0.1:8280 --pair-code K7Q2-MX4P --name laptop --roots ~/projects
colony setup --answers setup.yaml --yes
```

Válaszfájl (`chmod 600`, ha titok van benne; jobb a `COLONY_ADMIN_PASSWORD` és a `COLONY_DATABASE_URL` környezeti változó):

```yaml
mode: both
server:
  bind: 127.0.0.1
  port: 8080
  admin_handle: anna
  admin_name: Anna
  backups: true
guard:
  name: laptop
  roots: [/home/anna/projects]
  allow_unsandboxed: true
  max_concurrent_runs: 2
  linger: true
  clients: [claude]
```

`--dry-run`: kiírja a tervet (parancsok, fájlok, titkok nélkül), és semmit nem változtat. Az újrafuttatás a kész lépéseket kihagyja; hiba után onnan folytatja. Napló: `~/.local/state/colonyhall/setup.log` (a titkok maszkolva).

## Hibakeresés

- `colony doctor` (vagy `--json`): szerver, géptoken, `guard.yaml`, kliensek és belépésük, sandbox, tmux, git, szolgáltatás, szabad hely, mindegyikhez a javítás. A Gépek oldal gépenként mutatja ugyanezt.
- Őrnapló: `journalctl --user -u colony-guard -f` (Linux, WSL), `~/Library/Logs/colonyhall/hu.colonyhall.guard.log` (macOS).
- Szerver: `sudo docker compose --project-directory ~/.local/share/colonyhall/server logs -f server`.

## Kiadás (Norbinak)

- `v*` tag pusholása indítja a `.github/workflows/release.yml`-t: binárisok négy platformra, `SHA256SUMS`, `install.sh`, attestationök, és a `ghcr.io/badzso-boop/colonyhall:<tag>` kép. A GHCR-csomagot az első kiadás után **nyilvánossá kell tenni** (GitHub → Packages → colonyhall → Package settings → Change visibility), különben a telepítő nem tudja letölteni a képet.
- Próba közzététel nélkül: Actions → release → Run workflow (egy `v0.0.0-test` verzióval).
- Helyben: `make release-local VERSION=v0.3.0` → `dist/`.
