# Lokális kódoló agent: a determinisztikus keretrendszer részletes terve

> **Állapot: vázlat, Norbi és Claude Code közös kidolgozás alatt (2026-11-08, döntések: 2026-11-09, 2026-11-10).** Ebből készül a végleges 47-es handoff prompt. Ami itt „Javaslat”, az még nem döntés; a „Döntés” jelzésűek Norbi döntései; a „Nyitott” jelzésűek még döntésre várnak. A háttér és a források: `docs/21-lokalis-agent-forrasok.md`.

## 0. Irányváltás (döntés, Norbi, 2026-11-10): nem írunk saját agentet és saját modellfuttatót

A terv eredetileg azt írta le, hogy a guard maga vezeti az agent ciklusát, és maga indítja a modellfolyamatokat. Ez túl nagy vállalás, és mások már megírták (LM Studio, llama-swap, little-coder, OpenCode, Qwen Code, Goose, Hermes Agent: `docs/21`). Ezért három rétegre bontjuk:

1. **Modellfuttató: `llama.cpp` szerver a llama-swap mögött**, natívan a WSL-ben, systemd alatt, csak `127.0.0.1`-en. A llama-swap tölti be és rakja ki a modelleket igény szerint; a guard nem indít modellfolyamatot, csak a szabad VRAM-ot figyeli, és ha kevés (pl. játék), a helyi munka vár (49. kör). Az Ollama nem kell (maga is `llama.cpp`, de saját ütemezéssel, ami a miénkkel verekedne, és rerankelni nem tud). A gondolkodó modellek szövege a `reasoning_content` mezőben külön jön, a válasz streamelve.
2. **Az agent: egy kész, nyílt forrású kódoló agent**, amit a guard **új kliensként** futtat, ugyanúgy, mint a `claude`-ot és az `agy`-t (architektúra-szabály 7: új adapter). Jelöltek: little-coder (pi), OpenCode, Qwen Code; a 47. kör tervezési kapuja méri össze őket a 3060-on, a saját feladatainkon, és a győztes lesz a kliens. A ciklus belseje (eszközhívások, kontextuskezelés, tömörítés) az agenté.
3. **Amit mi írunk: a determinisztikus extrák, MCP szerverként**, a helyi agentnek **és a Claude-agenteknek is**:
   - `run_task`: a jóváhagyott projektprofil parancsai, szűrt hibalistával (4., 10. fejezet);
   - kódkeresés (a 36b kódindexe) és dokumentációkeresés (DevDocs);
   - az újrahasznosítás-ellenőrzés és a duplikációs ítélet (7. fejezet);
   - a guard ellenőrzései a futás előtt és után: a feladat ellenőrzése (3.), a kapuk és a csalásellenőrzés (9.), a sikertelen körök számlálása és a segítségkérés (11.).

**Mi marad érvényes:** a 16. fejezet döntései. Ami a ciklus belsejéről szól (6., 8., 12. fejezet: kontextuscsomag, eszközök, modellbeállítások), az a kiválasztott agentre vonatkozó **elvárás** lesz: ezeken mérjük a jelölteket, és ahol hiányzik, MCP-eszközzel vagy a kliens beállításával pótoljuk.

## 1. Alapelv: mi a modellé, mi a guardé

A helyi modell (12 GB VRAM, 7–14 milliárd paraméter) kevés kontextussal és gyengébb eszközhasználattal dolgozik. Ezért **a modell csak azt csinálja, amihez kreativitás kell**, minden mást a guard végez gépiesen, minden futásnál ugyanúgy.

**A modell feladata (csak ez):**
1. egy függvény vagy metódus **törzsének** megírása a megadott szignatúrához;
2. hiányzó **tesztesetek** megírása, ha a feladat kéri;
3. **választás** a guard által felkínált lehetőségek közül (pl. „ezt a meglévő függvényt használom” vagy „újat írok, mert …”);
4. egy rövid **magyarázat** a változtatásról.

**A guard feladata (determinisztikusan):** a feladat ellenőrzése, a kontextus összeállítása, a keresés (kód, dokumentáció, függőségek), az importok, a formázás, a fordítás, a lint, a tesztek, a hibák szűrése, a duplikáció- és csalásellenőrzés, a javítási kör vezérlése, a feladás és az eredmény visszaadása.

**Szabály:** ha egy lépés gépiesen is eldönthető, a guard dönti el, nem a modell.

## 2. A feladat életciklusa (állapotgép)

```
ÁTVÉTEL → ELLENŐRZÉS → ELŐKÉSZÍTÉS → FELDERÍTÉS → ÍRÁS → KAPUK ─┬→ KÉSZ
              │                       (1. kontextus)  (2. kontextus) │
              │                                         ↑            │
              └→ VISSZADOBVA (a tervezőnek)              └─ JAVÍTÁS ←┘ (5 sikertelen kör után)
                                                                      └→ SEGÍTSÉGKÉRÉS (ember)
                                                                           ├→ folytatás / új utasítás → JAVÍTÁS
                                                                           ├→ átadás Claude-nak
                                                                           └→ LEÁLLÍTVA
```

