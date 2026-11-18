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

## 13. Részletes terv (az 56. kör kapuja, 2026-11-17)

Ez a pont a fentieket teszi megépíthetővé. Ahol eltér a 2–9. ponttól, ez az érvényes (az eltérés indoka mellette).

### 13.1 Modulok

| Modul | Felelősség |
| --- | --- |
| `internal/modules/secrets` (új) | `workspace_secret`: titkosított titkok (`secretbox`), csak hivatkozással; a 24. dokumentum 2.3 táblája, előrehozva. Az 51c projektkulcsai és AI-kulcsai is ezt használják |
| `internal/modules/workshop` (új) | Kapcsolatok, bővítmények és verzióik, eszközök, rutinok, adattár, engedélyek, hívásnapló, a gateway eszközlistája és hívásirányítása |
| `internal/extrunner` (új) | Az `ext-runner` szerepkör: QuickJS WASM-ban, `wazero`-val; adatbázis és titok nélkül |
| `internal/platform/egress` (új) | Kimenő HTTP csak publikus címekre (a `planner/fetch.Safe` mintája kiemelve és általánosítva: DNS-rebinding ellen tárcsázáskor ellenőrzött IP, minden átirányítás újraellenőrizve, méret- és időkorlát) |
| `conversation` (bővül) | Interaktív blokkok az üzenetben, `message_action` |
| `internal/mcpserver` (bővül) | A munkaterület eszközeinek regisztrálása a mag eszközei mellé |

### 13.2 Adatmodell (migráció `00071`; minden tábla `workspace_id`, `ENABLE` + `FORCE RLS`, `tenant` szabály)

| Tábla | Lényeges oszlopok | Megjegyzés |
| --- | --- | --- |
| `workspace_secret` | a 24. dokumentum 2.3 szerint; a `kind` bővül: `connector_token`, `connector_header`, `connector_basic` | Soha nem olvasható vissza egészben; `hint` = utolsó 4 karakter |
| `connector` | `id`, `slug` (`[a-z][a-z0-9_]{1,15}`, egyedi az élők közt), `title`, `kind` (`mcp`, `http`), `base_url` (csak `https`, kivéve a 13.7 engedélylistáját), `auth` (`none`, `bearer`, `header`, `basic`), `header_name`, `secret_id`, `state` (`ok`, `error`), `last_error_id`, `tools_cache` (jsonb: az MCP-szerver eszközlistája, kockázattal), `tools_cached_at`, `created_by`, `created_at`, `deleted_at` | |
| `extension` | `id`, `slug` (`[a-z][a-z0-9_]{1,15}`), `title`, `maintainer_id` (a létrehozó; tulajdonos vagy admin), `bot_id` (13.9), `state` (`draft`, `live`, `disabled`, `suspended`), `live_version`, `approval_off` (bool; a tulajdonos kapcsolja, auditálva), `disabled_reason`, `error_window` (jsonb: utolsó hibák), `created_at`, `updated_at`, `deleted_at` | Mutálható állapotsor |
| `extension_version` | `id`, `extension_id`, `version` (1, 2, …), `manifest` (jsonb), `source` (TS/JS forrás), `bundle` (a lefordított JS), `bundle_sha256`, `created_by` (ember vagy agent), `test_result` (jsonb), `approved_by`, `approved_at`, `created_at` | **Append-only** (4. szabály); a jóváhagyás egy külön `extension_approval` sor, nem módosítás |
| `extension_approval` | `id`, `version_id`, `decided_by`, `decision`, `diff` (jsonb: új jog, új cím, új titok, új kapcsolat), `created_at` | Append-only |
| `agent_tool_grant` | `agent_id`, `target_kind` (`connector`, `extension`, `workshop`), `target_id`, `granted_by`, `created_at`, `revoked_at` | Alapból egy agent csak a mag eszközeit látja (mint ma) |
| `ext_store_doc` | `extension_id`, `collection`, `doc_id`, `data` (jsonb), `size_bytes`, `updated_at` | Mutálható; a tárhelykeretbe számít (új fajta: `extensions`) |
| `routine_state` | `extension_id`, `routine` (név a manifestből), `next_run_at`, `last_run_at`, `last_error_id`, `webhook_token_hash`, `run_count` | A manifest a forrás, ez csak az állapot |
| `tool_call` | `id`, `actor_id` (hívó agent vagy a bővítmény botja), `target_kind`, `target_id`, `tool`, `risk`, `approval_id`, `started_at`, `duration_ms`, `in_bytes`, `out_bytes`, `outcome` (`ok`, `error`, `denied`, `timeout`, `limit`), `error_id` | **Append-only**; tartalom nélkül. Havonta összesítve a csomag híváskeretéhez |
| `message_action` | `id`, `message_id`, `actor_id`, `action_id`, `value` (jsonb, max 4 KB), `created_at` | Append-only |

