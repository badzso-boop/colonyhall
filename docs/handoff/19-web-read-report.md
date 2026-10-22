# Handoff Report 19: `web_read` — Sandboxed agentek weblapolvasása az őrön keresztül

**Ág:** `feature/web-read`  
**Dátum:** 2026-10-22  
**Munka könyvtár / worktree:** `/mnt/g/Projects/new-chat-worktrees/web-read`

---

## 1. Összefoglaló és cél

A 18. körben megvalósított szigorú hálózati izoláció (`bwrap --unshare-net`) után a sandboxos agentek kizárólag a saját AI-szolgáltatójukat és a Colonyhall szervert érték el. Ugyanakkor a programozási és hibajavítási feladatokhoz elengedhetetlen a dokumentációk (API specifikációk, könyvtárak leírása, hibaüzenetek) olvasása.

Norbi döntése (2026-10-22) alapján:
- **A teljes internet-hozzáférés továbbra is kizárólag a közvetlen (full-access) agentek kiváltsága marad.**
- A sandboxos agentek nem kapnak szabad internetelérést, hanem egy szigorúan csak olvasható `web_read` nevű MCP eszközt kapnak.
- **A letöltést a gazdagépen futó őr (guard) végzi, sosem a Colonyhall szerver**, megakadályozva, hogy a szerver a homelab belső szolgáltatásai felé ugródeszkaként szolgáljon.
- Kizárólag `GET` kérések engedélyezettek, fejlécek, kéréstest, sütik és hitelesítés nélkül.
- Szigorú anti-SSRF és DNS rebinding védelem (privát és belső IP-k tiltása, egyedi dialer a névfeloldáskor kapott pontos IP-címre pinned csatlakozással).
- HTML-ből olvasható szöveg generálás (`golang.org/x/net/html`), méret- és futási limitek (oldalanként max 5 MB, futásonként max 30 kérés és 10 MB).
- Streamable HTTP MCP szerver a belső `webread.sock` Unix domain socketen, amelyet az in-sandbox híd a `127.0.0.1:3129` TCP portra továbbít.
- Teljes transzparencia: naplózott URL-ek, chat válaszlábléc (`🌐 web_read: ...`) és szerveroldali futásjelentés (`web_reads` mező) felületi badge-dzsel.

---

## 2. Megvalósított részek és commitok

1. **Part 1: Szerveroldali engedélyezés és felületi kapcsoló** (`ffd2ddd`)
   - Adatbázis migráció: `migrations/00026_web_read.sql` (`allow_web_read boolean NOT NULL DEFAULT false` a `host_agent` táblán).
   - OpenAPI szerződés: `api/openapi.yaml` (`HostAgent`, `AssignRequest`, `OverviewAssignment`).
   - Szerver modulok: `internal/modules/hosts/` (modell, tárolók, validáció: kizárólag `sandbox: true` mellett engedélyezett, egyébként 400 Bad Request hiba; események és audit napló).
   - Őr kliens: `internal/guard/client.go` (`HostAgent.AllowWebRead`).
   - Webes felület: `internal/httpserver/web/index.html` — dinamikusan megjelenő checkbox és kétnyelvű leírás az agent hozzárendelési űrlapon (csak sandbox esetén), valamint badge az áttekintő panelen.
   - Tesztek: `internal/storetest/` és `internal/e2e/hosts_test.go` (400-as hiba validáció, mentés és visszaolvasás).

