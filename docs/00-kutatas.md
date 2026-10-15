# Kutatási jegyzetek (2026-10-15)

A tervezés közben összegyűjtött tények, forrásokkal. Ami változékony (előfizetési szabályok, kísérleti funkciók), azt fejlesztés előtt újra ellenőrizni kell.

## Nyílt forrású chat platformok és valós idejű motorok

| Név | Milyen | Megjegyzés |
| --- | --- | --- |
| Rocket.Chat | Slack-szerű | Sok funkció; a fürtözés és a naplózás az Enterprise kiadásban van |
| Mattermost | Slack-szerű, fejlesztőknek | Go + React; erős integrációk |
| Zulip | Témákba rendezett beszélgetések | Apache 2.0 |
| Stoat (korábban Revolt) | Discord-szerű | AGPL-3.0 |
| Matrix + Element | Föderált, végpontok közötti titkosítással | Sok kész híd más platformokhoz |
| Centrifugo | Valós idejű motor (Go) | Bármilyen backend mellé |
| Laravel Reverb | PHP WebSocket szerver | Laravelhez |
| Soketi | Pusher-kompatibilis (Node) | 2025 tavasza óta nem frissült |

Döntés: saját szerver, mert az agent-specifikus részek (inbox, digest, feladatok, keretek, vészleállító) a rendszer magja.

Források: [pinggy.io](https://pinggy.io/blog/best_open_source_alternatives_to_slack_and_discord/), [toolradar](https://toolradar.com/guides/best-open-source-team-chat), [pistack](https://www.pistack.xyz/posts/centrifugo-vs-mercure-vs-soketi-self-hosted-websocket-push-server-2026/)

## Claude Code channels

- Egy channel olyan helyi (stdio) MCP szerver, ami a `claude/channel` képességet jelzi, és `notifications/claude/channel` eseményekkel üzenetet tol egy már futó Claude Code sessionbe. Az esemény `<channel source="..." ...>` címkében érkezik; válaszolni egy saját `reply` eszközzel lehet.
- Kutatási előzetes (research preview). Pro/Max fiókon külön engedélyezés nélkül elérhető; Team/Enterprise fióknál az adminnak engedélyeznie kell.
- Saját (nem Anthropic-allowlistes) channel csak a `--dangerously-load-development-channels server:<név>` kapcsolóval töltődik be, és **csak interaktív módban**; `-p` vagy Agent SDK esetén figyelmen kívül marad.
- Ha az agent épp dolgozik, az események sorba állnak, és a következő körben együtt érkeznek.
- Engedélykérés továbbítása: `claude/channel/permission` képesség; a kérés `notifications/claude/channel/permission_request`, a válasz `notifications/claude/channel/permission` (`allow` / `deny`), 5 betűs azonosítóval.

Források: [Channels](https://code.claude.com/docs/en/channels), [Channels reference](https://code.claude.com/docs/en/channels-reference)

## Claude Code keret- és költségadatok

- Státuszsor JSON: `rate_limits.five_hour.used_percentage`, `rate_limits.seven_day.used_percentage`, `resets_at` (csak Pro/Max, az első API-válasz után), `cost.total_cost_usd`, `context_window.used_percentage`. Forrás: [statusline](https://code.claude.com/docs/en/statusline)
- `claude -p --output-format json`: `total_cost_usd` és felhasználás; van `--max-budget-usd` kapcsoló. Forrás: [headless](https://code.claude.com/docs/en/headless)
- OpenTelemetry (`CLAUDE_CODE_ENABLE_TELEMETRY=1`): `claude_code.token.usage`, `claude_code.cost.usage`, `claude_code.api_request` esemény. Forrás: [monitoring](https://code.claude.com/docs/en/monitoring-usage)
- Codex: a session-fájlokban `token_count` események `rate_limits.primary` / `secondary` `used_percent` mezőkkel (közösségi forrás, ellenőrizni kell): [codex-ratelimit](https://github.com/xiangz19/codex-ratelimit)

## Előfizetési szabályok (változékony!)

- 2026 január–február: Anthropic letiltotta, hogy az előfizetés OAuth tokenje harmadik féltől származó eszközökben működjön (OpenClaw, OpenCode stb.), és a feltételekbe is beírta. [Lenz összefoglaló](https://lenz.io/c/anthropic-subscription-openclaw-terms-of-service-violation-63f46a30)
- 2026. június 15.: az Agent SDK / `claude -p` használatát érintő változásokat határozatlan időre elhalasztották; ezek továbbra is az előfizetés keretéből fogynak. 2026. október 7.: a Max és Team csomagok havi API kreditet is tartalmaznak. [Help Center](https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan), [Conductor](https://conductor.build/blog/claude-subscription-update)
- Következtetés a Colonyhallra: a gépen futó hivatalos CLI-k indítása (interaktív, csatolt terminál, vagy `claude -p`) a saját előfizetéssel rendben van; a saját előfizetés nem adható tovább másoknak szolgáltatásként.

## Más orchestratorok

- Conductor, Claude Squad, Vibe Kanban: a hivatalos `claude` CLI-t futtatják (közvetlenül vagy az Agent SDK-n át).
- Hermes Agent (Nous Research): saját agent-ciklus; Claude OAuth csak Max + extra kreditből megy, Pro-val nem; ChatGPT eszközkódos bejelentkezéssel a Codex modelleket éri el. [Hermes providers](https://hermes-agent.nousresearch.com/docs/integrations/providers)

## Kliensek MCP-támogatása

- Codex CLI: MCP kliens stdio és Streamable HTTP (Bearer token) szerverekhez, `~/.codex/config.toml`; nem interaktív mód: `codex exec`. [Codex MCP](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)
- Cursor CLI: `cursor-agent -p --approve-mcps --trust`; a fejnélküli MCP 2026-ban többször hibás volt. [Cursor fórum](https://forum.cursor.com/t/cursor-agent-p-mode-does-not-inject-mcp-server-tools-into-agent-context/155275)

## Név

A „Hivemoot” foglalt volt (https://github.com/hivemoot/hivemoot). A választott név: **Colonyhall**; 2026-10-15-én GitHubon és npm-en nem volt ilyen projekt. Hangyás alternatívák: Pheromail, Nestmoot, Antmoot. A domaint (`colony.ujjweb.hu` saját aldomain) nem kell külön ellenőrizni.
