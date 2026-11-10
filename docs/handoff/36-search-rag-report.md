# 36. kör jelentése: keresés és RAG, szöveg (36a)

Ág: `feature/rag` (worktree: `/mnt/g/Projects/new-chat-worktrees/rag`), a `main` `566ab51`-ből. Nincs beolvasztva, nincs push.

> **⛔ Merge-blokkoló: a valódi magyar kiértékelő készlet hiányzik.** A készletet (30–50 szöveges kérdés valódi adatból, a repón kívül) Norbi és Claude Code közösen írja, és a `colonyhall rag-eval -set … -as norbi` futtatásával mérjük. **Amíg ez nincs meg és nem futott le, az ág nem olvasztható be.** A formátum: `docs/11-rag-terv.md`, „Mérés: a kiértékelő készlet”.

## Ami elkészült

A csatornák, szálak, DM-ek, a wiki, a döntésnapló és a tömörítési összefoglalók keresése **kulcsszó** (Postgres teljes szöveges keresés, magyar szótövezéssel és pontos szavakkal) és **jelentés** (pgvector, a Raspberry Pi beágyazója) szerint, a kettő **RRF**-fel egyesítve, a homelab **rerankerével** újrarendezve. A jogosultság a lekérdezésben, a rangsorolás előtt dől el. Felületek: `GET /api/v1/search`, a `search` MCP-eszköz minden agentnek, a PWA keresőoldala és az állapotlap kártyája. Parancsok: `rag-backfill`, `rag-reembed`, `rag-eval`. Egy kis ONNX-modellszerver (`deploy/embed`) fut majd a Pi-n és a homelabon.

## Commitok

| Hash | Leírás |
| --- | --- |
| `e654523` | Settle round 36 open points: decision event, existing events, message edits |
| `c6951f8` | Add the round 36 search and RAG design and runbooks |
| `75038a0` | Use the pgvector Postgres image in dev, CI and the installer |
| `6dfa978` | Add the search index store: chunks, full-text and pgvector |
| `cb2e418` | Index chat, wiki, decisions and summaries; add hybrid search |
| `4f47bba` | Add rag-backfill, rag-reembed and rag-eval |
| `03b9056` | Add the model server for the Pi embedder and the homelab reranker |
| `2e225b5` | Document the search: spec, operations, plan and a study guide |
| `0a4d485` | Add the search page, the palette entry and the status card to the PWA |
| (ez) | Add the round 36 report |

## A 0. rész (tervezési kapu) és a mérések

A terv (`docs/11-rag-terv.md`), a Pi (`docs/12`) és a reranker (`docs/13`) runbookja, és a `CREATE EXTENSION vector` lépés (`docs/05`) elkészült. **Norbi 2026-11-10-én a mérés nélkül indította a megvalósítást** („most nem kell mérni, csak a kód”), ezért a célgépeken mért számok hiányoznak; a dokumentumokban „⏳ Mérés hátravan” jelöli őket.

Amit a gépekről csak olvasva megnéztem (2026-11-10):

- **A Raspberry Pi még nincs beállítva.**
- **A homelab:** i5-2500S, 4 mag, **csak AVX (nincs AVX2)**, 7,4 GB RAM, ebből ~3,8 GB szabad. A reranker méretét ez szabja meg.
- **A PC-ben még a GTX 1050 (2 GB) van**, a 3060 nincs bent; a 49. kör nincs kész. A 36b ezért külön ágon, később jön.
- **Az éles adatbázis:** PostgreSQL 16.14 a `pgvector/pgvector:pg16` képpel; a `vector` 0.8.5, a `pg_trgm` és az `unaccent` elérhető, egyik sincs telepítve; a `vector` nem *trusted* (superuser kell), a másik kettő igen. Adat: 568 üzenet, 31 wikioldal, 13 döntés, 1 tömörítés; az egész ~1 500 darab.

Amit a PC-n mértem (CPU, tájékoztató):

- **Félbehagyott modellmérés** (20 kitalált magyar kérdés, sok zavaró szöveggel): `multilingual-e5-small` int8 — első helyen 60%, első 5-ben 90%, első 20-ban 100%; csúcs-memória ~750 MB (a Pi 1 GB-jához szoros: a szerver kis kötegeket számol, és zram kell).
- **A teljes lánc valódi modellekkel**, a szintetikus készleten (Colonyhall → `deploy/embed/server.py` → Postgres + pgvector):