A `message` táblában új oszlop: `blocks` (jsonb, max 16 KB, NULL = sima üzenet); a szöveges `body` mindig marad (értesítés, keresés, a fejlesztői oldal és az agentek ezt olvassák). A `plan` és a `workspace_storage` másolata új oszlopot kap: `ext_calls_month` (NULL = korlátlan).

### 13.3 Az MCP-átjáró

- **Nevek.** A mag eszközei a mai nevükön maradnak (`send`, `read`, …), és **soha nem tartalmaznak `__`-t**: ez a névtér a magé. Kapcsolat eszköze: `<kapcsolat>__<eszköz>` (pl. `jira__search`); bővítményé: `x_<bővítmény>__<eszköz>` (pl. `x_gif__search`). Hossz: a kliens előtagjával (`mcp__colonyhall__`) együtt legfeljebb 64 karakter (a Claude API korlátja), ezért a slug legfeljebb 16, az eszköznév legfeljebb 24 karakter. *(Eltérés a 2.1-től: `.` helyett `__`, mert a Claude API eszközneve nem tartalmazhat pontot.)*
- **Regisztrálás.** A `/mcp` munkamenet indulásakor a mag eszközei mellé bekerül minden kapcsolat- és bővítményeszköz, **amire az agentnek engedélye van** (`agent_tool_grant`) és ami él. Egy agentfutás egy munkamenet, így a lista a futás elején friss; változáskor a go-sdk `tools/list_changed` értesítést küld a nyitott munkamenetekbe.
- **Hívás.** Minden kapcsolat- és bővítményhívás egy úton megy (`workshop.Service.Call`): a hívó friss állapota (vészleállítás, szünet, felfüggesztés → elutasítás) → az engedély újraellenőrzése → a kockázat → jóváhagyás, ha kell (13.6) → a havi keret → végrehajtás időkorláttal → `tool_call` sor → a válasz tömörítve (max 32 KB a modellnek, a végén „+N bájt levágva”).
- **Külső MCP.** A szerver MCP-kliensként (go-sdk, streamable HTTP) kapcsolódik a kapcsolat címéhez, a titkot fejlécként a szerver teszi rá az `egress` kliensen át. Az eszközlista 10 percig gyorsítótárazott, a Műhelyben frissíthető. A kockázat a szerver annotációiból jön (`readOnlyHint` → `read`, `openWorldHint` → `external`, különben `write`); hiányzó annotáció = `write`; az admin eszközönként felülírhatja (auditálva).
- **stdio-s MCP** csak őrön: ebben a körben **nem építjük** (az őrön a felhasználó saját MCP-szerverei ma is működnek a teljes hozzáférésű agenteknél; a hosztolt változat 51c).
- **„Ember = agent” hiányzó mag-eszközök:** `channel_create`, `channel_members` (hozzáadás/eltávolítás, a REST jogaival), `wiki_write`, `task_create`/`task_update` a tervezőben (ha még nincs), `pin`; a Műhely meta-eszközei (13.10). Mind a REST-végpontok szolgáltatásfüggvényeit hívja, ugyanazzal a jogellenőrzéssel.

### 13.4 Saját eszközök

**Deklaratív HTTP-eszköz** (a manifestben):

```yaml
tools:
  - name: my_issues
    kind: http
    connector: jira            # a kapcsolat adja az alap-URL-t és a titkot
    risk: read
    input: {type: object, properties: {user: {type: string}}, required: [user]}
    request:
      method: GET
      path: /rest/api/3/search
      query: {jql: "assignee = \"{{input.user}}\" AND statusCategory != Done"}
    output:
      pick: {key: "issues[].key", summary: "issues[].fields.summary"}
      max_items: 20
```

A sablon csak `{{input.x}}` helyettesítést ismer (URL-ben kódolva, JSON-törzsben JSON-ként), más kódot nem. Útvonalban egy bemenet egyetlen szegmens (a perjel is kódolva); több szegmenshez (pl. `owner/repo`) kifejezetten `{{input.x|path}}` kell, és `.`, `..` vagy üres szegmens ekkor sem lehet (terepi teszt és biztonsági átnézés, 2026-11-18). A kimenet egy egyszerű JSON-útvonal-részhalmaz.

