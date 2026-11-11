# A Colonyhall nyílt forráskódúvá tétele: átvilágítás és teendők

> A 48. kör első része (döntési kapu). Állapot: **2026-11-11, `main` = `acfd588`**. A dokumentum titkot nem tartalmaz; ahol egy találat értéke számít, csak a fajtáját és a helyét írja le.
>
> Már eldöntött (Norbi, 2026-11-11): **`AGPL-3.0-only` + CLA (kettős licenc)**, szerzői jog: **Ujj Norbert**, egyetlen nyilvános elérhetőség: **norbert.ujj@gmail.com**; a valódi címek, gépnevek és élesítési parancsok **átkerülnek**, nem csak kicserélődnek (`docs/handoff/48-open-source-prompt.md`, 0.2).

## Összefoglaló

| Terület | Eredmény | Kell-e döntés |
|---|---|---|
| Titkok a git-előzményben | **Nincs valódi titok** (459 commit, minden ág) | Nem: az előzmény maradhat |
| Személyes adat a munkafában | Címek, gépnevek, útvonalak ~45 fájlban, ebből 68 handoff-fájl külön | Igen: mi marad privát (0.2) |
| Függőségek licence | Mind kompatibilis az AGPL-lel és a kereskedelmi licenccel | Nem |
| Kód más projektből | Nincs átvett kód; a Monaco MIT, a betűtípusok OFL | Nem |
| Szerzők | Egyetlen szerző (Norbi, két névvel) | Nem |
| Név | Szabadnak tűnik (GitHub, npm, Docker Hub, 6 domain) | Igen: domain, GitHub-szervezet |
| Telemetria | Nincs; csak a felhasználó által beállított helyekre kapcsolódik | Nem |
| Fejlesztői seed | Rögzített tokenek, ha valaki élesben bekapcsolja | Igen: szigorítás most vagy a 40. körben |

## 0.1 Titkok és személyes adatok

### Titokkeresés a teljes előzményen

- **Eszközök:** gitleaks 8.28 (`--log-opts=--all`) és trufflehog 3.90 (`git --bare`, élő ellenőrzéssel) a repó tükörmásolatán: 459 commit, minden ág (a `feature/rag` is), kb. 23 MB.
- **Eredmény:** gitleaks 9, trufflehog 110 találat, **ellenőrzötten működő titok: 0**. Mindegyik a (c) osztályba tartozik:
  - tesztadat: `internal/platform/secretscan/secretscan_test.go:12–15`, `internal/guard/step_test.go:44,49,74` (a jwt.io közismert mintatokenje, `secret_bearer_token`), `internal/e2e/git_test.go:73`, `internal/e2e/search_test.go:158` (a „test” szó SHA-256-jának eleje), `internal/setup/setup_test.go:259`, `internal/guard/webread_test.go:338`;
  - helyőrző: `deploy/.env.prod.example:10` (`CHANGE_ME`), `deploy/compose.dev.yaml:30` (a dev adatbázis ismert jelszava), `internal/setup/server.go:248` és `i18n.go:85` (hibaüzenet-minta);
  - téves riasztás: a beépített Monaco szerkesztő 102 „Github” találata (`internal/httpserver/web/vendor/monaco/`, a licencfejléc URL-je).
- **Titokfájl soha nem került a repóba:** az előzményben létrehozott fájlok közül csak `deploy/.env.example` és `deploy/.env.prod.example` illik a mintára; ezek minden változatában üresek a titok-sorok.
- **Commit e-mail:** minden commitban ugyanaz a cím (a választott nyilvános cím).
- **Javaslat:** az előzmény változatlanul maradhat; nincs mit lecserélni. A nyilvánosságra hozás napján a keresést meg kell ismételni (a repó azóta nő).

### Személyes adatok a munkafában (b osztály)

Keresett minták: a Tailscale- és LAN-címek, `szmajli`, `ujjweb.hu`, `/mnt/g/`, `G:\Projects`, `NORBI-PC`, `pc-wsl`, `nato`, `homelab-shared-postgres`, `badzso`.

