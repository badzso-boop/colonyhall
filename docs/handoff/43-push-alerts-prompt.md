# Handoff prompt 43 (DRAFT): push notifications and alerts

> **Status: draft for information only. Do not start from this file.** Norbi and Claude Code discuss and work it out in detail first (decided 2026-11-07). When the design is agreed, this draft is replaced by a real handoff prompt with the usual branch, worktree and test rules.

## Why (Norbi, 2026-11-07)

1. **Push notifications for messages** were planned "later" since the start (mentions, DMs, questions waiting for an answer, approvals) and are still missing.
2. **Alerts:** the status page (round 35) shows problems only while someone looks at it. A guard going offline, a full disk, a missed backup, a stuck event consumer, the emergency stop, or (after round 36) the Raspberry Pi or the reranker being down must reach Norbi's phone.
3. **One channel for both:** alerts use the same push notifications as messages (Norbi's decision).

## Open questions to settle before the real prompt

- **Delivery:** Web Push from the PWA (service worker, VAPID keys; on iPhone only for a PWA added to the home screen) is the natural choice. Is a fallback needed when the browser is closed for days (e.g. ntfy, Telegram, e-mail)? Which one, and is an outside service acceptable?
- **Without HTTPS:** browsers allow Web Push only on HTTPS (or localhost). The server is HTTP over Tailscale during development (round 40 decision). This blocks Web Push until HTTPS exists. Options: HTTPS through `tailscale serve` only for this, a fallback channel first, or waiting.
- **What notifies:** mentions, DMs, thread replies, questions to humans, approval requests, delegation requests; per channel muting already exists (round 34). Quiet hours, per-device settings.
- **Which alerts, with which thresholds** (start from the status page's: consumer lag above 100 events or 60 s, a guard offline after 90 s, a failed schedule), plus disk space, backup age, Pi/reranker reachability, error rate of agent runs, Claude or Antigravity limits near the reserve. Severity levels; repeat and resolve notices (an alert is sent once and a "resolved" when it is over, not every minute).
- **Who gets alerts:** the owner only, admins, configurable.
- **Where the checks run:** in the server (a watcher on the status data) and in the guards (disk space, a local client problem).
- **Privacy:** what text appears on a lock screen (message content or only "new message in #channel").

## Rough parts (to be detailed)

1. Push subscriptions (contract first; per person and device), Web Push sending (VAPID keys as secrets), the service worker's notification handling, opening the right channel or thread on tap.
2. Notification rules and settings in the PWA (round 32+ settings), quiet hours.
3. An alert engine on the server: rules, state (firing, resolved), deduplication, history on the status page.
4. Guard-side checks reported in the host report.
5. Tests: subscription lifecycle, rule evaluation with a manual clock, deduplication, no secret in a payload.
