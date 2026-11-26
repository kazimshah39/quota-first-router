# Local monitor candidate evidence — 0.2.0 (unpublished)

Source: isolated `monitor-completion` branch from stable main `7ed965e`. Original unfinished monitor files and their checksum/tar recovery backup remain untouched in the original checkout. No production/config/account/library changes or remote writes were made for this feature.

## Passing local results

- `make test package`: Go unit/race/vet/gofmt, four release test groups, deterministic Linux package. Focused observation tests cover 200-record capacities, evictions, 100-candidate truncation, coalescing, recovery, redaction, concurrent snapshots, detached slices, unique epochs even with same clock and startup/failed health states.
- `node tests/monitor_ui_test.js`: standard-library-only controller regression for standalone self-observer loop, system theme, one-in-flight fetch, late response after logout, forbidden-response clearing, hidden abort and no persisted entered key. Real browser tests below are separate.
- Actual CPA 8.1.0 integration: public generic resources/menu, protected status/monitor auth, invalid method handling, monitor GET no quota-call side effect, epoch reset, original tier/shadow/active/protocol/streaming/concurrency/exhaustion/stale/recovery/failure/tool/cancellation/pinning/scheduler/rollback checks passed.
- 300s exact local-library mixed protocol/concurrent routing and monitor reads: **3,504 requests**, **3,336 decision evictions**, natural quota polling continued; RSS KiB min **76,484**, max **83,356**, final **81,684**. Histories stay <=200, no worker failure or policy acceptance failure. The existing soak budget is <512MiB and <128MiB growth; observed growth was ~7MiB, not unbounded history. Cancellation may emit a synthetic TLS EOF, not a failed assertion.
- Six genuine c-shared 0.2.0 builds passed strict ELF/Mach-O/PE architecture, export, dependency/minimum and ZIP/checksum matrix using existing pinned v0.1.2 toolchains/TLS workaround. Linux amd64 exact packaged ZIP passed real local store/native CPA routing/polling/rollback **plus new monitor acceptance**, 75s observation; evidence JSON remains private until sanitized final artifact handoff.
- Same-host noisy timing: choose **5,044 ns/op, 712B, 7 allocs**; recorded pick **8,438 ns/op, 1,048B, 10 allocs**. Stable choose in nearby run was **7,829 ns/op, 712B, 7 allocs**. Shared-host load makes timing comparisons non-deterministic; no claimed speedup. Recorded diagnostics add bounded 336B/3 allocations in this fixture, with no policy allocation change. No durable/network request-side work.

## Real Chrome/OMC observations

chrome-devtools-mux used isolated synthetic CPA ports and isolated OMC v0.4.13, not production. Standalone navigation formerly hung because a MutationObserver watched its own theme write; fixed by not observing self and avoiding unchanged attribute writes. Browser then loaded immediately.

Passed: standalone public shell/login, bad-key denial, memory-only successful login, full DOM account/model/history clearing on Forget/401, relogin, no localStorage writes, OMC nested iframe inherited login/current 0.2.0 rendering and fallback bad-key clearing, no key in URL. Synthetic response interception exercised degraded/stale/exhausted, failed worker, empty/no-traffic, recovery, malicious `<img ...>` labels as literal text with zero image nodes/script execution. Pause produced zero extra reads over >5s; hidden-state handler produced zero reads over >5s and exactly one visible-resume read. Keyboard Tab/Enter operated Pause with visible focus. Narrow 390px light view had no body overflow (account table intentionally scrolls), native buttons >=40px; desktop dark theme rendered; screenshots kept privately under `.local/`. Console had no error/warn on final standalone actual-CPA page.

Response-intercepted UI states prove rendering only, not backend generation of those states. Go and real-CPA fixture tests cover backend bounds/auth/lifecycle/routing separately. Browser coverage is Linux Chrome only, not six-platform browser proof.

## Pending / gates

- Final clean-source six-package rebuild comparison and final evidence/preservation audit are required on the completed feature source.
- Matching native monitor acceptance on Darwin amd64/arm64, Linux arm64 and Windows amd64/arm64 requires separately approved push/hosted CI. They are not marked passed from cross-builds or old v0.1.2 results. Windows-arm64 race remains explicitly unsupported.
- Store PR #404 remains under maintainer review; default official-source install cannot be verified until merged/available. No duplicate PR or feature metadata replacement.
- Feature publication/public install/browser acceptance and protected production rollout require separate explicit scope. Existing v0.1.2 stays immutable and active.
