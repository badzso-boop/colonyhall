# Colonyhall telepítése a homelabra

Állapot: 2026-10-21, terv. A döntések (Norbi, 2026-10-21): a homelab **közös Postgrese**, **tiszta lappal** (a PC-s dev adatok nem jönnek át), **csak Tailscale-en** elérhető (nincs nginx, nincs Cloudflare Tunnel), **Docker Compose**-zal. Az őrök natívan futnak (systemd felhasználói szolgáltatás), a homelabon és a PC-n is.

Mielőtt bármihez nyúlsz a homelabon: olvasd el a `~/homelab/CLAUDE.md`-t, a `~/homelab/README.md`-t és a `~/homelab/SHARED-POSTGRES.md`-t. Közös, élő infrastruktúra: amit egy másik projekt is használ (közös Postgres, Gitea), azon csak kis, dokumentált, visszafordítható lépés, és ami élő adatot vagy konfigurációt érint, ahhoz Norbi jóváhagyása kell.

## Áttekintés

| | |
|---|---|
| Kód | `https://github.com/badzso-boop/colonyhall` (privát), a homelabon: `~/homelab/colonyhall` (saját repó, a homelab repóban gitignore-olva) |
| Szerver | `deploy/compose.yaml`, konténer `colonyhall-server-1`, image a repóból |
| Cím | `http://100.85.111.88:8280` (a homelab Tailscale-címe; a 8080 foglalt) |
| Adatbázis | `homelab-shared-postgres`, adatbázis `colonyhall`, felhasználó `colonyhall_app`, hálózat `shared-db-net` |
| Fájlok | Docker volume `colonyhall_files` |
| Gitea | `homelab-gitea`, belső cím `http://100.85.111.88:3002` (lásd `04-gitea-agent-hozzaferes.md`) |
| Őrök | homelab: `~/.config/colonyhall/guard.yaml`; PC (WSL): ugyanígy, a szerver címe ugyanez |

## 1. A kód

```sh
gh repo clone badzso-boop/colonyhall ~/homelab/colonyhall
```

A `~/homelab` repó `.gitignore`-jába a `colonyhall/` sor (a többi projekthez hasonlóan), a `~/homelab/README.md` projekt-táblázatába egy sor: `colonyhall/` | — (csak Tailscale: `http://100.85.111.88:8280`) | chat embereknek és AI agenteknek.

Frissítés később: `cd ~/homelab/colonyhall && git pull && docker compose -f deploy/compose.yaml up -d --build` (a migrációk induláskor futnak).

## 2. Adatbázis a közös Postgresen

A `SHARED-POSTGRES.md` receptje szerint (a superuser jelszava a `~/homelab/.env`-ben; `docker exec -i`, különben a heredoc csendben semmit nem csinál):

```sh
PW=$(openssl rand -hex 24)
docker exec -i homelab-shared-postgres psql -U postgres -v ON_ERROR_STOP=1 <<SQL
CREATE ROLE colonyhall_app WITH LOGIN PASSWORD '$PW';
CREATE DATABASE colonyhall OWNER colonyhall_app;
REVOKE CONNECT ON DATABASE colonyhall FROM PUBLIC;
GRANT CONNECT ON DATABASE colonyhall TO colonyhall_app;
SQL
```

- A jelszó a `deploy/.env`-be kerül (lásd 3.), másodpéldány a homelab mintája szerint: `~/homelab/colonyhall-shared-db-creds.env` (600, nem git).
- Ellenőrzés: egy másik projekt userével csatlakozva `permission denied for database colonyhall`.
- A közös image `pgvector/pgvector:pg16`; a Colonyhall migrációi és tárolótesztjei ezen is átmennek (kipróbálva 2026-10-21).
- A napi mentés (`homelab-shared-postgres-backup`) automatikusan ezt az adatbázist is menti.

## 3. Beállítások: `deploy/.env`

```sh
cd ~/homelab/colonyhall
cp deploy/.env.prod.example deploy/.env && chmod 600 deploy/.env
```

Kitöltendő:

- `COLONY_BIND=100.85.111.88`, `COLONY_PORT=8280`
- `COLONY_DATABASE_URL=postgres://colonyhall_app:<jelszó>@homelab-shared-postgres:5432/colonyhall?sslmode=disable`
- `COLONY_GITEA_URL=http://100.85.111.88:3002`, `COLONY_GITEA_PUBLIC_URL=https://gitea.ujjweb.hu`
- `COLONY_GITEA_TOKEN`, `COLONY_GITEA_WEBHOOK_SECRET`: a `~/colonyhall-exchange/colonyhall/gitea-credentials.txt`-ből (`COLONY_GITEA_TOKEN`, `COLONY_GITEA_WEBHOOK_SECRET`), a képernyőre ki nem írva.

## 4. Indítás

```sh
docker compose -f deploy/compose.yaml up -d --build
curl -s -o /dev/null -w '%{http_code}\n' http://100.85.111.88:8280/readyz   # 200
docker compose -f deploy/compose.yaml logs --tail 30 server
```

- Csak a Tailscale-címen figyel (`COLONY_BIND`); a `0.0.0.0`-ra kötés tilos.
- `restart: unless-stopped`: a géppel együtt indul; `docker compose … down` után leállítva marad.
- Nincs dev seed: amíg a 5. lépés nincs meg, senki nem tud belépni.
- A fájltár kötete (`colonyhall_files`) a szerver felhasználójáé (`nonroot`, 65532): az image ezzel a tulajdonossal hozza a `/data/files` mappát, egy új kötet ezt kapja meg. Egy 2026-10-22 előtti, root-tulajdonú kötetnél (tünet: feltöltés és fájlmegnyitás 500, a naplóban `permission denied`) egyszer: `docker run --rm -v colonyhall_files:/data/files busybox chown -R 65532:65532 /data/files` (a homelabon ez 2026-10-22-én megtörtént).

## 5. Az első admin (Norbi maga futtatja)

A jelszót Norbi gépeli be, a stdin-ről megy, így nem kerül a shell-előzménybe:

```sh
cd ~/homelab/colonyhall
read -rs PW && echo "$PW" | docker compose -f deploy/compose.yaml run --rm -T server bootstrap -handle norbi -name Norbi; unset PW
```

- Csak üres adatbázison fut le (a második futás hibát ad).
- A kiírt tulajdonosi API-tokent jelszókezelőbe, vagy figyelmen kívül hagyni (a felülethez a jelszó kell).
- Utána belépés böngészőből: `http://100.85.111.88:8280`.

## 6. A Gitea átkötése

1. A Gitea `app.ini`-ben (`~/homelab/config/gitea/gitea/conf/app.ini`) a `[webhook] ALLOWED_HOST_LIST`-be a homelab Tailscale-címe is (`100.85.111.88`), a PC-é (`100.122.147.16`) a PC-s dev szerver leállítása után kivehető; a Gitea újraindítása. (Élő közös szolgáltatás: Norbi jóváhagyásával.)
2. A `colony-agents/hello-react` webhookja (és minden további repóé): cél `http://100.85.111.88:8280/api/v1/hooks/gitea`, ugyanazzal a titokkal. A Gitea felületén (repó → Beállítások → Webhookok) vagy API-n a repó tulajdonosának tokenjével (`PATCH /api/v1/repos/colony-agents/hello-react/hooks/1`).
3. A felületen új `#hello-react` csatorna és a repó–csatorna hozzárendelés a „Gitea” panelen.
4. Próba: egy push a repóba → „📦 Push to main …” a csatornában.

## 7. Agentek és gépek (tiszta lap)

1. A felületen („Új agent”): `@claude` (Claude Code) és `@antigravity` (Antigravity), és amit még szeretnél (pl. egy homelabos agent).
2. Gépenként párosítás: a „Gépek” panelen párosító kód, a gépen `colony guard enroll` (lásd 8.).
3. Agent-hozzárendelés a géphez: kliens, mód (`direct`), gyökérmappa, modell; a gyökérmappa a gép `guard.yaml` `roots` listáján belül legyen.
4. A közös agent-szabályok új adatbázisban a beépített alapszöveggel indulnak: a „Git (homelab Gitea)” részt (lásd `04-gitea-agent-hozzaferes.md`, illetve a PC-s szerver 1-es verziója) újra fel kell venni az „Agent szabályok” panelen.