**Kódeszköz:** `kind: js`, a forrás TS vagy JS. Mentéskor a szerver **esbuild-del** (Go-könyvtár, Node nélkül) egy fájlba fordítja (`bundle`), importok nélkül. Az eszköz egy exportált függvény: `export async function my_issues(input, ctx) { … }`.

**Host-függvények** (`ctx`):

| Függvény | Mit tud |
| --- | --- |
| `ctx.fetch(url, {method, headers, body, auth})` | Csak a manifest `egress` címeire, a szerver `egress` kliensén át. **`auth: "jira"`**: a szerver a kapcsolat titkát teszi a kérésre. *(Eltérés a 2.3-tól: nincs `secrets.get`; a titok soha nem jut be a JS-be, így a kód sem szivárogtathatja.)* |
| `ctx.store.get/put/delete/list(collection, …)` | A bővítmény saját adattára (13.2) |
| `ctx.colony.<eszköz>(input)` | A mag MCP-eszközei, **a hívó jogaival, de csak ha a karbantartó is megtehetné** (a kettő metszete; agent nélküli rutinban a bővítmény botjával) |
| `ctx.call(<eszköz>, input)` | Egy kapcsolat vagy a bővítmény másik eszköze (ugyanazon az úton, mint az átjáró) |
| `ctx.log(...)` | A futás naplójába (embernek, a Műhelyben) |

**Korlátok (javaslat):** hívásonként 64 MB memória, 10 mp falióra (megszakítással), 256 KB kimenet, 50 host-hívás, `fetch` válasz 5 MB; munkaterületenként 4 párhuzamos hívás, a futtatón összesen 16; havi hívásszám a csomagból (`ext_calls_month`). Önvédelem: 5 egymást követő hiba vagy 10 hiba 10 percen belül → `disabled`, értesítés a karbantartónak.

### 13.5 Az `ext-runner`

- **Ugyanaz a bináris**, `COLONY_ROLE=ext-runner` (a `config` csak ezt engedi az `all` mellett). Nincs adatbázis-kapcsolata, nincs titkosítási kulcsa, nem lát fájlt; csak a szerver felé beszél.
- **Protokoll:** a szerver `POST http://ext-runner:8095/v1/run` (belső Compose-hálózat), fejlécben `COLONY_EXT_RUNNER_TOKEN` (a telepítő generálja; mindkét folyamat megkapja). Kérés: `call_id`, a `bundle`, a függvény neve, a bemenet, a korlátok, egy **hívásonkénti host-token**. A futtató a host-függvényeket a szerver belső végpontján hívja vissza: `POST /internal/ext/host` a host-tokennel (csak a hívás idejére érvényes, a szerveren a hívás munkaterületéhez és hívójához kötve; csak a belső hálózaton, a nyilvános útválasztóban nincs). Megszakítás: a szerver bontja a kérést → a futtató a `wazero` kontextusával leállítja a modult.
- **Elkülönítés:** WASM-sandbox (nincs fájlrendszer, hálózat, óra a host-függvényeken kívül), friss modulpéldány hívásonként, külön folyamat; Compose-ban `read_only`, `cap_drop: ALL`, `no-new-privileges`, `mem_limit` 1 GB, `cpus` 2, csak a belső hálózat.
- **Ha nem fut** (vagy `COLONY_EXT_RUNNER_URL` üres): a JS-eszközök hibája „a kódfuttató nem érhető el”; a deklaratív eszközök, a kapcsolatok és minden más megy. A `/readyz` nem bukik el miatta; a Rendszerállapot oldal mutatja.
- **Telepítés:** `deploy/compose.yaml`, `compose.dev.yaml` és a telepítő compose-a új `ext-runner` szolgáltatást kap ugyanabból az image-ből; `colony setup --server` generálja a tokent. Tesztekben a futtató folyamaton belül indul ugyanazon az interfészen (`httptest`).
- **Könyvtár:** `github.com/fastschema/qjs` (QuickJS-NG WASM-ra fordítva, `wazero`-n fut; MIT). Fiatal (v0.0.x), ezért egy saját port (`extrunner.Engine`) mögött, hogy cserélhető legyen; a verziója rögzítve.

### 13.6 Jóváhagyás

- A `write` és `external` kockázatú kapcsolat- és bővítményhívás **alapból jóváhagyást kér**: a meglévő `approval` modul (`CreateRequest` + `WaitForDecision`, a jóváhagyási kártya a beszélgetésben, 10 perc). A kártyán: bővítmény, eszköz, kockázat, a bemenet (max 8000 karakter, ahogy ma).
- Ki dönthet: a hívó agent tulajdonosa, vagy admin (a mai szabály).
- **Agent nélküli rutin** `write`/`external` hívása: a jóváhagyási kártya a rutin kimeneti beszélgetésébe kerül, a futás addig vár (legfeljebb 10 perc, utána `timeout`).
- **`approval_off`:** a tulajdonos bővítményenként vagy kapcsolatonként kikapcsolhatja; auditálva, a Műhelyben jól látható jelzéssel. A mag eszközei (`send` stb.) nem kérnek jóváhagyást (mint ma).

