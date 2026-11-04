# 38. kör jelentése: kolléga saját agentekkel és saját Claude-előfizetéssel

Ág: `feature/colleague` (worktree: `/mnt/g/Projects/new-chat-worktrees/colleague`), 2026-11-04. A részek külön ágakon készültek (`feature/colleague-channels`, `-docsonly`, `-guards`, `-login`), és ide olvadtak be. Migrációk: `00046_owner.sql`, `00047_docs_only_agent.sql`.

## Commitok

| Commit | Rész | Tartalom |
| --- | --- | --- |
| `ddea310` | – | a handoff prompt |
| `2041ea5` | 1 | agent- és géptulajdonos, láthatóság (404), `owner_id` szűrő, csak a tulajdonos/admin ébreszt teljes hozzáférésű agentet |
| `fa86caa` | 2 | `PATCH /channels/{id}/members/{actorId}` (`can_write`) |
| `dd33b60` | 2 | minden csatornához kötött írás tiltva az olvasó tagnak |
| `a744f82` | 2 | PWA: „Csak olvashat” / „Írhat” tagonként |
| `f511ce2` | 3 | `write_scope: docs`, pár, fix utasításblokk (szerver) |
| `fa54ed9` | 3 | az őr Claude Code jogosultsági szabályokkal indítja |
| `403e031` | 3 | PWA: a csak-dokumentáció mód a varázslóban |
| `3ebbee0` | 3 | dokumentáció a csak-dokumentáció módhoz |
| `b1f62bd` | 4 | Claude-belépés: relé PWA → szerver → őr, az őr minden Claude-futásnál használja |
| `c455e38` | 5 | két őr egy gépen nem ütközik |
| `3ccd21d` | 5 | `max_concurrent_runs` |
| `5342f1d` | 5 | runbook: `docs/10-kollega-beallitasa.md` |
| `077a25c` | – | teszt-javítás a beolvasztás után |
| `f46d379` | 6 | PWA: Gépek tagoknak, tulajdonos, Claude-belépés ablak |
| `3fb37c4` | 6 | PWA: tulajdonos a varázslóban, tag csak a saját agentjeit látja |
| `b647487` | 6 | PWA: más személy agentje 404 helyett nyugodt üzenettel |
| `26d56b4` | 6 | PWA: „Személy” szűrő a Futások oldalon |
| `227cdf5` | – | `docs/01`, `02`, `06`, `task.md` |

## Döntések, ahol a prompt nyitva hagyta

- **Tulajdonos módosítása:** `PUT /agents/{id}/owner` és `PUT /hosts/{id}/owner` (külön végpont, `owner_id` hiánya = senki). A meglévő agent tulajdonosa a felületen nem módosítható (Norbi döntése: új agentet hoz létre); a gépé igen (Gépek → Áttekintés, csak a workspace tulajdonosa).
- **A tulajdonos többet lát az áttekintőben,** mint egy tag: a gépet, a jegyzeteket és az inboxot is (ahogy eddig csak admin); a csatornákhoz kötött részek (szálak, futások, kérdések) továbbra is a csatorna olvashatóságához kötöttek.
- **404 vs 403:** csatornahozzáférés hiánya előbb 403 (a régi viselkedés), a nem saját agent 404.
- **Jóváhagyás eldöntése** is a tulajdonosé vagy adminé (egy eszközhasználat engedélyezése feladatadás), és írási jog kell hozzá a csatornában.
- **DM egy személytől más agentjével:** 403 (a prompt: „DM csak a saját agentjeinek”).
- **Olvasó agent tag nem kap inbox-elemet** (különben futna, és nem tudna válaszolni).
- **Claude-belépés tokenfájl alapértéke** host módban: `claude.token` a géptoken mellett; a hiányzó fájl = nincs token (régen indítási hiba volt). A `HostReport` csak `claude_login_configured` és `claude_login_set_at` mezőt kap.
- **Sebességkorlát:** 5 próbálkozás / 10 perc / személy, memóriában.
- **A PWA a tokent nem tartja meg:** a mező küldéskor kiürül, a mutáció `gcTime: 0` és `reset()`, teszt ellenőrzi, hogy a cache-ben és az oldalon nincs.
- **`tmux_socket`** alapértéke `colonyhall` maradt (a tmux felhasználónként külön könyvtárat használ, így két felhasználó őre nem ütközik).
- **Agentek mappaengedélye:** a csak-dokumentáció agent `--restricted` alatt fut: a felhasználó és a projekt `.claude/` beállításait figyelmen kívül hagyja, így a sandboxos pár nem írhat neki engedélyt.

## A token útja, és hogy a szerveren nincs nyoma

