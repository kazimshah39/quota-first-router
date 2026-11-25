# Verification

## Compatibility and public checks

The release targets **CPA 8.1.0**, native ABI 1/schema 10, **Linux amd64/glibc 2.34+** only. The module requires Go 1.26.10+. Build flags enforce baseline x86-64 instructions.

Reproduce:

```sh
make GO=go test package cpa
make GO=go soak-test SOAK_SECONDS=120
```

The CPA download is verified against its release checksums and a pinned archive SHA256. The plugin ZIP contains only its correctly named root library, LICENSE, and THIRD_PARTY_NOTICES.md; checksums use SHA-256. Public CI has no personal-account credentials. Release workflows require tag/source version agreement and repeat the relevant checks before creating a draft.

## Coverage

- Policy: tier-before-reset, nearest reset inside tier, all-top/all-pool exhaustion, weekly/model limits, ties, unknown reset, unknown/stale/elapsed quota fallback, identity changes, disabled accounts, mixed-provider isolation.
- Provider/runtime: malformed responses, monthly-only allowed accounts, global denial, bounded body/timeouts, proxy enforcement, redirects, 401/403 invalidation, transient-cache retention, roster reconciliation, concurrent picks/status and redacted errors.
- Actual CPA fixtures: authenticated status, shadow non-interference, active routing, all-tier candidates, streaming, concurrent requests, no request-path quota calls, five-hour/weekly exhaustion, lower-tier fallback, all-empty no upstream attempt, recovery/rollover, 429 retries, explicit pinning, model eligibility, account disable/reenable, competing scheduler precedence and hot-disable rollback.
- Bounded soak: repeated streaming/non-streaming/account routing under GC pressure while background quota refresh continues.
- Release packaging: expected root paths/ELF architecture and checksum validation; corrupted checksum/unexpected paths rejected.

## Prior private live verification

A bounded private multi-account canary succeeded before public release preparation:

| Phase | Actual behavior |
|---|---|
| Shadow | Built-in routing chose account A; plugin proposed account B without overriding it |
| Active | Account B handled the request, confirmed by separately collected usage attribution |
| Disabled rollback | A normal request succeeded with the plugin disabled |

Account tier priorities and enabled/disabled flags were unchanged. The live experiment exposed a monthly-only quota response; it was fixed and covered by a regression. No live account was deliberately exhausted. Personal evidence and configuration backups are not part of the public repository or history.

## Known boundaries

- No immediate file-deletion guarantee: CPA may retain removed credentials during watcher reconciliation. The plugin removes its cached entry when CPA's roster drops it; disable an account for immediate operator removal.
- No Claude, Home dispatch, untested native platforms, or forced rerouting of pinned/retained connections.
- Quota can change between polling and inference. CPA retries an account that fails after selection.
- Global proxy must be set explicitly in plugin configuration if CPA uses one; custom base URLs are explicitly unsupported for quota polling.
- Native plugins share CPA's process, and provider quota APIs can change. Checksums establish artifact integrity, not a guarantee of provider compatibility forever.

See GitHub Actions for published-commit results and docs/operations.md for install/rollback steps. Official-store availability depends on the upstream registry PR; custom-source installation can be used independently.