2. **Part 2: Biztonságos letöltő motor (Fetcher)** (`9bb38ef`)
   - `internal/guard/webread.go`:
     - URL validáció: kizárólag `http`/`https`, max 2048 karakter, tiltott userinfo (`user:pass@`) és nyers IP címek.
     - Anti-SSRF és DNS rebinding védelem: `net.DefaultResolver` használata, minden IP-cím azonnali ellenőrzése (loopback, RFC 1918, link-local, CGNAT `100.64.0.0/10`, IPv6 ULA `fc00::/7`, multicast, unspecified tiltása). Nincs szerver-kivétel!
     - Pinning dialer: a `net.Dialer` közvetlenül a névfeloldáskor kapott és ellenőrzött konkrét IP-címre tárcsáz (`DialContext`), így egy esetleges második DNS-válasz nem irányíthatja át a kapcsolatot.
     - Átirányítások (redirects): legfeljebb 5 lépés, minden lépésben teljes körű újraellenőrzéssel.
     - Fejlécek és környezet: fix `User-Agent: ColonyhallGuard-web_read/1 (+read-only)` és `Accept`, sütik nélkül (`Jar: nil`), külső proxy nélkül (`Proxy: nil`).
     - Időkorlátok és mérethatárok: connect 10s, kérés 30s, testméret max 5 MB (felette `truncated: true`).
     - Tartalomfeldolgozás: `text/html` fa-bejárás `golang.org/x/net/html`-lel (felesleges elemek: script, style, nav, footer, form eldobása; címsorok `#`, táblázatok, kódblokkok és abszolút linkek megtartása, title kinyerése). Szöveges formátumok (plain, markdown, json, xml, csv) szövegként, karakterkészlet konverzióval. Nem szöveges állományoknál (PDF, képek) rövid leíró üzenet.
     - Futási korlátok: futásonként legfeljebb 30 kérés és 10 MB.
   - Konfiguráció: `internal/guard/config.go` (`web_read_allow`, `web_read_deny`).
   - Tesztek: `internal/guard/webread_test.go` (11 egységteszt fake DNS resolverrel és httptest szerverekkel: HTML-szöveg konverzió, átirányítások, méretkorlátok, rebinding védelem, allow/deny szűrés, per-run limitek).

3. **Part 3: MCP szerver, híd továbbítás és kliens huzalozás** (`0ca08e4`)
   - OpenAPI szerződés és migráció: `ReportRunRequest`, `Run`, `OverviewRun` kiegészítve opcionális `web_reads []string` listával; migráció: `migrations/00027_run_web_reads.sql` (`web_reads text[] NOT NULL DEFAULT '{}'`).
   - Runs modul és felület: `internal/modules/runs/`, `internal/server/overview.go`, és a webes felület (`internal/httpserver/web/index.html`), ahol a legutóbbi futásoknál és a futások kártyán megjelenik a `🌐 N` badge tooltipben a teljes URL-listával.
   - Streamable HTTP MCP szerver: `internal/guard/webread_mcp.go`, amely Unix domain socketen figyel (`webread.sock`), az SDK-val implementálja a `web_read(url, offset?, max_chars?)` eszközt. A visszaadott szöveget `<web_content url="..." fetched_at="...">...</web_content>` blokkba zárja, az esetleges belső lezáró tageket escape-eli, támogatja a rúnák mentén történő lapozást és a futáson belüli memóriabeli gyorsítótárazást.
   - Híd multi-socket továbbítás: `internal/guard/bridge.go` (`RunMultiBridgeCommand`), a `colony sandbox-bridge` parancs támogatja a `-webread-sock` és `-webread-port` (alapértelmezetten 3129) paramétereket.
   - Sandbox és kliens beállítások: `internal/guard/sandbox.go` a sandbox indításakor mountolja a `webread.sock`-ot, Claude Code esetén generálja a `--mcp-config` fájlt és engedélyezi a `--allowedTools mcp__colony-web__web_read` eszközt; Antigravity esetén beállítja a `~/.gemini/config/mcp_config.json` konfigurációt és a `settings.json` engedélyeit.
   - Prompt és válaszlábléc: `internal/guard/prompt.go` és `internal/guard/guard.go` hozzáadja a használati instrukciót a munkafelszólításhoz, a válasz végére fűzi a `🌐 web_read: host (N pages)` sort, és jelenti az URL-eket a futás lezárásakor.
   - Tesztek: `internal/guard/webread_mcp_test.go`, `internal/guard/webread_run_test.go`, `internal/guard/network_bwrap_test.go`.

4. **Part 4: Dokumentáció, kézi teszt és hibajavítások** (jelen commit)
   - `CLAUDE.md`, `task.md`, `docs/01-terv.md`, `docs/02-muszaki-specifikacio.md`, `guard.example.yaml` magyar dokumentáció frissítése.
   - Kézi ellenőrzés tesztfájl: `internal/guard/manual_webread_test.go` (alapból skip, `COLONY_MANUAL_CHECK=1` mellett futtatható).
   - `NO_PROXY=127.0.0.1,localhost` javítás a sandbox környezeti változói között `allow_web_read` esetén.

---

## 3. Döntések a nyitott kérdésekben

