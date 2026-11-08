# Telepítő: terv (41. kör, tervezési kapu)

Állapot: **jóváhagyva** (Norbi, 2026-11-05; a döntések a dokumentum végén). A kötelező döntések a `docs/handoff/41-installer-prompt.md`-ben; ez a dokumentum a megvalósítás terve és a nyitott kérdések. Angol összefoglaló a későbbi nyilvános README-hez: `15-telepites-terv.en.md`.

Cél: valaki, aki még sosem látta a projektet, néhány paranccsal telepíti a szervert és a gépeire az őrt, és működő rendszert kap, Norbi homelabjára jellemző beállítás nélkül.

## Mai állapot (amire építünk)

- A `colony` (őr, CLI) és a `colonyhall` (szerver) `CGO_ENABLED=0`-val már ma lefordul `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64` célra (ellenőrizve). Go nem kell a felhasználó gépén.
- A szerver Docker-képe (`deploy/Dockerfile`) beágyazza a PWA-t; a verzió `-X colonyhall/internal/server.Version` -nel állítható. Az őr verziója ma fix konstans (`guard.Version = "0.2.0"`): változóvá kell tenni.
- Van már: `colonyhall bootstrap` (első workspace és tulajdonos üres adatbázison, jelszó stdin-ről), `colony guard enroll`, `colony guard install` (systemd user unit). Ezekre épül a telepítő, nem duplikálja őket.
- A `deploy/compose.yaml` ma Norbi homelabjára szabott (külső `shared-db-net`, kötelező Tailscale IP); a telepítő saját, általános compose-fájlt generál.
- A szerver ma **nem** utasítja el a sandboxos agentet sandbox nélküli gépen: ez kis szerveroldali kiegészítés lesz (macOS miatt kell).
- A CodeGraph (39. kör) még nincs megvalósítva: a telepítő akkor kínálja, ha az őr már ismeri (különben kihagyja, és szól).

## 1. A darabok és a sorrend

Új felhasználó útja (egy gép, ami szerver és őr is):

1. **Letöltés**: `install.sh` (lásd 2.), ami a platformnak megfelelő `colony` binárist tölti le és ellenőrzi (`SHA256SUMS`).
2. **`colony setup`** megkérdezi: *Szerver*, *Őr*, vagy *Mindkettő*.
3. **Szerver**: függőségek (Docker, Compose) → beállítások (cím, adatbázis) → `.env` generált titkokkal → `docker compose up -d` → várakozás a `/readyz`-re.
4. **Első admin (tulajdonos)**: handle, név, jelszó (rejtett bevitel, vagy generált és egyszer kiírt) → `colonyhall bootstrap` a konténerben, jelszó stdin-en.
5. **A PWA megnyitása**: kiírja a címet (`http://127.0.0.1:8080/app/`), és megnyitja, ha van böngésző.
6. **Gép hozzáadása**: „Mindkettő” esetén a telepítő maga kér párosító kódot a frissen létrehozott tulajdonos nevében (a session-nel, amit a bootstrap után a jelszóval nyit); egyébként a felhasználó a Gépek oldalon kéri, és beírja.
7. **Őr**: függőségek (git, tmux, bwrap Linuxon, curl) → `guard.yaml` → `colony guard enroll` → szolgáltatás (systemd vagy launchd) → indítás → `colony doctor`.
8. **AI-kliens belépés**: a telepítő ellenőrzi, és megmondja, mit kell a felhasználónak futtatnia (soha nem lép be helyette).
9. **Első agent**: a PWA Agentek oldalán; a telepítő a végén erre mutat egy rövid lépéssorral.

Kihagyható: **csak őr** egy meglévő szerverhez (a legtöbb gép, és a kollégák esete: szervercím + párosító kód), és **csak szerver** (ha az őrök más gépeken futnak).

## 2. Kiadások (GitHub Releases)

- **Workflow** (`.github/workflows/release.yml`), `v*` tag pusholásakor (és kézi indítással próbára):
  - `colony` és `colonyhall` a négy célra, `CGO_ENABLED=0 -trimpath -ldflags "-s -w -X …Version=<tag>"`; csomag: `colony_<ver>_<os>_<arch>.tar.gz`, `colonyhall_<ver>_<os>_<arch>.tar.gz`;
  - a PWA egyszer épül (Node), és mindkét szerverbináris és a kép beágyazza;
  - **szerverkép** a GHCR-re: `ghcr.io/<owner>/colonyhall:<ver>` és `:latest`, `linux/amd64` + `linux/arm64` (buildx);
  - `SHA256SUMS` az összes csomagra, és **build provenance attestation** (`actions/attest-build-provenance`) a csomagokra és a képre;
  - `install.sh` is release-asset, így verzióhoz kötött.
