# Platform support — published 0.1.2

**For 0.2.0:** see [release status](release-status.md) for exact-source native acceptance. The same routing/monitor targets apply; optional warmup is Linux/macOS only and is not part of the six-platform routing acceptance.

All six required targets passed genuine matching-architecture CPA 8.1.0 native acceptance in [CI run 45785056147](https://github.com/kazimshah39/quota-first-router/actions/runs/45785056147), source `ebc5d89`. Release publication/public-download/production results are tracked separately in [release status](release-status.md). Existing v0.1.0/v0.1.1 assets remain immutable Linux-amd64-only releases.

## Source and routing scope

Platform work started at stable `d1e1571829267bd93b484e05698a018cb2b022be` in an isolated worktree. Router implementation remains unchanged except version 0.1.2. Codex OAuth only, tier-first then nearest five-hour reset, safe stale/unknown delegation, shadow default, 1m polling / 5m maximum age / 10s timeout and 30s reconciliation remain intact. Monitoring stays paused in the original worktree, backed up and checksummed; its source/UI/routes are not included.

## Native evidence matrix

Every row passed correct machine/format/export/dependency checks, exact candidate ZIP installation through CPA's real local store API, authenticated diagnostics/defaults, shadow non-interference, active tier/reset selection, both protocols and streaming, 24 concurrent requests, no request-path polling, natural polling, tier descent/recovery, disable/reenable rollback and unchanged synthetic priority/disabled flags. Invalid-header and actual opposite-architecture libraries were rejected without registering or crashing CPA. Each lane ran native unit/vet and supported race checks; windows/arm64 race is explicitly unsupported, not passed.

| Target | Compiler / binary baseline | Genuine native host | 75s repeated requests | Quota GETs | Result |
|---|---|---|---:|---:|---|
| linux/amd64 | Zig 0.15.2, x86-64 v1, ELF64, glibc 2.34 | ubuntu-24.04 / x86_64 | 714 | 18 | Pass |
| linux/arm64 | Zig 0.15.2, ELF64 AArch64, glibc 2.34 | ubuntu-24.04-arm / aarch64 | 707 | 18 | Pass |
| darwin/amd64 | Zig 0.15.2 / SDK 26.1, Mach-O x86-64, deployment 13.0 | macos-15-intel / x86_64 | 443 | 18 | Pass |
| darwin/arm64 | Zig 0.15.2 / SDK 26.1, Mach-O arm64, deployment 13.0 | macos-15 / arm64 | 486 | 18 | Pass |
| windows/amd64 | LLVM-MinGW 20261117 UCRT, PE32+ x86-64 | windows-2025 / AMD64 | 684 | 18 | Pass |
| windows/arm64 | LLVM-MinGW 20261117 UCRT, PE32+ ARM64 | windows-11-arm / ARM64 | 651 | 18 | Pass |

Full host OS, Go version, CPA/library/archive hashes and results are in `docs/evidence/0.1.2/native-*.json`. Binary/package checksums and dependencies are in the same directory. CPU emulation is not used as native acceptance. The above request counts identify this recorded run, not an invented guarantee for every future run.

## Runtime boundaries

- Linux candidates depend on libc.so.6; package checks reject newer-than-2.34 GLIBC references. Matching Ubuntu native runners and the existing Debian Docker suite passed; all historical distribution/kernel versions are not claimed tested.
- Darwin candidates require macOS 13 according to binary deployment metadata and use only system CoreFoundation/Security, libSystem, libobjc and libresolv. Native tests ran macOS 15; macOS 13 itself has not been runtime-tested. Stock Go 1.26's macOS 12 floor does not lower this binary requirement.
- Windows candidates import KERNEL32 and UCRT API DLLs, not compiler DLLs. Windows Server 2025 amd64 and Windows 11 ARM64 passed. Go's general floor is Windows 10 / Server 2016, but those oldest systems were not tested. PE subsystem 6.1 is not an OS support promise.
- Darwin amd64 uses isolated Go 1.26.10 source with goroutine TLS moved from slot 6 to slot 11 to avoid collision with CPA's Go runtime. `darwin_toolchain.py` never patches installed Go; the coordinator disassembles the candidate and requires slot-11 accesses and no slot-6 accesses (984 / 0 observed). Native CPA loading/routing passed. Compatibility with other plugins independently using the same reserved slot is not promised.

## Reproduce

Coordinator prerequisites: Linux amd64, Go **1.26.10**, make, Python **3.12+**, network access for checksum-pinned tools. Tools stay in ignored `.local/toolchains`. Supply `MACOS_SDK_PATH` for an appropriate SDK or explicitly opt in to SDK download. Tool URLs/digests are pinned in `build_matrix.py` and `darwin_toolchain.py`.

```sh
make GO=go test package cpa
python3 scripts/build_matrix.py --go go --allow-sdk-download
python3 scripts/release.py matrix --output dist/release
# Requires six matching native records, not cross-builds alone:
python3 scripts/release.py matrix --output dist/release --require-native
```

`make package TARGET_OS=... TARGET_ARCH=... LIBRARY=...` supports a configured native/cross CC. Never use stock Darwin-amd64 Go for this CPA plugin. ZIP root contains only the platform `.so`/`.dylib`/`.dll`, LICENSE and THIRD_PARTY_NOTICES.md. Header/source/nested/traversal/duplicate/wrong-target assets are rejected. The sorted manifest must cover exactly six version-matched archives. Offline tests also reject missing exports, truncated binaries, excess runtime baseline, missing/duplicate/unsorted/bad checksums, missing targets and mismatched native evidence.

On a **disposable matching native host**, configure its compiler (`native_setup.py` in CI), download pinned CPA and extract the exact candidate (`unpack_candidate.py`), then run, substituting platform/extensions:

```sh
python3 scripts/release.py cpa --goos linux --goarch amd64
python3 scripts/unpack_candidate.py --goos linux --goarch amd64
python3 tests/native_host.py --disposable-native-runner \
  --cpa dist/cpa/cli-proxy-api --library dist/native/quota-first-router.so \
  --archive dist/release/quota-first-router_0.1.2_linux_amd64.zip \
  --output dist/release/native-linux-amd64.json --seconds 75
```

Fixtures use synthetic credentials and local TLS/proxy/inference/quota endpoints. Only the explicit loopback store fixture permits insecure HTTP. macOS/Windows disposable runners use noninteractive system trust; cleanup removes the exact certificate, verifies macOS certificate absence, and the VM is destroyed. Linux uses SSL_CERT_FILE. Never run this trust fixture on a shared personal desktop. macOS trust-setting removal has a known hosted-image hang (runner-images #12116); deleting the exact certificate without `-t` passes cleanup. No personal certificate store is modified.

CPA may atomically rewrite auth files during native tests. A changed file deliberately invalidates its snapshot during reconciliation. The fixture verifies only documented top-tier unknown-quota delegation followed by bounded recovery, never weakens tier precedence or accepts arbitrary wrong choices. An earlier ARM64 run exposed this safe fallback; the final recorded run had zero fallbacks on all six targets.

## CI and regression gates

Regular CI is read-only/nonpublishing: unit/race/vet, original Linux resource soak, pinned six cross-builds, byte comparison against a clean Git export, six native lanes and hash-bound complete artifact aggregation. Clean builds require no ignored references/private monitoring work. The Linux candidate also passed original failure/retry, competing scheduler, auth/pinning/filtering, tool payload and cancellation checks, plus a 120s/1,520-request local soak with RSS 75,692–79,332 KiB (final 78,532).

Publication is separate/manual: explicit approved input, environment gate, existing version tag, successful exact-source CI SHA and all six native records. Existing releases are never replaced. Public release, store changes, and production rollout are separate operations.

## References

- [Go 1.26 ports](https://go.dev/doc/go1.26#ports) and [minimum requirements](https://go.dev/wiki/MinimumRequirements): toolchain baseline, not evidence of oldest-OS plugin testing.
- [GitHub hosted runner architectures](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).
- [Closed PR #402](https://github.com/router-for-me/CLIProxyAPI-Plugins-Store/pull/402) and [platform issue #1](https://github.com/kazimshah39/quota-first-router/issues/1); current state belongs in release status.
