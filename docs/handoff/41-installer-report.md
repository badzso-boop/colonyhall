# 41. kör jelentése: telepítő a teljes projekthez

Ág: `feature/installer` (worktree: `/mnt/g/Projects/new-chat-worktrees/installer`), 2026-11-05. Tervezési kapu: a terv (`docs/15-telepites-terv.md`) Norbi jóváhagyásával, a döntései a terv végén. Nincs tag, nincs release, nincs push; Norbi gépeihez nem nyúltam.

## Commitok

| Commit | Rész | Tartalom |
| --- | --- | --- |
| `42932f2` | – | a handoff prompt |
| `5e9e0f0`, `a9f472a` | 0 | a terv és az angol összefoglaló; Norbi döntései |
| `0172480` | 1 | kiadások: `scripts/release.sh`, `.github/workflows/release.yml`, `make release-local`, `--version` mindkét binárisnál, az őr verziója `-ldflags`-szel |
| `3a2335a` | 2 | `scripts/install.sh` + tesztek hamis GitHubbal, `make test-install` (Debian, Ubuntu, Fedora konténer), `make shellcheck` |
| `ffbfd5b` | 3–5 | `colony setup` (szerver és őr), `colony doctor`, `colony upgrade`, `colony uninstall` (`internal/setup`) |
| `ad836d5` | 4–5 | a szerver elutasítja a sandboxos agentet sandbox nélküli gépen; `HostReport.doctor` (szerződés-első), az őr beküldi |
| `938a643` | 6 | PWA: „Új gép” varázsló egysoros telepítéssel platformonként, állapot-ellenőrzés gépenként |
| `484da8b` | 7 | `README.md` (angol, új felhasználóknak), `docs/16-telepites.md`, `CLAUDE.md`, `docs/06` |

## Mi hol tesztelt

| Mi | Hogyan | Platform |
| --- | --- | --- |
| `install.sh` (nyilvános és privát kiadás, token nélkül hiba, rossz ellenőrzőösszeg, a token nem kerül argumentumba, a tárhelyre nem megy tovább) | Go-tesztek hamis GitHub-bal (API, letöltés, külön „tárhely” host), valódi `sh`, `curl`, `tar` | WSL Ubuntu (helyben) + `debian:12`, `ubuntu:24.04`, `fedora:41` konténer |
| `scripts/release.sh`, `--version` | helyi futtatás két célra, a kibontott bináris a tag verzióját mondja | helyben |
| a négy platform fordítása | `GOOS/GOARCH` keresztfordítás | linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 |
| `colony setup` szerver | hamis futtatóval a parancssorrend, a `.env` `0600` és tartalma, a compose-fájl (saját/külső DB), a jelszó csak stdin-en, a napló titokmentes, az újrafuttatás nem cserél jelszót és nem bootstrapol újra, `0.0.0.0` `--yes`-szel tilos | Go-teszt |
| `colony setup` őr | hamis futtatóval: Linux (systemd, linger), WSL systemd nélkül (`/etc/wsl.conf`, megállás az újraindítási tanáccsal), WSL keepalive (PowerShell-parancs), macOS (launchd, nincs bwrap/systemctl, csak teljes hozzáférés); `guard.yaml` golden fájlok platformonként, és az őr `LoadConfig`-ja elfogadja | Go-teszt |
| próbafuttatás, folytatás hiba után | `--dry-run` nem futtat és nem ír; a kész lépés újrafuttatáskor kimarad | Go-teszt |
| `colony upgrade` | letöltés, ellenőrzés, csere `.prev`-vel, rossz ellenőrzőösszeg, a token nem jut a tárhelyre | Go-teszt hamis GitHub-bal |
| `colony doctor`, `uninstall` | hamis futtató és hamis szerver; az eltávolítás a konfigot megtartja | Go-teszt |
| szerver: sandbox nélküli gép, doctor a jelentésben | e2e | – |
| PWA | Vitest: telepítési parancs, állapot-ellenőrzés a Gépek oldalon | – |

**Csak papíron (nem futott valódi rendszeren):**
- a valódi `docker compose up` és `bootstrap` (a GHCR-kép még nem létezik; Norbi gépén nem indítottam idegen compose-projektet);
- a valódi `systemctl --user`, `loginctl`, `launchctl`, a WSL `wsl.conf` és a Windows-feladat;
- a Claude Code telepítőjének letöltése és futtatása;
- a GitHub Actions workflow (első futása a próba-indítás vagy az első tag).

## Kézi teszt Norbinak

**WSL (egy tartalék disztribúción, a meglévőt nem érinti):**
1. PowerShell: `wsl --install -d Ubuntu-24.04 --name colony-test`, belépés.
2. A release után: `gh release download -R badzso-boop/colonyhall -p install.sh -O install.sh && sh install.sh` → „őr”, a szerver címe és egy kód a Gépek oldalról.
3. Elvárt: a systemd bekapcsolását kéri (ha nincs), `wsl --shutdown` után újrafuttatva folytatja; a keepalive-ot felajánlja; a végén `colony doctor` zöld (a Claude belépésen kívül).
4. Windows-kijelentkezés/bejelentkezés után a gép online a Gépek oldalon.
5. `colony uninstall --purge`, a Windows-feladat törlése (Feladatütemező → „Colonyhall WSL keepalive”), `wsl --unregister colony-test`.

