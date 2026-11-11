# Handoff prompt 48: making Colonyhall open source, and its GitHub Pages site

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/open-source ../new-chat-worktrees/open-source main
cd ../new-chat-worktrees/open-source
claude "$(cat docs/handoff/00-ground-rules.md docs/handoff/48-open-source-prompt.md)"
```

---

Branch: `feature/open-source`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/open-source`. This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never touch `main`, never merge, never push. Also **never**:
- change the repository's visibility;
- rewrite git history (`filter-repo`, force push);
- create releases, tags or GitHub Pages deployments;
- change repository settings, secrets or branch protection on GitHub.

You prepare everything; Norbi does the irreversible steps himself, from a checklist you write.

After each part: `gofmt`, `make test`, `make lint`, `make test-web` when `web/` or `site/` changed. **Commit only when they pass.**

**This round has two design gates** (part 0 and part 5). At each one you stop and wait for Norbi's answer in the chat.

## Why (Norbi, 2026-11-05)

The private repository `badzso-boop/colonyhall` will become public. Colonyhall may later be sold as a subscription too.

Two goals:
1. **A complete, ordered list** of what has to happen before the repository can be public, and the safe parts of it done on this branch.
2. **A GitHub Pages site** for the project that carries its vibe: **not a basic landing page**, something much more complex. Norbi describes the vision later (section 5); until then only the technical groundwork is allowed.

## What is known already (binding)

- The name stays **Colonyhall**; the existing repository becomes public (no new repository unless the audit shows the history cannot be published; then say so and stop).
- The server image on GHCR is public; the installer (round 41, `feature/installer`, `docs/15-telepites-terv.md`, `docs/16-telepites.md`, the new `README.md`) is the way people install it.
- Patterns were taken from AGPL projects (Mattermost, Dendrite, Synapse, Stoat) **but no code was copied** (`CLAUDE.md`, "Licenc"). The audit must confirm this.
- Round 40 (`docs/handoff/40-security-prompt.md`) covers security. Link to its results; do not redo them.
- **The licence is decided (Norbi, 2026-11-11): `AGPL-3.0-only` with a CLA, i.e. dual licensing** (the Element/Synapse model). Do not re-open the choice; part 0.3 only checks that it works.
  - Everyone may use, self-host and modify Colonyhall under the AGPL. Whoever offers a modified version as a network service must publish its source (AGPL section 13), or buy a commercial licence from Norbi.
  - Contributors sign a CLA so that their code can also be offered under the commercial licence. Base it on the Apache Individual CLA (as Element does), and add Element's commitment: every contribution also stays available under an OSI-approved licence (the AGPL).
  - Paid features may later live in a separate `ee/` directory under a commercial licence (the Cal.com model). Do not create it in this round; only make sure nothing in the layout or the licence files rules it out.

## 0. Audit and checklist (design gate)

Write `docs/17-nyilt-forraskod.md` (Hungarian). For every item give: what was found (with file paths and line numbers, **never the secret itself**), what you propose, and what Norbi must decide. Then **stop**.

### 0.1 Secrets and private data, in the working tree **and in the whole git history**

**Pre-scan (2026-11-11, before this round; repeat it, the history has grown):** gitleaks 8.28 and trufflehog 3.90 over all 459 commits of every branch found **no real secret** and no verified one. Every finding was a test fixture (`secretscan_test.go`, `step_test.go`, `git_test.go`, `search_test.go`, `setup_test.go`, `webread_test.go`), a placeholder in `deploy/.env*.example`, or a false positive in the vendored Monaco editor (102 "Github" hits). No `.env`, key or token file was ever committed. Every commit has the same author e-mail (the public contact above). So the history question below is expected to end with "keep the history"; the remaining work is class (b): Norbi's Tailscale and LAN addresses, host and user names and `/mnt/g/` paths in about 25 tracked files (docs, `CLAUDE.md`, `deploy/.env.prod.example`, a few tests).

