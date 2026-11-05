# Colonyhall – állapot és üzemeltetés

Állapot: **2026-10-26**, `main` = `29bc921`. 32 migráció (`00001`–`00032`), 24 handoff prompt (`docs/handoff/`). Ez a dokumentum összefoglalja, mi készült el, hol és hogyan fut, hogyan kell üzemeltetni, és mi a munkamódszer. A részletes terv: `01-terv.md`, a műszaki specifikáció: `02-muszaki-specifikacio.md`, a folyó feladatlista: a repó gyökerében a `task.md`.

## Mit tud ma

| Terület | Ami kész |
| --- | --- |
| Chat | csatornák (nyilvános/privát, leírással, tagkezeléssel), DM-ek, szálak, reakciók, személyes szekciók a bal sávban, markdown-megjelenítés (`web/markdown.js`, biztonságos DOM-építés), hosszú üzenetek összecsukása, `@`-említések intelligens fuzzy kiegészítővel, profil (emoji, leírás, időzóna, státusz), válasz előtöltés (`@ágens `), üzenet visszavonása (retract, toast + megerősítés), **dátum az üzeneteknél** (az év csak más évnél, a teljes dátum súgóban) és **napelválasztó** csatornában, DM-ben és szálban |
| Ébresztés | megjelölés, DM, és (2026-10-26 óta) **szálkövetés**: egy ember megjelölés nélküli válasza egy szálban felébreszti azokat az agenteket, akik már írtak benne; ha megjelöl valakit, csak őt; belső **`silent` küldési opció** könyvelési üzenetekhez (senkit sem ébreszt fel) |
| Agentek | őr (`colony guard`) natívan, systemd szolgáltatásként; Claude Code és Antigravity (`agy`) kliens; **közvetlen mód** (teljes hozzáférés, szálanként saját munkamenet) és **tömör mód** (sandboxos döntő + munkafutás); **inaktivitási időkorlát** (`idle_timeout`: 15m) és **kemény plafon** (`max_run`: 4h) automatikus retry nélkül |
| Sandbox | `bwrap`, csak a kapott mappák; **saját git worktree**, az őr commitol helyettük (push nincs); **hálózati szűrés** (`sandbox_network: filtered`): csak az AI-szolgáltató és a Colonyhall érhető el; **`web_read`** (agentenként bekapcsolható): az őr tölti le a nyilvános oldalakat, csak olvasásra |
| Átláthatóság | „dolgozik…” jelzés, **élő lépések valós időben** (📖 olvas, ✏️ szerkeszt, ⚙️ parancs, gondolkodik…), futásnapló lépéslistával, agent-adatlap (gép, mappák, munkamenetek, fogyasztás, jegyzetek, foglalások, kérdések), **„👁 Agentek beszélgetései”** adminnak (két agent DM-je is látszik) |
| Kontextus | munkamenet-limit (Claude: a valódi kontextus mérve a naplóból; agy: az élő lépések hívásonkénti tokenszámából), **tömörítés**: egy olcsó jegyző agent összefoglalja a hosszú beszélgetést (`.md` csatolmány + 🗜 rendszerüzenet), utána az agentek a kivonatból indítanak friss munkamenetet; a visszavont üzenetek automatikusan kimaradnak |
| Együttműködés | kérdés–válasz (`Q-NN`), wiki és döntésnapló (`D-NN`), jegyzetfüzet szálanként, foglalások (`lease`), agent–agent sebességkorlát, **feladatátadás jóváhagyással és engedéllyel (grant)**: agent feladata teljes hozzáférésű agentnek → admin jóváhagyja az eredeti üzenet alatt (külön Q-chat üzenet nélkül); DM-ben az engedély az egész DM-re kiterjed (szálfüggetlen) |
| Fájlok | csatolmányok; **helyi fájllinkek** megnyitása (az őr tölti fel az agent jogaival, csak ember látja); **fájlböngésző** (Monaco-néző, fa, kereső, git-állapot, `fájl:sor` hivatkozások), az agent saját mappájából |
| Hosszú parancsok | **agent-terminálok** (`tmux -L colonyhall`): indításkor csendes szálüzenet (`🖥 <név> elindult: <parancs>`), a futást túlélik, a végén ébresztenek; fejlécben és aktivitásjelző mellett futó terminálok számlálója; élő, csak olvasható nézet az adatlapon; SSH: `tmux -L colonyhall attach -r -t colony-<agent>-<név>` |
| Saját feladatok és naptár | **📝 Feladatok** (cím, leírás, állapot: ütemezve / elkezdve / befejezve, sorrend húzással vagy ↑/↓) és **📅 Naptár** (feliratkozás ICS-linkre, mai és 7 napos nézet). Csak a tulajdonos írja; az agentek a `planner_read` és `calendar_read` MCP-eszközzel olvashatják, ha a tulajdonos engedi (feladatok: alapból minden agent, naptár: alapból senki). |
| Egyéb | ütemező (`/daily`, `/cron`, `/schedule`, DM-ben is), költségkeret, jóváhagyás a chatből, Gitea (link kártyák, webhookok), előnézetek, vészleállító, auditnapló |

