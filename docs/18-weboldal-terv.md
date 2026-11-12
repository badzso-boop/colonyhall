# A Colonyhall weboldala: terv

A 48. kör 5. része (`site/`, GitHub Pages). Ez a dokumentum az 5.1-es pont: Norbi víziója és a rá épülő terv. A műszaki alap (az 5.2-es pont) már kész: a `site/build.mjs` ellenőrzi a kötelező szabályokat, a `.github/workflows/pages.yml` csak kézi indításra fut.

**Állapot:** a terv elfogadva (2026-11-12); a hangyavázlatok kapuja lezárult (12. szakasz); a 0–8. lépés megvalósítva (52. kör, `feature/site`). A Pages továbbra is csak kézi indításra fut.

## 1. Norbi döntései (2026-11-12)

- **Kinek szól:** elsősorban fejlesztőknek, akik maguk üzemeltetik. A fizetős ügyfél később jön, az első verzióban nincs neki külön rész vagy gomb.
- **Hangulat:** látványos, interaktív és kreatív. Nem alap landing page. Referencia nincs: a saját ötletünk legyen.
- **A fő motívum:** görgetés közben időnként hangyák másznak be, felettük az agent felhasználóneve (pl. `@assistant`). Végigmásznak a monitoron, és mennek tovább.
- **A hangyák stílusa:** kicsit rajzfilmes, karakteres.
- **Nyelv:** angol, i18n-nel, hogy később bármilyen nyelvre lefordítható legyen.
- **Domain:** egyelőre `badzso-boop.github.io/colonyhall`, saját domain később.
- **Terjedelem:** az első verzióba **minden** bekerül, ami ebben a dokumentumban szerepel (a 4. szakasz összes eleme).
- **Hang:** nincs.
- **Közzététel:** amíg a repó privát, csak helyben fejlesztjük; a Pages akkor indul, amikor a repó nyilvános lesz (`docs/17`, 0.9 / 9. lépés).

**További döntések (2026-11-12, második rész):**

- **A szöveg hangneme:** nem HR- vagy sales-szöveg. Kulturáltan, tárgyilagosan leírja, mire képes az alkalmazás, mi a vízió, és mi valósult meg eddig. Kimondja, hogy **a projekt aktív fejlesztés alatt áll**, és hogy ha valami nem működik jól, **a GitHub Issues-ban lehet jegyet nyitni**. A szövegeket nem Norbi írja: a tervezetük a 10. szakaszban és az A. függelékben van, a megvalósító ebből dolgozik.
- **Strata:** **minden kör** bekerül (az A. függelék), a kész és a tervezett körök is.
- **A hangyák kinézete:** a megvalósító **több vázlatot** rajzol (különböző karakterekkel), Norbi választ, és csak utána épül rájuk az oldal.
- **Nevek:** az agentek `@assistant`, `@reviewer`, `@scribe`, `@builder` és `@homelab`; a kitalált ember **Dori**.

## 2. Koncepció: leereszkedés a bolyba

A márka már meglévő karakterére épül (`docs/07-design-system.md`, 1. szakasz): **hangyaboly éjjel**, mély, meleg feketék, és egyetlen erős piros, a katonahangya. Új arculat nincs: az oldal ugyanazokat a tokeneket, betűtípusokat és a hangya jelet használja, mint a PWA.

A görgetés egy éjszakai hangyabolyba visz le. A teteje a felszín egy izzó monitorral, lefelé járatok vezetnek, és minden szekció egy **kamra**. A járatok görgetés közben rajzolódnak ki, és összekötik a kamrákat, így az oldal egy összefüggő utazás, nem egymás alá rakott blokkok sora.

A hangyák az agentek. Mindegyiknek neve van, és a nevük végig ugyanaz: aki a hero-ban üzenetet hordott, az a kapunál és a járattérképen is felbukkan. Így a látogató megérzi, amit a termék tud: **az agentek egy közös térben dolgoznak, és az ember mindent lát.**

A fő üzenet, amit a látogatónak magával kell vinnie: *„Humans see everything. Humans can stop anything.”* A legfontosabb interakció (a vészleállítás, 4.4) ezt teszi kézzelfoghatóvá.

## 3. Az oldal szerkezete

Egyetlen hosszú oldal (`/en/`), a kamrák sorrendjében. A gyökér (`/`) a nyelvre irányít (JS nélkül az angolra).

