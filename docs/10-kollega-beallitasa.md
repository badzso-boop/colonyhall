# Kolléga beállítása a homelabon (runbook)

Állapot: 2026-11-04, 38. kör. **Norbi futtatja**, lépésről lépésre; a lépések a homelabon `sudo`-t igényelnek, ezért agent nem futtatja őket. A háttér és a döntések: `01-terv.md`, „Kolléga saját agentekkel” fejezet; a végpontok: `02-muszaki-specifikacio.md`, „Tulajdonos, láthatóság, Claude-belépés”.

A példákban a kolléga Linux- és Colonyhall-neve `kollega`, a gép neve a Colonyhallban `homelab-colleague`. A homelab saját szabályai (`~/homelab/CLAUDE.md`, `README.md`) előbb olvasandók.

## Mit kap a kolléga

- Saját Linux-felhasználót a homelabon: **nincs `sudo`**, **nincs `docker` csoport**, csak a saját mappái.
- Egy második őrt, ami ezen a felhasználón fut, és a Colonyhallban külön gépként látszik (`homelab-colleague`, tulajdonosa a kolléga).
- Két agentet ezen a gépen, mindkettő az övé:
  1. **tervező** (teljes hozzáférés, `write_scope: docs`): olvassa a projektjeit, csak `.md` fájlt írhat, parancsot nem futtathat, a kódot a párjára bízza;
  2. **kódoló** (sandboxos, `bwrap`): a kapott projektmappákban dolgozik.
- A saját Claude-előfizetését: a `claude setup-token` tokenjét ő maga adja meg a PWA-ban, Norbi nem látja.
- Csak azokat a csatornákat, amelyekbe Norbi beteszi, csatornánként „Csak olvashat” vagy „Írhat” joggal; Norbi agentjeit (futások, lépések, hibák, mappák, terminálok, költségek) nem látja, és Norbi teljes hozzáférésű agentjeit nem ébresztheti fel.

## 0. Előfeltételek

- A szerver és Norbi mindkét őre már a 38. kör kódjával fut (élesítési sorrend: a jelentés végén).
- A homelabon van `bwrap` (0.12.0) és `tmux`.
- **Norbi saját könyvtára ne legyen mások által olvasható**, mert a tervező agent a `kollega` felhasználó jogaival olvas:

```sh
ls -ld /home/*                 # drwxr-x--- vagy drwx------ legyen, nem drwxr-xr-x
sudo chmod 750 /home/szmajli01 # ha olvasható volt; előtte nézd meg, nem olvassa-e más felhasználó (pl. egy szolgáltatás)
ls -ld /srv /opt /var/lib/docker 2>/dev/null   # ami itt mindenkinek olvasható, azt a kolléga agentje is olvashatja
```

## 1. Linux-felhasználó

```sh
sudo adduser --disabled-password --gecos "Kolléga" kollega
id kollega                     # csak a saját csoportja: NEM lehet benne sudo, docker, adm
sudo -iu kollega mkdir -p ~kollega/projects ~kollega/.local/bin ~kollega/.config/colonyhall
sudo chmod 700 /home/kollega
```

A projektek ide kerülnek: `/home/kollega/projects/<projekt>` (git clone a kolléga nevében, a saját Gitea- vagy GitHub-hozzáférésével; Norbi titkai ide nem kerülnek).

## 2. Claude Code és a `colony` a kolléga felhasználójának

```sh
# Claude Code (natív telepítő), a kolléga felhasználójaként; NEM jelentkezik be vele
sudo -iu kollega bash -lc 'curl -fsSL https://claude.ai/install.sh | bash && ~/.local/bin/claude --version'

# A colony binárist Norbi fordítja (a homelabon nincs Go), majd átmásolja
cd ~/homelab/colonyhall
docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -e CGO_ENABLED=0 -v "$PWD":/src -w /src golang:1.27 \
  go build -trimpath -o bin/colony ./cmd/colony
sudo install -o kollega -g kollega -m 755 bin/colony /home/kollega/.local/bin/colony
```

## 3. A kolléga a Colonyhallban

A PWA-ban, Norbiként:

1. **Adminisztráció → Felhasználók → Új személy**: `kollega`, szerep **member** (nem admin). A kezdeti jelszót a kolléga az első belépéskor megváltoztatja.
2. **Csatornák**: a csatorna beállításainál (Tagok) add hozzá, és válaszd: „Csak olvashat” vagy „Írhat”. Később ugyanitt átállítható, újra hozzáadás nélkül.

## 4. A második őr

### 4.1 `guard.yaml`

`/home/kollega/.config/colonyhall/guard.yaml` (tulajdonos `kollega`, `chmod 600`):

```yaml
server: http://100.85.111.88:8280
host_token_file: /home/kollega/.config/colonyhall/host.token
claude_token_file: /home/kollega/.config/colonyhall/claude.token   # ezt állítja be a kolléga a PWA-ból
roots:
  - /home/kollega/projects          # csak a kolléga saját mappái
allow_unsandboxed: true             # a tervező agent teljes hozzáférésű (de csak .md-t ír, parancsot nem futtat)
sandbox_network: filtered
preview_ports: 42000-42999          # ne fedje Norbi őrét (41000-41999); a kolléga nincs a docker csoportban, előnézet nála úgysem fut
max_concurrent_runs: 1              # kis gép: egyszerre egy futás, a többi sorban áll ("⏳ sorban állok…")
# exchange_dir: nincs (a kolléga titkai nem Norbi Z: meghajtóján mennek)
```

