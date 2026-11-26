# Platform support — local 0.1.2 candidate

Status: **not release-ready; five native acceptances are pending**. Public v0.1.0 and v0.1.1 remain immutable Linux-amd64-only releases. No tag, release, upload, registry change, new PR, production change or remote workflow execution was performed for this candidate.

## Source and routing scope

The `platform-readiness` worktree starts at stable `d1e1571829267bd93b484e05698a018cb2b022be`. Router implementation is unchanged except version 0.1.2; bridge/ABI/policy/quota behavior is retained. Codex OAuth only, tier-first then nearest five-hour reset, safe stale/unknown delegation, shadow default, 1m polling / 5m maximum age / 10s timeout and 30s reconciliation remain intact.

Monitoring source/UI remains paused in the original worktree and is not in this candidate. A private checksummed backup and original-file manifest are retained under its `.local/platform-baseline/`. Neither checkout's build depends on ignored `references/` clones.

## Evidence matrix

All rows below have genuine c-shared binaries, required `cliproxy_plugin_init`, correct architecture/format, strict ZIP layout and matching sorted SHA-256 manifest. **Built is not native-tested.** CPA hosts are pinned to 8.1.0; release.py pins all six upstream archive hashes.

| Target | Compiler / binary baseline | Native CPA / local store install | Proposed native runner; execution status |
|---|---|---|---|
| linux/amd64 | Zig 0.15.2, x86-64 v1, ELF64, glibc 2.34 | **Pass**, exact packaged library, CPA `69607634` | Local x86-64 Linux; pass |
| linux/arm64 | Zig 0.15.2, ELF64 AArch64, glibc 2.34 | **Pending** | `ubuntu-24.04-arm`; not dispatched |
| darwin/amd64 | Zig 0.15.2 / SDK 26.1, Mach-O x86-64, deployment metadata 13.0 | **Pending** | `macos-15-intel`; not dispatched |
| darwin/arm64 | Zig 0.15.2 / SDK 26.1, Mach-O arm64, deployment metadata 13.0 | **Pending** | `macos-15`; not dispatched |
| windows/amd64 | LLVM-MinGW 20261117 UCRT, PE32+ x86-64 | **Pending** | `windows-2025`; not dispatched |
| windows/arm64 | LLVM-MinGW 20261117 UCRT, PE32+ ARM64 | **Pending** | `windows-11-arm`; not dispatched; Go race detector unsupported |

Linux depends on `libc.so.6`; no newer-than-2.34 GLIBC reference is accepted. Darwin dependencies are system CoreFoundation/Security, libSystem, libobjc and libresolv; current binary deployment metadata requires macOS 13, even though stock Go 1.26 itself can run on macOS 12. Windows imports KERNEL32 and UCRT API DLLs, not compiler DLLs. PE subsystem 6.1 is **not** an OS support promise. The Go runtime Windows floor is Windows 10 / Server 2016, but the candidate's UCRT private import and ARM64 behavior still need native tests; no Windows minimum is declared runtime-verified. Minimum-version machines have not been tested on nonlocal targets.

Darwin amd64 needs special care: stock Go uses TLS slot 6, also used by CPA's Go runtime. `scripts/darwin_toolchain.py` builds checksum-verified Go 1.26.10 source in an isolated tool directory, changes the linker/runtime slot to 11, and never patches installed Go. The coordinator disassembles the candidate and requires slot-11 accesses and no slot-6 accesses (observed 984 / 0). This follows the attributed quota-reset-router workaround. Native CPA loading is still mandatory; the workaround does not by itself establish compatibility with other native Go plugins using the same slot.

## Reproduce locally

Prerequisites: Linux amd64 coordinator, Go **1.26.10**, make, Python **3.12+**, network access for checksum-pinned tool downloads. Tools stay in ignored `.local/toolchains`. Supply `MACOS_SDK_PATH` for an appropriate SDK or explicitly opt in to the documented SDK download. Tool versions/source URLs/digests are pinned in `build_matrix.py` and `darwin_toolchain.py`.

```sh
make GO=go test package cpa                 # established default Linux build
python3 scripts/build_matrix.py --go go --allow-sdk-download
python3 scripts/release.py matrix --output dist/release
# Expected to FAIL until all six matching native records exist:
python3 scripts/release.py matrix --output dist/release --require-native
```