| # | Kamra | Tartalom | Fő elem |
|---|---|---|---|
| 0 | **Felszín (hero)** | név, egymondatos ígéret, telepítő parancs másolás gombbal, GitHub-link | az élő monitor (4.2) |
| 1 | **The hall** | mi a Colonyhall: csatornák, szálak, emberek és agentek egy helyen | a bemutató folytatása, feromon-nyomok (4.3) |
| 2 | **The gate** | az irányítás: jóváhagyás, delegálási engedélyek, Claude-limitek, leállítás | Approve / Stop, a boly megfagy (4.4) |
| 3 | **The tunnels** | az architektúra: szerver, őrök, agentek, sandbox | interaktív járattérkép (4.5) |
| 4 | **Workrooms** | funkciók: élő lépések, terminálok, fájlböngésző, tervező és naptár, keresés, PWA | kis kamrák, mindegyikben egy hangya végzi az adott munkát |
| 5 | **Your ground** | a saját gépeden fut: nincs telemetria, a bejelentkezések a tieid, csak oda kapcsolódik, amit beállítasz (`docs/17`, 0.8) | a boly fala, kívül semmi |
| 6 | **The nursery** | telepítés három lépésben, platformok, `colony doctor` | terminálkamra (4.6) |
| 7 | **Strata** | „Built by agents”: a fejlesztési körök rétegei | rétegek és kövületek (4.7) |
| 8 | **Open colony** | AGPL-3.0 + kereskedelmi licenc, CLA, közreműködés, `SECURITY.md` | |
| 9 | **Lábléc** | dokumentáció (`docs/en/`), API-referencia, GitHub, forráskód-link, licenc | alvó hangyák (4.8) |

A kamrák nevei munkacímek; a végleges angol szöveg a megvalósításkor készül, és az i18n-fájlba kerül.

## 4. Interakciók és mozgás

### 4.1 A névvel ellátott hangyák

- **Kinézet:** kicsit rajzfilmes, karakteres: a testhez képest nagyobb fej, nagy szemek, kifejező csápok, hat láb. Sötét test, piros részletek takarékosan (pl. a katonahangya feje, vagy amit éppen visz). A márkajelből (`BrandMark`) indul ki, hogy egy családba tartozzanak.
- **Rajzolás:** kódból, Canvas 2D-ben, nem képfájlból. A lábak lépésciklusa procedurális (váltott háromlábú járás, mint a valódi hangyáké), a test enyhén imbolyog, a csápok tapogatnak. Így bármennyi hangya rajzolható, és nincs képfájl-költség.
- **Névcímke:** a hangya felett lebeg és követi (`@assistant`, `@reviewer`, `@scribe` és társaik). Valódi DOM-szöveg, nem a canvasra festve, hogy olvasható maradjon és fordítható legyen.
- **Megjelenés:** görgetés közben, időzítve és véletlenszerűen egy-egy hangya bemászik a képernyő szélén, végigmegy egy úton (a monitoron, egy járat mentén, egy szekció címe mellett), majd eltűnik. Egyszerre legfeljebb néhány látszik, hogy ne legyen zsúfolt.
- **Személyiség:** minden agentnek saját járásmódja van (gyors és ideges, lassú és alapos, folyton megáll szaglászni). Egy-egy apró mozdulat jelzi a szerepét: a `@scribe` jegyzetlapot cipel, a `@reviewer` meg-megáll és „megnéz” valamit.
- **Egér:** a hangyák kikerülik a kurzort; ha a kurzor megáll, egy kíváncsi hangya odamegy és megszaglássza.
- **Kattintás:** egy kis „feladatmorzsát” ejt le, és a legközelebbi hangya odamegy, felveszi és elviszi a legközelebbi járatba. Mobilon ugyanez koppintásra.

### 4.2 Az élő monitor (hero)

- A felszínen egy izzó monitor áll, rajta egy kicsinyített, de valódi PWA-kinézetű chat: oldalsáv csatornákkal, üzenetfolyam, egy agent élő lépései.
- **Előre megírt forgatókönyv** fut rajta (kitalált ember és kitalált agentek, 5. szakasz). Pl. egy ember `@assistant`-tól kér egy összefoglalót, az átadja a `@reviewer`-nek, a lépések élőben jönnek.
- Az üzeneteket a hangyák **hordják be**: egy hangya kis piros csomaggal felmászik a monitorra, és amikor a csomag a képernyőre ér, abból lesz az új üzenet.
- Ha egy agent dolgozik, a hangyája bemegy a monitor alatti munkakamrába, és onnan érkeznek a lépései (`reading…`, `running tests…`).
- A forgatókönyv körbe jár; ha a látogató görget, a monitor megáll, és visszatérve onnan folytatja.

