# 42. kör jelentése: CI, minél párhuzamosabban

Ág: `feature/ci` (worktree: `/mnt/g/Projects/new-chat-worktrees/ci`). **A `feature/installer`-re épül** (abból ágazik, és a `main` be van olvasztva), mert a CI a telepítő kör `release.yml`-jére, `shellcheck`-jére és `test-install`-jára épít. Beolvasztás: előbb a `feature/installer`, utána ez. Nincs push, és a GitHubon még nem futott.

## Commitok

| Hash | Leírás |
| --- | --- |
| `67d8700` | Make the shell scripts executable in git |
| `8799510` | Measure the parser's pathological-line test in CPU time |
| `2197ff8` | Add one make target per CI job and pin the tool images |
| `9e0ba37` | Add the CI workflow: parallel jobs, a path filter and one gate |
| `8f3a643` | Add make ci and make ci-quick: the CI's jobs locally, in parallel |
| `0dc4399` | Give the parser's pathological-line test a 2 s CPU bound |
| `b2f411d` | Run make ci-quick on a snapshot too |
| `118ce39` | Document the CI and make ci |
| (ez) | Add the round 42 report |

## A jobok gráfja

```
                 ┌─ go-test ─────────┐
                 ├─ go-e2e ──────────┤
                 ├─ go-race¹ ────────┤
                 ├─ go-pg (Postgres) ┤
                 ├─ go-lint ─────────┤
                 ├─ gen-check ───────┤
                 ├─ web-typecheck ───┤
 changes ────────┼─ web-lint ────────┼──► ci-ok  (az egyetlen kötelező ellenőrzés)
 (útvonalszűrő)  ├─ web-test [1]² ───┤
                 ├─ devpage-test ────┤
                 ├─ shellcheck ──────┤
                 ├─ actionlint ──────┤
                 ├─ vuln³ ───────────┤
                 ├─ install-test ────┤
                 └─ build ──┬─ docker-image ─┤
                            └─ e2e-browser⁴ ─┘
```

1. A race detector csak két esetben fut: a `main`-re pusholt Go-változásnál, illetve éjjel. Pull requestnél csak akkor, ha maga a CI változik.
2. Mátrix, ma egyetlen szelettel (lásd lent).
3. Csak függőségváltozásnál és éjjel fut.
4. Előkészítve, kikapcsolva.

**Útvonalszűrő** (`scripts/ci-changes.sh`, harmadik féltől származó action nélkül):
- csak dokumentáció változik → csak a `ci-ok` fut, és zöld;
- csak `web/` → a Go-jobok kimaradnak;
- `deploy/` → `build` és `docker-image`;
- ismeretlen fájl → a Go-jobok futnak;
- a CI saját fájljai → minden fut.

**`ci-ok`** (`scripts/ci-ok.sh`) piros lesz, ha
- egy job elbukott vagy megszakadt;
- egy job kimaradt, pedig a szűrő szerint futnia kellett volna;
- egy job hiányzik a listájából.

A böngészős teszt bekapcsolásakor a `ci-ok`-on nem kell változtatni: a `browser` szűrőkimenet dönt róla.

**Egy döntés, ahol a prompt mást írt:** az `install-test` nem vár a `build`-re. A tesztek hamis kiadást szolgálnak ki, és a tesztbinárist maguk fordítják, így nincs szükségük a build artefaktjára, és azonnal indulhatnak.

## Mért idők

**Egyenként, a PC-n (32 mag, WSL2, a repó a `/mnt/g`-n):**

| Ellenőrzés | Idő |
| --- | --- |
| Go-tesztek az e2e nélkül | 58 mp |
| e2e | 29 mp |
| race, minden csomag | 194 mp (ebből az e2e 156 mp, a guard 17 mp) |
| lint | 21 mp |
| PWA típusellenőrzés | 15 mp |
| PWA lint | 47 mp |
| fejlesztői oldal | 1 mp |
| PWA build | 29 mp |
| Go build | 17 mp |
| gen-check | 20 mp |
| `npm ci` | 96 mp |