- Run `gitleaks` and `trufflehog` (in Docker; scan `git log --all`, not just HEAD), and grep for:
  - tokens (`col_`, `colh_`, `sk-ant-`, `ghp_`, `github_pat_`), passwords, `.env` files ever committed, private keys;
  - Norbi's infrastructure: Tailscale and LAN addresses (`100.85.111.88`, `100.122.147.16`, `192.168.1.101`), host names (`szmajli`, `nato`, `pc-wsl`), `gitea.ujjweb.hu`, home paths (`/home/szmajli01`, `/mnt/g/...`), e-mail addresses, real names;
  - the development seed's fixed tokens (they are documented as dev-only; confirm they cannot work on a production server).
- Classify each finding: (a) must go before publishing, (b) harmless but personal, better moved, (c) fine.
- **The history question:** if anything of class (a) is in the history, propose the options and their cost, and let Norbi choose:
  - rotate the secret and keep the history (enough when the secret is revoked);
  - rewrite the history with `git filter-repo` (every clone and worktree breaks; the open PRs and the handoff commit references change);
  - publish a fresh repository with a squashed first commit.

### 0.2 What stays private

Decide with Norbi which parts are internal:
- `CLAUDE.md`, `docs/handoff/*`, `docs/05-homelab-telepites.md`, `docs/10-kollega-beallitasa.md`, `task.md`, `deploy/compose.yaml` (homelab-specific);
- Norbi's memory and agent configuration.

Options:
- keep them public as a "how it was built with agents" story (it may even be a selling point);
- move them to a private companion repository;
- generalise them.

Propose one; note that the agents' working instructions (`CLAUDE.md`) are read by Claude Code in every round, so moving them changes the workflow.

### 0.3 License (decided: AGPL-3.0 + CLA; check that it works)

