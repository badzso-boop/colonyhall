# Több ügyfél egy rendszeren (többbérlős működés): terv

> Állapot: **terv, 2026-11-11**, Norbi döntéseivel (lent). A homelab csak **tesztkörnyezet**; piacra vitelnél bérelt, erősebb szerver jön.
>
> **A 0. fázis (elkülönítés) kész (45. kör, 2026-11-11):** a leltár, a döntések és az eltérések a `23-elkulonites.md`-ben; a működés a `02-muszaki-specifikacio.md` „Többbérlős működés” fejezetében.

## 0. Döntések (Norbi, 2026-11-11)

| Kérdés | Döntés |
|---|---|
| Modell | Egy közös rendszer: közös adatbázis, több egyforma szerverpéldány; **nem** ügyfelenként külön konténer |
| Elérés | **Aldomain ügyfelenként** (`acme.<domain>`); a domain még nincs kiválasztva |
| Guardok | **Első körben az ügyfél saját gépén.** Később a mi szerverünkön is, ügyfelenként elkülönítve, **csak API-kulccsal** (nem bejelentkezett előfizetéssel) |
| Modellköltség | **Az ügyfélé**: saját előfizetés a saját gépen, API-kulcs a mi szerverünkön (a plusz költséget az ügyfél vállalja). Később prémium: OpenRouter-hívás, továbbszámlázva |
| Busz a példányok között | **Postgres `LISTEN/NOTIFY`** + az eseménynapló sorszáma; külső szolgáltatás nem kell |
| Operátori hozzáférés | **Fejlesztői módban teljes rálátás, és ezt a felület mindig kiírja**; élesben csak **naplózott vészhozzáférés**, ami az ügyfél auditnaplójában is látszik |
| Hol fut | Tesztelés a homelabon; éles: később bérelt szerver |

## 1. Cél, és ami nem cél

- **Cél:** több cég (ügyfél) ugyanazt a Colonyhall-telepítést használja, egymástól teljesen elválasztva; az ügyfél saját adminja kezeli a saját felhasználóit, csatornáit, wikijét, agentjeit és gépeit; a platform üzemeltetője (Norbi) külön operátori fiókkal mindent kezel.
- **Nem cél (most):** több adatközpont, elosztott adatbázis, ügyfelenként külön adatbázis vagy telepítés, saját fizetési rendszer (kész szolgáltatót használunk).
- **Az önálló telepítés megmarad:** aki AGPL alatt a saját gépén futtatja, annak a többbérlős réteg ki van kapcsolva (`COLONY_MULTITENANT=0`, alapértelmezés), és minden úgy működik, mint ma.

## 2. Fogalmak

- **Ügyfél** = egy munkaterület (`workspace`), saját aldomainnel (`slug`), csomaggal és állapottal.
- **Ügyfél-tulajdonos** (`owner`): a cég „szerver managere”; a mai `owner` szerepkör.
- **Operátor:** a platform üzemeltetője. **Nem** munkaterület tagja, külön fiók, külön cím (`admin.<domain>`), kötelező kétfaktoros belépés.
- **Fejlesztői mód:** teszteléshez; az operátor mindent lát, a felület ezt minden oldalon kiírja.

## 3. Ami már megvan a kódban

- 54 táblából 39-ben van `workspace_id`; a többi (15) egy olyan sorhoz kötődik, amiben van (pl. `reaction` → `message`, `session` → `actor`), vagy globális (`workspace`, `consumer_offset`).
- A szerepkörök munkaterületenként: `owner`, `admin`, `member`, `observer` (`internal/modules/identity/model.go`).
- A felhasználónév munkaterületen belül egyedi (`UNIQUE (workspace_id, handle)`); a belépés már kezeli, ha egy név több munkaterületen is van (`ErrWorkspaceRequired`).
- A busz már most „csak sorszámot jelző” (`internal/platform/bus`, Dendrite-minta): a fogadó maga olvassa az eseménynaplót, így egy elveszett jelzés nem okoz adatvesztést. A Postgres NOTIFY egy új adapter ehhez a porthoz.
- Az eseményfogyasztók tárolt pozícióból folytatnak (`consumer_offset`), és ha nincs jelzés, időnként maguk is lekérdeznek (`Fallback`, 5 mp).
- A WebSocket-szétosztás már munkaterület szerint szűr (`internal/gateway`).

## 4. Elérés: aldomain

