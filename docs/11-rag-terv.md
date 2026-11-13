# Keresés és RAG: terv (36. kör, tervezési kapu)

Állapot: **a 36a megvalósítva a `feature/rag` ágon; a mérés a célgépeken hátravan** (Norbi, 2026-11-10: „most nem kell mérni, csak a kód”). A Pi- és a homelab-számok a gépek beállításakor kerülnek ide; a kód a modelltől független (HTTP-port, a modell neve a válaszból jön). A megvalósítás jelentése: `handoff/36-search-rag-report.md`. A kötelező döntések a `docs/handoff/36-search-rag-prompt.md`-ben; ez a dokumentum a megvalósítás terve, a mérések, és a végén a **Norbi döntésére váró kérdések**. Ahol egy szám nem a célgépen mért, ott a szöveg kimondja: „becslés”.

> **Változás (Norbi, 2026-11-13): a szöveg-beágyazó a homelabon fut, nem a Pi-n.** Ugyanaz a szerver és modell (`deploy/embed/`, `multilingual-e5-small` int8), konténerben, külön compose-projektben (`deploy/embed/compose.yaml`, `shared-db-net`, `http://embed:8090`); 430–560 MB memória, a 766 darab ~3 perc alatt beágyazva. A reranker is fut, de a `jina-reranker-v2` helyett az `mmarco-mMiniLMv2` (`--batch 1`): a mérés és az ok a `13-rag-reranker-beallitasa.md`-ben. A lenti Pi-számok és a `12-rag-pi-beallitasa.md` a Pi-s változatra érvényesek, ami alternatíva marad.

## Mai állapot (amire építünk)

**Gépek (2026-11-10, csak olvasva ellenőrizve):**

| Gép | Ami számít | Következmény |
| --- | --- | --- |
| **Raspberry Pi 3B** | még nincs beállítva (sehol nem szerepel a doksikban); 4× Cortex-A53 1,2 GHz, 1 GB RAM | minden Pi-szám **becslés** a PC-n mért egyszálas számokból |
| **Homelab** (`homelab`) | Intel i5-2500S, 4 mag, **csak AVX (nincs AVX2/FMA)**, 7,4 GB RAM, ebből ~3,8 GB szabad; 103 GB szabad lemez | a reranker CPU-n fut, régi utasításkészlettel: a modell méretét ez szabja meg |
| **PC** (`pc-dev`) | Ryzen 9 5950X; a GPU **ma még GTX 1050 (2 GB)**, az RTX 3060 nincs beszerelve; a 49. kör (GPU-futtatókörnyezet, relé, ütemező) még csak prompt | a 36b mérése és megvalósítása a 3060 és a 49. kör után |

**Adatbázis (éles, `colonyhall`, PostgreSQL 16.14, `pgvector/pgvector:pg16`, csak olvasva):**

- `vector` 0.8.5, `pg_trgm` 1.6, `unaccent` 1.1 **elérhető**, egyik sincs telepítve. A `pg_trgm` és az `unaccent` *trusted*: a migráció az adatbázis tulajdonosával (`colonyhall_app`) maga létrehozhatja. A `vector` **nem trusted**: superuser kell (Norbi, egyszer, lásd `05-homelab-telepites.md`).
- A `hungarian` és a `simple` szövegkereső konfiguráció megvan.
- **Adatmennyiség:** 568 üzenet (228 ezer karakter, átlag 401, max 7 753 karakter; 266 üzenet 200 karakter alatt, 11 üzenet 2 000 fölött), 20 csatorna (6 nyilvános, 1 privát, 13 DM), 31 wikioldal (51 revízió, 128 ezer karakter), 13 döntés, 1 tömörítés. Az adatbázis 20 MB. Az elmúlt 7 napban 16 új üzenet.

**Ebből a legfontosabb tanulság:** a teljes szöveges előzmény **kb. 1 500 darab** (chunk). Ennyit a Pi a becsült sebességével is percek alatt beágyaz, ezért a PC-s visszamenőleges beágyazás (backfill) ma fölösleges bonyolítás (lásd 4. és a döntések).

**Kód (amire építünk):**

- Eseménynapló tartós fogyasztóval (`internal/eventlog/consumer.go`, `consumer_offset` tábla): az indexelő egy új fogyasztó, ugyanúgy, mint a `delivery`.
- Jogosultság: `policy.CanReadChannel` (admin mindent lát, más csak a tagként elérhető csatornát); a wiki és a döntések ma **az egész workspace-nek** olvashatók (a kollégának is); a törölt wikioldalt csak ember látja.
- Mappaengedélyek: `folder_grant` (agent, gép, útvonal, `read`/`write`, `revoked_at`); a teljes hozzáférésű agent mappája az `agent_host.root_dir` (sandbox nélkül nincs `folder_grant`).
- Tulajdonos: `actor.owner_id`, `host.owner_id` (38. kör).
- Relé az őrön át: a 20. kör `host.fs.request` kerete (`internal/gateway/host.go`); a 49. kör erre építi a GPU-relét.
- A meglévő „keresések”: a wiki `List` és a döntések `ListDecisions` ma egyszerű szövegegyezéssel szűr. Ezeket a kör nem cseréli le, csak a `search` eszköz mellé kerül.
- **Fejlesztői és CI-Postgres:** ma `postgres:17-alpine` (`deploy/compose.dev.yaml`, `scripts/ci.sh`, `.github/workflows/ci.yml`), abban nincs `pgvector`. A 36a mindhármat `pgvector/pgvector:pg16`-ra állítja, hogy a tesztek ugyanazon a verzión fussanak, mint az éles (PostgreSQL 16).
- Események, amelyekből indexelünk: `message.created`, `message.retracted`; `wiki.created`, `wiki.appended`, `wiki.edited`, `wiki.deleted`, `wiki.restored`; `compaction.done` (a szerkesztés és az újragenerálás is új tömörítésként jön); és az új `decision.recorded`. Üzenetszerkesztés nincs (`task.md`, „Tervezendő”).

