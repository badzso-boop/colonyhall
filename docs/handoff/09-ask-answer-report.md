# Handoff Report: Kérdés–válasz protokoll (Ask / Answer / Escalation)

Dátum: 2026-10-16  
Ág: `feature/ask-answer`  
Szerző: Antigravity CLI  
Felülvizsgáló: Claude Code  

---

## 1. Változtatások commitonként

- `92d4d54` **Step 1: Add question table migration and OpenAPI contract**
  - Adatbázis migráció: `migrations/00012_question.sql` létrehozza a `question` táblát (`id`, `number`, `workspace_id`, `asked_by`, `to_actor_id`, `to_human`, `channel_id`, `thread_root_id`, `text`, `options`, `default_option`, `deadline`, `state`, `answer`, `answered_by`, `created_at`, `answered_at`, `escalated_at`, `withdrawn_at`, `expired_at`) és az indexeket (`question_workspace_number_idx`, `question_thread_asked_idx`, `question_due_idx`).
  - OpenAPI specifikáció: `api/openapi.yaml` kibővítve a `/questions` és `/questions/{questionId}/answer` végpontokkal és sémákkal (`Question`, `QuestionList`, `AnswerQuestionRequest`, `QuestionState`).
  - SQL lekérdezések és sqlc: `internal/modules/question/postgres/queries.sql`, `sqlc.yaml`, `internal/apispec/apispec.gen.go` és a modulok sqlcgen fájljai frissítve.

- `91e0d8c` **Step 2: Implement question module and store tests**
  - Domain modell (`internal/modules/question/model.go`): `Question`, `AskInput`, állapotok (`open`, `answered`, `escalated`, `withdrawn`, `expired`), `FormattedNumber()` (`Q-12`), `ParseReference()` (`Q-12`, `12` vagy UUID feloldása).
  - Események (`internal/modules/question/events.go`): `question.asked`, `question.answered`, `question.escalated`, `question.withdrawn`, `question.expired`.
  - Tár interfész és adapterek (`internal/modules/question/store.go`):
    - `memory/store.go`: in-memory referencia implementáció thread-safe szálbiztonsággal.
    - `postgres/store.go`: PostgreSQL adapter tranzakciós lekérdezésekkel és sorszámozással.
  - Szolgáltatás (`internal/modules/question/service.go`): üzleti logika, jogosultságok, egyetlen nyitott kérdés per szál és kérdező, teljes hozzáférésű agent átirányítása emberre (`to_human`), háttérfolyamat (`RunDue`, `Loop`) 30 perces eszkalációval és határidős alapértelmezett választással.
  - Támogató interfészek: `identity.Directory.Admins(ctx, workspaceID)` az adminok megkeresésére, `hosts.Service.IsFullAccess(ctx, agentID)` a teljes hozzáférésű agent ellenőrzésére.
  - Adattár tesztek (`internal/storetest/storetest.go`, `backends_test.go`): `testQuestion` futtatása memórián és Postgresen (`make test-pg`).

- `1e971c2` **Step 3: Integrate question module with delivery and server background loop**
  - Kézbesítés (`internal/modules/delivery/model.go`, `service.go`): `ItemAnswer = "answer"` tétel típus. A `question.answered` eseményre feliratkozva a kézbesítő modul ébresztő (`FlagUnread | FlagWake`) tételt hoz létre a kérdező inboxában. Az `Inbox()` betölti a válaszüzenetet és beállítja a megfelelő fejléceket.
  - Beszélgetés olvasó (`internal/modules/conversation/api.go`, `service.go`): `MessageByID` metódus hozzáadva a `Reader` interfészhez.
  - Szerver huzalozás (`internal/server/server.go`): `qStore`, `qSvc`, `App.Question` regisztrálása és `a.Question.Loop(ctx, 15*time.Second, ...)` indítása a háttérben.

- `341c667` **Step 4: Add MCP tools and REST handlers for questions**
  - MCP eszközök agenteknek (`internal/modules/question/mcptools/tools.go`):
    - `ask`: kérdés feltevése szálban csapattársnak (`@handle`) vagy embernek (`human`), visszatérési érték `Q-12`.
    - `answer`: nyitott vagy eszkalált kérdés megválaszolása, a válasz posztolása a szálba és a kérdező inboxába kézbesítése.
    - `withdraw`: saját nyitott/eszkalált kérdés visszavonása (kizárólag a kérdező által).
  - REST API kezelők (`internal/modules/question/httpapi/handlers.go`):
    - `GET /api/v1/questions?state=`: kérdések szűrése és listázása.
    - `POST /api/v1/questions/{questionId}/answer`: nyitott/eszkalált kérdés megválaszolása ember által.