- **Verzió:** SemVer, a tag adja (`v0.3.0`). `colony --version`, `colonyhall --version`, a szerver `/readyz`/rendszerállapot és a host-jelentés ugyanazt mutatja. Az őr a saját verzióját jelenti (ma is), a Gépek oldal jelzi a régi őrt (ma is).
- **`make release-local`**: ugyanezeket a csomagokat és a `SHA256SUMS`-ot építi a `dist/`-be helyben (kép nélkül, vagy `RELEASE_IMAGE=1`-gyel helyi képpel), a tesztekhez.

**Privát repó, token.** Amíg a repó privát, a letöltéshez egy **fine-grained, csak olvasási** token kell (Contents: read, csak erre a repóra). A token forrása, ebben a sorrendben: `GITHUB_TOKEN` környezeti változó → `gh auth token` → rejtett bekérés. Soha nem kerül fájlba, naplóba, és **soha nem lesz parancssori argumentum**: a `curl` a fejlécet a stdin-ről kapja (`curl --config -`, `header = "Authorization: Bearer …"`), így `ps`-ben és a shell-előzményben sem látszik. A release-asset letöltése az API-n át megy (`Accept: application/octet-stream`); a GitHub egy aláírt, más hostra mutató címre irányít át, ahová a `curl` alapból **nem** viszi tovább az `Authorization` fejlécet (nem használunk `--location-trusted`-et). Nyilvános repónál ugyanez a szkript token nélkül megy (`releases/latest/download/…`).

**Hogyan jut a felhasználóhoz az `install.sh`?**
- *Privát időszakban* (javaslat): `gh release download -R <owner>/colonyhall -p install.sh -O - | sh` (a `gh` kezeli a belépést, token nem kerül az előzménybe), vagy a release-oldalról böngészőben letöltve `sh install.sh`.
- *Nyilvánosan*: `curl -fsSL https://github.com/<owner>/colonyhall/releases/latest/download/install.sh | sh`, és mellette az „előbb nézd meg” változat (`curl -fsSLO …/install.sh && less install.sh && sh install.sh`).

## 3. A telepítő felépítése

- **`install.sh` (POSIX `sh`, `shellcheck`-tiszta, kicsi):** felismeri az OS-t és az architektúrát (WSL-t is: `/proc/sys/fs/binfmt_misc/WSLInterop` vagy `WSL_DISTRO_NAME`), megkeresi a legújabb vagy a megadott (`--version`) kiadást, letölti a `colony` csomagot és a `SHA256SUMS`-ot, ellenőrzi (`sha256sum` / `shasum -a 256`), a `~/.local/bin`-be teszi (`--prefix`), és elindítja a `colony setup`-ot a saját argumentumaival.
- **`colony setup` (Go)** végzi a valódi munkát. **Miért így:** a shell-szkript csak a „tyúk-tojás” részt oldja meg (honnan lesz bináris). Minden más Go-ban: tesztelhető (hamis futtatókkal, golden fájlokkal), egy helyen fordítható (i18n), ugyanaz a kód Linuxon, WSL-en és macOS-en, és a `colony doctor`/`upgrade`/`uninstall` ugyanezeket a lépéseket használja. Egy hosszú shell-szkript ezt nem tudná.
- **Viselkedés:**
  - interaktív alapból; `--yes` (minden alapértelmezés elfogadva, megerősítést igénylő lépés `--yes` nélkül megáll), kapcsolók (`--server`, `--guard`, `--server-url`, `--pair-code`, `--roots` …) vagy **válaszfájl** (`--answers setup.yaml`) felügyelet nélküli telepítéshez;
  - `--dry-run`: kiírja a tervet (a pontos parancsokat, a fájlokat és tartalmukat titkok nélkül), és semmit nem változtat;
  - **idempotens**: minden lépés előbb ellenőrzi az állapotot (megvan → kihagyja; eltér → megmutatja és javítja megerősítés után);
  - **napló**: `~/.local/state/colonyhall/setup.log` (titkok nélkül, a tokeneket és jelszavakat maszkolja);
  - **folytatható**: a kész lépések egy állapotfájlba kerülnek (`~/.local/state/colonyhall/setup-state.json`); hiba után az újrafuttatás onnan folytatja.