1. PWA: `PUT /hosts/{hostId}/claude-login {token}` (jelszómező; a mező azonnal kiürül).
2. Szerver: a kérést naplózó middleware csak a metódust és az útvonalat írja; `hosts.LoginRelay` a tokent egy memóriabeli `loginPending` struktúrában tartja, adatbázisba, eseménybe, auditba nem ír.
3. `/ws/host`: csak `{"type":"host.claude_login.request","request_id":…}`.
4. Őr: `GET /api/v1/host/claude-login/{id}` (géptoken) → a token **egyszer**; a második olvasás már token nélküli. A hívás végén (siker, hiba, időtúllépés) a függő kérés törlődik.
5. Őr: `claude -p /usage …` a jelölt tokennel a gyermekfolyamat környezetében; ha elfogadja, `claude.token` (`0600`, atomikus), különben a régi marad.
6. Őr → szerver: `{ok, error_code?, set_at?}`; a szerver `host.claude_login_set` eseményt ír (ki, melyik gép).

Bizonyíték: `internal/e2e/claude_login_test.go` (`TestClaudeLoginRelayNeverStoresTheToken`) a teljes kört lejátssza egy hamis őrrel, és ellenőrzi, hogy a token nincs egyetlen eseményben (az audit ezekből épül), a gépek listájában, a szerver debug-szintű naplójában, és a relé memóriájában sem marad (`LoginRelay.Pending() == 0`). A Postgres-tárakba a szerver kódja nem ír tokent (nincs rá oszlop, a relé nem kap tárolót). Őroldali tesztek: `internal/guard/claudelogin_test.go` (hibás ellenőrzésnél a régi marad, `0600`, a direct/decide futás és a sandbox a tokent kapja, a belépési fájl másolata eltűnik). PWA: `web/src/features/hosts/components/HostAccess.test.tsx`.

## A csak-dokumentáció mód kézi ellenőrzése

**Nem futott.** Az alapszabályok tiltják a valódi `claude` futtatását, és Norbi még nem engedélyezte (kb. 5 rövid `haiku` futás az ő előfizetésén). A recept (egy üres próbamappában, a Claude Code 2.1.x-szel):

```sh
D=$(mktemp -d /tmp/docsonly.XXXXXX); cd "$D"; echo x > keep.txt
S='{"disableAllHooks":true,"permissions":{"allow":["Edit(/'"$D"'/**/*.md)","Write(/'"$D"'/**/*.md)","mcp__colonyhall"],"defaultMode":"dontAsk","deny":["Bash","NotebookEdit","PowerShell","Agent","WebFetch","WebSearch"],"disableBypassPermissionsMode":"disable"}}'
run() { timeout 300 claude -p "$1" --restricted --permission-mode dontAsk --permission-prompts none \
  --tools Read,Write,Edit,Glob,Grep --disallowedTools Bash,NotebookEdit,PowerShell,Agent,WebFetch,WebSearch \
  --allowedTools mcp__colonyhall --settings "$S" --strict-mcp-config --no-session-persistence \
  --output-format stream-json --verbose --model haiku </dev/null > "$2.jsonl"; echo "exit=$? $2"; }
run 'Create notes.md containing the word hello. Use the Write tool.' md
run 'Create main.go containing "package main". Use the Write tool.' go
run 'Change keep.txt so it says y. Use the Edit tool.' edit
run 'Run the shell command: touch pwned.txt' bash
run 'Create ../outside.md containing hello.' outside
ls -la "$D"; cat notes.md
```

Elvárt: `notes.md` létrejön; `main.go`, `pwned.txt`, `../outside.md` nem; `keep.txt` változatlan; minden futás magától véget ér, jóváhagyási kérdés nélkül. A kapcsolók és a szabálysorrend tesztje: `internal/guard/docsonly_test.go`.

## A runbook próbája (ami helyben tesztelhető)

- A homelabhoz nem nyúltam. A runbook lépései közül helyben bizonyított: két őr egy folyamatban, külön HOME-mal semmilyen mappán, socketen és porton nem osztozik (`internal/guard/isolation_test.go`); a `guard.yaml` mezői (`claude_token_file`, `preview_ports`, `max_concurrent_runs`) validálódnak; az őr `claude.token`-t a géptoken mellé teszi alapból; a belépési relé, a tulajdonos és a láthatóság e2e-tesztben.
- Nem próbált: `adduser`, `loginctl enable-linger`, `machinectl shell`, a Claude Code natív telepítője a kolléga felhasználójánál. Ezek a homelabon, Norbi által futnak.
- Fontos lépés a runbookban: Norbi saját könyvtára ne legyen mások által olvasható (`chmod 750`), mert a tervező agent a kolléga felhasználójának jogaival olvas.

## Eredmények