## 1. Szöveges beágyazó modell és szerver a Pi 3B-n

> **⏳ Mérés hátravan** (a Pi beállításakor, a valódi gépen): a rövid üzenet és az 512 tokenes darab ideje, az áteresztőképesség, a memória.

**Előzetes, félbehagyott mérés a PC-n** (2026-11-10, CPU, ONNX Runtime, csak tájékoztató; a szkriptek és az adatkészlet nincsenek a repóban):

| Modell | Fájl | Csúcs-memória | Első helyen jó (20 magyar kérdés) | Első 5-ben | MRR@10 |
| --- | --- | --- | --- | --- | --- |
| `multilingual-e5-small` int8 | 118 MB | ~750 MB | 60 % | 90 % | 0,71 |
| `multilingual-e5-small` fp32 | 470 MB | ~1 150 MB | 70 % | 90 % | 0,78 |
| `paraphrase-multilingual-MiniLM-L12-v2` int8 | 118 MB | ~750 MB | 65 % | 80 % | 0,72 |

A jó válasz minden kérdésnél benne volt az első 20-ban (ezért van értelme a rerankernek). A csúcs-memória a Pi 1 GB-jához szoros: a szerver kis kötegekkel dolgozik (a Pi-n 4 bemenet egyszerre, `--batch 4`), a memóriaarénát kikapcsolja, és zram-swap kell.

**A teljes lánc a PC-n, valódi modellekkel** (2026-11-10: a Colonyhall → `deploy/embed/server.py` az `e5-small` int8-cal és a `jina-reranker-v2` int8-cal → Postgres + pgvector; a szintetikus készlet, 18 kérdés): minden konfiguráció megtalálta a választ az első 5-ben; a kulcsszavas keresés ~1 ms, a hibrid ~10 ms, a rerank ~0,6 mp (kb. 20 jelölt, 2 szál). A szintetikus készlet könnyű: a minőséget a valódi készlet méri (merge-blokkoló).

**Javaslat:** `intfloat/multilingual-e5-small` (384 dimenzió, 118 M paraméter, XLM-R szótár, magyarul tanítva), **int8-ra kvantált ONNX**-ban, ONNX Runtime alatt.

**A szerver:** ONNX Runtime + `tokenizers` egy kicsi, a repóban lévő HTTP-szerver mögött (`deploy/embed/`, Python, egy fájl), systemd szolgáltatásként.

- Miért nem `llama.cpp`: a hivatalos ARM64-es kiadás újabb ARM utasításkészletre fordul, és a Pi 3B Cortex-A53 magján valószínűleg el sem indul; a Pi-n forrásból kellene fordítani (`-mcpu=cortex-a53`, kb. 1 óra). Az ONNX Runtime-nak van hivatalos `aarch64` csomagja, ami ARMv8.0-n fut, és **ugyanaz a modellfájl** fut a PC-n is (a backfill és a tesztek ugyanazt a vektort kapják, a lebegőpontos kerekítés kis eltérésével).
- Az Ollama a Pi 1 GB-jához túl nehéz, és ezt a modellt hivatalosan nem is kínálja.

**API-szerződés** (OpenAI-kompatibilis, hogy később cserélhető legyen):

- `POST /v1/embeddings` `{"input": ["query: ...", ...]}` → `{"model": "multilingual-e5-small-int8@4d24e2bc01a4", "data": [{"index": 0, "embedding": [...]}]}` (mean pooling, L2-normált).
- **Az előtagot a Colonyhall teszi fel** (`query: ` a keresésnél, `passage: ` a darabokra), nem a szerver; így a szerver buta, és a teszt a kliensben van.
- Kötegelés: a Colonyhall legfeljebb 8 bemenetet küld egyszerre; a szerver 512 tokenre vágja a bemenetet (a darabolás úgy készül, hogy ne kelljen), és egyszerre egy kérést számol.
- **A modell neve és verziója minden válaszban**: a Colonyhall minden válasznál összeveti a tárolt modellnévvel; eltérésnél nem ír vektort, hibát naplóz, és az állapotlapon jelez (modellcsere = szándékos újrabeágyazás, lásd 5.).
- `GET /healthz` → `{"ok": true, "model": "..."}` (a telepítés ellenőrzéséhez). A Colonyhall nem kérdezgeti: az állapotot az utolsó valódi hívás eredménye adja.
- Időkorlátok a Colonyhall felől: kereséskor (egy rövid kérdés) **1,5 mp**, kötegben **30 mp**. Kereséskor időtúllépés → a keresés kulcsszavas módban válaszol, és ezt kiírja.

**A Pi üzemeltetése** (részletek: `12-rag-pi-beallitasa.md`): 64 bites Raspberry Pi OS Lite; 512 MB zram-swap (SD-kártyára nem swapolunk); a napló memóriában (`Storage=volatile`), hogy az SD-kártya ne kopjon; a modell a kártyán (csak olvasás); automatikus indulás bootkor; tűzfal: csak a homelab érheti el a portot (LAN, `ufw`); hozzáférés token nélkül, mert csak a homelab IP-je engedett (döntés, lásd a végén).

## 2. GPU-szolgáltatások a PC-n (36b)

A 3060 még nincs a gépben, és a 49. kör (futtatókörnyezet, relé, VRAM-ütemező) sincs kész, ezért itt **csak a terv**; a mérés a 36b elején, a 49. kör után.

**Jelöltek (a VRAM becslés, fp16):**

