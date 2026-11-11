# Kézi tesztlista – PWA (31–33. kör)

Fix lista a PWA (`/app/`) kézi végigpróbálásához (31–33. kör és a 2026-11-01-i javítások). Minden kiadás után ugyanígy, ugyanebben a sorrendben megyünk végig rajta, hogy a futások összevethetők legyenek. Az automatikus tesztek (`make test`, `make test-web`) mellett ez az a kör, ahol a működést a saját szemünkkel látjuk.

**Ha új funkció készül, a köre végén ide is bekerülnek a pontjai.** Ha egy hiba miatt javítás készült, a hibát kiváltó lépés is legyen itt.

## Hogyan használd

- **Azonosítók:** minden ellenőrzésnek van azonosítója (pl. `K-03`). Hibánál erre hivatkozz („K-03 bukik: …”), így a javítás és az újratesztelés ugyanarra mutat.
- **Futtatás:** a jelölőnégyzetes listát másold át a fájl végén lévő „Futások” rész alá egy új bejegyzésbe (dátum, verzió), és ott pipáld. Maga a lista ne változzon futás közben.
- **Jelölések:**
  - **[őr]**: élő őr kell hozzá (terminál, fájlböngésző, mappaválasztó, valódi agentmunka). A fejlesztői stacken nincs őr, ezeket az éles szerveren, egy **teszt-agenttel** próbáld.
  - **[⚠]**: adatot hoz létre, töröl vagy leállít valamit. Élesben csak `teszt-` kezdetű agenteken, csatornákon és oldalakon; a vészleállítót csak a fejlesztői stacken.
  - **[📱]**: telefonon (vagy 390 px széles nézetben) is nézd meg.
- **Méretek:** asztali 1440×900, közepes 1024×768, telefon 390×844 (a böngésző eszköz-nézete is jó, de a telefonos pontokat legalább egyszer valódi telefonon, a telepített PWA-ban is).
- **Témák:** minden fő képernyőt nézz meg világos és sötét témában. Elég, ha a fő képernyőt mindkettőben megnézed; a részletek egy témában.
- **Kinek a szemével:** a legtöbb pont adminként (`@norbi`). A „Jogosultságok” rész egy **tag** (member) fiókkal és egy **agent** tokenjével megy.

## Előkészítés

| Mi | Fejlesztői stacken | Élesben |
| --- | --- | --- |
| Szerver | `make dev` → `http://localhost:8080/app/` (belépés: `norbi` / `colonyhall-dev-password`) | `http://100.64.0.10:8280/app/` |
| Tag fiók | „Gépek” helyett: `POST /api/v1/actors` `{"kind":"human","handle":"teszt-tag","role":"member"}`, jelszó `set-password` paranccsal | egyszer létrehozva, újrahasznosítva |
| Teszt-agent | `@fake-agent` (seed) | `/app/agents` → „Új agent”: `teszt-sandbox` (sandbox, a homelabon) és `teszt-full` (teljes hozzáférés, a PC-n, egy teszt-mappában) |
| Csatornák | `#general` | `#teszt` (te + a teszt-agentek) |
| Böngésző | friss lap, Ctrl+F5 | ugyanígy; előtte a szerver és az őrök frissítve |

---

## 1. Belépés, PWA, téma, nyelv (31. kör)

- [ ] **K-01** Kijelentkezve az `/app/` a belépő oldalra visz; rossz jelszóra érthető hibaüzenet jön, jóra a kezdőképernyő.
- [ ] **K-02** A felhasználói menüben a téma (világos / sötét / rendszer) azonnal vált, újratöltés után megmarad.
- [ ] **K-03** A nyelv (magyar / angol) vált, minden felirat lefordul (nincs nyers kulcs, pl. `create.title`), újratöltés után megmarad.
- [ ] **K-04** [📱] A telefonon „Hozzáadás a kezdőképernyőhöz” után a PWA saját ikonnal, önálló ablakban indul.
- [ ] **K-05** Új verzió kirakása után „Új verzió érhető el” értesítés jön „Frissítés” gombbal; magától nem tölt újra, a gombra igen.
- [ ] **K-06** A `/app/design` stílusoldal betölt, a témaváltó működik, a komponensek mindkét témában olvashatók.

