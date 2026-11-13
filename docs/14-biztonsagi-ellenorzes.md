# Biztonsági ellenőrzés (40. kör)

Állapot: 2026-11-13, a `feature/security` ágon. Mérce: **OWASP ASVS 4.0, 2. szint** (a Colonyhallra érvényes fejezetek) és az **OWASP Top 10 for LLM Applications (2025)**. A táblázat a fontos követelményeket nézi fejezetenként, nem az ASVS mind a ~280 pontját; ahol egy pont állapotát a kód nem mutatja egyértelműen, ott „részben” szerepel.

Jelölés: ✅ teljesül · 🟡 részben · ❌ nem teljesül · – nem vonatkozik rá.

**Tudatosan kimarad (Norbi döntése, 2026-11-12):** HTTPS és HSTS, a karbantartó PC-jén nem root őr, a homelab `bypassPermissions` agentje, a homelab LAN-portjai. A szerver alapból `127.0.0.1`-en figyel; magánhálózaton (pl. Tailscale) érdemes futtatni.

## Összesítés

| Mérce | ✅ | 🟡 | ❌ | – |
|---|---|---|---|---|
| ASVS 4.0 L2 (37 pont) | 29 | 4 | 3 | 1 |
| LLM Top 10 (10 pont) | 6 | 2 | 0 | 2 |

## ASVS 4.0, 2. szint

| # | Követelmény | Állapot | Hol / miért |
|---|---|---|---|
| V2.1.1 | Jelszó legalább 12 karakter | ✅ | `identity/password.go`: 12–128 karakter |
| V2.1.7 | Kiszivárgott jelszavak elleni ellenőrzés | ❌ | Nincs (külső szolgáltatás vagy nagy lista kellene); döntés: később |
| V2.2.1 | Automatizált próbálkozás elleni védelem | ✅ | Tartós korlát handle-ként 5, IP-nként 20 próba / 15 perc (`identity/ratelimit.go`, `auth_failure` tábla, 40. kör) |
| V2.2.x | Engedélylista a kizárás alól | ✅ | `COLONY_LOGIN_ALLOWLIST`; a hibák így is naplózódnak |
| V2.4.1 | Jelszó tárolása erős hash-sel | ✅ | argon2id, 19 MiB, t=2 (`identity/password.go`) |
| V2.5 | Elfelejtett jelszó folyamata | – | Nincs ilyen folyamat: jelszót az admin vagy a parancssor állít be |
| V2.10.4 | Szolgáltatási titkok nincsenek a kódban | ✅ | `.env` gitignore-olva, `make privacy` a CI-ban; tokenek és `.env` 0600-as új fájlba íródnak (`7fca7f8`) |
| V3.2.1 | Új munkamenet-token belépéskor | ✅ | Belépéskor új munkamenet (`identity/session.go`) |
| V3.3.1 | Kilépéskor a munkamenet érvénytelen | ✅ | A munkamenet a szerveren visszavonódik |
| V3.3.2 | Újrahitelesítés legalább 30 naponta | ✅ | `SessionTTL` 30 nap |
| V3.4.1 | Süti `Secure` jelzővel | 🟡 | Csak `COLONY_SECURE_COOKIES=1`-gyel (HTTPS nélkül nem lehet); a HTTPS kimarad |
| V3.4.2–3 | Süti `HttpOnly`, `SameSite` | ✅ | `HttpOnly`, `SameSite=Lax` (`identity/httpapi/handlers.go`) |
| V3.5.x | Token alapú munkamenet | ✅ | A tokenek csak hash-elve, a megjeleníthető előtag 8 karakter (`TokenPrefix`, fuzz-teszt) |
| V3.5.3 | Egy MCP-munkamenet nem vehető át | ✅ | A munkamenet a megnyitó szereplőhöz kötve (`f63d273`) |
| V4.1.1 | Hozzáférés-ellenőrzés a szerveren | ✅ | Minden hívásnál, nem csak csatlakozáskor (`CLAUDE.md` 9. szabály) |
| V4.2.1 | Közvetlen objektumhivatkozás (IDOR) ellen | ✅ | RLS minden táblán, cross-tenant e2e minden útvonal-paraméterre valódi acme-objektummal (`384a6a1`) |
| V4.2.2 | CSRF elleni védelem | ✅ | `X-CSRF-Token` minden sütis, nem biztonságos kérésnél (`httpx/httpx.go`) |
| V4.3.1 | Admin felület többlépcsős azonosítással | ❌ | Nincs MFA; a többbérlős operátorhoz tervezett (51. kör) |
| V4.3.2 | Csak a tulajdonos ad tokent másnak | ✅ | `828f42e`; profilszerkesztés rangellenőrzéssel (`4a985be`) |
| V5.1.x | Bemenet-ellenőrzés, korlátok | ✅ | Handle-, hossz- és határidő-korlátok (`6b4428c`, `6fda78d`), fuzz-tesztek |
| V5.2.1 | HTML-tisztítás | ✅ | Egy közös, nyers HTML-t tiltó markdown-megjelenítő; React nyers HTML nélkül |
| V5.2.6 | SSRF elleni védelem | ✅ | `web_read` és a sandbox-proxy (`b30cf66`); a beágyazó és a reranker címét csak az üzemeltető adja meg |
| V5.3.4 | SQL-injekció ellen | ✅ | sqlc, paraméteres lekérdezések |
| V5.3.x | Kimenet kódolása más környezetbe | ✅ | CSV-cellák képletvédelme (fuzz), terminálkimenet címkéi escape-elve (`d2db729`), őr-lépéscímkék tisztítva (`6a22c3a`) |
| V7.1.1 | Nincs titok és jelszó a naplóban | ✅ | Tokenek soha; a kizárás eseménye jelszó nélkül |
| V7.4.1 | Általános hibaüzenet azonosítóval | ✅ | „I ran into an error” + `[error:<id>]`, a részlet csak embernek |
| V7.x | Biztonsági események naplózása | ✅ | Hash-láncolt audit (`audit/model.go`), `auth.locked_out` |
| V8.1.1 | Érzékeny válasz nem kerül gyorsítótárba | ✅ | Az API válaszai `Cache-Control: no-store` (`0ce1c2b`) |
| V8.3.x | Érzékeny adat kezelése | 🟡 | A titkok csak a `colonyhall-exchange` mappán át; a beszélgetések titkosítatlanul az adatbázisban (az üzemeltető gépén) |
| V9.1.1 | TLS minden kapcsolaton | ❌ | Nincs beépített HTTPS (döntés); magánhálózat ajánlott |
| V12.1.x | Feltöltési méret és tárolás | ✅ | Méretkorlát, a feltöltés lemezen várakozik, nem memóriában (`70b9388`) |
| V12.3.x | Útvonal-bejárás elleni védelem | ✅ | A mappaböngésző feloldja a symlinkeket (`dca04b5`); fájllinkek vezérlőkarakter nélkül (`6a22c3a`) |
| V12.5.x | Feltöltött fájl kiszolgálása | ✅ | `Content-Disposition: attachment`, `nosniff` |
| V13.1.x | API-szerződés és típusellenőrzés | 🟡 | OpenAPI-szerződés, minden e2e-válasz ellenőrizve; a kérések tartalomtípusát nem minden végpont ellenőrzi külön |
| V14.2.x | Függőségek | ✅ | `govulncheck`, `npm audit`, Dependabot (`a7034f6`), rögzített image-digestek (`f6b6d4e`) |
| V14.3.x / 14.4.x | Biztonsági fejlécek | 🟡 | CSP, `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`, COOP megvan; HSTS nincs (HTTPS nélkül nincs értelme) |
| V14.x | Futtatókörnyezet megszigorítása | ✅ | Csak olvasható gyökér, `cap_drop: ALL`, `no-new-privileges`, memória- és PID-korlát (`e976960`); a build-környezetbe nem kerül titok vagy git-előzmény (`9cf0f28`); a kiadás aláíró tokenje el van választva a függőségek kódjától (`1d28804`) |

