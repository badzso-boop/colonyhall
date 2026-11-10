# A szöveges reranker a homelabon (36. kör)

Állapot: **terv** (a `11-rag-terv.md` része). Norbi futtatja, vagy Claude Code az ő jóváhagyásával. A homelabhoz nyúlás előtt: `~/homelab/CLAUDE.md` és `README.md`.

Mit ad: a keresés első 30 találatát egy kis többnyelvű cross-encoder újrarendezi (a kérdést és a darabot együtt olvassa, ezért pontosabb, mint a vektoros hasonlóság). Ha nem fut, a keresés rerank nélkül válaszol, és ezt kiírja.

> **⏳ Mérés hátravan:** a modell végleges választása (lásd `11-rag-terv.md`, 3.) és a `COLONY_RERANK_TOP` az első futtatáskor mért idő alapján. A homelab processzora (i5-2500S) csak AVX-et tud.

## 1. Külön compose-projekt

**Nem** a közös `~/homelab/docker-compose.yml`-be, hanem saját mappába: `~/homelab/colonyhall-rerank/`.

```sh
mkdir -p ~/homelab/colonyhall-rerank/model
cd ~/homelab/colonyhall-rerank
cp ~/homelab/colonyhall/deploy/rerank/compose.yaml .
```

A `compose.yaml` (a repóban: `deploy/rerank/compose.yaml`) lényege:

```yaml
services:
  rerank:
    build: ../colonyhall/deploy/embed     # ugyanaz a kis ONNX-szerver, mint a Pi-n
    command: ["--mode", "rerank", "--model", "/model", "--host", "0.0.0.0", "--port", "8091", "--threads", "2"]
    volumes:
      - ./model:/model:ro
    networks: [shared-db-net]             # csak a Colonyhall szerver éri el
    mem_limit: 1536m
    cpus: 2
    read_only: true
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "python", "-c", "import urllib.request;urllib.request.urlopen('http://127.0.0.1:8091/healthz')"]
      interval: 30s
networks:
  shared-db-net:
    external: true
```

- Nincs kiadott port a gépen (`ports:` nincs): a Colonyhall szerver a `shared-db-net` hálózaton, névvel éri el (`http://rerank:8091`).
- `cpus: 2`: a homelab többi szolgáltatása ne lassuljon keresés közben.

## 2. A modell

A választott modell ONNX int8 fájlja és tokenizere a `deploy/embed/MODEL.md`-ben rögzített Hugging Face-commitról, sha256-tal ellenőrizve, a `model/` mappába. A modellnév a válaszokban `<modell>-int8@<sha256 első 12 jele>`.

## 3. Indítás és ellenőrzés

```sh
docker compose up -d --build
docker compose ps        # healthy
docker run --rm --network shared-db-net curlimages/curl -s http://rerank:8091/v1/rerank \
  -H 'Content-Type: application/json' \
  -d '{"query":"hol fut a szerver?","documents":["A szerver a homelabon fut.","Ma esik az eső."]}'
```

Elvárt: két pontszám, az első dokumentumé a nagyobb.

A Colonyhall `deploy/.env`-jében: `COLONY_RERANK_URL=http://rerank:8091`, és ha kell, `COLONY_RERANK_TOP=30`. Utána a szerver újraindítása (`docker compose -f deploy/compose.yaml up -d`).

## 4. Ha baj van

- **Lassú a keresés:** a Colonyhall állapotlapján a rerank ideje; ha tartósan 2 mp fölött, kisebb `COLONY_RERANK_TOP` (pl. 20) vagy kisebb modell.
- **Memória:** `docker stats colonyhall-rerank-rerank-1`; a korlát 1,5 GB.
- **Leállítás:** `docker compose down` – a keresés rerank nélkül megy tovább.
