# 39. kör jelentése: CodeGraph minden agentnek, az őr köti be

Ág: `feature/codegraph` (worktree: `/mnt/g/Projects/new-chat-worktrees/codegraph`), a `main` `566ab51`-jéről. Nincs merge és nincs push. Migráció nem kellett.

## Commitok

| Hash | Leírás |
| --- | --- |
| `5de3611` | Add CodeGraph to the host report |
| `4993161` | Wire CodeGraph into every run from the guard |
| `3db9a3a` | Show CodeGraph on the Machines page |
| `a88debf` | Add a manual test with the installed CodeGraph |
| `269fe23` | Document CodeGraph: wiring, indexing, operations, the colleague |
| (ez) | Add the round 39 report |

## Mi készült el

1. **Beállítás és felismerés** (`guard.yaml` `codegraph:`; ennek részei: `bin`, `enabled`, `max_files`):
   - induláskor `codegraph --version` fut;
   - a gépjelentésben (`HostReport.codegraph`, szerződés-elsőként) ott a verzió és az indexelt repók, legfeljebb 50, mindegyikhez az állapot, a fájlszám, az utolsó indexelés és a hiba vagy kihagyás oka;
   - a Gépek oldalon egy új „CodeGraph” sor mutatja ezeket;
   - ha nincs bináris, vagy `enabled: false`, semmi nem változik.
2. **Indexelés (az őr):**
   - csak repógyökeret indexel, gyűjtőmappát soha;
   - egyszerre egy feladat fut, `nice` és `ionice` alatt;
   - új repóban `init`, utána `sync` a futás elején; aszinkron, a futás nem vár rá;
   - a `max_files` fölötti repót kihagyja;
   - hiba után egy órát vár, nem próbálkozik ciklusban;
   - **direkt agentnél:** a gyökér, utána azok a repók, amelyekben a futás lépései dolgoztak;
   - **sandboxos agentnél:** az írható mappák és a worktree.
3. **MCP-szerver minden work- és direkt futásban:**
   - **Claude Code:** `alwaysLoad`, és `--allowedTools mcp__codegraph`. A csak dokumentációt író tervező is megkapja; a settings allow listájában is benne van, a `.md`-n kívüli írás továbbra is tiltva.
   - **Antigravity:** a futás saját MCP-konfigjában, sandboxban `mcp(codegraph/*)` engedéllyel.
   - **Sandboxban:** a bundle csak olvasható mount, a szerver bent fut.
   - Minden CodeGraph-folyamat környezete: `CODEGRAPH_TELEMETRY=0`, `DO_NOT_TRACK=1`, `CODEGRAPH_NO_UPDATE_CHECK=1`, `CODEGRAPH_NO_DAEMON=1`, `CODEGRAPH_DIR=.codegraph`.
4. **Soha nem kerül gitbe:**
   - a `.git/info/exclude` kap egy `.codegraph*/` sort;
   - a sandboxos commit pathspecje is kizárja ezeket a mappákat. Teszt bizonyítja, hogy akkor is kimaradnak, ha az agent kiürítette a CodeGraph `.gitignore`-ját, és az adatbázis 11 MB.
5. **Tipp az agenteknek:** egy rövid szakasz a futás szabályaihoz („egyszer `codegraph_explore`, szűk kérdéssel, utána csak a megjelölt sorok”), csak ha a futásnak van CodeGraph-ja.
6. **Dokumentáció:**
   - `docs/02` „CodeGraph minden agentnek”;
   - `docs/06` (hol vannak az indexek, újraindexelés, frissítés a három gépen, kikapcsolás);
   - `docs/10` (CodeGraph a kollégának, `codegraph install` nélkül);
   - `guard.example.yaml`, `task.md`.

## A csak olvasható mappa (3. rész kérdése)

**Eredmény: nem működik, ezért ezek a mappák nem kapnak CodeGraph-ot.**
- A CodeGraph 1.6.2 MCP-szervere egy csak olvasható mappában lévő indexnél el sem indul: `EROFS: read-only file system, open '.../.codegraph/writer.pid.2.tmp'`. Indításkor zárfájlt ír az adatbázis mellé.
- Ugyanez írható mountnál működik. Mindkettőt valódi bwrap alatt próbáltam ki, eldobható repón.
- Kerülőutak, amik nem járhatók:
  - a bwrap `--tmp-overlay` kapcsolója a 0.10-es verzióban jött, a gépen 0.9.0 van;
  - a `CODEGRAPH_DIR` csak mappanevet fogad el, útvonalat nem, így az index nem tehető máshová.
