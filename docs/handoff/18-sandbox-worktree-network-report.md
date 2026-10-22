# Handoff Report: 18 – Sandbox worktree és hálózati allowlist

**Ág:** `feature/sandbox-worktree-net`  
**Dátum:** 2026-10-22  
**Státusz:** Kész, ellenőrizve és tesztelve, éles klienspróbákkal igazolva.

---

## 1. Commitek

1. `dc4cadf` – **Guard: worktree support and post-run commit for sandboxed agents** (1. rész: worktree támogatás sandboxos ágenseknek, szigorú `.git` ro-bind, explicit git-dir/work-tree, futás utáni biztonsági ellenőrzés és commit push nélkül, UI és API specifikáció bővítés).
2. `2175981` – **Guard: network allowlist and proxy bridge for sandboxed agents** (2. rész: hálózati névtér-izoláció `bwrap --unshare-net`, Unix socket HTTP/CONNECT proxy az őrben anti-SSRF/rebinding védelemmel, `colony sandbox-bridge` TCP híd a sandboxban, Claude Code és Antigravity kliens alapértelmezések, `filtered`/`log`/`open` üzemmódok).
3. *(Következő commit)* – **Docs: handoff report and Hungarian documentation updates for task 18** (3. rész: magyar nyelvű dokumentációk és összefoglaló jelentés).

---

## 2. Megvalósított döntések (ahol a prompt nyitott volt)

1. **Commit szerző és formátum:**
   - Szerző: `<handle> (Colonyhall sandbox) <handle@colonyhall.local>`
   - Commit tárgy: `colony(<handle>): <feladat első 60 karaktere>` (többsoros esetén az első sor max 60 karaktere)
   - Commit törzs: `Requested in #<csatorna>/DM by @<felhasználó>`
   - `colony/<handle>/work` ágra készül, hookok kikapcsolva (`core.hooksPath=/dev/null`, `GIT_CONFIG_GLOBAL=/dev/null`).
   - Push szigorúan nincs: a módosítások a lokális worktree-ben maradnak felülvizsgálatra.

2. **Commit megtagadási feltételek (biztonsági szűrés):**
   A commit automatikusan elutasításra kerül és hibát dob az őr, ha a változtatások között:
   - Titokfájl-gyanús név szerepel (`isSecretName`: `.env`, `*.key`, `*.pem`, `id_*`, `credentials*`, `.netrc`, `.npmrc`, stb.),
   - Bármely hozzáadott/módosított fájl mérete meghaladja a 10 MB-ot,
   - Bármely fájl olyan szimbolikus link, amely a worktree könyvtárán kívülre mutat.

