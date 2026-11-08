# Handoff prompt 44 (DRAFT): archiving old messages to long-term storage

> **Status: draft for information only. Do not start from this file.** Norbi and Claude Code discuss and work it out in detail first (decided 2026-11-07). When the design is agreed, this draft is replaced by a real handoff prompt with the usual branch, worktree and test rules.

## Why (Norbi, 2026-11-07)

Messages, events, runs and their steps are never deleted (append-only, rule 4). That is right, but the live database keeps growing, queries and backups get slower. Norbi wants:

- **Nothing lost.**
- After **30 or 60 days** (to be decided), old messages move to **long-term storage**, out of the database the system runs on, to keep it fast.
- They can be **brought back into the chat at any time**; it may take longer.

## Open questions to settle before the real prompt

- **Is it needed yet, and how much?** At today's size Postgres with the right indexes handles millions of rows easily. Measure first: table sizes, growth per month, the slow queries. The first step may be time **partitioning** (below) alone; moving data out may come later.
- **The tiers** (industry practice: hot, warm, cold):
  - **hot:** the live tables, the last 30–60 days;
  - **warm:** older monthly **partitions** of the same tables (Postgres native partitioning by month), still queryable, cheap to detach;
  - **cold:** detached partitions exported to compressed files (e.g. JSON Lines or Parquet, with checksums) on disk or object storage (a MinIO bucket on the homelab, or an S3-compatible service later), and dropped from the database.
- **What moves:** messages and their versions; events (the event log is the source of everything else: what do the consumers need, what does the audit chain need); runs and their step lists (probably the earliest, they are large); files (the blobs are already outside the database).
- **What stays hot even when old:** channel and thread metadata, pinned or starred messages, decisions (`D-NN`), wiki, compaction summaries (they already summarise old conversations), a small stub per archived message so threads and replies keep their shape.
- **Bringing back:** per channel and time range, by a human (and an agent?), into a "restored" partition or a read-only view; how long it stays hot after a restore; what the PWA shows ("older messages are archived: load them" with a progress state).
- **Search (round 36):** are archived messages still searchable? (Their chunks may stay in the search index with a flag, so a hit offers "restore".)
- **Rules that change:** archiving moves data, it does not delete it, but rule 4 and the audit hash chain must say how a moved row is verified (checksums of the exported files, recorded in the audit log).
- **Backups:** the cold files need their own backup and a restore test (the restore drill Norbi postponed).

## Rough parts (to be detailed)

1. Measurements and the decision (partitioning only, or partitioning plus cold export).
2. Migrating the large tables to monthly partitions without downtime (expand–contract, rule 6).
3. The archive job: detach, export with checksums, verify, drop; idempotent and resumable; audited.
4. Restore: import a range back, the PWA's "load archived" flow, expiry back to cold.
5. Status page: sizes per tier, the last archive run, restore jobs.
6. Tests: a full archive-and-restore round trip with identical content, an interrupted export resumes, a thread spanning the boundary still renders.