| Szerep | Jelölt | Méret | Dimenzió | VRAM (becslés) | Megjegyzés |
| --- | --- | --- | --- | --- | --- |
| kód beágyazó | `jinaai/jina-embeddings-v2-base-code` | 161 M | 768 | ~0,4 GB | 8k kontextus, 30 programnyelv; `llama.cpp` támogatja |
| kód beágyazó | `nomic-ai/CodeRankEmbed` | 137 M | 768 | ~0,4 GB | kódkeresésre tanítva, `search_query:` előtaggal |
| kód beágyazó | `nomic-embed-code` | 7 B | 3 584 | ~4–5 GB (Q4) | a legjobb pontosság, de a 47. kör kódoló modellje mellett nem fér el: **kiesik** |
| kód beágyazó | `BAAI/bge-m3` | 568 M | 1 024 | ~1,2 GB | többnyelvű, szövegre is jó; tartalék |
| kód reranker | `jinaai/jina-reranker-v2-base-multilingual` | 278 M | – | ~0,6 GB | kódkeresésre is tanítva |
| kód reranker | `BAAI/bge-reranker-v2-m3` | 568 M | – | ~1,2 GB | a `llama.cpp --reranking` biztosan kezeli |

**Elosztás a 12 GB-on:** a 47. kör kódoló modellje (14 B Q4: ~9 GB + KV-gyorsítótár) mellett a két kicsi modell (~1–1,8 GB együtt) **folyamatosan betöltve** marad; a keresés így nem vár betöltésre. Ha nem fér el, a 49. kör ütemezője dönt (interaktív keresés > a helyi agent generálása > tömeges indexelés).

**Futtatókörnyezet:** a beágyazó és a reranker is `llama.cpp` szerverként (`--embedding`, illetve `--reranking`), két systemd szolgáltatás, csak `127.0.0.1`-en; az Ollama csak a kódoló modellé (47. kör). Ok: egy, rögzített verziójú futtatókörnyezet a két kis modellnek, és az Ollamának nincs rerank API-ja.

**Elérés:** **a PC őrének meglévő kapcsolatán át** (49. kör GPU-reléje: `embed` és `rerank` kérés), új port nincs. A szerver a reléhez egy `CodeEmbedder` és `CodeReranker` portot kap.

**Rövid hiba:** a szerver a hívás előtt megnézi, él-e a PC őrének kapcsolata és jelentett-e egészséges GPU-szolgáltatást az utolsó 90 mp-ben; ha nem, **azonnal** (hálózati várakozás nélkül) ezt adja:
`code semantic search unavailable: the GPU machine (pc-dev) is offline; keyword code search and CodeGraph still work`.
Ha a kapcsolat él, de a válasz 3 mp-en belül nem jön meg: `...: the GPU machine (pc-dev) did not answer in 3 s; ...`.

## 3. Szöveges reranker a homelabon

> **⏳ Mérés hátravan** (a homelabon, a reranker beállításakor): 50 jelölt pontozásának ideje és a memória. A homelab processzora (i5-2500S) csak AVX-et tud, ezért a PC-n mért szám nem vihető át.

**Jelöltek:** `cross-encoder/mmarco-mMiniLMv2-L12-H384-v1` (118 M, gyors), `jinaai/jina-reranker-v2-base-multilingual` (278 M), `BAAI/bge-reranker-v2-m3` (568 M, valószínűleg túl lassú ezen a gépen). **Kiinduló választás:** a `jina-reranker-v2-base-multilingual` int8, és ha 50 jelölt 2 mp fölött van, a `mmarco` vagy kevesebb jelölt (`COLONY_RERANK_TOP`, alapból 30). A kód modellfüggetlen: a szerver a Jina/Cohere-formátumú `/v1/rerank`-et hívja.

**Telepítés:** külön compose-projekt `~/homelab/colonyhall-rerank/` (nem a közös `docker-compose.yml`), ugyanaz az ONNX Runtime-os kis szerver, mint a Pi-n (`deploy/embed/Dockerfile`, `deploy/rerank/compose.yaml`; `POST /v1/rerank`, Jina/Cohere-formátum), memóriakorlát 1,5 GB, CPU-korlát 2 mag (a homelab többi szolgáltatása ne lassuljon), egészség-ellenőrzés, csak a `shared-db-net`-en vagy `127.0.0.1`-en elérhető. Részletek: `13-rag-reranker-beallitasa.md`.

## 4. Visszamenőleges beágyazás (backfill)

**Szöveg:** az éles adat kb. 1 500 darab. (A Pi sebessége még nincs mérve; ennyi darab a legóvatosabb becsléssel is egy órán belül kész.) Ezért **nincs külön PC-s backfill** (D3).

**Hogyan:** az indexelő egy eseménynapló-fogyasztó (`search`), és minden forrást a *mostani* állapotából indexel. Egy új fogyasztó a napló elejéről indul, ezért **az első induláskor magától végigolvassa a teljes előzményt**: a darabok `embedding = NULL`-lal jönnek létre, és a szokásos beágyazó munkás tölti fel őket a Pi-vel. Kivétel a döntések: a régiekhez nincs `decision.recorded` esemény, ezeket a `colonyhall rag-backfill` indexeli (egyszer, élesítés után). Így nem kell adatbázis-hozzáférés a szerveren kívül, nincs titok a `colonyhall-exchange`-en, és garantáltan ugyanaz a modell számol.

- `colonyhall rag-backfill`: a döntések, és `-restart`-tal a napló újraolvasása elölről (idempotens: a változatlan darab megtartja a vektorát); `-dry-run` csak számol; a haladást soronként írja; `-embed` a beágyazást is helyben végzi.
- **Ha egyszer nagy lesz az előzmény** (pl. modellcsere 100 ezer darabnál): `colonyhall rag-backfill -embed -embed-url …` a PC-n futó (ugyanazzal a modellfájllal dolgozó) beágyazóra mutathat egy SSH-alagúton át (a PC a homelab Postgresét nem éri el közvetlenül, és nem is kell: a parancs a szerverkonténerben fut, csak a beágyazó címe más). Ehhez nem kell titok a repóban vagy a chatben.