| Állapot | Ki dolgozik | Mi történik | Kimenet |
| --- | --- | --- | --- |
| Átvétel | guard | a tervező feladata megérkezik (`local_task`) | feladat-azonosító, sorban állás |
| Ellenőrzés | guard | séma- és tartalmi ellenőrzés (3. fejezet) | tovább, vagy visszadobás okkal |
| Előkészítés | guard | worktree, projektprofil, kontextuscsomag (4–6. fejezet) | kontextuscsomag |
| Felderítés | guard gyűjt, a felderítő modell dönt (1. kontextus, csak olvas) | a meglévő megoldások keresése és megítélése (7. fejezet) | ellenőrzött `use` / `extend` / `new` döntés |
| Írás | az író modell (2. kontextus, friss) | törzs vagy teszt megírása, szerkesztő eszközökkel (8. fejezet) | szerkesztések |
| Kapuk | guard | formázás, build, lint, tesztek, ellenőrzések (9. fejezet) | zöld, vagy szűrt hibalista |
| Javítás | az író modell, guard vezérli | a szűrt hibák alapján javít (11. fejezet) | új szerkesztések |
| Segítségkérés | ember | 5 sikertelen javítási kör után a guard embert kér, teljes kontextussal (11. fejezet) | folytatás, új utasítás, átadás Claude-nak vagy leállítás |
| Kész / Leállítva / Visszadobva | guard | eredmény a szálba és a tervezőnek (13. fejezet) | eredményjelentés |

**Korlátok feladatonként** (döntés, 2026-11-09, pontosítva 2026-11-10): **5 sikertelen javítási kör** (hiba → javítás → megint hiba, ötször) után a guard nem adja fel, hanem **embert kér** (11. fejezet); mellette kiinduló alapérték 20 modellhívás és 15 perc; mérés után pontosítjuk. **Mind állítható, és kikapcsolható** (gépenként, projektenként vagy feladatonként). Kikapcsolt korlátnál is megállít a stagnálás felismerése (11. fejezet), így a futás nem pöröghet vég nélkül. Egyszerre egy helyi futás a GPU-n (ez nem korlát, hanem a 49-es kör ütemezője).

## 3. A feladat formája és ellenőrzése

A tervező (Claude vagy a kolléga tervező agentje) **szerkezetes feladatot** ad, nem szöveget:

```yaml
goal: "Parse the agy reset time from a quota message"   # egy mondat
repo: /path/to/repo            # a helyi agent egyik kiadott mappája
language: go                   # vagy felismerjük
target:
  kind: implement_function     # implement_function | add_method | write_tests | fix_failing_test
  file: internal/guard/agyquota.go
  symbol: parseAgyReset
  signature: "func parseAgyReset(msg string) (time.Duration, bool)"
  after_symbol: asAgyQuota     # hová kerüljön (új függvénynél)
acceptance:
  tests:                       # ezeknek zöldnek kell lenniük
    - internal/guard/agyquota_test.go:TestParseAgyReset
  write_tests: false           # a modell írjon-e új teszteket
constraints:
  new_dependencies: false
  max_changed_lines: 80
  read_only_paths: ["**/*_test.go"]   # alapból: a meglévő tesztek nem módosíthatók
hints:                         # nem kötelező: a tervező tippjei
  - "Durations look like 6h43m54s; any part may be missing."
```

**Determinisztikus ellenőrzés (visszadobás okkal, ha nem teljesül):**
- a `target.file` létezik, vagy új fájlnál a mappa létezik;
- `implement_function` esetén van szignatúra, és az a nyelv szerint értelmes (a guard a fordítóval vagy elemzővel ellenőrzi);
- van elfogadási feltétel (létező teszt, vagy `write_tests: true`);
- a megadott tesztek léteznek, és *most* pirosak (különben nincs mit megoldani); `fix_failing_test` esetén a teszt piros;
- a feladat egy fájlt és egy szimbólumot érint;
- a mappa a helyi agent kiadott mappái között van.

**Döntés (2026-11-09): egy feladat = egy szimbólum.** Ami nagyobb, azt felbontjuk.

### 3.1 Feladatbontás (egy nagyobb munkából egyszimbólumos feladatok)

A tervező adhat egy nagyobb munkát is (pl. „valósítsd meg ezt az interfészt”, „legyenek zöldek ezek a tesztek”). Ezt a guard bontja egyszimbólumos feladatokra, **először gépiesen, a modell csak tartalék**:

1. **Determinisztikus bontás** a metódus- és fájlnevek, a CodeGraph és a fordító alapján:
   - meg nem valósított interfészmetódusok vagy absztrakt metódusok → metódusonként egy `implement_function`;
   - piros tesztek → a teszt által hívott, hiányzó vagy hibás szimbólumok (fordítási hiba, veremnyom) → szimbólumonként egy feladat;
   - a tervező által felsorolt szimbólumok (név, fájl, szignatúra) → szimbólumonként egy feladat;
   - a sorrend a függőségi gráfból jön: előbb a hívott, utána a hívó.
2. **A helyi modell mint tartalék**, ha a gépies bontás nem elég (pl. a munka csak szövegesen van leírva): a modell **kötött sémában** (3. fejezet) feladatlistát ad, amit a guard ugyanúgy ellenőriz, mint a tervezőét; amit nem tud ellenőrizni, azt visszaadja a tervezőnek.
3. A részfeladatok **egy közös ágon**, sorban futnak; ha az egyik feladva, a többi vár, és a tervező kap jelentést.

**Nyitott (a 47-es tervezési kapuhoz):** a bontás pontos szabályai nyelvenként; mikor adja vissza a guard a munkát a tervezőnek bontás helyett.

## 4. Projektprofil (milyen parancsok léteznek)

A guard projektenként felismeri a parancsokat; a modell **csak ezeket kérheti, név szerint**, szabad shell nincs.

| Nyelv | Felismerés | format | build / típusellenőrzés | lint | teszt (egy célra) | érintett tesztek |
| --- | --- | --- | --- | --- | --- | --- |
| Go | `go.mod` | `goimports -w`, `gofmt` | `go build ./...`, `go vet` | `golangci-lint run --out-format json` (ha van config) | `go test -json -run '^Név$' ./csomag` | CodeGraph `affected` |
| TypeScript/JS (Node) | `package.json`, `tsconfig.json` | `prettier --write` (ha van config) | `tsc --noEmit` | `eslint -f json` | `vitest run --reporter=json fájl` vagy `jest --json` | CodeGraph `affected` |
| Python | `pyproject.toml`, `requirements*.txt` | `ruff format` (vagy `black`) | `mypy` (ha van config), `python -m compileall` | `ruff check --output-format json` | `pytest -q --junitxml=… útvonal::teszt` | CodeGraph `affected` |
| Java | `pom.xml` / `build.gradle(.kts)` | `google-java-format` (ha van) | `mvn -q compile` / `gradle compileJava` | Checkstyle/SpotBugs (ha be van állítva) | `mvn -Dtest=Osztály#metódus test` / `gradle test --tests` | CodeGraph `affected` |

| PHP (később) | `composer.json` | `php-cs-fixer fix` / `phpcbf` (ha van config) | `php -l`, `phpstan analyse --error-format=json` (ha van config) | PHP_CodeSniffer JSON (ha be van állítva) | `phpunit --filter Név --log-junit …` | CodeGraph `affected` |

**Döntés (2026-11-09): egyszer felderítve, ember jóváhagyja, a guard elmenti.**
- **Felderítés:** az első feladat előtt a guard felismeri a projektet (táblázat fent, plusz a `Makefile`, a `package.json` scriptjei, a CI-fájlok), és **profiljavaslatot** tesz: hogyan kell telepíteni a függőségeket, buildelni, tesztelni, lintelni, formázni, elindítani.
- **Jóváhagyás:** a javaslatot ember hagyja jóvá a felületen (a Gépek oldalon, projektenként); jóváhagyás nélkül nincs futás.
- **Mentés:** a guard a jóváhagyott profilt elmenti (projektenként, a gépen és a szerveren is látható), és onnantól **csak azt** használja, nem ismeri fel újra futásonként.
- **Szerkesztés futás közben is:** a parancsok a felületen hozzáadhatók, szerkeszthetők, törölhetők; minden változás új profilverzió (ki, mikor), a futásnaplóban a használt profilverzió látszik.
- **Változásjelzés:** ha a repó jelentősen változik (új `package.json` script, új `Makefile` cél, más build-eszköz), a guard új javaslatot tesz, de nem cseréli magától.
- **Két külön lépés (döntés):** a **letöltő** (`deps`: `go mod download`, `npm ci`, `pip install`, `composer install` …) hálózattal fut, a profil saját lépéseként; a **futtató** (build, lint, teszt) **hálózat nélkül**. Ha egy projekt tesztjei hálózatot kérnek, az a profilban külön, jóváhagyandó engedély.
- **Ha egy eszköz hiányzik** (pl. nincs golangci-lint), az a kapu kimarad, és ez az eredményben látszik („lint: nincs beállítva”).
- A repóban lévő `colonyhall.tasks.yaml` (ha van) csak **javaslat** a felderítéshez, nem futtatható közvetlenül: a repó tartalma adat, nem utasítás, ezért ebből is csak jóváhagyás után lesz profil.

