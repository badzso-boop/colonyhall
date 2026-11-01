# 33. kör, 6. rész: gépszintű mappalistázás — szállítási döntés

Ág: `feature/host-fs-relay` (a `67c5fdb`-ből). A mappaválasztónak (7. rész) az agent létrehozása **előtt** kell mappákat listáznia, akár olyan gépen is, amelyen még nincs agent. A meglévő fájlböngésző-relé (`POST /agents/{agentId}/fs`) agentenkénti: a keretet az agent tokenjével nyitott `/ws/ui` kapcsolat viszi, és az agent engedélyeire szűkít. Ez itt nem használható.

## Mit adtam hozzá

- **REST (szerződés: `api/openapi.yaml`):**
  - `POST /api/v1/hosts/{hostId}/fs` `{op: "list_dirs", path?}` → `HostDirListing`. Csak emberi admin (tag, agent: 403; géptoken: 401). Üres `path` = a `guard.yaml` gyökerei. Auditálva: `host.fs_requested` esemény `{host_id, op}` tartalommal, **útvonal nélkül**. 15 mp várakozás → 504; ha a gépnek nincs élő gépszintű WebSocketje (vagy vissza van vonva): **409, `detail: "host_offline"`** (az 503-at a `httpx` részletek nélkül adja vissza, ezért 409, hogy a kliens a `detail` alapján felismerje).
  - `GET /api/v1/host/fs/{requestId}` és `POST /api/v1/host/fs/{requestId}/result` (204) a géptokennel (`requireHost`). Más gép kérése 404, időtúllépés vagy már megválaszolt kérés után 410; a válasz legfeljebb 1 MiB és 500 elem (400 felett).
- **WebSocket: új `/ws/host` végpont** (`internal/gateway/host.go`, `HostGateway`):
  - őr → `{"type":"hello","token":"colh_…"}`; szerver → `{"type":"welcome","protocol":1,"host_id":"…"}`; szerver → `{"type":"host.fs.request","request_id":"…"}`.
  - A hitelesítés a `hosts.Service.AuthenticateHost`-tal megy (a gateway egy kis interfészen át hívja, adaptert nem importál). Agent- vagy felhasználói token itt nem érvényes.
  - **Csak ezt az egy kerettípust** viszi, és csak a saját gépnek: eseményt, chattartalmat, agentkeretet nem. Az őr a hello után nem küld semmit.
- **Memóriabeli relé:** `hosts.FsRelay` (`internal/modules/hosts/fsrelay.go`), az agentenkénti `files.FsRelay` mintájára, gép szerinti tulajdonjog-ellenőrzéssel. Időkorlát tesztben állítható (`App.HostFs.Timeout`).
- **Őr:** a `Supervisor` indításkor elindítja a `HostWS`-t (`internal/guard/hostfs.go`), ami a géptokennel nyitva tartja a `/ws/host` kapcsolatot (újracsatlakozás 1→30 mp visszalépéssel). `host.fs.request` keretre lekéri a kérést, a `HostFsScope.ListDirs`-szal válaszol, és visszaküldi az eredményt.
- **Listázás (őr):** csak a feloldott gyökereken belül; csak mappák; a gyökerekből kifelé mutató symlinket nem követi és nem listázza; legfeljebb 500 elem, név szerint rendezve, `truncated`; elemenként `name`, `path` (abszolút), `is_git_repo`, `has_children`, `hidden`, `heavy` (`.git`, `node_modules`, `vendor`, `dist`, …). A titkos mappák (`.ssh`, `.gnupg`, `.aws`, `.config/colonyhall`, a meglévő `deniedHome` lista bárhol a gyökerek alatt, a titoknév-minták, a tokenfájlok és az exchange mappa) `denied: true` jelölést kapnak, részletek nélkül, és beléjük listázni nem lehet. Üres útvonalnál a gyökerek az elemek, a `name` ilyenkor a teljes abszolút útvonal. Hibakód: `denied`, `not_found`, `not_a_directory` (200-as válaszban, mint az agentenkénti relénél).

## Miért külön `/ws/host`, és nem hello-változat a `/ws/ui`-n

- A `/ws/ui` egy *szereplő* (ember vagy agent) eseményfolyama: sorszám, visszajátszás, láthatósági szűrés. Egy gépnek nincs szereplője, és nem is láthat eseményeket (egy géptoken soha ne kapjon chattartalmat). Egy géptokenes hello a `/ws/ui`-n minden ágban külön kezelést kívánt volna (`connection.actor` nélkül), és könnyű lett volna hibával eseményt szivárogtatni.
- A külön végpont kicsi, egyetlen kerettípust ismer, saját nyilvántartással (`hostID → kapcsolatok`), és ebből közvetlenül adódik a `host_offline`: ha nincs kapcsolat, a push `false`, azonnal 409 (az agentenkénti relé ilyenkor 15 mp-ig vár).
- Lekérdezés (polling) nincs: a keret azonnal megy, az őr egy GET + egy POST után válaszol, így a válasz jellemzően jóval egy másodpercen belül megjön.
- A rétegszabályok maradnak: a gateway csak a `hosts` gyökércsomag típusát látja egy interfészen át; a REST-útvonalak a `hosts/httpapi`-ban vannak (`RegisterFs`), az őr csak a nyilvános API-t használja.

## Nyitva maradt

- A szerveroldali ellenőrzés, hogy a `POST /agents` csak a gép jelentett gyökerein belüli mappát fogadjon el, a 2. rész (agent-létrehozás) dolga; ebben az ágban nincs.
- Egy visszavont gép nyitott `/ws/host` kapcsolatát a szerver nem bontja azonnal; a kérés viszont `host_offline`, és a `GET/POST /host/fs` a visszavont tokent 401-gyel elutasítja.
- Telepítési sorrend: előbb a szerver, utána az őrök (a régi őr nem nyit `/ws/host`-ot, ilyenkor a gép `host_offline`).