**Kód (36b):** a PC őre indexel (a 39. kör CodeGraph-listája), a GPU-n ágyaz be, és a darabokat a meglévő őr–szerver API-n küldi (új végpont, géptokennel). Nincs adatbázis-alagút. Becslés: egy Colonyhall-méretű repó ~6 600 szimbólum (5 700 Go, 900 TypeScript); a doc-comment nélküliek (~60 %) leírását a helyi modell írja, ~1 mp/szimbólum a 3060-on (becslés) → ~1–1,5 óra egyszer, utána csak a változás.

## 5. Adatmodell

**Két tábla, beágyazási terenként egy** (a dimenzió más, és a jogosultsági oszlopok is mások).

**`search_chunk_text`:**

| Oszlop | Típus | Mire |
| --- | --- | --- |
| `id` | uuid | |
| `workspace_id` | uuid | |
| `source_kind` | text | `message` \| `wiki` \| `decision` \| `summary` |
| `source_id` | uuid | üzenet, wikioldal, döntés vagy tömörítés |
| `source_ref` | text | a wikioldal slugja, a döntés száma (`D-12`); egyébként üres |
| `part` | int | hányadik darab a forrásban |
| `visibility` | text | `channel` (csatornatagság) \| `workspace` (wiki, döntés) |
| `channel_id`, `thread_root_id` | uuid null | |
| `author_id` | uuid | |
| `source_at` | timestamptz | a forrás ideje (szűréshez, rendezéshez) |
| `header` | text | a determinisztikus kontextusfejléc (6.) |
| `body` | text | a darab szövege |
| `content_hash` | text | sha256(fejléc + `\0` + szöveg): változatlan darab újraindexelésnél megtartja a vektorát |
| `tsv_hu`, `tsv_simple` | tsvector, generált | `to_tsvector('hungarian', ...)`, `to_tsvector('simple', ...)` |
| `embedding` | vector(384) null | |
| `embed_model` | text null | pl. `multilingual-e5-small-int8@3f9a…` |
| `embedded_at` | timestamptz null | |
| `embed_attempts`, `embed_retry_at` | int, timestamptz null | újrapróbálkozás: egy sikertelen köteg legkorábban ekkor jön újra |
| `indexed_at` | timestamptz | |

**`search_chunk_code`** (36b): `id`, `workspace_id`, `host_id`, `repo_root`, `path` (abszolút, a gépen), `kind` (`file` \| `function` \| `method` \| `type`), `qualified_name`, `signature`, `description`, `description_source` (`doc` \| `model`), `lang`, `line_start`, `line_end`, `ident_text` (a szétszedett azonosítók), `tsv` (generált, `simple`), `content_hash`, `embedding vector(768)` (a választott modell dimenziója), `embed_model`, `embedded_at`, `indexed_at`.

**Indexek:**

- GIN a `tsv_hu`-n, a `tsv_simple`-n és a kód `tsv`-jén; `gin_trgm_ops` a `qualified_name`-en és a `path`-on (elírás, részleges név);
- **HNSW** (`vector_cosine_ops`, `m = 16`, `ef_construction = 64`, a pgvector alapértékei; ennyi adatnál bőven elég);
- B-tree: `(workspace_id, channel_id, source_at)`, egyedi `(source_kind, source_id, part)` (az esemény alapú frissítéshez), `(workspace_id, host_id, path)` a kódra; részleges index `WHERE embedding IS NULL` (ez maga a beágyazási sor).

**A beágyazási sor** maga a tábla: a munkás `WHERE embedding IS NULL AND (embed_retry_at IS NULL OR embed_retry_at <= now)` szerint vesz ki 8 darabot. Egy munkás van (a szerverben), ezért nem kell `FOR UPDATE SKIP LOCKED`; nincs külön sortábla és nincs River-feladat: a sor állapota az adatban van. Egy sikertelen köteg 5 mp múlva jöhet újra (egy rossz darab nem állítja meg a többit), a munkás maga 5 mp-től 5 percig lép vissza.

**Modellcsere:** a beágyazó a válaszában mást nevez meg, mint a tárolt vektorok modellje → a munkás nem ír, az állapotlap kiírja („run colonyhall rag-reembed”); a `rag-reembed` parancs `embedding = NULL`-ra állítja a régi modellű sorokat, és a munkás újraszámolja őket; közben a kulcsszavas keresés működik, és a szemantikus ág csak az új modellű sorokon keres (a lekérdezés a modellnévre szűr, két modell vektorát soha nem hasonlítja össze). **Más dimenziójú modell** = új migráció (a `vector(384)` oszlop típusa változik).

**Append-only:** a keresőtáblák **nem** elsődleges adatok, hanem újraépíthető vetületek (mint egy gyorsítótár): a forrás törlésekor vagy visszavonásakor a darab sora **törlődik**. Ezt a `docs/02` kimondja, hogy ne ütközzön a 4. szabállyal.

**`search_log`:** `id`, `workspace_id`, `actor_id`, `at`, `query` (szöveg), `mode`, `result_count`, `result_refs` (a találatok forrása), `timings_ms` (jsonb: kulcsszavas, szemantikus, rerank, összesen), `degraded` (pl. `["no_embedder", "no_reranker"]`). **30 nap után törlődik** (óránkénti takarító; a `search_log` nem tartozik az append-only adatok közé). Csak admin látja (a kérdésben lehet érzékeny szöveg).

## 6. Darabolás (chunking)

**Szöveg** (a beágyazó 512 tokenes ablakához):