**A Vitest a fő tanulság:**
- A `/mnt/g`-n 2 workerrel **1246 mp (21 perc)** volt, aminek 78%-a a jsdom indítása. A 9p fájlrendszer miatt lassú, a tesztek nem lassúak.
- Ugyanez ext4-en (`/tmp`): **40 mp** 2 workerrel, 22 mp 4 workerrel.
- A szeletelés ext4-en, 2 workerrel, szeletenként: 2 szelet 20 mp, 3 szelet 15 mp, 4 szelet 11 mp.
- A szeletelés tehát lineárisan gyorsít, de egy szelet saját előkészítése (checkout, Node, `npm ci`) a runneren 30–60 mp, és a GitHub jobonként percre kerekítve számláz. 40 mp-es tesztsornál a második szelet többe kerül, mint amennyit hoz. **Ezért ma egy szelet.** A mátrix megmaradt, így a szeletek száma a `shard` lista és a `SHARDS` együttes emelésével növelhető, amikor a tesztsor néhány perc fölé nő.
- A „5–6 perc” a promptban a `/mnt/g`-n, 8 workerrel mért szám volt.

**`make ci` párhuzamosan, a végső futás** (pillanatkép ext4-en, 32 mag):

| Job | mp |
| --- | --- |
| go-test | 46 |
| go-e2e | 49 |
| go-race | 189 |
| go-pg | 15 |
| go-lint | 14 |
| web-typecheck | 30 |
| web-lint | 32 |
| web-test | 29 |
| devpage-test | 1 |
| shellcheck | 3 |
| actionlint | 11 |
| vuln | 13 |
| install-test | 40 |
| gen-check | 12 |
| build | 25 |
| docker-image | 69 |
| **falidő** | **189 mp** (a race határozza meg; nélküle kb. 70 mp) |

- `make ci-quick` (pillanatképen, egy változott tesztfájllal): **33 mp**. Ugyanez helyben a `/mnt/g`-n 99 mp volt.
- **Lefedettség** (csak tájékoztatásul, nincs küszöb): Go 69,0% (`go test ./... -coverpkg=./internal/...,./cmd/...`). A PWA lefedettségéhez kellene a `@vitest/coverage-v8`, ami nincs telepítve, ezért nem mértem.

## Percbecslés a GitHubon (privát repó)

**Becslés, nem mérés:** a GitHubon még nem futott. A privát repók Linux-runnere 2 magos. A számlázás jobonként, percre felfelé kerekítve történik. Egy job saját előkészítése 30–60 mp.

**Számlázott percek futásonként (becslés):**

| Változás | Számlázott perc | Falidő |
| --- | --- | --- |
| csak dokumentáció | 2 (`changes` és `ci-ok`) | |
| PWA | kb. 14 | |
| Go és PWA, pull request | kb. 28 | kb. 8 perc (a `build` → `docker-image` lánc) |
| ugyanez a `main`-re pusholva, race detectorral | kb. 36 | kb. 10 perc |
| éjszakai futás (race és vuln) | kb. 12 | |

**Havi becslés, napi 10 push** a `main`-re (ma Norbi így dolgozik), ha mindegyik kódváltozás:
- 10 × 36 × 30 ≈ **10 800 perc**, plus éjjel 30 × 12 ≈ **360 perc**;
- ha a pushok fele csak dokumentáció: kb. 5 700 perc.

A **GitHub Free 2 000**, a **Pro 3 000** ingyenes percet ad havonta privát repóra. Fölötte percenként fizetsz; az aktuális árat a GitHub „Actions billing” oldalán érdemes megnézni.

**Nyilvános repóban a standard runnerek ingyenesek** (és 4 magosak). A nyílt forráskódúvá tétel után tehát ez a költség megszűnik.

Amíg privát, három dolog csökkenti a költséget:
- kevesebb, összevont push;
- pull requestek a `main` helyett (ott nincs race);
- a kis jobok (`devpage-test`, `shellcheck`, `actionlint`) összevonása egy jobba. A prompt külön jobot kért, és a szűrő miatt ritkán futnak, ezért nem vontam össze őket.

