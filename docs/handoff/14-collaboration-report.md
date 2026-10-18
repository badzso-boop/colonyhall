# Handoff Report: Agent–agent együttműködés (Agent Collaboration)

Dátum: 2026-10-18  
Ág: `feature/collaboration`  
Szerző: Antigravity CLI  
Dokumentáció: `docs/01-terv.md`, `docs/02-muszaki-specifikacio.md`, `CLAUDE.md`  

---

## 1. Összefoglaló

A 14. feladat („Agent–agent együttműködés”) célja a Colonyhallban futó több, akár különböző típusú AI ágens hatékony és biztonságos együttműködésének megteremtése volt. Az implementáció 6 szorosan összefüggő, különálló feladatcsoportot valósított meg részenkénti commitokkal, szerződés-első megközelítéssel, teljes tesztlefedettséggel és a dev webfelület bővítésével.

---

## 2. Megvalósítás commitonként

### 1. Közös agent szabályok (`bed74d3`)
- **Új modul:** `internal/modules/agentrules/` (domain modell, események, adattár, alkalmazási logika, HTTP API és dev UI integráció).
- **Adatbázis migráció (`migrations/00018_agent_rules.sql`):**
  - `agent_rules` append-only tábla (`workspace_id`, `version`, `text`, `created_by`, `created_at`).
  - Verziószámozás munkaterületenként növekvő egész számokkal; mentett sor hiányában a beépített v0 (angol nyelvű, beágyazott konstans) érvényesül.
- **Dinamikus helyőrzők (rendering):**
  - `{handle}`: a csatlakozó ágens saját handle-je.
  - `{admins}`: a munkaterület emberi adminjai és tulajdonosa vesszővel elválasztva (`@norbi, @alice`).
  - `{workspace}`: a munkaterület neve.
  - Ismeretlen `{...}` kifejezések változatlanok maradnak; a szöveg kizárólag emberek által szerkesztett adat, soha nem keveredik ágensektől jövő utasításokkal.
- **REST végpontok:**
  - `GET /api/v1/agent-rules`: emberi lekérdezés (aktuális szöveg, verzió, előzményverziók listája).
  - `GET /api/v1/agent-rules/{version}`: korábbi verzió szövegének megtekintése.
  - `PUT /api/v1/agent-rules`: csak adminok/tulajdonosok által hívható szerkesztés; új verziót, eseményt és auditnapló-bejegyzést hoz létre.
  - `GET /api/v1/agent/instructions`: ágens tokenes végpont, a hívóra egyedileg kitöltött szöveget és az érvényes verziószámot adja vissza.
- **MCP és Dev felület:**
  - Az MCP inicializálásakor a szerver az `Instructions` mezőben a hívó ágensre renderelt szabályokat adja át.
  - A dev felületen (`index.html`) megjelent a „Szabályok” panel: szövegszerkesztő, „Mentés” gomb (csak adminoknak), és az előzményverziókra kattintva azok azonnali visszanézhetősége.

### 2. Szabályok átadása a CLI-knek (`d4b3acc`)
- **Őr (`internal/guard`):**
  - Szabályok gyorsítótárazása ágensenként (`GET /api/v1/agent/instructions`).
  - Időzített (10 perces) és eseményvezérelt (host konfiguráció verzióváltásakor történő) újratöltés. Hálózati hiba esetén az utolsó ismert jó szöveggel fut; ha még nincs ilyen, figyelmeztetést naplóz, de soha nem blokkolja a futást.
- **Claude Code közvetlen futások:**
  - Minden futáskor átadásra kerül a `--append-system-prompt <szöveg>` kapcsolóval (nem módosítja a felhasználó helyi `CLAUDE.md` fájlját).
- **Antigravity közvetlen futások:**
  - A prompt elejére kerül beszúrásra `<colonyhall_rules version="N">...</colonyhall_rules>` keretben, **kizárólag** új beszélgetés kezdetén, munkamenet-rotáció után, vagy ha a szabályverzió módosult az adott szál előző futása óta. A legutóbb alkalmazott verziószámot a szál állapotfájlja perzisztálja a session azonosító mellett.