## 2. A keret: fejléc, oldalsáv (31. kör)

- [ ] **K-07** Fejléc: oldalsáv-kapcsoló, logó, morzsasor (a nyitott oldal/csatorna neve), keresőmező (megnyitja a palettát), „Válaszra vár” számláló, „⚡ N dolgozik”.
- [ ] **K-08** Adminnak a Claude-keret gyűrűje látszik, ha egy gép jelentette (borostyán 80 % fölött); súgóban a részletek.
- [ ] **K-09** [⚠] *(csak fejlesztői stack)* A „Mind áll” gombot 2 másodpercig nyomva tartva bekapcsol a vészleállítás: az egész fejléc piros, „Feloldás” gomb jelenik meg; rövid kattintás nem csinál semmit. A „Feloldás” visszaállítja a normál állapotot.
- [ ] **K-10** Oldalsáv sorrendje: Munkaterület oldalai → saját szekciók → Csatornák → Közvetlen üzenetek → AI-beszélgetések („Te és az agentek”, „Agentek egymás közt”). Egy agent–agent DM az utóbbiban, két átfedő avatarral és `@a ↔ @b` címmel.
- [ ] **K-11** Olvasatlan beszélgetés félkövér, megemlítésnél piros számláló; az aktív sor piros bal csíkkal kiemelt.
- [ ] **K-12** A böngészőfül címében a megemlítések száma, a faviconon piros pötty, ha van olvasatlan megemlítés.
- [ ] **K-13** A szerver leállításakor (vagy a hálózat bontásakor) kb. 3 másodperc után „Újracsatlakozás…” sáv jelenik meg, visszaálláskor eltűnik, és a kimaradt üzenetek megjönnek.
- [ ] **K-14** [📱] Telefonon alsó sáv (Beszélgetések, Válaszra vár, Agentek, Több), a kezdőképernyő a beszélgetéslista.

## 3. Hírfolyam és szerkesztő (31. kör)

- [ ] **K-15** Egy hosszú csatornában felfelé görgetve betöltődnek a régebbi üzenetek (ugrálás nélkül). *(Javított hiba: csatornában nem töltöttek be a régi üzenetek, 500-as hiba.)*
- [ ] **K-16** Új üzenet érkezésekor, ha nem vagy alul, „↓ N új üzenet” gomb jelenik meg; visszatéréskor „Új üzenetek” elválasztó jelzi, hol tartottál; alul olvasottnak jelölődik.
- [ ] **K-17** Markdown: félkövér, dőlt, kód, kódblokk (másolás gombbal), lista, link; `snake_case` nem lesz dőlt; HTML nyers szövegként jelenik meg.
- [ ] **K-18** Hosszú üzenet összecsukva, „Több” / „Kevesebb”; kép kattintásra nagyítóban nyílik; fájl-chip letölthető.
- [ ] **K-19** A szerkesztőben `@` után ajánlólista (a csatorna tagjai elöl), nyilakkal és Enterrel választható; `/` után a parancsok listája.
- [ ] **K-20** Küldés **sima HTTP-n is** működik csatornában, megemlítéssel és DM-ben. *(Javított hiba: „crypto.randomUUID is not a function”.)*
- [ ] **K-21** Küldés után „Elküldve” értesítés „Visszavonás” gombbal; 10 másodpercen belül visszavonható, az üzenet eltűnik.
- [ ] **K-22** `/schedule 2026-12-01 09:00 szöveg`, `/daily 09:00 szöveg` és `/cron …` létrehozza az ütemezést (értesítés a következő futással); hibás formánál súgó jelenik meg.
- [ ] **K-23** Csatolás: „+” → Fájl feltöltése; húzással; képbeillesztéssel (Ctrl+V). A tálcán folyamatjelző, hiba esetén „Újra” gomb, túl nagy fájlnál érthető hiba.
- [ ] **K-24** [📱] Telefonon a „+” menüben „Fotó” (kamera) is van; Enter új sort ad, a küldés gomb küld.
- [ ] **K-25** [őr] Amíg egy agent dolgozik, „@agent dolgozik…” sor látszik az élő lépéssel (📖, ✏️, ⚙️…) és az eltelt idővel; a válasz után eltűnik.