- **Nyelv:** angol alapból, magyar, ha a locale `hu` (`LC_ALL` / `LC_MESSAGES` / `LANG`), vagy `--lang hu`. Üzenetkatalógus Go-ban (egyszerű kulcs → szöveg tábla, tesztelve, hogy minden kulcs megvan mindkét nyelven).
- **A lépések** egy közös interfészt valósítanak meg (`Check`, `Plan`, `Apply`), a külső parancsok egy `Runner` mögött (Docker, csomagkezelő, `systemctl`, `launchctl`, `sudo`), így a tesztek hamis futtatóval ellenőrzik a parancssorrendet.

## 4. A szerver telepítése

- **Docker és Compose:**
  - *Linux:* ha hiányzik, megerősítés után a disztribúció csomagjából (`apt install docker.io docker-compose-v2` / `dnf install docker docker-compose` vagy moby), szolgáltatás engedélyezése. A felhasználó `docker` csoportba vétele **külön kérdés**, és a telepítő megmondja, hogy ez root-egyenértékű jog; nélküle a telepítő `sudo docker …`-t használ.
  - *macOS:* Docker Desktop vagy OrbStack; a telepítő nem telepíti, hanem megmutatja a letöltési oldalt, és megvárja, hogy fusson.
  - *WSL:* Docker Desktop WSL-integrációja vagy a disztribúción belüli Docker (systemd kell hozzá, lásd 5.).
- **Hely:** `~/.local/share/colonyhall/server/` (`compose.yaml`, `.env`); `--server-dir`-rel máshova.
- **Adatbázis:**
  - alapból **saját Postgres** a compose-ban (`postgres:17`, névvel ellátott kötet, csak a belső hálózaton);
  - vagy **meglévő Postgres**: kapcsolati URL (rejtett bevitel, mert jelszót tartalmaz), a telepítő ellenőrzi az elérést és hogy üres-e az adatbázis.
- **Titkok** az `.env`-ben (`0600`): Postgres-jelszó (generált), és ami még kell; soha nem a compose-fájlban.
- **Cím:** alapból `127.0.0.1:8080` (csak erről a gépről). Választható: LAN-cím, vagy Tailscale-cím (ha a `tailscale ip -4` ad egyet). `0.0.0.0` csak külön megerősítéssel, és a telepítő kiírja, hogy ez minden hálózati csatolón elérhetővé teszi. **TLS nincs beépítve** az első körben: a telepítő Tailscale-t vagy fordított proxyt javasol, és HTTPS mögött `COLONY_SECURE_COOKIES=1`-et állít.
- **Első admin (tulajdonos):** handle, megjelenített név, jelszó: rejtett bevitel kétszer, vagy generált (egyszer kiírva, „mentsd el”). A `bootstrap` csak üres adatbázison fut (ma is így van), újrafuttatáskor kihagyja.
- **Gitea:** opcionális lépés (URL, belső URL, csak olvasási token, webhook-titok generálva), a mai `COLONY_GITEA_*` változókkal; alapból kihagyva.
- **Mentés:** a telepítő megmondja, mit kell menteni (az adatbázis és a `files` kötet; az `.env`). Opció: napi `pg_dump` + a fájlkötet archiválása a `~/colonyhall-backups/`-ba, 14 nap megőrzéssel, systemd user timerrel (Linux/WSL) vagy launchd-vel (macOS). Külső Postgresnél csak a fájlkötet, és figyelmeztet, hogy az adatbázist ott mentsék.
- **Frissítés:** `colony setup server --upgrade` (vagy `colony server upgrade`): új képcímke az `.env`-ben, `docker compose pull && up -d`; a migrációk induláskor futnak (ma is). Előtte opcionális mentés.
- **Eltávolítás:** `colony uninstall --server`: `docker compose down`; az adat (kötetek, `.env`) megmarad, csak `--purge`-dzsel törlődik (külön megerősítéssel).

## 5. Az őr telepítése

- **Párosítás:** szervercím + párosító kód a Gépek oldalról (vagy „Mindkettő” esetén automatikusan), gépnév (alapból a hostname).
- **`guard.yaml`** a válaszokból, kommentezett alapértékekkel, `0600`:
  - `roots`: a felhasználó által választott mappák; alapból a `~/projects` (vagy `~/code`, ha létezik). **WSL-en a `/mnt/*` Windows-meghajtókat nem kínálja alapból**: kiírja, hogy azok lassúak és a Windows-jogosultságok érvényesek rájuk, és csak külön kérésre veszi fel;
  - `exchange_dir`: opcionális, alapból nincs;
  - sandbox: Linuxon `bwrap` (ha nincs, telepíti megerősítés után); `allow_unsandboxed` kérdés (mit jelent: teljes hozzáférésű agentek); macOS-en nincs sandbox, `allow_unsandboxed: true`, és szól, hogy ott csak teljes hozzáférésű agent futhat;
  - `preview_ports`, `max_concurrent_runs` (alapból 2 kis gépen, 0 = korlátlan), `claude_token_file` (alapértelmezett hely).
