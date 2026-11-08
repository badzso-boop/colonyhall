# Handoff prompt 45 (DRAFT, plan only): several customers on one system

> **Status: draft for information only. Do not start from this file.** Norbi and Claude Code discuss and work it out in detail first (decided 2026-11-07). The first real round is a **design document only**; building it needs a stronger server than the homelab.

## Why (Norbi, 2026-11-07)

Colonyhall may become a subscription product. Then several customers (organisations) share one installation, each fully separated from the others.

## Where the project stands

Every table and every call is already bound to a `workspace_id`, and permissions are checked on every call (rule 9). That is the usual starting point for a shared-database model. Missing: sign-up, more than one workspace in practice, per-customer limits, billing, operations for many customers.

## Open questions to settle before the design round

- **Isolation model** (the industry's three options, with their trade-offs):
  - one shared database, `workspace_id` everywhere (what exists), plus **Postgres row-level security** as a second line of defence;
  - a schema per customer;
  - a database (or a whole installation) per customer, for customers who demand it.
- **Guards and agents:** a customer's guards and agents belong to that customer only (enrolment, tokens, the host gateway, the WebSocket fan-out, the event log replay).
- **Files, search, archive:** per-customer storage prefixes or buckets, the search index (round 36) and the archive (round 44) filtered by customer.
- **Limits per customer:** users, agents, machines, storage, API rate, search; the noisy-neighbour problem.
- **Sign-up and accounts:** organisations, invitations, the owner role per organisation, a super-admin for the operator (Norbi) that is separate from customers' admins and audited.
- **Billing:** plans, trials, payment provider, invoices. **Tax:** Norbi runs a sole proprietorship under the Hungarian VAT exemption (AAM); selling a subscription, especially to customers abroad, raises VAT questions (thresholds, cross-border digital services). This needs an accountant before any pricing decision.
- **Operations:** a stronger server or a cloud host, backups and restores per customer, upgrades without downtime, the load balancer and several server instances (a shared event bus first: today the bus is in memory), monitoring per customer.
- **Legal:** terms of service, privacy policy, a data processing agreement for customers (see the GDPR draft, round 46).

## Rough parts of the design round

1. A design document comparing the isolation models for this codebase, with a recommendation.
2. The data model changes (organisations, plans, limits) and the migration path from today's single workspace.
3. The security model (row-level security, the super-admin, cross-tenant tests).
4. Infrastructure sizing and costs.
5. A phased plan: what can be built on the homelab, what needs the new server.
