# Operation and rollback

## Unpublished monitor candidate

See [monitor guide](monitor.md) for menu/direct paths, memory-only fallback login, public-shell/private-API split, health/reason meanings, history reset/eviction and disabled-route recovery. This read-only page does not refresh provider quota, change config or prove actual request outcomes. Production remains public v0.1.2; no candidate rollout is approved. New routes need feature-specific native/browser/publication gates before any rollout.

## Before a rollout

1. Confirm CPA 8.1.0, management access, native library architecture, existing scheduler plugins, and current priorities. Do not change account tiers for this plugin.
2. Run `make test`, `make build`, and the isolated `make integration-test` first.
3. Store a mode-0600 configuration backup outside Git. Record its SHA256 and the plugin artifact SHA256. Do not put OAuth material, API keys, or management secrets in project files or reports.
4. If CPA uses `requests.proxy-url`, copy that value to this plugin's `proxy_url` without exposing it in logs. Per-account settings take precedence.

## Shadow-first canary

For the published Linux release, place `quota-first-router.so` as a regular file in the resolved platform plugin directory, then configure it with `enabled: true`, `priority: 100`, `mode: shadow`. Use the existing management config API (or the dashboard plugin config editor). Verify `registered` and `effective_enabled` in the plugins list and authenticated status; do not assume a file copy loaded it. Existing plugin settings must be preserved.

Wait for trustworthy quota snapshots. Check every enabled Codex account: five-hour utilization/reset, long-window quota, errors and freshness. Do not infer correct behavior from a status endpoint existing. Send a minimal normal Codex request and check that `last_decision` proposes the nearest reset in the highest usable tier, while shadow mode changes no actual choice. Compare with an independently computed snapshot order. Existing traffic may update `last_decision`; use an isolated canary instance if request attribution is ambiguous.

## Active canary gate

Only proceed after shadow evidence is correct. PATCH config with `{"mode":"active"}`. Wait for its mode and refreshed snapshots, then send a minimal request. Observe the account actually selected through CPA's request usage records; compare it to the proposal and current host cooldown/eligibility state. Do not deliberately exhaust real accounts. A small number of failover attempts is acceptable if the host reports a real failure; stop repeated attempts if all accounts cannot serve the canary model.

Run a real-account canary only with the operator's authorization. It can consume a small amount of model quota. The optional tests/live_canary.py requires an already installed library, explicit configuration/environment/database paths and an authorization flag; it never copies a library over a potentially mapped artifact and is never run by public CI.

## Rollback

PATCH `/v0/management/plugins/quota-first-router/enabled` with `{"enabled":false}`. Verify `effective_enabled` is false and ordinary routing works again. This restores existing CPA routing without changing tiers or interrupting running requests. Keep the plugin installed but disabled after a canary unless an ongoing active deployment was explicitly requested. Confirm account priority/disabled fields remain unchanged; token refreshes performed by CPA itself are not routing changes.

If full config restoration is needed, reconcile unrelated concurrent edits first; never overwrite newer user changes blindly. Prefer disabling/removing only this plugin's config entry. Replace native artifacts only while unloaded. Do not kill the shared proxy to perform routine rollback.

## Candidate platform limits

Version 0.1.2 has six archives using `.so` (Linux), `.dylib` (Darwin), and `.dll` (Windows). All six passed matching native CPA/store/routing acceptance; actual publication and protected rollout state is in release-status.md. Follow [platform-support.md](platform-support.md) for exact packages, pinned tools, runtime limits and disposable-native tests. Never treat a valid header/export as proof of loadability. Native CA fixtures must not be run on a personal/shared macOS or Windows machine. Monitoring is deferred.

## Native version upgrades

Disabling a plugin does not prove its native library has been unmapped. Never overwrite a file CPA may still have mapped. The store installer can report a loaded-artifact lock; do not bypass it by copying onto that path.

The verified 0.1.0 → 0.1.1 live upgrade used a separately checksum-verified release extracted into a **new regular file**, `quota-first-router-v0.1.1.so`, in the same platform directory. CPA 8.1.0's plugin config `store.version` was set to `0.1.1` (with matching repository/id/release-tag metadata), along with explicit shadow mode. CPA selected that versioned artifact. Verify the management listing's actual path, registered/effective-enabled fields, status version, and loaded file hash before active use. Preserve all other plugin configuration. Keep previous files and protected backups for rollback; changing desired version back is a separate controlled rollout, not permission to overwrite either file.

This upgrade path was tested against CPA 8.1.0 only. For other hosts use their supported lifecycle/install procedure. A disposable test-host restart for same-version reinstall is not evidence that restarting shared production is required or safe.

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