- **Rövid üzenet** (≤ 350 token): egészben, egy darab.
- **Hosszú üzenet, wikioldal, tömörítési összefoglaló**: bekezdésenként (a markdown címsorok és üres sorok mentén) összevonva, **~300 token** célmérettel és **~50 token** átfedéssel; a kódblokk nem vágódik ketté, ha 350 token alatt van.
- **Döntés**: egy darab (`D-12: <szöveg> — Indoklás: <indoklás>`; ma max 300 + 500 karakter).
- **Determinisztikus kontextusfejléc** minden darab elején (beágyazva és kulcsszóra is indexelve), például:
  - üzenet: `#colonyhall-dev · szál: „Telepítő: első kiadás” · @claude-pc · 2026-11-05`;
  - DM: `DM @norbi ↔ @claude-pc · 2026-11-05`;
  - wiki: `Wiki: Gitea agent-hozzáférés › PR-recept`;
  - döntés: `Döntés D-12 · #colonyhall-dev · 2026-11-03`;
  - összefoglaló: `Összefoglaló · #colonyhall-dev · szál: „…” · 2026-11-06`.
  A szál címe a szálgyökér első sora, legfeljebb 80 karakter. Nem modell írja (az kimarad, lásd a prompt).
- **Szálkontextus a rövid üzenetnél:** csak a fejléc (a szál címe). Nem fűzzük hozzá az előző üzeneteket, mert az a vektort a szál témája felé húzná, és a darab szövege más lenne, mint amit a találat mutat.

**A „Döntések” wikioldal (`dontesek`) nem kerül a wikis darabok közé**: a döntéseket a `decision.recorded` esemény indexeli, saját típusként. Így egy döntés egyszer szerepel, a számával.

**Kód** (36b): szimbólumonként egy darab a CodeGraph határai mentén, és fájlonként egy (a fájl szerepe egy mondatban). Beágyazva: a minősített név, a szignatúra és a leírás; **a törzs soha**.

**Ami soha nem kerül indexbe:**

- a `system` típusú üzenetek (a tömörítés rendszerüzenete csak hivatkozás; az összefoglaló maga külön forrás);
- **titokgyanús szöveg:** egy kis, tesztelt mintakészlet (`internal/platform/secretscan`: API-kulcs-, token-, privát kulcs-, jelszó-URL-minták, a gitleaks szabályai alapján, kód másolása nélkül); ha egy darab találatot ad, **az egész darab kimarad** (kulcsszóra sem kereshető), és a napló csak a forrás azonosítóját írja;
- csatolt fájlok (csak a tömörítés összefoglalója); hibarészletek (`run_error`); naptár-URL-ek (nem forrás);
- kódban: `.env`, `*.pem`, `*.key`, `id_*`, a `colonyhall-exchange` mappa, a `.git`, a `.codegraph*`, a gitignore-olt fájlok, és minden, amit a 18. kör commit-szűrője titoknak tart.

## 7. Hozzáférés-szabályozás

**Szöveg**, a lekérdezés mindkét ágában ugyanaz a feltétel, **a rangsorolás előtt**. A szolgáltatás minden keresésnél kiszámolja az olvasó kereshető csatornáit (a tagságai, a saját DM-jeivel; adminnak ezen felül minden nem-DM csatorna), és a lekérdezés ezt kapja tömbként:

```sql
WHERE c.workspace_id = $ws
  AND (c.visibility = 'workspace' OR c.channel_id = ANY($channel_ids))
  -- a vektoros ágban még: AND c.embed_model = $model
```

Miért így, és nem `EXISTS (… channel_member …)`-ral a lekérdezésben: ugyanaz a szabály (`ChannelIDsOf`, `ListChannels`) dönt, mint a felület többi részén, és a memóriabeli tár is ugyanígy szűr, így a szerződéstesztek mindkét tárra ugyanazt mérik. A tömb kicsi (ma 20 csatorna).

- csatorna: tagság (az agentekre is); admin (az admin szerepű agent is, ahogy a `policy.CanReadChannel` szerint ma is olvashatja) minden nem-DM csatornát lát;
- **DM: csak a tagjainak**, adminnak sem (a prompt szerint; ma a felületen az admin minden DM-et láthat: **döntés D1**);
- wiki, döntés: az egész workspace, ugyanúgy, mint ma (a kollégának is: **döntés D2**);
- törölt wikioldal: kimarad (a keresés nem mutat törölt tartalmat, embernek sem);
- a felfüggesztett (halted) agent kereshet (az olvasás neki is szabad).

**Kód** (36b):

```sql
WHERE c.workspace_id = $ws AND (
      $is_admin
   OR ($is_agent AND (
         EXISTS (SELECT 1 FROM folder_grant g
                 WHERE g.agent_id = $actor AND g.host_id = c.host_id AND g.revoked_at IS NULL
                   AND (c.path = g.path OR starts_with(c.path, rtrim(g.path, '/') || '/')))
      OR EXISTS (SELECT 1 FROM agent_host a
                 WHERE a.agent_id = $actor AND a.host_id = c.host_id AND NOT a.sandbox AND a.root_dir <> ''
                   AND (c.path = a.root_dir OR starts_with(c.path, rtrim(a.root_dir, '/') || '/')))))
   OR (NOT $is_agent AND EXISTS (                 -- a kolléga: a saját agentjei mappái
         SELECT 1 FROM folder_grant g JOIN actor ag ON ag.id = g.agent_id
         WHERE ag.owner_id = $actor AND g.host_id = c.host_id AND g.revoked_at IS NULL
           AND (c.path = g.path OR starts_with(c.path, rtrim(g.path, '/') || '/'))))
)
```

- A mappaengedélyt **a lekérdezés pillanatában** olvassa: egy visszavont engedély azonnal hat, nincs szinkronizálandó másolat.
- A teljes hozzáférésű agent a saját `root_dir`-jét látja a saját gépén (nincs `folder_grant`-ja).
- A kolléga teljes hozzáférésű tervezőjének `root_dir`-je ugyanígy (az `agent_host` ágon, a tulajdonos szerint is kell: a megvalósításban mindkét ág, teszttel).
- **Takarítás:** amit egyetlen aktív engedély sem fed le az adott gépen, azt a szerver a következő gépjelentéskor törli az indexből (és az őr nem is indexeli).
- Az útvonalak összehasonlítása szöveges előtaggal, perjelhatárra igazítva (`/a/b` nem fedi a `/a/bc`-t); a gép az útvonalat feloldva (symlink nélkül) küldi.