## 4. Szálak és panelek (31. kör)

- [ ] **K-26** Egy üzenet „Válasz szálban” gombja (vagy kijelölve `r`) jobbra megnyitja a szálat saját szerkesztővel; a fő üzenet alatt összesítő: válaszszám, résztvevők, utolsó válasz ideje.
- [ ] **K-27** Az URL tartalmazza a nyitott panelt; a böngésző „Vissza” gombja bezárja a panelt, nem lép el az oldalról.
- [ ] **K-28** **Széles (≥ 1280 px):** a szál jobbra kerül. Ha a szál nyitva van, és megnyitod a terminált vagy a fájlokat, a munkapanel **balra**, az oldalsáv helyére kerül, a szál marad; az oldalsáv fiókként nyílik.
- [ ] **K-29** **Közepes (1024 px):** egyszerre egy jobb oldali panel; a rejtett panelre váltósáv vált vissza.
- [ ] **K-30** [📱] **Telefon:** beszélgetés → szál → munkapanel egymás utáni képernyők, „Vissza” gombbal.
- [ ] **K-31** Esc bezárja a legfelső panelt.

## 5. Terminálpanel (31. kör)

- [ ] **K-32** [őr] Egy agent DM-jében a „Terminál” gomb (vagy Ctrl Shift X) megnyitja a terminálok listáját állapotjelvényekkel (fut, kész, hibás, leállítva).
- [ ] **K-33** [őr] Egy futó terminál kimenete 2 másodpercenként frissül és alulra görget; a kimenet és az SSH-sor másolható.
- [ ] **K-34** [őr] [⚠] Adminként a „Leállítás” megerősítés után leállítja a terminált; az állapot „leállítva” lesz.

## 6. Fájlböngésző (31. kör)

- [ ] **K-35** [őr] A „Fájlok” gomb (vagy Ctrl Shift F) megnyitja az agent mappáját: lustán nyíló fa, git-jelvények (M, A, D, ?, R), ág és ahead/behind, keresés.
- [ ] **K-36** [őr] Kódfájl a Monaco-nézőben, a téma követi az oldalét; markdown formázva és „Nyers” nézetben; kép előnézettel; 2 MB fölött eleje/vége nézet.
- [ ] **K-37** [őr] Sorokat kijelölve „Hivatkozás beszúrása” beírja a szerkesztőbe a `fájl:10-20` hivatkozást, a „Link másolása” vágólapra teszi; a chatben a hivatkozás chip, kattintásra a megfelelő sorra ugrik.
- [ ] **K-38** [őr] A `.git` és a titkos fájlok (pl. `.env`) nem látszanak; a `node_modules` összecsukva.

## 7. Billentyűparancsok és parancspaletta (31–33. kör)

- [ ] **K-39** `?` megnyitja a súgót; minden parancs fel van sorolva, szűrhető. Gépelés közben a `?` nem nyitja meg.
- [ ] **K-40** Ctrl K megnyitja a palettát: beszélgetések, agentek és oldalak kereshetők (pl. „wiki”, „@backend”), Enter oda visz.
- [ ] **K-41** Alt ↑/↓ az előző/következő beszélgetés, Alt Shift ↑/↓ az előző/következő olvasatlan.
- [ ] **K-42** A Naptárban M, W, D vált nézetet, ← → lapoz, T a mára ugrik; az Agentek oldalon a `/` a keresőmezőbe visz.

## 8. Lebegő ablakok (32. kör)

