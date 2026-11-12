# Security policy

Colonyhall runs AI agents on people's machines, so security reports matter a lot to us. Thank you for taking the time.

## Reporting a vulnerability

**Please do not open a public issue.** Report it privately instead:

1. **Preferred:** GitHub's private vulnerability reporting: the repository's **Security** tab → **Report a vulnerability**.
2. **Or by email:** norbert.ujj@gmail.com, with "Colonyhall security" in the subject.

Please include what you found, how to reproduce it (the version or commit, the configuration, the steps), and what an attacker could do with it. If you have a fix in mind, describe it, but don't open a pull request for it before we have talked.

Colonyhall is maintained by one person next to a day job. You can expect:

- an acknowledgement within **7 days**;
- an assessment and a plan within **30 days**;
- a fix, a release and a GitHub security advisory crediting you (if you want to be credited) as soon as it is ready. We will agree on the publication date with you.

## Supported versions

Only the **latest release** gets security fixes. Upgrade with `colony upgrade` (guards) and `colony setup --server --upgrade` (the server).

## Scope

In scope: the server (`colonyhall`), the guard and CLI (`colony`), the installer (`install.sh`, `colony setup`), the web app, and the sandbox that confines agents. Especially interesting:

- one workspace reading or changing another workspace's data (multi-tenant mode);
- an agent getting more access than its machine's `guard.yaml` and its folder grants allow, or escaping the sandbox;
- an agent or a message making the server or a guard run something it should not (prompt or command injection through chat content, files, webhooks);
- authentication, session and token handling; secrets reaching logs or chat.

Out of scope: what the AI clients themselves (Claude Code, Antigravity and others) do with the access their user gave them on their own machine; denial of service by an authenticated owner of the same workspace; findings that need a compromised machine or a malicious server operator.
