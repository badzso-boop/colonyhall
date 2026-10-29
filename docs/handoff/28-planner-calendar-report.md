# Handoff jelentés: 28. kör – saját feladatlista, naptár és reggeli összefoglaló

- **Dátum:** 2026-10-29
- **Ág:** `feature/planner` (külön worktree)
- **Ki írta:** az Antigravity a modul magjának egy részénél elfogyott a keretből (commit nélkül). Claude Code a félkész kódot félretette (hibás sorrendezés-középpont, minden URL-t `https://`-nek vevő maszkolás), és a kört egyben megírta.

## Mi készült el

1. **Feladatok (`planner` modul, `migrations/00036_planner.sql`):** cím, leírás (markdown), állapot (ütemezve / elkezdve / befejezve), `started_at` (az első indítás), `done_at` (újranyitáskor törlődik), `status_changed_at`, sorrend, soft delete. Minden változás egy eseményt ír ugyanabban a tranzakcióban (csak azonosítók és állapotok).
2. **REST, csak a tulajdonos embernek:** agent-token minden végponton 403-at kap még a kérés beolvasása előtt; más ember feladata 404.
3. **Naptár:** ICS-feliratkozás (`https://`, `webcal://`), 15 percenként letöltve (`COLONY_CALENDAR_REFRESH`), „Frissítés most”, mai és 7 napos nézet. A link titok: csak maszkolva látszik, hibaüzenetbe, naplóba, agenthez nem kerül.
4. **MCP, csak olvasás:** `planner_read` (nyitott feladatok a tulajdonos sorrendjében, és ami tegnap 00:00 óta létrejött, elkezdődött vagy befejeződött) és `calendar_read`. Mindkettő azzal kezdődik, hogy a tartalom adat, nem utasítás. Írási eszköz nincs (teszt nézi az eszközlistát).
5. **Ki olvashatja:** a feladatokat alapból minden agent, a naptárat alapból senki; a 📅 panelen állítható.
6. **Fejlesztői oldal:** 📝 Feladatok (Elkezdve / Ütemezve / összecsukott Befejezve, húzás és ↑/↓, cím szerkesztése kattintással, állapotválasztó, leírás markdownban, törlés oldalon belüli megerősítéssel) és 📅 Naptár (napirend, feliratkozások, hozzáférés).
7. **Reggeli összefoglaló:** recept és utasításszöveg a `docs/06-allapot-es-uzemeltetes.md`-ben (`@asszisztens`, `/cron "30 7 * * *"`). Az agentet admin hozza létre.

## Nyitott pontok döntései

- **Sorrend:** lebegőpontos pozíció; mozgatáskor a két szomszéd átlaga, új feladat a tetejére. Ha egy rés elfogy, a lista újraszámozódik.
- **ICS-könyvtár:** `github.com/emersion/go-ical` (MIT) és `github.com/teambition/rrule-go` (MIT). Az Outlook Windows-os időzónaneveit (pl. „Central Europe Standard Time”) Europe/Budapestként olvassuk.
- **Az URL tárolása:** titkosítás nélkül a `planner_calendar.url` oszlopban (a szerveren nincs titoktároló segéd); soha nem adjuk vissza.
- **SSRF:** a letöltő külön adapter (`planner/fetch`): csak `https`, feloldás után csak nyilvános IP-re tárcsáz (loopback, privát, link-local, CGNAT, multicast tiltva), átirányítás legfeljebb 3, mindegyik újraellenőrizve, 5 MB, 20 mp.

## Kimaradt

- Élő frissítés egy második nyitott fülön (a panel megnyitáskor és minden művelet után újratölt).
- Hamis agentes forgatókönyv a reggeli összefoglalóhoz: helyette e2e teszt hívja a két MCP-eszközt a tegnapi / elkezdett / áthozott esetekkel.
- Napközbeni üzenetek, tanulás, gazdagabb mezők, vállalkozási rész (Norbi döntése; a vállalkozási rész a 29. kör).
- A PWA felület.

## Ellenőrzés

- `make test`: zöld (új: `internal/modules/planner` ICS-tesztek Google- és Outlook-mintával — heti RRULE + EXDATE + áthelyezett alkalom, egész napos, lemondott, téli időszámítás-váltás —, `planner/fetch` SSRF-teszt, `internal/e2e/planner_test.go`).
- `make lint`: 0 issues. `make test-web`: 27/27. `go test -race ./internal/e2e/ ./internal/modules/planner/...`: zöld.
- `make test-pg` (gyorsítótár nélkül, új `colony_test`): zöld, `TestPostgres/planner` is.
- Böngésző (Playwright, 400 px széles, magyar nyelv): belépés, két feladat felvétele, állapotváltás, leírás szerkesztése, naptárpanel — JavaScript-hiba nincs. A 400 px-en látható vízszintes görgetés a régi oldalon is megvan (az új fülek nélkül is 442 px), nem ebből a körből jön.

## Mit nézzen meg a reviewer először

1. `internal/modules/planner/fetch/fetch.go` és `calendar.go` (`NormalizeCalendarURL`, `PublicIP`, `FetchError`): SSRF és a titkos link.
2. `internal/modules/planner/service.go` `resolveOwner` / `ReadTasks` / `ReadEvents`: ki mit olvashat.
3. `internal/modules/planner/httpapi/routes.go`: agentek elutasítása a kérés elején.