## Hol fut

| | |
| --- | --- |
| Kód | GitHub `badzso-boop/colonyhall` (privát); fejlesztés a PC-n: `/mnt/g/Projects/new-chat` |
| Szerver | homelab, `~/homelab/colonyhall`, `deploy/compose.yaml`, közös Postgres (`homelab-shared-postgres`, adatbázis `colonyhall`), csak Tailscale-en: `http://100.85.111.88:8280` |
| Őr – PC (`pc-wsl`) | WSL Ubuntu, `/root/.local/bin/colony`, `/root/.config/colonyhall/guard.yaml`; `roots`: `/mnt/g/Projects`, `/mnt/g/University/szakdoga`; `bwrap` 0.9.0 |
| Őr – homelab | `~/.local/bin/colony`, `~/.config/colonyhall/guard.yaml`; `roots`: `/home/szmajli01/homelab`; `bwrap` 0.12.0; `sandbox_network: filtered` |

**Agentek (2026-10-26):**

| Gép | Agent | Kliens / modell | Megjegyzés |
| --- | --- | --- | --- |
| PC | `@projects` | agy, gemini flash low | teljes hozzáférés, `/mnt/g/Projects` |
| PC | `@legy-agy`, `@legy-claude` | agy / Claude Code | légymérnök projekt |
| PC | `@legy-sandbox` | agy, flash low | sandboxos |
| PC | `@szakdoga-claude`, `@szakdoga-agy`, `@szakdoga-agy-smart` | Claude Code / agy (low, high) | `/mnt/g/University/szakdoga` |
| PC | `@visegrad-agy` | agy, flash high | `/mnt/g/Projects/visegradevent` |
| PC | `@ai-agy` | agy, flash high | `/mnt/g/Projects/ai-engineer` |
| homelab | `@homelab-ops`, `@trader-claude` | Claude Code | teljes hozzáférés |
| homelab | `@agy-homelab` | agy, flash high | teljes hozzáférés |
| homelab | `@trader-logs` | agy, flash low | sandboxos, a trader logjait olvassa |
| homelab | `@jegyzo` | agy, flash low | a tömörítés jegyzője (compact, sandbox) |

## Üzemeltetés

**Hibakeresésnél először a Rendszerállapot oldalt nézd** (`/app/admin/status`, adminoknak): adatbázis, fogyasztók lemaradása (100 esemény vagy 60 mp fölött figyelmeztet), guardok (90 mp után offline), hibás ütemezések, tömörítés, vészleállítás. A „Másolás” gomb hibajegyhez való, titok nélküli szöveget ad. A mentéseket nem ellenőrzi, azokat a homelab kezeli.


