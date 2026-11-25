# Release status

## v0.1.1 — published and deployed

- Release: https://github.com/kazimshah39/quota-first-router/releases/tag/v0.1.1
- Source: `754f859`; follow-up CI archive-name fix: `278333e`.
- Release verification/publish run: https://github.com/kazimshah39/quota-first-router/actions/runs/45562317076 — passed.
- Main CI: https://github.com/kazimshah39/quota-first-router/actions/runs/45562878396 — passed. The first main run exposed a hard-coded v0.1.0 reproducibility filename; this was fixed to use the source version.
- Public ZIP SHA256: `bb38c37da67cf91007ed0e58b487a673fd3f7f8d88a8a1803c61d62b37781e61`.
- Store-downloaded and live-loaded library SHA256: `e5d9444ba01b037ee8ae6472b6655b270f27e58060fad7262989a2e2cf51dece`.

The real CPA store installer discovered the public custom registry, downloaded and checksum-verified v0.1.1, confirmed shadow/1m/5m defaults, exercised active/disable, and reinstalled after restarting only its disposable host. The store-downloaded library then passed the expanded actual-CPA protocol/failure/tool/cancellation integration suite.

Production switched from public v0.1.0 to public v0.1.1 using a new versioned regular file and CPA's desired-version configuration; neither a mapped library overwrite nor a shared-service restart was needed. Protected backups are retained locally. Final v0.1.1 live matrix passed **26/26** bounded requests across four available models, both protocols/streaming modes, parallel requests and a forced tool call. A separate six-request gate independently ranked the offered top-tier reset order, confirmed actual account attribution through read-only usage records, and verified a complete function-call/output follow-up. Nine enabled accounts had fresh error-free snapshots; account tiers/disabled flags and unrelated settings were preserved.

The final-code ten-minute mixed-protocol synthetic soak completed **7,616 requests** with background polling and CPA RSS **75,332–80,632 KiB** (final **79,340 KiB**), under `GOGC=20`/`GOMEMLIMIT=256MiB`. The remaining exhaustion/failover/rollback assertions passed afterward. The synthetic all-exhausted case intentionally logs a 503; it is an expected negative test, not a soak failure. An earlier 7,800-request soak passed its load phase but failed an immediate 429-recovery assertion; the harness now waits for the host cooldown and does not claim recovery from 401 by verification alone.

Browser testing verified discovery, mode saves, disable/reenable and rollback inference, and showed v0.1.1 running after refresh. See [test matrix](test-matrix.md) for coverage, parser regressions and explicit limits. Final natural-refresh observation is recorded below once complete.

## v0.1.0 — published

- Repository: https://github.com/kazimshah39/quota-first-router
- Stable release: https://github.com/kazimshah39/quota-first-router/releases/tag/v0.1.0
- Supported platform: **Linux amd64/glibc 2.34+, CPA 8.1.0, Codex OAuth**.
- Default: **shadow mode**, **1m quota polling**, **5m maximum snapshot age**.
- CI passed: https://github.com/kazimshah39/quota-first-router/actions/runs/45556190680
- Release checks passed: https://github.com/kazimshah39/quota-first-router/actions/runs/45556640103

Assets:

- [Linux amd64 ZIP](https://github.com/kazimshah39/quota-first-router/releases/download/v0.1.0/quota-first-router_0.1.0_linux_amd64.zip)
- [checksums.txt](https://github.com/kazimshah39/quota-first-router/releases/download/v0.1.0/checksums.txt)
- ZIP SHA256: `ca672536ef50ef554724b0e70e07cb4fe7788a31a1176c3264072c797a6be6e8`

## Installer verification

The published release was discovered through the author's public custom registry and installed through an isolated CPA 8.1.0's real plugin-store API. CPA downloaded and checksum-verified the public ZIP. Checks confirmed version 0.1.0, default shadow/1m/5m configuration, active toggle, disable, and same-version reinstall after restarting only the disposable test host.

The library downloaded by that installer passed the full synthetic routing suite again. This is not a claim of a two-version upgrade test: v0.1.0 is the first version. The CI/release checks also passed a two-minute soak under GC pressure, concurrent/streaming tests and clean-source reproducibility.

## Official plugin-store listing — PR submitted, review pending

**PR:** https://github.com/router-for-me/CLIProxyAPI-Plugins-Store/pull/402

At submission the PR is open and changes only registry.json by appending our metadata. The upstream maintainers control review and merge. Until it is merged and clients refresh the registry, this plugin is **not claimed to be present in the default official store**.

Install now by adding this custom registry source, retaining existing sources:

```yaml
plugins:
  store-sources:
    - https://raw.githubusercontent.com/kazimshah39/quota-first-router/main/registry.json
```

Refresh the store, search **Quota First Router**, install, and follow the README to enable shadow mode first. Public publication does not automatically activate the plugin on any existing server.