`make package TARGET_OS=... TARGET_ARCH=... LIBRARY=...` also supports a properly configured native/cross CC. Do not use a stock Darwin-amd64 Go build for CPA. Archives are flat: `quota-first-router.so`, `.dylib` or `.dll`, plus LICENSE and THIRD_PARTY_NOTICES.md; headers/source/nested paths are rejected. Manifest entries are sorted and must cover exactly the six version-matched archives. Matrix tests reject wrong machines, missing exports, truncated binaries, new runtime baselines, missing/duplicate/unsorted/bad checksums, missing targets, traversal, duplicate ZIP members and missing native evidence.

On a **disposable matching native host**, prepare the native compiler (`native_setup.py` in CI), download pinned CPA and extract the actual candidate using `unpack_candidate.py`, then run (substitute OS/arch/extensions):

```sh
python3 scripts/release.py cpa --goos linux --goarch amd64
python3 scripts/unpack_candidate.py --goos linux --goarch amd64
python3 tests/native_host.py --disposable-native-runner \
  --cpa dist/cpa/cli-proxy-api --library dist/native/quota-first-router.so \
  --archive dist/release/quota-first-router_0.1.2_linux_amd64.zip \
  --output dist/release/native-linux-amd64.json --seconds 75
```

The portable fixture uses synthetic local TLS/proxy/inference/quota endpoints and the actual CPA store direct-artifact installer against the exact candidate ZIP. Per-account proxy keeps inference local; only this loopback fixture has an explicit insecure-store exception. Native macOS/Windows temporary CA trust is removed in cleanup; Linux uses SSL_CERT_FILE. Never run this trust fixture on a shared personal desktop. The harness checks native architecture/header, pinned CPA version, invalid library survival, registration, auth rejection, defaults, shadow non-interference, active tier/reset choice, both protocols/stream modes, 24 concurrent requests, no request-path polling, natural background polling, tier exhaustion/recovery, disable/reenable and unchanged synthetic priority/disabled flags. Evidence records archive/library/CPA hashes, host architecture/OS, Go version and measured request/poll counts. Supported native unit/race/vet checks are in CI; windows/arm64 race is explicitly skipped, not passed.

The original Linux Docker suite and RSS-measured soak remain in place, including failure/retry, competing scheduler, pinned/auth filtering, tool payloads and cancellation. See [verification](verification.md) and [test matrix](test-matrix.md). Candidate evidence is retained in `docs/evidence/0.1.2/` and the review record below.

## Access blockers and retry

The only accessible native machine is Linux amd64. Read-only repository runner inventory returned `total_count: 0`; the tailnet MacBook was offline, and a later SSH retry timed out again. There are no accessible native Linux ARM64, Intel Mac or Windows hosts. Emulators/cross-builds are not substituted. GitHub documents all six proposed native runner labels, but their actual execution requires a source/CI push and possibly manual dispatch, explicitly outside current local-only authorization. No billing, availability or green-run claim is made from a documented label.

**Smallest next approval:** permit pushing the reviewed `platform-readiness` source/CI branch and running the nonpublishing CI/native matrix. This does not authorize tags/releases/uploads, issue/PR comments, registry changes, a new store PR or production rollout. Alternatively provide SSH/native-host access to disposable matching machines for all five pending targets. Any runner-specific failures must be fixed and rerun before release readiness is claimed.

## CI and publication boundaries

Local `actionlint` and gate tests pass; hosted CI is **pending**, not green. Regular CI builds six candidates, rebuilds a clean Git export and compares bytes, runs six native lanes, then requires six exact hash-bound native records before aggregating `verified-six-platform-release`. CI has read-only contents permission and no release step. The manual release workflow requires explicit input, `release-approval` environment, existing version-matching tag, successful CI from the exact source SHA and complete native evidence. Configure required environment reviewers separately before any authorized publication. Existing releases must never be replaced.

## External references checked during this work

- [Go 1.26 ports](https://go.dev/doc/go1.26#ports): macOS toolchain floor and Windows ARM changes; not plugin-load acceptance.
- [Go minimum requirements](https://go.dev/wiki/MinimumRequirements): Go OS baseline, not a verified candidate runtime floor.
- [GitHub hosted runners](https://docs.github.com/en/actions/reference/runners/github-hosted-runners): labels/architectures, not an executed matrix.
- [Closed official-store PR #402](https://github.com/router-for-me/CLIProxyAPI-Plugins-Store/pull/402), [open issue #1](https://github.com/kazimshah39/quota-first-router/issues/1): read-only state rechecked 2026-11-26.
