# Handoff Report: WebSocket-ébresztés az őrhöz (WebSocket Wake)

Dátum: 2026-10-16  
Ág: `feature/ws-wake`  
Szerző: Antigravity CLI  
Felülvizsgáló: Claude Code  

---

## 1. Változtatások commitonként

- `33c45f0` **Add fallback_poll and disable_ws to guard config**
  - Új `FallbackPoll` (`time.Duration`, YAML: `fallback_poll`) és `DisableWS` (`bool`, YAML: `disable_ws`) mezők hozzáadása az `AgentConfig` struktúrához (`internal/guard/config.go`).
  - Az `AgentConfig.defaults()` mostantól alapértelmezetten 60 másodpercre állítja a `FallbackPoll` értékét (a meglévő 5 másodperces `Poll` megőrzése mellett).
  - A `guard.example.yaml` mintafájl kiegészítése a `fallback_poll: 60s` leírással a `defaults` sorban.
  - Új egységteszt (`TestAgentConfigDefaults`) az alapértelmezések ellenőrzésére.

- `8bc85c5` **Implement WebSocket wake for guard agents**
  - Új WebSocket figyelő komponens (`internal/guard/ws.go`):
    - URL feloldás és séma-konverzió (`http` → `ws`, `https` → `wss`) a `/ws/ui` végponthoz.
    - `coder/websocket` kliens beállítása úgy, hogy az `http.Client.Timeout` ne bontsa le a tartós kapcsolatot 30 másodperc után (a `Transport` és a `Jar` megmarad, a kliens szintű `Timeout` 0-ra állítva).
    - `hello` keret küldése az agent bearer tokenjével, `last_seq` nélkül (fejtől indulva).
    - Bejövő `event` és `resync_required` keretek azonnali ébresztő jelzést küldenek a belső `wakeCh` csatornára; a `welcome` és `replay_done` keretek ignorálva vannak.
    - Exponenciális visszalépés (`wsBackoff`: 1s, 2s, 4s, 8s, 16s, 30s-nál plafonozva, túlcsordulás-védelemmel) szétkapcsolódáskor.
    - Token-megújulás figyelése: gép módban a `hostTokens` 10 perccel a lejárat előtt új tokent kér a szervertől; a WebSocket figyelő háttér-időzítője észleli az új tokent, `StatusNormalClosure` kóddal lezárja a kapcsolatot, és azonnal, visszalépési várakozás nélkül újrakapcsolódik az új tokennel.
    - Minden kapcsolódás, szétkapcsolódás és újracsatlakozási kísérlet kizárólag debug szinten naplózódik, a token sosem kerül a logokba.
  - Az `Agent.Run` ciklus átalakítása (`internal/guard/guard.go`):
    - A fix 5 másodperces alvás helyett a ciklus `select`-tel vár a `wakeCh` ébresztő jelzésére vagy a `fallback_poll` (alapértelmezetten 60s, hibák esetén visszalépő) időzítőre.
    - Induláskor egy kezdeti jelzést helyez a `wakeCh`-be, így induláskor vagy újrainduláskor az agent azonnal feldolgozza a várakozó inbox tételeket.
    - Kikapcsolt WebSocket (`DisableWS: true`) esetén megtartja az eredeti 5 másodperces `poll` alapú lekérdezést.
    - A `watchHalt` biztonsági mechanizmus érintetlen: a futások alatt továbbra is 2 másodpercenként REST-en ellenőrzi a `/me` állapotot.
  - Egységtesztek (`internal/guard/ws_test.go`):
    - `TestWSBackoffBounds`: visszalépési határok ellenőrzése [1s, 30s] tartományban.
    - `TestWSTokenChangeTriggersReconnect`: fake token forrással igazolva, hogy a token váltása azonnali újracsatlakozást vált ki az új tokennel.
    - `TestWSWakeOnEvents`: igazolja, hogy az `event` és `resync_required` keretek ébresztenek, míg a `welcome` és `replay_done` keretek nem.

- `d6c78d6` **Add e2e tests for WebSocket wake and fallback polling**
  - Új végponttól végpontig tartó tesztek (`internal/e2e/guard_ws_test.go`):
    - `TestGuardWebSocketWakeReactsFast`: 60 másodperces `fallback_poll` mellett egy megemlítés érkezésekor az agent a WebSocket esemény hatására kevesebb mint 2 másodpercen belül (< 100 ms a gyakorlatban) felébred és válaszol.
    - `TestGuardFallbackPollPicksUpWhenWSUnavailable`: érvénytelen WS token hookkal szimulálva a WebSocket elérhetetlenségét; a rövid tartalék `fallback_poll` időzítő így is megbízhatóan felveszi és megválaszolja az üzenetet.

