# A Colonyhall design systemje

**Kötelező a PWA minden képernyőjére** (`web/`). A kód felépítését a `docs/08-frontend-architektura.md` írja le, ez a dokumentum a kinézetet és a viselkedést. Az élő változat a **`/app/design`** oldalon van: minden token és komponens ott látszik, a téma váltható. A minták sima HTML-ben: `ui/ui-mockup/`.

Forrás a kódban:

- tokenek: `web/src/shared/styles/tokens.css`;
- komponensek: `web/src/shared/ui/`;
- a kontrasztszabályok tesztje: `web/src/shared/styles/contrast.test.ts`.

---

## 1. Karakter

Hangyaboly éjjel: mély, meleg feketék, és egyetlen erős piros (a katonahangya). A piros jelentése „Colonyhall”, az elsődleges művelet, a megemlítés és a veszély. **Takarékosan kell használni**, hogy megtartsa a súlyát. Rajta kívül semmi sem harsány.

---

## 2. Színek: csak szemantikus tokenek

A komponensek **soha nem használnak nyers színt** (`#e5383b`, `bg-red-500`), csak a tokenek nevét (`bg-surface`, `text-fg-muted`). Ezt teszt ellenőrzi (`no-raw-colors.test.ts`). Nyers érték csak a `tokens.css`-ben szerepelhet.

| Token | Világos | Sötét | Mire |
| --- | --- | --- | --- |
| `bg` | `#faf8f7` | `#0b0a0a` | az oldal háttere |
| `surface` | `#ffffff` | `#141212` | panelek, oldalsáv, fejléc |
| `surface-raised` | `#ffffff` | `#1c1919` | ablakok, menük, kártyák |
| `surface-sunken` | `#f3efed` | `#0f0d0d` | hover, mezők háttere, kódsor |
| `border` | `#e6e0de` | `#2a2525` | elválasztó vonal (dekoráció) |
| `border-strong` | `#8f8582` | `#6f6464` | mezők és gombok kerete (≥ 3:1) |
| `fg` | `#141111` | `#f2eeee` | fő szöveg |
| `fg-muted` | `#6b6262` | `#a39a9a` | másodlagos szöveg |
| `fg-subtle` | `#8a807e` | `#857b7b` | ikon, placeholder, nem lényeges jelzés (≥ 3:1; folyó szövegre nem) |
| `accent` | `#c1121f` | `#d42a2e` | piros **kitöltés** (elsődleges gomb, számláló jelvény) |
| `accent-hover` | `#a40f1a` | `#bf2226` | a kitöltés hover állapota |
| `accent-fg` | `#ffffff` | `#ffffff` | szöveg a piros kitöltésen |
| `accent-soft` | `#fbe9ea` | `#3a1416` | piros árnyalatú háttér (aktív sor, megemlítés) |
| `accent-text` | `#c1121f` | `#ff6b6e` | **piros szöveg** a felületeken (link, „N válasz”) |
| `danger` / `danger-soft` | `#c1121f` / `#fbe9ea` | `#ff6b6e` / `#3a1416` | veszélyes művelet, hiba |
| `warning` / `warning-soft` | `#8a5d00` / `#fdf3dc` | `#e0a63a` / `#33270f` | figyelmeztetés, borostyán |
| `success` / `success-soft` | `#2a7046` / `#e5f4ea` | `#5cc286` / `#12291c` | siker |
| `info` / `info-soft` | `#2f5f9e` / `#e6eefb` | `#7aa7e6` / `#15233a` | tájékoztatás |
| `focus-ring` | piros, 50% | piros, 60% | a fókusz körvonala |
| `overlay` | fekete, 60% | fekete, 65% | ablakok mögötti sötétítés |
| `terminal-bg` / `terminal-fg` | `#0f0d0d` / `#e9e4e3` | ugyanaz | **a terminál mindig sötét**, mindkét témában |
| `brand-paper` / `brand-ink` | `#faf8f7` / `#0b0a0a` | ugyanaz | témától független háttér a logó és az ikonok bemutatásához |