### 13.7 Kimenő forgalom (`egress`)

Csak `https`, csak publikus IP (a `planner.PublicIP` szabálya: nincs loopback, magáncím, link-local, CGNAT/Tailscale, metaadat-cím, a szerver saját címei), tárcsázáskor ellenőrizve (DNS-rebinding ellen), minden átirányítás újra; kódeszköznél ráadásul csak a manifest `egress` hosztjaira. **Saját telepítésen** a szerver üzemeltetője engedhet belső címeket: `COLONY_EGRESS_ALLOW` (CIDR-ek és hosztok vesszővel, alapból üres); **többbérlős módban ez tiltott** (indításkor hiba).

### 13.8 Rutinok

- **Indítók:** `cron` (a `schedule` modul elemzője és a karbantartó ciklus, időzónával, a kihagyott futások `once`/`skip` szabályával); `event` (ebben a körben: `message` egy csatornában szűrővel — szerző-fajta, minta —, és `action`: kattintás a bővítmény üzenetén); `webhook` (`POST /api/v1/hooks/ext/{routineId}/{token}`, a token csak hash-elve tárolva, a törzs max 256 KB, adatként kerül tovább, 11. szabály).
- **Agentes futás:** a rutin a kimeneti beszélgetésbe üzenetet ír **a bővítmény botjának nevében** (13.9), megemlítve az agentet, a prompttal; az agent a mai úton ébred (őr, kézbesítés). A „ki adhat feladatot kinek” szabály a botra is érvényes: teljes hozzáférésű agentnek csak a jóváhagyási úton.
- **Agent nélküli futás:** egy eszköz fut közvetlenül; a kimenete (szöveg vagy blokkok) a bot nevében megy ki.
- A mai `/schedule` és `/cron` **változatlanul működik** (a saját táblájával); később átköltöztethető, ebben a körben nem.

### 13.9 A bővítmény botja (aki a kimenetet írja)

**Javaslat:** minden bővítmény kap egy saját bot-actort (`@x-gif`, fajtája `agent`, gép nélkül, így soha nem ébred; a karbantartó a tulajdonosa). Ő írja a rutinok és az agent nélküli eszközök üzeneteit, ő a csatornák tagja, a jogai ≤ a karbantartóé. Előny: látszik, melyik bővítmény írt, csatornánként szabályozható, megemlíthető, kikapcsoláskor némul. (Alternatíva: egy közös `@muhely` actor a munkaterületen — egyszerűbb, de nem látszik, ki írt.)

### 13.10 Az Építő

- **Nem rendszeractor**, hanem egy rendes agent, amit a tulajdonos vagy admin a Műhelyből hoz létre egy előre kitöltött varázslóval („Építő” sablon: leírás, profil, ajánlott sandboxos mód; a gépet, klienst és modellt ő választja). Így a 8. szabály (változatlan konfiguráció) érintetlen.
- **Meta-eszközök** (`workshop` engedéllyel, csak ha az agent tulajdonosa tulajdonos vagy admin): `workshop_list`, `connector_create` (titok nélkül: a titkot az ember adja meg a felületen; az eszköz egy „add meg a titkot” linket ad), `extension_draft` (manifest + forrás → új verzió), `extension_test` (próbafuttatás egy próbacsatornába, mellékhatással járó hívások jelölve), `extension_publish` (**jóváhagyási kérelem** a karbantartónak; élesíteni csak ember tud), `extension_rollback`, `channel_create`.
- **Építési napló:** minden verzió után a `wiki`-ben a `Műhely/<slug>` oldal frissül (mit csinál, miért, ki kérte, melyik agent építette, jogok, verziók és tesztek, hibák).
- **Export/import:** egy JSON-csomag (manifest, forrás, bundle, a napló), titkok nélkül; importálva vázlat lesz.
- A „megbízott építő” és a „Javaslom a magba” gomb: ha marad hely az 56d-ben.

### 13.11 Interaktív üzenetek