**macOS (kézi lista, nincs CI):**
1. `sh install.sh` → „őr”: kiírja, hogy nincs sandbox; a `guard.yaml`-ban `require_sandbox: false`, `allow_unsandboxed: true`.
2. `launchctl print gui/$(id -u)/hu.colonyhall.guard` fut; napló: `~/Library/Logs/colonyhall/hu.colonyhall.guard.log`.
3. A PWA-ban sandboxos agent létrehozása erre a gépre: elutasítja („this machine has no sandbox”); teljes hozzáférésű: működik.
4. „Szerver” mód Docker Desktoppal vagy OrbStackkel: `colony setup --server`, a `/app/` megnyílik.
5. `colony upgrade` egy újabb kiadásra, `colony uninstall`.

## Első kiadás – ellenőrzőlista Norbinak

1. **Nincs szükség tokenre a workflow-hoz:** a beépített `GITHUB_TOKEN` (packages: write, contents: write, attestations, id-token) elég. A felhasználói letöltéshez a privát időszakban `gh auth login` vagy egy **fine-grained, csak olvasási** token (Contents: read, csak erre a repóra) a `GITHUB_TOKEN`-ben.
2. **Próba közzététel nélkül:** Actions → release → Run workflow (`v0.0.0-test`): a binárisok és a kép épülnek, nem publikál.
3. **Tag:** `git tag v0.3.0 && git push origin v0.3.0` (a `main`-ről, a beolvasztás után).
4. **Ellenőrzés:** a release-en 8 csomag + `SHA256SUMS` + `install.sh`; `gh attestation verify colony_v0.3.0_linux_amd64.tar.gz -R badzso-boop/colonyhall`; a GHCR-en `ghcr.io/badzso-boop/colonyhall:v0.3.0` két architektúrával.
5. **A GHCR-csomag láthatósága: nyilvános** (Packages → colonyhall → Package settings → Change visibility) – döntés szerint; nélküle a telepítő nem tudja letölteni a képet.
6. Egy próbatelepítés egy tiszta gépen vagy a WSL-tesztdisztróban (fent).
7. Norbi meglévő gépei: az élesítési sorrend változatlan (szerver, utána az őrök); az új őr verziója a tag lesz, a Gépek oldal „régi őr” jelzése ehhez igazodik.

## Döntések, ahol a prompt nyitva hagyta

- A szerverbinárisok is kikerülnek a négy platformra (a prompt kérte), bár a telepítő a Docker-képet használja.
- A `colony setup` újrahasznosítja a meglévő `guard.Enroll`-t és `guard.GenerateUnit`-et; a `colony guard enroll`/`install` megmaradt.
- A doctor a jelentésben csak azonosítót, állapotot és tényt küld (nem javítási szöveget): a PWA a saját nyelvén mutatja, a javításért `colony doctor`-ra utal.
- Sandbox nélküli gép = a jelentés tartalmazza az OS-t, de nem a sandboxot (így a régi tesztgépek, amik OS-t nem jelentenek, nem érintettek). Linux bubblewrap nélkül is ide tartozik (ott sem futhatott sandboxos agent).
- Mentés Linuxon rendszerszintű systemd-időzítővel (a Docker `sudo`-val fut, egy user timer nem tudna).
- A rejtett bevitel `stty`-vel (nincs új Go-függőség).

## Amit a reviewer először nézzen

1. **Titokkezelés a szkriptekben:** `scripts/install.sh` (`fetch`: a token `curl --config -`-on, stdin-ről; `--proto =https --proto-redir =https`; `ask_token` rejtett bevitel), `internal/setup/upgrade.go` (`Release.get`: a token csak az API-hostnak, átirányításkor más hostra/portra lekerül), `internal/setup/engine.go` (`Mask`, `Secret`), `server.go` (a `.env` `0600`, a jelszó stdin-en).
2. **Ami `sudo`-val fut:** csomagtelepítés (`apt-get`, `dnf`), Docker-parancsok Linuxon, `systemctl enable --now docker`, `loginctl enable-linger`, `tee /etc/wsl.conf`, a mentés systemd-unitjai `/etc/systemd/system`-be. Mindegyik előtt megerősítés (vagy `--yes`). Lásd `deps.go`, `server.go`, `guard.go`.
3. **Letöltött harmadik fél telepítői:** `guard.go` `installClient`: a Claude Code telepítője (`https://claude.ai/install.sh`) letöltés, cím + méret + SHA-256 kiírása, kérésre megmutatás, megerősítés, csak utána `sh`. Az Antigravity helye üres (döntés).

## Kimaradt

- macOS-sandbox, Windows WSL nélkül, csomagtárolók (apt, Homebrew), aláírt macOS-csomag, többbérlős változat (a prompt szerint).
- Az Antigravity telepítési módja (Norbi döntése: később egy másik agent tölti ki; `clientInstallers["agy"]`).
- Kolléga-mód automatizálása (csak útmutató).
- A CodeGraph (39. kör még nincs megvalósítva): a telepítő nem kínálja; ha elkészül, a `guardDeps`-be és a doctorba kell felvenni.
- Valódi szervertelepítés konténerben (a GHCR-kép első kiadása után érdemes egy Docker-in-Docker tesztet a `make test-install`-ba tenni).

## Eredmények

A `484da8b` állapoton:

- `make test`: zöld.
- `make lint`: 0 hiba.
- `make shellcheck` (`scripts/install.sh`, `release.sh`, `test-install.sh`): tiszta.
- `make test-install`: zöld (Debian 12, Ubuntu 24.04, Fedora 41; 3 × 5 teszt).
- `make test-web`: zöld (82 tesztfájl, 526 teszt).
- `make gen-check`: tiszta.
- `make test-pg`: nem futott (ebben a körben tár nem változott; az új `HostReport` mezők a meglévő JSON-oszlopba kerülnek).