**Miért két piros?** Sötét témában ugyanaz a piros nem lehet egyszerre jól olvasható szöveg a sötét felületen és jó háttér a fehér szövegnek. Ezért van külön `accent` (kitöltés) és `accent-text` (szöveg).

**Kontraszt (WCAG AA), teszttel ellenőrizve, mindkét témában:**

- szöveg (`fg`, `fg-muted`, `accent-text`, `danger`, `warning`, `success`, `info`, a terminál szövege): legalább **4,5:1** minden felületen, amelyen megjelenik;
- `fg-subtle` és `border-strong` (ikon, placeholder, vezérlő kerete): legalább **3:1**;
- `accent-fg` a piros kitöltésen (alap és hover): legalább 4,5:1.

**A szín soha nem az egyetlen jelzés:** a veszélyt ikon is jelzi, az állapotot felirat vagy forma, a gépet a neve.

Új token felvétele: érték mindkét témában a `tokens.css`-ben, ha szöveg vagy vezérlő, akkor sor a `contrast.test.ts`-be, és minta az `/app/design` oldalra.

---

## 3. Téma

- Három mód: **Rendszer** (alapértelmezés), **Világos**, **Sötét**. A felhasználói menüben és az `/app/design` oldalon lehet váltani.
- A `.dark` osztály a `<html>`-en kapcsolja át a tokeneket. Az `index.html` inline szkriptje **az első kirajzolás előtt** állítja be, így nincs villanás.
- A választás a böngészőben marad (`colony.theme`). Ha a tárolás tiltott, a rendszerbeállítás érvényes.
- A tokenek maguk váltanak, ezért `dark:` variáns ritkán kell.

---

## 4. Tipográfia

- **Felület:** Inter. **Kód, terminál, fájlút, technikai handle:** JetBrains Mono. Mindkettő SIL OFL licencű, **saját szerverről** jön (`@fontsource-variable/*`), Google Fonts nincs. A böngésző csak a ténylegesen használt nyelvi készletet tölti le (latin, latin-ext: ő, ű).
- **Méretskála:**

  | Osztály | Méret | Sortáv | Mire |
  | --- | --- | --- | --- |
  | `text-meta` | 12 px | 1,4 | idő, jelvény, meta |
  | `text-ui` | 13 px | 1,45 | oldalsáv, gombok, menük, másodlagos szöveg |
  | `text-body` | 14 px | 1,5 | **üzenet törzse, alapértelmezett** |
  | `text-title` | 16 px | 1,3 | panel- és ablakcím |
  | `text-page` | 20 px | 1,3 | oldalcím |
  | `text-display` | 24 px | 1,25 | üres állapot, nagy cím |

- Vastagság: 400, 500, 600.
- Változó számokhoz (tokenszám, időzítő, számláló) a `tabular` osztály kell, hogy a számok egyforma szélesek legyenek.

---

## 5. Forma, tér, mélység, mozgás

- **Térköz:** 4 px-es lépések (a Tailwind alapskálája).
- **Lekerekítés:** `rounded-sm` 4 px, `rounded-md` 8 px, `rounded-lg` 12 px, `rounded-full`.
- **Formanyelv: az ember kör, az agent lekerekített négyzet.** Mindenhol így van: avatar, megemlítés, taglista. Ez a fő megkülönböztető jel, a szín nem helyettesíti. Az agent avatarja körül a színe (alapból a gépéé) gyűrűként jelenik meg.
- **Mélység:** sötét témában keretek, világosban lágy árnyék (`shadow-raised`, `shadow-overlay`). Az ablakok mögött `overlay` sötétítés enyhe elmosással.
- **Mozgás:** 150 ms, `ease-out`, csak panelekre és ablakokra. Ha a `prefers-reduced-motion` be van kapcsolva, nincs animáció. Magától csak a „dolgozik” jelzés lüktet.

---

## 6. Komponensek (`shared/ui`)

Egy funkció nem gyárt saját gombot vagy ablakot: ezeket használja, és ha valami hiányzik, itt kell bővíteni (tesztekkel, és mintával az `/app/design` oldalon).