Az első valódi futás után a repó *Settings → Billing / Actions usage* oldala mutatja a pontos perceket.

## Ellenőrzés push nélkül

- **`actionlint` 1.7.12** (Docker, rögzítve): tiszta. **`shellcheck` v0.11.0**: tiszta (az összes `scripts/*.sh`, az új `scripts/ci*.sh` is).
- **`make ci-selftest`:** 19 szűrőeset, 10 kapueset, és egy ellenőrzés arra, hogy minden szkript futtatható a gitben. Az `actionlint` job futtatja.
- **A `changes` lépés shell-kódját** a workflow-ból kivéve lefuttattam erre az ágra. Mind az öt esemény a várt kimenetet adta: pull request a `main` ellen; push; push nulla „before”-ral (ott „all”-ra vált); éjszakai futás; kézi indítás.
- **Minden job parancsa lefutott helyben** (`make ci`, a fenti táblázat). A workflow jobjai ugyanazokat a make targeteket hívják.
- **Az `act` nem futott:** a telepítését (`go install github.com/nektos/act@v0.2.97`) az engedélykezelő letiltotta, mert külső kódot futtatna. Ha engedélyezed, lefuttatom. Az `act` amúgy sem tudja a GHA cache-t és az artefaktokat úgy, mint a GitHub.

Amit így nem lehetett ellenőrizni, az első valódi futás mutatja meg:
- a `docker/build-push-action` GHA-cache-e (`cache-to: type=gha`) a szűk jogosultságokkal (`contents: read`);
- a `golangci-lint-action` v9 a v2.15.0-s linterrel;
- a service container Postgres;
- a valódi percek.

## Mellékesen talált hibák

1. **A szkriptek nem voltak futtathatók a gitben** (100644). A repó NTFS-en van `core.fileMode=false`-szal, így a `chmod` nem jutott el az indexbe. **A 41-es kör `release.yml`-je a GitHubon elbukott volna** a `scripts/release.sh` hívásánál („Permission denied”). Javítva. A `ci-selftest` mostantól figyeli, és a `make ci` pillanatképe (`git stash create`) az index módjait használja.
2. **Időfüggő teszt.** A markdown parser „20 000 karakteres sor < 500 ms” tesztje falidőt mért, és a párhuzamos `make ci` alatt elbukott (1,3 mp). Ez mindkét másolatot érintette: a fejlesztői oldalét és a PWA-ét.
   - CPU-idő alatt is elbukott: 707 ms.
   - Most a határ **2 mp CPU-idő**. A parser önmagában 20–250 ms-ot használ; a katasztrofális backtracking, ami ellen a teszt véd, ennél sokkal lassabb.
   - **Megfigyelés:** a PWA parserje erre a bemenetre enyhén szuperlineáris, duplázásonként kb. háromszoros idő (5k: 2 ms, 10k: 7 ms, 20k: 20 ms). Ma nem gond, mert 20 000 karakter fölött sima szövegként jelenik meg, de érdemes lehet egyszer megnézni.

## Branch protection: a lépések Norbinak

**Előfeltétel:** **privát repón a branch protection és a ruleset csak GitHub Pro, Team vagy Enterprise csomaggal érhető el.** Ingyenes csomaggal privát repón ez a rész a nyilvánossá tételig vár. A `ci-ok` addig is fut, csak nem kötelező.

Lépések (Settings → Rules → Rulesets → New branch ruleset):
1. **Név:** `main`. **Enforcement:** Active. **Target:** a default branch.
2. **Bypass list:** *Repository admin* (te), „Always”. **Ma te pusholsz közvetlenül a `main`-re, és ez így maradhat:** az agentek nem pusholnak, kolléga még nem ír a repóba. A bypass mellett a közvetlen push továbbra is működik. A CI utólag lefut rá, a piros `main` pedig e-mailt és jelzést ad.
3. **Rules:**
   - *Restrict deletions*;
   - *Block force pushes*;
   - *Require status checks to pass*, ezen belül:
     - `ci-ok` (forrás: GitHub Actions). Csak akkor választható ki, ha már egyszer lefutott, ezért előbb pusholj egyet;
     - a *Require branches to be up to date before merging* **ne** legyen bekapcsolva. Minden más merge után újrafuttatást kérne, ami pénz; később merge queue-val jobb.
