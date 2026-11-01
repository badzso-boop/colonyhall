# 33. kör jelentése: agentek és gépek

Claude Code, 2026-11-01.

- **Ág:** `feature/pwa-agents-hosts`, külön worktree-ben (`/mnt/g/Projects/new-chat-worktrees/pwa-agents-hosts`), a `main`-ből (`a0e2e10`).
- **Prompt:** `docs/handoff/33-pwa-agents-hosts-prompt.md`.
- **Munkamegosztás:** a szerveroldalt, a mappaválasztót, a könyvtárat, a létrehozó és a beállító ablakot én írtam. Három jól elkülönülő részt háttér-agentek írtak külön worktree-ben, a briefem alapján; ezeket átnéztem és beolvasztottam:
  - a gépszintű mappalistázás (6. rész, `feature/host-fs-relay`);
  - az agent-áttekintő panel (3. rész, `feature/pwa-agent-overview`);
  - a Gépek oldal és varázsló (5. rész, `feature/pwa-hosts-pages`).

## Élesítés (fontos)

1. **Migráció van:** `00038_colors.sql` (gép- és agentszín; a meglévő gépek létrehozási sorrendben kapnak színt) és `00039_agent_archive.sql` (archiválás, csak aktív agentekre egyedi handle, `agent_instructions`). Indításkor magától lefut.
2. **Sorrend: előbb a szerver, utána a két őr.** Az őr új `/ws/host` kapcsolatot nyit a mappaválasztóhoz. A régi őrrel minden működik, csak a mappaválasztó mondja, hogy „A gép most nem elérhető”. Futó agentmunka alatt ne indítsd újra az őröket.
3. **Viselkedésváltozás:** a fejlesztői oldal agent-beállító űrlapjai mostantól csak olvashatók, a keretek kivételével. Agentet létrehozni és törölni a PWA-ban lehet (`/app/agents`). A 14 meglévő agent marad, ahogy van, csak a beállításuk nem szerkeszthető.

## Commitok

| Commit | Rész | Mit |
| --- | --- | --- |
| `67c5fdb` | 1. | Gép- és agentszínek: paletta, migráció, `PATCH /hosts/{id}`, szín a profilban, `effective_color`; a web tokenjei és kontraszttesztje |
| `03a8341`, `37403d5` | 6. | Gépszintű mappalistázás: `POST /hosts/{id}/fs` (`list_dirs`), `/ws/host` gép-tokenes kapcsolat, az őr listázója (háttér-agent) |
| `d9f15ec` | 2. | Megváltoztathatatlan agentek: `POST /agents`, lezárt végpontok (409), keretek, archiválás, újra kiadható handle, saját utasítás, őr: `killed_by: "archived"`, élő munkamenet-keret, fejlesztői oldal csak olvasható |
| `8c1c203` | 3. | Inbox (`count`, `oldest_at`) az áttekintőben |
| `e807b92` | 7. | Mappaválasztó |
| `fc2e5de`, `1205a03` | 3. | Agent-áttekintő panel (háttér-agent) |
| `698a0c3`, `9bc79f4` | 5. | Gépek oldal, fülek, „Új gép” varázsló (háttér-agent) |
| `d9991fa` | 4. | Agentkönyvtár, létrehozó ablak (6 lépés), beállító ablak (5 fül) |
| `38d9c4f`, `cd4ce9b` | javítás | Kártyacsík; hamis „Közben módosult” sáv és üres lekérés a beállító ablakban |
| `5f0bd23`, `4b77394`, utolsó | docs | Terv, specifikáció, üzemeltetés, design system, jelentés, képek |

## Mi készült

### Szerver

- **Színek:** tíz megnevezett szín, soha nem hex (`internal/platform/palette`). A gépszín kötelező; agentnek lehet saját színe, különben a gépét mutatja. Az `effective_color`-t a szerver számolja (`identity.Service.HostColors` → `hosts.AgentHostColors`).
- **`POST /agents`** (`internal/modules/agents`, új modul): minden ellenőrzés írás előtt fut, utána egy tranzakcióban jön létre az aktor, a hozzárendelés, az engedélyek, a tagságok és a saját utasítás. A memóriás tár nem tud visszagörgetni, ezért az „előbb mindent ellenőrizni” nem csak szépség: enélkül a tesztkörnyezetben félkész agent maradhatna. A gépes agentnek nincs saját API-tokenje (az őr kér neki rövid lejáratút); API-kulcsos agentnek van (egyszer látható).
  - Ellenőrzés: a gép jelentette a klienst; a gyökérmappa, a worktree-repó és minden engedély a gép jelentett gyökérmappáin belül van; az utasítás ≤ 8000 karakter, nem titok, és a közös szabályokkal együtt ≤ 16 000.
