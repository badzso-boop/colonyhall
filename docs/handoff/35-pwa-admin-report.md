# 35. kör jelentése: az adminisztráció a PWA-ban

Claude Code, 2026-11-03, ág: `feature/pwa-admin` (a `main` `76d110f`-jéből). Nincs beolvasztva, nincs élesítve.

## Commitok

- `09fb60a` Csatornazár az agentek ellen (szerver, őr).
- `6878a6d` Ártábla, árfolyam, becsült költség, futásszűrők.
- `18003d4` Emberek kezelése csak a tulajdonosnak, audit-szűrők és CSV, rendszerállapot.
- Az utolsó commit: a teljes PWA-rész, a biztonsági javítások, dokumentáció, képek.

## Pontok

Minden pont elkészült, az alábbi eltérésekkel.

1. **Navigáció:** „Adminisztráció” csoport, `/app/admin/…`, a nem jogosult 404-stílusú üres állapotot lát; a Felhasználók csak tulajdonosnak.
2. **Futások:** időszak, KPI-k, Claude-gyűrűk gépenként, két grafikon, agenttáblázat rendezéssel és mini-grafikonnal, napi keretek, futáslista végtelen görgetéssel, futás-Sheet.
3. **Beállítások:** mind a hat szakasz, „Mi változik” összegzéssel.
4. **Felhasználók:** táblázat, új felhasználó, szerepkör, jelszó, letiltás.
5. **Tokenek:** saját, másoké, háromlépéses kulcscsere (`localStorage`-ban csak azonosítók és pipák; a böngészős teszt ellenőrizte, hogy a titok nem kerül oda).
6. **Audit:** szűrők, kibontható sor szövegként, láncellenőrzés, CSV.
7. **Vészleállítás és csatornazár:** feloldó Dialog jelszóval, a passkey helye megjelölve; 🔒 az oldalsávon, sárga sáv „Feloldás” gombbal, halványított agent-avatarok.
8. **Rendszerállapot:** kártyák ikonnal és szöveggel, 10 mp-es frissítés, „Másolás”.

## Döntések, amiket a prompt rám hagyott

- **Függő ébresztések zárolt csatornában: megmaradnak,** a feloldás után ébresztenek. Teszt: `internal/e2e/channel_lock_test.go`.
- **Letiltás: a meglévő `stopped` állapot.** Embernél is megfelel: a tokeneket és a munkameneteket elutasítja, a munkameneteket visszavonja.
- **„Utoljára látva” = utolsó belépés,** mert a munkamenetek nem tárolnak utolsó használatot. A felület ki is írja.
- **Jelszóreset után az API-tokenek érvényesek maradnak;** a Dialog megmondja, és a Tokenek oldalon visszavonhatók. A kiválasztás szándékos: a reset nem feltétlenül jelent kompromittálódást, és egy kikapcsolt token csendben eltörhetne egy szkriptet. Ha mást szeretnél, kis változtatás.
- **Új ember létrehozásakor** a szerver API-tokent is ad (a meglévő viselkedés); a felület ezt nem mutatja, csak a jelszót.
- **Nincs „Javasolt értékek” gomb és nincs alapár:** forrás-URL nélkül nem ajánlok árat.

## Eltérések és korlátok

- **Tokenek:** a szerver csak hash-t tárol, nincs előtag és „utoljára használva”. A lista és a kulcscsere ezért névvel és dátummal azonosít. A pótláshoz migráció kellene, és a régi tokeneknél akkor is üres maradna.
- **Futás-Sheet:** nincs benne az utasítások verziója, mert a futás nem tárolja. Külön, kis szerverbővítés lenne.
- **Agenttáblázat „Utolsó futás”:** napi pontosságú (a `/usage`-ból jön).
- **Egyéni időszak:** legfeljebb 90 nap vissza. Ha nem a mai napig tart, az átlagos időtartam „–”, mert a szerver összesítője a teljes lekért időre szól. A KPI költsége mindig a kiválasztott napok összege.
- **Tag is olvashatja a `/runs` és `/usage` végpontot** (a 22. kör óta embereknek szól, az agent-áttekintő használja). Az admin oldal rejtett előlük, de ezekre nem 403 jön. Az árak és az audit admin-only.
- **Fájltár mérete:** kimaradt, a szerver nem tudja olcsón.
- **Grafikon-színek:** az agentek 33. körös palettája nem teljesíti a színtévesztő-ellenőrzést (narancs/borostyán ΔE 1,1; zöld/lime 5,8). A prompt kötelezővé tette ezeket a színeket, ezért jelmagyarázat, tooltip és táblázat viszi az azonosítást. A modellekhez választott hat színes sorrend normál látással megfelel, színtévesztőknél egy pár (`pink`/`teal`, ΔE 5,2) a határ alatt van, sötét módban pedig kettő a világossági sávon kívül; ezért ott is ugyanez a másodlagos jelölés.
- **A képek** futásadatai mockok (a memóriás szerveren nincs gép); a csatornazár, az audit, a jelszavas feloldás, a tokenek és az emberek valódi API-n futottak.

## Az ellenőrzés közben talált és javított hibák

Mindegyikhez teszt tartozik, ami a javítás nélkül elbukik.