| Hely | Találat | Javaslat |
|---|---|---|
| `docs/handoff/*` (68 fájl) | 155 | 0.2 dönti el (javaslat: privát repó) |
| `docs/05-homelab-telepites.md` | 24 | privát repóba, változatlanul |
| `docs/04-gitea-agent-hozzaferes.md` | 23 | általános recept marad nyilvánosan példacímekkel; a valódi címek a privát repóba |
| `docs/06-allapot-es-uzemeltetes.md` | 11 | az üzemeltetési rész a privát repóba, az általános rész angol áttekintésként marad |
| `CLAUDE.md` | 8 | a gépekre vonatkozó rész a `CLAUDE.local.md`-be |
| `docs/10-kollega-beallitasa.md` | 4 | privát repóba |
| `deploy/.env.prod.example` | 4 | példacímek; a valódi értékek a homelab `deploy/.env`-jében már megvannak |
| `deploy/compose.yaml` | 1 (komment) | a homelab-specifikus rész `compose.override.yaml`-ba vagy általánosítva |
| `guard.example.yaml` | 2 | példaútvonal (`~/projects`) |
| `docs/01-terv.md`, `00-kutatas.md`, `02-…`, `09-…`, `15-…`, `16-…`, `README.md` (docs) | 1–3 | példára cserélni |
| `ui/ui-mockup/*.html` | 5 | példa gépnév |
| Tesztek (Go és web, ~20 fájl) | 1–9 | dokumentációs címek (`192.0.2.x`, `100.64.0.x`, `example.test`) |
| `internal/guard/fsbrowse.go:86` (komment) | 1 | példaútvonal |
| `docs/01-terv.pdf` (régi export, 2026-10-15) | 6 | **törlés**: a forrás a `.md`, a PDF elavult |

- **Nem személyes adat (c):** a `badzso-boop/colonyhall` repónév (`README.md`, `scripts/install.sh:14`, `internal/setup/server.go:20`, `web/src/features/hosts/model.ts:86`): ez a nyilvános repó címe. Ha a repó szervezethez költözik (0.4), ezek egy helyről változnak.
- **A képernyőképek** (`docs/handoff/31–35-screens`, 242 kép) a fejlesztői seed kitalált adatait mutatják (`@fake-agent`, `@peter`, `@anna`, `example.com`); valódi munkaterület nem látszik rajtuk. A handoff mappával együtt mozognak.
- **A fejlesztői seed tokenjei** (`internal/server/server.go:689–690`): csak `COLONY_DEV_SEED=1` mellett jönnek létre (`Makefile:23`, `deploy/compose.dev.yaml:35`); az éles `deploy/compose.yaml` és a telepítő nem állítja be. **Kockázat:** ha valaki egy elérhető szerveren bekapcsolja, a nyilvánosan ismert tokenekkel admin lesz. **Javaslat:** a szerver ne induljon el seeddel, ha nem `127.0.0.1`-en figyel, kivéve egy külön kapcsolóval (`COLONY_DEV_SEED_ALLOW_REMOTE=1`, a telefonos teszthez). Kicsi változás, teszttel; vagy a 40. körbe kerül.

## 0.2 Mi marad privát

**Javaslat: privát kísérő repó (`colonyhall-ops`) + `CLAUDE.local.md`**, a prompt kötelező szabálya szerint (először áthelyezés, utána csere).

| Tartalom | Hová | Miért |
|---|---|---|
| `docs/handoff/*` (promptok, jelentések, képek) | `colonyhall-ops` | 155 személyes találat, belső munkajegyzet; a „így épült agentekkel” történetet később a weboldal válogatva elmesélheti |
| `task.md` | `colonyhall-ops` | belső feladatlista |
| `docs/05`, `docs/10`, a `docs/06` üzemeltetési része, a `docs/04` valódi címei | `colonyhall-ops` | a homelab és a kolléga gépei |
| `docs/19`, `docs/21` (helyi agent terve, források) | nyilvános marad | általános tervezési anyag, nincs benne személyes adat |
| `CLAUDE.md` gépspecifikus része | `CLAUDE.local.md` (gitignore) | a Claude Code a `CLAUDE.md` mellett automatikusan betölti |
| `CLAUDE.md` általános része | nyilvános marad | a közreműködők agentjeinek is hasznos |
| Memória, `guard.yaml`, `~/.config/colonyhall/` | érintetlen | már most a repón kívül vannak |