- **Döntés (a prompt szerint):** a sandboxos agent csak olvasható mappája nem kap CodeGraph-ot, és az őr nem is indexeli. A jogosultság nem bővül.
- **Pontos szabály:** a futás csak akkor kapja meg a szervert, ha a munkamappája írható (worktree vagy írható engedély); indexelni csak az írható engedélyeket indexeli.
- **Lehetséges későbbi megoldás:** futásonként egy írható másolat az index adatbázisáról az agent saját mappájában, a csak olvasható repó `.codegraph`-jára mountolva. Bonyolultabb, és egy nagy repónál futásonként kb. 50 MB másolás, ezért nem csináltam meg.

## Ahol a prompt mást írt (és miért)

- **`codegraph init --local-only` nincs** a v1.6.2-ben. A CodeGraph magától ír egy `.codegraph/.gitignore`-t, ami az adatbázist kizárja, önmagát viszont nem. Ezért egy `git add -A` behúzná. Az őr ezért a repó `.git/info/exclude`-jába vesz fel egy `.codegraph*/` sort; ez helyi, nem commitolódik, és a worktree-kre is érvényes. Helyette `init --yes`.
- **`CODEGRAPH_DIR=.codegraph` rögzítve.** Windows-meghajtón (`/mnt/g`) a CodeGraph új projektnél magától `.codegraph-wsl`-t választana, és akkor a kizárások és a mountok rossz mappát céloznának.
- **A sandboxos agent repóját az őr saját sandboxban indexeli** (`Sandbox.IndexCommand`). Ezt a prompt nem kérte, biztonsági okból tettem bele.
  - **Ok:** az agent írhatja a repót, és ha az őr sandbox nélkül indexelne, egy elültetett link (`leak.txt -> ~/.ssh/...`) behúzhatna egy külső fájlt az indexbe, amit az agent aztán `codegraph_explore`-ral kiolvashatna.
  - **Megoldás:** az indexelő sandboxa csak a repót látja, csak olvashatóan, hálózat nélkül. Egyedül a `.codegraph/` írható.
  - Valódi bwrap-teszt bizonyítja: a link célja nem olvasható, a repó nem írható.
- **A tipp nem a Szabályok oldalra került, hanem az őr fűzi a futás szabályaihoz.** A közös szabályok az adatbázisban vannak (a te élő, szerkesztett szöveged), ahhoz nem nyúltam, a beépített alapszöveg pedig csak új telepítésnél számít. Így a tipp csak ott jelenik meg, ahol a CodeGraph tényleg elérhető. Ha inkább a Szabályok oldalon szeretnéd, a szöveg a `CodeGraphHint`-ben van (`internal/guard/codegraph.go`).
- **`--no-watch`** az MCP-szervernél. A CodeGraph maga írja, hogy a watcher lassú a WSL `/mnt` meghajtóin; a kapcsolódáskori felzárkózás és az őr futás eleji `sync`-je frissen tartja az indexet.
- **Nem kapnak CodeGraph-ot:**
  - az agents mód (a régi, géppárosítás nélküli konfig); mindhárom gép host módban fut;
  - a sandbox nélküli Antigravity work-futás. Az agy-nak nincs MCP-konfig kapcsolója, és a felhasználó saját konfigjához nem nyúlunk; a direkt agy-futások megkapják.

## Valódi futás

- **A valódi CodeGraph 1.6.2 az őr kódútjain,** a PC-n (`COLONY_MANUAL_CODEGRAPH=1 go test ./internal/guard/ -run TestManualCodeGraph -v`):
  - az indexelő sandboxában beindexelt egy eldobható repót;
  - az index nem látszott a `git status`-ban;
  - utána egy sandboxos work-futás mountjaival elindított MCP-szerver válaszolt egy `codegraph_explore` hívásra (8,1 KB, a `main.go` forrásával). 1,5 mp.
- **Valódi agentfutás (Claude Code a `codegraph_explore`-ral, tokenek a Futások oldalról): nem csináltam meg.** Az alapszabály szerint valódi `claude`/`agy` futás csak jóváhagyással mehet, és az élesítés is hozzá tartozik. Recept a jóváhagyásod után:
  1. élesítés (lent);
  2. a PC-n egy direkt agentnek, aminek a gyökere ez a repó, egy kérdés, például: „Hogyan dönti el az őr, hogy egy futás kap-e CodeGraph-ot?”;
  3. a Futások oldalon a lépések között ott lesz a `codegraph_explore`, és az eszközhívások száma és a tokenek összevethetők egy korábbi, hasonló futáséval.