- `345549c` **Step 5: Allow answer items in direct guard and add prompt instruction**
  - Őr szűrés (`internal/guard/direct.go`): a `humanOnly` szűrő a közvetlen módban futó agenteknél az emberi üzenetek mellett engedélyezi az `it.Kind == "answer"` típusú tételeket is, mivel azok nem feladatot, hanem a feltett kérdésre érkező választ hordozzák.
  - Rendszerutasítás (`internal/guard/direct.go`: `DirectPrompt`): kiegészítés a döntési helyzetekre: *"When you need a decision, use ask (to human for anything irreversible) instead of guessing."*
  - Egységtesztek (`internal/guard/direct_test.go`): `TestDirectPrompt` és `TestHumanOnly` ellenőrzi az instrukciót és az answer tételek átengedését.

- `3ed3bd3` **Step 6: Add question answering UI to dev web page**
  - Dev felület (`internal/httpserver/web/index.html`):
    - Új gomb a fejlécben és az ütemező panelen: `❓ Válaszra vár`.
    - `#questionsPanel` panel: az embereknek szóló nyitott és eszkalált kérdések listája szálhivatkozással, gyors választógombokkal a megadott opciókhoz, és szabad szöveges válaszadó űrlappal.
    - Chat integráció: `/answer Q-12 <válasz>` és `/answer <uuid> <válasz>` parancs a fő chat beviteli mezőjében.

- `[current]` **Step 7: Add e2e tests and documentation for question module**
  - Átfogó végponttól végpontig tartó szerződéstesztek (`internal/e2e/question_test.go`):
    - `TestQuestionMCPLifecycle`: `ask` (human) -> szálüzenet -> duplikált nyitott kérdés tiltása ugyanabban a szálban -> ember válaszol REST-en -> válasz a szálban és ébresztés a kérdező inboxában -> új kérdés feltehető a szálban -> `withdraw`.
    - `TestQuestionRedirectFullAccess`: teljes hozzáférésű agent (`mode="direct", sandbox=false`) kérdezésekor automatikus átirányítás emberekre (`to_human: true`, `to_actor_id: nil`).
    - `TestQuestionPermissions`: csak a címzett agent (vagy ember/admin) válaszolhat; idegen agent válasza elutasítva; idegen actor nem vonhatja vissza a kérdést.
    - `TestQuestionEscalation`: agentnek szóló kérdés 30 perc után automatikusan eszkalálódik a háttérciklusban, szálüzenet értesíti az adminokat (`@norbi`), majd bármely ember megválaszolhatja.
    - `TestQuestionDeadlineFallback`: határidős és alapértelmezett válasszal rendelkező kérdés határidőkor automatikusan lezárul az alapértelmezéssel, és a kérdező inboxába kézbesül.
    - `TestQuestionToHumanNeverResolves`: az embernek szóló kérdés sosem eszkalálódik vagy jár le automatikusan.
    - `TestQuestionRESTAPIErrors`: OpenAPI validáció ismeretlen kérdésre (404), üres válaszra (400) és már megválaszolt kérdésre (409).
  - Dokumentáció: `docs/02-muszaki-specifikacio.md`, `docs/01-terv.md`, `CLAUDE.md`.

---

## 2. Érintett fájlok jegyzéke