- `acme.<domain>` → a szerver a `Host` fejlécből (`slug`) határozza meg a munkaterületet **minden** kérésnél, a belépés előtt is. Ismeretlen vagy felfüggesztett slug: egységes hibaoldal, információszivárgás nélkül.
- A belépési süti csak az adott aldomainre szól (`Domain` attribútum nélkül, `__Host-` előtaggal HTTPS-en), így egy ügyfél sütije a másiknál nem érvényes.
- A munkamenet és minden token munkaterülethez kötött; ha a kérés aldomainje és a token munkaterülete eltér, a kérés elutasítva (és naplózva).
- Az őrök és az MCP is az aldomaint használják (`https://acme.<domain>/ws/guard`, `/mcp`).
- `admin.<domain>`: csak az operátori felület.
- **Tesztelés domain nélkül:** a böngészők a `*.localhost` címeket a gépre irányítják (`acme.localhost:8080`); a homelabon egy IP-t névbe ágyazó wildcard DNS-szolgáltatással (`acme.<ip-kötőjelekkel>.sslip.io`) vagy a saját domain wildcard bejegyzésével, ha már lesz.

## 5. Adatmodell (új és bővített táblák)

- `workspace` bővítése: `slug` (egyedi, kisbetű, kötőjel), `state` (`active`, `suspended`, `deleting`), `plan_id`, `created_at`, `suspended_reason`.
- `plan`: név, korlátok (felhasználók, agentek, gépek, tárhely MB, API-kérés/perc, keresés/nap), ár (később).
- `workspace_usage`: napi számlálók munkaterületenként (üzenet, futás, token, tárhely, keresés), a korlátokhoz és a számlázáshoz.
- `operator`: az operátori fiókok (nem `actor`!): név, jelszó-hash, TOTP-titok, állapot.
- `operator_session`: külön munkamenetek, rövid lejárattal.
- `break_glass`: vészhozzáférés: operátor, munkaterület, indoklás, kezdés, lejárat, lezárás.
- `operator_audit`: minden operátori művelet; a munkaterületet érintők az ügyfél `audit_log`-jába is bekerülnek.
- Migráció a „kiterjeszt, majd szűkít” mintával (6. szabály); a mai munkaterület `slug`-ot kap (pl. `norbi`), az önálló telepítés ettől nem változik.

## 6. Elkülönítés: `workspace_id` + sorszintű védelem (RLS)

- **Első védvonal (megvan):** minden szolgáltatáshívás a hívó munkaterületére szűr.
- **Második védvonal (új): Postgres RLS.**
  - Minden `workspace_id`-s táblán szabály: `workspace_id = current_setting('colony.workspace_id')::uuid`.
  - A 15 tábla `workspace_id` nélkül: vagy kap egy (denormalizált) `workspace_id`-t, vagy a szabály a szülő során át ellenőriz (`EXISTS (…)`); táblánként eldöntve és tesztelve.
  - A szerver minden tranzakció elején beállítja: `SET LOCAL colony.workspace_id = '…'` (a `db.TxRunner`-ben, egy helyen).
  - Az alkalmazás adatbázis-felhasználója `NOBYPASSRLS`; csak az operátori és a háttérfolyamatok (archiválás, mentés) kapnak külön, auditált felhasználót.
  - Ha a beállítás hiányzik, a lekérdezés **semmit** nem lát (biztonságos alapértelmezés).
- **Ügyfélszétválasztási tesztkészlet:** két munkaterület adatokkal; minden REST-végpont, MCP-eszköz, WebSocket-csatorna és őr-végpont lefut a „másik” ügyfél azonosítóival, és mindnek el kell utasítania. Új végpont nem kerülhet be teszt nélkül (egy ellenőrzés a végpontlistát a tesztlistával veti össze).

## 7. Több szerverpéldány

| Ma memóriában | Több példánynál |
|---|---|
| Busz (`bus.InProc`) | **Postgres NOTIFY adapter**: commit után `NOTIFY colony_events, '<seq>'`; minden példány egy állandó kapcsolaton `LISTEN`-el, és az eseménynaplóból olvas. A NOTIFY tranzakciós: csak a commit után megy ki. Kimaradt jelzést a sorszám és a meglévő 5 mp-es tartalék lekérdezés pótol |
| Eseményfogyasztók (pl. kézbesítés) | Fogyasztónként **egy** példány dolgozik: advisory lock a fogyasztó nevére; ha az a példány leáll, egy másik veszi át |
| Ütemező | Ugyanígy: egy példány, advisory lockkal |
| Böngésző-kapcsolatok (`gateway.conns`) | Maradhat példányonként: mindegyik a saját kapcsolatainak küldi ki, amit a naplóból olvasott |
| Őr-kapcsolatok (`gateway/host.go`) | Új tábla: `host_connection` (gép, példány, mióta). A fájlböngésző, a belépési közvetítés és a terminálkérések (`hosts/fsrelay.go`, `loginrelay.go`, `files/fsrelay.go`) célzott jelzéssel mennek ahhoz a példányhoz, amelyiken az őr van (`NOTIFY colony_instance_<id>`), a válasz ugyanígy vissza |
| Belépési korlátozó (`identity/session.go`) | Közös tárba (Postgres tábla) |
| Fájlok (helyi kötet) | Objektumtár-adapter (S3-kompatibilis; a homelabon MinIO), ügyfelenként külön előtaggal |