- [ ] **K-43** Az oldalsávban a Feladatok, Naptár, Jegyzetek mellett hoverre megjelenik a ⧉ gomb; kattintásra lebegő ablak nyílik a chat fölött. A fejléc dokkjából is nyitható.
- [ ] **K-44** A második ablak **az első mellé** nyílik, nem rá. *(Javított hiba.)*
- [ ] **K-45** A címsorral húzható; a szélekhez 16 px-en belül tapad; mind a 8 élről méretezhető (legalább 280×200); fókuszált címsoron Alt+nyilak mozgat, Alt Shift+nyilak méretez.
- [ ] **K-46** Halványítás gomb: Normál (mindig 100 %) → Áttetsző (kurzor nélkül 55 %) → Halvány (20 %); hover vagy fókusz mindig visszahozza 100 %-ra; a halvány ablak is kattintható.
- [ ] **K-47** A **nem legfelső** ablak gombjára (pl. Kis méret) **egyetlen** kattintás elég. *(Javított hiba: az első kattintás csak előrehozta az ablakot.)*
- [ ] **K-48** Kis méret: 48 px-es kör az ikonnal és jelvénnyel (Feladatok: elkezdett feladatok száma; Naptár: percek a következő eseményig, ha egy órán belül van). Húzható, a közelebbi oldalhoz tapad; kattintásra ugyanott nyílik vissza.
- [ ] **K-49** A „Teljes oldal” gomb (vagy a címsor dupla kattintása) az oldalra visz és bezárja az ablakot.
- [ ] **K-50** Az ablakok helye és állapota újratöltés után megmarad.
- [ ] **K-51** [📱] Telefonon nincs lebegő ablak: a megnyitott ablak tartalma alsó lapként jelenik meg.

## 9. Feladatok (32. kör)

- [ ] **K-52** [⚠] A gyors hozzáadás mezőbe írt cím Enterre feladat lesz az „Ütemezve” csoportban.
- [ ] **K-53** Az állapotkör kattintásra léptet: ütemezve → elkezdve → befejezve; a „Befejezve (14 nap)” csoport összecsukott.
- [ ] **K-54** Lista nézetben ↑/↓ gombokkal, Tábla nézetben húzással mozgatható (oszlopok között is: állapotváltás). A választott nézet újratöltés után megmarad.
- [ ] **K-55** Egy feladatra kattintva oldalsó lapon a részletek: leírás markdownnal, szerkesztés, törlés megerősítéssel.
- [ ] **K-56** Egy másik böngészőfülön végzett változás azonnal megjelenik (élő frissítés), a lebegő ablakban is.

## 10. Naptár (32. kör)

- [ ] **K-57** Feliratkozás nélkül üres állapot jelzi, hogy itt nem lehet eseményt létrehozni; a „Naptárak” gombbal ICS-link adható hozzá (név, szín). A link maszkolva látszik, az utolsó letöltés vagy a hiba kiírva.
- [ ] **K-58** Havi nézet: hétfővel kezdődő 6×7-es rács, a mai nap piros körben, egész napos események sávként, időzítettek „09:30 Cím” sorként, „+N további” felugró listával; a napra kattintva Nap nézet.
- [ ] **K-59** **Október végén / március végén** (óraátállítás) a havi nézet hibátlanul betölt. *(Javított hiba: 42 nap + 1 óra.)*
- [ ] **K-60** Heti nézet: egész napos sáv, órarács 07:00-tól (a 07:00 felirat látszik), átfedő események egymás mellett, piros „most” vonal a mai napon.
- [ ] **K-61** Eseményre kattintva felugró részletek: cím, idő, naptár, helyszín, leírás (linkek kattinthatók).
- [ ] **K-62** Egy naptár láthatósága kikapcsolható; eseményei eltűnnek, újratöltés után is.
- [ ] **K-63** [📱] Telefonon a hónap pöttyöket mutat, a kiválasztott nap napirendje a rács alatt; a Hét helyett „3 nap”.
- [ ] **K-64** A lebegő Naptár ablak: mini hónap pöttyökkel, alatta a következő 8 nap napirendje; egy napra kattintva odagörget.

## 11. Jegyzetek (32. kör)

- [ ] **K-65** A Jegyzetek oldal bal oldalt agentenként, azon belül szálanként listáz (szálcím, darabszám, idő); a keresés ékezet nélkül is talál; „Szál megnyitása” jobbra nyitja a szálat.
- [ ] **K-66** A lebegő Jegyzetek ablak követi a szálpanelben nyitott szálat; 📌-vel rögzíthető, ilyenkor szálváltáskor sem vált; szál nélkül „Nyiss meg egy szálat”.