## 5. Munkaterület és biztonság

- Minden feladat **saját git worktree-ben** fut (a 12. kör mintájára), a feladat ágán. A kiinduló állapotot a guard rögzíti (commit hash).
- **Pillanatkép minden szerkesztés előtt**; ha a kapuk elbuknak, és a modell nem javítja, a guard visszaállítja az utolsó jó állapotot.
- A futás **sandboxban** (bwrap), hálózat nélkül; a modellszerver, a CodeGraph, a keresés és a dokumentáció a guard hídjain át érhető el.
- A parancsok környezete tiszta: nincs titok a környezeti változókban, a hálózat ki van kapcsolva a futtató lépésekben (build, lint, teszt); csak a külön letöltő lépés kap hálózatot (4. fejezet, döntés).

## 6. A kontextuscsomag (mit lát a modell)

A guard minden modellhívás előtt **tokenkeretből** gazdálkodva rakja össze, mit lát a modell. Példa 16 000 tokenes kontextusra (javaslat, méréssel pontosítjuk):

| Rész | Keret | Honnan |
| --- | --- | --- |
| Rendszerutasítás és szabályok | 800 | rögzített, verziózott sablon |
| A feladat (3. fejezet) | 400 | a tervezőtől |
| A célfájl váza: importok, típusok, a szomszédos függvények szignatúrái | 1 500 | CodeGraph / nyelvi elemző |
| A célfüggvény hívói és hívottjai (csak szignatúra + 1 soros leírás) | 1 500 | CodeGraph |
| Az érintett típusok definíciója | 1 500 | CodeGraph |
| Újrahasznosítási jelöltek (7. fejezet) | 1 200 | kódindex, függőségek, standard könyvtár |
| Az elfogadási tesztek kódja | 2 000 | a tesztfájlból, csak az adott teszt(ek) |
| Dokumentációrészletek (automatikus, 10. fejezet) | 1 000 | DevDocs |
| Előző kör szűrt hibái (javításkor) | 1 000 | 9–10. fejezet |
| A modell válaszának tartalék | 5 100 | – |

- **Determinisztikus vágás:** ha egy rész nem fér bele, a guard rögzített sorrend szerint rövidít (először a dokumentáció, aztán a hívók, legutoljára a teszt és a feladat), és jelöli, mi maradt ki.
- **Egész fájlt soha nem kap a modell**, csak szimbólumokat és ablakokat.

**Döntés (2026-11-09): a modell kérhet plusz kontextust, darabszám-plafon nélkül; a guard a kontextus telítettségét figyeli, és dinamikusan ad.**
- A modell `show_symbol`, `search_code`, `search_docs` hívással kérhet többet, ahányszor akar.
- A guard minden hívás előtt méri: a kontextusablak mekkora része foglalt (tokenben), mennyi kell a válasznak, és a GPU-n mennyi a szabad VRAM (a KV-cache a kontextussal együtt nő; 49-es kör, host report).
- **Amíg belefér:** a kért részlet hozzákerül.
- **Ha közel a határ:** a guard gépiesen helyet csinál, rögzített sorrendben: a régebbi körök nyers kimenete helyett rövid összefoglaló; a már nem használt részletek (pl. egy korábbi `show_symbol`) kiesnek; a dokumentáció rövidül. Ha így sem fér, a kért részlet csak a szignatúrával és az egysoros leírással kerül be, és a modell ezt tudja.
- **Ha a modell több kontextust is kezelne, és a VRAM engedi,** a guard nagyobb kontextusablakkal töltheti be a modellt (a 49-es kör ütemezőjével egyeztetve).
- Minden kérés naplózva (mit kért, mi fért bele), ez a méréshez is kell: ha egy feladattípusnál mindig ugyanazt kéri, azt a guard előre beteheti a csomagba.

## 7. Újrahasznosítás írás előtt (a Ponytail-létra, gépiesen)

**Döntés (2026-11-10): a felderítés külön kontextusban fut, az írás egy friss kontextusban.** A gépies keresés csak **jelölteket** talál; hogy egy jelölt tényleg megoldja-e a feladatot, az értelmezés, ezért egy **felderítő modell** ítéli meg, és a guard gépiesen ellenőrzi az ítéletét. (Ugyanez a minta a kutatásban és a gyakorlatban: az Agentless külön helykereső fázisa, az Aider architect/editor módja, a little-coder külön tervező módja és friss végrehajtó munkamenete.)

**1. lépés: a guard jelölteket gyűjt** (gépiesen), a létra minden fokán, és megáll az első találatnál, ami átlépi a küszöböt:

1. **Kell-e egyáltalán?** Ha a célszimbólum már létezik ugyanazzal a szignatúrával, és az elfogadási tesztek zöldek, a feladat „már kész” állapotban visszamegy a tervezőnek.
2. **Van-e ilyen a kódbázisban?** Hibrid keresés a kódindexben (név, szignatúra, egysoros leírás), plusz CodeGraph-keresés hasonló nevekre és azonos paramétertípusokra. Küszöb felett: jelölt.
3. **Megoldja-e a standard könyvtár?** Keresés a nyelv standard könyvtárának dokumentációjában (DevDocs) a feladat kulcsszavaira.
4. **Megoldja-e egy már telepített függőség?** Keresés a projekt függőségeinek exportált API-jában (Go: modulcache; Node: `node_modules` típusdefiníciói; Python: telepített csomagok; Java: a függőségek osztályai).

**2. lépés: a felderítő modell dönt** (1. kontextus, csak olvasó eszközök).
- **Kap:** a feladatot, a guard jelöltjeit (név, helye, szignatúrája, egysoros leírás), és olvasó eszközöket: CodeGraph, `search_code`, `search_docs` (DevDocs), a függőségek exportált API-ja, `show_symbol`.
- **Válasz kötött sémában**, bizonyítékkal:

```json
{ "reuse": "use",    "symbol": "strings.Fields", "evidence": "splits on whitespace, which is all the task needs" }
{ "reuse": "extend", "symbol": "internal/guard.parseDuration", "evidence": "handles h/m/s but not a missing part; one branch to add" }
{ "reuse": "new",    "evidence": "none of the candidates parses durations with missing parts",
  "checked": ["time.ParseDuration", "internal/guard.parseDuration"] }
```

**3. lépés: a guard ellenőrzi az ítéletet** (gépiesen; ami nem stimmel, azt eldobja, és egyszer újrakérdez):
- a hivatkozott szimbólum létezik, a szignatúrája illik a feladathoz;
- hívható ebből a modulból (a modulhatár-szabályok, pl. a depguard szerint);
- `extend` esetén a szimbólum a kiadott mappákban van, és írható;
- `new` esetén a `checked` lista tartalmazza a guard legjobb jelöltjeit (a felderítő nem hagyhatja ki őket).

**4. lépés: az író modell** (2. kontextus, friss) csak a feladatot, az ellenőrzött döntést és a szükséges részleteket kapja; a felderítés keresgélése nem foglalja a kontextusát.
- `use` esetén a guard ellenőrzi, hogy az írt kód tényleg hívja a választott szimbólumot;
- `extend` esetén a feladat célszimbóluma a meglévő szimbólumra vált (a tervező jóváhagyásával, ha más fájlban van);
- `new` esetén az írás utáni duplikációellenőrzés (9. fejezet) a második réteg.

**Modellek szerepenként** (döntés, 2026-11-10): a felderítő és az író modell **külön beállítható** (`scout_model`, `writer_model`); lehet ugyanaz a modell két kontextusban, vagy két különböző. Hogy melyik legyen, Norbi dönti el, miután a 49-es körben lemérte, mely modellek futnak jól és megbízhatóan a 3060-on. A **modellcsere megengedett** (egy kártyán kb. 15 másodperc újratöltés): megvárjuk, és **a felületen látszik** („modellcsere: qwen… → …”), a 49-es kör ütemezőjével egyeztetve.

A küszöbértékeket (hasonlósági pontszámok) a mérőkészleten állítjuk be.

**Döntés (2026-11-09): a duplikációs ítéletet csak ember bírálhatja felül, a modell kérésére, indoklással.**
1. A guard elutasítja az új szimbólumot („ez már létezik itt: …”), és megmutatja a meglévőt.
2. A modell **vagy** átáll a meglévőre, **vagy** vitatja: ekkor **kötelező indoklást** ad kötött sémában, hogy miért kell az új (pl. „más modul, a szabályok szerint nem hívhatja”, „a meglévő nem kezeli a hiányzó részeket”). Indoklás nélkül a vita érvénytelen, és a feladat a szokásos javítási körrel megy tovább.
3. Vitatott ítéletnél a guard **embert kér**: kérdés a szálban (a jóváhagyási kérdések mintájára, `Q-NN`), benne a két szimbólum egymás mellett, a hasonlósági pontszám és a modell indoklása. A feladat addig vár, a GPU közben más munkát kaphat.
4. Az ember három közül választ: **az új maradhat** (a guard erre az egy párra nem dob vissza többet), **használja a meglévőt**, vagy **a feladat leáll** (a tervező jelentést kap).
5. A döntés, az indoklás és a döntő ember naplózva, és a mérésbe is bekerül (ha egy küszöb sokszor téved, állítani kell rajta).
- A modell maga soha nem írhatja felül, és a tervező sem: ez ember döntése.

## 8. Eszközök, amiket a modell kap

Kevés, szűk eszköz, mindegyik JSON-sémával, kötött kimenettel (a modell csak sémának megfelelő hívást tud adni):

