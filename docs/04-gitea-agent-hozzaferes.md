# Gitea és az agentek git-hozzáférése

Állapot: 2026-10-21, kipróbálva egy WSL-es gépen és a szerveren. **A címek, nevek és útvonalak példák** (`100.64.0.10` a Gitea gépének Tailscale-címe, `100.64.0.20` a fejlesztői gépé, `gitea.example.com` a nyilvános cím); a saját értékeidet írd be helyettük.

## Mire való

- **A saját kódunk (Colonyhall stb.) GitHubon marad.** A homelab Gitea az **agentek által létrehozott repóké** (próbaprojektek, amiket egy agent kezd el).
- A Colonyhall a Gitea-ból link-kártyákat mutat, a webhookokból üzenetet tesz a repóhoz rendelt csatornába, az agentek pedig a `git_status` MCP-eszközzel kérdezhetnek rá egy PR-re vagy issue-ra.
- Az agentek **csak a belső (Tailscale) címen** érik el a Gitea-t; az internetes cím csak a felhasználó böngészőjének való.

## A Gitea

| | |
|---|---|
| Konténer | `homelab-gitea` (a szerver közös szolgáltatása, egy saját Compose-stackben) |
| Belső cím (agentek, Colonyhall szerver) | `http://100.64.0.10:3002` |
| SSH (nem használjuk, a HTTPS-token elég) | `ssh://git@100.64.0.10:2223` |
| Nyilvános cím (csak embernek) | `https://gitea.example.com` (nginx + Cloudflare Tunnel) |
| Fiókok | `admin` (admin, ember), `colony-agents` (az agentek fiókja, nem admin) |
| Fontos `app.ini` beállítások | `DISABLE_REGISTRATION = true`, `ENABLE_PUSH_CREATE_USER = true` (az agent egy sima `git push`-sal hoz létre repót), `[webhook] ALLOWED_HOST_LIST` (lásd „Ismert gondok”) |

## Tokenek

| Token | Fiók | Jogok | Hol van |
|---|---|---|---|
| Olvasási (Colonyhall szerver) | `admin` | `read:repository`, `read:issue` | WSL: `deploy/.env` (`COLONY_GITEA_TOKEN`) |
| Írási (agentek) | `colony-agents` | `write:repository`, `write:issue` (**`write:user` nélkül**) | gépenként: `~/.config/colonyhall/gitea/token` és `git-credentials` (600) |
| Webhook-titok | – | – | WSL: `deploy/.env` (`COLONY_GITEA_WEBHOOK_SECRET`); a Gitea webhookjában ugyanez |

Szabályok: token és titok soha nem kerül a chatbe, a repóba vagy naplóba; gépek között a közös átadó mappán át megy (`~/colonyhall-exchange/<projekt>/`, 600), és az átvevő a beállítás után törli onnan. Ha egy token kiszivárog: a Gitea-ban (`colony-agents` → Beállítások → Alkalmazások) visszavonni, újat kérni, és minden gépen kicserélni a két fájlban.

## A Colonyhall szerver beállítása

`deploy/.env` (gitignore-olva, minta: `deploy/.env.example`):

```sh
COLONY_BIND=0.0.0.0                          # a webhook Tailscale-en érkezik (WSL NAT módban a LAN-ról így sem érhető el)
COLONY_GITEA_URL=http://100.64.0.10:3002   # az API-t mindig itt hívja
COLONY_GITEA_PUBLIC_URL=https://gitea.example.com  # csak a linkek felismeréséhez; a kártyák erre mutatnak
COLONY_GITEA_TOKEN=…                         # olvasási token
COLONY_GITEA_WEBHOOK_SECRET=…
```

Utána `make dev`. Ellenőrzés: `curl http://100.64.0.20:8080/readyz` a homelabról 200.

## Új gép beállítása az agenteknek (WSL kész, homelab ugyanígy)

Az agentek (Claude Code, Antigravity) a gépen a felhasználó saját Linux-fiókjával futnak, ezért a git-beállítást úgy kell megadni, hogy **csak a belső Gitea-ra** vonatkozzon, és ne írja felül a felhasználó saját (GitHubos) nevét és belépését.

1. **Token átvétele** a közös átadó mappából (`AGENTS_GITEA_WRITE_TOKEN`), a képernyőre nem kiírva:

   ```sh
   G=~/.config/colonyhall/gitea
   umask 077; mkdir -p $G; chmod 700 $G
   TOKEN=$(grep -E '^AGENTS_GITEA_WRITE_TOKEN=' ~/colonyhall-exchange/colonyhall/gitea-credentials.txt | cut -d= -f2-)
   printf 'http://colony-agents:%s@100.64.0.10:3002\n' "$TOKEN" > $G/git-credentials
   printf '%s\n' "$TOKEN" > $G/token
   chmod 600 $G/*
   ```

   (A WSL-ről az átadó mappa `/mnt/z/colonyhall-exchange/…`, vagy `ssh homelab 'grep …'`.)

2. **Az agentek git-identitása** (`$G/gitconfig-agents`):

   ```ini
   [user]
   	name = colony-agents
   	email = colony-agents@noreply.gitea.example.com
   ```