## 12. Ütemezések (32. kör)

- [ ] **K-67** [⚠] „Új ütemezés”: mind az öt mód (Egyszer gyorsgombokkal, Naponta + munkanapok, Hetente napválasztóval, Havonta, Haladó) alatt magyar összefoglaló, a következő 5 futás (a szervertől) és a cron.
- [ ] **K-68** Haladó módban hibás cron esetén a szerver indoka jelenik meg, a mentés tiltott.
- [ ] **K-69** A szerkesztő „+” menüjében „Ütemezés…” a beírt szöveggel és az aktuális csatornával/szállal nyit; mentés után a szerkesztő kiürül.
- [ ] **K-70** A táblázat: üzenet, hova, ismétlés szavakkal, következő és utolsó futás, futások, állapot; Aktív / Kész / Törölt szűrő, adminnak „Mindenkié”.
- [ ] **K-71** [⚠] „Megszakítás” megerősítés után; „Másolás új ütemezésbe” kitöltve nyitja az ablakot.

## 13. Wiki (32. kör)

- [ ] **K-72** Az index: címkék számmal bal oldalt („Összes” elöl), témasorok (cím, kivonat, címkék, szerző, bejegyzésszám, résztvevők, utolsó aktivitás); rendezés; keresés; „Új” jelölés.
- [ ] **K-73** [⚠] „Új téma” ablak: cím, címkék chipként (ajánlással), szöveg írás/előnézet füllel; létrehozás után a témára visz.
- [ ] **K-74** Egy témában a bejegyzések kártyaként; egy agent hozzáfűzése külön válaszként jelenik meg, a szerver fejléce nélkül; emberi szerkesztés „✎ szerkesztette @…” sorként.
- [ ] **K-75** „Idézet” a válaszmezőbe idézi a bejegyzést; „Link” vágólapra teszi; a válasz elküldve megjelenik.
- [ ] **K-76** Előzmények: két változat különbsége, a változatlan részek „… N változatlan sor …” sorrá csukva.
- [ ] **K-77** [⚠] Törlés megerősítéssel; a „Törölt” szűrőben látszik és visszaállítható.

## 14. Foglalások (32. kör)

- [ ] **K-78** A táblázat oszloponként rendezhető; a lejárat élő visszaszámlálás, 5 percen belül borostyán, lejárva piros „lejárt”; a hátralévő idő sávja fogy.
- [ ] **K-79** Aktív / Előzmények (7 nap) szűrő; tartó szerinti és szabad szöveges szűrés.
- [ ] **K-80** [⚠] „Felszabadítás” megerősítés után; a szöveg megnevezi az erőforrást és a tartót.
- [ ] **K-81** [📱] Telefonon kártyák ugyanazokkal az adatokkal.

## 15. Szabályok és Döntések (32. kör)

- [ ] **K-82** A Szabályok olvasó nézetében a `{handle}`, `{admins}`, `{workspace}` súgós chipként; „Előnézet @agentként” kitölti őket.
- [ ] **K-83** [⚠] Szerkesztés: osztott nézet, helyőrző-beszúró sáv, 8000-es számláló (90 %-tól borostyán); „Mentés új verzióként” előtt a különbség látszik.
- [ ] **K-84** A verzió-idővonalon egy régi verzió „Összevetés”-sel a mostanihoz mérhető.
- [ ] **K-85** A Döntések oldal: `D-` szám, szerző, idő, indoklás, „felülírja / felülírta” jelzés, keresés, link a beszélgetésre.

## 16. Agentek oldal (33. kör)

- [ ] **K-86** Az `/app/agents` gépek szerint csoportosítva mutatja az agenteket (csoportfejléc: gép színe, neve, online-e, darabszám); a kártyán bal oldalt a gép színcsíkja, az avataron a szín gyűrűje.
- [ ] **K-87** A keresés (`/`) név, leírás, gép, kliens, modell és állapot szerint is talál, a találat kiemelve; elírással is (fuzzy).
- [ ] **K-88** A szűrőchipek (gép, kliens, hozzáférés, állapot, archivált) kombinálhatók, az URL-ben vannak (a link megosztható, újratöltés után megmaradnak). Archivált agent csak az „archivált” szűrővel látszik.
- [ ] **K-89** Csoportosítás (gép / állapot / nincs) és nézet (kártya / táblázat) vált, és megmarad.
- [ ] **K-90** Kártyára kattintva jobbra az áttekintő panel nyílik; a ⚙ a beállító ablakot nyitja.