### 4.3 Feromon-nyomok: a delegálás

- Amikor egy agent feladatot ad egy másiknak, halványan izzó piros nyom húzódik a két hangya (és a két üzenet) között, ami lassan elhalványul.
- Ez a piros egyetlen „harsány” felhasználása a kamrákban, így megtartja a súlyát.

### 4.4 A kapu: jóváhagyás és vészleállítás

- Egy hangya egy feladattal megáll egy piros kapunál, mellette a PWA jóváhagyó kártyája (kitalált tartalom: „`@builder` wants to run `make deploy` on `demo-server`”).
- **Approve:** a kapu kinyílik, a hangya továbbmegy, a kártyán a „approved by you” felirat jelenik meg.
- **Stop:** **az egész oldal bolya megfagy**: minden hangya megáll, a nyomok kihunynak, a monitoron a futó lépés „stopped” lesz, és megjelenik a fő üzenet. Egy „Resume colony” gomb újraindítja őket.
- Billentyűzettel teljesen kezelhető; a kártya gombjai valódi `<button>`-ok.

### 4.5 A járattérkép: az architektúra

- Föld alatti térkép SVG-ben. Középen a **csarnok** (a szerver: web, API, Postgres). Körülötte az **előőrsök** a gépeken (pl. `laptop`, `homelab`, `gpu-box`), mindegyikben egy őr (`colony`). A **járatok** a kapcsolatok.
- Görgetésre a járatok kirajzolódnak, majd hangyák indulnak rajtuk: üzenet a csarnoktól az előőrsig, válasz vissza.
- A **sandbox** egy lezárt falú kamra az előőrsben: a hangya bent dolgozik, nekimegy a falnak, és visszafordul.
- Rámutatásra (és fókuszra) minden elem egy rövid szövegben elmondja a szerepét.

### 4.6 A terminálkamra: telepítés

- Egy terminál (mindig sötét, mint a PWA-ban) begépeli a telepítést: `curl … install.sh`, `sh install.sh`, majd a `colony setup` kérdései és válaszai (kitalált gépnévvel).
- Amikor a telepítés kész, a terminál körül kikel egy új kis kolónia: néhány kis hangya kibújik, és elindul az oldal teteje felé.
- Mellette a valódi parancsok másolás gombbal, és a platformlista (Linux, WSL2, macOS).

### 4.7 Strata: „Built by agents”

- Az oldal alja a talaj rétegeiből áll. Minden réteg egy fejlesztési kör, benne egy „kövület”: a kör száma, neve és egymondatos leírása.
- A rétegek alatt számlálók görgetésre számolnak fel: körök, commitok, agentek, sorok.
- **Minden kör** bekerül, a 0-stól (az alapok) az utolsóig. A kész körök a mélyebb, tömör rétegek; a folyamatban lévő kör a „most ásott” réteg (animált, morzsálódó szél); a tervezett körök halvány, szaggatott körvonalú rétegek a legalján, „planned” címkével. Így a rétegek maguk mutatják, hogy a projekt aktív fejlesztés alatt áll.
- **A szöveg** az A. függelékből jön (szám, név, egy mondat, állapot). Csak ez kerül ki, semmi a privát munkajegyzetből (címek, gépek, személyes adatok). Az adat egy JSON-fájl a `site/`-ban, a mondatok az i18n-fájlban.
- **Számlálók:** körök, commitok, migrációk, tesztek. Egy buildhez kötött script számolja őket a `git` előzményből és a repóból, nem élőben; a script kimenete be van commitolva, hogy a build hálózat és teljes előzmény nélkül is fusson.

### 4.8 Apró meglepetések

- **Név begépelése:** ha a látogató (nem beviteli mezőben) begépel egy agentnevet, pl. `@scribe`, előjön a hangyája és integet.
- **Alvó hangyák a láblécben:** összegömbölyödve alszanak (kis „z”), és felébrednek, ha föléjük viszed az egeret vagy rájuk koppintasz.
- **Napszak:** sötét témában éjszakai felszín (csillagok, izzó monitor), világosban hajnal (halvány égbolt, harmat). A téma a `prefers-color-scheme`-et követi, és van kézi váltó.

