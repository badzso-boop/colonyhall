# Handoff Report: Jóváhagyás a chatből (Approvals from Chat)

Dátum: 2026-10-16  
Ág: `feature/approvals`  
Szerző: Antigravity CLI  
Felülvizsgáló: Claude Code  

---

## 1. Változtatások commitonként

- `c3ff8ec` **Add approvals migration and OpenAPI contract**
  - Új adatbázis-migráció (`migrations/00009_approvals.sql`): létrehozza az append-only `approval_request` táblát és kiegészíti az `agent_host` táblát az `approvals` oszloppal (`VARCHAR(20) NOT NULL DEFAULT 'none'`).
  - OpenAPI szerződés frissítése (`api/openapi.yaml`): új végpontok (`GET /api/v1/approvals/{approvalId}`, `POST /api/v1/approvals/{approvalId}`) és sémák (`ApprovalRequest`, `DecideApprovalRequest`, `ApprovalState`, `ApprovalDecision`), valamint `HostAgent.approvals` és `AssignRequest.approvals` mezők.
  - Generált kód elkészítése `make gen` futtatásával (`internal/apispec`, `internal/eventlog/postgres/sqlcgen`, stb.).
  - Linter (`.golangci.yml` `no-foreign-adapters`) és sqlc konfiguráció frissítése.

- `3067aa2` **Implement server approval and hosts modules**
  - Új szerveroldali modul: `internal/modules/approval/` a rétegzési szabályok szerint:
    - `model.go`: adatmodellek, állapotok (`pending`, `allowed`, `denied`, `expired`), döntési típusok (`allow`, `deny`), és `Redact` segédfüggvény.
    - `events.go`: `approval.requested` és `approval.decided` eseménydefiníciók.
    - `store.go`, `memory/store.go`, `postgres/store.go`: adattár-interfész és implementációk feltételes frissítéssel (`Decide` csak `pending` állapotú sorra fut le, törlés nincs).
    - `service.go`: üzleti logika, csatornaolvasási jogosultságok ellenőrzése, kérés létrehozása, thread-üzenet posztolása az agent nevében (`🔐 Approval needed: <tool_name> [approval:<id>]`), döntési várakozás (`WaitForDecision`), lejárat kezelése, eseménykezelő vészleállító aktiválásakor (`HandleEvent`).
    - `httpapi/handlers.go`: REST végpontok regisztrálása és kiszolgálása (`GET` és `POST /api/v1/approvals/{approvalId}`).
    - `mcptools/tools.go`: az `approve` MCP eszköz regisztrálása agentek számára; az `X-Colonyhall-Channel` és `X-Colonyhall-Thread` fejlécek alapján elindítja a jóváhagyási folyamatot, blokkol a döntésig/lejáratig/leállításig, majd Claude-kompatibilis JSON válasszal tér vissza (`{"behavior":"allow","updatedInput":...}` vagy `{"behavior":"deny","message":...}`).
  - `hosts` modul frissítése: `Approvals` mező beépítése a modellbe, eseményekbe, tárakba és a REST API-ba, valamint validáció (`chat` approvals csak `claude-code` kliensnél engedélyezett).
  - MCP szerver infrastruktúra: `RegisterTools` átadja a bejövő `*http.Request` referenciát, így az MCP eszközök hozzáférnek a kérés fejléceihez.
  - Tárolótesztek: `internal/storetest` kibővítése a jóváhagyási tárak tesztelésével (memória és PostgreSQL).

- `d84b1b0` **Add chat approvals support to guard**
  - `internal/guard/client.go`: `HostAgent` típus kiegészítése az `Approvals` mezővel.
  - `internal/guard/config.go`: `AgentConfig` kiegészítése az `Approvals` mezővel és alapértelmezéssel (`none`).
  - `internal/guard/host.go`: a Supervisor `reconcile` és `startAgent` folyamataiban az agent konfigurációjának kulcsa és beállítása figyelembe veszi az `Approvals` mezőt.
  - `internal/guard/adapter.go`: `DirectRequest` kiegészítése `PermissionPromptTool string` és `Env []string` mezőkkel.
  - `internal/guard/claudecode.go`: Claude direct módban felveszi a `--permission-prompt-tool` argumentumot, és átadja az extra környezeti változókat, miközben nem-sandboxos futásnál megőrzi az `os.Environ()` változókat.
  - `internal/guard/direct.go`: Claude direct módban, ha `approvals: chat`, beállítja a `--permission-prompt-tool mcp__colonyhall__approve` argumentumot, átadja az `MCP_TOOL_TIMEOUT=660000` környezeti változót (10 perc jóváhagyási időkorlát + 60 mp), az ideiglenes 0600-as MCP konfigurációba beírja a szál `X-Colonyhall-Channel` és `X-Colonyhall-Thread` fejléceit, a közvetlen futás kontextus-időkorlátját pedig megnöveli a jóváhagyási idővel (+11 perc).
  - Egységtesztek hozzáadása az argumentumok, a környezeti változók és az MCP fejlécek ellenőrzésére.