## 17. Új agent ablak (33. kör)

- [ ] **K-91** [⚠] „Új agent”: hat lépés bal oldalt; a figyelmeztető sáv („Létrehozás után … nem módosítható”) **minden lépésen** ott van; a nem módosítható mezők mellett 🔒, a többinél „utólag is módosítható”.
- [ ] **K-92** Handle: nagybetű és szóköz hibát ad; egy aktív agent nevére „használja”; egy archivált agent nevére „átveheted”.
- [ ] **K-93** Gép és kliens: a gépkártyákon szín, online állapot, jelentett kliensek verzióval; nem jelentett kliens nem választható; „API-kulcsos agent” választható. A „bypassPermissions” mellett figyelmeztetés, alapból „auto”.
- [ ] **K-94** Hozzáférés: Sandbox / Teljes hozzáférés kártya magyarázattal; a gyökérmappa, a worktree és a mappaengedélyek a mappaválasztóból (lásd 19.); az előnézet csak teljes hozzáférésnél, a web_read csak sandboxnál kapcsolható.
- [ ] **K-95** Utasítás: helyőrző-beszúró, számláló, titok-figyelmeztetés; egy `GITHUB_TOKEN=ghp_…` sor beírására piros figyelmeztetés; a „Hatályos utasítás” a közös szabályok után mutatja a sajátot.
- [ ] **K-96** Összegzés: két oszlop („Nem módosítható” / „Utólag módosítható”); a „Létrehozás” gomb csak a tudomásulvétel bepipálása után él.
- [ ] **K-97** [⚠] Létrehozás után az agent megjelenik a könyvtárban és (gépes agentnél) a gép őrénél; API-kulcsos agentnél a token egyszer látható, másolható.
- [ ] **K-98** Szerverhiba (pl. a gép közben offline lett, vagy közben elfoglalták a nevet) esetén az ablak a hibás lépésre ugrik, és ott kiírja a szerver üzenetét.
- [ ] **K-99** Beírt adatokkal bezárva rákérdez („Elveted…?”).
- [ ] **K-100** [📱] Telefonon teljes képernyő; az első képernyő a lépéslista, a „Vissza” oda visz vissza.

## 18. Beállító ablak, törlés, archiválás (33. kör)

- [ ] **K-101** A ⚙ (vagy az áttekintő „Beállítások” gombja) megnyitja a beállító ablakot; az URL `?agent=<handle>&settings=<fül>` (a link másolható, újratöltve ugyanott nyílik).
- [ ] **K-102** Betöltéskor **nincs** „Közben módosult” sáv. *(Javított hiba.)*
- [ ] **K-103** [⚠] Profil: emoji, szín (a gép színe vagy saját), leírás, állapotszöveg, időzóna menthető; mentés előtt „Mi változik” lista; mentetlen változással fülváltáskor vagy bezáráskor rákérdez.
- [ ] **K-104** [⚠] Keretek: munkamenet-keret (100k/150k/300k/nincs) és napi keret menthető, mellette a mai fogyasztás. [őr] A változás után a futó agent munkája **nem** szakad meg.
- [ ] **K-105** Konfiguráció: minden érték csak olvasható, 🔒-tel; „Másolat új agentként” kitöltve nyitja az „Új agent” ablakot (a handle-re „használja” hibát ad, amíg a régi él).
- [ ] **K-106** Utasítás: a saját utasítás formázva, a „Hatályos utasítás” lenyitható.
- [ ] **K-107** [⚠] Veszélyes műveletek: Szüneteltetés / Folytatás; Leállítás megerősítéssel.
- [ ] **K-108** [⚠] [őr] „Agent törlése”: a következmények listája; a gomb csak a handle beírása után él. Törlés után:
  - az agent eltűnik a könyvtárból, az „archivált” szűrőben látszik;
  - a régi üzenetei megmaradnak;
  - a gépe elengedi, a futó termináljai „leállítva” lesznek;
  - a megemlítése senkit nem ébreszt;
  - a beállító ablaka csak olvasható, de „Másolat új agentként” működik.