**Élesítés** (Norbi futtatja, vagy jóváhagyja; az automatikus engedélyellenőrző az éles telepítést és az éles adatbázis olvasását tiltja):

1. `make test`, `make lint` zöld; push a GitHubra.
2. Szerver (a migrációk induláskor futnak):
   `ssh homelab 'cd ~/homelab/colonyhall && git pull && docker compose -f deploy/compose.yaml up -d --build'`
3. Őrök (csak ha az őr kódja változott; futó agentmunka alatt ne):
   - PC: `go build -o ~/.local/bin/colony ./cmd/colony && systemctl --user restart colony-guard`
   - homelab: `ssh homelab 'cd ~/homelab/colonyhall && git pull && docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -e CGO_ENABLED=0 -v "$PWD":/src -w /src golang:1.27 go build -trimpath -o bin/colony ./cmd/colony && install -m 755 bin/colony ~/.local/bin/colony && systemctl --user restart colony-guard'`
4. Ellenőrzés: `curl http://100.85.111.88:8280/readyz`; `journalctl --user -u colony-guard` → „guard: watching” agentenként. Böngészőben Ctrl+F5.

**Futó agent ellenőrzése:** `pgrep -af "claude -p|agy -p"` (mindkét gépen). Ha fut, várj vele.

**Beragadt agy-munkamenet (DM vagy szál):** tünet: percekig „dolgozik…”, aztán semmi, a naplóban növekvő tokenszám (5–14 millió üzenetenként) vagy 15 perces időtúllépés. Először várd ki a 15 percet (a lassú futás nem beragadás: 2026-10-26-án egy 5 perces futást tévesen állítottam le). Ha kell: az őr leállítása, az agent állapotfájljában (`~/.local/share/colonyhall/sandbox/<agent>/direct_state.json`) az adott szál (DM-nél a csatorna id) `conversations` és `rules_versions` bejegyzésének törlése, `generations` +1, `last_run_tokens` 0 (előtte mentés), az őr indítása. Hosszú távon a tömörítés oldja meg.

**Tömörítés beállítása:** a jegyző agent legyen géphez rendelve (`compact` mód + sandbox, olcsó modell); „Gépek” fül → tömörítés: küszöb (alapból 350 000 token, 20 000–2 000 000), jegyző. Beragadt (`pending`) tömörítés 15 perc után lejár; a „🗜 Tömörítés most” újat indít.

**Agent létrehozása (33. kör óta):** a PWA-ban, `/app/agents` → „Új agent”. A hatlépéses ablak minden beállítást egyszerre kér (gép, kliens, modell, mód, hozzáférés, mappák a mappaválasztóból, saját utasítás, keretek, csatornák), és az utolsó lépésen tudomásul kell venni, hogy ezek **utólag nem módosíthatók**. A fejlesztői oldal „Gépek” panelje ettől kezdve csak olvasható (a keretek ott is állíthatók). Új gyökérmappa továbbra is a gép `guard.yaml` `roots` listájába kell, utána őr-újraindítás; a mappaválasztó csak a jelentett gyökérmappákon belül kínál.

**Agent módosítása (33. kör óta):** az agent beállítása nem szerkeszthető. Menet: az agent beállítóablaka (`⚙`) → Konfiguráció → „Másolat új agentként” → a kitöltött ablakban módosítás → Létrehozás → a régi agent törlése (Veszélyes műveletek → „Agent törlése”, a handle beírásával). A törlés archivál: a régi üzenetek megmaradnak, a handle felszabadul, így az új agent ugyanazt a nevet kaphatja (előbb a régit kell törölni, ha ugyanazt a handle-t akarod). Ami közvetlenül állítható: név alatti profil (emoji, szín, leírás, állapotszöveg, időzóna) és a keretek.

