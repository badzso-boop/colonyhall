# Műhely: a felhasználók saját bővítményei (56. kör): terv

> Állapot: **elfogadott irány, 2026-11-17: az 56. kör, az 51c előtt.** Norbi döntései a 0. pontban; a kapu kérdései eldőltek (12. pont), a részletek (pontos korlátok, végpontok) a kör promptjába kerülnek. Kapcsolódik: a 24. dokumentum (hosztolt ajánlat, **51c**: a hosztolt őr futtatja a hosztolt ügyfelek Építő- és rutinagentjeit), a 02. dokumentum MCP-eszközei.

## 0. Ami már eldőlt (Norbi, 2026-11-15 – 11-17)

| # | Kérdés | Döntés |
| --- | --- | --- |
| 1 | Az irány | **Nagy szabadság a felhasználóknak.** Minden munkaterület a saját agentjeivel bővítheti a saját Colonyhallját; a modern, AI-központú működést kell kiszolgálnia, nem egy kívánságlistát (az app még nincs kint, kívánságlista nem kell) |
| 2 | A határ | **A mag működésébe nem szólhatnak bele** (5. pont). Mindenbe, ami a magra épül, igen: saját MCP-eszközök létrehozása és szerkesztése, meglévő MCP-szerverek bekötése, automatizálás, interaktív üzenetek |
| 3 | Saját eszközök futtatása | **Teljes:** deklaratív HTTP-eszköz **és** kód (JS/TS QuickJS-en, WASM-ban, `wazero`) is, már az első változatban |
| 4 | Hol fut az Építő és a rutinok agentje | Saját telepítésen a felhasználó őrén. **Hosztolt ügyfélnél** (nincs saját őre) **a szerveren futó hosztolt őrön** — ez az **51c** része (24. dokumentum) |
| 5 | Tokenek | **Az ügyfél saját API-kulcsa fizeti** (a 24. dokumentum titkosított AI-kulcsai) |
| 7 | Ki hozhat létre bővítményt | **A tulajdonos és az adminok.** A tagok használhatják az engedélyezett bővítményeket |
| 8 | Író és kifelé küldő hívások | **Alapból jóváhagyást kérnek.** A tulajdonos bővítményenként kikapcsolhatja („ha valaki bátor”); a kikapcsolás auditba kerül |
| 9 | A menü neve | **Műhely** (angolul: Workshop) |
| 10 | A JS-futtató | **Rögtön külön szerepkörben** (`ext-runner`, külön folyamat), hogy egy hibája vagy túlterhelése ne érintse a szervert |
| 11 | Körszám és sorrend | **56. kör, az 51c előtt** |
| 6 | Út a forráskódig | Akinek jók a bővítményei, Norbi felajánlhatja neki, hogy PR-t nyisson a GitHubon a fő projektbe (megbízott építő, 7. pont) |

## 1. A kép

```
            ┌───────────────────────── a felhasználó rétege (munkaterületenként) ─────────────────────────┐
            │  kapcsolatok   saját eszközök   rutinok   interaktív üzenetek   bővítmény-adattár   Építő  │
            └──────────────────────────────────────────────┬──────────────────────────────────────────────┘
                                                           │ csak a v1 bővítési API-n át
            ┌──────────────────────────────────────────────▼──────────────────────────────────────────────┐
            │  MAG: belépés, jogok, bérlők, audit, csomag/kvóta, vészleállító, break-glass, séma, a modulok │
            └──────────────────────────────────────────────────────────────────────────────────────────────┘
```

A bővítmény **adat, nem a forráskód módosítása**: egy manifest, deklaratív részek és sandboxban futó kód, munkaterületenként tárolva és verziózva. A közös bináris minden munkaterületen ugyanaz; a különbség a munkaterület bővítményeiben van. Így a frissítés nem ütközik, és a többbérlős szerver is működik.

## 2. Az építőkockák

### 2.1 MCP-átjáró (a központi elem)
Minden agent **egy MCP-végponthoz** kapcsolódik: a Colonyhallhoz. Az átjáró háromféle eszközt ad tovább:

1. **A Colonyhall saját eszközeit** (ma: `internal/modules/*/mcptools`). Elv: **amit egy ember a felületen meg tud csinálni, azt az agentje MCP-n át is**, ugyanazokkal a jogokkal. Hiányzik még többek közt: csatorna létrehozása és tagkezelése, kitűzés, wiki írása, tervező írása, rutin- és bővítménykezelés (az Építőnek).
2. **Bekötött külső MCP-szervereket** (2.2).
3. **A munkaterület saját eszközeit** (2.3).