- [ ] **K-109** [⚠] Ugyanazzal a handle-lel új agent hozható létre; a megemlítés már az újat ébreszti.
- [ ] **K-110** A fejlesztői oldal (`/`) „Gépek” panelje nem enged agentet létrehozni vagy beállítani, csak utal a PWA-ra; a keretek ott is állíthatók.

## 19. Mappaválasztó (33. kör)

- [ ] **K-111** [őr] Bal oldalt a gép `guard.yaml` gyökérmappái; a lista kb. egy másodpercen belül betölt; a morzsasor szegmensei kattinthatók.
- [ ] **K-112** [őr] Ikonok: git-repó (ág ikon), titkos mappa lakattal (nem választható, súgó: „Titkos mappa, nem adható”), nehéz mappa (`node_modules`) halványan; a rejtett mappák kapcsolóval jelennek meg; a szűrő a betöltött mappák közt keres.
- [ ] **K-113** [őr] Gyökérmappánál egy mappa választható; worktree-nél csak git-repó.
- [ ] **K-114** [őr] Mappaengedélyeknél jelölőnégyzet + olvasás/írás kapcsoló; a tálca összegzi („3 mappa: 2 olvasás, 1 írás”); szülő és almappa együtt kijelölve figyelmeztet, ha a szülő már lefedi.
- [ ] **K-115** [őr] Billentyűzet: nyilak, Enter megnyit, Szóköz kijelöl, Backspace feljebb.
- [ ] **K-116** Leállított őrrel „A gép most nem elérhető”, az utolsó látás idejével és „Újra” gombbal; adminnak „Útvonal beírása” tartalék, a gyökérmappákon kívüli útvonal figyelmeztető chipet kap.

## 20. Agent-áttekintő panel (33. kör)

- [ ] **K-117** Fejléc: nagy avatar színgyűrűvel, név, `@handle`, leírás; chipek: állapot, gép (pötty + név), kliens · modell, Sandbox vagy Teljes hozzáférés (borostyán pajzs). Gombok: DM, Fájlok, Terminálok, Beállítások; adminnak „…” menü.
- [ ] **K-118** [őr] Most: hol dolgozik (szál-link), élő lépés és eltelt idő; az inbox darabszáma és a legrégebbi elem kora.
- [ ] **K-119** [őr] Munkamenetek: kontextus-sáv (80 %-tól borostyán, közel a kerethez „hamarosan új munkamenet”).
- [ ] **K-120** Felhasználás: ma és 7 nap token/futás, sáv a napi kerethez, 7 napos oszlopdiagram; Utolsó futások (hiba esetén „Hiba részletei”).
- [ ] **K-121** Konfiguráció 🔒-tel és „Részletek” (a beállító ablak Konfiguráció fülére visz); Együttműködés (delegációs engedélyek visszavonással, foglalások felszabadítással, nyitott kérdések, közös csatornák); Jegyzetek (3 legutóbbi); Utasítás (első sorok).
- [ ] **K-122** A szakaszok összecsukhatók, és újratöltés után úgy maradnak.
- [ ] **K-123** Ember avatarjára kattintva a kis változat: profil, DM, közös csatornák.

## 21. Gépek oldal, színek, „Új gép” (33. kör)