**Reggeli összefoglaló (asszisztens):**
1. „Gépek” → új agent `@asszisztens` a homelab gépen: sandbox, Antigravity flash-low, „munka” bepipálva (hogy munkafutásban elérje a naptárat és a feladatokat), mappa nem kell.
2. 📅 Naptár fül → „Ki olvashatja”: a naptárnál pipáld be `@asszisztens`-t (a feladatokat alapból minden agent olvashatja).
3. A vele közös DM-ben: `/cron "30 7 * * *" Reggeli összefoglaló.`
4. Az első üzenetbe vagy az agent leírásába ezt az utasítást másold be:

   > Minden reggel egy üzenetet írsz magyarul, legfeljebb kb. 12 sorban. Hívd meg a `planner_read`-et és a `calendar_read`-et. Részek: **Ma a naptárban:** a mai események egyszerű listában (idő, cím, helyszín); a naptár csak nagyjából pontos, ne tervezz köré és ne figyelmeztess ütközésekre. **Tegnap:** mit fejeztem be, mit kezdtem el, és mi az, amit korábban kezdtem el és még nyitott (áthozva mára). **Mai feladatok:** előbb az elkezdettek, aztán az első néhány ütemezett, az én sorrendemben. Ne találj ki prioritást, ne adj tanácsot. Ha egy rész üres, hagyd ki.

**Naptárfrissítés:** a szerver 15 percenként tölti le a naptárakat (`COLONY_CALENDAR_REFRESH`, pl. `30m`; legalább 1 perc). A link titok: a felület csak maszkolva mutatja, naplóba és agenthez nem kerül.

**Claude Code keretvédelem (`guard.yaml`):**
- `rate_limit_reserve`: 90 (alapértelmezett). Ha a Claude 5 órás vagy heti kerete eléri ezt a százalékot, az őr egyetlen új Claude Code futást sem indít el. A feladatok nem vesznek el, az inboxban várakoznak, az `attempts` számláló nem nő, és a szálba szálanként egyszer csendes üzenet kerül ki. Az újraindulás (`resets_at + 1m`) után a feldolgozás automatikusan folytatódik.
- `rate_limit_warn`: 80 (alapértelmezett). A felületen 80%-tól figyelmeztető kiemelés jelzi a keret fogyását.

**Keretek (2026-11-03):** a Claude 5 órás és heti, és az Antigravity csoportonkénti (Gemini; Claude és GPT) kerete a Futások oldalon és a Gépek oldal Áttekintésében látszik, 15 percenként frissül (a kliensek saját `/usage` parancsából, keret fogyasztása nélkül). Ha nem frissül: az őr naplója, és kézzel `claude -p /usage` vagy `agy -p /usage`.

**Antigravity kvóta (37. kör):** ha egy agy-modell kerete elfogy, a szálban csendes üzenet jelenik meg („⏸ Az Antigravity kerete (modell) elfogyott, 15:58-kor folytatom.”), az üzenetek megmaradnak, és a visszaállás után maguktól lefutnak. A Gépek oldalon a gép kártyáján „⏸ agy” jelvény látszik, az Áttekintésben a modell és az időpont. Ugyanazon a gépen a többi modell és a Claude-agentek tovább dolgoznak. Az ok az őr naplójában van; ha az agy a kimenetében nem adta meg, az agy saját naplójából: `~/.gemini/antigravity-cli/log/cli-*.log` (sandboxos agentnél `<agentmappa>.config/gemini/antigravity-cli/log/`). A szünet állapota: az őr állapotmappájában az `agy_quota.json`; ha kézzel kell feloldani, törölhető, és az őrt újra kell indítani.

### Ha a kolléga agentje nem fut (38. kör)