## 8. Őr a homelabon

A homelabon nincs Go; az őrt egy Go-konténer fordítja le, telepítés nélkül:

```sh
cd ~/homelab/colonyhall
docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -e CGO_ENABLED=0 -v "$PWD":/src -w /src golang:1.27 \
  go build -trimpath -o bin/colony ./cmd/colony
install -m 755 bin/colony ~/.local/bin/colony
```

`~/.config/colonyhall/guard.yaml` (minta: `guard.example.yaml`):

```yaml
server: http://100.85.111.88:8280
host_token_file: /home/szmajli01/.config/colonyhall/host.token   # kötelező; az enroll ide írja
roots:
  - /home/szmajli01/homelab        # amit Norbi engedélyez az agenteknek
exchange_dir: /home/szmajli01/colonyhall-exchange
allow_unsandboxed: true            # a saját agentek közvetlen módban, sandbox nélkül futnak
# require_sandbox marad az alapértéken (true): bwrap nélkül a sandboxos agentek nem futnak, a közvetlenek igen
```

```sh
colony guard enroll -server http://100.85.111.88:8280 -code <KÓD a Gépek panelről> -name homelab
colony guard install -config ~/.config/colonyhall/guard.yaml   # bejelentkezett shellből: a PATH-ban legyen ~/.local/bin (claude, agy)
loginctl enable-linger "$USER"                                 # bejelentkezés nélkül is fusson
systemctl --user daemon-reload && systemctl --user enable --now colony-guard
journalctl --user -u colony-guard -f                           # "guard: watching"
```

- A `claude` és az `agy` a `~/.local/bin`-ben van; a `guard install` a telepítő shell PATH-át írja a szolgáltatásba, ezért **bejelentkezett (login) shellből** kell futtatni.
- A géptoken: `~/.config/colonyhall/host.token` (600), az `enroll` alapból ide írja; a `guard.yaml` `host_token_file` mezője erre mutasson.
- Az agentek git-hozzáférése a homelabon: `04-gitea-agent-hozzaferes.md`, „Új gép beállítása” (ugyanaz a belső cím, `http://100.85.111.88:3002`).

## 9. Őr a PC-n (WSL)

Ugyanígy, a szerver címe `http://100.85.111.88:8280`, `host_token_file: /root/.config/colonyhall/host.token`; a `colony` bináris a repóból (`go build -o ~/.local/bin/colony ./cmd/colony`), a `guard.yaml` a `~/.config/colonyhall/`-ban, `roots` a PC-n engedett mappák. A WSL-ben a linger már be van kapcsolva (2026-10-21), a WSL-t a Windows Feladatütemező tartja életben (bejelentkezéskor `conhost.exe --headless wsl.exe -d Ubuntu-24.04 -- sleep infinity`).

A PC-s dev stack ezután leáll: `make down` (a `restart: unless-stopped` miatt leállítva marad); a `deploy/.env` dev beállításai maradhatnak.

## 10. Lezárás

- A közös átadó fájl (`~/colonyhall-exchange/colonyhall/gitea-credentials.txt`) törlése, ha minden token a helyén van (szerver `deploy/.env`, a gépeken `~/.config/colonyhall/gitea/`).
- `CLAUDE.md` „Fejlesztői környezet” és az élő Claude Doc frissítése: a szerver a homelabon fut.

## Ha vissza kell állni

- A szerver: `docker compose -f deploy/compose.yaml down` (az adat a közös Postgresben és a `colonyhall_files` volume-ban marad).
- A PC-s dev stack: `make dev` (a régi dev adatbázis érintetlen).
- Az adatbázis eldobása (csak ha tényleg újra kell kezdeni, Norbi jóváhagyásával): `DROP DATABASE colonyhall; DROP ROLE colonyhall_app;`.
