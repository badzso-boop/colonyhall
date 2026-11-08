# Lokális kódoló agent: a determinisztikus keretrendszer részletes terve

> **Állapot: vázlat, Norbi és Claude Code közös kidolgozás alatt (2026-11-08).** Ebből készül a végleges 47-es handoff prompt. Ami itt „Javaslat”, az még nem döntés; a „Nyitott” jelzésűek Norbi döntésére várnak.

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
ÁTVÉTEL → ELLENŐRZÉS → ELŐKÉSZÍTÉS → ÚJRAHASZNOSÍTÁS → ÍRÁS → KAPUK ─┬→ KÉSZ
              │                                         ↑           │
              └→ VISSZADOBVA (a tervezőnek)              └─ JAVÍTÁS ←┘ (max N kör)
                                                                     └→ FELADVA
```

| Állapot | Ki dolgozik | Mi történik | Kimenet |
| --- | --- | --- | --- |
| Átvétel | guard | a tervező feladata megérkezik (`local_task`) | feladat-azonosító, sorban állás |
| Ellenőrzés | guard | séma- és tartalmi ellenőrzés (3. fejezet) | tovább, vagy visszadobás okkal |
| Előkészítés | guard | worktree, projektprofil, kontextuscsomag (4–6. fejezet) | kontextuscsomag |
| Újrahasznosítás | guard, majd modell választ | a meglévő jelöltek keresése (7. fejezet) | „használja X-et” vagy „újat ír” döntés |
| Írás | modell | törzs vagy teszt megírása, szerkesztő eszközökkel (8. fejezet) | szerkesztések |
| Kapuk | guard | formázás, build, lint, tesztek, ellenőrzések (9. fejezet) | zöld, vagy szűrt hibalista |
| Javítás | modell, guard vezérli | a szűrt hibák alapján javít (11. fejezet) | új szerkesztések |
| Kész / Feladva / Visszadobva | guard | eredmény a szálba és a tervezőnek (13. fejezet) | eredményjelentés |

**Korlátok feladatonként** (javaslat, mérés után pontosítjuk): legfeljebb 4 javítási kör, 20 modellhívás, 15 perc, egyszerre egy helyi futás a GPU-n.

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
- a feladat egy fájlt és egy szimbólumot érint (javaslat: több szimbólumot a tervező bontson több feladatra);
- a mappa a helyi agent kiadott mappái között van.

**Nyitott:** engedünk-e több szimbólumot egy feladatban, vagy szigorúan egyet?

## 4. Projektprofil (milyen parancsok léteznek)

A guard projektenként felismeri a parancsokat; a modell **csak ezeket kérheti, név szerint**, szabad shell nincs.

| Nyelv | Felismerés | format | build / típusellenőrzés | lint | teszt (egy célra) | érintett tesztek |
| --- | --- | --- | --- | --- | --- | --- |
| Go | `go.mod` | `goimports -w`, `gofmt` | `go build ./...`, `go vet` | `golangci-lint run --out-format json` (ha van config) | `go test -json -run '^Név$' ./csomag` | CodeGraph `affected` |
| TypeScript/JS (Node) | `package.json`, `tsconfig.json` | `prettier --write` (ha van config) | `tsc --noEmit` | `eslint -f json` | `vitest run --reporter=json fájl` vagy `jest --json` | CodeGraph `affected` |
| Python | `pyproject.toml`, `requirements*.txt` | `ruff format` (vagy `black`) | `mypy` (ha van config), `python -m compileall` | `ruff check --output-format json` | `pytest -q --junitxml=… útvonal::teszt` | CodeGraph `affected` |
| Java | `pom.xml` / `build.gradle(.kts)` | `google-java-format` (ha van) | `mvn -q compile` / `gradle compileJava` | Checkstyle/SpotBugs (ha be van állítva) | `mvn -Dtest=Osztály#metódus test` / `gradle test --tests` | CodeGraph `affected` |