Blokkok: `text` (markdown), `buttons` (max 5, `action_id`, `label`, `style`), `select` (opciók), `form` (mezők: szöveg, szám, választó; max 10), `table` (max 20 sor), `image` (csak a Colonyhall tárában lévő `file_id`, külső kép soha). Az agent a `send` eszköz új `blocks` paraméterével küldheti, a bővítmény a kimenetével. Kattintás: `POST /api/v1/messages/{id}/actions` (csak aki olvashatja a beszélgetést; a gomb szűkíthet `allowed` listával) → `message_action` sor + `conversation.MessageActionTaken` esemény → ha a küldő agent, az inboxába adatként kerül (és felébred); ha bővítménybot, az `action` indítójú rutin fut. A PWA megjeleníti; a fejlesztői oldal a `body` szöveget mutatja.

### 13.12 Többbérlős mód, vészhozzáférés, felfüggesztés

Minden új tábla RLS alatt; a gateway és a host-végpont a hívó munkaterületének hatókörében fut; a futtató munkaterület-azonosítót csak a host-tokenen át „lát”. Felfüggesztett munkaterület: minden hívás és rutin áll (a mai hitelesítés). A vészhozzáférés a Műhelyt csak olvasva látja, titkot és a hívásnapló bemeneteit soha (azok nincsenek is tárolva). A cross-tenant e2e készlet lefedi az összes új végpontot és paramétert.

### 13.13 Tesztek

Hamis külső MCP-szerver (go-sdk, `httptest`) és hamis HTTP API (Jira, GIF) a tesztekben; a hamis agent forgatókönyvei az Építőhöz és az agentes rutinokhoz; **ellenséges JS-eszközök** (végtelen ciklus, memóriabomba, nagy kimenet, nem bejelentett cím, belső cím DNS-rebindinggel, más munkaterület adattára, a host-token újrahasználata a hívás után) — mindegyik egy teszt, ami nélküle elbukna; a futtató leállítása alatt a szerver működik; cross-tenant: kapcsolat, bővítmény, adattár, webhook, akció.

### 13.14 A részek

| Rész | Tartalom | Kész, ha |
| --- | --- | --- |
| **56a** | `secrets`, `egress`, `workshop` (kapcsolatok, engedélyek, hívásnapló), átjáró a `/mcp`-n, külső MCP és HTTP-kapcsolat, kockázat és jóváhagyás, `approval_off`, „ember = agent” eszközök, Műhely: Kapcsolatok, Engedélyek, Hívásnapló | Egy agent a titkot nem látva hív egy hamis külső MCP-eszközt; a `write` hívás jóváhagyásra vár; más munkaterület nem éri el |
| **56b** | Bővítmények, verziók, manifest, deklaratív HTTP-eszköz, `ext-runner` (QuickJS/WASM, host-függvények, korlátok), adattár, életút, önvédelem, Compose és telepítő; Műhely: Bővítmények | Egy JS-eszköz a korlátain belül fut; minden ellenséges eszköz megáll; a futtató nélkül a szerver működik |
| **56c** | Rutinok (cron, esemény, webhook; agentes és agent nélküli), bővítménybotok, interaktív blokkok és akciók, PWA-megjelenítés | A Jira-példa végig működik egy hamis Jira ellen; egy gombnyomás eljut a küldő agentig |
| **56d** | Az Építő sablon, meta-eszközök, próbafuttatás, jóváhagyási képernyő a manifest változásával, építési napló, export/import; ha marad hely: megbízott építő | A hamis agent mint Építő létrehozza, teszteli és (emberi jóváhagyás után) élesíti a GIF-példát |

### 13.15 A kapu döntései (Norbi, 2026-11-17)