- **A `CLAUDE.local.md` és a worktree-k:** a gitignore-olt fájl nem kerül át az új worktree-be. Javaslat: a fájl a `colonyhall-ops` repóban él, és a fő mappában meg minden worktree-ben egy symlink mutat rá; a handoff promptok első kódblokkja (`git worktree add …`) kiegészül egy `ln -s` sorral. A `.gitignore`-ba bekerül a `CLAUDE.local.md` (a mostani `*.local` minta nem fedi).
- **A handoff-hivatkozások:** a `CLAUDE.md`, a `docs/` és a kódkommentek néhol `docs/handoff/NN-…`-re hivatkoznak. Ezek a privát repóra mutató szöveges hivatkozásra változnak, vagy kikerülnek.
- **A munkamódszer:** a körök promptjai és jelentései ezentúl a `colonyhall-ops` repóba kerülnek; a kód ága ugyanúgy a nyilvános repóban.
- **Alternatíva** (ha a történetet most akarod megmutatni): a handoff mappa nyilvános marad, csak a címek cserélődnek benne. Hátránya: 68 fájl átírása, és a jövőbeli promptoknál is figyelni kell.

## 0.3 Licenc: az AGPL-3.0-only + CLA működik

### Függőségek

- **Go** (`go-licenses report ./...`, 25 modul): MIT, BSD-3-Clause, Apache-2.0, ISC; a `segmentio/asm` licence MIT-0 (az eszköz nem ismerte fel, kézzel ellenőrizve). Nincs GPL-család, nincs ismeretlen.
- **npm, a kiadott PWA** (`license-checker --production`, 99 csomag): MIT 93, ISC, Apache-2.0, 0BSD, és két betűtípus **OFL-1.1** (`@fontsource-variable/inter`, `jetbrains-mono`). Az OFL beépítve terjeszthető AGPL-es és kereskedelmi szoftverrel is; a licencszöveget mellékelni kell (`NOTICE`).
- **npm, csak a buildhez** (393 csomag): ezek nem kerülnek a kiadásba; köztük MPL-2.0 (`lightningcss`), CC-BY-4.0 (`caniuse-lite`, adat), Python-2.0 (`argparse`), BlueOak-1.0.0. A terjesztett binárist nem érintik.
- **Beépített Monaco 0.52.2** (`internal/httpserver/web/vendor/monaco/`): MIT, a `LICENSE` megvan. A minifikált fájl más csomagokat is tartalmaz (pl. `marked`); a Monaco npm-csomagja ehhez `ThirdPartyNotices.txt`-t ad, ami a beépített példányból hiányzik. **Javaslat:** bemásolni, és a `NOTICE` hivatkozzon rá.
- **Következtetés:** minden futásidejű függőség engedékeny; kettős licenc mellett is továbbadható.

### Átvett kód és szerzők

- A Mattermost, Dendrite, Synapse és Stoat kódjából nincs bemásolt rész (a `CLAUDE.md` szabálya; a kommentek csak mintára hivatkoznak). A 48. kör további része a jellegzetes azonosítókra is rákeres.
- **Szerzők:** az előzmény minden commitja Norbié (két szerzőnév, ugyanaz a cím); az agentek munkája az ő nevén került be. Kereskedelmi licenchez jelenleg senki más engedélye nem kell. Ha a kolléga commitol, előbb alá kell írnia a CLA-t.

### Javaslatok (a kapun jóváhagyandó)