**HNSW és szűrés:** a pgvector a `WHERE` feltételt maga alkalmazza, tehát jogosulatlan sor soha nem jön vissza; a szűrt HNSW-keresés gondja a *teljesség* (kevesebb találat), nem a szivárgás. Erre a pgvector 0.8 `hnsw.iterative_scan = relaxed_order` beállítását használjuk (a tranzakcióban `SET LOCAL`). Ennyi adatnál (néhány ezer sor) a tervező gyakran teljes átnézést választ, ami pontos.

**Tesztek (36a, megvan):** a `storetest` készletben mindkét ágra (kulcsszavas és vektoros), memóriában és Postgresen: a nem tag csatornája nem jön vissza, más workspace semmit nem lát; az e2e-ben mindhárom módban: az agent csak a tag csatornáit és a saját DM-jét látja, az admin a privát csatornát igen, egy másik admin DM-jét nem. **36b-ben:** visszavont engedély azonnal kizár, `/a/b` engedély nem ad `/a/bc`-t, a kolléga csak a saját agentjei mappáit.

## 8. Rangsorolás

**Kulcsszavas ág (szöveg):** `websearch_to_tsquery` mindkét konfigurációval (`hungarian` a szótövezett egyezésre: „szerverre” → „szerver”; `simple` a pontos szavakra, azonosítókra), a kettő `||`-vel. **A szavak VAGY-kapcsolatban** vannak (a `&` helyett `|`, a kifejezések `"…"` és a kizárás `-szó` megmarad): egy természetes kérdés („miért bukott el a markdown teszt”) ritkán tartalmazza a válasz minden szavát; a `ts_rank_cd(..., 32)` a több egyezést előre teszi. A találati szavak kiemelése `ts_headline`-nal, `«` és `»` közé (sima szöveg, HTML nélkül).

**Ékezetek (53. kör, 2026-11-13):** a valódi készletben az ékezet nélkül írt kérdések („konyv”, „ugyfel”) 86%-ban kerültek az első 10-be, az ékezetesek 96%-ban, mert a két `tsvector` megtartja az ékezeteket, így ezeknél csak a vektoros ág dolgozott. Ezért van egy harmadik vektor (`tsv_unaccent`, `simple` az ékezet nélküli szövegen) és ugyanígy kezelt kérdés; a pontszámba a másik kettő mellé adódik. Az ékezeteket a `colony_unaccent()` veszi le (`translate()`, beépített és `IMMUTABLE`), **nem** az `unaccent` bővítmény: azt az alkalmazás szerepköre nem hozhatja létre, és a migráció egy meglévő szervert megállítana. A memóriás tár ugyanazokat a betűket hajtja össze (`memory/store.go`). Szótövezés az ékezet nélküli vektoron nincs (a magyar szótövező az ékezetes végződésekre épül), ezért az ékezet nélküli ragozott alakot a vektoros ág fogja.

**Kulcsszavas ág (kód, 36b):** az `ident_text` az azonosítók szétszedve (`parseAgyReset` → `parse agy reset`, `max_concurrent_runs` → `max concurrent runs`), és ugyanez a kérdésre is; mellette `pg_trgm` hasonlóság a `qualified_name`-re és a `path`-ra (`%` operátor, `similarity` > 0,3).

**Vektoros ág:** `ORDER BY embedding <=> $q LIMIT 50`, koszinusz; `hnsw.ef_search = 100`.

**Egyesítés:** Reciprocal Rank Fusion, `k = 60`, mindkét ágból az első 50; **a kulcsszavas ág súlya 0,2** (a szemantikusé 1), a valódi készlet mérése alapján (2026-11-10, lásd „Mérés”).

**Változatos jelöltlista (53. kör, 2026-11-13):** egy szál minden válaszának darabja a fejlécében hordja a szál nyitóüzenetét, így egy hosszú szál sok, egymáshoz hasonló darabbal töltötte meg a rerank jelöltlistáját; a valódi készletben két döntés emiatt esett ki, pedig csak döntésekre szűrve mindhárom módban első volt (rerank 0,999). Ezért az egyesített lista első `max(Limit, RerankTop)` helyén egy forrásból legfeljebb **2**, egy szálból legfeljebb **3** darab lehet (`PoolPerSource`, `PoolPerThread`, `service.go` `diversify`); a többi nem vész el, utánuk jön az eredeti sorrendben, és kitölti a helyeket, ha nincs elég más forrás. Rerank nélkül is így áll össze az oldal.

**Mérendő:** a kulcsszavas ág 0,2-es súlya mellett egy csak kulcsszóval talált darab (0,2/61) a szemantikus ág bármelyik jelöltje mögé kerül; ha a valódi készlet ezt mutatja, a súly újramérése.

**Rerank:** az egyesített (és a fenti korláttal rendezett) lista első **10** eleme (`COLONY_RERANK_TOP`, alap 10; a homelabon 20, lásd `13-rag-reranker-beallitasa.md`) a rerankerhez, legfeljebb `COLONY_RERANK_TIMEOUT` ideig (alap 5 mp; utána a keresés rerank nélkül válaszol), (a darab fejléce + szövege, max ~300 token); a végső sorrend a reranker pontszáma; a reranker nélküli helyek RRF szerint maradnak.

**Végső korlát:** alapból 8, legfeljebb 20. Találatonként: típus, kivonat (a kulcsszavas találat szavai), forráshivatkozás (csatorna + üzenet, wiki slug, `D-NN`, tömörítés), szerző, idő, és embernek, ha admin, minden fokozat pontszáma. Az agent a találatokat `<colonyhall_search_result …>` keretben kapja; a szöveg HTML-escape-elt, így egy találat nem zárhatja be a keretét, és nem nyithat újat.