- **Dependency licences:** run `go-licenses` (Go) and `license-checker` (npm, `web/`), list everything that is not MIT/BSD/Apache/ISC, and check the vendored Monaco editor's licence and notices. Confirm each one is compatible with AGPL-3.0 **and** with a commercial licence of the same code (a dependency that only allows GPL-family use would block the dual licence: flag it).
- **No borrowed AGPL code:** confirm that nothing was copied from Mattermost, Dendrite, Synapse or Stoat (`CLAUDE.md`, "Licenc"). With dual licensing this matters even more: Norbi can only sell a commercial licence for code he owns.
- **Ownership of the existing code:** today every commit is Norbi's (two author names, both his; agent work is committed under his name). Confirm this with `git log --all`, and list any other author that appears (e.g. Norbi's colleague after round 38) with whose permission or CLA Norbi needs before the commercial licence covers their commits.
- **SPDX identifier: `AGPL-3.0-only`** (decided, 2026-11-11; as Synapse). Use it in `LICENSE`, `README.md`, `package.json` files and the GHCR image labels.
- **AGPL section 13 (network users must be offered the source):** propose how the server shows a "Source code" link (e.g. in the PWA's About or Settings page and in `/api/v1/version`), configurable so a modified deployment points to its own source.
- **The CLA mechanism:** propose the CLA text (Apache ICLA-based, with the commitment above, plus a corporate variant only if cheap) and how it is signed on GitHub (CLA Assistant or the `contributor-assistant` GitHub Action; no secrets needed for forks). Say what it costs contributors.
- Copyright holder: **Ujj Norbert** (decided, 2026-11-11), with the year; the `LICENSE` file; source-file headers (propose: no headers, one `LICENSE` + `NOTICE`).
- How a buyer gets the commercial licence: one paragraph for the README. The only contact is **norbert.ujj@gmail.com** (decided, 2026-11-11); use it in the README, `SECURITY.md`, the CLA and the Code of Conduct, and nowhere else. The commercial terms themselves are out of scope.

### 0.4 The name and the brand

- Is "Colonyhall" free to use? Search GitHub, npm, Docker Hub, domains and an EU/US trademark database (by name only, say what you searched).
- Note whether a domain should be registered before the announcement.

### 0.5 Community files

The repository needs:
- `CONTRIBUTING.md`: how to build, test and send a PR; the layer rules from `CLAUDE.md` in short;
- `CODE_OF_CONDUCT.md`: Contributor Covenant 2.1;
- `SECURITY.md`: how to report a vulnerability privately (GitHub private vulnerability reporting), the supported versions;
- issue and PR templates, `CODEOWNERS`, `.github/FUNDING.yml` if Norbi wants it.

### 0.6 Documentation language

- Today the design docs are Hungarian, while code, commits and the README are English.
- Propose what a public visitor needs in English: README, install guide, architecture overview, contributing guide, the API reference generated from `api/openapi.yaml`.
- The Hungarian docs can stay for Norbi.

### 0.7 GitHub settings (Norbi applies them)

The checklist covers:
- branch protection on `main`;
- required checks;
- Dependabot (Go modules, npm, GitHub Actions);
- CodeQL;
- secret scanning and push protection;
- private vulnerability reporting;
- Discussions or not;
- the topics and description;
- the social preview image.

### 0.8 Telemetry and privacy statement

- Confirm the server and the guard send nothing anywhere except to the user's own server and the AI providers the user chose.
- Write that down for the README.

### 0.9 The order of the day of publishing

A numbered checklist for Norbi, from the last rotation of secrets to switching the visibility, the first public release (round 41's checklist) and the announcement.

**Stop here** and tell Norbi in the chat what you found and what he has to decide.

## 1. Clean-up (after the gate)

- Remove or generalise the class (a) and (b) findings in the working tree, as approved: config examples with placeholder addresses, documents moved or generalised.
- Add `.gitleaks.toml` and a CI job that runs gitleaks on every PR.
- **Tests:** a test or CI check that fails if one of Norbi's addresses or host names appears again in a tracked file outside the private parts.

## 2. License and notices

- `LICENSE` (the AGPL-3.0 text, unmodified), `NOTICE` (third-party notices, including the vendored Monaco editor), the licence section in `README.md` (AGPL-3.0, the commercial licence option, the CLA).
- `CLA.md` and the signing check (CLA Assistant config or the `contributor-assistant` workflow), **written but not enabled**: Norbi enables it.
- The "Source code" link required by AGPL section 13, as approved at the gate, with a test.

## 3. Community files and templates

As in 0.5.

## 4. Public docs and CI

- The English docs from 0.6.
- An API reference page generated from `api/openapi.yaml` (it can live on the GitHub Pages site later).
- CI that works for forks: no secrets needed for tests, and `make test` runs on a public runner.

## 5. The GitHub Pages site (second design gate)

### 5.1 Vision (Norbi fills this in later)

> **TODO (Norbi):** the vision, the mood, the references, what the visitor should feel and do. Not a basic landing page: something much more complex.

**Until Norbi has written this section, do only 5.2.** Then propose a design in `docs/18-weboldal-terv.md` (Hungarian), and **stop** for his approval before building it. The design covers:
- the concept;
- the page structure;
- interactions and motion;
- the tech choice and why;
- how it shows the product without real data;
- a performance budget.

### 5.2 Groundwork (allowed now)

- A `site/` folder, separate from `web/`, its own `package.json`. No dependency on the app's build.
- A GitHub Actions workflow that builds it and deploys it to GitHub Pages **only on manual dispatch** until Norbi enables Pages. Write it, do not enable it.
- Rules the site must keep whatever the vision is:
  - **no tracking, no cookies, no third-party scripts or fonts loaded at runtime** (self-host everything);
  - works without JavaScript for the core text;
  - respects `prefers-reduced-motion` and `prefers-color-scheme`;
  - accessible (WCAG AA contrast, keyboard, alt texts);
  - fast (set a budget, measure it in CI with Lighthouse CI or similar);
  - English first, Hungarian optional.
- **Assets that show the product** must use invented demo data (a seeded demo workspace with made-up agents, channels and messages), never screenshots of Norbi's real workspace.
  - Add a `make demo-seed` (or reuse the dev seed) that produces the demo state, so screenshots and recordings can be regenerated.

## Out of scope

- Changing the visibility, history rewrites, releases, Pages deployment and GitHub settings: Norbi does these from the checklist.
- A hosted or paid offer: only the licence consequences are in scope.
- The site's actual design and build: they wait for 5.1.

## Report

`docs/handoff/48-open-source-report.md` (Hungarian), with:
- the commits;
- the audit results (counts per class, no secrets);
- the decisions Norbi made at the gates;
- the publishing-day checklist (final);
- what is still open for the site;
- what the reviewer should look at first (the history scan and its coverage, the licence compatibility table, the list of code authors for the CLA, the leftover personal data check).