- (jelenlegi) **Docs: mark WebSocket wake as implemented**
  - `docs/02-muszaki-specifikacio.md`: az Őr fejezetben az 5 másodperces lekérdezés leírásának cseréje a WebSocket-ébresztés specifikációjára.
  - `CLAUDE.md`: az „Eltérések a tervtől” szakasz frissítése (az őr már nem 5 mp-enként pollol, hanem WebSocketen figyel, a 60 mp-es poll csak tartalék).

---

## 2. Hozott döntések (ahol a specifikáció nyitott volt)

1. **`http.Client.Timeout` elkerülése a WebSocket tárcsázásnál:**  
   A `coder/websocket.Dial` a háttérben Go `net/http` kérést indít. Ha a megadott `http.Client` rendelkezik `Timeout`-tal (a Supervisor esetén ez 30s), a Go HTTP kliens a timeout lejártakor lezárja a kapcsolatot, még sikeres WebSocket upgrade után is. Ezért az őr a `wsDialClient()` segítségével másolatot készít, megtartva a meglévő `Transport`-ot és `Jar`-t, de a `Timeout` értékét 0-ra állítja.

2. **Induláskori azonnali inbox-ellenőrzés:**  
   A WebSocket protokoll a fejtől indul (`last_seq` nélkül), így nem játssza vissza a kapcsolat létrejötte előtt érkezett korábbi eseményeket. Ha az agent indításakor vagy újraindításakor már várakozott üzenet az inboxban, az azonnali jelzés nélkül 60 másodpercig (az első `fallback_poll` lejártáig) feldolgozatlan maradt volna. Ezért az `Agent.Run` a `wakeCh` inicializálásakor elhelyez egy kezdeti ébresztő jelzést, ami azonnal elindít egy `Tick`-et.

3. **Exponenciális visszalépés újrakezdése aktív kapcsolat után:**  
   Ha a kapcsolat sikeresen felépült és fogadta a kereteket, de később hálózati hiba miatt megszakad, a következő újracsatlakozási kísérlet újra az 1 másodperces alapértékről indul (`attempt = 0`), nem a korábbi hibák felhalmozott idejével.

4. **Azonnali újracsatlakozás token forgatásakor:**  
   Gép módban a tokenek 1 óráig érvényesek, és a `hostTokens` 10 perccel a lejárat előtt megújítja őket. Amikor a háttérben futó token-ellenőrző észleli a megújult tokent, a kapcsolatot normál lezárással bontja, és a ciklus azonnal, visszalépési alvás nélkül kapcsolódik újra az új tokennel.

5. **Adatverseny-megelőzés a naplózásban (`-race` tisztaság):**  
   Az `Agent.Tick` futása során frissíti az `a.me` mezőt. Hogy elkerüljük az adatversenyt a háttérben futó WebSocket figyelő naplózásával, a `watchWS` a figyelő indulásakor egyszer olvassa ki a nevet (`agentName`), így a párhuzamos írás és olvasás teljesen kizárt.

---

## 3. Ami kimaradt vagy szándékosan nem készült el

- **Élő AI hívások:** A szabályoknak megfelelően valódi `claude` vagy `agy` bináris nem futott; minden tesztelés determinisztikus hamis runnerrel és szkriptelt adapterekkel történt.
- **Események sorszám szerinti visszajátszása:** A specifikáció explicit előírása alapján az őr nem használ `last_seq`-et a `hello` keretben, hanem a fejtől indul; minden esemény csupán jelzésként szolgál az inbox REST végpontjának lekéréséhez.

---

## 4. Lefuttatott parancsok és eredményeik

1. **`make test`**  
   - Eredmény: `PASS` minden modulra és belső csomagra.
2. **`make lint`**  
   - Eredmény: `0 issues` (a depguard rétegszabályokkal és bodyclose ellenőrzéssel együtt).
3. **`make test-pg`**  
   - Eredmény: `PASS` a PostgreSQL tárolótesztekre.
4. **`make gen-check`**  
   - Eredmény: Tiszta, a generált kódok naprakészek.
5. **`go test -count=1 -race ./internal/guard ./internal/e2e`** (kétszer egymás után futtatva):  
   - Eredmény: Mindkétszer hibátlanul, adatverseny nélkül lefutott (`ok colonyhall/internal/guard`, `ok colonyhall/internal/e2e`).