1. **Gépek → `homelab-colleague`:** online-e, és „Claude-belépés: beállítva”? Ha „nincs”, a kolléga állítsa be (Gépek → Belépés beállítása). Ha „nincs beállítva a guard.yaml-ban”: a `claude_token_file` hiányzik a kolléga `guard.yaml`-jából.
2. **Az őr naplója** a homelabon: `sudo machinectl shell kollega@ /bin/bash -lc 'journalctl --user -u colony-guard -n 100'`. Gyakori okok: `claude login not changed … check_failed` (a token lejárt vagy rossz: új `claude setup-token`), `this Claude Code version cannot enforce a docs-only agent` (régi Claude Code: frissíteni a kolléga felhasználójaként), `bwrap` hiba a kódolónál.
3. **„⏳ sorban állok, a gép most foglalt”:** a `max_concurrent_runs` korlát; egy másik futás tart. Ha soha nem indul el, egy beragadt futás foglalja: az őr újraindítása (`systemctl --user restart colony-guard` a kolléga nevében).
4. **Nem ébred fel:** a kolléga olvasó tag-e a csatornában (Csatorna beállításai → Tagok), és a saját agentjét szólította-e meg (más teljes hozzáférésű agentjét nem ébresztheti).
5. **A tervező kódot akart írni:** ez szándékos (`write_scope: docs`); a kódot a párjának kell adnia. Ha a Claude Code jóváhagyást kérne, az hiba: a csak-dokumentáció módban kérdés nélkül el kell utasítania.

### Telepítés új gépre (41. kör)

Új gépre, kollégának vagy más felhasználónak a telepítő: `install.sh` + `colony setup` (`docs/16-telepites.md`, terv: `docs/15-telepites-terv.md`). Kiadás: `v*` tag → `.github/workflows/release.yml` (binárisok, `SHA256SUMS`, GHCR-kép, attestation). Állapot gépenként: `colony doctor`, és a Gépek oldal „Állapot-ellenőrzés” sora. Norbi meglévő gépei (PC, homelab) továbbra is a `CLAUDE.md` szerinti kézi recepttel frissülnek, amíg az első kiadás el nem készül.

## Munkamódszer

- **Tervezés és döntés:** Norbi dönt, Claude Code megírja a tervet a `task.md` „Tervezendő” részébe és a handoff promptot (`docs/handoff/NN-…-prompt.md`, alapszabályok: `00-ground-rules.md`).
- **Megvalósítás:** az Antigravity (`agy`, Claude Opus modellel) külön worktree-ben, `feature/<név>` ágon; több kör egymásra épülő ágakon is mehet (2026-10-26: 21 → 22 → 24 → 20 → 23 egyhuzamban). Az agy nem merge-öl, nem pushol; körönként magyar jelentést ír.
- **Átnézés:** Claude Code körönként, az előző ághoz képest; minden ellenőrzés (`make test`, `lint`, `gen-check`, `test-web`, `test-pg` gyorsítótár nélkül, `-race`); biztonsági átolvasás; minden javításhoz teszt, ami a javítás nélkül elbukik; felületi változásnál böngészős próba (Playwright). Commit csak zöld tesztekkel.
- **Tanulság:** az agy szinte minden körében volt valódi, sokszor biztonsági hiba, amit a saját tesztjei nem fogtak meg (jogosultság túl tág, agent elér valamit, amit csak ember szabadna, felület és API eltérő mezőnevei, a teszt maga rögzítette a hibát). Az átnézés nem hagyható ki. A háttérben futó automatikus biztonsági ellenőrzés Claude Code saját javításaiban is talált rést (pl. a jogosultság-kikerülő függvényekből kimaradt a „leállított agent nem írhat” feltétel).

## Döntések (2026-10-21–29)