- **A „Forráskód” link (AGPL 13. pont):** új környezeti változó `COLONY_SOURCE_URL` (alapból a nyilvános repó címe); a szerver a `GET /api/v1/version` új végponton adja vissza a verzióval együtt; a PWA a Beállítások / Névjegy részen és a bejelentkező oldal láblécében mutatja. Aki módosított változatot üzemeltet, a saját forrására állítja.
- **A CLA aláírása:** a `contributor-assistant/github-action` (külső szolgáltatás és titok nélkül, az aláírások a repó egy külön ágában); a szöveg az Apache Individual CLA alapján, kiegészítve az Element-féle vállalással. Céges változat egyelőre nem kell. A hozzájárulónak egyszeri megjegyzés a PR-ben („I have read the CLA…”).
- **Fejlécek:** nincs fájlonkénti licencfejléc; egy `LICENSE` (az AGPL-3.0 szövege), egy `NOTICE` (szerzői jog, harmadik felek), és SPDX-azonosító a `package.json`-okban.

## 0.4 A név

- **Keresve (2026-11-11):** GitHub (felhasználó, szervezet, repó), npm, Docker Hub, DNS (`.com`, `.io`, `.dev`, `.app`, `.hu`, `.org`), webes keresés „Colonyhall” és „Colony Hall” névre.
- **Eredmény:** a GitHubon csak a saját repód; a `colonyhall` GitHub-felhasználónév és szervezetnév szabad; npm-csomag és Docker Hub-névtér nincs; a hat domain egyikének sincs DNS-bejegyzése (valószínűleg szabad, a regisztrátornál ellenőrizendő). Védjegyet a keresés nem talált; a legközelebbi a „COLONY” szoftveres védjegy (42. osztály, Collectively Intelligent Ltd).
- **Nem ellenőrizve:** hivatalos védjegy-adatbázis (EUIPO TMview, USPTO). Ezt a kereskedelmi indulás előtt érdemes megnézni.
- **Döntések:**
  - domain regisztrálása a bejelentés előtt (javaslat: `colonyhall.dev` vagy `.com`);
  - a repó maradjon `badzso-boop/colonyhall`, vagy kerüljön egy `colonyhall` GitHub-szervezetbe (a régi cím átirányít; a kódban négy helyen kell átírni).

## 0.5 Közösségi fájlok

Hiányzik mind: `CONTRIBUTING.md` (build, teszt, PR, a rétegszabályok röviden, a CLA), `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1, kapcsolat: a nyilvános cím), `SECURITY.md` (GitHub privát sebezhetőség-bejelentés, támogatott verzió: a legutóbbi kiadás), issue- és PR-sablonok, `CODEOWNERS`. **Kérdés:** kell-e `FUNDING.yml` (GitHub Sponsors)?

## 0.6 Dokumentáció nyelve

Javasolt angol anyag a nyilvános látogatónak: `README.md` (már angol), telepítési útmutató (a `docs/15-telepites-terv.en.md` és a `docs/16` alapján), architektúra-áttekintés (a `CLAUDE.md` és a `docs/02` rövidítve), `CONTRIBUTING.md`, API-referencia az `api/openapi.yaml`-ból (statikus HTML, később a weboldalon). A magyar tervdokumentumok maradnak.

## 0.7 GitHub-beállítások (Norbi kapcsolja be)

Ágvédelem a `main`-en (PR kötelező, a `ci-ok` kötelező ellenőrzés, force push tiltva); Dependabot (Go, npm, GitHub Actions); CodeQL (Go, JavaScript/TypeScript); titokkeresés és push-védelem; privát sebezhetőség-bejelentés; Discussions (javaslat: igen, a kérdéseknek); témák (`chat`, `ai-agents`, `self-hosted`, `mcp`, `claude-code`, `go`, `react`); leírás; közösségi előnézeti kép (a 5. rész demóadataiból). **A fork-PR-ek CI-ja:** a mostani jobok titok nélkül futnak; a fizetős perceket a fork-PR-ek is fogyasztják, ezért javaslat: „Require approval for all outside collaborators” a workflow-futtatásoknál.

## 0.8 Telemetria és adatvédelmi nyilatkozat

- **A szerver** csak oda kapcsolódik, amit az üzemeltető beállít: a Gitea (`COLONY_GITEA_URL`), a feliratkozott naptárak (ICS), a beállított AI-szolgáltatók és modellszerverek. Nincs beépített telemetria, frissítéskeresés vagy hibajelentés (OpenTelemetry és Prometheus a `CLAUDE.md`-ben tervként szerepel, a kódban nincs).
- **Az őr** a saját szerverével beszél, a helyi CLI-ket (`claude`, `agy`) futtatja (ezek a saját szolgáltatójukhoz kapcsolódnak), és a `web_read` eszközzel csak az agent által kért nyilvános oldalakat tölti le.
- **A telepítő és a CLI:** a `colony upgrade` és az `install.sh` a GitHub kiadásait kéri le, a telepítő a `claude` CLI hivatalos telepítőjét (`https://claude.ai/install.sh`), ha azt választod. Ezek csak kifejezett parancsra futnak.
- **A PWA:** nem tölt be külső scriptet, betűtípust vagy képet (a betűtípusok be vannak építve; a markdown-megjelenítő tiltja a távoli képeket).
- A README-be egy rövid „Privacy” bekezdés kerül ezzel a tartalommal.