### 4.9 Mozgás általában

- A szekciók belépése, a járatok kirajzolása és a számlálók görgetéshez kötöttek (CSS scroll-driven animációk, ahol a böngésző tudja; máshol `IntersectionObserver`).
- Csak `transform` és `opacity` animál, layout nem.
- **`prefers-reduced-motion`:** a hangyák nem másznak, hanem a helyükön állnak (címkével), a járatok azonnal készen vannak, a monitor és a terminál a forgatókönyv végállapotát mutatja, a számlálók a végső számot. Minden információ ugyanúgy elérhető, és a Stop gomb is működik (állapotot vált, animáció nélkül).

## 5. A termék bemutatása valódi adat nélkül

- Minden, ami a terméket mutatja, **kitalált**: egy kitalált ember (**Dori**), kitalált agentek (`@assistant`, `@reviewer`, `@scribe`, `@builder`, `@homelab`), kitalált csatornák (`#general`, `#release`), kitalált gépek (`laptop`, `homelab`, `gpu-box`), kitalált repó és fájlnevek. A `@homelab` agent a `homelab` gépen fut, és az ottani szolgáltatásokra figyel (pl. ő jelzi, hogy kész a mentés).
- **Nincs képernyőkép** Norbi munkaterületéről. A monitor és a jóváhagyó kártya nem kép, hanem HTML és CSS a PWA tokenjeivel és kinézetével, így éles és fordítható.
- A forgatókönyvek (hero, kapu, terminál) egy-egy JSON-fájlban vannak, a szövegük az i18n-fájlokból jön.
- **A közösségi előnézeti kép** (`og:image`, GitHub social preview) a kész oldal hero-járól készül, Playwrighttal, a buildhez tartozó scripttel, hogy újragenerálható legyen. Erre a célra nem kell `make demo-seed`, mert az oldal nem a valódi appot fényképezi; a `make demo-seed` akkor kell, ha később valódi app-képernyőképek vagy videó kerül az oldalra.

## 6. Technika és miért

- **Keretrendszer nélkül**, a meglévő, függőség nélküli `site/build.mjs`-re építve. Az oldal egyetlen, gazdag oldal, nem dokumentációs portál; egy keretrendszer itt csak méretet és függőséget hozna. Ha később sok oldal lesz (pl. a dokumentáció a site-on), az Astro szóba jöhet.
- **HTML + CSS** a tartalomnak és az elrendezésnek. A tokenek a PWA-ból jönnek: a `web/src/shared/styles/tokens.css` színértékeit a build átmásolja, hogy egy helyen változzanak.
- **Canvas 2D** a hangyáknak (sok mozgó elem, olcsó), egy közös `requestAnimationFrame`-ciklussal. A névcímkék DOM-elemek.
- **SVG** a járatoknak, a térképnek, a kapunak, a rétegeknek és a JS nélküli statikus illusztrációknak.
- **Saját, kis JS-modulok** (ES modules, build nélkül vagy egyszerű összefűzéssel): `ants` (rajzolás és mozgás), `colony` (melyik hangya mikor jön, közös állapot, Stop/Resume), `scripts` (a forgatókönyvek lejátszása), `i18n`. Külső könyvtár nincs.
- **i18n:** minden szöveg nyelvenként egy JSON-fájlban (`site/src/i18n/en.json`), a forgatókönyvek és az agentnevek feliratai is. A build nyelvenként külön, előre kitöltött HTML-t készít (`/en/`, később pl. `/hu/`), így a szöveg JS nélkül is ott van, és a keresők is látják. A JS csak a forgatókönyvek szövegét veszi ugyanabból a fájlból. Új nyelv = egy új JSON-fájl.
- **Betűtípus:** Inter és JetBrains Mono, ahogy a PWA-ban, saját hosztolva, latin részhalmazra szűkítve (`woff2`), hogy beférjen a keretbe.
- **GitHub Pages:** a cím alútvonal (`/colonyhall/`), ezért minden hivatkozás relatív; a saját domainre váltás így csak egy `CNAME` fájl.

## 7. Elérhetőség, JS nélkül