| Eszköz | Mit csinál | Mit nem enged |
| --- | --- | --- |
| `replace_body(symbol, body)` | a célfüggvény törzsének cseréje | más szimbólum módosítását |
| `add_function(after_symbol, code)` | új függvény a megadott helyre | más fájlba írást (csak a célfájl) |
| `add_test(test_file, code)` | új teszt, ha `write_tests: true` | meglévő teszt módosítását |
| `show_symbol(name)` | egy szimbólum forrása (kontextuskeretből) | egész fájl olvasását |
| `search_code(query)` / `search_docs(query)` | keresés (kódindex, DevDocs) | hálózatot |
| `run(task, target)` | a profil egy parancsa (`build`, `test` …) | szabad parancsot |
| `done(summary)` | a modell késznek jelöli | – |

- **Importok:** a modell **soha nem kezel importot**. A guard szerkesztés után automatikusan rendezi őket (Go: `goimports`; TS: a TypeScript nyelvi szerver „organize imports”; Python: `ruff --fix` az import-szabályokra; Java: IDE-szintű eszköz vagy egyszerű feloldás). Ami nem oldható fel, az fordítási hibaként jön vissza.
- **Nincs:** shell, fájlírás tetszőleges helyre, hálózat, git-parancs.

## 9. Kapuk (gépies ellenőrzések minden szerkesztés után)

Rögzített sorrend, az első bukásnál megáll (fail-fast), és csak az addigi hibákat adja vissza:

1. **Formázás és importrendezés** (automatikus javítás, nem hiba).
2. **Szintaxis és fordítás / típusellenőrzés.**
3. **Tiltott minták a változtatott sorokban** (csalásellenőrzés):
   - új lint-kikapcsolás (`//nolint`, `eslint-disable`, `@ts-ignore`, `# type: ignore`, `@SuppressWarnings`);
   - teszt átugrása vagy gyengítése (`t.Skip`, `.skip`, `@pytest.mark.skip`, `@Disabled`), üres catch/except, `TODO`/`FIXME` maradvány, hibakereső kiírás (`fmt.Println`, `console.log`, `print`), keménykódolt elvárt érték, ami csak a tesztet elégíti ki (heurisztika, figyelmeztetés);
   - meglévő teszt vagy `read_only_paths` alá eső fájl módosítása;
   - új függőség (`go.mod`, `package.json`, `requirements*.txt`, `pyproject.toml`, `pom.xml`, `build.gradle` változása), ha a feladat nem engedi.
4. **Méret:** a változtatott sorok száma a `max_changed_lines` alatt; csak a célfájl (és engedélyezett tesztfájl) változott.
5. **Lint** csak a változtatott fájlokra, csak az új hibák (a meglévő hibák nem a modell dolga).
6. **Célzott tesztek:** az elfogadási tesztek, utána a CodeGraph szerint érintett tesztek.
7. **Duplikációellenőrzés:** az új szimbólum nem majdnem-másolata-e egy meglévőnek (név- és szignatúra-hasonlóság, normalizált törzs-ujjlenyomat, embedding-hasonlóság a kódindexben). Küszöb felett: elutasítás „ez már létezik itt: …” üzenettel.
8. **Végső teljes futás** (döntés, 2026-11-09): csak az érintett csomag vagy modul összes tesztje; az egész repó a CI dolga (`make ci`, GitHub CI).

## 10. Hibaszűrés és automatikus kontextus-kiegészítés

**Egységes hibaformátum** minden nyelvre:

```json
{ "kind": "compile|lint|test|gate", "file": "internal/guard/agyquota.go", "line": 71, "col": 9,
  "rule": "undefined", "message": "undefined: parseResetX",
  "code": ["69 | ...", "70 | ...", "71 | d, ok := parseResetX(m)", "72 | ...", "73 | ..."],
  "test": { "name": "TestParseAgyReset/6h43m54s", "expected": "6h43m54s", "actual": "0s" } }
```

- **Forrás:** a gépileg olvasható kimenet (`go test -json`, golangci-lint JSON, `tsc`, ESLint JSON, Vitest/Jest JSON, `ruff` JSON, `mypy`, `pytest` JUnit XML, `javac`, Maven/Gradle Surefire XML).
- **Rendezés:** fordítási hibák előre, utána a lint, végül a teszthibák; ismétlődők összevonva (azonos szabály, azonos ok); a veremnyomból csak a projekt saját keretei; a teszthibánál az elvárt és a kapott érték külön mezőben.
- **Mennyiség (döntés, 2026-11-10):** nincs darabszám-plafon körönként (helyben fut, nincs tokendíj); a határt a kontextus telítettsége adja (6. fejezet), mert a kontextusablak helyben is véges, és a kis modellt a zaj összezavarja. Ha nem fér be minden, a sorrend szerint a hátsók csak egy sorban (fájl, sor, szabály) maradnak benne, és a modell tudja, hány maradt ki.
- **Automatikus kiegészítés (determinisztikus triggerek):**
  - ismeretlen azonosító (`undefined: X`, `Cannot find name 'X'`, `NameError`, `cannot find symbol`) → a guard rákeres X-re a kódban és a dokumentációban, és a találat szignatúráját a hibához csatolja;
  - típuseltérés → a két típus definíciója a hibához csatolva;
  - rossz számú argumentum → a hívott függvény szignatúrája csatolva;
  - teszthiba → a teszt forrása és az assertion sora csatolva.