| Komponens | Mire, szabály |
| --- | --- |
| `Button` | `primary` (piros, képernyőnként legfeljebb egy fő művelet), `secondary`, `ghost`, `danger`; `sm`/`md`; `loading` |
| `IconButton` | **kötelező `label`** (ez az akadálymentes név és a tooltip); opcionális számláló jelvény |
| `Input`, `Textarea`, `Select` | `border-strong` keret, hibánál `aria-invalid` |
| `Checkbox`, `Switch` | Radix; mindig címkével |
| `Badge` | számláló, állapot, git-betű (`mono`); tónus: `neutral`, `accent`, `success`, `warning`, `info`, `danger` |
| `Avatar` | `human` kör / `agent` négyzet; emoji vagy monogram; állapot: `working` (piros, lüktet), `idle`, `offline` (üres gyűrű), `paused` |
| `Tooltip` | rövid magyarázat; a `TooltipProvider` az `app/`-ban van |
| `Menu…` | Radix legördülő menü; veszélyes tétel `danger` |
| `Tabs`, `TabList`, `Tab`, `TabPanel` | vízszintes vagy függőleges fülek |
| `Dialog` | rövid űrlap, megerősítés; `md`, `lg`, `xl` (nagy beállítóablak) |
| `Sheet` | hosszabb, csak olvasható részlet |
| `toast()` + `Toaster` | visszajelzés, egy opcionális művelettel (pl. „Visszavonás”) |
| `EmptyState`, `Skeleton` | üres és töltődő állapot |
| `Kbd` | billentyű a súgóban |
| `HoldButton` | nyomva tartással megerősít (vészleállító, 2 mp); egérrel és Szóközzel/Enterrel is |
| `BrandMark` | a hangya jel |

Ikonok: **Lucide** (`lucide-react`), sűrű helyen 16 px, máshol 20 px. A logó saját.

---

## 7. Hogyan nyílnak az ablakok és panelek

Ötféle réteg van. A célja szerint kell választani, nem ízlés szerint:

1. **Dialog** (középen; `md` 448 px, `lg` 512 px, `xl` 896 px): rövid űrlap és megerősítés. Esc és a háttérre kattintás bezárja (ha mentetlen bevitel van, előbb rákérdez). A fókusz bent marad, bezárás után visszatér a kiváltó elemhez. **640 px alatt alulról felcsúszó lap lesz belőle**, fogantyúval.
2. **Sheet** (asztali gépen jobbról, telefonon alulról): hosszabb, csak olvasható részlet, az aktuális képernyőhöz tartozik.
3. **Pane** (panel): tartós oszlop a keretben, nem modális (szál, terminál, fájlböngésző, agent-áttekintő). A helyét a panelkezelő szabályai adják (31. kör, 5. rész), és az URL-ben is benne van.
4. **Lebegő ablak** (32. kör): mozgatható, kicsinyíthető, áttetsző ablak az oldalon belül (feladatok, naptár, jegyzetek).
5. **Toast** (asztali gépen jobbra lent, telefonon fent): válasz nélküli visszajelzés.

**Rétegsorrend:** panel < lebegő ablak < Sheet és Dialog < Toast (`--z-pane`, `--z-window`, `--z-overlay`, `--z-toast`).

**Szabályok:**

- egyszerre **legfeljebb egy** Dialog vagy Sheet lehet nyitva;
- kivételek: megerősítő Dialog egy Sheet vagy egy nagy (`xl`) Dialog fölött, és a mappaválasztó a nagy beállítóablak fölött (33. kör);
- **`window.confirm` / `alert` / `prompt` tilos**;
- veszélyes műveletnél `danger` gomb kell, megerősítő Dialogban;
- a vészleállító `HoldButton`, ablak nélkül.

---

## 8. Akadálymentesség

- Ikon-gombnak kötelező a felirat (`IconButton` `label`).
- A fókusz látható (`focus-ring`), a billentyűzetes használat mindenhol működik.
- Minden ablak, menü és fül Radix-alapú: a szerepek, a fókusz és az Esc maguktól helyesek.
- A szín nem lehet egyetlen jelzés; a kontrasztot teszt ellenőrzi.
- `prefers-reduced-motion` mellett nincs animáció.
- Telefonon az iPhone-os biztonsági sávok (`safe-area-inset-*`) be vannak tartva.