3. **Csak a belső Gitea-ra szűkített globális beállítás** (előtte `cp ~/.gitconfig ~/.gitconfig.bak-$(date +%Y%m%d%H%M%S)`):

   ```sh
   git config --global credential."http://100.64.0.10:3002".helper "store --file=$G/git-credentials"
   git config --global includeIf."hasconfig:remote.*.url:http://100.64.0.10:3002/**".path "$G/gitconfig-agents"
   ```

   - A token csak ennek a hostnak megy ki; a GitHub `gh`-s belépése érintetlen.
   - Minden repóban, amelyiknek a remote-ja a belső Gitea, a commit szerzője automatikusan `colony-agents` (git ≥ 2.36 kell a `hasconfig:remote.*.url`-hoz; a WSL-en 2.43).
   - **Mindig a belső címet** (`http://100.64.0.10:3002/…`) használd remote-nak, akkor is, ha a homelabon helyben futsz: a feltétel és a credential erre a címre szól.

4. **Ellenőrzés:**

   ```sh
   curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: token $(cat $G/token)" http://100.64.0.10:3002/api/v1/repos/search?limit=1   # 200
   ```

5. **Ha a gép kész** (WSL és homelab is): az átadó fájlt törölni (`~/colonyhall-exchange/colonyhall/gitea-credentials.txt`).

## Munkafolyamat az agenteknek

A rövid változat a Colonyhall közös agent-szabályaiban is benne van („Git (homelab Gitea)”, 1-es verzió, 2026-10-21), így minden agent megkapja; a fejlesztői oldal „Agent szabályok” paneljén szerkeszthető.


- **Új repó:** helyben `git init -b main`, remote `http://100.64.0.10:3002/colony-agents/<név>.git`, majd `git push -u origin main` — a push létrehozza (privátként). Az API-n nem lehet (ahhoz `write:user` kellene).
- **Munka:** saját ágon (`colony/<agent>/<téma>`), commit, `git push -u origin <ág>`; a `main`-re közvetlenül nem.
- **PR nyitása** (a token a `$G/token` fájlban):

  ```sh
  curl -s -X POST -H "Authorization: token $(cat ~/.config/colonyhall/gitea/token)" \
    -H 'Content-Type: application/json' \
    http://100.64.0.10:3002/api/v1/repos/colony-agents/<repó>/pulls \
    -d '{"title":"…","body":"…","head":"<ág>","base":"main"}'
  ```

- **Webhook egy új repóra** (a tulajdonos tokenjével; a titok a Colonyhall szerver `deploy/.env`-jéből):

  ```sh
  curl -s -X POST -H "Authorization: token $(cat ~/.config/colonyhall/gitea/token)" -H 'Content-Type: application/json' \
    http://100.64.0.10:3002/api/v1/repos/colony-agents/<repó>/hooks \
    -d '{"type":"gitea","active":true,"events":["push","pull_request","status"],
         "config":{"url":"http://100.64.0.20:8080/api/v1/hooks/gitea","content_type":"json","secret":"<webhook-titok>"}}'
  ```

- **Csatorna hozzárendelése** (ember, a fejlesztői oldal „Gitea” paneljén, vagy `POST /api/v1/git/mappings {"repo":"colony-agents/<repó>","channel_id":"…"}` admin tokennel). Hozzárendelés nélkül a szerver elfogadja a webhookot, de nem posztol.

## Az első repó: `colony-agents/hello-react`

- React + Vite „hello world”, privát; helyben egy agent gyökérmappáján belül (pl. `~/projects/colonyhall/sandbox-test/hello-react`).
- Webhook: 1-es azonosító, a fenti címre. Csatorna: `#hello-react`.
- Kipróbálva (2026-10-21): push-sal létrehozás; commit `colony-agents` névvel; PR #1 nyitása és beolvasztása API-n; link-kártya a nyilvános linkre (repó és PR is), a lekérés a belső címen; webhook-üzenetek a `#hello-react` csatornában („📦 Push to main…”, „🟣 PR #1 merged…”), az `@` megtörve, így senkit nem ébresztenek.

## Ismert gondok

- **`ALLOWED_HOST_LIST = private` nem engedi a Tailscale-címet** (javítva 2026-10-21). A Gitea „private” listája a hagyományos privát tartományokat jelenti; a Tailscale `100.64.0.0/10` (CGNAT) ebbe nem tartozik, ezért a webhook „webhook can only call allowed HTTP servers” hibával elmaradt. A Gitea `app.ini`-ben a Colonyhall szerver Tailscale-címének is benne kell lennie (pl. `private,100.64.0.20`); ha a szerver máshova költözik, ezt is frissíteni kell.
- A Gitea a `pull_request` eseményt aleseményekre bontja (címke, hozzárendelés, review…); ezek a csatornában „Git event: … on …” üzenetként jelenhetnek meg. Ha zavar, a webhookban csak a szükségeseket kell bekapcsolni.
- A `colony-agents` commitjai `colony-agents@noreply.gitea.example.com` címmel mennek; a Gitea ezeket a fiókhoz köti, ha ez a cím a fiók e-mailje (vagy ha a fióknál hozzá van adva).
