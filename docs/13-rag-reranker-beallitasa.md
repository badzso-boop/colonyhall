# A szöveges reranker a homelabon (36. kör)

Állapot: **fut a homelabon 2026-11-13 óta** (a `11-rag-terv.md` része). Norbi futtatja, vagy Claude Code az ő jóváhagyásával. A homelabhoz nyúlás előtt: `~/homelab/CLAUDE.md` és `README.md`.

Mit ad: a keresés első 30 találatát egy kis többnyelvű cross-encoder újrarendezi (a kérdést és a darabot együtt olvassa, ezért pontosabb, mint a vektoros hasonlóság). Ha nem fut, a keresés rerank nélkül válaszol, és ezt kiírja.

> **Mérés (2026-11-13, homelab, i5-2500S, csak AVX, 2 szál):**
> - `jina-reranker-v2` int8: 20 jelölt **17 mp** (az int8 gyors útja AVX2-t vagy VNNI-t kér), és a licence nem kereskedelmi (CC BY-NC 4.0) → **nem ezt használjuk**.
> - `mmarco-mMiniLMv2-L12-H384-v1` (fp32, Apache-2.0): 20 valódi darab `--batch 10`-zel 7,5 mp, **`--batch 1`-gyel 2,9 mp** (a csomagban minden darab a leghosszabbig töltődik, a valódi darabok hossza pedig nagyon szór); 4 szálon kb. 25%-kal gyorsabb, de elvenné a gépet a többi szolgáltatás elől.
> - A keresés rerankkel így 2,8–4,9 mp. Beállítás: `COLONY_RERANK_TOP=20`, `COLONY_RERANK_TIMEOUT=8s`.

## 1. Külön compose-projekt

**Nem** a közös `~/homelab/docker-compose.yml`-be, hanem saját mappába: `~/homelab/colonyhall-rerank/`.

```sh
mkdir -p ~/homelab/colonyhall-rerank/model
cd ~/homelab/colonyhall-rerank
cp ~/homelab/colonyhall/deploy/rerank/compose.yaml .
```

A `compose.yaml` a repóban: `deploy/rerank/compose.yaml` (a kép a `deploy/embed/Dockerfile`-ból épül: ugyanaz a kis ONNX-szerver, mint a Pi-n, `--mode rerank`-kel). A lényeg: csak a `shared-db-net` hálózaton, `rerank` néven érhető el; `mem_limit: 1536m`, `cpus: 2`, csak olvasható fájlrendszer, egészség-ellenőrzés a `/healthz`-en. A modell nevét a `.env` adja: `RERANK_MODEL_NAME=jina-reranker-v2-int8`.

- Nincs kiadott port a gépen (`ports:` nincs): a Colonyhall szerver a `shared-db-net` hálózaton, névvel éri el (`http://rerank:8091`).
- `cpus: 2`: a homelab többi szolgáltatása ne lassuljon keresés közben.

## 2. A modell

A modell (`mmarco-mMiniLMv2-L12-H384-v1`) ONNX fájlja és tokenizere a `deploy/embed/MODEL.md`-ben rögzített Hugging Face-commitról, sha256-tal ellenőrizve, a `model/` mappába (a letöltés receptje ott van). A `.env`-ben `RERANK_MODEL_NAME=mmarco-mminilmv2-l12`; a válaszokban a név `mmarco-mminilmv2-l12@3e9a03ed1e96`.

## 3. Indítás és ellenőrzés

```sh
docker compose up -d --build
docker compose ps        # healthy
docker run --rm --network shared-db-net curlimages/curl -s http://rerank:8091/v1/rerank \
  -H 'Content-Type: application/json' \
  -d '{"query":"hol fut a szerver?","documents":["A szerver a homelabon fut.","Ma esik az eső."]}'
```

Elvárt: két pontszám, az első dokumentumé a nagyobb.

A Colonyhall `deploy/.env`-jében: `COLONY_RERANK_URL=http://rerank:8091`, `COLONY_RERANK_TOP=20` (alap 10) és `COLONY_RERANK_TIMEOUT=8s` (alap 5 mp, ami a homelabon 20 jelöltnél túl szűk). Utána a szerver újraindítása (`docker compose -f deploy/compose.yaml up -d`).

## 4. Ha baj van

- **Lassú a keresés:** a Colonyhall állapotlapján a rerank ideje; ha a rerank rendszeresen kiesik („not reranked”), kisebb `COLONY_RERANK_TOP`, nagyobb `COLONY_RERANK_TIMEOUT`, kisebb modell, vagy később a GPU.
- **Memória:** `docker stats colonyhall-rerank-rerank-1`; a korlát 1,5 GB, a mért használat kb. 825 MB.
- **`--batch`:** maradjon 1 (lásd fent a mérést); nagyobb csomag csak egyforma hosszú szövegeknél gyorsít.
- **Leállítás:** `docker compose down` – a keresés rerank nélkül megy tovább.