| # | Kérdés | Döntés |
| --- | --- | --- |
| 1 | Ki írja a bővítmények kimenetét | **Minden bővítmény saját botot kap**, nyomon követhetően; **a bot nevét a létrehozáskor adják meg** (alapérték `x-<slug>`, a mag neveivel és a meglévő actorokkal nem ütközhet; később nem változik, hogy a nyom megmaradjon) |
| 2 | Belső címek saját telepítésen | **Igen:** `COLONY_EGRESS_ALLOW` (alapból üres, többbérlős módban tilos). Norbi ötlete: a bővítmény-„piactér” később **Giteában** él (bővítményenként egy repó, a csomag fájlokként); az export formátuma ezért egy repóba tehető mappaszerkezet (`manifest.yaml`, `src/`, `BUILDLOG.md`), nem egyetlen JSON. A piactér maga nem ennek a körnek a része |
| 3 | Titok a JS-ben | **Nem jut be**: a szerver teszi rá a kérésre (`auth: "<kapcsolat>"`) |
| 4 | QuickJS-könyvtár | `fastschema/qjs` **saját port mögött** (`extrunner.Engine`), rögzített verzióval; **a fejlődését követjük**: a `task.md`-ben ismétlődő tétel, és a `make vuln` mellé egy `make deps-watch`, ami kiírja, van-e újabb verzió |
| 5 | Havi híváskeret | `ext_calls_month` a csomagban; **NULL = korlátlan** (az alapérték, mint a tárhelynél) |
| 6 | Korlátszámok | A 13.4 értékei az alapértékek, finomhangolás később. **A szerver tulajdonosa állítja** a meglévő módszerrel: környezeti változók a szerver `.env`-jében (mint a `COLONY_MAX_UPLOAD_MB`): `COLONY_EXT_MEMORY_MB` (64), `COLONY_EXT_TIMEOUT` (10s), `COLONY_EXT_OUTPUT_KB` (256), `COLONY_EXT_HOST_CALLS` (50), `COLONY_EXT_CONCURRENCY_WORKSPACE` (4), `COLONY_EXT_CONCURRENCY_TOTAL` (16), `COLONY_EXT_DISABLE_AFTER` (`5/10/10m`: egymást követő hibák / hibák / ablak). Többbérlős módban a csomag felülírhatja (később, a csomagtábla oszlopaival); a Rendszerállapot oldal mutatja az érvényes értékeket |

## 14. Megvalósítás

### 14.1 56a: MCP-átjáró és kapcsolatok (2026-11-17)

**Kész:** `internal/platform/egress` (13.7), `internal/modules/secrets` (titkosított titkok, `rotate-secrets` minden szerveren), `internal/modules/workshop` (kapcsolatok, engedélyek, kockázat, jóváhagyás, hívásnapló, `mcpremote` a külső MCP-szerverekhez), az átjáró a `/mcp`-n (`<kapcsolat>__<eszköz>`, csak engedéllyel), REST (`/api/v1/workshop/*`), migráció `00071`, a PWA **Műhely** oldala (Kapcsolatok, Engedélyek, Hívásnapló), `COLONY_EGRESS_ALLOW`.

**Eltérések és pontosítások a 13. ponthoz képest:**
- **Csatorna létrehozása és tagkezelés agentnek** (13.3, „ember = agent”): ma a mag szándékosan tiltja (`policy.CanCreateChannel`, `CanManageMembers`). Ezt az 56a nem lazítja; az Építő meta-eszközeként jön az **56d**-ben, a `workshop` engedélyhez és a tulajdonos/admin gazdához kötve.
- **A csomag havi híváskerete** (`ext_calls_month`, 13.15/5.) az **56b**-be került, a futtató korlátaival együtt (a csomagtábla és az operátori felület egyszerre változik).
- **stdio-s MCP-szerver** nincs (13.3): saját telepítésen az őrön futó saját MCP-szerverek a teljes hozzáférésű agenteknél ma is működnek; a hosztolt változat az 51c.
- **Biztonsági átnézés után** (`f24f540`): kockázatot csökkenteni csak a tulajdonos tud (mint a jóváhagyás kikapcsolását; admin csak emelhet); ha a kapcsolat címe más szerverre (scheme + host) mutat, a titkot újra meg kell adni; a titok átirányításkor csak a kapcsolat saját hosztjára megy.
- A kapcsolat hibája az agentnek eszközhibaként (nem protokollhibaként) megy, hogy el tudja olvasni; jóváhagyás beszélgetés nélkül (`X-Colonyhall-Channel` hiányzik) nem kérhető, ilyenkor a hívás elutasítva.

### 14.2 56b: bővítmények és a kódfuttató (2026-11-17)

**Kész:** migráció `00072` (`extension`, `extension_version`, `extension_approval`, `ext_store_doc`; a csomag `ext_calls_month` mezője és másolata), `internal/extrunner` (QuickJS WASM-ban `wazero`-n, a `fastschema/qjs` v0.0.6 az `extrunner.Engine` mögött; a `qjs.wasm` a repóban, az ellenőrzőösszege tesztben rögzítve), az `ext-runner` szerepkör (`COLONY_ROLE=ext-runner`, HTTP-protokoll hívásonkénti host-tokennel, a szerver belső figyelője `COLONY_EXT_HOST_ADDR`), `workshop/compile` (esbuild Go-könyvtár: TS → IIFE, importok tiltva), a bővítmények életútja (létrehozás bottal, verzió, jóváhagyás eltérés-listával, visszaállás, ki/bekapcsolás, átvétel, törlés), deklaratív HTTP-eszközök (`{{input.x}}`, kimenet-leképezés), JS-eszközök host-függvényekkel (`fetch`, `store`, `colony`, `call`, `log`), önvédelem, havi híváskeret, `COLONY_EXT_*` korlátok, Compose `ext-runner` szolgáltatás a `workshop` profilban (a telepítő bekapcsolja, tokent generál), PWA: Műhely → **Bővítmények** fül, az operátor csomagszerkesztőjében a havi híváskeret.

