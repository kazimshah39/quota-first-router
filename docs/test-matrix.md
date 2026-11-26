# Comprehensive validation matrix

Historical public-release target: CPA 8.1.0 (`69607634`), Linux amd64/glibc 2.34+. Local 0.1.2 six-target build and native acceptance status: [platform evidence](platform-support.md). The five nonlocal native results remain pending. Tests use synthetic credentials unless explicitly described as live. A pass is not a claim that every provider behavior or future CPA version is supported.

## Automated coverage

| Behavior | Check | Result |
|---|---|---|
| Highest usable tier before reset ordering, deterministic ties | policy tests; 1,000 seeded randomized 20-account rosters and shuffled candidate orders | Pass |
| Five-hour, weekly/monthly, explicit monthly-only allowance, reset boundaries | quota/policy tests | Pass |
| Stale/future observations and unknown quota delegate without dropping tier precedence | stale-boundary tests, policy tests and real-host quota outage | Pass |
| Additional model limits: array/object, normal slug, camelCase, duplicates, exact names | `TestAdditionalQuotaShapes` | Pass; regression fixes in 0.1.1 |
| Malformed quota fields, spend-control exhaustion, proxy validation | comprehensive/quota/config tests | Pass; regression fixes in 0.1.1 |
| Credential replacement, disabled/deleted roster, corrupt credentials, cancellation/recovery | runtime/comprehensive tests | Pass |
| Redirect rejection, request deadline, bounded body, HTTP errors/backoff, account proxy | HTTP quota tests | Pass |
| Worker/lifecycle concurrency and panic fallback | runtime tests, Go race detector | Pass; ten repeated race runs on final patch |
| Quota parser robustness | bounded native Go fuzzing | Initial patch: 312,983 inputs; final patch: 340,107 inputs in 20 seconds, no crash |
| Selection overhead | `BenchmarkChoose` (three candidates, EPYC host) | Final patch: 1,662 ns/op, 712 B/op, 7 allocations; microbenchmark, not end-to-end latency |
| ABI/schema registration, authenticated diagnostics | actual CPA binary integration | Pass |
| Responses and Chat Completions, stream/nonstream | actual CPA binary fixture | Pass |
| Explicit pin, disabled/model filters, competing scheduler | actual CPA binary fixture | Pass |
| Tier exhaustion, all-empty no upstream, rollover, recovery | actual CPA binary fixture | Pass |
| Unknown quota fail-open in highest tier, no request-path quota GET | actual CPA binary fixture | Pass |
| Upstream 500/502/503/429/401 failover | actual CPA binary fixture | Pass; recovery is host-controlled, see limits below |
| Function definitions/output payloads, client disconnect followed by healthy request | actual CPA binary fixture | Pass |
| Parallel routing and resource-limited mixed-protocol soak | eight workers, `GOGC=20`, `GOMEMLIMIT=256MiB`, RSS assertions | Ten-minute final-patch run recorded in release status |
| Release ZIP/checksum/layout validation | Python packaging tests and actual CPA store installer | Version-specific evidence in release status |

Run: `make test integration-test`; `make soak-test SOAK_SECONDS=600`. Fuzz: `go test -run '^$' -fuzz FuzzParseCodexQuota -fuzztime=20s -parallel=4`. Benchmark: `go test -run '^$' -bench . -benchmem`.

## Live and dashboard checks

With explicit operator authorization, public v0.1.0 and then v0.1.1 were downloaded and checksum-verified, then loaded from new versioned files without overwriting a mapped library or restarting shared CPA. Nine enabled Codex accounts were discovered. Protected configuration/library backups and detailed account evidence remain outside tracked files.

Each of the v0.1.0 and v0.1.1 live matrices passed **26/26** bounded requests across four discovered available models, both protocols and streaming modes, eight concurrent requests and a forced tool call. Read-only OMC records attributed all 26; all **25 active** requests agreed with the plugin proposal. The shadow request kept the host-selected account. The final v0.1.1 six-request gate independently checked top-tier/reset ranking and actual usage attribution for all four models, then verified the forced function call and its output follow-up. Model names/account IDs are deliberately omitted from public evidence.

Chrome DevTools browser testing verified plugin discovery, five declared fields, saved shadow/active modes, enabled switch, disable/reenable, and a successful ordinary request while disabled. A briefly stale dashboard during reenable resolved on refresh; no browser data was cleared. The final public patch, installer, natural refresh observation, account-field preservation and active deployment results are recorded in `release-status.md`.

## Boundaries and observed host behavior

- 401 failover succeeds, but verification does not necessarily clear CPA's unauthorized state. 429 recovery can wait for a host cooldown even after successful credential verification. The test waits for bounded recovery; the plugin does not own or clear host cooldowns.
- A mixed-protocol soak initially encountered an unexpected choice. The harness now records full private diagnostics for any deviation and checks that traffic remains in the usable top tier. Later successful runs must not be used to pretend that first run passed.
- Tool pass-through is tested synthetically; a live forced function call completed. Full application tool execution, realtime/WebSocket transport, every model and every alias are not claimed.
- Cancellation checks client disconnect and subsequent health, not a provider-side billing/refund or remote-abort guarantee.
- No real quota was deliberately exhausted. Failure/exhaustion tests use network-isolated fixtures. Long-term daily reliability is not established by a ten-minute soak.
- The native artifact can remain mapped after disable. The optional canary now requires an already installed library; deployments use new versioned files. Disposable-host restart/reinstall and live version switching are distinct tests.
- Official-store PR #402 is closed; issue #1 remains open for six-platform coverage. The custom registry serves existing Linux releases. No claim of official listing or full-platform runtime acceptance is made.