## 9. Ha valami nem elérhető

| Mi nem elérhető | Mit csinál a keresés | Milyen gyorsan |
| --- | --- | --- |
| a Pi (szöveg-beágyazó) | kulcsszavas módban válaszol, a válasz eleje: `semantic search unavailable: the embedding server is not responding; keyword results only` | a kérdés beágyazása 1,5 mp-es időkorláttal; ha az előző 30 mp-ben már hibás volt, azonnal (nincs újrapróbálás minden keresésnél) |
| a homelab reranker | rerank nélkül, RRF szerint; a válaszban: `reranker unavailable; results are not reranked` | 2 mp időkorlát; utána 30 mp-ig azonnal kihagyja |
| a PC (GPU) | kód szemantikus ága és rerank nélkül; a fenti rövid szöveg | azonnal, a kapcsolat állapotából (2.) |
| beágyazási sor elmaradása | a még be nem ágyazott darabok csak kulcsszóra találhatók | – |

A munkás a Pi kiesésekor exponenciálisan lép vissza (5 mp → 5 perc), nem pörög. Egy beállítatlan szolgáltatás (nincs `COLONY_EMBED_URL` vagy `COLONY_RERANK_URL`) nem kap figyelmeztető sort minden keresésnél, csak a `degraded` mezőben szerepel.

**Állapotlap** (35. kör, `/admin/status`, `system.search`): darabszám, a sor hossza (be nem ágyazott darabok), az utolsó beágyazás ideje, a modellek, a Pi és a reranker: beállítva, elérhető, modell, utolsó sikeres hívás, és a hiba rövid osztálya (`timeout`, `unreachable`, `invalid answer`, `model changed`; a nyers hiba nem, mert benne van a szerver címe), modellcsere esetén figyelmeztetés. 36b-ben a GPU-szolgáltatások.

## 10. Üzemeltetés

| Gép | Memória | Lemez | Mi fut |
| --- | --- | --- | --- |
| Pi 3B | ~750 MB csúcs (PC-n mérve; a Pi-n ⏳) | ~120 MB modell | beágyazó szerver |
| homelab | ⏳ mérés hátravan (korlát 1,5 GB) | ~120–300 MB modell; az indexek az adatbázisban: szöveg ~1 500 sor × (384×4 B + szöveg) ≈ 5 MB indexekkel együtt | reranker, Postgres |
| PC (36b) | ~1–1,8 GB VRAM | ~1 GB modellek | kód beágyazó, kód reranker |

- **Sor:** egyetlen beágyazó munkás a szerveren (kötegenként 8, egyszerre egy köteg a Pi-n; a Pi egyszerre egy kérést szolgál ki).
- **Ha a keresés lassú, üres, vagy azt mondja, a GPU-gép offline:** a Rendszerállapot oldal Keresés kártyája (beágyazó, reranker, várakozó darabok) és a szervernapló `search:` sorai.

## Telepítési sorrend (a megvalósítás után)

1. `CREATE EXTENSION vector` a `colonyhall` adatbázisban superuserként, ha a Colonyhall felhasználója nem superuser (`16-telepites.md`).
2. A Pi beállítása (`12-rag-pi-beallitasa.md`), egészség-ellenőrzés a homelabról.
3. A homelab reranker (`13-rag-reranker-beallitasa.md`).
4. A szerver (migráció; `COLONY_EMBED_URL`, `COLONY_RERANK_URL` a `deploy/.env`-ben). Induláskor az indexelő magától végigolvassa az előzményt.
5. `docker compose -f deploy/compose.yaml exec server colonyhall rag-backfill` (a régi döntések).
6. Az őröket ez a kör nem érinti.
7. 36b: a 49. kör és a GPU-szolgáltatások, majd a PC őre.

## Mérés: a kiértékelő készlet

`colonyhall rag-eval -set FÁJL` kiírja egymás mellett a négy konfiguráció (kulcsszavas, szemantikus, hibrid, hibrid + rerank) recall@5, recall@10, MRR@10, nDCG@10 értékét és a p50/p95 késleltetést, alatta konfigurációnként azokat a kérdéseket, amelyeknél nincs jó forrás az első 10-ben (53. kör). A készlet JSON:

```json
{
  "name": "valódi magyar készlet, 2026-11",
  "questions": [
    { "id": "q01", "kind": "paraphrase", "query": "hol fut az éles szerver?", "relevant": ["message:0192…", "wiki:homelab-telepites"] },
    { "id": "q02", "kind": "decision",   "query": "miért nem konténerben fut az őr?", "relevant": ["decision:D-4"] }
  ]
}
```

- `relevant`: `message:<uuid>`, `wiki:<slug>`, `decision:D-<szám>`, `summary:<tömörítés uuid>`; egy forrás több darabja egynek számít.
- **Valódi készlet** (nincs `seed`): az éles adatbázison fut, egy ember nevében (`-as norbi`); a keresések nem kerülnek a naplóba. A készlet **nem kerül a repóba** (valódi szöveg van benne); Norbi és Claude Code közösen írja (30–50 szöveges kérdés). **Amíg nincs meg és nem futott le, az ág nem olvasztható be.**
- **Szintetikus készlet** (`testdata/rag-eval/synthetic.json`, `seed`-del): kitalált csatornák, üzenetek, wiki és döntések egy eldobható szerveren (memóriában, vagy `-seed-db` egy üres Postgresre); hamis beágyazóval fut a CI-ban (`cmd/colonyhall` teszt), valódi modellekkel `-embed-url`, `-rerank-url`.

## Ütemezés (javaslat)

- **36a ezen az ágon** (`feature/rag`), a 36b **külön ágon** (`feature/rag-code`) a 39. és a 49. kör beolvasztása után: ma egyik sincs a `main`-en, a 3060 nincs bent, és a 36b-hez mérni kell.
- A merge-blokkoló (a valódi magyar kiértékelő készlet) a 36a-ra is érvényes.