4. **Ha a kolléga vagy külső hozzájárulók írnak:**
   - *Require a pull request before merging*, 1 jóváhagyással;
   - a bypass maradhat neked, vagy veheted szigorúbbra.
5. A jelvény a README tetején már benne van (`.../actions/workflows/ci.yml/badge.svg?branch=main`). Privát repón csak bejelentkezve látszik.

## Ami kimaradt, és miért

- **Böngészős e2e tesztek:** a prompt szerint még nem kellenek. A terv a `docs/02` „CI” részében van:
  - Playwright a `build` artefakt bináris ellen, memóriabeli tárakkal és dev seeddel;
  - szerep- és címkeszelektorok;
  - négy útvonal: belépés, üzenet, szál, admin oldalak.

  A job helye a workflow-ban kész. Bekapcsolni a `ci-changes.sh` egy sorával és a job lépéseivel lehet.
- **Self-hosted runner:** nincs. Nyilvános repón idegen pull requestek kódja futna a gépeiden.
  - **Akkor lenne értelme,** ha a privát időszakban a percek túl drágák, vagy ha GPU-s tesztek kellenek (47. kör).
  - **Ami kellene hozzá:**
    - egy külön, eldobható VM vagy konténer a homelabon, saját felhasználóval, a hálózatból elszigetelve;
    - `--ephemeral` runner;
    - csak privát repóhoz, vagy csak `main`-push eseményre;
    - soha nem az élő szerver vagy az őrök gépe.
- **Telepítés a CI-ból** és **lefedettségi küszöb:** a prompt szerint nincs.
- **Átfedés a 40-es körrel:** a `make vuln` és a rögzített `govulncheck` v1.9.0 már itt elkészült. A 40-es kör csak hivatkozzon rá, a Dependabotot pedig az csinálja.

## Mit nézzen meg először az átnéző

1. **Jogosultságok** (`.github/workflows/ci.yml`):
   - a tetején `permissions: {}`, jobonként csak `contents: read`;
   - `persist-credentials: false`;
   - a `${{ }}` értékek csak `env:`-en át jutnak a shellbe;
   - nincs titok, nincs `pull_request_target`.
2. **Az útvonalszűrő és a `ci-ok`:**
   - a `scripts/ci-changes.sh` szabályai, főleg a „docs-only” eset és az, hogy az ismeretlen fájl Go-nak számít;
   - a `scripts/ci-ok.sh` „kimaradt, pedig kellett volna” ága;
   - a `ci-ok` `needs` listája és a szkript táblája összhangban van (a szkript jelzi, ha nem).
3. **Az eldobható Postgres a `make ci`-ben** (`scripts/ci.sh`):
   - `--tmpfs`, véletlen port csak `127.0.0.1`-en;
   - `docker rm -f` kilépéskor, megszakításkor is;
   - a dev stack `colony_test` adatbázisához nem nyúl.
4. **A pillanatkép és a csomag-cache** (`scripts/ci.sh`):
   - `git stash create` + a nem követett fájlok;
   - a `~/.cache/colonyhall-ci/web-<lock-hash>` `node_modules`-ra mutató link;
   - a `touch`, ami megakadályozza, hogy egy al-make `npm ci`-t futtasson a linken át. Ez egyszer kiürítette a cache-t, mielőtt javítottam.

## Végső eredmények

A teljes `make ci` (a fenti táblázat) 16/16 jobbal zöld, 189 mp falidővel. Benne van a `go test ./...`, a `golangci-lint`, a `test-pg` (eldobható Postgres), a `gen-check`, a race detector minden csomagon, a PWA típusellenőrzés, lint és Vitest, a fejlesztői oldal tesztjei, a `shellcheck`, az `actionlint`, a `ci-selftest`, a `govulncheck` és `npm audit` (nincs találat), az `install-test` (Debian, Ubuntu, Fedora), a PWA és a binárisok buildje, valamint a Docker-kép.