- **JS nélkül** minden szöveg olvasható, a kamrák statikus SVG-illusztrációt mutatnak, a monitoron és a terminálban a forgatókönyv statikus átirata látszik. A Stop és a meglepetések JS nélkül nincsenek, de semmi információ nem vész el.
- **A dekoráció nem zavar:** a canvas `aria-hidden`, a hangyák névcímkéi a képernyőolvasónak nem hangzanak el egyenként. Egy rövid szöveg leírja, mi látszik („Animated ants, each one an agent, carry messages across the page.”).
- **Billentyűzet:** minden interaktív elem elérhető és látható fókuszt kap (`focus-ring`); a hangyák nem kapnak fókuszt.
- **Kontraszt:** a PWA tokenjei, a `contrast.test.ts` szabályaival (szöveg ≥ 4,5:1, ikon és keret ≥ 3:1), mindkét témában.
- **Szüneteltetés:** van egy állandó gomb, ami megállítja az összes mozgást (WCAG 2.2.2), és az oldal emlékszik rá a lapon belül (süti és tárolás nélkül, csak a munkamenetben).

## 8. Teljesítménykeret

- **Az egész oldal: 500 kB** tömörítés előtt (a `build.mjs` már ellenőrzi). Becslés: HTML + CSS ~60 kB, JS ~80 kB, SVG-k ~80 kB, két betűtípus részhalmaza ~120 kB, előnézeti kép ~100 kB. Ha a betűtípus nem fér bele, előbb a JetBrains Monót cseréljük rendszerfontra a terminálban, nem a keretet emeljük.
- **Lighthouse CI** a Pages-workflowban (mobil profil): Performance ≥ 90, Accessibility = 100, Best Practices ≥ 95, SEO ≥ 95; LCP < 2,5 s, CLS < 0,05, TBT < 200 ms.
- **Futásidő:** a hangyák egy ciklusban mozognak, ami **megáll**, ha nincs hangya a képernyőn, ha a fül a háttérben van (`visibilitychange`), vagy ha a látogató szüneteltetett. Egyszerre legfeljebb ~12 hangya (mobilon ~5). A canvas a `devicePixelRatio`-hoz igazodik, de legfeljebb 2×. A JS `defer`-rel töltődik, és nem blokkolja a szöveg megjelenését.
- **Mobil:** kevesebb hangya, érintésre reagálnak, a járattérkép függőleges elrendezést kap, és nincs vízszintes görgetés.

## 9. Kötelező szabályok (az 5.2-ből, változatlanul)

- nincs követés, süti, futásidőben betöltött külső script vagy betűtípus;
- a fő szöveg JS nélkül is működik;
- `prefers-reduced-motion` és `prefers-color-scheme` tiszteletben tartva;
- WCAG AA, billentyűzet, alt szövegek;
- gyors, mérve a CI-ban;
- angol elsőként.

## 10. A szövegek

**Hangnem:** tárgyilagos, rövid mondatok, fejlesztőnek szólva. Nincs „revolutionary”, „seamless”, „supercharge”, nincs felkiáltójel, nincs kitalált ügyfélvélemény vagy szám. Ami még nem kész, az „planned”, nem ígéret. A szöveg angol, az i18n-kulcsok alatt; az alábbi a tervezet, a megvalósító finomíthatja a hosszát az elrendezéshez, de a tartalmát nem.

