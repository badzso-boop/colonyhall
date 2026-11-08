# Telepítési útmutató

Állapot: 2026-11-05, 41. kör. A terv és a döntések: `15-telepites-terv.md`. Angol rövid változat: a repó `README.md`-je. Norbi homelabjának egyedi beállításai továbbra is a `05-homelab-telepites.md`-ben és a `CLAUDE.md`-ben vannak.

## Röviden

```sh
gh release download -R badzso-boop/colonyhall -p install.sh -O install.sh   # amíg a repó privát (előtte: gh auth login)
less install.sh
sh install.sh            # letölti és ellenőrzi a colony-t, majd elindítja a `colony setup`-ot
```

A `colony setup` megkérdezi, mi legyen a gép: **szerver**, **őr** vagy **mindkettő**. Minden lépés előtt megmutatja, mit futtatna; a `sudo`-t és a telepítést megerősítés után.

## A szerver

- Mappa: `~/.local/share/colonyhall/server/` (`compose.yaml`, `.env` `0600`-val; macOS-en a `backup.sh` is).
- Adatbázis: alapból saját Postgres a compose-ban; vagy meglévő Postgres kapcsolati URL-lel (a kérdés rejtett, mert jelszót tartalmaz). A konténerből a `localhost` maga a konténer: a gép címét vagy egy Docker-hálózat nevét kell megadni.
- Cím: alapból `127.0.0.1:8080`. Választható a helyi hálózati vagy a Tailscale-cím. `0.0.0.0` csak külön megerősítéssel, felügyelet nélküli módban csak `allow_all_interfaces: true`-val.
- Linuxon a Docker-parancsok **`sudo`-val** futnak (döntés: a `docker` csoport tagsága root-egyenértékű).
- Első tulajdonos: felhasználónév, név, jelszó (rejtve, kétszer; üresen generál egyet, és egyszer kiírja). A jelszó a `colonyhall bootstrap`-nak stdin-en megy, parancssorba nem kerül.
- Mentés (opció): éjjel 3:30-kor `pg_dump` és a fájlkötet archívuma, 14 nap megőrzéssel.
  - **Linuxon** rendszerszintű systemd-időzítő rootként fut (mert a Docker `sudo`-val fut), ezért minden, amit használ, root tulajdonú: a szkript `/usr/local/lib/colonyhall/backup.sh`, a mentések helye `/var/backups/colonyhall/` (`0700`). A felhasználó által írható szkriptet vagy mappát a root nem futtathat és nem írhat, különben a felhasználó (vagy bármi, ami a nevében fut, pl. egy agent) rootot kapna. A szkript a felhasználó `compose.yaml`-jét és `.env`-jét sem olvassa: az adatbázis-konténert a compose címkéi alapján találja meg.
  - **macOS-en** launchd ügynök a felhasználó nevében, a mentések a `~/colonyhall-backups/`-ba.
  - Visszaállítás: `sudo docker compose … exec -T db pg_restore -U colony -d colonyhall --clean < colonyhall-ÉÉÉÉHHNN-ÓÓPP.dump`, a fájlok: `sudo docker run --rm -v colonyhall_files:/data -v /var/backups/colonyhall:/backup busybox tar xzf /backup/files-….tar.gz -C /data` (macOS-en `~/colonyhall-backups` és `sudo` nélkül).
- Frissítés: `colony setup --server --upgrade` (az új képcímke az `.env`-be, `pull`, `up -d`; a migrációk induláskor futnak). Előbb a szerver, utána az őrök.
- Eltávolítás: `colony uninstall --server` (az adat marad), `--purge`-dzsel az adat is törlődik.

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
- Kolléga külön Linux-felhasználóval: `10-kollega-beallitasa.md` (az első körben csak útmutató).
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