- **Szolgáltatás:**
  - *Linux és WSL:* systemd user unit (a mai `colony guard install` kódja), `loginctl enable-linger` (sudo kell; megerősítés után), indítás és ellenőrzés;
  - *WSL systemd nélkül:* megerősítés után `[boot] systemd=true` az `/etc/wsl.conf`-ba (sudo), és kiírja, hogy ezután Windows-ból `wsl --shutdown`, majd a terminál újranyitása kell, és a telepítő újrafuttatása folytatja;
  - *macOS:* `launchd` LaunchAgent (`~/Library/LaunchAgents/hu.colonyhall.guard.plist`, `RunAtLoad`, `KeepAlive`, naplók a `~/Library/Logs/colonyhall/`-ba), `launchctl bootstrap gui/$UID`.
- **WSL keepalive** (opció): a telepítő a WSL-interopon át (`powershell.exe`) létrehoz egy Windows-feladatot a felhasználó bejelentkezésére: `conhost.exe --headless wsl.exe -d <distro> -- sleep infinity` (ahogy Norbinál a `nato`-n). Előtte megmutatja a pontos parancsot. Ha nem kéri: kiírja, hogy az őr csak addig fut, amíg egy WSL-terminál nyitva van.
- **Külön Linux-felhasználó (kolléga-mód, 38. kör):** az első körben **csak útmutató** (a telepítő a végén rámutat a `docs/10`-re és a `colony setup --guard`-ra, amit a kolléga felhasználójaként kell futtatni). Indok: felhasználó létrehozása root-művelet, és jogosultsági döntéseket igényel (ki olvashat mit), amit jobb tudatosan, kézzel csinálni.

## 6. Függőségek platformonként

| | Linux (apt/dnf) | WSL | macOS (brew) |
| --- | --- | --- | --- |
| git, curl | csomag | csomag | Xcode CLT / brew |
| tmux | csomag | csomag | brew |
| bwrap | `bubblewrap` | `bubblewrap` | – (nincs sandbox) |
| Docker (csak szerver) | csomag + szolgáltatás | Docker Desktop vagy csomag | Docker Desktop / OrbStack (kézzel) |
| Claude Code | hivatalos telepítő | hivatalos telepítő | hivatalos telepítő |
| Antigravity (`agy`) | hivatalos telepítő | hivatalos telepítő | hivatalos telepítő |
| CodeGraph | a 39. kör után | a 39. kör után | a 39. kör után |

- A telepítő előbb **listázza**, mi hiányzik, és mi futna (`sudo apt-get install -y git tmux bubblewrap`), és csak megerősítés után futtatja. Más csomagkezelőnél (pacman, zypper, apk) kiírja a parancsokat, és nem futtat.
- **Harmadik fél telepítője (Claude Code, Antigravity, CodeGraph): nincs vak `curl | sh`.** A telepítő letölti a szkriptet egy ideiglenes mappába, kiírja a forrás URL-jét, a méretét és a SHA-256-ját, felajánlja a megtekintését (`$PAGER`), és csak megerősítés után futtatja. Ahol a gyártó közöl ellenőrző összeget, azt is összeveti. Csak akkor kínálja, ha a felhasználó az adott klienst választja.

## 7. AI-kliens belépés

- **Ellenőrzés:**
  - *Claude Code:* `claude -p /usage --output-format json --no-session-persistence --strict-mcp-config` (modellhívás nélkül, az őr ma is ezt használja); siker = be van lépve. Ha a `claude_token_file` létezik, azt is jelzi („PWA-ból beállított belépés”).
  - *Antigravity:* `agy -p /usage --output-format json`.
- **Üzenet (nem lép be helyette, AI-fiók jelszót soha nem kér):**
  > „A Claude Code nincs bejelentkezve ezen a gépen. Futtasd ugyanezzel a felhasználóval: `claude` (és kövesd a belépést), vagy add meg a saját előfizetésed tokenjét a PWA-ban: Gépek → Claude-belépés (`claude setup-token` a saját gépeden). Utána: `colony doctor`.”
- Ugyanezt mondja az őr a naplójában és a host-jelentésben, ha egy futás belépési hiba miatt bukik.

## 8. `colony doctor`

