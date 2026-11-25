# Release status

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
