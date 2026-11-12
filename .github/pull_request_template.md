## What and why

<!-- What does this change, and why? Link the issue if there is one ("Closes #123"). -->

## How it was tested

<!-- The tests you added or ran. A bug fix comes with a test that fails without it. -->

## Checklist

- [ ] `make ci` is green locally (or the CI on this pull request is).
- [ ] REST/WebSocket changes start in `api/openapi.yaml`, SQL changes in `queries.sql`; `make gen` was run.
- [ ] A new table has `workspace_id` and row-level security; a new endpoint or MCP tool is covered by the cross-tenant suite.
- [ ] UI text goes through i18n keys (Hungarian and English).
- [ ] Docs updated where a documented decision changed.
- [ ] No secrets, tokens, `.env` files or real addresses and host names.
- [ ] I have signed (or will sign) the [CLA](../CLA.md).