---

## 9. A keret (31. kör)

- **Fejléc (48 px):**
  - ☰, logó és munkaterület, morzsasor;
  - középen a kereső, ami a parancspalettát nyitja (Ctrl K);
  - jobbra: Válaszra vár (számláló), „⚡ N dolgozik”, adminnak a Claude-keret gyűrűje (borostyán 80 %-tól) és a „Mind áll” `HoldButton`, végül a felhasználói menü.
  - Vészleállításkor az egész fejléc `accent` színű, „Feloldás” gombbal.
- **Oldalsáv (260 px):**
  - a Munkaterület oldalai, majd a saját szekciók;
  - Csatornák (`#` vagy lakat);
  - Közvetlen üzenetek (kör avatar);
  - AI-beszélgetések: „Te és az agentek” (négyzet avatar állapotpöttyel), „Agentek egymás közt” (két átfedő avatar, `@a ↔ @b`, szem ikon, ha csak megfigyelés).
  - Olvasatlan: félkövér; megemlítés: piros számláló; aktív sor: `accent-soft` háttér és 2 px piros bal csík.
- **Panelek** (a szabályok: `shared/panes/panes.ts`):
  - **széles (≥ 1280 px):** a szál jobbra kerül (440 px). A munkapanel (fülekkel: terminál, fájlok, agent-áttekintő) jobbra kerül, ha ott üres; ha a szál nyitva van, balra, az oldalsáv helyére (560 px), és ilyenkor az oldalsáv fiókként nyílik.
  - **közepes:** egy jobb oldali hely, az elrejtett panelre egy váltósávval lehet visszaváltani.
  - **telefon:** egymás után nyíló képernyők, „Vissza” gombbal.
- **Szerkesztő:**
  - lekerekített keret;
  - a „+” menü (fájl, telefonon kamera), Fájlok és Terminál gomb, súgó (csak ha elfér), piros küldés gomb;
  - fölötte a tálca (chipek folyamatjelző csíkkal) és a felugró listák (`@`, `/`).
- **Üzenet:**
  - 36 px avatar, név, agentnél `@handle` monóban, idő a teljes dátummal súgóban;
  - ha téged említenek: `accent-soft` háttér és 2 px piros bal csík;
  - szál-összesítő: résztvevők, „N válasz” `accent-text`-tel, utolsó válasz ideje.
- **Billentyűparancsok:** a `?` súgó felsorolja mindet. Ctrl K, `?`, Alt ↑/↓, Alt Shift ↑/↓ (olvasatlanok), Esc (panel), `r` (válasz szálban), Ctrl Shift F (fájlok), Ctrl Shift X (terminálok; a Ctrl Shift T a böngészőé), ↑ üres szerkesztőben.

## 10. Lebegő ablak és oldalfejléc (32. kör)

### Lebegő ablak (`layout/windows`, állapot: `shared/windows`)

- **Kinek van:** Feladatok, Naptár, Jegyzetek; fajtánként legfeljebb egy, több fajta lehet egyszerre nyitva. Megnyitás: az oldalsáv ⧉ gombja (hoverre látszik), a fejléc dokkja, az oldal fejlécének ⧉ gombja vagy a parancspaletta.
- **Méret:** alapból 340×440 (jegyzetek 360×420), legalább 280×200, legfeljebb a nézet. A címsor 34 px, `surface-raised`, `border-strong` keret, `shadow-overlay`, `rounded-lg`.
- **Elhelyezés:** az első jobbra fent nyílik; ha egy nyitott ablak már ott van, az új **mellé balra** kerül, ha nincs hely, egy címsornyit lépcsőzetesen eltolva. Ahova a felhasználó húzta, ott marad (böngészőnként, `localStorage`).
- **Mozgatás, méretezés:** címsorral húzható, a címsor mindig elérhető marad, 16 px-en belül a szélekhez tapad; méretezés mind a 8 élről és sarokról. Billentyűzet: fókuszált címsoron Alt+nyilak mozgat, Alt+Shift+nyilak méretez (16 px lépés).
- **Gombok** (balról jobbra): Halványítás, Kis méret (—), Teljes oldal (□, az oldalra visz és bezárja az ablakot; a címsor dupla kattintása ugyanez), Bezárás (×).
- **Áttetszőség** (a Halványítás gomb lépteti):

  | Mód | Ha a kurzor/fókusz nincs rajta | Ikon |
  | --- | --- | --- |
  | Normál | 100 % | kör |
  | Áttetsző (alapérték) | 55 % | félkör |
  | Halvány | 20 % | szaggatott kör |

  Hover vagy fókusz mindig 100 %-ra hozza; 200 ms átmenet, `prefers-reduced-motion` mellett azonnali. Az áttetsző ablak is kap kattintást (nincs átkattintás).