## 11. A javítási kör vezérlése

- **Körönként:** a modell a szűrt hibákat és a csatolt részleteket kapja, és csak szerkesztő eszközt hívhat.
- **Stagnálás felismerése:** ha ugyanaz a hiba két egymást követő körben megmarad, vagy a hibák száma nem csökken, a guard **stratégiát vált** gépiesen:
  1. bővebb kontextus (a hívottak teljes forrása, a dokumentáció);
  2. visszaállítás az eredeti állapotra, és újrakezdés a hibalistával mint „ezeket kerüld el” megjegyzéssel.
- **Segítségkérés (döntés, 2026-11-10):** **5 sikertelen javítási kör** után (hiba → javítás → megint hiba, ötször) a guard nem adja fel, hanem **embert kér**: kérdés a szálban (`Q-NN`, a jóváhagyási kérdések mintájára). A feladat addig vár, a worktree és a pillanatképek megmaradnak, a GPU közben más munkát kaphat.

### 11.1 Amit az ember kap (amennyi kontextust csak lehet)

A kérdés rövid összefoglalóval kezdődik, a részletek egy kattintással nyílnak (a futás részletei között, csak embernek):

- **Hol akadt el:** a fájl, a szimbólum, a sor; melyik kapu bukik (fordítás, lint, melyik teszt); egy mondatban gépiesen: „a `TestParseAgyReset/6h43m54s` 5 köre piros: elvárt `6h43m54s`, kapott `0s`”.
- **A feladat** teljesen (3. fejezet), a tervező tippjeivel, és **a felderítő döntése** a bizonyítékkal (7. fejezet).
- **Körönkénti történet:** minden körnél a modell rövid magyarázata, a diff, a kapuk eredménye és a szűrt hibák; látszik, változott-e a hiba vagy ugyanaz maradt (a stagnálás felismerésének jelzéseivel, és hogy melyik stratégiát próbálta a guard).
- **A jelenlegi diff** az eredeti állapothoz képest, és az utolsó zöld pillanatkép.
- **Az utolsó kör teljes, szűretlen kimenete** a futtatott parancsokból (a profil parancsai, verzióval), ha a szűrt nézet nem elég.
- **Amit a modell látott:** a kontextuscsomag összetétele (mi fért bele, mi maradt ki), a modell és a promptsablon verziója.
- **A guard gépies javaslata**, ha van (pl. „a feladat két szimbólumot érint, bontsd ketté”, „a teszt hálózatot kér”, „a hiba egy nem kiadott fájlban van”).
- **Hivatkozások:** a szál, ahonnan a feladat jött, a futásnapló, a worktree útvonala (meg lehet nyitni a fájlböngészőben és a terminálban).

### 11.2 Amit az ember választhat

- **Folytatás:** további javítási körök (pl. még 5), változatlan feladattal.
- **Új utasítás vagy tipp:** szöveg, ami a feladat `hints` mezőjébe kerül (adat, nem a rendszerutasítás része), és onnan folytatódik a javítás, vagy előről az írás.
- **Átadás Claude-nak** (egy gomb): a feladat a fenti teljes kontextussal (11.1) sémás csomagként megy egy kiválasztott Claude-agentnek (alapból a tervezőnek); Claude vagy befejezi, vagy új, pontosabb feladatot ad a helyi agentnek.
- **Leállítás:** a feladat leáll, a tervező szerkezetes jelentést kap (mi sikerült, mi nem, az utolsó hibalista és a guard javaslata).

A döntés, a döntő ember és az esetleges új utasítás naplózva, és a mérésbe is bekerül (melyik feladattípusnál kell gyakran ember).

## 12. A modell beállításai

- **Hőmérséklet 0, rögzített seed** (amennyire a futtató engedi), hogy ugyanarra a bemenetre ugyanazt adja.
- **Kötött kimenet:** az eszközhívások JSON-sémával (llama.cpp nyelvtan vagy az Ollama `format` mezője).
- **Rögzített, verziózott promptsablonok**; a sablon verziója a futásnaplóban szerepel (mint az utasításverzió a 35. kör óta).

## 13. Az eredmény

