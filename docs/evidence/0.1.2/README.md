# 0.1.2 verification evidence

All six targets passed native CPA/store/routing acceptance in [CI 45785056147](https://github.com/kazimshah39/quota-first-router/actions/runs/45785056147), source `ebc5d89`. This directory records that exact artifact set; later documentation-only commits do not change router code, but publication still requires a successful exact-tag-source CI run.

- `checksums.txt`: six ZIP SHA-256 values.
- `binary-matrix.json`: architecture/ABI/dependency/minimum metadata and ZIP hashes.
- `native-<os>-<arch>.json`: genuine native host OS/machine/Go version, CPA/library/archive SHA-256, 75s repeated request counts, polling, local store install, invalid-header/opposite-architecture rejection and passed routing/rollback acceptance.

All six were native, not emulated. Request counts: Darwin amd64 443, Darwin arm64 486, Linux amd64 714, Linux arm64 707, Windows amd64 684, Windows arm64 651. Each observed 18 quota GETs over its fixture lifecycle and zero unknown-quota fallback events in this recorded run. Native unit/vet and supported race checks passed; windows/arm64 race is unsupported and explicitly skipped.

## Other verified checks

- Six genuine pinned-toolchain c-shared builds; strict ELF/Mach-O/PE machine/export/dependency/runtime-baseline checks; flat license/library ZIPs and complete sorted checksums.
- Clean `git archive HEAD` rebuild produced byte-identical six packages in hosted CI. Ignored reference clones, private files and paused monitoring are not dependencies. Cross-host byte identity is not promised: published artifacts are the actual CI artifact set, not different local builds.
- Go unit/race/vet/gofmt and four offline Python test groups passed, including missing/wrong/duplicate/traversal/checksum/native-evidence rejection.
- Original exact-package Linux Docker acceptance and 120s local resource soak passed: 1,520 mixed protocol/stream requests; background polling continued; RSS KiB min 75,692 / max 79,332 / final 78,532. Suite includes failure/retry statuses, competing scheduler, auth/pinning/model filtering, tools, cancellation, tier/recovery/stale behavior and rollback. A cancellation-induced TLS EOF was logged; assertions/exit passed. CPA-owned retained 401 state is not claimed to recover automatically.
- actionlint 1.7.12 passed locally without shellcheck; actual hosted six-native CI also passed. Normal CI has no release-creation step. Manual publication requires exact source/tag/run, six native records and explicit approved input/environment.
- Earlier nonpassing runs exposed portable certificate/trust cleanup problems and one documented safe top-tier unknown-quota fallback while CPA atomically rewrote credentials. Those were fixed in fixtures without changing the router implementation. macOS cleanup removes the exact system certificate and verifies absence; disposable runner teardown removes remaining VM state. See platform-support.md for limits.
- Original 11 paused monitor-file hashes, recoverable archive and production config/library hashes matched before rollout. Router/ABI/bridge/module source stayed byte-for-byte at stable base except version. Production outcomes after authorized rollout are recorded separately in release-status.md.

Private raw logs, protected configuration/account evidence, download caches and original-file manifests are not published. No OAuth material, personal account details, management secrets or real request content is in these synthetic records.