- `ab3740e` **Add approvals UI to dev page**
  - `internal/httpserver/web/index.html`:
    - `[approval:<id>]` formátum felismerése a chatben; embereknek „Részletek” gomb (modális ablakban eszköznév és bemenet), valamint függő kérésnél „Engedélyezem” és „Elutasítom” gombok; döntés után a végleges állapot kijelzése.
    - Új `#appModal` dialógus eszközadatok áttekintésére és azonnali jóváhagyásra/elutasításra.
    - Automatikus frissítés WebSocket eseményekre (`approval.decided`, `approval.requested`).
    - A „Gépek” hozzárendelési űrlapján `approvals` legördülő mező (`none`, `chat`), valamint meglévő hozzárendeléseknél a beállítás megjelenítése.

- `4a60f50` **Add e2e tests for chat approvals**
  - `internal/e2e/approval_test.go`:
    - `TestApprovalAllowFlow`: valós MCP végponton agent munkamenet egyedi fejlécekkel (`X-Colonyhall-Channel`), szálüzenet ellenőrzése, ember általi jóváhagyás, MCP allow JSON válasz ellenőrzése, végleges állapot ellenőrzése.
    - `TestApprovalDenyFlow`: elutasítási folyamat, emberi indoklás továbbítása Claude-nak.
    - `TestApprovalExpiryFlow`: időtúllépés ellenőrzése a léptethető órával (`e.clock.Advance`), automatikus lejárat és deny válasz.
    - `TestApprovalAuthorizationRules`: agent 403 ellenőrzése olvasásra és döntésre; csatornatagsággal nem rendelkező ember 403 ellenőrzése privát csatornában; második döntési kísérlet esetén 409 Conflict ellenőrzése.
    - `TestApprovalEmergencyStopDuringWait`: várakozás közbeni vészleállító aktiválásakor azonnali deny válasz és denied/expired állapot ellenőrzése.
    - `TestApprovalSecretRedaction`: érzékeny tokenek (`col_...`) automatikus kitakarásának ellenőrzése az `input` mezőben.

---

## 2. Hozott döntések (ahol a specifikáció nyitott volt)

1. **MCP hívások és HTTP fejlécek átadása:** Az MCP SDK `mcp.RequestExtra.Header` mezőjét használjuk, amelyet a Colonyhall saját MCP stream transport rétege tölt fel a bejövő HTTP kérés fejléceiből. A fakeagent tesztkliens egy egyedi `http.RoundTripper` wrappert (`headersTransport`) kapott, amellyel bármilyen egyedi fejléc egyszerűen injektálható a tesztekben.
2. **Környezeti változók kezelése közvetlen (unsandboxed) futásnál:** Amikor a Claude Code direct módban fut, a folyamat nem kap `bwrap` homokozót. Annak érdekében, hogy az `MCP_TOOL_TIMEOUT` környezeti változó beállítása ne törölje a rendszerszintű környezeti változókat (pl. `PATH`, `HOME`, Claude bejelentkezési adatok), a kód `wrap == nil` esetén `os.Environ()`-ból indul ki és ahhoz fűzi hozzá az új változókat.
3. **Időkorlátok összehangolása:** A specifikáció szerint a jóváhagyási időkorlát 10 perc. Az `MCP_TOOL_TIMEOUT` értéke erre 60 másodperc ráhagyást kap (11 perc = 660 000 ms), az őr közvetlen futási időkorlátja pedig `approvals == "chat"` esetén automatikusan kiegészül ugyanezzel a 11 perces időtartammal, megelőzve a futás idő előtti leállítását jóváhagyásra várakozás közben.
4. **Vészleállító kezelése:** A vészleállító aktiválásakor az `approval.Service` eseménykezelője a munkaterület összes függő kérését `expired` („emergency stop”) állapotra állítja. Emellett a `WaitForDecision` ciklus minden iterációban ellenőrzi az agent aktuális állapotát (`actor.Halted`), így azonnal elutasítja a várakozó futást („agent halted”).

---

## 3. Ami kimaradt vagy szándékosan nem készült el

- **Élő AI CLI hívások:** A projekt szabályai szerint valódi `claude` vagy `agy` parancs nem futott; a tesztelés determinisztikus MCP szimulációval és mock runnerrel történt.
- **Merge a `main` ágra:** Az utasításoknak megfelelően a változtatások kizárólag a `feature/approvals` ágon találhatók, nem kerültek merge-elésre vagy pusholásra.

---

## 4. Lefuttatott ellenőrzések és eredményeik

1. **`make gen-check`**
   - Eredmény: Tiszta, a generált kód (`apispec`, `sqlcgen`) naprakész.
2. **`make test-pg`**
   - Eredmény: `PASS`. Minden tárolóteszt, beleértve az új `TestPostgres/approval` tesztet is, sikeresen lefutott PostgreSQL ellen.
3. **`go test -count=1 -race ./internal/guard ./internal/e2e`**
   - Eredmény: `PASS`. A guard és az e2e csomagok teljes versenhelyzet-ellenőrzése (race detector) zöld.
4. **`make test`**
   - Eredmény: `PASS`. Az összes belső csomag egység-, szerződés- és végponttól végpontig tartó tesztje sikeres.
5. **`make lint`**
   - Eredmény: `0 issues`.