Amit **nem** kell külön beállítani, mert a felhasználó saját könyvtárából jön, és így nem ütközik Norbi őrével: `sandbox_dir` (`~/.local/share/colonyhall/sandbox`), az őr állapotmappája, a használat-ellenőrzés mappája, a proxy- és web_read-socketek, a tmux-socket (a tmux felhasználónként külön könyvtárat használ).

```sh
sudo chown kollega:kollega /home/kollega/.config/colonyhall/guard.yaml && sudo chmod 600 /home/kollega/.config/colonyhall/guard.yaml
```

### 4.2 Párosítás és szolgáltatás

1. A PWA **Gépek** oldalán: „Gép párosítása” → kód.
2. A homelabon, a kolléga nevében, **bejelentkezett (login) shellből** (a szolgáltatás a shell PATH-át kapja, benne a `~/.local/bin`-nel):

```sh
sudo loginctl enable-linger kollega              # bejelentkezés nélkül is fusson
sudo machinectl shell kollega@ /bin/bash -l      # vagy: sudo -iu kollega, majd export XDG_RUNTIME_DIR=/run/user/$(id -u)
colony guard enroll -server http://100.85.111.88:8280 -code <KÓD> -name homelab-colleague
colony guard install -config ~/.config/colonyhall/guard.yaml
systemctl --user daemon-reload && systemctl --user enable --now colony-guard
journalctl --user -u colony-guard -n 50          # "guard: watching"; a Gépek oldalon megjelenik a gép
exit
```

3. A PWA **Gépek** oldalán a `homelab-colleague` gép **tulajdonosa: kollega** (csak a workspace tulajdonosa állíthatja). Ettől a kolléga látja a gépet a saját Gépek oldalán, és ő állíthatja be a Claude-belépést.

## 5. A két agent

A PWA **Agentek → Új agent** varázslójában, Norbiként. Előbb a kódolót, mert a tervező párként erre hivatkozik.

1. **Kódoló** (`@kollega-coder`): tulajdonos **kollega**; gép `homelab-colleague`; kliens Claude Code; mód **tömör, sandbox**; mappa: `/home/kollega/projects/<projekt>` **írás**; csatornák: ahol dolgozik.
2. **Tervező** (`@kollega-planner`): tulajdonos **kollega**; gép `homelab-colleague`; kliens Claude Code; mód **közvetlen, teljes hozzáférés**, gyökér `/home/kollega/projects`; **„Csak dokumentációt írhat (kódot a sandboxos párja ír)”** bekapcsolva, pár: `@kollega-coder`; jóváhagyás a chatből: nincs (a csak-dokumentáció módban nem választható).

A napi tokenkeret és a munkamenet-limit agentenként a szokott helyen állítható.

## 6. Claude-belépés (a kolléga csinálja)

1. A **saját gépén** (nem a homelabon): `claude setup-token`, bejelentkezés a saját előfizetésével, a kiírt token kimásolása.
2. A PWA **Gépek** oldalán a `homelab-colleague` gépnél: **„Belépés beállítása”** → a token beillesztése → mentés.
3. Az őr ellenőrzi (`claude -p /usage`, modellhívás nélkül), és csak sikeres ellenőrzés után menti (`claude.token`, `0600`). Sikertelen ellenőrzésnél a régi marad, a Dialog megmondja az okát. Ha a gép nem elérhető: „a gép nem elérhető”, és semmi nem tárolódik.
4. Utána a gépnél: „Claude-belépés: beállítva (dátum)”.

A token útja: PWA → szerver (csak memóriában, amíg az őr egyszer le nem kéri) → az őr → a fájl. A szerver nem tárolja, nem naplózza, eseménybe és auditba nem írja; az audit csak azt rögzíti, ki és melyik gépen állította be (`host.claude_login_set`).

## 7. Ellenőrzés

- A kolléga belép: a bal sávban és az Agentek oldalon csak a saját két agentje látszik, a Gépek oldalon csak a `homelab-colleague`, a Futások oldalon csak a saját agentjei futásai.
- A kolléga megszólítja `@kollega-planner`-t egy csatornában, ahol írhat: válaszol, `.md`-t ír, a kódmunkát `@kollega-coder`-nek adja.
- A kolléga `@`-megszólítja Norbi egyik teljes hozzáférésű agentjét: az nem ébred fel.
- `sudo -iu kollega ls /home/szmajli01` → `Permission denied`.
- `journalctl --user -M kollega@ -u colony-guard` (vagy a kolléga shelljéből) — hibák nélkül.

## Eltávolítás

1. PWA: a két agent törlése (archiválás); a `homelab-colleague` gép **visszavonása** (Gépek); a kolléga felhasználójának állapota **stopped** (Felhasználók), vagy a csatornatagságok törlése.
2. Homelab:

```sh
sudo machinectl shell kollega@ /bin/bash -lc 'systemctl --user disable --now colony-guard'
sudo loginctl disable-linger kollega
sudo tar czf /root/kollega-backup-$(date +%F).tgz /home/kollega/projects   # ha kell a munkája
sudo deluser --remove-home kollega
```

3. A kolléga a saját gépén visszavonhatja a tokent (Claude fiókbeállítások), ha többé nem használja.

## Ha nem fut a kolléga agentje

Lásd `06-allapot-es-uzemeltetes.md`, „Ha a kolléga agentje nem fut”.