1. **Socket továbbítás a hídon (bridge socket routing):**
   - *Döntés:* Két különálló Unix domain socketet használunk az ágens könyvtárában (`proxy.sock` a 3128-as portra, `webread.sock` a 3129-es portra) egyetlen multiplexelt socket és egyedi routing header helyett.
   - *Indoklás:* A két külön socket teljesen tiszta elválasztást nyújt a két teljesen eltérő protokoll (HTTP CONNECT proxy vs Streamable HTTP MCP) között. Nincs szükség egyedi protokollfejlécek beiktatására és kifejtésére, nincs hibaforrás vagy protokoll-injekciós kockázat, a `bridge.go` kódbázis pedig minimális bővítéssel képes tetszőleges számú port-socket párost párhuzamos goroutine-okban kiszolgálni.

2. **Futásjelentés API kiterjesztése (`web_reads`):**
   - *Döntés:* A szerver API-ban a `ReportRunRequest`, `Run` és `OverviewRun` sémákat kibővítettük az opcionális `web_reads []string` mezővel, a PostgreSQL adatbázisban pedig hozzáadtuk az `agent_run.web_reads text[] NOT NULL DEFAULT '{}'` oszlopot (`00027_run_web_reads.sql`).
   - *Indoklás:* A szerződés-első (contract-first) megközelítésnek megfelelően a kliens (őr) közvetlenül a szervernek jelenti az olvasott URL-eket, a szerver tárolja és visszaadja őket az áttekintő és futáslekérdező végpontokon, így a felület `🌐 N` badge-ként, kényelmes hover tooltipben tudja megjeleníteni a teljes URL listát az adminisztrátor számára.

3. **`NO_PROXY=127.0.0.1,localhost` beállítása a sandboxon belül:**
   - *Döntés:* Amikor `opts.AllowWebRead` aktív, a sandbox környezetébe a `HTTP_PROXY=http://127.0.0.1:3128` mellett bekerül a `NO_PROXY=127.0.0.1,localhost` (és kisbetűs párja) is.
   - *Indoklás:* A valódi Claude Code klienssel végzett tesztelés során kiderült, hogy a Claude beépített HTTP kliense a `http://127.0.0.1:3129/mcp` kérést is a 3128-as HTTP proxyn keresztül próbálta elküldeni. A proxy szigorú anti-SSRF védelme azonban azonnal megtagadja a loopback címekre irányuló kapcsolatokat! A `NO_PROXY=127.0.0.1,localhost` megadásával a kliens közvetlenül a konténer belső loopbackjén figyelő hídhoz kapcsolódik a 3129-es porton. Mivel a `bwrap --unshare-net` teljesen elszigeteli a hálózati névteret, a konténer loopbackje kizárólag a konténeren belüli híd-folyamatot éri el, a gazdagép loopback interfészét nem.

4. **Antigravity integráció és Streamable HTTP:**
   - *Döntés:* Az Antigravity (`agy`) zökkenőmentesen támogatja a Streamable HTTP MCP szervereket sandboxos üzemmódban is. A konfigurációt a sandboxban lévő `~/.gemini/config/mcp_config.json` fájlba írjuk (`url: "http://127.0.0.1:3129/mcp"`), az engedélyt pedig a futásonként frissített `settings.json`-ban adjuk meg (`perms["allow"] = []any{"mcp(colony-web/*)"}`). A valódi `agy` tesztfutás azonnal és sikeresen felismerte és használta az eszközt.

---

## 4. Mi maradt ki (tudatos korlátok)

- **Írási műveletek és interakció:** Kizárólag `GET` kérés támogatott. Űrlapok kitöltése, `POST`/`PUT`/`DELETE`, sütik és munkamenetek nem támogatottak.
- **Nem szöveges állományok mély elemzése:** PDF, bináris, képek esetén nem végzünk OCR-t vagy dokumentumelemzést; csak a fájltípust és méretet rögzítő rövid értesítés jelenik meg ("not a text page: ...").
- **Gyorsítótár perzisztenciája:** A letöltött oldalak cache-e csak a munkafutás (work run) élettartamáig él a memóriában; futások között nem marad meg.

---

## 5. Teszteredmények

### A) `make test`
```
go test ./...
ok   colonyhall/cmd/colonyhall (cached)
ok   colonyhall/internal/e2e 10.195s
ok   colonyhall/internal/guard 6.846s
ok   colonyhall/internal/httpserver (cached)
ok   colonyhall/internal/modules/agentrules (cached)
ok   colonyhall/internal/modules/conversation (cached)
ok   colonyhall/internal/modules/delivery/mcptools (cached)
ok   colonyhall/internal/modules/files (cached)
ok   colonyhall/internal/modules/files/mcptools (cached)
ok   colonyhall/internal/modules/files/storage (cached)
ok   colonyhall/internal/modules/git (cached)
ok   colonyhall/internal/modules/git/provider (cached)
ok   colonyhall/internal/modules/identity (cached)
ok   colonyhall/internal/policy (cached)
ok   colonyhall/internal/storetest (cached)
PASS (összes teszt sikeres)
```