| Hol | Szöveg (tervezet) |
|---|---|
| Hero, cím | **Colonyhall** |
| Hero, alcím | A self-hosted chat for people and AI agents. |
| Hero, bekezdés | Your agents (Claude Code, Antigravity and others) run on your own machines. In Colonyhall they talk, ask questions and hand tasks to each other in shared channels and threads. You see every step, and you can stop any of them. |
| Hero, gombok | `Install` · `View on GitHub` |
| The hall | Channels, threads and direct messages, the way a team chat works, except that half the members are agents. Mention an agent and the guard on its machine wakes it with the message. Its answer, and every step on the way, arrives in the same thread. |
| The gate | Agents work for people, not on their own authority. An agent's task for a full-access agent waits for a human's approval, and one approval covers that conversation for a while. Usage limits, budgets and an emergency stop are always one click away. *Humans see everything. Humans can stop anything.* |
| The tunnels | One server (the web app, the API and Postgres in Docker Compose) and one guard per machine. The guard runs the AI clients you already use, logged in by you, and reports to the server over its public API. Where it can, it runs agents in a sandbox with their own worktree and a network allowlist. |
| Workrooms | Live steps of every run. Terminals for long commands. A file browser with git status. A planner and calendars, read-only for agents. Search across channels, threads and the wiki. A web app you can install on your phone. |
| Your ground | Colonyhall has no telemetry, no update check and no error reporting. It connects only to what you configure: your AI providers, your git server, the calendars you subscribe to. It never logs in to an AI account for you. |
| The nursery | One installer for the server and the guards, on Linux, WSL2 and macOS. It shows its plan before it changes anything (`--dry-run`), can be run again to repair a setup, and `colony doctor` tells you what is wrong and how to fix it. |
| Strata, bevezető | Colonyhall is built round by round, mostly by AI coding agents, with a human reviewing every round. Each layer below is one round. |
| **Állapot** (a Strata alatt) | **Under active development.** Colonyhall runs every day on its author's machines, but it is young: things change between versions, and some things will break. If something does not work the way it should, [open an issue on GitHub](…/issues/new/choose). Questions and ideas are welcome there too. |
| **Vízió** (az Állapot után) | The goal is a place where a small team and its agents work together like colleagues: agents that can ask, hand off and report, on hardware you own, under rules you set, with a human able to see and stop everything. Next on the list: push notifications, a local coding agent on small models, and GPU-backed search. |
| Open colony | Colonyhall is free software under the AGPL-3.0. You can use it, self-host it and change it. If you offer a modified version as a network service, publish its source, or ask about a commercial licence. Contributions need a one-time CLA signature, so that they can be offered under both licences. Security issues: see `SECURITY.md`, not the public issue tracker. |
| Lábléc | Docs · API reference · GitHub · Issues · Source · License (AGPL-3.0-only) |

Az issue-link a repó issue-űrlapjainak választójára mutat (`/issues/new/choose`, a 48. kör 3. részéből), a cím egy helyen, a build konfigurációjában van, hogy a repó áthelyezésekor egy helyen kelljen módosítani.

## 11. Megvalósítás

Javaslat a sorrendre, egy körben, de lépésenként commitolva:

0. **Hangyavázlatok (kapu):** egy külön vázlatoldal (`site/sketches/`, nem kerül a buildbe) **legalább négy különböző hangyakarakterrel** (eltérő arányok, szemek, csápok, színhasználat), mindegyik járás közben, névcímkével, sötét és világos háttéren. **Itt meg kell állni**, Norbi választ (vagy kombinál), és csak utána jön a többi lépés.
1. Alap: i18n-build, tokenek átvétele, a kamrák statikus HTML-je és SVG-i (így már JS nélkül kész az oldal), Lighthouse CI.
2. A hangyamotor: rajzolás, járás, névcímke, utak, egér és kattintás, szüneteltetés, a csökkentett mozgás.
3. Az élő monitor és a feromon-nyomok.
4. A kapu (Approve / Stop, a boly megfagy).
5. A járattérkép és a munkakamrák.
6. A terminálkamra és a kikelő kolónia.
7. Strata (az A. függelék összes köre) és a számlálók.
8. Meglepetések, napszakok, előnézeti kép, végső mérés.

Minden lépés után: `npm --prefix site run build` és a Lighthouse-mérés helyben; a Pages-workflow továbbra is csak kézi indításra fut.

## 12. A hangyák kinézete (döntés)

- **Norbi választása (2026-11-12): a „C” vázlat („Mark”).** A vázlatoldal (`site/sketches/`) öt karaktert mutatott (A–E); Norbi a C-t választotta keverés nélkül.
- **A karakter:** felülnézetes hangya, a `BrandMark` arányaival (kerek fej, kis tor, hosszú potroh), nagy fehér szemekkel, pislogással és tapogató csápokkal. **Az egész hangya piros**, mint a logó; a cipelt csomag fehér, piros kerettel, hogy ne olvadjon bele a fejbe.
- **Következmény a piros használatára:** a hangyák maguk a logó, ezért a piros rajtuk a márkajel szerepét tölti be, nem „harsány” kiemelésként számít. A feromon-nyomok (4.3) továbbra is a piros egyetlen kiemelő használata a kamrákban; a többi felület (szöveg, kártyák, keretek) a tokenek szerint visszafogott marad. A hangyák száma (legfeljebb ~12, mobilon ~5) és ritkasága tartja meg a piros súlyát.
- **Felülnézet:** a hangya bármilyen irányba fordulhat, így ugyanaz a rajz mászik a monitoron, a járatokban és a térképen.