| konfiguráció | recall@5 | recall@10 | MRR@10 | nDCG@10 | p50 | p95 |
| --- | --- | --- | --- | --- | --- | --- |
| kulcsszavas | 1,000 | 1,000 | 0,944 | 0,955 | 1 ms | 2,2 ms |
| szemantikus | 1,000 | 1,000 | 1,000 | 1,000 | 8,6 ms | 9,4 ms |
| hibrid | 1,000 | 1,000 | 0,972 | 0,975 | 9,4 ms | 11,3 ms |
| hibrid + rerank | 1,000 | 1,000 | 0,972 | 0,979 | 607 ms | 661 ms |

A szintetikus készlet könnyű (18 kérdés), ezért minden konfiguráció jó: **a minőséget a valódi készlet méri.** A rerank ~0,6 mp a PC-n (`jina-reranker-v2` int8, 2 szál, ~20 jelölt); **a homelabon lassabb lesz**, ott mérni kell, és ha 2 mp fölött van, kisebb `COLONY_RERANK_TOP` vagy kisebb modell (`deploy/embed/MODEL.md`).

A CI-ban a szintetikus készlet a hamis beágyazóval fut (`cmd/colonyhall`, `TestSyntheticEvalSet`): az eszközt próbálja ki, a szemantikus számai nem mérvadók.

## Döntések, ahol a prompt nyitva hagyta

- **D1 – DM-ek:** a keresésben csak a DM tagjai látják, adminnak sem jön más DM-je (a prompt szerint). Ma a felületen az admin minden DM-et láthat.
- **D2 – wiki és döntések:** az egész workspace kereshet bennük (a kolléga is), ugyanúgy, mint ma a listákban.
- **D3 – visszamenőleges beágyazás a szerveren, a Pi-vel** (nem a PC-n): az adat kicsi. Az indexelő eseménynapló-fogyasztó, és egy új fogyasztó a napló elejéről indul, ezért **az első induláskor magától végigolvassa az előzményt**; a régi döntésekhez (nincs eseményük) egyszer kell a `rag-backfill`.
- **D4 – a 36b külön ágon**, a 39. és 49. kör után.
- **D5 – a Pi és a reranker token nélkül**, a hálózat szűri.
- **A modellszerver** saját, kicsi Python + ONNX Runtime program (`deploy/embed/server.py`), nem `llama.cpp`: a `llama.cpp` általános ARM64-es kiadása a Pi 3B processzorán valószínűleg nem indul, az ONNX Runtime-nak van hivatalos `aarch64` csomagja, és ugyanaz a modellfájl fut a PC-n is.
- **A kereshető csatornák listáját a szolgáltatás számolja** (ugyanaz a szabály, mint a felületen), és a lekérdezés tömbként kapja (`channel_id = ANY($1)`), nem `EXISTS (… channel_member …)`-ral: így a memóriabeli és a Postgres-tár ugyanazt a szerződést teljesíti. A szűrés így is a lekérdezésben, a rangsorolás előtt van.
- **A kulcsszavak VAGY-kapcsolatban:** a `websearch_to_tsquery` ÉS-e helyett, mert egy kérdés ritkán tartalmazza a válasz minden szavát; a `ts_rank_cd` a több egyezést előre teszi.
- **A modellcsere felismerése:** a beágyazó válaszában lévő modellnév (a fájl sha256-ának első 12 jegyével) és a tárolt vektorok modellje; eltérésnél a munkás nem ír, az állapotlap szól, a `rag-reembed` vált.
- **Az állapotlap nem a nyers hibát mutatja**, csak az osztályát (`timeout`, `unreachable`, `invalid answer`, `model changed`), mert a nyers hibában benne van a szerver címe (környezeti érték).
- **A tipp az agenteknek** a szerver által az utasítások végére fűzött rövid szakasz (nem a Szabályok oldal szövege, az Norbi élő szövege).
- **A keresőtáblák nem append-only adatok**, hanem újraépíthető vetületek: a darab frissül és törlődik a forrásával. Ezt a `docs/02` kimondja.
- **Az üzenetszerkesztés** nem létezik; a `task.md` „Tervezendő” része és a `docs/02` rögzíti, hogy ha lesz, újra kell indexelni.
- **A fejlesztői és CI-Postgres** `postgres:17-alpine` helyett `pgvector/pgvector:pg17` (ugyanaz a Postgres 17, pgvectorral); a telepítő saját Postgrese is ez.