- Ragadós munkamenet (sticky session) nem kell; a terheléselosztó bármelyik példányra küldhet.
- A NOTIFY-hoz példányonként egy közvetlen kapcsolat kell (nem kapcsolatgyűjtő tranzakciós módban).
- A NOTIFY elég, amíg egy adatbázis van, és az események száma másodpercenként ezres nagyságrend alatt marad (ma: néhány óránként). NATS vagy hasonló csak több adatközpontnál vagy nagyon nagy forgalomnál kell; a busz port, így az csere, nem átírás.

## 8. Guardok és agentek

### 8.1 Az ügyfél saját gépén (első kör)

- Mint ma: az ügyfél `owner`-e párosítási kódot kér a Gépek oldalon, az ügyfél a saját gépén `colony setup --guard`-dal telepíti, a gép az ő munkaterületéhez kötődik.
- A gépi token és az agent-tokenek munkaterülethez kötöttek; egy őr csak a saját munkaterülete agentjeit kapja meg.
- A gyökérmappákat a gépen lévő `guard.yaml` adja meg, a szerver nem bővítheti (meglévő szabály); a sandboxos agentek `bwrap` alatt.
- Az agentek az ügyfél saját előfizetésével vagy kulcsával futnak; nekünk nincs költségünk.

### 8.2 A mi szerverünkön (később, külön szolgáltatás)

- Ügyfelenként **külön Linux-felhasználó** (vagy konténer), saját home-mappával (`/srv/tenants/<slug>/`) és saját őrrel, ami csak az ő munkaterületéhez van párosítva.
- Erőforráskorlát ügyfelenként: systemd slice (CPU, memória, folyamatszám), lemezkvóta.
- **Csak API-kulccsal:** `ANTHROPIC_API_KEY` a Claude Code-nak, OpenAI-kompatibilis cím és kulcs a 47. kör agentjének (pl. OpenRouter). Bejelentkezett személyes előfizetés itt nem futhat.
- A kulcsot az ügyfél adja meg a felületen; titkosítva tároljuk (saját kulcs, nem a chatben), csak az ő őre kapja meg.
- A feltételekben: „a modellek költsége az ügyfélé”.
- **Prémium (később):** a mi OpenRouter-kulcsunkkal, ügyfelenként mért tokenekkel (a futásnapló már méri), havi kerettel és továbbszámlázással.

## 9. Szerepkörök és az operátor

| Szint | Szerep | Mit tehet |
|---|---|---|
| Ügyfél | `owner` | Felhasználók, szerepkörök, csatornák, wiki, agentek, gépek, a csomag és a számlázási adatok megtekintése |
| Ügyfél | `admin` | Ugyanez a tulajdonjog és a számlázás nélkül |
| Ügyfél | `member`, `observer` | Mint ma |
| Platform | operátor | Ügyfél létrehozása, felfüggesztése, törlése; csomag és korlátok; használat; rendszerállapot; vészhozzáférés |

- A Rendszerállapot oldal adatbázis-kártyája (teljes méret, legnagyobb táblák, napi minták; 50. kör) az egész telepítésről szól, ezért itt **csak az operátoré** lesz; az ügyfél adminja nem látja.

- **Fejlesztői mód** (`COLONY_OPERATOR_DEV_ACCESS=1`):
  - az operátor minden munkaterület tartalmát látja, külön lépés nélkül;
  - a felület **minden oldalon kiírja**: „Fejlesztői mód: az operátor minden adatot lát”;
  - induláskor figyelmeztetés a naplóban;
  - éles módban (`COLONY_ENV=production`) a szerver **nem indul el** ezzel a kapcsolóval.
- **Éles mód: vészhozzáférés:**
  - az operátor indoklással és időkorláttal (pl. legfeljebb 4 óra) nyitja meg;
  - az ügyfél auditnaplójában és az `owner` értesítései között megjelenik;
  - lejáratkor magától zárul; minden olvasás naplózva.
  - A tartalom nélküli adatokat (méret, használat, állapot, hibák) az operátor vészhozzáférés nélkül is látja.