## Élesítés sorrendje

1. **Szerver először.** A gépjelentés új mezőt kap, a régi szerver pedig a szigorú dekódolás miatt elutasítaná az új őr jelentését.
2. **Utána mindhárom őr** (PC, homelab, nato), a `CLAUDE.md` receptje szerint, futó agentmunka nélkül.
   - A CodeGraph már telepítve van, kikapcsolt telemetriával.
   - A `guard.yaml`-hez nem kell nyúlni: az alapértelmezett `~/.local/bin/codegraph` mindhárom gépen megvan.
   - A homelabon az őr systemd-szolgáltatásként fut, abszolút útvonallal keresi, a `PATH` nem számít.
3. **Első indexelések:** az első futások után a Gépek oldalon megjelennek a repók. A Colonyhall repó (807 fájl) a homelabon 11 mp alatt indexelődött, a PC-n a `/mnt/g` miatt lassabb lehet.
4. **A kollégánál** (ha már fut): a `docs/10` új CodeGraph-lépése.

## Ellenőrzés

A teljes `make ci` zöld (16/16, 195 mp falidő):

| Job | eredmény | mp |
| --- | --- | --- |
| go-test | ok | 48 |
| go-e2e | ok | 51 |
| go-race | ok | 195 |
| go-pg | ok | 17 |
| go-lint | ok | 16 |
| web-typecheck | ok | 30 |
| web-lint | ok | 33 |
| web-test | ok | 31 |
| devpage-test | ok | 1 |
| shellcheck | ok | 3 |
| actionlint | ok | 11 |
| vuln | ok | 14 |
| install-test | ok | 42 |
| gen-check | ok | 13 |
| build | ok | 26 |
| docker-image | ok | 79 |

**Új tesztek** (`internal/guard/codegraph_test.go`, egy hamis `codegraph` szkripttel, ami minden hívást a környezetével együtt rögzít):
- felismerés: nincs bináris, ki van kapcsolva, nem fut;
- `init`, majd `sync` a helyes repóra, a kötelező környezettel; az exclude sor egyszer kerül be, a `git status` tiszta;
- csak repógyökér: gyűjtőmappa, almappa és relatív út nem;
- `max_files` kihagyás;
- egyszerre egy feladat, és duplán nem kerül sorba;
- hibánál nincs ciklus, csak a `RetryAfter` után;
- repók a lépésekből;
- a tipp;
- MCP-konfigok (Claude `alwaysLoad`, env; agy; a csak CodeGraph-os konfigban nincs token);
- `--allowedTools` a work-, a direkt és a csak dokumentációt író futásokban (az utóbbiban az írás továbbra is csak `.md`);
- sandbox: a bundle csak olvasható, CodeGraph nélkül nincs mount, mindkét kliens konfigja;
- az indexelő sandbox argumentumai (nincs hálózat, csak az index írható, a linkelt `.codegraph` elutasítva), és egy valódi bwrap-futás;
- a commit nem viszi be az indexet;
- a host policy (csak olvasható munkamappánál nincs CodeGraph);
- e2e: a jelentés átmegy és vágódik, ismeretlen állapotból `failed` lesz;
- PWA: a Gépek oldal sora.

## Mit nézzen meg először az átnéző

1. **A sandbox-mountok:**
   - `CommandWithOptions`: a bundle `--ro-bind`-ja, és csak akkor, ha a futásnak van CodeGraph-ja;
   - `IndexCommand`: egyedül a `.codegraph` `--bind`, a repó és a git-metaadat csak olvasható, `--unshare-net`;
   - a `TestIndexInSandboxSeesOnlyRepo` valódi bwrap-pal bizonyítja, hogy a link és a külső fájl nem olvasható.
2. **A commit-szűrő:** `commitPathspec` és `isCodeGraphPath` a `worktree.go`-ban; mellette a `.git/info/exclude` írása (`excludeCodeGraph`, linket nem követ).
3. **Az indexelési korlátok:** csak repógyökér (`prepare`), `max_files`, egy feladat egyszerre, `RetryAfter` és `MinSync` (`Ensure`), alacsony prioritás (`lowPriority`; az `ionice`-t csak akkor használja, ha ki tudja próbálni).
4. **A csak dokumentációt író tervező:** a `docsOnlyArgs` és a `DocsOnlySettings` most `mcp__codegraph`-ot is enged; az írás szabályai változatlanok.
5. **Frissítés:** a `codegraph upgrade` kézi lépés marad, és utána az őrt újra kell indítani, mert a bundle útvonala verziónként más, és az őr induláskor olvassa be.