- **Rétegzés:** a kattintott ablak kerül előre, `z-index`-szel; a DOM-sorrend **nem** változik (különben a böngésző elnyeli az előrehozó kattintást).
- **Kicsinyítve:** 48 px-es kör az ablak Lucide-ikonjával, `border-strong` gyűrűvel, hírnél piros gyűrűvel és jelvénnyel (Feladatok: elkezdett feladatok száma; Naptár: perc a következő eseményig, ha egy órán belül van, pl. „12p”; Jegyzetek: pötty új jegyzetnél). Húzható, a közelebbi oldalszélhez tapad; kattintásra (vagy Enterre) ugyanott nyílik vissza.
- **Telefonon** nincs lebegő ablak: a legfelső nyitott ablak tartalma alsó Sheetben jelenik meg.
- **Egy tartalom-komponens funkciónként:** az ablak és az oldal ugyanazt használja, az ablak tömör változatban (pl. a naptár mini-hónap + napirend).

Képek: `docs/handoff/32-screens/windows-*.png`.

### Oldalfejléc (`shared/ui/PageHeader`)

Minden munkaterület-oldal tetején: ikon (20 px) és cím (`text-page`), jobbra az oldal műveletei (nézetváltó, szűrők, „Új …” elsődleges gomb), legjobbra a ⧉ kiugrasztás, ha az oldalnak van lebegő ablaka. Magassága és alsó vonala a beszélgetés fejlécével egyezik. Nézetváltó: `radiogroup` szegmensekkel, a kiválasztott `accent-soft`.

### Diff (`shared/ui/DiffView`)

Sor szintű különbség (saját LCS, `shared/lib/diff.ts`): törölt sor `danger-soft` háttér, áthúzva, „−”; új sor `success-soft`, „+”; a hosszú változatlan szakaszok „… N változatlan sor …” sorrá csukódnak, a változások körül 2 sor marad. A wiki előzményei és a szabályok használják.

## 11. Gépek és agentek (33. kör)

### Színpaletta

Tíz megnevezett szín, **soha nem szabad hex**: a szerver a nevet tárolja, a kliens a tokenekre fordítja (`hueVar(név, "solid" | "soft" | "fg")`, `shared/lib/palette.ts`). Témánként három token: `--color-hue-<név>-solid` (csík, pötty, gyűrű; ≥ 3:1 minden felületen), `-soft` (háttér), `-fg` (szöveg a `-soft`-on, ≥ 4,5:1). A kontraszttesztek ellenőrzik. A márka `accent` pirosa nincs a palettán; a `red` hidegebb, hogy egy piros gép ne tűnjön megemlítésnek vagy hibának.

| Név | Világos: solid / soft / fg | Sötét: solid / soft / fg |
| --- | --- | --- |
| `red` | `#e03e6c` / `#fae5eb` / `#af1d46` | `#d3456d` / `#30121b` / `#ea7b9b` |
| `orange` | `#ce6722` / `#faeee5` / `#9e4f1a` | `#ba652c` / `#301e12` / `#eaa77b` |
| `amber` | `#ae7b13` / `#faf3e5` / `#8a610f` | `#ae8129` / `#302612` / `#f0c775` |
| `lime` | `#5f9310` / `#f2fae5` / `#4a730d` | `#78ae29` / `#243012` / `#bff075` |
| `green` | `#18954c` / `#e5faee` / `#13763d` | `#29ae60` / `#12301f` / `#7beaa9` |
| `teal` | `#189589` / `#e5faf8` / `#13766d` | `#29aea0` / `#12302d` / `#7beadf` |
| `cyan` | `#1c8eab` / `#e5f6fa` / `#167188` | `#2993ae` / `#122a30` / `#7bd4ea` |
| `blue` | `#3e79e0` / `#e5edfa` / `#1d52af` | `#4579d3` / `#121d30` / `#7ba4ea` |
| `violet` | `#843ee0` / `#eee5fa` / `#5c1daf` | `#935ed9` / `#1f1230` / `#ab7bea` |
| `pink` | `#e03eaf` / `#fae5f4` / `#af1d83` | `#d13da5` / `#301227` / `#ea7bc9` |