- **DirectPrompt optimalizálás:**
  - Eltávolításra került a szabályok szöveges duplikációja a `DirectPrompt`-ból, megtartva csak a futásspecifikus információkat (válasz automatikus posztolása, csere mappa útvonala, worktree ág).

### 3. Agent jegyzetfüzet (`71b2ab0`)
- **Új modul:** `internal/modules/notes/`.
  - *Indoklás:* A jegyzetfüzet önálló modulként valósult meg, mivel saját append-only életciklussal rendelkezik, nem terheli a `conversation` modult, és tiszta jogosultsági határt képez.
- **Adatbázis migráció (`migrations/00019_agent_notes.sql`):**
  - `agent_note` append-only tábla (`workspace_id`, `agent_id`, `thread_root_id`, `text`, `created_at`). A legfrissebb rekord reprezentálja a szálhoz tartozó aktuális jegyzetet.
- **MCP eszközök:**
  - `note_get(thread?)` és `note_set(text, thread?)`: az opcionális üzenetazonosítót automatikusan a szál gyökerére oldja fel.
  - Szigorú adatizoláció: az ágensek kizárólag a saját jegyzeteiket érhetik el, egymásét soha.
- **Emberi hozzáférés és őr integráció:**
  - `GET /api/v1/agents/{id}/notes`: emberek számára elérhető audit/felügyeleti végpont.
  - Friss futások indításakor (első futás vagy rotáció után) az őr automatikusan beilleszti az ágens meglévő jegyzetét `<colonyhall_note>...</colonyhall_note>` adatkeretben; folytatott munkamenetben nem ismétli, mivel a kontextus már tartalmazza.

### 4. Döntésnapló (`2de3785`)
- **Új modul:** `internal/modules/decision/`.
- **Adatbázis migráció (`migrations/00020_decisions.sql`):**
  - `decision` append-only tábla (`id`, `number`, `workspace_id`, `author_id`, `channel_id`, `thread_root_id`, `text`, `rationale`, `supersedes`, `created_at`).
  - Munkaterületenként növekvő sorszámozás (`D-1`, `D-2`, ...).
- **Atomi tranzakciós Wiki hozzáfűzés:**
  - A `decision_add(text, rationale, supersedes?, thread?)` hívásakor ugyanabban az adatbázis-tranzakcióban jön létre a döntés rekordja, és kerül hozzáfűzésre új szakaszként a „Döntések” wikioldalhoz (`D-NN — szöveg. Miért: indoklás...`).
  - Felülbírált döntések (`supersedes`): a korábbi döntés nem törlődik (append-only), csak megjelölésre kerül az új bejegyzésben.
- **MCP és REST:**
  - `decision_list(limit? = 5, query?)`: a legfrissebb nem felülbírált döntések gyors listázása.
  - `GET /api/v1/decisions` és a dev felület wiki paneljén közvetlen döntéslista és oldalugrás.

### 5. Közös erőforrás foglalások (`2ed57a2`)
- **Új modul:** `internal/modules/lease/`.
- **Adatbázis migráció (`migrations/00021_leases.sql`):**
  - `lease` tábla (`id`, `workspace_id`, `resource`, `holder_id`, `note`, `acquired_at`, `expires_at`, `released_at`, `released_by`).
  - Normalizált kisbetűs erőforrásnevek (`[a-z0-9._:/-]`).
- **Párhuzamosság-kezelés és tranzakciós zárolás:**
  - PostgreSQL-en `pg_advisory_xact_lock(hashtext(workspace_id || ':' || resource))` védi a konkurens foglalásokat.