### Az első mérés a valódi adaton (2026-11-10)

32 kérdés (döntések, wiki, üzenetek, egy összefoglaló), az éles adat 2026-11-10-i mentésének másolatán, a PC CPU-ján (`multilingual-e5-small` int8, `jina-reranker-v2` int8, 4 szál). A készlet a repón kívül van (`/root/colonyhall-eval/`), Norbi még átnézi.

| konfiguráció | recall@5 | recall@10 | MRR@10 | nDCG@10 | p50 |
| --- | --- | --- | --- | --- | --- |
| kulcsszavas | 0,30 | 0,44 | 0,31 | 0,31 | 40 ms |
| szemantikus | 0,69 | 0,81 | 0,56 | 0,60 | 58 ms |
| hibrid | 0,58 | 0,73 | 0,44 | 0,50 | 98 ms |
| hibrid + rerank, 10 jelölt | 0,73 | 0,73 | 0,70 | 0,69 | 2,8 mp |
| hibrid + rerank, 20 jelölt | 0,80 | 0,83 | 0,79 | 0,78 | 6,8 mp |
| hibrid + rerank, 30 jelölt | 0,81 | 0,84 | 0,78 | 0,78 | 11,0 mp |

- **A reranker sokat javít, de CPU-n lassú:** a régi 30 jelölt és a fix 2 mp mellett **minden rerank kiesett volna** (a valódi darabok hosszabbak a szintetikusnál). **Döntés (Norbi, 2026-11-10):** 10 jelölt, MRR@10 ≈ 0,70 elfogadva (otthoni projekt, nem nagy szerver); az időkorlát állítható (`COLONY_RERANK_TIMEOUT`, alap 5 mp). Később a reranker a PC GPU-jára kerülhet (49. kör), akkor több jelölt is belefér.
- **A homelabon** (i5-2500S, AVX2 nélkül) a 10 jelölt is lassabb lesz: élesítés előtt ott mérni kell (`docs/13`).
- **A hibrid egyenlő súllyal rosszabb volt a csak szemantikusnál:** a kulcsszavas ág VAGY-kapcsolata szinte minden darabra talál egy közös szót, és ez a zaj az RRF-ben lehúzta a jó szemantikus találatokat.

### A hibrid hangolása (2026-11-10, Norbi kérésére)

Ugyanazon a 32 kérdésen, reranker nélkül (a „hibrid” sor), majd a legjobbak rerankerrel (10 jelölt):

| beállítás | recall@5 | recall@10 | MRR@10 | nDCG@10 |
| --- | --- | --- | --- | --- |
| csak szemantikus (összevetésnek) | 0,69 | 0,81 | 0,56 | 0,60 |
| kulcsszavas súly 1 (régi) | 0,58 | 0,73 | 0,44 | 0,50 |
| kulcsszavas súly 0,5 | 0,69 | 0,80 | 0,49 | 0,55 |
| kulcsszavas súly 0,3 | 0,72 | 0,80 | 0,52 | 0,58 |
| **kulcsszavas súly 0,2** | **0,75** | **0,83** | **0,54** | **0,60** |
| kulcsszavas súly 0,1 | 0,73 | 0,81 | 0,57 | 0,61 |
| kulcsszavas jelöltek: 5 / 10 / 20 (súly 1) | 0,56 / 0,53 / 0,55 | 0,72 / 0,75 / 0,73 | 0,42 / 0,44 / 0,44 | – |
| RRF `k` = 10 / 30 (súly 0,3) | 0,73 / 0,75 | 0,81 / 0,81 | 0,53 / 0,55 | – |
| súly 1 + rerank | 0,73 | 0,73 | 0,70 | 0,69 |
| súly 0,3 + rerank | 0,77 | 0,80 | 0,74 | 0,73 |
| **súly 0,2 + rerank** | **0,80** | **0,83** | **0,79** | **0,78** |

- **Döntés:** a kulcsszavas ág súlya **0,2** (`KeywordWeight`). A hibrid így a recallban jobb a csak szemantikusnál (a pontos nevek és számok a kulcsszavas ágból jönnek), és rerankkel 10 jelöltből is annyit ér (MRR 0,79), mint korábban 20 jelöltből, 2,5 mp alatt.
- A kulcsszavas jelöltek számának szűkítése nem segít; a `k` állítása alig változtat.
- **Óvatosan:** 32 kérdésnél egy kérdés ~0,03; a 0,1 és a 0,3 közti különbség ezen belül van. Ha a készlet bővül vagy Norbi átírja, újra kell mérni.

## Norbi döntésére vár

Norbi 2026-11-10-én a megvalósítást elindította; ezeket a megvalósítás a **javasolt alapértékkel** csinálja, és mind egy helyen átállítható:

- **D1 – DM-ek a keresésben:** csak a DM tagjai találják meg, adminnak sem jön más DM-je (a prompt szerint). Ma a felületen az admin minden DM-et láthat; ha a keresésben is kell, egy feltétel a lekérdezésben.
- **D2 – wiki és döntések:** az egész workspace kereshet bennük (a kolléga is), ugyanúgy, mint ma a listákban. Ha a privát csatornában hozott döntést el kell rejteni, a döntés csatornájára szűrünk.
- **D3 – visszamenőleges beágyazás:** a szerveren, a Pi-vel, a szokásos munkással (nem a PC-n). Nagy előzménynél a parancs a PC beágyazójára is mutathat.
- **D4 – a 36b külön ágon**, a 39. és 49. kör beolvasztása és a 3060 beszerelése után.
- **D5 – a Pi és a reranker token nélkül**, a hálózat szűri (a Pi tűzfala csak a homelabot engedi, a reranker csak a homelabon belül érhető el). Ha a Pi-t más is eléri a LAN-on, egy közös token a fejlécben.