**Mérések és döntések megvalósítás közben:**
- **Memória:** a QuickJS saját memóriakorlátja (`JS_SetMemoryLimit`) egy tömbökből álló tömböt nem állított meg (az időkorlátig futott), és a WASM-memória alapból 4 GB-ig nőhet. Ezért a futtató a `qjs.wasm` memóriaszakaszába kemény felső határt ír (`capMemory`), amit a `wazero` kényszerít ki: a malloc elbukik, a szkript „out of memory”-t kap. A határ folyamatonként rögzített (`COLONY_EXT_MEMORY_MB`).
- **Fájlrendszer:** a `qjs` a munkakönyvtárat gyökérként felcsatolja, és a `qjs:std`/`qjs:os` modulok elérhetők; ezért minden hívás egy friss, üres ideiglenes könyvtárat kap, amit utána töröl (teszt: `/etc/passwd` és a környezet nem olvasható, állapot nem marad meg a hívások közt). A konténer gyökere csak olvasható.
- **A titok nem jut be a JS-be** (13.15/3.): `ctx.fetch(url, {auth: "<kapcsolat>"})` esetén a szerver teszi rá a hitelesítést, és csak a kapcsolat saját címére; egyébként csak a manifest `egress` hosztjaira mehet kérés.
- **`ctx.colony` a hívó jogaival** fut, egy memórián belüli MCP-munkamenetben ugyanazokkal az eszközökkel és ellenőrzésekkel, amit az agent kap (a Műhely eszközei nélkül). Ami változtat (pl. `send`), az írónak számít: „olvas” kockázatú eszköz nem hívhatja.
- **Beágyazott hívás** (`ctx.call`): a belső eszköz kockázata nem lehet nagyobb a külsőénél (a külső jóváhagyása csak annyit fed le); legfeljebb 3 szint.
- **A karbantartó jogai:** ha a karbantartó már nem élő tulajdonos/admin, a bővítmény nem fut, amíg egy admin át nem veszi (`take-over`).
- **Tesztfuttatás:** a felületről egy verzió bármely eszköze futtatható a bővítmény botjaként (jóváhagyás nélkül, mert ember indította; a hívásnaplóban tesztként).

**Biztonsági átnézés után** (`12eac36`): az „olvas” eszköz nem hívhat író mag-eszközt; a tulajdonos `approval_off`-ja nem öröklődik egy olyan új verzióra, amit nem a tulajdonos élesített és ami újat ad; a jóváhagyásra várás után a hívás újraellenőrzi az engedélyt, az élő verziót, a karbantartót és az agentet (a kapcsolatoknál is).

**Hátra (56c–d):** rutinok és interaktív üzenetek; az Építő és a Műhely meta-eszközei (köztük a csatorna létrehozása agentnek, 14.1).

### 14.3 56c: rutinok és interaktív üzenetek (2026-11-17)

**Kész:** migráció `00073` (`message.blocks`, `message_action`, `routine_state`, `routine_run`), a manifest `routines` mezője, az ütemező és a futtatósor (`RoutineLoop`, 5 mp; a futás tranzakción kívül, a hívás szokott útján: hely, keret, jóváhagyás, napló, önvédelem), a `workshop` eseményfogyasztó (üzenet és gombnyomás), a webhook (`POST /api/v1/hooks/ext/{munkaterület}/{rutin}/{token}`), interaktív blokkok (`text`, `buttons`, `select`, `table`) az agentek `send` eszközében és a botok kimenetében, `POST /api/v1/messages/{id}/actions`, a PWA-ban a blokkok megjelenítése és a Műhelyben a rutinok (következő futás, webhook-cím, utolsó futások).

**Rutin a manifestben:**

```json
{"name": "daily", "trigger": {"cron": "0 8 * * 1-5", "timezone": "Europe/Budapest"}, "tool": "mine", "output": "napi-feladatok"}
{"name": "standup", "trigger": {"cron": "30 8 * * 1-5"}, "agent": "helper", "prompt": "Foglald össze…", "output": "napi-feladatok"}
{"name": "invoice", "trigger": {"webhook": true}, "tool": "poll", "output": "penzugy"}
{"name": "asked", "trigger": {"message": {"channel": "napi-feladatok", "contains": "jira?"}}, "tool": "mine"}
{"name": "clicked", "trigger": {"action": "ok"}, "tool": "thanks"}
```