## Ami kimaradt (és miért)

- **36b (kódkeresés a GPU-n):** a 39. és 49. kör és a 3060 után, külön ágon.
- **36c (dokumentáció a RAG-ban), a Gemini API, LLM által írt kontextusfejléc, szerveroldali „válasz hivatkozásokkal” eszköz, terheléselosztó:** a prompt szerint nem ennek a körnek a része.
- **A célgépek mérése** (Pi, homelab reranker): Norbi döntése szerint később, a gépek beállításakor.
- **A keresési napló felülete:** a napló az adatbázisban van (30 nap), felület nem készült hozzá.

## Élesítési sorrend

1. Norbi: `CREATE EXTENSION vector` a `colonyhall` adatbázisban (`docs/05`). **Enélkül az új szerver nem indul el** (a migráció egyértelmű hibával áll meg).
2. A Pi beállítása (`docs/12`), a homelabról `curl http://PI_IP:8090/healthz`.
3. A homelab reranker (`docs/13`).
4. A szerver: `COLONY_EMBED_URL`, `COLONY_RERANK_URL` a `deploy/.env`-ben, utána a szokásos frissítés. Induláskor az indexelő végigolvassa az előzményt.
5. `docker compose -f deploy/compose.yaml exec server colonyhall rag-backfill` (a régi döntések).
6. Az őröket ez a kör nem érinti.

A Pi és a reranker nélkül is élesíthető: akkor a keresés kulcsszavas, és ezt kiírja.

## A végső `make ci`

2026-11-10, a munkafa pillanatképén, eldobható Postgresszel (`pgvector/pgvector:pg17`):

| Job | Eredmény | mp |
| --- | --- | --- |
| go-test | ok | 48 |
| go-e2e | ok | 53 |
| go-race | ok | 209 |
| go-pg | ok | 14 |
| go-lint | ok | 15 |
| web-typecheck | ok | 33 |
| web-lint | ok | 36 |
| web-test | ok | 34 |
| devpage-test | ok | 1 |
| shellcheck | ok | 2 |
| actionlint | ok | 10 |
| vuln | ok | 14 |
| install-test | ok | 393 |
| gen-check | ok | 13 |
| build | ok | 28 |
| docker-image | ok | 81 |

Összesen 393 mp. A PWA-t böngészőben (1440/1024/390 px, két téma) nem néztem meg, csak a tesztek futottak.

## Mit nézzen meg először a bíráló

1. **A jogosultság a lekérdezésben:** `internal/modules/search/service.go` (`access`), `postgres/queries.sql` (`KeywordSearch`, `SemanticSearch`: a `visibility`/`channel_id = ANY` feltétel), és a tesztek: `internal/storetest/search.go`, `internal/e2e/search_test.go` (`TestSearchAccessControl`: mindhárom módban, más admin DM-je).
2. **A szerkesztés és törlés követése:** `indexer.go` (visszavonás, wiki szerkesztés és törlés, a `dontesek` oldal kihagyása, az összefoglaló cseréje), `TestSearchFollowsTheSources`.
3. **A kiesés:** `service.go` (`backend.skip`, időkorlátok, `notice`), `TestSearchDegradesFast`; a munkás visszalépése (`RunEmbedder`).
4. **A találatok keretezése:** `mcptools/tools.go` (`Format`), `TestSearchMCPTool` (egy találat nem zárhatja be a keretét).
5. **A migráció** (`00048_search.sql`): a `vector` kiterjesztés kezelése superuser nélkül.
6. **A titokszűrő** (`internal/platform/secretscan`): ami titkosnak látszik, be sem kerül.

## Tananyag

`docs/tanulas/kereses-es-rag.md`: tokenizálás és `tsvector`, `ts_rank_cd` és BM25, azonosítók és trigramok, beágyazás és koszinusz, két beágyazási tér, `query:`/`passage:`, darabolás, HNSW, RRF, cross-encoder rerank, hozzáférés a keresésben, prompt injection, recall@k/MRR/nDCG, és mit érdemes kipróbálni.