## 0.9 A nyilvánossá tétel napja (Norbinak)

1. A 40. kör (biztonság) beolvasztva és élesítve; a 48. kör beolvasztva.
2. A `colonyhall-ops` repó létrehozva, feltöltve; a `CLAUDE.local.md` symlink a fő mappában és a worktree-kben működik.
3. Titokkeresés újra a teljes előzményen (gitleaks, trufflehog); a személyesadat-ellenőrzés zöld a CI-ban.
4. A még élő tokenek és titkok rendezése: a Gitea olvasási tokenje és webhook-titka, az owner API-kulcs, a géptokenek maradhatnak (soha nem kerültek a repóba), de ha bármelyik bekerült egy nyilvánossá váló handoff-fájlba vagy képbe, cserélni kell (a mostani keresés nem talált ilyet).
5. Ha szervezetbe költözik: a repó áthelyezése, a négy kódbeli hivatkozás frissítése, a GHCR-image útja.
6. GitHub-beállítások a 0.7 szerint; a CLA-workflow bekapcsolása.
7. A láthatóság átállítása nyilvánosra.
8. Az első nyilvános kiadás a 41. kör ellenőrzőlistája szerint (`docs/16-telepites.md`, „Kiadás”); a GHCR-csomag nyilvánosra állítása.
9. A weboldal (5. rész) élesítése, ha kész.
10. Bejelentés.

## Döntések a kapun (Norbi)

1. **Mi marad privát:** a javasolt `colonyhall-ops` + `CLAUDE.local.md`, vagy a handoff mappa nyilvános marad átírva?
2. **A régi PDF-ek** (`docs/01–03-*.pdf`) törlése: igen?
3. **A fejlesztői seed szigorítása:** most ebben a körben, vagy a 40. körben?
4. **A „Forráskód” link** a javasolt formában (`COLONY_SOURCE_URL`, `GET /api/v1/version`, Névjegy és lábléc)?
5. **CLA:** a `contributor-assistant` GitHub Action, céges változat nélkül?
6. **Domain** (melyik, mikor) és **GitHub-szervezet** (marad a `badzso-boop` alatt vagy `colonyhall` szervezet)?
7. **`FUNDING.yml`** és **Discussions**: kell?

## Döntések a kapun (Norbi, 2026-11-11)

- A régi PDF-ek (`docs/01–03-*.pdf`) törölhetők.
- A fejlesztői seed szigorítása később (nem ebben a körben).
- A „Forráskód” link a javasolt formában (`COLONY_SOURCE_URL`, `GET /api/v1/version`, Névjegy és lábléc).
- CLA: a `contributor-assistant` GitHub Action, céges változat nélkül.
- A repó marad `badzso-boop/colonyhall`; domain később.
- `FUNDING.yml`: egyelőre nem.
- A privát kísérő repó (`colonyhall-ops`): elvben igen; a kezelése az agentekkel nyitott (lásd a beszélgetést).