## OWASP Top 10 for LLM Applications (2025)

| # | Kockázat | Állapot | Hol / miért |
|---|---|---|---|
| LLM01 | Prompt injection | 🟡 | Minden chat-elem `<colonyhall_message>` keretben (fuzz-teszt: a keret nem törhető), webhook-szövegben az említések hatástalanítva (fuzz-teszt); jóváhagyás, sandbox. Teljesen megakadályozni nem lehet: az éles adatban is volt „SYSTEM_MESSAGE”-szerű üzenet |
| LLM02 | Érzékeny adat kiszivárgása | 🟡 | Hiba csak azonosítóval a chatben; az őr tokenjei és a kliensek belépései a tiltólistán (`3f13345`); a teljes hozzáférésű agent a saját gépén mindent lát (döntés) |
| LLM03 | Ellátási lánc | ✅ | Modellek rögzített commitról, sha256-tal; image-digestek; az ONNX Runtime telemetriája kikapcsolva |
| LLM04 | Adat- és modellmérgezés | – | Nincs tanítás vagy finomhangolás |
| LLM05 | Kimenet nem megfelelő kezelése | ✅ | Biztonságos markdown, lépéscímkék és fájllinkek tisztítása, agent-kimenet sosem fut parancsként (`CLAUDE.md` 11. szabály) |
| LLM06 | Túlzott önállóság | ✅ | Teljes hozzáférésű agentnek agent csak jóváhagyással ad feladatot, vészleállítás, sandbox; előnézet csak az őr által elfogadott agentnek (`e643fac`) |
| LLM07 | Rendszerprompt kiszivárgása | ✅ | A promptokban nincs titok (a kód nyilvános) |
| LLM08 | Vektor- és beágyazási gyengeségek | ✅ | Hozzáférés-szűrés minden keresésnél, RLS; a munkaterület-szintű találat nem árulja el a privát helyet (`14ac419`) |
| LLM09 | Félretájékoztatás | – | Az agentek válaszai egy csapat munkaeszközében; nem a szerver állítása |
| LLM10 | Korlátlan erőforrás-használat | ✅ | Napi tokenkeret, Claude- és Antigravity-limitek, futásonkénti időkorlát, agent–agent sebességkorlát |

## Fuzz-tesztek

A `go test` a mintákat futtatja; hosszabb futás például: `go test ./internal/guard -run '^$' -fuzz FuzzClaudeStreamLine -fuzztime 10m`.

| Célpont | Hol | Talált |
|---|---|---|
| Claude Code és agy stream-json, `/usage` | `internal/guard/fuzz_test.go` | a fájlútvonal- és MCP-eszköz-címkék vezérlőkaraktert és hosszú szöveget vihettek a PWA-ba (javítva) |
| Fájllinkek | ugyanott | NUL-t vagy dekódolt `%00`/`%0A`-t tartalmazó link (javítva) |
| Prompt-keret, titkos fájlnevek | ugyanott | – |
| Kliens-IP és engedélylista | `internal/platform/clientip` | – |
| CSV képletvédelem, említések, handle, token-előtag, webhook-említések | `audit/httpapi`, `conversation`, `identity`, `git` | – |

## Nyitott (Norbi döntése)

1. Kiszivárgott jelszavak ellenőrzése (V2.1.7).
2. MFA az admin felülethez (V4.3.1), legkésőbb a többbérlős operátornál.
3. HTTPS és HSTS (V9, V14.4): kimarad, amíg csak magánhálózaton fut.