1. **Rossz feloldó jelszó kiléptetett:** a kliens minden 401-et lejárt munkamenetnek vett, így vészleállítás közben a belépőoldalra dobott. Böngészős teszt fogta meg. `shared/api/client.test.ts`.
2. **A tulajdonos nem kapott saját tokent** („not allowed”), a kulcscsere elakadt az első lépésben. `TestOwnTokens`.
3. **Automatikus biztonsági átnézés, régi WebSocket-jog:** a kapcsolat a csatlakozáskori szerepkört őrizte, a lefokozott admin tovább kapta az eseményeket. Letiltásra és jelszóresetre is. Most bezárul, a kliens újra hitelesít. Visszajátszás nem okoz hurkot. `TestAccessChangeClosesTheSocket`.
3b. **A commit utáni második biztonsági átnézés, ugyanez a terület:** a visszavont token, a törölt agent és a hitelesítés közben történt jogváltozás nem zárta a kapcsolatot; kijelentkezés után is nyitva maradt. Most mind zár; a kijelentkezés és a lejárat legfeljebb egy percen belül (percenkénti újraellenőrzés embereknél). `TestRevokedTokenClosesItsSocket`, `TestArchivedAgentLosesItsSocket`, `TestLogoutClosesTheSocketSoon`.
4. **Automatikus biztonsági átnézés, jelszóreset:** a munkamenetek visszavonása külön lépés volt. Most egy tranzakció a jelszócserével.
5. **A feloldó jelszó korlátlanul próbálgatható volt:** 5 hiba után 15 percre 429, a belépéstől külön számolva, hogy egy belépési zárolás ne akadályozza a feloldást. `TestEmergencyClearIsRateLimited`.
6. **Agent admin rendszerállapota:** a `ListHosts` embert kér, így az agent adminnak 403 jött volna a korábban működő bannerre. Most ő csak az `emergency_stop`-ot kapja. `TestSystemStatus`.
7. **CSV-képletinjekció:** az `=`, `+`, `-`, `@` kezdetű cellák `'` előtagot kapnak.
8. **Telefonon** a hosszú oldal az alsó sáv alá csúszott, és a modellnév betűnként tört. Csak a képeken látszott.

## Tesztek

```
make test        ok
make lint        0 issues
make test-web    80 fájl, 511 teszt, ok (típusellenőrzés és lint is)
make test-pg     ok (a storetest új részei is)
go test -race ./internal/guard/ ./internal/e2e/ ./internal/gateway/ ./internal/eventlog/   ok
make gen-check   ok
```

Böngészős bejárás (`docs/handoff/35-screens/`): három méret, két téma, vízszintes túlcsordulás és konzolhiba nélkül.

## Élesítési sorrend

1. Előbb a **szerver** (két új migráció: `00043`, `00044`; induláskor futnak).
2. Utána az **őrök**, mert a `channel.locked` leállítása az őrben van: PC, homelab, nato. Előtte `pgrep -af "claude -p|agy -p"` minden gépen; futó munka alatt ne.

Ha az őrök késnek, a zár akkor is hat az új írásokra és ébresztésekre, csak a már futó munka nem áll le azonnal.

Élesítés után az árakat és az árfolyamot neked kell megadni (Beállítások → Árak és pénznem), addig „nincs ár” látszik.

## PWA és fejlesztői oldal

| Funkció | PWA | Fejlesztői oldal |
| --- | --- | --- |
| Csevegés, szálak, csatornák, DM | ✅ | ✅ |
| Agentek, gépek | ✅ | panelek megvannak |
| Feladatok, naptár, jegyzetek, ütemezések, wiki, szabályok, foglalások | ✅ | panelek megvannak |
| Futásnapló, felhasználás | ✅ bővebb | ✅ |
| Beállítások (tömörítés, fájllekérés, feladatátadás, Gitea) | ✅ | ✅ |
| Vészleállítás és feloldása, tokenek | ✅ | ❌ (a kódjában nincs rá hívás) |
| Audit, audit-CSV | ✅ | ❌ |
| Felhasználók, árak, csatornazár, rendszerállapot, kulcscsere | ✅ | ❌ |

A táblázat a fejlesztői oldal kódjának keresésével készült (panelek és hívott végpontok), nem funkciónkénti kipróbálással. A kivezetés előtt érdemes egy-két hétig csak a PWA-t használni, és közben figyelni, mihez kell még visszamenni.

## Teendő a beolvasztás után

- `task.md`: **a tulajdonosi kulcs cseréje** továbbra is Norbi teendője; a Tokenek oldalon most már végigvezetett. Csere után jelölhető késznek.
- Az élő claude.ai dokumentumot frissíteni kell (`docs/README.md`).

## Amit az átnéző először nézzen meg

1. `gateway.accessChanged`: a kapcsolat bezárása és az `authSeq`.
2. `identity/admin.go`: `IssueToken` önmagára, `ClearEmergency` korlát.
3. `audit/httpapi` export: jogosultság a fejlécek előtt, `noFormula`.
4. `internal/server/status.go`: nem kerül-e titok a válaszba.
5. `features/admin/pages/TokensPage.tsx`: a varázsló nem tárol titkot.
