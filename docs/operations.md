# Operation and rollback

## Before a rollout

1. Confirm CPA 8.1.0, management access, native library architecture, existing scheduler plugins, and current priorities. Do not change account tiers for this plugin.
2. Run `make test`, `make build`, and the isolated `make integration-test` first.
3. Store a mode-0600 configuration backup outside Git. Record its SHA256 and the plugin artifact SHA256. Do not put OAuth material, API keys, or management secrets in project files or reports.
4. If CPA uses `requests.proxy-url`, copy that value to this plugin's `proxy_url` without exposing it in logs. Per-account settings take precedence.

## Shadow-first canary

Place `quota-first-router.so` as a regular file in the resolved platform plugin directory, then configure it with `enabled: true`, `priority: 100`, `mode: shadow`. Use the existing management config API (or the dashboard plugin config editor). Verify `registered` and `effective_enabled` in the plugins list and authenticated status; do not assume a file copy loaded it. Existing plugin settings must be preserved.

Wait for trustworthy quota snapshots. Check every enabled Codex account: five-hour utilization/reset, long-window quota, errors and freshness. Do not infer correct behavior from a status endpoint existing. Send a minimal normal Codex request and check that `last_decision` proposes the nearest reset in the highest usable tier, while shadow mode changes no actual choice. Compare with an independently computed snapshot order. Existing traffic may update `last_decision`; use an isolated canary instance if request attribution is ambiguous.

## Active canary gate

Only proceed after shadow evidence is correct. PATCH config with `{"mode":"active"}`. Wait for its mode and refreshed snapshots, then send a minimal request. Observe the account actually selected through CPA's request usage records; compare it to the proposal and current host cooldown/eligibility state. Do not deliberately exhaust real accounts. A small number of failover attempts is acceptable if the host reports a real failure; stop repeated attempts if all accounts cannot serve the canary model.

Run a real-account canary only with the operator's authorization. It can consume a small amount of model quota. The optional tests/live_canary.py requires explicit configuration/environment/database paths and an authorization flag; it is never run by public CI.

## Rollback

PATCH `/v0/management/plugins/quota-first-router/enabled` with `{"enabled":false}`. Verify `effective_enabled` is false and ordinary routing works again. This restores existing CPA routing without changing tiers or interrupting running requests. Keep the plugin installed but disabled after a canary unless an ongoing active deployment was explicitly requested. Confirm account priority/disabled fields remain unchanged; token refreshes performed by CPA itself are not routing changes.

If full config restoration is needed, reconcile unrelated concurrent edits first; never overwrite newer user changes blindly. Prefer disabling/removing only this plugin's config entry. Replace native artifacts only while unloaded. Do not kill the shared proxy to perform routine rollback.

## Failure behavior

- Missing/stale quota in the highest usable tier: normal CPA fallback, not a lower-tier guess.
- Fresh quota exhaustion: excluded for that selection; lower tiers can become usable. All observed accounts exhausted: normal host no-capacity result.
- 401/403 during quota polling: discard the snapshot. Do not refresh tokens independently or mark the account permanently disabled.
- Other quota errors: keep the previous success until it ages out and use bounded exponential backoff plus Retry-After.
- Past reset: refresh, not assumed quota restoration; another exhausted window still blocks.
- Account removed/disabled or credential identity changed: invalidate/reconcile cached state; CPA filters current request candidates immediately.
- Unsupported custom base URL/proxy format: report a fixed redacted error and delegate; never silently connect around it.
- Another higher-priority scheduler: CPA 8.1.0 invokes that scheduler instead; this plugin's decision counters remain unchanged. Plugin priority and account tier priority are separate.

## Adding Claude later

Implement a Claude credential/quota adapter with separate polling/auth headers and quota fixtures. Keep the Codex-only request guard until Claude-specific selection and real-host isolation tests pass. Do not combine providers' tier pools or compare unrelated model limits.