- [ ] **K-124** Az `/app/hosts` (csak adminnak) gépkártyái: színcsík, név, online (90 mp-en belül) vagy „utoljára … ezelőtt”, OS, őrverzió, agentek száma; figyelmeztetések (elutasított útvonal, nem írható csere-mappa, visszavont).
- [ ] **K-125** Áttekintés fül: kliensek verzióval, sandbox, csere-mappa, gyökérmappák, Claude-keret, elutasított útvonalak agenttel és okkal.
- [ ] **K-126** Agentek fül: a gép agentjei; „Áttekintés” a panelt, „Beállítások” a beállító ablakot nyitja; „Új agent ezen a gépen” a létrehozó ablakot ezzel a géppel.
- [ ] **K-127** Mappák fül: melyik agent hol kapott jogot (avatarok, olvasás/írás); „Böngészés” a mappaválasztót nyitja jelvényekkel.
- [ ] **K-128** Worktree-k és előnézetek fül: worktree-k (agent, útvonal, ág), előnézetek (link, állapot, lejárat, „Leállítás” megerősítéssel).
- [ ] **K-129** [⚠] Beállítások fül: név és szín (a színminták élő előnézettel az agentek gyűrűjén); mentés után a szín mindenhol (könyvtár, áttekintő, oldalsáv) átvált. Visszavonás megerősítéssel (a gép tokenjei megszűnnek).
- [ ] **K-130** [⚠] „Új gép” varázsló: név, párosítókód visszaszámlálással, másolható `colony guard enroll …` parancs; a gépen lefuttatva a varázsló **magától** továbblép, utána a `guard.yaml` minta és a `colony guard install` parancs; „Kész” a gép oldalára visz.

## 22. Jogosultságok (31–33. kör)

Tag (member) fiókkal:

- [ ] **K-131** Nem látja a Gépek és a Szabályok oldalt az oldalsávban; a `/app/hosts` közvetlen címen sem ad adatot.
- [ ] **K-132** Az Agentek oldalon nincs „Új agent” gomb; a beállító ablakban nem menthet és nem törölhet (a szerver 403-at ad, a felület nem kínálja).
- [ ] **K-133** Az áttekintő panelen nincs admin menü, inbox-sor és konfiguráció.
- [ ] **K-134** A Foglalásokat látja, de az ütemezések közül csak a sajátjait („Mindenkié” nincs).

Agent tokennel (pl. `curl -H "Authorization: Bearer <agent token>"`):

- [ ] **K-135** `POST /api/v1/agents`, `DELETE /api/v1/agents/<id>`, `PATCH /api/v1/hosts/<id>`, `POST /api/v1/hosts/<id>/fs` → 403.
- [ ] **K-136** `PUT /api/v1/agents/<id>/host` bárkinek 409 `agent_config_immutable`, a részlet megnevezi a `POST /api/v1/agents`-et.

## 23. Javított hibák (2026-11-01)

- [ ] **K-137** [őr] Egy agenttel nyitott DM-ben a „Fájlok” és a „Terminál” gomb megnyitja a panelt; **nem** írja, hogy „Nincs közös csatornád ezzel az agenttel”.
- [ ] **K-138** Az oldalsáv „+” gombjai (Csatornák, Közvetlen üzenetek, Te és az agentek) a saját felületen nyitnak ablakot, nem a régire visznek: új csatorna (név, nyilvános/privát), új DM emberrel, új beszélgetés agenttel (kereső, archivált agent nincs benne); kiválasztás után a beszélgetés megnyílik.
- [ ] **K-139** [őr] Egy teljes hozzáférésű agent megkér egy sandboxos agentet egy DM-ben; a sandboxos válasza **jóváhagyási kérdés nélkül** megérkezik, és a teljes hozzáférésű agent dolgozik tovább. Egy kéretlen, önálló kérés továbbra is jóváhagyásra vár.
- [ ] **K-140** [őr] Egy sandboxos Antigravity-agent futása után az őr naplójában nincs `sandbox network refused … aicode.googleapis.com`.

---

## Futások

Minden futás egy új bejegyzés: dátum, ki, hol (fejlesztői / éles), verzió (`git log --oneline -1`), böngésző és eszköz, eredmény. A bukott pontokat azonosítóval és egy mondattal sorold fel; a javítás után ugyanazt a pontot futtasd újra, és jelöld.

### Minta

- **Dátum:** 2026-11-02 · **Ki:** Norbi · **Hol:** éles · **Verzió:** `7fb8ee5` · **Eszköz:** Chrome 1440×900 + Android telefon (PWA)
- **Eredmény:** 135 / 140 rendben
- **Bukott:**
  - K-60: a heti nézetben a 07:00 felirat le van vágva.
  - …