- **MCP eszközök és viselkedés:**
  - `lease_acquire(resource, ttl_minutes = 30, note?)`:
    - Ha más tartja: azonnal visszatér a `"taken by @x until HH:MM (note)"` szöveggel (nincs hiba, nincs blokkoló várakozás).
    - Ha a hívó tartja: meghosszabbítja a lejárati időt.
    - Egyébként létrehozza a foglalást.
  - `lease_release(resource)`: aktív foglalás elengedése (csak a birtokos ágens teheti meg).
- **Emberi felügyelet és automatikus felszabadítás:**
  - `GET /api/v1/leases`: aktív foglalások listája az embereknek.
  - `POST /api/v1/leases/{id}/release`: ember általi kényszerített feloldás.
  - Dev felületen „Foglalások” panel listával és „Feloldás” gombbal.
  - Eseményvezérelt automatikus feloldás: ágens leállítása (`StateStopped`/`StateHalted`), törlése vagy a vészleállító (`EventEmergencyActivated`) azonnal felszabadítja a lefoglalt erőforrásokat.

### 6. Sebességkorlát és párhuzamos futás (`02a2a96`)
- **20 üzenetes sebességkorlát:**
  - Ha egy szálban vagy DM-ben 10 percen belül legalább 20 ágens-üzenet érkezett anélkül, hogy közben emberi üzenet jött volna, a szerver elutasítja a következő ágens-üzenetet:
    - HTTP státusz: `429 Too Many Requests`.
    - Hiba: `"this conversation is paused: too many agent messages; a human has to reply"`.
- **Egyszeri admin figyelmeztetés:**
  - Szünetenként pontosan egyszer a szerver beszúr egy `system` típusú üzenetet a szálba/DM-be, megemlítve a munkaterület emberi adminjait (`⚠️ Conversation paused: at least 20 agent messages were posted... @norbi please check in.`).
  - Külön tranzakcióban kerül mentésre, így az ágens üzenetének elutasítása nem görgeti vissza a rendszerüzenetet.
  - A szünet ideje alatt érkező további ágens próbálkozások szintén 429-cel elbuknak, de nem spamelik a szálat újabb figyelmeztetésekkel.
- **Szünet feloldása:**
  - Bármilyen emberi üzenet az adott szálban vagy DM-ben azonnal feloldja a korlátozást.
- **Párhuzamos több-ágens említés:**
  - Amikor egy üzenet több ágenst is megemlít (`@bot-1 és @bot-2`), a delivery modul mindegyik ágensnek önálló `Delivery` inbox bejegyzést készít `FlagMentioned | FlagWake` jelzőkkel. Mindkét ágens párhuzamosan aktiválódik a saját munkamenetében.

---

## 3. Tesztelés és verifikáció

Minden komponens szigorú minőségbiztosításon ment keresztül:

1. **Egység- és szerződéstesztek:**
   - Memóriabeli és PostgreSQL backendek a `storetest` keretrendszerben (beleértve a konkurens advisory lock vizsgálatát `go test -race` kapcsolóval).
2. **Végponttól végpontig (E2E) tesztek:**
   - `internal/e2e/decisions_test.go`: döntés életciklus, wiki szinkronizáció, sorszámozás, limit-ellenőrzések.
   - `internal/e2e/leases_test.go`: foglalás, meghosszabbítás, párhuzamos ütközés, emberi feloldás, leállításkori automatikus felszabadítás.
   - `internal/e2e/ratelimit_test.go`: párhuzamos említés kézbesítése mindkét ágensnek, 20 üzenetes szál és DM sebességkorlát, egyszeri rendszerüzenet, emberi válasz általi szünetfeloldás.
   - `internal/guard/*_test.go`: közvetlen futtatás és CLI paraméterezés ellenőrzése fake runnerrel (valódi AI CLI nem futott).
3. **Statikus kódellenőrzés:**
   - `make lint`: 0 hiba, rétegződési szabályok (`depguard`) betartva.
   - `make gen-check`: OpenAPI és sqlc definíciók teljes összhangban a generált kóddal.
   - `make test`: teljes regressziós tesztkészlet sikeresen lefutott.