## 10. Korlátok

- A korlát a szolgáltatásban, a művelet előtt ellenőrződik (felhasználó, agent és gép létrehozása, feltöltés, keresés); túllépéskor érthető hibaüzenet az ügyfélnek, és egy esemény.
- Kérésszám-korlát munkaterületenként (a „hangos szomszéd” ellen); több példánynál a számláló közös tárban.
- Napi használati számlálók (`workspace_usage`) a felületre és a számlázáshoz.

## 11. Regisztráció és számlázás

- **Első lépés:** az operátor hozza létre az ügyfelet, és egy egyszer használatos meghívólinket ad az ügyfél tulajdonosának (e-mail-küldés még nincs; a linket a felület mutatja).
- **Később:** önálló regisztráció, e-mailes meghívók (SMTP), fizetési szolgáltató (pl. Stripe), csomagok, próbaidő.
- **Adó:** alanyi adómentességgel (AAM), főleg külföldi vevőknél, ÁFA-kérdések jönnek; árazás előtt könyvelő.
- **Jog:** felhasználási feltételek, adatvédelmi tájékoztató, adatfeldolgozási szerződés (46. kör, GDPR).

## 12. Mentés és visszaállítás ügyfelenként

- Napi teljes adatbázis-mentés (mint ma), plusz ügyfelenkénti logikai export (minden tábla a `workspace_id` szerint, a fájlok előtagja), hogy egy ügyfél visszaállítható vagy kiadható legyen a többi érintése nélkül.
- Visszaállítási próba rendszeresen.

## 13. Tesztelés a homelabon

- Két szerverpéldány egy terheléselosztó (Caddy vagy nginx) mögött, közös Postgres, MinIO.
- Két teszt-ügyfél (`acme`, `beta`) kitalált adatokkal, egy-egy őrrel.
- Ellenőrzés: üzenet az egyik példányon → megjelenik a másikhoz kapcsolódó böngészőben; a fájlböngésző az őr példányán át működik; egy példány leállítása után a fogyasztók és az ütemező átkerülnek; az ügyfélszétválasztási tesztek zöldek.
- A homelab éles Colonyhallja ettől független marad (külön adatbázis, külön portok).

## 14. Fázisok

| Fázis | Tartalom | Kész, ha |
|---|---|---|
| **0. Elkülönítés** ✅ (45. kör) | `slug`, aldomain szerinti munkaterület, RLS minden táblán, ügyfélszétválasztási tesztek, `COLONY_MULTITENANT` kapcsoló; plusz: kapcsolható saját Gitea ügyfelenként, külön adatbázis-szerepkör az új telepítéseknél | A tesztkészlet zöld; az önálló telepítés változatlan |
| **1. Operátor** (51a: kész, a vészhozzáférés 51b; `24-hosztolt-ajanlat.md`) | Operátori fiók 2FA-val, `admin.<domain>`, ügyfél létrehozása és felfüggesztése, meghívólink, fejlesztői mód kiírással, vészhozzáférés | Egy új ügyfél a meghívóval be tud lépni; a vészhozzáférés látszik az ügyfél auditjában |
| **2. Több példány** | NOTIFY-busz, fogyasztók és ütemező advisory lockkal, őr-kapcsolatok táblája és célzott jelzések, közös korlátozó, objektumtár | A 13. pont próbái a homelabon |
| **3. Korlátok és csomagok** | `plan`, `workspace_usage`, kikényszerítés, használati oldal | Túllépéskor elutasítás és esemény |
| **4. Számlázás és regisztráció** | Fizetési szolgáltató, önálló regisztráció, e-mail | Könyvelővel egyeztetve |
| **5. Agentek a mi szerverünkön** | Ügyfelenkénti Linux-felhasználó és őr, API-kulcsok titkosítva, erőforráskorlát; később OpenRouter-prémium | Két ügyfél agentje egy gépen fut, egymás mappáit nem látják |

## 15. Nyitott kérdések

- A domain (az aldomainekhez wildcard DNS és tanúsítvány kell).
- Az operátori 2FA módja (TOTP alkalmazás, később hardverkulcs).
- Az e-mail-küldés szolgáltatója (meghívók, értesítések).
- A Claude Team/Enterprise használata a mi szerverünkön: a feltételeket ellenőrizni kell, ha az API-kulcs mellett ez is szóba jön.
- A RAG (36. kör) keresési indexe és a későbbi archívum ügyfelenkénti szűrése: a 36. kör beolvasztása után ellenőrizni.
