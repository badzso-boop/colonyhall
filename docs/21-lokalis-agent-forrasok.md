# Források: helyi kódoló agent kis modellekkel (kutatás, 2026-11-10)

> Ez a dokumentum a `docs/19-lokalis-agent-harness-terv.md`, a 47. kör (helyi agent) és a 49. kör (GPU) döntéseinek hátterét gyűjti össze: mit csinálnak mások, és mit vettünk át belőle. A számok a források saját mérései, kis mintákon; mi a 3060-on, a saját feladatainkon mérünk (47. kör, tervezési kapu). A legtöbb forrás másodlagos (blog, összefoglaló); ahol ez számít, jelölve.

## Összefoglalás: mit vettünk át

1. **A keret többet számít, mint a modell.** Kis modellnél a jól megépített keretrendszer (harness) a teljesítmény jó részét adja (little-coder: 45,6% vs 19,1% ugyanazzal a 9B-s modellel).
2. **A „mit és hol kell módosítani” külön lépés.** A sikertelen esetek többsége a hely megtalálásán bukik el, nem az íráson (Agentless, BLAgent). A felderítés és az írás külön kontextusba kerül (Aider architect/editor, little-coder plan → friss implement).
3. **Lint- és fordítás-ellenőrzött szerkesztés.** A hibás szerkesztést el se fogadjuk, a hibát azonnal visszaadjuk (SWE-agent).
4. **Duplikáció ellen rétegek kellenek:** írás előtti keresés, gépi másolatkereső, és szándék szerinti (LLM-es) összevetés.
5. **Nem írunk saját agentet és saját modellfuttatót** (döntés, 2026-11-10): kész, nyílt forrású kódoló agentet futtat a guard új kliensként, a modelleket a `llama.cpp` szolgálja ki a llama-swap mögött; mi csak a determinisztikus extrákat írjuk, MCP szerverként.

## Kutatás és mérések