- **Lezárt végpontok:** `PUT/DELETE /agents/{id}/host`, `POST /agents/{id}/grants`, `DELETE /grants/{id}` és `POST /actors` `kind: agent` → 409, `type: agent_config_immutable`, a megoldást megnevező részlettel. Ehhez a `httpx` kapott egy `RegisterProblemType`-ot.
- **Keretek:** `PATCH /agents/{id}/limits`. Az őr a munkamenet-keretet újraindítás nélkül veszi át, mert különben egy keretváltás megszakítaná a futó munkát. A keretek ezért kikerültek az őr konfigurációs kulcsából.
- **Archiválás:** `DELETE /agents/{id}`. Megszűnnek a tokenek és munkamenetek, a hozzárendelés és az engedélyek (a gép-tokenek is), a csatornatagságok és a delegációs engedélyek; az inbox ébresztés nélkül nyugtázódik. A handle felszabadul (részleges egyedi index), a megemlítés az aktív agentre oldódik fel.
- **Saját utasítás:** a közös szabályok után, „## Instructions for @handle” fejléccel kapja meg az agent, minden kézbesítési úton (`/agent/instructions`, MCP, `--append-system-prompt`, Antigravity).
- **Őr:**
  - az archivált agent termináljait `killed_by: "archived"`-del állítja le;
  - újraindításkor a terminálok futnak tovább (eddig „halt”-tal leálltak);
  - a `/ws/host`-on fogadja a mappalistázást.
- **Mappalistázás:** a háttér-agent jegyzete a választott átvitelről: `docs/handoff/33-host-fs-relay-notes.md`. Röviden: külön `/ws/host` végpont, nem a felhasználói eseményfolyam, mert gép-token soha nem kaphat chat-tartalmat. Offline gépre azonnal 409 `host_offline`, nem 5xx, mert a `httpx` az 5xx részletét elnyeli.

### PWA

- **Agentek (`/app/agents`):**
  - fuzzy keresés kiemeléssel (`/` fókuszál);
  - URL-ben tárolt szűrők: gép, kliens, hozzáférés, állapot, archivált;
  - csoportosítás gép vagy állapot szerint, kártya és táblázat nézet;
  - a kártya a géphez illő színcsíkot, színes avatargyűrűt, állapotot, klienst és modellt, hozzáférést mutat.
- **Létrehozó ablak:**
  - hat lépés;
  - a figyelmeztető sáv minden lépésen ott van;
  - 🔒 minden nem módosítható mezőnél, „utólag is módosítható” a többinél;
  - mappák a mappaválasztóból;
  - utasítás-szerkesztő helyőrzőkkel, számlálóval, titok-figyelmeztetéssel és „Hatályos utasítás” előnézettel;
  - kötelező tudomásulvétel;
  - szerverhibánál a hibás mező lépésére ugrik;
  - bezárás előtt rákérdez.
  - Az ablak bárhonnan címezhető: `?new=1&host=…`, illetve `?new=1&copy=<handle>` („Másolat új agentként”).
- **Beállító ablak:** `?agent=<handle>&settings=<fül>`.
  - Fülek: Profil (mentés „Mi változik” listával), Keretek, Konfiguráció (csak olvasható, „Másolat új agentként”), Utasítás, Veszélyes műveletek (szüneteltetés, leállítás, törlés a handle beírásával).
  - Az archivált agentnél minden csak olvasható, kivéve a másolatot.
- **Mappaválasztó:** egyetlen, több és böngésző mód; csak git-repó szűrés; olvasás/írás kapcsoló; tálca összegzéssel és a beágyazott kijelölés figyelmeztetésével; offline, időtúllépés és titkos mappa állapot; admin tartalék („Útvonal beírása”); billentyűzet.
- **Áttekintő panel (háttér-agent):** fejléc (állapot, gép, kliens, hozzáférés, műveletek, admin menü) és kilenc összecsukható szakasz, a nyitottságukat nézőnként megjegyzi.
- **Gépek (`/app/hosts`, háttér-agent):**
  - lista állapottal és figyelmeztetésekkel;
  - fülek: Áttekintés, Agentek, Mappák, Worktree-k és előnézetek, Beállítások (szín élő előnézettel, visszavonás);
  - „Új gép” varázsló kóddal, visszaszámlálással és másolható paranccsal; a beíratást élőben várja.

### Fejlesztői oldal