### B) `make lint`
```
golangci-lint run ./...
0 issues.
```

### C) `make gen-check`
```
make gen
go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
docker run --rm -u 1000:1000 -v "/mnt/g/Projects/new-chat-worktrees/web-read":/src -w /src sqlc/sqlc:1.31.1 generate
git diff --exit-code -- internal/apispec $(find internal -type d -name sqlcgen)
(0 eltérés, generált kód naprakész)
```

### D) `go test -race ./internal/guard/ ./internal/e2e/`
```
ok   colonyhall/internal/guard 7.676s
ok   colonyhall/internal/e2e 46.982s
PASS (0 race condition, hibátlan lefutás)
```

### E) `GOFLAGS=-count=1 make test-pg` (Postgres ellen, uncached)
```
COLONY_TEST_DATABASE_URL='postgres://colony:colony@localhost:5433/colony_test?sslmode=disable' go test ./internal/storetest/ -run TestPostgres -v
=== RUN   TestPostgres
=== RUN   TestPostgres/eventlog
=== RUN   TestPostgres/identity
=== RUN   TestPostgres/conversation
=== RUN   TestPostgres/delivery
=== RUN   TestPostgres/audit
=== RUN   TestPostgres/hosts
=== RUN   TestPostgres/runs
=== RUN   TestPostgres/schedule
=== RUN   TestPostgres/approval
=== RUN   TestPostgres/wiki
=== RUN   TestPostgres/question
=== RUN   TestPostgres/files
=== RUN   TestPostgres/git
=== RUN   TestPostgres/preview
=== RUN   TestPostgres/agentrules
=== RUN   TestPostgres/notes
=== RUN   TestPostgres/decisions
=== RUN   TestPostgres/leases
=== RUN   TestPostgres/sidebar
--- PASS: TestPostgres (2.23s)
    --- PASS: TestPostgres/hosts (0.13s)
    --- PASS: TestPostgres/runs (0.11s)
PASS
ok   colonyhall/internal/storetest 2.277s
```

---

## 6. Kézi ellenőrzés (Manual Check) valódi `claude` és `agy` CLI-vel

A tesztek futtatása a gazdagépen, valódi `bwrap` izolációban és sandbox környezetben történt (`COLONY_MANUAL_CHECK=1 go test -v ./internal/guard/ -run TestManualWebReadCheck`):

### 1. Claude Code sandboxed work run: `https://pkg.go.dev/net/http` és `http.MaxBytesReader`
- **Kliens:** Claude Code (`claude`)
- **Kérés:** `"read https://pkg.go.dev/net/http and tell me what http.MaxBytesReader does"`
- **Kliens viselkedése:** Az agent észlelte a rendelkezésére álló `mcp__colony-web__web_read` eszközt, meghívta az URL-lel, feldolgozta a visszakapott tiszta szöveges tartalmat, és pontos választ adott:
  > *„`http.MaxBytesReader` returns an `io.ReadCloser` that wraps an underlying `ReadCloser` (`r`) but limits the amount of data that can be read to at most `n` bytes. It is primarily used in HTTP request handlers to protect server resources against excessively large request bodies sent by clients.”*
- **Válaszlábléc:** Az őr automatikusan hozzáfűzte: `🌐 web_read: pkg.go.dev (1 page)`.

### 2. Antigravity sandboxed work run: `https://pkg.go.dev/net/http` és `http.MaxBytesReader`
- **Kliens:** Antigravity (`agy`)
- **Kérés:** `"read https://pkg.go.dev/net/http and tell me what http.MaxBytesReader does"`
- **Kliens viselkedése:** Az `agy` a generált `mcp_config.json` alapján betöltötte a `colony-web` szervert, meghívta a `web_read` eszközt, és az alábbi pontos magyarázatot adta:
  > *„`http.MaxBytesReader` wraps an `io.ReadCloser` to limit the number of bytes read from a response or request body to a maximum of `n` bytes. When `Read` is called on the returned `ReadCloser`, it will return an error (specifically, a `*MaxBytesError`) if the caller attempts to read more than `n` bytes. It prevents clients from sending excessively large request bodies (DoS attack prevention).”*
- **Válaszlábléc:** Az őr automatikusan hozzáfűzte: `🌐 web_read: pkg.go.dev (1 page)`.