- `migrations/00012_question.sql` – adatbázis migráció.
- `api/openapi.yaml` – kérdés REST sémák és végpontok.
- `sqlc.yaml`, `.golangci.yml` – konfigurációs frissítések.
- `internal/apispec/apispec.gen.go` – generált specifikációs típusok.
- `internal/modules/question/model.go` – domain modell, formázott azonosítók.
- `internal/modules/question/events.go` – esemény definíciók.
- `internal/modules/question/store.go` – Store port felület.
- `internal/modules/question/memory/store.go` – memóriabeli adapter.
- `internal/modules/question/postgres/store.go` – PostgreSQL adapter.
- `internal/modules/question/postgres/queries.sql` – sqlc lekérdezések.
- `internal/modules/question/postgres/sqlcgen/` – generált adatbázis kódok.
- `internal/modules/question/service.go` – üzleti logika, háttérciklus (`RunDue`, `Loop`).
- `internal/modules/question/httpapi/handlers.go` – REST kezelők.
- `internal/modules/question/mcptools/tools.go` – MCP eszközök (`ask`, `answer`, `withdraw`).
- `internal/modules/identity/service.go`, `api.go` – `Admins` lekérdezés támogatása.
- `internal/modules/hosts/service.go`, `api.go` – `IsFullAccess` ellenőrzés.
- `internal/modules/delivery/model.go`, `service.go` – `ItemAnswer` integráció.
- `internal/modules/conversation/service.go`, `api.go` – `MessageByID` olvasó.
- `internal/guard/direct.go`, `direct_test.go` – őr válasz-engedélyezés és prompt kiegészítés.
- `internal/server/server.go` – szerver oldali bekötés és háttérciklus indítás.
- `internal/storetest/storetest.go`, `backends_test.go` – adattár szerződéstesztek.
- `internal/httpserver/web/index.html` – fejlesztői felület és `/answer` parancs.
- `internal/e2e/question_test.go` – végponttól végpontig tartó e2e tesztek.
- `docs/02-muszaki-specifikacio.md` – műszaki leírás frissítése.
- `docs/01-terv.md`, `CLAUDE.md` – tervek és ütemezés jelölése.

---

## 3. Üzleti és biztonsági szabályok érvényesítése

| Szabály | Megvalósítás | Teszt |
|---|---|---|
| Teljes hozzáférésű agent átirányítása | `service.Ask`: ha a cél agent `mode == "direct" && !sandbox`, automatikusan `to_human: true, to_actor_id: nil` | `TestQuestionRedirectFullAccess` |
| Egy nyitott kérdés szálanként és kérdezőnként | `service.Ask`: `CountOpenByThreadAsker > 0` esetén `ErrQuestionAlreadyOpen` | `TestQuestionMCPLifecycle` |
| Megcélzott agent válaszolási joga | `service.Answer`: csak a cél agent (vagy admin) válaszolhat | `TestQuestionPermissions` |
| Embereknek szóló kérdés válasza | `service.Answer`: bármely emberi felhasználó vagy admin válaszolhat | `TestQuestionMCPLifecycle` |
| Eszkalált kérdés válasza | `service.Answer`: `q.State == StateEscalated` esetén csak ember válaszolhat | `TestQuestionEscalation` |
| Visszavonási jogosultság | `service.Withdraw`: kizárólag a kérdező (`q.AskedBy`) vonhatja vissza | `TestQuestionPermissions` |
| 30 perces eszkaláció | `service.RunDue`: 30 perc után `escalated` állapot és adminok említése a szálban | `TestQuestionEscalation` |
| Határidős alapértelmezett lezárás | `service.RunDue`: határidő lejártakor lezárás `default_option`-nel és inbox kézbesítés | `TestQuestionDeadlineFallback` |
| Emberi kérdés nem dől el magától | `Due` lekérdezés szűrője: `WHERE to_human = false` | `TestQuestionToHumanNeverResolves` |
| Közvetlen őr ébresztése válasz esetén | `guard.humanOnly`: engedélyezi az `it.Kind == "answer"` tételeket | `TestHumanOnly`, `TestQuestionMCPLifecycle` |

---

## 4. Lefuttatott ellenőrzések és eredmények

1. **`make test`**
   - Eredmény: `PASS`
   - Valamennyi belső csomag tesztjei (e2e, guard, identity, conversation, delivery, question, wiki, storetest) hiba nélkül lefutottak.
2. **`make lint`**
   - Eredmény: `0 issues`
   - A `golangci-lint` ellenőrzés (beleértve a `depguard` rétegzési szabályokat és a formázást) hiba nélkül teljesül.
3. **`make test-pg`**
   - Eredmény: `PASS`
   - Valamennyi PostgreSQL adattár-teszt (beleértve a `TestPostgres/question`-t) hibátlanul zöld.
4. **`make gen-check`**
   - Eredmény: `git diff --exit-code` tiszta; az OpenAPI és sqlc kódgenerálás teljesen naprakész.
5. **Versenyhelyzet-vizsgálat (`go test -race ./internal/guard ./internal/e2e`)**
   - Eredmény: `PASS` adatverseny (race condition) nélkül.