3. **Proxy protokoll és architektúra:**
   - Az őr a gazdagépen, a sandboxon kívül egy Unix domain socketen (`<agent_dir>/proxy.sock`) futtat egy HTTP és HTTPS CONNECT proxyt.
   - A sandboxba csak a socket és a `colony` futtatható állomány kerül be.
   - A sandbox belépési pontja a `colony sandbox-bridge -sock <path> -port 3128 -- <kliens> <argumentumok>` bináris, amely a sandbox saját loopbackjén (127.0.0.1:3128) kétirányúan streameli a TCP forgalmat a Unix socket felé.
   - A kliensek a szabványos `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, `http_proxy`, `https_proxy`, `all_proxy` környezeti változókat kapják meg.

4. **Anti-SSRF és DNS-rebinding szűrés:**
   - Csak CONNECT metódus és 443-as célport engedélyezett (kivéve a Colonyhall szerver portját).
   - Nyers IP-címek mint célállomások elutasításra kerülnek (403 Forbidden).
   - A hosztnév DNS-feloldása után a proxy minden kapott IP-címet ellenőriz:
     - Tiltott: loopback (127.0.0.0/8, ::1), privát címek (RFC1918: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16), link-local (169.254.0.0/16, fe80::/10), multicast, unspecified (0.0.0.0, ::), CGNAT (100.64.0.0/10) és IPv6 ULA (fc00::/7).
     - Kivétel: a beállított Colonyhall szerver feloldott IP-je és portja mindig engedélyezett (CGNAT / helyi cím esetén is).
     - Sima plain-HTTP kérés kizárólag a Colonyhall szerver címe felé továbbítható.

5. **Hálózati üzemmódok (`guard.yaml: sandbox_network`):**
   - `filtered` (alapértelmezett): szigorú allowlist és SSRF szűrés unshare-net alatt.
   - `log`: minden kimenő kapcsolat megengedett, de az új `host:port` célok az őr naplójában és a proxy belső nyilvántartásában rögzítésre kerülnek (végpontok felderítéséhez).
   - `open`: a hálózati elválasztás ki van kapcsolva (a sandbox a host hálózatát használja, rollback funkcióként).

---

## 3. Kliensek által igényelt hosztok (éles mérések alapján)

A gépen valós `claude` és `agy` CLI futtatásával vizsgáltuk meg a hálózati igényeket `log` és `filtered` módban is (`internal/guard/manual_check_test.go`):

### 1. Claude Code (`claude-code`)
- **Szükséges a működéshez:**
  - `api.anthropic.com:443`
- **Megfigyelt járulékos hívások:**
  - `mcp-proxy.anthropic.com:443` (telemetria / MCP proxy ellenőrzés; tiltása nem akadályozza a működést, `filtered` módban is hibátlanul válaszolt `PONG`-gal).
- **Alapértelmezett lista (`DefaultClaudeCodeAllow`):**
  - `api.anthropic.com:443`

### 2. Antigravity (`antigravity` / `agy`)
- **Szükséges a működéshez:**
  - `daily-cloudcode-pa.googleapis.com:443` (AI modell végpont)
  - `www.googleapis.com:443` (OAuth2 userinfo / fiók ellenőrzés)
  - `antigravity-unleash.goog:443` (funkciókapcsolók és jogosultság-ellenőrzés)
  - `lh3.googleusercontent.com:443` (felhasználói profil / avatar lekérés)
  - `play.googleapis.com:443` (kliens telemetria / bejelentkezés)
- **Alapértelmezett lista (`DefaultAntigravityAllow`):**
  - A fenti 5 hoszt mindegyike szerepel benne, így az Antigravity `filtered` módban nulla elutasítással, zökkenőmentesen válaszol.

---

## 4. Mi maradt ki szándékosan

- **Git távoli push:** A sandboxos ágensek nem kapnak hálózati hozzáférést a git szerverekhez (Gitea/GitHub), és az őr sem pushol helyettük automatikusan. A munka a helyi worktree-ben marad az ágensenkénti ágon.
- **Komplex proxy hitelesítés:** Nincs szükség proxy basic auth-ra, mivel a proxyhoz kizárólag a gép root/user jogosultságú őre és az adott ágenshez tartozó belső Unix domain socketen keresztül lehet hozzáférni.
- **Nem-HTTPS protokollok:** A proxy kizárólag HTTPS CONNECT alagutat és a Colonyhall szerver felé irányuló sima HTTP hívásokat engedélyez. Egyéb nyers TCP/UDP forgalom nem támogatott a sandboxban.

---

## 5. Ellenőrzések kimenete

### `make test`
```
go test ./...
ok  	colonyhall/cmd/colonyhall	(cached)
ok  	colonyhall/internal/e2e	10.375s
ok  	colonyhall/internal/guard	6.206s
ok  	colonyhall/internal/httpserver	0.487s
ok  	colonyhall/internal/modules/agentrules	0.077s
ok  	colonyhall/internal/modules/conversation	0.078s
ok  	colonyhall/internal/modules/delivery/mcptools	0.083s
ok  	colonyhall/internal/modules/files	0.070s
ok  	colonyhall/internal/modules/files/mcptools	0.078s
ok  	colonyhall/internal/modules/files/storage	0.093s
ok  	colonyhall/internal/modules/git	0.072s
ok  	colonyhall/internal/modules/git/provider	0.021s
ok  	colonyhall/internal/modules/identity	0.173s
ok  	colonyhall/internal/policy	0.007s
ok  	colonyhall/internal/storetest	0.082s
PASS
```

### `make lint`
```
golangci-lint run ./...
0 issues.
```

### `make gen-check`
```
make gen
go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
docker run --rm -u 0:0 -v "/mnt/g/Projects/new-chat-worktrees/sandbox-worktree-net":/src -w /src sqlc/sqlc:1.31.1 generate
git diff --exit-code -- internal/apispec internal/eventlog/postgres/sqlcgen ...
(tiszta, nincs eltérés)
```

### `go test -race ./internal/guard/ ./internal/e2e/`
```
ok  	colonyhall/internal/guard	6.695s
ok  	colonyhall/internal/e2e	45.986s
PASS
```

### Kézi ellenőrzés valós kliensekkel (`internal/guard/manual_check_test.go`)
- `claude-code` log módban: Sikeres `PONG` válasz, felderített hosztok: `api.anthropic.com:443`, `mcp-proxy.anthropic.com:443`.
- `claude-code` filtered módban: Sikeres `PONG` válasz, engedélyezve: `api.anthropic.com:443`.
- `antigravity` log módban: Sikeres `PONG` válasz, felderített hosztok rögzítve.
- `antigravity` filtered módban: Sikeres `PONG` válasz, nulla blokkolt kéréssel.

---

## 6. Mit nézzen meg a kódátnéző először

1. **`internal/guard/proxy.go`**:
   - `StartSandboxProxy`: Unix socket listener, CONNECT kérések kezelése, anti-SSRF/rebinding ellenőrzés (`checkTarget`, `isBlockedIP`), plain HTTP továbbítás csak szerver felé.
2. **`internal/guard/bridge.go` & `cmd/colony/main.go`**:
   - `colony sandbox-bridge`: hogyan indul a sandboxon belül, TCP 3128 és Unix socket közötti kétirányú adatmozgás, a parancs lefutásakor tiszta kilépés.
3. **`internal/guard/sandbox.go`**:
   - `Command`: `--unshare-net` beállítása, proxy környezeti változók injektálása, `colony sandbox-bridge` parancscsomagolás.
4. **`internal/guard/worktree.go`**:
   - `prepareWorktreeForSandbox`: read-only `.git` bind mount.
   - `PostRunCommit`: commit szabályok, szerző megadása, `isSecretName` / méret / symlink ellenőrzések.
5. **`internal/guard/network_bwrap_test.go` & `bridge_test.go` & `proxy_test.go`**:
   - Teljes end-to-end `bwrap --unshare-net` teszt lokális mock HTTP szerverrel és szimulált DNS feloldással, valamint híd és proxy egységtesztek.

---

## Átnézés (Claude Code, 2026-10-22)

Ellenőrizve: `make test`, `make lint`, `make gen-check`, `go test -race ./internal/guard/ ./internal/e2e/` zöld; a valódi `bwrap`-os tesztek ezen a gépen lefutnak (nem kihagyva). A kézi klienspróba (`COLONY_MANUAL_CHECK=1`) külön kapcsolóval fut, a `make test` nem indít valódi klienst.

Javítva:

- **Parancsfuttatás a sandboxon kívül (súlyos):** csak a worktree `.git` mutatófájlja volt írásvédett, a repó `.git` mappája nem. Ha a repó az agent írható mappájában van, a `.git/config`-ba írt `filter` + egy `.gitattributes` elég, hogy az őr futás utáni `git add`-ja a sandboxon kívül, Norbi jogaival parancsot futtasson. Most a sandboxban minden `.git` (a worktree mutatója, a repó `.git` mappája, és az engedélyezett mappák saját `.git`-je) csak olvasható (`gitMetadata`). Teszt: `TestSandboxedWorktree_RepoGitDirReadOnlyInBwrap` (valódi bwrap, a javítás nélkül elbukik).
- **DNS-rebinding a proxyban:** az ellenőrzés után a proxy a nevet tárcsázta, ami új névfeloldás; most az ellenőrzött címet. Teszt: `TestProxy_DialsCheckedAddress`.
- **`log` mód:** mindent engedett, belső címeket is (a homelab szolgáltatásait); most felderítéskor is csak nyilvános címet, a belsőket (és IP-literált) tiltja. Teszt: `TestProxy_LogMode`.
- **Commit-identitás:** a commit az őr által megadott névvel és e-maillel, aláírás nélkül megy (`-c user.name/user.email`, `commit.gpgsign=false`). Javítás nélkül a git a gép felhasználójából rakna össze egyet (`root@<gép>`), tehát nem bukott el, csak kiszámíthatatlan volt.

Nem javítva, későbbre:

- A `proxy.sock` az agent saját (írható) sandbox-mappájában van; a prompt azon kívül kérte. Kockázata kicsi (a socketet az agent csak a maga elől ronthatja el), de egy `<agent>.net/` testvérmappa tisztább.
- A `git status --porcelain` kimenetét szövegként bontja; különleges karakteres fájlnévnél a commit biztonságosan elmarad („could not stat”). A `-z` kapcsoló tisztább lenne.