Amit ez ad: a titkok a szerveren maradnak (az agent nem látja a Jira-tokent); minden eszközhívás naplózva (ki, melyik agent, melyik eszköz, mennyi idő, eredmény mérete; a tartalom nem); agentenként engedélyezett eszközlista (a mai profilok kiterjesztése); a vészleállító és a felfüggesztés minden hívást megállít; a csomag korlátozhatja a hívások számát.

Névtér: `colony.*` (mag), `<kapcsolat>.*` (külső), `ext.<bővítmény>.*` (saját). Ütközés nem lehet: a mag neve foglalt.

### 2.2 Kapcsolatok (connectors)
Munkaterületenként regisztrált külső MCP-szerver (streamable HTTP) vagy HTTP API (alap-URL + hitelesítés). Az azonosító adatok a `workspace_secret` táblában titkosítva (`secretbox`, 51a), soha nem látszanak újra egészben. Beállítható, ki és melyik agent használhatja. **stdio-s MCP-szerver** (helyi parancs) csak őrön futhat: saját telepítésen a felhasználó őrén, hosztolt ügyfélnél a hosztolt őr sandboxában (51c).

### 2.3 Saját eszközök
A felhasználó vagy az agentje hozza létre és szerkeszti. Két fajta, ugyanazzal a felülettel az átjáróban:

- **Deklaratív HTTP-eszköz:** bemeneti JSON-séma, kéréssablon (metódus, útvonal, fejlécek, törzs; a titok csak hivatkozással), válasz-leképezés (JSON-útvonalak, tömörítés a modellnek). Nem fut benne idegen kód.
- **Kód (JS/TS):** QuickJS `wazero`-ban (WASM, tisztán Go, nincs cgo). A TS-t mentéskor fordítjuk JS-re. Korlátok hívásonként: memória (pl. 64 MB), futásidő (pl. 10 mp, üzemanyag/megszakítás), kimenet mérete; munkaterületenként párhuzamos hívások száma (a csomagból). **Csak a host-függvényeket látja:** `fetch` (csak a manifestben bejelentett címekre, a szerveren át), `secrets.get` (csak a saját, bejelentett titkai), `store.*` (2.6), `colony.*` (a mag eszközei a létrehozó jogaival), `log`. Nincs fájlrendszer, nincs folyamat, nincs nyers socket.

A kód **külön szerepkörben** fut (`ext-runner`, döntés: 0. pont, 10.): ugyanaz a bináris, külön folyamat (Compose-ban külön szolgáltatás ugyanabból az image-ből, saját CPU- és memóriakorláttal), a szerverrel belső, hitelesített csatornán beszél. Hosztolt ügyfélnél sem kell hozzá őr. Ha az `ext-runner` nem fut, a saját kódos eszközök hibát adnak („a futtató nem érhető el”), minden más működik. A telepítő és a `deploy/compose.yaml` felveszi a szolgáltatást. Ez az első szerepkör a `COLONY_ROLE=all` mellett: a szerepkör-választás csak erre a folyamatra kell.

### 2.4 Rutinok (automatizálás)
**Indító** (cron, Colonyhall-esemény, bejövő webhook saját titkos URL-lel) + **agent** (prompt, engedélyezett eszközök) + **kimenet** (csatorna, DM, szál). Kétféle futás:
- **agentes:** az agent felébred a prompttal (őrön: saját gépen vagy a hosztolt őrön, 51c);
- **agent nélküli:** egy saját eszköz fut közvetlenül (olcsó, nincs token; pl. „minden reggel küldd el a Jira-szűrő eredményét”).

Példa (Norbi): hétköznap 8:00 → Jira-kapcsolat + `colony.send` → `#napi-feladatok`, mindenkinek a saját jegyei. A mai `/schedule` és `/cron` ennek egyszerű esetei lesznek.

### 2.5 Interaktív üzenetek
Az agent vagy egy eszköz **blokkokat** küldhet: szöveg, gombok, választó, űrlap, táblázat, kép. A kattintás vagy a beküldés eseményként visszamegy a küldő agenthez vagy egy saját eszközhöz (ki kattintott, mit). Ezzel felületi kód nélkül lesz szavazás, jóváhagyási folyamat, GIF-választó. **Később:** sandboxolt iframe-widgetek (`sandbox` attribútum `allow-same-origin` nélkül, `postMessage`-API, szigorú CSP, hálózat csak a szerveren át), ha a blokkok nem elegek.

