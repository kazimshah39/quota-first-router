# Routing and health monitor (unpublished 0.2.0 candidate)

This read-only page shows cached quota and **scheduler decisions**, not actual served accounts, request success, token/cost analytics or a permanent queue. CPA can retry after a scheduler choice; concurrent traffic replaces the global latest choice. Use host Request Events/Logs for actual outcomes. No production monitoring rollout has occurred.

## Open and authenticate

In compatible Oh My CPA, choose **Plugins → Quota First Router**. CPA registers its menu at `/v0/resource/plugins/quota-first-router/monitor`. The same path can be opened directly in CPA. OMC v0.4.13 embedding and standalone CPA 8.1.0 were browser-tested on Linux Chrome with synthetic accounts.

HTML/CSS/JS resources are public but generic and contain no diagnostics. Private data comes only from management-authenticated `GET /v0/management/plugins/quota-first-router/monitor`. The existing authenticated `/status` schema is preserved. Existing supported same-origin host credentials can be reused; no key is placed in assets/URLs/logs or newly persisted. Direct/fallback entry keeps the entered management key in memory for this page only. **Forget key** clears all displayed private account/model/history details; 401/403 clears them and stops retries until login. It does not log out the parent host or delete that host's saved login. Never paste credentials into URL parameters. Host-stored login remains governed by the host.

## Read the page

- **Healthy observations:** current worker/cache coverage looks healthy, not evidence that inference succeeds. Accounts may still be exhausted.
- **Waiting for quota:** accounts are discovered but initial quota reads have not completed. **No eligible accounts:** no cached eligible Codex accounts. Neither means broken inference by itself.
- **Needs attention:** stale/unknown/reset-pending data, current polling error, roster failure or truncated account coverage. **Worker failed:** recovered panic/failure flag; check host logs and reconfigure/reload through Plugin management.
- **Account order:** the timestamped latest offered-candidate snapshot for the selected observed model. Higher numeric tiers first, then earliest known five-hour reset. An unoffered account's tier/model eligibility is **unknown**, not disabled. Pinning, model filters, cooldowns and retries are host-owned.
- **Quota usable/exhausted/stale:** based on observed blocking windows and server time/max_age. Exhausted is known-empty; unknown/stale top-tier quota delegates safely to CPA. A passed reset is **waiting for fresh quota**, not assumed replenished. Explicit monthly-only allowance ranks after known five-hour resets within its tier. Model windows are displayed independently; model filtering never sends inference or quota requests.
- **Refresh/errors:** last successful quota-read age, next scheduled attempt and current sanitized error/failure count. A recovered incident is historical, not a current failure.
- **Latest/recent scheduler choices:** active choice, shadow proposal or delegation, timestamp/model/stream/mode. Multiple scheduler decisions may belong to one retried request; there is no reliable per-request correlation in this API.

## Refresh and history limits

The UI fetches cached diagnostics every five seconds while visible. **Pause updates** stops automatic refresh and aborts an in-flight read; **Refresh now** remains available. Hidden tabs suspend/abort reads; returning visible resumes unless paused. Only one read is in flight; requests have a 10s timeout and bounded backoff up to 60s. Failures retain clearly labeled stale data except authentication rejection, which clears it. UI reads do not trigger provider quota GETs, inference, settings changes or scheduler picks. Quota polling remains 1m, max age 5m, timeout 10s, reconciliation 30s by default.

History is memory-only: up to **200 decisions**, **200 coalesced incident records**, **100 candidate/exclusion details per decision**, **1,000 account rows**, bounded labels and **8 MiB API response**. Eviction/truncation counts are shown. Long labels are truncated; do not use the display as a unique identifier for colliding long names. Epoch/start time changes on restart/reconfiguration, clearing history/counters. No database, durable export, alerts or raw provider error bodies. Account labels may contain personal information, so access is management-only. All rendering uses text nodes, never account-provided HTML.

Disabled/unloaded plugins may lose both page and API registration. A 404 means unavailable, not confirmed disabled without host evidence. Use the host's Plugin management to check enablement/rollback. Resource CSP permits same-origin scripts/styles/connect/framing only; a new external dashboard origin is not automatically supported.

## Verification and release boundaries

Local candidate checks include Go race/unit/vet/format, offline packaging, Node controller race tests, actual CPA resource/menu/auth/routing integration, standalone and embedded Chrome checks and a five-minute concurrent monitor/routing history-rollover soak. See `docs/evidence/0.2.0/README.md` for measured results and honest pending gates.

Six candidate shared libraries build and pass machine/export/dependency/checksum checks. Linux amd64 exact-ZIP native acceptance additionally verifies protected monitor/public shell and no polling/pick side effects. **The other five changed-feature native runs remain pending separately approved remote execution.** v0.1.2's native passes do not validate this new feature. No feature release/tag/upload, store-review surface update or production rollout is implied. Candidate version 0.2.0 is tentative until final remote-version checks.
