# Verification

## Compatibility and public checks

Published releases target **CPA 8.1.0**, native ABI 1/schema 10, **Linux amd64/glibc 2.34+** only. The local 0.1.2 candidate builds six targets; native acceptance is currently Linux amd64 only. See [platform support](platform-support.md) for exact toolchains, commands, binary/runtime minima and pending tests. The candidate pins Go 1.26.10 and baseline x86-64 instructions.

Reproduce:

```sh
make GO=go test package cpa
make GO=go soak-test SOAK_SECONDS=120
```

The CPA download is verified against its release checksums and a pinned archive SHA256. The plugin ZIP contains only its correctly named root library, LICENSE, and THIRD_PARTY_NOTICES.md; checksums use SHA-256. Public CI has no personal-account credentials. The candidate release workflow has manual dispatch only: explicit publication input, approval environment, existing tag/source agreement, successful exact-source CI run and six matching native records are required. Normal CI and tag pushes cannot create a draft or public release. Local lint is not a hosted workflow pass.

## Coverage

- Policy: tier-before-reset, nearest reset inside tier, all-top/all-pool exhaustion, weekly/model limits, ties, unknown reset, unknown/stale/elapsed quota fallback, identity changes, disabled accounts, mixed-provider isolation.
- Provider/runtime: malformed responses, monthly-only allowed accounts, global denial, bounded body/timeouts, proxy enforcement, redirects, 401/403 invalidation, transient-cache retention, roster reconciliation, concurrent picks/status and redacted errors.
- Actual CPA fixtures: authenticated status, shadow non-interference, active routing, all-tier candidates, streaming, concurrent requests, no request-path quota calls, five-hour/weekly exhaustion, lower-tier fallback, all-empty no upstream attempt, recovery/rollover, 429 retries, explicit pinning, model eligibility, account disable/reenable, competing scheduler precedence and hot-disable rollback.
- Bounded soak: repeated streaming/non-streaming/account routing under GC pressure while background quota refresh continues.
- Release packaging: expected root paths/ELF architecture and checksum validation; corrupted checksum/unexpected paths rejected.

## Comprehensive v0.1.1 validation

See [test matrix](test-matrix.md) and [release status](release-status.md) for the expanded regression fixes, final fuzz/race results, 7,616-request ten-minute resource-measured soak, public installer verification, two protocol/model live matrices, tool follow-up, dashboard controls, versioned upgrade and final deployment evidence. Historical checks below remain historical; current results are not inferred from old artifacts.

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

Published v0.1.0 installer checks passed against real CPA: public custom-source discovery, actual GitHub ZIP/checksum download, shadow 1m/5m defaults, active/disable, same-version reinstall after isolated-host restart, followed by the full routing suite against that installed library. See [release status](release-status.md) for asset hashes, successful Actions run links and the official listing PR.

See GitHub Actions for published-commit results and docs/operations.md for install/rollback steps. Official-store PR #402 is closed for incomplete platforms; issue #1 is open. Custom-source installation of the existing Linux release is independent. No resubmission/publication is authorized by local implementation approval.