## 13. Karbantartás (az 52. kör után)

**Felépítés.** `site/src/page.html` a sablon (`{{t:kulcs}}` szöveg, `{{md:kulcs}}` szöveg kis jelöléssel: `` `kód` ``, `**erős**`, `*dőlt*`, `[szöveg](link:név)`; `{{link:név}}` cím; `{{svg:név}}` a `src/svg/` fájljai). A címek egy helyen vannak: `site/site.config.json` (`repository`, `siteUrl`), a belőle képzett linkek (`issues`, `docs`, `api`, `license`, …) a `site/build.mjs` elején. A színeket a build a PWA `tokens.css`-éből másolja (`dist/css/tokens.css`). A scriptek (`src/js/`) ES-modulok, külső könyvtár nélkül; a szövegeiket a build a lapba ágyazza (`<script id="site-data">`), így nincs külön letöltés.

**Új nyelv.** Egy új fájl: `site/src/i18n/<nyelv>.json`, az `en.json` szerkezetével (a hiányzó kulcsok angolul jelennek meg; a build hibát ad, ha egy kulcs nincs az `en.json`-ban). A build magától elkészíti a `/<nyelv>/` oldalt és a `hreflang` hivatkozásokat. Más teendő nincs.

**A Strata frissítése, ha egy kör állapota változik.** Három hely: ennek a dokumentumnak az A. függeléke, a `site/src/data/strata.json` (`state`: `shipped`, `digging` vagy `planned`), és új kör esetén az `en.json` `strata.rounds.r<szám>` kulcsa (`name`, `line`). A sorrendet és a megjelenést a build adja (a folyamatban lévő kör felül, a kész körök alatta, a tervezettek szaggatottan a legalján).

**Számlálók.** `npm --prefix site run counters` (a `main` ágon, a kör beolvasztása után): a `git` előzményből és a repóból számol (kész körök, commitok, migrációk, Go-tesztfüggvények + Vitest/Node-tesztesetek), és a `site/src/data/counters.json`-t írja. A kimenet be van commitolva, a build hálózat és teljes előzmény nélkül fut.

**Közösségi előnézeti kép.** `npm --prefix site install --no-save playwright@1.56.1`, egyszer `npx --prefix site playwright install chromium`, majd `npm --prefix site run build && npm --prefix site run social && npm --prefix site run build`. A kép (`site/src/img/social.jpg`, 1200×630) a hero-ról készül, miután a monitoron megtörtént az átadás. A GitHub social preview-ba ugyanezt a képet kell feltölteni (Settings → General).

**Ellenőrzés.** `npm --prefix site run build` (szabályok, kulcsok, méretkeret), a Pages-workflow Lighthouse CI-ja (mobil profil, a 8. szakasz küszöbeivel).

## A. függelék: a rétegek (Strata) szövege

Minden kör egy réteg. Állapot: **shipped** (kész és beolvasztva), **digging** (folyamatban), **planned** (tervezve). Ha egy kör állapota változik, ezt a táblát és a `site/` adatfájlját is frissíteni kell.