A beolvasztott `feature/colleague` ágon, a `227cdf5` állapoton:

- `make test`: zöld.
- `make lint`: 0 hiba.
- `make test-pg` (Postgres, `make dev` mellett, utána `make down`): zöld (`internal/storetest` 4,2 s), benne az új tárfüggvények (`SetActorOwner`, `SetHostOwner`, a `RunFilter.AgentIDs`, `SetMemberCanWrite`, `write_scope`/`partner_id`).
- `go test -race ./internal/guard/ ./internal/e2e/ ./internal/gateway/`: zöld (17 s, 153 s, 1 s).
- `make gen-check`: tiszta.
- `make test-web`: zöld (81 tesztfájl, 522 teszt) a PWA első két commitja után; a későbbi PWA-commitok érintett tesztjei (`features/agents`, `features/admin`) külön zöldek.

## Élesítési sorrend

1. **Szerver** (két migráció indításkor fut): `git pull && docker compose -f deploy/compose.yaml up -d --build` a homelabon, `curl …/readyz`.
2. **Minden meglévő őr** (PC és homelab): az új őr az új végpontokat használja (`window: machine_busy` jegyzet, `host.claude_login.request`); futó agentmunka alatt ne.
3. **Az új őr** csak a runbook után (`docs/10-kollega-beallitasa.md`).
4. A PWA a szerverrel együtt frissül (beágyazott build).

## Amit a reviewer először nézzen

1. **Láthatóság (1. rész):** `identity.Actor.SeesAgent`, és minden helye: `internal/server/overview.go`, `internal/modules/runs/service.go` (`seesAgent`, `agentScope`), `runs/usage.go`, `files/service.go` (négy hely), `approval/service.go`, `notes/service.go`, `gateway/gateway.go` (lépések), `delivery/service.go` (ébresztés), `conversation/dm.go`, `question/service.go`. Tesztek: `internal/e2e/owner_test.go`.
2. **A token relé (4. rész):** `internal/modules/hosts/loginrelay.go`, `hosts/httpapi/login.go`, `internal/guard/claudelogin.go`, `claudecode.go` (`token()`), `sandbox.go` (`claudeToken()`), `usage.go`.
3. **A csak-dokumentáció szabályok (3. rész):** `internal/guard/claudecode.go` (`docsOnlyArgs`, `DocsOnlySettings`, `ErrDocsOnlyUnsupported`), és a kézi ellenőrzés fenti receptje.

## Kimaradt

- A homelabon semmi (Linux-felhasználó, őrtelepítés, beállítás): Norbi futtatja a runbookot.
- Antigravity-belépés személyenként, Team-előfizetés, személyenkénti erőforráskvóta a `max_concurrent_runs`-on és az agentenkénti napi kereten túl.
- Meglévő agent tulajdonosának módosítása a felületen (Norbi döntése), az agentek száma a Felhasználók oldalon.
- A régi fejlesztői oldal (`internal/httpserver/web/index.html`) nem kapta meg az új funkciókat.
- Olvasóra állított tag futó munkáját az őr nem állítja le (csak `member_removed`-re); a futás a válasznál bukik el.
- **Gyanított régi hiba (nem javítva, nem ellenőrizve):** a `colony terminal-mcp` a terminálok állapotát a `<sandbox_dir>/<handle>/terminals` alá írja (`-state <agent mappa>`), a felügyelő viszont a `<state>/terminals`-t figyeli (`host.go`, `filepath.Dir(SandboxDir)`). Ha így van, az agent által indított terminál befejezése nem ébreszt, és vészleállításkor nem áll le.

## Átnézés (Claude Code, 2026-11-04)

- **A csak-dokumentáció mód kézi ellenőrzése lefutott** (Claude Code 2.1.317, haiku, a jelentés receptjével, egy üres próbamappában): a `notes.md` létrejött („hello”); a `main.go`, a `keep.txt` módosítása és a `../outside.md` elutasítva (`permission_denials`); shell-eszköz nem volt, a modell fájlírással próbálta, az is elutasítva; jóváhagyási kérdés egyik futásnál sem jött. Minden futás 0-s kóddal, magától ért véget.
- **Javítva, két hiányzó láthatósági szűrő** (teszttel, ami nélküle elbukik):
  - az előnézetek (`/previews`, `/previews/{id}`) a tag számára a más agentjeiét is mutatták; `TestPreviewsFollowAgentVisibility`;
  - a foglalások emberi listája (`/leases`) is; `TestLeasesFollowAgentVisibility`. Az agentek (MCP) nézete nem változott, az összehangoláshoz kell.
- **Kivéve a repóból:** a `web/tsconfig.tsbuildinfo` build-melléktermék; `*.tsbuildinfo` a `.gitignore`-ba.
