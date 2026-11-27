# Routing and health monitor (unpublished 0.2.0 candidate)

This read-only page shows cached quota and **scheduler decisions**, not actual served accounts, request success, token/cost analytics or a permanent queue. CPA can retry after a scheduler choice; concurrent traffic replaces the global latest choice. Use host Request Events/Logs for actual outcomes.

## Open and authenticate

In compatible Oh My CPA, choose **Plugins → Quota First Router**. CPA registers its menu at `/v0/resource/plugins/quota-first-router/monitor`. The same path can be opened directly in CPA. OMC v0.4.13 embedding and standalone CPA 8.1.0 were browser-tested on Linux Chrome with synthetic accounts.

HTML/CSS/JS resources are public but generic and contain no diagnostics. Private data comes only from management-authenticated `GET /v0/management/plugins/quota-first-router/monitor`. The existing authenticated `/status` schema is preserved. Existing supported same-origin host credentials can be reused; no key is placed in assets/URLs/logs or newly persisted. Direct/fallback entry keeps the entered management key in memory for this page only. **Forget key** clears all displayed private account/model/history details; 401/403 clears them and stops retries until login. It does not log out the parent host or delete that host's saved login. Never paste credentials into URL parameters. Host-stored login remains governed by the host.

## Read the page

- **Healthy observations:** current worker/cache coverage looks healthy, not evidence that inference succeeds. Accounts may still be exhausted.
- **Waiting for quota:** accounts are discovered but initial quota reads have not completed. **No eligible accounts:** no cached eligible Codex accounts. Neither means broken inference by itself.
- **Needs attention:** stale/unknown/reset-pending data, current polling error, roster failure or truncated account coverage. **Worker failed:** recovered panic/failure flag; check host logs and reconfigure/reload through Plugin management.
- **Account order:** the timestamped latest offered-candidate snapshot for the selected observed model. Higher numeric tiers first, then earliest known five-hour reset. An unoffered account's tier/model eligibility is **unknown**, not disabled. Pinning, model filters, cooldowns and retries are host-owned.
- **Account titles and plans:** titles use the email reported by CPA when available; `@googlemail.com` displays as `@gmail.com` without changing the credential. The card shows the subscription reported by quota data, such as Plus or Free, when known. **Details** shows the numeric routing priority, which is separate from the subscription.
- **Quota bars:** each account card shows the percentage **left**, not used, with its reset countdown. Green means at least 70% left, amber at least 25%, and red below 25%, matching Oh My CPA’s Codex quota view. Muted bars show cached data when quota is stale or waiting for a reset refresh. Unknown readings display “Not reported,” not a zero bar. Reset times use Oh My CPA’s minutes, hours and days format (for example `19m left` or `2d 4h left`); hover over a reset time to see its exact timestamp. Open **Details** for polling errors and other/model limits. **Routing activity** combines decisions and incidents, newest first, showing six entries until expanded with **Show more**. Filters never send inference or quota requests.
- **Quota usable/exhausted/stale:** based on observed blocking windows and server time/max_age. Exhausted is known-empty; unknown/stale top-tier quota delegates safely to CPA. A passed reset is **waiting for fresh quota**, not assumed replenished. Explicit monthly-only allowance ranks after known five-hour resets within its tier.
- **Refresh/errors:** last successful quota-read age, next scheduled attempt and current sanitized error/failure count. A recovered incident is historical, not a current failure.
- **Latest/recent scheduler choices:** a scheduler choice or delegation, timestamp, model and streaming status. Multiple scheduler decisions may belong to one retried request; there is no reliable per-request correlation in this API.

### Selection reasons

| Displayed reason | Meaning |
| --- | --- |
| Earliest five-hour reset in highest usable tier | The chosen account has the earliest known five-hour reset among usable accounts in the highest usable tier. |
| Usable quota; reset time unknown | A usable account was chosen without a known five-hour reset, after known resets in that tier. |
| Unknown quota in highest usable tier; delegated to CPA | The plugin did not have enough current quota evidence in a higher tier to safely pick or descend. |
| No observed usable quota / No known usable quota | The plugin could not make a supported choice; this is not evidence of what CPA served. |
| Not exclusively Codex; delegated to CPA | The request's provider eligibility was outside this plugin's scope. |
| Quota usable / exhausted / stale / unknown | The cache permits this selection, has a known blocking limit, is too old, or has insufficient quota data, respectively. |
| Account disabled / Credential changed | The host reports disabled, or the credential identity changed and the cached quota is unsafe to reuse. |
| Reset passed; waiting for fresh quota | A cached reset time passed; refresh must confirm the actual allowance. |

Polling incidents show sanitized codes such as `quota_http_503`; `credential_list_failed` means the host roster read failed, and `worker_failed` means the monitoring worker reported a failure. `Recovered` means a later quota/roster read succeeded. These codes do not report inference failures.

When optional warmup is enabled, the last outcome appears under an account’s **Details**. **Window confirmed** means two fresh quota reads showed a stable new reset; HTTP success alone does not confirm the timer. **Warmup paused** does not mean routing or quota polling stopped.

## Refresh and history limits

The UI fetches cached diagnostics every five seconds while visible. **Refresh** reads the cache immediately. The embedded page follows the host’s theme; the standalone page follows the browser’s preference. Hidden tabs stop automatic reads and cancel an in-flight read; returning to the page refreshes it. The router's background quota checks continue even if the page is hidden or closed. Only one read is in flight; requests have a 10s timeout and bounded backoff up to 60s. Failures retain clearly labeled stale data except authentication rejection, which clears it. UI reads do not trigger provider quota GETs, inference, settings changes or scheduler picks. Quota polling remains 1m, max age 5m, timeout 10s, reconciliation 30s by default.

History is memory-only: up to **200 decisions**, **200 coalesced incident records**, **100 candidate/exclusion details per decision**, **1,000 account rows**, bounded labels/model quota details and **8 MiB API response**. Eviction/truncation counts are shown. If the cached view cannot fit within the response cap, the API returns 503 rather than an oversized response. Long labels are truncated; do not use the display as a unique identifier for colliding long names. Epoch/start time changes on restart/reconfiguration, clearing history/counters. No database, durable export, alerts or raw provider error bodies. Account labels may contain personal information, so access is management-only. All rendering uses text nodes, never account-provided HTML.

There is no separate routing mode. Enable Quota First Router in CPA to use its scheduler; disable it to return to CPA routing and stop the worker. Disabled/unloaded plugins may lose both page and API registration. A 404 means unavailable, not confirmed disabled without host evidence. Use the host's Plugin management to check enablement/rollback. Resource CSP permits same-origin scripts/styles/connect/framing only; a new external dashboard origin is not automatically supported.

## Verification and release boundaries

Local candidate checks include Go race/unit/vet/format, offline packaging, Node controller race tests, actual CPA resource/menu/auth/routing integration, standalone and embedded Chrome checks and a five-minute concurrent monitor/routing history-rollover soak. See `docs/evidence/0.2.0/README.md` for measured results and honest pending gates.

Six candidate shared libraries build and pass machine/export/dependency/checksum checks. Linux amd64 exact-ZIP native acceptance additionally verifies protected monitor/public shell and no polling/pick side effects. **The other five changed-feature native runs remain pending separately approved remote execution.** v0.1.2's native passes do not validate this new feature. No feature release/tag/upload, store-review surface update or production rollout is implied. Candidate version 0.2.0 is tentative until final remote-version checks.