```yaml
status: done            # done | stopped | rejected | already_done | handed_over
task_id: …
branch: local/<handle>/<task>
diff_stats: { files: 1, added: 24, removed: 3 }
gates: { format: ok, build: ok, forbidden: ok, size: ok, lint: ok, tests: ok, duplicate: ok }
reuse: { decision: use, symbol: strings.Fields }
new_symbols: [parseAgyReset]
rounds: 2
model: qwen2.5-coder:14b-q4_K_M
prompt_template: v3
notes: "…"              # a modell rövid magyarázata
```

- A szálba egy rövid összefoglaló megy, a különbség a futás részleteinél látszik (a futás-Sheetben).
- **A változtatást a tervező (vagy ember) fogadja el:** a helyi agent soha nem olvaszt be semmit.

## 14. Megfigyelhetőség és visszajátszás

- Minden lépés naplózva: állapotváltások, a kontextuscsomag összetétele (mi fért bele, mi maradt ki), minden modellhívás bemenetének hash-e és a kimenet, a kapuk eredménye.
- **Visszajátszás:** egy feladat újrafuttatható ugyanazzal a modellel és sablonnal, és összevethető (ez a mérőkészlethez és a hibakereséshez kell).

## 15. Mérés

- Kb. 20 feladat a saját repóidból, nyelvenként néhány, ismert jó megoldással; a feladatok a 3. fejezet formájában.
- **Mérőszámok:** sikeres-e (kapuk zöldek, és a megoldás megegyezik vagy egyenértékű), körök száma, idő, tokenek, visszadobott duplikációk, csalásellenőrzés találatai.
- **Összevetés:** modellek és tömörítések, a keretrendszer egyes részei be- és kikapcsolva (mennyit ér az újrahasznosítás-létra, a hibaszűrés, a kontextus-kiegészítés), és a helyi agent a Claude-dal ugyanazokon a feladatokon.

## 16. Döntések és nyitott kérdések

**Döntések (Norbi, 2026-11-09 és 2026-11-10):**

1. **Egy feladat = egy szimbólum.** A nagyobb munkát a guard bontja: először gépiesen (metódus- és fájlnevek, CodeGraph, fordító, piros tesztek), tartalékként a helyi modell kötött sémában (3.1).
2. **Projektprofil:** egyszer felderítve, ember jóváhagyja, a guard elmenti; a parancsok futás közben is hozzáadhatók, szerkeszthetők, törölhetők a felületen (4.).
3. **Hálózat:** külön letöltő lépés hálózattal, külön futtató hálózat nélkül (4., 5.).
4. **Plusz kontextus:** a modell kérhet, darabszám-plafon nélkül; a guard a kontextus telítettségét és a VRAM-ot figyeli, és dinamikusan ad, illetve gépiesen tömörít (6.).
5. **Korlátok:** 20 hívás, 15 perc kiindulásnak; állíthatók és kikapcsolhatók; a stagnálás felismerése mindig stratégiát vált (2., 11.).
6. **Végső teljes futás:** csak az érintett modul; az egész repó a CI dolga (9.).
7. **Nyelvek:** először Go és TypeScript (hogy a Colonyhallon mérhessük), utána Python, Java, PHP (4., 10.).
8. **Duplikációs ítélet:** csak ember bírálhatja felül. Ha a modell indoklással vitatja, a guard kérdést tesz fel a szálban, és az ember dönt: maradhat, használja a meglévőt, vagy leáll (7.).
9. **Felderítés és írás külön kontextusban** (2026-11-10): a felderítő modell olvasó eszközökkel ítéli meg a guard jelöltjeit (`use` / `extend` / `new`, bizonyítékkal), a guard gépiesen ellenőrzi, az író modell friss kontextusban dolgozik (7.).
10. **Modellek szerepenként:** a felderítő és az író külön beállítható (ugyanaz vagy más modell); Norbi a 3060-on végzett mérések után választ. A modellcsere (kb. 15 mp) megengedett, a felületen látszik (7.).
11. **Hibák körönként:** nincs darabszám-plafon, a kontextus telítettsége a határ (10.).
12. **Segítségkérés:** 5 sikertelen javítási kör után a guard embert kér, a lehető legtöbb kontextussal (hol akadt el, körönkénti történet, diff, kimenetek); az ember folytathatja, új utasítást adhat, átadhatja Claude-nak egy gombbal, vagy leállíthatja (11.).

13. **Kész eszközökre építünk (2026-11-10):** `llama.cpp` + llama-swap a futtató (Ollama nélkül), egy kész kódoló agent új kliensként (a tervezési kapu választ: little-coder, OpenCode, Qwen Code), és mi csak a determinisztikus extrákat írjuk MCP szerverként (0. fejezet).

**Nyitott:**

1. **A feladatbontás pontos szabályai** nyelvenként, és mikor kerül vissza a munka a tervezőhöz bontás helyett (3.1; a 47-es tervezési kapuban).
