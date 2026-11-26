# Local candidate verification record

Candidate: 0.1.2, stable base `d1e1571829267bd93b484e05698a018cb2b022be`, platform-readiness branch. These are local checks, not a hosted CI run or public release. Source commit is available in local Git history; binary/package builds disable VCS embedding for deterministic clean-export comparison.

- `checksums.txt`: final six genuine ZIP hashes, also listed in release notes.
- `binary-matrix.json`: architecture/ABI/dependencies/minimum metadata for all six candidate packages.
- `native-linux-amd64.json`: exact final ZIP and library SHA, CPA 8.1.0 executable SHA, native OS/machine and Go version; real local store installation, routing, polling, rollback and invalid-library rejection. No personal credentials or prompts are present.
- Five other native records are deliberately absent. `matrix --require-native` must fail until all six exist; fake fixture records appear only inside temporary unit-test directories.

## Commands and results

- `python3 scripts/build_matrix.py --allow-sdk-download`: pass all six targets with pinned toolchains, deployment/dependency/format/export checks and deterministic flat packaging.
- `make test`: pass Go race/unit/vet/gofmt and four offline Python test groups (format/machine/export, runtime baseline, packaging/matrix/native-evidence rejection, registry).
- `tests/native_host.py ... --seconds 75`: pass Linux amd64 using the exact packaged candidate and its local store installer. Request/poll counts and native hashes are in JSON. Invalid-header library is discovered and rejected while CPA remains functional; wrong architecture is rejected by offline format tests, not claimed as a separately executed native wrong-architecture load test.
- `SOAK_SECONDS=120 python3 tests/host_integration.py --cpa dist/cpa/cli-proxy-api --library dist/native/quota-first-router.so`: pass against the exact packaged library, 1,520 mixed protocol/stream requests; background polling continued; RSS KiB min 75,692 / max 79,332 / final 78,532. Original authenticated/shadow/active/tier/stale/exhaustion/recovery/failure/tool/cancellation/pinning/competing-scheduler/rollback suite also passed. Local TLS fixture logged a client EOF during cancellation; assertions and exit status passed. CPA-owned 401 retained state is not claimed to recover automatically.
- `actionlint -shellcheck= .github/workflows/ci.yml .github/workflows/release.yml` with actionlint 1.7.12: pass. Six proposed native lanes and read-only nonpublishing artifact gate are defined locally; hosted execution is pending approval. Shellcheck was not installed and is not claimed as run.
- Clean Git export/rebuild comparison: see final local result recorded below after execution.
- Preservation: all 11 original monitor-file SHA256 values and four production config/library hashes matched the pre-work manifests. Stable routing/ABI/bridge/module files match the base byte-for-byte; no monitor sources/UI/routes in this candidate.

Private raw logs, original-file manifests, recoverable monitoring archive, tool downloads and draft issue/PR responses are outside tracked evidence. No push/dispatch, tag/release/upload, public comment/new PR/registry edit or production change was performed.