### Agentless (helykeresés → javítás → ellenőrzés)
- **Mi ez:** kutatási pipeline agent nélkül, rögzített fázisokkal: a hely lépcsőzetes keresése (fájl → osztály/függvény → sor), több javítási jelölt diff formában, ellenőrzés.
- **Eredmény:** SWE-bench Lite 27,3%, majd 32,0%. A szerzők a benchmark hibáit is jelezték (Lite-S).
- **Átvettük:** a helykeresés külön, determinisztikus lépés (`docs/19`, 7. fejezet: felderítés).
- Forrás: [Agentless (arXiv)](https://arxiv.org/html/2407.01489v2)

### BLAgent és RGFL (a helykeresés a szűk keresztmetszet)
- **BLAgent (2026):** a sikertelen esetek kb. 82%-a a lépcsőzetes helykeresés valamelyik fokán bukik el; jobb helykereséssel az Agentless 27,6%-ról 34–38%-ra javul.
- **RGFL:** érveléssel vezetett hibakeresés; fájlszinten a Hit@1 71,4% → 85%, végponttól végpontig +12,8%.
- **Átvettük:** a keresésbe (CodeGraph, kódindex, rerank) érdemes befektetni, nem a modellbe.
- Források: [BLAgent](https://www.emergentmind.com/papers/2605.17965), [RGFL (arXiv)](https://www.arxiv.org/pdf/2601.18044)

### SWE-agent (agent–számítógép felület)
- **Mi ez:** a felület (az eszközök formája) határozza meg a teljesítményt. A legfontosabb elemnek a lint-ellenőrzött szerkesztés bizonyult: szintaktikailag hibás szerkesztés nem megy át, a hiba pontosan visszamegy.
- **Átvettük:** kapuk minden szerkesztés után (`docs/19`, 9. fejezet). Kis modellekkel nem mérték (GPT-4 Turbo).
- Forrás: [SWE-agent (hyper.ai)](https://hyper.ai/papers/2405.15793)

### Aider (architect/editor mód, repótérkép)
- **Mi ez:** két szerep: az egyik modell megtervezi, a másik megírja a szerkesztést. A repót tree-sitterrel térképezi fel (szignatúrák, PageRank-rangsor).
- **Figyelmeztetés:** gyenge modelleknél a repótérképet ki is kapcsolják, mert a sok kontextus összezavarja őket.
- **Átvettük:** felderítő és író külön szerep; kevés, célzott kontextus. (Másodlagos források.)
- Források: [Aider (agentwiki)](https://agentwiki.org/aider), [Aider review 2026](https://toolbrain-astro.pages.dev/blog/aider-review-2026/)

### little-coder (kis modellekre hangolt harness)
- **Mi ez:** a „pi” agent bővítményei kis helyi modellekhez; alapból `llama.cpp` szerverrel és Qwen3.6-35B-A3B-vel; 8 GB VRAM-os laptopon mérve.
- **Eredmény:** Aider Polyglot: Qwen3.5-9B **45,6%** (sima Aiderrel 19,1%); Qwen3.6-35B-A3B 78,7%. Terminal-Bench 2.0: 24,6% / 9,2%.
- **Megoldásai:** külön tervező mód és friss végrehajtó munkamenet (más modellel is, ~15 mp modellcsere); tömörítés 80%-os kontextusnál; ismétlés- és üresválasz-figyelő; a hibás formátumú eszközhívások javítása; nagy fájlból 30 sor, a modell keresésre terelve; egész fájl felülírása tiltva; szerkesztés csak olvasás után.
- **Átvettük:** a 47. kör egyik fő jelöltje kész agentnek (lent).
- Forrás: [little-coder (GitHub)](https://github.com/itayinbarr/little-coder)

### Harness-bench (helyi modellek × agentek)
- **Mi ez:** 16 feladat, öt agent (pi, qwen, aider, opencode, claude), helyi modellek egy M3 Max-on.
- **Eredmény:** Qwen3.6-27B + pi az egyetlen 16/16; a modell és az agent illeszkedése számít (pl. gpt-oss-120b opencode-dal összeomlik, Qwen3.6-27B aiderrel). Q4 tömörítés ≈ Q8; a 3B aktív paraméteres MoE kb. annyit ér, mint a 27B-s sűrű modell.
- **Figyelmeztetés:** az opencode alapból elolvasta a rejtett értékelő szkriptet (csalás a mérésben).
- Forrás: [Benchmarking Local LLMs Against Coding Agent Harnesses](https://neuralnoise.com/2026/harness-bench-wip/)

### Sebastian Raschka: helyi kódoló agentek
- **Tapasztalat:** a tokenfogyasztást főleg az agent határozza meg, nem a modell; a kis modellek tipikus hibái: rossz eszköz, hibás JSON-argumentum, rossz útvonal, fölösleges visszakérdezés. Egy szűkebb, újrapróbálkozó, erősebb projektkontextust adó keret a Qwen3.6-szerű modelleket használhatóvá teheti. Cél: 20–30 token/mp.
- Forrás: [Using Local Coding Agents](https://magazine.sebastianraschka.com/p/using-local-coding-agents)

### HeyNeo: melyik helyi modell kódoláshoz
- **Eredmény:** qwen3.6:27b a legjobb összteljesítményű (eszközhívás helyes paraméterekkel 84,6%); a deepseek-coder:33b kódgenerálásban erős, agentként 10%.
- **Átvettük:** a modellt az eszközhívás megbízhatóságán is mérni kell, nem csak a kódon.
- Forrás: [Which Local LLM Should You Actually Use](https://heyneo.com/blog/local-llm-coding-eval)

### Duplikáció az agentek kódjában (gyakorlat)
- **Tapasztalat:** képességindex, amit írás előtt kötelező megnézni; „ugyanaz a szándék” (nem csak ugyanaz a szöveg) a valódi kockázat; a szintaktikai másolatkereső (jscpd) az átnevezett újraírást nem veszi észre, ahhoz LLM-es csoportosítás kell. Anekdotikus, mérés nélkül.
- Források: [Product Hunt-beszélgetés](https://www.producthunt.com/p/vibecoding/how-do-you-stop-ai-coding-agents-from-silently-duplicating-code), [finding-duplicate-functions](https://www.mdskills.ai/skills/finding-duplicate-functions), [code-deduplication](https://skillselion.com/skills/alinaqi/claude-bootstrap/code-deduplication)

## Kész eszközök (a „ne találjuk fel újra” döntés alapja)

### llama.cpp szerver és llama-swap (futtató)
- **llama-server:** OpenAI-kompatibilis felület, streamelés, generálás, embedding, rerank, nyelvtanhoz vagy JSON-sémához kötött kimenet. Gondolkodó modellnél a `--reasoning-format deepseek` a gondolkodást külön `reasoning_content` mezőben adja (a válasz szövegétől elválasztva).
- **llama-swap:** proxy a `llama-server` elé: a konfigban felsorolt modellek közül a kértet igény szerint betölti, a régit kirakja, egyetlen OpenAI-kompatibilis címet ad. Nginx mögött a streameléshez a pufferelést ki kell kapcsolni.
- **Döntés (2026-11-10):** ez a futtató a WSL-ben, natívan, systemd alatt, csak `127.0.0.1`-en; a guard nem indít maga modellfolyamatokat, csak a szabad VRAM-ot figyeli (49. kör).
- Források: [llama-swap](https://www.sourcepulse.org/projects/1775233), [reasoning_content (provider-llamacpp)](https://workers.iii.dev/workers/provider-llamacpp.md)

### LM Studio (kísérletezéshez)
- **Mi ez:** asztali alkalmazás és fej nélküli démon (`lms daemon up`, systemd), igény szerinti betöltés (JIT), üresjárat után kirakás (TTL, alapból 60 perc), „auto-evict”.
- **Miért nem ez a futtató:** zárt forrású alkalmazás, saját ütemezéssel; a modellek kipróbálására viszont kényelmes.
- Források: [LM Studio headless](https://lmstudio.ai/docs/developer/core/headless), [TTL és auto-evict](https://lmstudio.ai/docs/app/api/ttl-and-auto-evict)

### Kész kódoló agentek (a 47. kör jelöltjei)
- **little-coder / pi:** kis modellekre hangolva (fent).
- **OpenCode:** helyi modell, MCP, nem interaktív futás (szöveg vagy JSON kimenet). Figyelem: nem interaktív módban minden engedélyt automatikusan megad, és a harness-bench szerint a rejtett értékelőt is elolvasta, ezért nálunk csak sandboxban.
- **Qwen Code:** a Qwen saját terminálos kódoló agentje, nem interaktív mód (`qwen -p`), MCP (`qwen mcp list`).
- **Goose (Block):** minden bővítmény egy MCP szerver; teljesen helyi modellekkel is fut.
- **Hermes Agent (Nous Research):** beépített MCP-kliens, helyi OpenAI-kompatibilis szerverekkel; inkább általános személyes agent, nem kódolásra kihegyezett.
- Források: [Goose CLI vs. Qwen Code](https://terminaltrove.com/compare/ai-coding-agents/goose-cli-vs-qwen-code/), [Qwen Code](https://mcp.so/cli/qwen-code), [OpenCode non-interactive](https://pkg.go.dev/github.com/MerrukTechnology/OpenCode-Native@v1.5.0), [OpenCode + llama-swap](https://reddit.sentinel-team.org/posts/1ru6qml/snapshots/2026-03-16T20%3A46%3A16.316967Z), [Hermes Agent MCP](https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp), [Hermes Agent](https://agentic-ai.readthedocs.io/en/latest/AgentPlatforms/hermes-agent/)

### Amit nem használunk, és miért
- **Open WebUI:** embereknek szóló chatfelület, nem agent; nekünk a Colonyhall ez a felület.
- **herdr:** terminál-multiplexer kódoló agenteknek (állapotjelzés, agentek egymás vezérlése); a Colonyhall a tmux-os termináljaival és az agent-agent üzenetekkel ezt már tudja. Ötletnek jó: az agent állapotának (vár, tétlen, dolgozik) felismerése.
- Források: [herdr (Better Stack)](https://betterstack.com/community/guides/ai/herdr-ai-agent/), [herdr (innFactory)](https://innfactory.ai/en/ai-harness/herdr/)
