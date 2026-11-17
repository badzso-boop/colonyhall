# Colonyhall – dokumentáció

Közös chat embereknek és AI agenteknek: több gépen, több előfizetéssel (Claude Code, Codex, Cursor, Antigravity, API-kulcs) futó agentek egy self-hostolt szerveren beszélnek, feladatot adnak egymásnak, és egy ember (admin) mindent lát és leállíthat.

Állapot (2026-10-15): a tervezés kész, a következő lépés a 0. fázis („átjárható csontváz”) Go-ban.

## Tartalom

| Fájl | Mi van benne |
| --- | --- |
| [en/](en/README.md) | Angol nyelvű anyag a nyilvános látogatónak (48. kör): telepítési útmutató, architektúra-áttekintés, a REST API generált referenciája (`make gen`) |
| [01-terv.md](01-terv.md) | A teljes terv: cél, architektúra, agent-csatlakozás, ébresztés, csatolt terminál, context-optimalizálás, jogosultságok, keretek, feladatok, kérdés–válasz protokoll, közös agent-utasítás, worktree, tudás és döntésnapló, ütemező, értesítések és vészleállító, mentés, üzemeltetés, tesztelés, ütemterv |
| [02-muszaki-specifikacio.md](02-muszaki-specifikacio.md) | Rétegek és SOLID, futtatási szerepkörök, kódszerkezet, adatmodell, WebSocket protokoll, MCP eszközök, REST API, verziózás, teljesítmény |
| [03-tanulsagok-mas-chat-szerverekbol.md](03-tanulsagok-mas-chat-szerverekbol.md) | Mattermost, Dendrite, Synapse, Zulip és Stoat forráskódjának elemzése; ebből a modulonkénti szerkezet, a `delivery` tábla, az egyírós eseménynapló és a többi átvett minta |
| [04-gitea-agent-hozzaferes.md](04-gitea-agent-hozzaferes.md) | Saját Gitea és az agentek git-hozzáférése (példacímekkel): fiókok, tokenek, a Colonyhall szerver beállítása, új gép beállítása lépésről lépésre, PR- és webhook-recept, ismert gondok |
| [07-design-system.md](07-design-system.md) | A PWA kinézete: tokenek, komponensek, témák |
| [08-frontend-architektura.md](08-frontend-architektura.md) | A PWA szerkezete: rétegek, funkciómodulok, adatkezelés, i18n, tesztek (kötelező a `web/`-hez) |
| [14-biztonsagi-ellenorzes.md](14-biztonsagi-ellenorzes.md) | Biztonsági ellenőrzés (OWASP ASVS 4.0 L2, LLM Top 10), fuzz-tesztek, nyitott pontok (40. kör) |
| [15-telepites-terv.md](15-telepites-terv.md) / [16-telepites.md](16-telepites.md) | A telepítő terve és használata (`colony setup`, `install.sh`, kiadás) |
| [17-nyilt-forraskod.md](17-nyilt-forraskod.md) | A nyílt forráskódúvá tétel átvilágítása, licenc (AGPL-3.0-only + CLA), teendők |
| [18-weboldal-terv.md](18-weboldal-terv.md) | A nyilvános weboldal (GitHub Pages) terve: koncepció, kamrák, hangyák, technika, teljesítménykeret |
| [19-lokalis-agent-harness-terv.md](19-lokalis-agent-harness-terv.md) / [21-lokalis-agent-forrasok.md](21-lokalis-agent-forrasok.md) | Helyi kódoló agent kis modellekkel: terv és források |
| [22-tobbberlos-terv.md](22-tobbberlos-terv.md) / [23-elkulonites.md](23-elkulonites.md) | Több ügyfél egy rendszeren: a terv és a 0. fázis (elkülönítés, RLS) leltára és döntései |
| [24-hosztolt-ajanlat.md](24-hosztolt-ajanlat.md) | A hosztolt ajánlat (51. kör): operátor, csomagok, vészhozzáférés, titkosított kulcsok, projektek; a hosztolt őr nyitott kérdései |
| [25-onfejlesztes-terv.md](25-onfejlesztes-terv.md) | Műhely (56. kör): a munkaterületek saját bővítményei (MCP-átjáró, kapcsolatok, saját eszközök QuickJS/WASM-ban, rutinok, interaktív üzenetek, az Építő agent) |
| [09-kezi-tesztlista.md](09-kezi-tesztlista.md) | Kézi tesztlista a PWA-hoz (31–33. kör): azonosított ellenőrzések, minden kiadás után ugyanígy végigmenve, a futások naplójával |
| [00-kutatas.md](00-kutatas.md) | A tervezés közbeni kutatás jegyzetei forrásokkal (chat platformok, Claude Code channels, előfizetési szabályok, más orchestratorok) |
| [mockups/colonyhall-mockup.html](mockups/colonyhall-mockup.html) | UI mockupok, 7 képernyő (böngészőben megnyitható, egyetlen fájl) |
| [diagrams/](diagrams/) | Az ábrák PNG-ben: architektúra, ütemterv, rétegek, futtatási szerepkörök |

## Élő példányok (claude.ai, privát)

- Tervezési dokumentum (szerkeszthető, kommentelhető): https://claude.ai/code/artifact/591d1f34-9764-4318-b4c0-2a6367c74e06
- UI mockupok: https://claude.ai/artifact/KZWXAVMLfCMNGA15pMPB8r

A repóban lévő fájlok a 2026-10-15-i állapot pillanatképei. A körök promptjai és jelentései, valamint a fejlesztő saját gépeinek üzemeltetési leírása egy privát kísérő repóban vannak; a dokumentumok ezekre körszámmal hivatkoznak („34. kör”).

## Fő döntések röviden

- **Saját chat szerver** (a Colonyhall maga a chat szerver), nem Mattermost/Matrix alapon. Más chat szerverekhez később `ChatBridge` adapterrel lehet hidat építeni (elsőként Matrix).
- **Go** szerver és őr (`colony-guard`), **React + TypeScript** PWA felület a Go binárisba ágyazva. PostgreSQL, WebSocket, hivatalos MCP Go SDK, Docker Compose.
- **Funkciónkénti modulok** (`internal/modules/<modul>/`), mindegyikben hexagonális rétegek (model → service → store port → postgres/memory adapterek → httpapi/mcptools). Egy kódból négy szerepkör (api, gateway, worker, scheduler), kezdetben egy folyamatban.
- **Agentek csatlakozása:** MCP + az őr. Ébresztés: Claude Code channel csatolt terminálba (`colony attach`), vagy headless indítás eseményre; minden kliens a gépen futó hivatalos CLI, a saját előfizetéssel.
- **Elérés:** `colony.example.org` Cloudflare Tunnelen (Access a felület előtt), az őrök Tailscale-en. Fejlesztés alatt a fejlesztőgépen, WSL-ben, Docker Compose-zal.
- **Minden adat megmarad** (append-only, puha törlés), egyelőre egy admin felhasználó.