**Hol látszik:** az agent négyzetes avatarján 2 px-es gyűrű (`effective_color`: saját szín, különben a gépé); a gép-chipen pötty; a könyvtárban és a géplistán 3 px-es bal csík; a csoportfejlécen pötty. **A szín soha nem az egyetlen jelzés:** a gép neve mindig mellette vagy súgóban van.

### Nagy Dialog (`shared/ui/LargeDialog`)

`max-w-4xl`, legfeljebb 85 vh. Fejléc (cím, alatta pl. avatar és gép-chip), bal oldalt függőleges fülek vagy számozott lépések (`aria-current="step"`; hibás lépés `danger` színben), jobbra a görgethető tartalom, alatta ragadós lábléc. Telefonon teljes képernyős, az első képernyő a fül/lépéslista, a „Vissza” visszavisz rá. **Egy szintnyi gyermek-ablak** (megerősítő Dialog, mappaválasztó) nyílhat fölötte; a fókusz bezáráskor visszatér (a „legfeljebb egy Dialog” szabály engedélyezett kivétele, lásd 7.).

### A „nem módosítható” minta

- Állandó `warning` `Notice` **minden lépés tetején**: „🔒 Létrehozás után … nem módosítható. Változtatáshoz törölni kell az agentet, és újat létrehozni.”
- 🔒 minden nem módosítható mező címkéje mellett; „utólag is módosítható” megjegyzés a többinél.
- Az összegzés két oszlopa: „🔒 Nem módosítható” és „Utólag módosítható”.
- Kötelező jelölőnégyzet („Tudomásul veszem …”): a „Létrehozás” gomb addig tiltott.
- A beállítóablakban a konfiguráció csak olvasható, fölötte `info` sáv és „Másolat új agentként” gomb.
- Törlés: megerősítő Dialog a következmények listájával; a gomb csak a handle beírása után él.

### Mappaválasztó (`features/hosts/components/FolderPicker`)

Asztalon `max-w-3xl`, 70 vh (a nagy Dialog fölött), telefonon teljes képernyő. Bal oldalt a gép `guard.yaml` gyökérmappái, jobbra morzsasor (szegmensenként kattintható), szűrőmező, „Rejtett mappák” kapcsoló és a mappalista. Ikonok: mappa, git-repó (`FolderGit2`), lakat a titkos (`denied`, tiltott, súgó: „Titkos mappa, nem adható”), halványított a nehéz (`heavy`). Módok: **single** (gyökérmappa; worktree-nél csak git-repó), **multi** (jelölőnégyzet + olvasás/írás kapcsoló soronként, alul tálca: „3 mappa: 2 olvasás, 1 írás”, figyelmeztetés, ha egy szülő hozzáférése már lefed egy kijelölt almappát), **browse** (a Gépek oldalon, kijelölés nélkül, mappánkénti jelvényekkel). Állapotok: betöltés (vázsorok), offline (EmptyState az utolsó látással és „Újra”), időtúllépés („Újra”). Admin tartalék: „Útvonal beírása” (a gyökérmappákon kívüli útvonal figyelmeztető chipet kap). Billentyűk: nyilak, Enter megnyit, Szóköz kijelöl, Backspace feljebb.

## 12. Ami még jön

- **34. kör:** a chat hiányzó részei (`D-12`/`Q-12` chipek, csatorna-beállítások, némítás, „Szálaim”).
- **35. kör:** az admin oldalak (futások, keretek, audit, felhasználók).