- **Eszközös rutin:** az eszköz a bővítmény botjaként fut; a bemenete a rutin `input`-ja és egy `trigger` mező (mi indította: üzenet szövege és szerzője, a webhook törzse adatként, a gomb azonosítója és a kattintó). A válaszát a bot írja ki; egy `{"text", "blocks"}` objektumból interaktív üzenet lesz. Üzenet- és gombnyomás-indítónál kimeneti csatorna nélkül a kiváltó szálban válaszol.
- **Agentes rutin:** a bot a kimeneti csatornában megszólítja az agentet a prompttal (az indító adatai kódblokkban, adatként). A „ki adhat feladatot kinek” szabálya a botra is vonatkozik (a bot agent).
- **Élesítéskor** a rutinok ütemezése frissül, és a bot (a jóváhagyó admin nevében) bekerül a kimeneti és a figyelt csatornákba; ha egy csatorna nem létezik, a jóváhagyás hibát ad. Egy új vagy változott rutin megjelenik a jóváhagyás előtti eltérés-listában.
- **Webhook:** a cím a Műhelyben kérhető, egyszer látszik, csak hash-elve tárolódik; új cím kérésekor a régi megszűnik. A cím a munkaterületet is tartalmazza, így a kérés rendszermód nélkül szűkít; minden hiba ugyanaz a 404; a törzs legfeljebb 256 KB.
- **Gombnyomás:** csak ember, és csak írási joggal a beszélgetésben. Egy agent üzenetén a kattintó nevében válasz megy a szálba, ami megszólítja az agentet (így az a szokott úton ébred); egy bővítmény botjának üzenetén a bővítmény gombnyomás-rutinja fut.

**Biztonsági átnézés után** (`2c440ed`): gomb- és opciócímkében nem lehet `@` (a kattintás senki mást nem szólíthat meg); üzenet-rutin csak ott figyel, ahova a bot jóváhagyással bekerült; a webhook-cím megszűnik a rutinnal, és a bővítmény kikapcsolásával vagy törlésével; a gombnyomáshoz írási jog kell.

**Hátra (56d):** az Építő és a Műhely meta-eszközei (köztük a csatorna létrehozása agentnek, 14.1), az építési napló a wikiben, export/import.

### 14.4 56d: az Építő, építési napló, export/import (2026-11-17)

**Kész:** az Építő meta-eszközei (`workshop/mcptools/builder.go`), az építési napló a wikiben, export/import (REST), a PWA-ban a Műhely **Építő** fülje (engedély adása, javasolt utasítás) és a Bővítmények fülön export/import.

- **Ki az Építő:** bármely agent, aki `workshop` engedélyt kapott, és akinek a gazdája élő tulajdonos vagy admin (13.10: nem rendszeractor; a 8. szabály érintetlen). Ha a gazda elveszti a jogát, az agent elveszti a meta-eszközöket.
- **Meta-eszközök** (mind a gazda nevében, annál többet nem tehetnek): `workshop_list`, `extension_create`, `extension_draft` (a verzió szerzője az Építő, a nyom kedvéért), `extension_get`, `extension_test` (a bővítmény botjaként), `channel_create` (a gazda hozza létre, az Építő tag lesz; agent maga továbbra sem hozhat létre csatornát — 14.1 döntése így teljesül), `extension_publish` és `extension_rollback`: **jóváhagyási kártya** a beszélgetésében azzal, amit a verzió hozzáad; a hívás vár, és **aki engedélyezi, az hagyja jóvá** (csak tulajdonos/admin). Kapcsolatot és titkot továbbra is csak ember vesz fel.
- **Építési napló:** minden verzió, jóváhagyás és visszaállás után a karbantartó nevében frissül a `Műhely – <slug>` wikioldal: mit csinál, bot, karbantartó, eszközök, rutinok, kapcsolatok/kifelé/adattár, a verziók szerzővel és jóváhagyóval, és hogy melyik jóváhagyás mit adott hozzá.
- **Export/import:** `GET …/export` fájlokként (`manifest.json`, `src/extension.ts`, `BUILDLOG.md`; titok soha) — egy Gitea-repóba tehető mappa (13.15/2.); `POST /workshop/extensions/import` vázlatként hozza létre (más rövid névvel és bottal is), jóváhagyni a szokott módon kell; a hivatkozott kapcsolatoknak itt is létezniük kell.
- **Nem készült el (később):** a „megbízott építő” szerepkör és a „Javaslom a magba” gomb (13.10), a bővítmény-katalógus Giteában.