### 2.6 Bővítmény-adattár
Bővítményenként és munkaterületenként JSON-gyűjtemények (dokumentum = kulcs + JSON), egyszerű lekérdezéssel, a munkaterület tárhelykeretébe számolva. Saját adatbázissémát a bővítmény nem kap.

### 2.7 Az Építő (a Műhely menü)
Beépített rendszeragent, minden munkaterületen egy; a **Műhely** menüpontból érhető el, csak a tulajdonosnak és az adminoknak. Meta-eszközei: `connector_create`, `tool_create`, `tool_update`, `tool_test`, `routine_create`, `channel_create`, `extension_publish`, `extension_rollback`. A felhasználó beszélget vele („szeretnék minden reggel Jira-összefoglalót”); az Építő visszakérdez, összerakja, próbafuttat egy próbacsatornába, megmutatja a kért jogokat, és **a jóváhagyás után** élesít. Az ügyfél saját API-kulcsával fut (0. pont, 5.). Saját telepítésen a felhasználó választja ki, melyik őrön és klienssel; hosztolt ügyfélnél a hosztolt őrön (51c).

## 3. Bővítmény = manifest + részek

```yaml
id: jira-napi
version: 3
title: Napi Jira-feladatok
connectors: [jira]            # amit használ
secrets: [jira_token]
egress: [acme.atlassian.net]  # kód fetch-e csak ide
tools:
  - name: my_issues           # → ext.jira-napi.my_issues
    kind: http | js
    risk: read | write | external
routines:
  - trigger: {cron: "0 8 * * 1-5"}
    agent: napi-osszefoglalo
    output: "#napi-feladatok"
store: [sent]
```

## 4. Életút, nyom és tudástár

- **Állapotok:** vázlat → teszt (próbafuttatás próbacsatornába, a mellékhatással járó hívások megjelölve) → jóváhagyás → éles → kikapcsolva. Minden módosítás új verzió; bármelyikre vissza lehet állni egy kattintással.
- **Jóváhagyás:** a tulajdonos vagy az admin látja a manifest változását (új jog, új cím, új titok); jogbővítés mindig jóváhagyást kér.
- **Önvédelem:** ismételt hibánál (pl. 5 hiba 10 percen belül) a bővítmény magától kikapcsol és értesít; a vészleállító a bővítményeket is megállítja.
- **Építési napló (a tudástár egysége):** minden bővítményhez automatikus wikioldal: mit csinál, miért, ki kérte, melyik agent építette, a döntések, a jogok, a verziók a teszteredményekkel, a hibák. A tudástár a munkaterület wikije; az Építő ebből tanul, mielőtt újat épít.
- **Hordozhatóság:** export egyetlen csomagba (manifest + kód + építési napló, titkok nélkül), import másik munkaterületre. Később ebből katalógus.

## 5. A mag: amihez a felhasználó nem nyúlhat

Belépés és munkamenetek; a szerepkör- és jogosultságmodell; a bérlők elkülönítése (RLS, `docs/23`); az auditnapló; csomagok, kvóta, számlázás; a vészleállító; a break-glass; az adatbázisséma és a migrációk; az őr sandboxa. A bővítmény ezeket csak olvasni sem tudja közvetlenül, csak a v1 API-n át.

**Alapszabály: egy bővítménynek soha nem lehet több joga, mint a létrehozójának.** Ha a létrehozó jogot veszít vagy kilép, a bővítményei felfüggesztődnek, amíg egy admin át nem veszi őket.

A v1 bővítési API verziózott és visszafelé kompatibilis; szerződéstesztek őrzik, hogy egy magfrissítés nem töri el a bővítményeket.

## 6. Biztonság