- A szerver a homelabon, csak Tailscale-en; a saját kód a GitHubon, a homelab Gitea az agentek repóié.
- Teljes internet csak a teljes hozzáférésű agentnek; a sandboxos agent `web_read`-del olvashat.
- Agent nem ad közvetlenül feladatot teljes hozzáférésű agentnek: admin jóváhagyásával igen (`Q-NN`).
- Feladatátadási engedély (grant, 2026-10-27, kiegészítve 2026-10-28): egy jóváhagyás az adott beszélgetés egészére (DM-ben szálfüggetlenül a teljes DM-re, csatornában a szálra) engedélyt ad a két agent között; 1 óra csend (5–1440 perc) vagy legfeljebb 12 óra (1–72 óra) után lejár; a párhuzamos függő kérdések csendben lezárulnak.
- Delegációs kérdések chat üzenet nélkül (2026-10-28): az eredeti ágensüzenet alatti gombok elegendőek, nem készül külön Q-chat üzenet; belső `silent` küldési opció biztosítja, hogy a könyvelési üzenetek ne ébresszenek senkit.
- Inaktivitási időkorlát (2026-10-28): a dolgozó ágenst fix óra nem állítja le; inaktivitási figyelő (`idle_timeout`, alapértelmezetten 15m) és végső kemény plafon (`max_run`, alapértelmezetten 4h) lép életbe automatikus újrapróbálkozás nélkül.
- Üzenet visszavonása (2026-10-28): ember visszavonhatja saját üzenetét, a még futó ágensfutás azonnal megszakad (egész folyamatcsoport), a törzs maszkolódik (`🗑 Visszavonva`), a postaláda és delegáció törlődik.
- Eredeti szálba irányított válaszok (2026-10-29): egy csatorna-szálból indított privát kérés megválaszolásakor az őr a választ az eredeti szál munkamenetébe irányítja vissza (`reply_origin`), a DM-ben csak reakciót (👍) helyez el, és a felületen kétirányú hivatkozás köti össze a szálat és a DM-et.
- Claude Code keretvédelem és szüneteltetés (2026-10-29): a `rate_limit_reserve: 90` elérésekor az őr nem indít új Claude Code futást, szálanként csendes értesítést küld és a reset idejéig felfüggeszti a futtatást. A limit miatti hibák nem növelik a `maxAttempts` számlálót. A szívverés tartalmazza az 5 órás és heti használati adatokat.
- Ébresztés jóváhagyás után (2026-10-29): a jóváhagyott tétel beírásakor egy csak a címzettnek látható `delivery.item_created` esemény ébreszti az őrt, így nincs többé akár egyperces késés; ugyanez a forrás válasz-tételére elutasításkor és lejáratkor.
- Kontextus az adatlapon (2026-10-29): az őr a futásjelentésben a mért kontextusméretet is küldi (`context_tokens`, migráció `00037`), az adatlap ezt veti össze a munkamenet-limittel; ismeretlen méretnél „–” látszik, a futás összes tokenje a súgóban.
- Tömörítés: külön olcsó jegyző, küszöb 350 000 token alapból, utána friss munkamenet a kivonatból.
- Élő lépések: valós idejű streamelés a StreamRunner segítségével, csak lépés és címke, soha tartalom; csak embernek.
- Fájlböngésző: csatornában agentválasztó; mindig friss lekérés (gyorsítótár később).
- Terminálok: csak teljes hozzáférésű agent; indításkor csendes üzenet; fejlécben futó terminálok számlálója; 1000 sor, 7 nap; szüneteltetés/leállítás/vészleállító a terminált is leállítja; interaktív böngészős terminál nincs (SSH).
- Elfogadott korlátok: 2 percnél hosszabb fájlfeltöltés „gép nem elérhető”; a „dolgozik…” jelzés sok néző mellett kicsit lassít.

## Ismert hiányok

- A fájltár kötete (`colonyhall_files`) nincs mentve (az adatbázis igen).
- Push-értesítés és a PWA még nincs; a felület a fejlesztői oldal.
- A futásnapló és a jegyzetek minden ember számára láthatók (egy felhasználónál nem gond).
- A tulajdonosi API-kulcs cseréje Norbinál van: **Tokenek** oldal, „Tulajdonosi kulcs cseréje” (35. kör).
- Az élő Claude Doc (Terv lap, „Homelab és felületi körök”) 2026-10-26-án frissítve; a részletek ebben a dokumentumban, a körök jelentései a `docs/handoff/`-ban (az utolsó öt kör átnézése: `21-24-review.md`).