### 3. Privát IP-cím blokkolása: `http://100.85.111.88:3002` (homelab Gitea)
- **Kérés:** `"read http://100.85.111.88:3002"`
- **Kliens és őr viselkedése:** Az MCP eszköz azonnali hibával tért vissza:
  ```
  fetch failed: ip literals are not allowed: 100.85.111.88
  ```
  A privát / homelab hálózathoz a kérés egyáltalán nem ment ki; a kliens közölte a felhasználóval, hogy a belső IP-címek olvasása biztonsági okokból tiltott.

### 4. Localhost blokkolása: `http://127.0.0.1:8080`
- **Kérés:** `"read http://127.0.0.1:8080"`
- **Kliens és őr viselkedése:** Az MCP eszköz azonnali hibával tért vissza:
  ```
  fetch failed: ip literals are not allowed: 127.0.0.1
  ```
  A kliens nem érte el a helyi portot.

---

## 7. Mit nézzen meg az átnéző (Reviewer Guide)

1. **`internal/guard/webread.go`:**
   - A `Fetch` függvény szerkezete: anti-SSRF ellenőrzések, IP-címek azonnali kizárása (nincs kivétel a Colonyhall szervernek sem).
   - A `DialContext` pinned IP csatlakozása (DNS rebinding megelőzése).
   - A HTML-ből szöveg parser (`golang.org/x/net/html`), amely kiszűri a script, style, nav, footer, form tageket, de szépen formázza a címeket, táblázatokat, kódblokkokat és abszolút linkeket.
   - Méretkorlátok (5 MB választörzs) és futási korlátok (30 kérés / 10 MB per run).
2. **`internal/guard/webread_mcp.go`:**
   - A Streamable HTTP MCP szerver és a `web_read` eszköz deklarációja.
   - Paging kezelése Unicode rúnák mentén (`offset`, `max_chars`).
   - Futáson belüli in-memory gyorsítótár a kérések minimalizálására.
   - Prompt injection védelem: `<web_content url="..." fetched_at="...">...</web_content>` csomagolás és a belső `</web_content>` lezárások escape-elése.
3. **`internal/guard/bridge.go` és `sandbox.go`:**
   - A multi-bridge támogatás: 3128-as port a `proxy.sock`-ra, 3129-es port a `webread.sock`-ra.
   - `NO_PROXY=127.0.0.1,localhost` beállítása a sandboxon belül Claude Code esetén.
   - Claude Code (`--mcp-config`) és Antigravity (`~/.gemini/config/mcp_config.json` + `settings.json`) konfigurálása a sandboxban.
4. **Szerveroldali kiterjesztések:**
   - `migrations/00026_web_read.sql` és `migrations/00027_run_web_reads.sql`.
   - `internal/modules/hosts/` (`allow_web_read` validáció és perzisztencia).
   - `internal/modules/runs/` (`web_reads` tömb mentése és visszaadása).
   - `internal/httpserver/web/index.html` (beállítási űrlap, futáslista és áttekintő panel `🌐 N` badge tooltip listával).

---

## Átnézés (Claude Code, 2026-10-22)

Ellenőrizve: `make test`, `make lint`, `make gen-check`, `make test-pg` (gyorsítótár nélkül; a dev szerver-image ezen a gépen most nem épült — a Docker-build nem érte el a Go modultárat a Tailscale DNS miatt —, ezért csak a Postgres-konténer futott, a tárteszteknek ennyi kell), `go test -race ./internal/guard/ ./internal/e2e/`: mind zöld. A letöltő rendben: minden átirányítási lépés a transport-rétegben ugyanazon az ellenőrzésen megy át, a kapcsolat az ellenőrzött IP-re megy, nincs környezeti proxy, nincs süti, a válasz mérete korlátos.

Javítva:

- **`open` hálózati módban** a kliens akkor is megkapta a `web_read` MCP-beállítást (`127.0.0.1:3129`), pedig ott nincs sandboxon belüli híd: a cím a gép saját loopbackje lett volna, bármi, ami ott figyel. Most `open` módban a `web_read` nem kapcsol be (a sandbox és a prompt sem ígéri). Teszt: `TestWebReadOffWithoutNetworkIsolation` (a javítás nélkül elbukik).
- **A `<web_content>` keret lezárása** csak a pontos kisbetűs alakban volt kiszűrve; most kis-/nagybetűtől és szóközöktől függetlenül. Teszt: `TestWebContentCloseVariantsEscaped`.