- **Prompt injection:** a kapcsolatokból jövő tartalom (Jira-jegy, levél) adat, nem utasítás (11. szabály). Az eszközök kockázati jelölése: `read`, `write`, `external` (adat kifelé). A `write` és `external` hívás alapból jóváhagyást kér az agentes futásban; a tulajdonos bővítményenként kikapcsolhatja (a szabadság megmarad, csak tudatosan; a kikapcsolás auditba kerül).
- **Hálózat:** a kód csak a szerveren át, csak a bejelentett címekre ér ki; belső címek (LAN, Tailscale, a szerver szolgáltatásai, metaadat-címek) tiltva, mint a `web_read`-nél. Ez megtartja az adatvédelmi ígéretet: a szerver csak oda kapcsolódik, amit a munkaterület bejelentett.
- **Erőforrás:** hívásonkénti memória- és időkorlát, munkaterületenkénti párhuzamosság és havi hívásszám a csomagból.
- **Titkok:** csak hivatkozással; naplóba, chatbe, a modellnek soha.
- **Többbérlős mód:** a bővítmény minden adata `workspace_id`-s, RLS alatt; a kódfuttató a hívó munkaterület hatókörében fut.

## 7. Megbízott építő és a fő projekt

- **Megbízott építő** szerepkör (Norbi vagy az operátor adja): a bővítményoldalon „Javaslom a magba” gomb → issue vagy PR-vázlat a GitHubra a csomagból és az építési naplóból.
- A PR a szokott úton megy: CLA (`CLA.md`), `make ci`, review. A bővítményből a magban rendes modul lesz (`internal/modules/<modul>`, `web/src/features/<név>`); a bővítmény ezután kikapcsolható.

## 8. Hol fut mi

| Rész | Saját telepítés | Hosztolt ügyfél |
| --- | --- | --- |
| MCP-átjáró, kapcsolatok (HTTP), deklaratív és JS-eszközök, agent nélküli rutinok, adattár | szerver | szerver |
| Építő és agentes rutinok | a felhasználó őre | **a hosztolt őr (51c)** |
| stdio-s MCP-szerverek | a felhasználó őre | a hosztolt őr sandboxa (51c) |
| Tokenek | a felhasználó kliense vagy API-kulcsa | **az ügyfél saját API-kulcsa** |

## 9. Fázisok (javaslat)

| Rész (56. kör) | Tartalom |
| --- | --- |
| **56a** | MCP-átjáró: a mag eszközeinek kiegészítése (ember = agent elv), kapcsolatok, titkok (`workspace_secret` a 24. dokumentumból, előrehozva), agentenkénti engedélyek, kockázati jelölés, hívásnapló |
| **56b** | Saját eszközök: deklaratív HTTP és QuickJS/WASM futtató az `ext-runner` szerepkörben a host-függvényekkel, adattár, manifest, verziók, jóváhagyás, önvédelem |
| **56c** | Rutinok (cron, esemény, webhook; agentes és agent nélküli) és interaktív üzenetek |
| **56d** | Az Építő és a **Műhely** menü, építési napló a wikiben, export/import |
| Később | iframe-widgetek, katalógus, megbízott építő és „Javaslom a magba”; hosztolt ügyfélnél az agentes részek az 51c-vel |

Az 56a–d saját telepítésen önmagában működik; a hosztolt ügyfelek agentes részeihez az 51c kell.

## 10. Példák

- **Napi Jira:** 2.4 példája.
- **GIF-küldés:** kapcsolat a Tenorhoz (az ügyfél kulcsával), JS-eszköz `gif_search`, `/gif` perjel-parancs, interaktív üzenet választóval; a kiválasztott GIF a tárba másolódik (kvóta), a böngésző nem tölt külső képet.
- **Számla-figyelő:** webhook a könyvelőprogramból → agent nélküli rutin → üzenet a `#penzugy`-be gombbal („Rendben”, „Kérdezek”).

## 11. Kapcsolódás a meglévőhöz

- `internal/modules/*/mcptools`, eszközprofilok (02. dokumentum) → az átjáró első forrása.
- `secretbox`, `workspace_secret` (51a, 24. dokumentum 2.3) → kapcsolatok titkai.
- `schedule` modul (`/schedule`, `/cron`) → rutinok.
- `web_read` (őr) → a hálózati tiltólista mintája.
- `wiki` → építési napló; `audit` → hívásnapló és jóváhagyások.

## 12. A kapu döntései (Norbi, 2026-11-17)

Mind eldőlt (0. pont, 7–11.): bővítményt a tulajdonos és az adminok hoznak létre; az író és kifelé küldő hívások alapból jóváhagyást kérnek, bővítményenként kikapcsolható; a menü neve **Műhely**; a JS-futtató rögtön külön szerepkörben (`ext-runner`); **56. kör, az 51c előtt**. A pontos korlátok (memória, idő, hívásszám), a végpontok és a táblák a kör promptjában.
