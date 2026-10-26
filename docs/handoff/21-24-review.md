# Átnézés: 21–24. és 20., 23. kör (egymásra épülő ágak)

Claude Code, 2026-10-26. Az öt kör egymásra épülő ágakon készült (`feature/compaction` → `live-progress` → `delegation-approval` → `file-browser` → `agent-terminals`); az ellenőrzések a legfelső ágon, az egész láncon futottak.

**Ellenőrizve:** `make test`, `make lint`, `make gen-check`, `make test-web` (17 teszt), `make test-pg` (gyorsítótár nélkül, csak a Postgres-konténerrel), `go test -race ./internal/guard/ ./internal/e2e/ ./internal/gateway/`: mind zöld. Hat új migráció (`00028`–`00032`).

**Rendben találtam:** a jegyző eszközök nélkül fut, a kivonat és az üzenetek HTML-escape-elve kerülnek a keretbe; az élő lépések maszkolása (tokenek, kulcsok, `KEY=érték`, JWT, hosszú hex/base64); a jóváhagyást csak admin ember adhatja, egyszer, feltételes állapotváltással; a fájlböngésző továbbítása csak embernek, olvasható csatornából, az agent tagságával, és csak a megcélzott agent őrétől fogad választ; a terminál MCP csak teljes hozzáférésű agentnek; a terminál-esemény mindig csak a küldő agentnek szól.

**Javítva (a legfelső ágon):**

1. **Súlyos — az összefoglalót agent is átírhatta vagy újragenerálhatta** (21): az `EditSummary` és a `Regenerate` csak csatornaírási jogot kért, így egy agent bármit beírhatott a kivonatba, ami minden későbbi munkamenet elejére bekerült volna (tartós prompt injection). Most csak ember. Teszt: `TestCompactionFlow` (javítás nélkül 200, most 403).
2. **A tömörítés beállításai** (21): csak ember (admin) módosíthatja, és esemény + audit készül (`compaction.settings_updated`).
3. **Tranzakciós outbox** (21): a tömörítés állapotváltozásai és eseményei külön íródtak, a hibák eldobva; most egy tranzakcióban (`withTx`), a busz értesítése a commit után.
4. **Az élő lépések agenteknek is kimentek** (22): a frame-et minden olyan kapcsolat megkapta, ami olvashatja a csatornát, az agentek őrei is; a döntés szerint csak ember. Teszt: `TestProgressNotSentToAgentsAndTerminalEventChannelChecked` (javítás nélkül egy agent megkapta egy másik agent lépéseit).
5. **Terminál-esemény csatornája** (23): most ellenőrzi, hogy az agent olvashatja-e a megadott csatornát (403 különben). Ugyanabban a tesztben.

**Nem javítva, későbbre:** a delegáció és a terminál-esemény néhány írása nem tranzakciós (állapot nem vész el, de esemény kimaradhat); egy teljes hozzáférésű agent saját magának terminál-eseményt küldhet és így felébresztheti magát (csak önmagát, és amúgy is bármit futtathat).