- A „Gépek” panel új-agent űrlapja helyén utalás áll a PWA-ra.
- Lekerültek a hozzárendelés-, levétel- és engedélygombok; ugyanígy az adatlapon a mappaengedély hozzáadása és törlése.
- A keretek szerkesztése megmaradt, és már a `PATCH /agents/{id}/limits`-et hívja.

## Tesztek

- **Áthelyezett tesztek:** 20 e2e-fájl változott.
  - A `POST /agents`-en mennek az agentet létrehozó segédek: `createAgent` (17 hívás), `createFullAccessAgent` (34), `createSandboxedAgent` (12), az agent-típusú `createActor` (23), és közvetlenül 24 helyen.
  - A seedelt `@fake-agent` fix tokenjét használó tesztek egy `e.configure` tesztsegéddel kapnak gépet (16 hívás). Ez a hosts szolgáltatást folyamaton belül hívja, mert az API-n létrehozás után már nem lehet konfigurálni.
  - A `hosts_test` négy tesztje és a preview-jelző teszt az új szabályok szerint lett átírva: a validáció `POST /agents`-szel megy, a „levétel” helyett archiválás van.
- **Új tesztek:**
  - lezárt végpontok 409-e;
  - atomikus létrehozás (rossz útvonalnál semmi nem íródik, az őr sem látja);
  - titok és túl hosszú utasítás;
  - keretek;
  - archiválás (tokenek, tagságok, inbox, őr-konfiguráció);
  - handle újrafelhasználása és tiltása, amíg a régi aktív;
  - megemlítés az új agentnek;
  - jogosultságok (tag és agent: 403);
  - színek;
  - inbox az áttekintőben;
  - storetest: archiválás, újra kiadható handle, saját utasítás;
  - őr: terminálok `archived`-del, élő keret;
  - mappalistázás.
- **Eredmények:**
  - `make test` zöld;
  - `make lint`: 0 hiba;
  - `make test-web`: 58 fájl, 391 teszt;
  - `make gen-check` zöld;
  - `make test-pg` (`GOFLAGS=-count=1`) zöld;
  - `go test -race ./internal/guard/ ./internal/e2e/` zöld.
- **Vitest:** a workerek száma 25%-ra csökkent. A terhelt WSL-en (párhuzamos háttér-agentekkel) 50%-nál a workerek nem indultak el időben.

## Eltérések és korlátok

- **Megjelenített név:** a profil-API nem módosítja, ezért a beállító ablakban csak olvasható.
- **`sandbox_network`, `preview_ports`:** az őr nem jelenti őket; a létrehozó ablak csak a `sandbox`-ot és az `allow_unsandboxed`-et mutatja.
- **„Vészleállítás csak ennek”:** nincs agentenkénti vészleállító végpont, ezért „leállítva” állapotba teszi az agentet.
- **Elavult őr figyelmeztetés:** a szerver nem adja meg a saját verzióját, így a figyelmeztetés ki van kapcsolva.
- **Gépjelentések:** esemény nélkül érkeznek, ezért a Gépek oldal 30 másodpercenként frissít.
- **Mappák fül:** a mappaválasztó modális ablak, ezért a fül összegzést mutat, és gombbal nyitja a böngésző módot.
- **Áttekintő:**
  - az `OverviewAssignment`-ben nincs előnézet-jelző;
  - az `OverviewThread`-ben nincs utolsó futási idő;
  - az utolsó futások a `/runs` végpontról jönnek, mert az áttekintő futásaiban nincs hely és időtartam.
- **Mappaválasztó:** csak lista nézete van (mappáról mappára), a fa nézet nem készült el.
- **Képek:** a mappaválasztó mappáit a Playwright adja (nincs élő őr); az offline állapot a valódi szerveré.

## Képek (`docs/handoff/33-screens/`)

| Kép | Mit mutat |
| --- | --- |
| `directory-cards-dark.png`, `directory-table-light.png`, `directory-search-light.png`, `directory-phone-dark.png` | Agentkönyvtár: kártyák, táblázat, keresés kiemeléssel, telefon |
| `overview-dark.png` | Áttekintő panel a könyvtár mellett |
| `create-step1…6-dark.png`, `create-step3-phone-light.png`, `create-step6-phone-dark.png` | A létrehozó ablak mind a hat lépése a figyelmeztető sávval; az összegzés a bepipált tudomásulvétellel |
| `settings-*-light.png`, `delete-confirm-dark.png` | A beállító ablak öt füle; törlés-megerősítés a handle beírásával |
| `picker-multi-dark.png`, `picker-single-light.png`, `picker-offline-light.png` | Mappaválasztó: több mód hozzáféréssel és titkos mappával, egyetlen mód, offline |
| `hosts-dark.png`, `hosts-phone-light.png` | Gépek oldal |