Ellenőrzi, és mindegyikhez javítást ír ki: a szerver elérhetősége és verziója; a géptoken érvényes-e; a kliensek megvannak-e, és be vannak-e lépve (7.); sandbox (`bwrap` működik-e, macOS-en „nincs”); `tmux`; CodeGraph (ha van); a szolgáltatás állapota (systemd / launchd), linger; WSL: systemd és keepalive; szabad hely a sandbox- és az állapotmappában; a `guard.yaml` érvényessége. Kimenet: ember által olvasható és `--json`. **A host-jelentésbe** (szerződés-első: `HostReport.doctor`, ellenőrzések listája `id`, `status` = ok/warn/fail, `message`) az őr rendszeresen beküldi; a Gépek oldal gépenként mutatja.

## 9. `colony upgrade` és `colony uninstall` (őr)

- **upgrade:** a legújabb (vagy `--version`) kiadás letöltése, `SHA256SUMS`-ellenőrzés, a bináris atomi cseréje (a régi `colony.prev` néven marad), a szolgáltatás újraindítása, `doctor`. Futó agentmunka alatt figyelmeztet, és `--force` nélkül vár (az őr a jelentésben tudja, fut-e munka).
- **uninstall:** a szolgáltatás leállítása és eltávolítása, a bináris törlése; a `~/.config/colonyhall` (géptoken, `guard.yaml`) és az állapot megmarad, csak `--purge`-dzsel törlődik. Kiírja, hogy a gépet a Gépek oldalon is vissza kell vonni (vagy `--revoke`-kal megteszi, ha a géptoken még érvényes).

## 10. Tesztelés

- **Go-egységtesztek** (a `make test` része): lépések hamis `Runner`-rel (parancssorrend), `guard.yaml` golden fájlok platformonként és válaszkészletenként, systemd-unit és launchd-plist tartalom, `.env` és compose-generálás, i18n-kulcsok teljessége, a token soha nem argumentum és nem napló.
- **`install.sh`:** `shellcheck`, és konténeres tesztek (`make test-install`, Docker kell, ezért nem a `make test` része) `debian:12`, `ubuntu:24.04`, `fedora:41` képeken, egy helyi **hamis release-szerver** ellen (statikus fájlok, token nélküli és tokenes változat; a token-ellenőrzés a hamis szerverben). Valódi GitHub-hívás nincs a tesztekben.
- **Szerver:** hamis Docker-futtatóval a parancssorrend; és egy valódi futás Docker-in-Docker konténerben (a `make test-install` része), ami a `/readyz`-ig és a bootstrapig megy.
- **WSL:** kézi tesztterv Norbinak egy tartalék disztribúcióra (`wsl --install -d Ubuntu-24.04 --name colony-test`): systemd bekapcsolása, őr, keepalive, eltávolítás.
- **macOS:** GitHub Actions macOS-futtatón a Go-tesztek, az `install.sh --dry-run` és a plist ellenőrzése (`plutil -lint`), **csak tag-re és kézi indításra**. Költség: privát repóban a macOS-perc 10×-esen számít; egy kb. 6 perces futás kb. 60 percet visz a havi keretből (Free: 2000, Pro: 3000 perc). Alternatíva: kézi tesztlista Norbinak, ha van hozzáférése Machez.

## Szerveroldali kiegészítések ebben a körben

- A sandboxos agent elutasítása olyan gépen, amelynek a jelentése szerint nincs sandbox (`report.sandbox == ""`), érthető hibaüzenettel („ezen a gépen nincs sandbox (pl. macOS); csak teljes hozzáférésű agent futhat rajta”).
- `HostReport.doctor` (8.), a verzió kiírása (`--version` mindkét binárisnál).
- A PWA „Új gép” varázslója platformonként a megfelelő egysoros telepítési parancsot mutatja a párosító kóddal, és gépenként a doctor eredményét.

## Norbi döntései (2026-11-05)

1. A név végleges: **Colonyhall**; a meglévő privát repó (`badzso-boop/colonyhall`) lesz később nyilvános.
2. A **szerverkép (GHCR-csomag) nyilvános**, a kód egyelőre privát.
3. Az `install.sh` a privát időszakban **`gh release download`**-dal érhető el; később a GitHubról közvetlenül.
4. Linuxon alapból **`sudo docker`** (a `docker` csoport csak külön, magyarázott kérésre).
5. **TLS** az első körből kimarad; csak útmutató, hogyan állítható be.
6. Az **Antigravity telepítési módja** üresen marad (helyőrző); később egy másik agent tölti ki.
7. **macOS:** csak kézi tesztlista, nincs macOS CI-futtató.
8. `max_concurrent_runs` alapértéke új gépen: **2**.
9. **Kolléga-mód:** az első körben csak útmutató.