| # | Név | Mondat | Állapot |
|---|---|---|---|
| 0 | Foundation | The server, the guard, channels and direct messages, and an emergency stop that halts every agent at once. | shipped |
| 1 | Direct mode | An agent answers a message in its own CLI session, as if you had typed to it in a terminal. | shipped |
| 2 | Direct mode fixes | Fixes from the first review of direct mode. | shipped |
| 3 | Instant wake-up | The guard wakes an agent over a WebSocket the moment a message arrives, instead of polling. | shipped |
| 4 | Run log | Every run is recorded on the server, with its steps and token usage. | shipped |
| 5 | Approvals | Risky actions wait for a human's yes or no in the chat. | shipped |
| 6 | Team wiki | A wiki that people and agents both read and write. | shipped |
| 7 | Everyday use | The guard runs as a service, agents are created from a form, and secrets move through an exchange folder, never through the chat. | shipped |
| 8 | Cost limits | Daily token budgets and session rotation keep long conversations affordable. | shipped |
| 9 | Questions | An agent can ask a person or another agent and wait for the answer, with escalation when nobody replies. | shipped |
| 10 | Files | Files can be shared in the chat. | shipped |
| 11 | Git links | Commits, pull requests and issues show up as cards, and Gitea events arrive by webhook. | shipped |
| 12 | Worktrees | Each full-access agent works in its own git worktree, so agents do not step on each other. | shipped |
| 13 | Previews | An agent can run the app it built and share it by link. | shipped |
| 14 | Collaboration | Shared agent rules, a notebook, recorded decisions, leases on shared resources, and a rate limit between agents. | shipped |
| 15 | Sidebar and profiles | Channel sections, channels with members, profiles, a "working…" indicator and an agent panel. | shipped |
| 16 | File links | File paths in an agent's message open in the browser, fetched read-only by the guard. | shipped |
| 17 | Markdown | One safe markdown renderer for chat messages and the file viewer. | shipped |
| 18 | Sandbox isolation | Sandboxed agents get their own worktree, and their network goes through an allowlist. | shipped |
| 19 | Web reading | Sandboxed agents read public web pages through the guard, with GET requests only. | shipped |
| 20 | File browser | Browse an agent's folder: tree, search, git status, a code viewer and line references. | shipped |
| 21 | Compaction | When a conversation grows too long, a scribe agent summarises it and the others continue from the summary. | shipped |
| 22 | Live progress | You see what an agent is doing, step by step, while it works. | shipped |
| 23 | Agent terminals | Long commands outlive a run, wake the agent when they finish, and can be watched live. | shipped |
| 24 | Delegation approval | A task from an agent to a full-access agent waits for a human's approval. | shipped |
| 25 | Delegation grants | One approval covers the whole conversation between two agents for a while. | shipped |
| 26 | Daily-use fixes | Fixes from a day of real use: mention autocomplete, replies, retracting a message, run timeouts. | shipped |
| 27 | Reply routing | Answers find their way back to the thread that asked, and the guard pauses before a Claude usage limit runs out. | shipped |
| 28 | Planner | A personal task list and subscribed calendars, read-only for agents, with a morning summary. | shipped |
| 29 | Planner in the sandbox | Sandboxed agents read the planner through the guard, without ever holding a token. | shipped |
| 30 | Dates and context | Dates and day separators in the chat, wake-up right after an approval, and context size on the agent card. | shipped |
| 31 | Web app foundation | The new web app: a design system, the logo, the app shell and the panel layout. | shipped |
| 32 | Web app pages | Floating windows, tasks, calendar, notes, schedules, the wiki, leases and agent rules. | shipped |
| 33 | Agents and machines | An agent directory, immutable agents with a detailed creation dialog, and a page for every machine. | shipped |
| 34 | Complete chat | Channel editing, muting, a threads view, question, approval and compaction cards, and profiles. | shipped |
| 35 | Admin | Runs and usage with estimated cost, users, tokens, the audit log, system status, the emergency stop and channel locks. | shipped |
| 36 | Search | Hybrid full-text and vector search over channels, threads, the wiki and decisions, with access checked on every query. Code and document search are planned. | shipped |
| 37 | Quota pauses | The guard recognises an Antigravity quota error and pauses that model until it resets. | shipped |
| 38 | Colleagues | A colleague can bring their own agents and their own Claude subscription, with owners and visibility. | shipped |
| 39 | Code graph | Every agent gets a code graph of its repository, wired in by the guard. | shipped |
| 40 | Security hardening | A systematic hardening pass. Security headers, a content security policy and request deadlines are already in. | planned |
| 41 | Installer | One installer for the server and the guards: rerunnable, with a dry run, and `colony doctor`. | shipped |
| 42 | Continuous integration | Every check runs on GitHub for every push, in parallel; `make ci` runs the same checks locally. | shipped |
| 43 | Push and alerts | Push notifications to your phone, and alerts when something needs you. | planned |
| 44 | Archive | Old messages move to long-term storage. | planned |
| 45 | Tenant isolation | Row-level security on every table, so several teams can share one server safely. Operator tools are planned. | shipped |
| 46 | Data protection | Anonymising a person and exporting their data. | planned |
| 47 | Local agent | A coding agent on small local models, kept on track by a deterministic harness. | planned |
| 48 | Open source | The AGPL licence, the CLA, community files, English docs, and this site. | digging |
| 49 | GPU machine | Local model runtimes, a GPU relay through the guard, and a VRAM scheduler. | planned |
| 50 | Token cleanup | Expired tokens are cleaned up, and admins see the database size. | shipped |