- **Felülírás:** egy opcionális `colonyhall.tasks.yaml` a repó gyökerében (ugyanazok a kulcsok: `format`, `build`, `lint`, `test`, `test_one`); a guard séma szerint ellenőrzi, és csak a projekt saját eszközeit engedi (pl. `make`, `npm run`, a nyelv eszközei).
- **Ha egy eszköz hiányzik** (pl. nincs golangci-lint), az a kapu kimarad, és ez az eredményben látszik („lint: nincs beállítva”).
- **Nyitott:** a projektprofilt a guard minden futás előtt újra felismeri, vagy egyszer elmenti és a Gépek oldalon jóváhagyod?

## 5. Munkaterület és biztonság

- Minden feladat **saját git worktree-ben** fut (a 12. kör mintájára), a feladat ágán. A kiinduló állapotot a guard rögzíti (commit hash).
- **Pillanatkép minden szerkesztés előtt**; ha a kapuk elbuknak, és a modell nem javítja, a guard visszaállítja az utolsó jó állapotot.
- A futás **sandboxban** (bwrap), hálózat nélkül; a modellszerver, a CodeGraph, a keresés és a dokumentáció a guard hídjain át érhető el.
- A parancsok környezete tiszta: nincs titok a környezeti változókban, a hálózat ki van kapcsolva a tesztek alatt is (javaslat; ha egy projekt tesztjei hálózatot kérnek, az a profilban külön engedély).

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
- **Nyitott:** a modell kérhet-e plusz kontextust egy eszközzel (pl. `show_symbol`), vagy csak azt látja, amit a guard összerakott? Javaslat: kérhet, de csak keretből, és legfeljebb 3-szor feladatonként.

## 7. Újrahasznosítás írás előtt (a Ponytail-létra, gépiesen)

A guard **minden írás előtt** végigmegy a létrán, és megáll az első találatnál, ami átlépi a küszöböt:

1. **Kell-e egyáltalán?** Ha a célszimbólum már létezik ugyanazzal a szignatúrával, és az elfogadási tesztek zöldek, a feladat „már kész” állapotban visszamegy a tervezőnek.
2. **Van-e ilyen a kódbázisban?** Hibrid keresés a kódindexben (név, szignatúra, egysoros leírás), plusz CodeGraph-keresés hasonló nevekre és azonos paramétertípusokra. Küszöb felett: jelölt.
3. **Megoldja-e a standard könyvtár?** Keresés a nyelv standard könyvtárának dokumentációjában (DevDocs) a feladat kulcsszavaira.
4. **Megoldja-e egy már telepített függőség?** Keresés a projekt függőségeinek exportált API-jában (Go: modulcache; Node: `node_modules` típusdefiníciói; Python: telepített csomagok; Java: a függőségek osztályai).

**Amit a modell kap:** a legjobb legfeljebb 5 jelölt (név, helye, szignatúrája, egysoros leírás), és egy kötelező választás a válaszában:

```json
{ "reuse": "use", "symbol": "strings.Fields" }
{ "reuse": "new", "reason": "none of the candidates parses durations with missing parts" }
```

**Ami gépies a választás után:**
- `use` esetén a guard ellenőrzi, hogy a modell által írt kód tényleg hívja a választott szimbólumot;
- `new` esetén az írás utáni duplikációellenőrzés (9. fejezet) dönt, nem a modell indoklása.

**Nyitott:** a küszöbértékeket (hasonlósági pontszámok) a mérőkészleten állítjuk be. Kell-e emberi felülbírálás, ha a guard szerint duplikáció, de a tervező szerint nem?

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
8. **Végső teljes futás** (javaslat: csak a csomag vagy modul összes tesztje, nem az egész repó; az egész repó a tervező vagy a CI dolga).

## 10. Hibaszűrés és automatikus kontextus-kiegészítés

**Egységes hibaformátum** minden nyelvre:

```json
{ "kind": "compile|lint|test|gate", "file": "internal/guard/agyquota.go", "line": 71, "col": 9,
  "rule": "undefined", "message": "undefined: parseResetX",
  "code": ["69 | ...", "70 | ...", "71 | d, ok := parseResetX(m)", "72 | ...", "73 | ..."],
  "test": { "name": "TestParseAgyReset/6h43m54s", "expected": "6h43m54s", "actual": "0s" } }
```

- **Forrás:** a gépileg olvasható kimenet (`go test -json`, golangci-lint JSON, `tsc`, ESLint JSON, Vitest/Jest JSON, `ruff` JSON, `mypy`, `pytest` JUnit XML, `javac`, Maven/Gradle Surefire XML).
- **Rendezés és vágás:** fordítási hibák előre; fájlonként az első; legfeljebb 5 hiba körönként; ismétlődők összevonva; a veremnyomból csak a projekt saját keretei; a teszthibánál az elvárt és a kapott érték külön mezőben.
- **Automatikus kiegészítés (determinisztikus triggerek):**
  - ismeretlen azonosító (`undefined: X`, `Cannot find name 'X'`, `NameError`, `cannot find symbol`) → a guard rákeres X-re a kódban és a dokumentációban, és a találat szignatúráját a hibához csatolja;
  - típuseltérés → a két típus definíciója a hibához csatolva;
  - rossz számú argumentum → a hívott függvény szignatúrája csatolva;
  - teszthiba → a teszt forrása és az assertion sora csatolva.

## 11. A javítási kör vezérlése

- **Körönként:** a modell a szűrt hibákat és a csatolt részleteket kapja, és csak szerkesztő eszközt hívhat.
- **Stagnálás felismerése:** ha ugyanaz a hiba két egymást követő körben megmarad, vagy a hibák száma nem csökken, a guard **stratégiát vált** gépiesen:
  1. bővebb kontextus (a hívottak teljes forrása, a dokumentáció);
  2. visszaállítás az eredeti állapotra, és újrakezdés a hibalistával mint „ezeket kerüld el” megjegyzéssel;
  3. feladás.
- **Feladás:** a korlát elérésekor a guard **szerkezetes hibajelentést** ad vissza a tervezőnek: mi sikerült, melyik kapu bukott, az utolsó hibalista, a próbálkozások rövid listája, és egy gépies javaslat (pl. „a feladat két szimbólumot érint, bontsd ketté”, „a teszt hálózatot kér”).

## 12. A modell beállításai

- **Hőmérséklet 0, rögzített seed** (amennyire a futtató engedi), hogy ugyanarra a bemenetre ugyanazt adja.
- **Kötött kimenet:** az eszközhívások JSON-sémával (llama.cpp nyelvtan vagy az Ollama `format` mezője).
- **Rögzített, verziózott promptsablonok**; a sablon verziója a futásnaplóban szerepel (mint az utasításverzió a 35. kör óta).

## 13. Az eredmény

```yaml
status: done            # done | gave_up | rejected | already_done
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

## 16. Nyitott kérdések Norbinak (összesítve)

1. Egy feladat egy szimbólum, vagy engedünk többet? (3.)
2. A projektprofil minden futás előtt újra felismerve, vagy egyszer elmentve és jóváhagyva? (4.)
3. Hálózat a tesztek alatt: alapból tiltva, projektenként engedélyezhető? (5.)
4. A modell kérhet-e plusz kontextust (`show_symbol`), és mennyit? (6.)
5. A duplikációs ítélet felülbírálható-e emberileg vagy a tervező által? (7., 9.)
6. A javítási körök és időkorlátok (4 kör, 20 hívás, 15 perc) jók-e kiindulásnak? (2.)
7. A végső teljes futás csak a csomagra vagy modulra vonatkozzon, az egész repó a CI dolga? (9.)
8. Melyik nyelvekkel induljunk a hibaszűrőkben: Go és TypeScript először, aztán Python és Java? (4., 10.)
