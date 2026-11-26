# Quota First Router

A Codex OAuth scheduler plugin for **CLIProxyAPI 8.1.0**. It preserves your account tiers and uses the account whose **five-hour quota resets soonest within the highest usable tier**.

## Example

| Priority | Account | Reset in | Remaining | Selection |
|---:|---|---:|---:|---|
| 20 | A | 20 minutes | 40% | First |
| 20 | B | 2 hours | 70% | After A is unavailable |
| 20 | C | 5 minutes | 0% | Skipped |
| 10 | D | 10 minutes | 80% | Only when tier 20 has no usable account |

Higher numeric priority means a higher tier. The plugin never changes priorities, credentials, or quota. Requests already running are not moved. Newly available higher-tier accounts take precedence on new selections. For immediate operator removal, disable an account: CPA may retain deleted files in its live roster until watcher reconciliation; the plugin removes its cache entry when the host roster drops it.

## Rules

- Only requests exclusively routed to **Codex** are handled. Claude and mixed-provider requests use CPA's router unchanged.
- CPA filters model eligibility, authentication, cooldowns, disabled accounts, attempted accounts, and explicit account pinning before offering candidates.
- Fresh exhausted five-hour, weekly/monthly, or applicable model-specific windows exclude an account. All accounts exhausted means CPA's normal no-capacity error, not an upstream attempt with a known-empty account.
- Within the highest usable tier: nearest future five-hour reset first; equal times use credential ID; known reset times precede unknown reset times with otherwise known available quota. Some Codex plans expose only a monthly window: when the quota response explicitly says `allowed: true`, those accounts remain usable but sort after accounts with a known five-hour reset in the same tier.
- If any candidate's quota is unknown/stale in the highest usable tier, delegate to CPA rather than accidentally skip that tier. Exclusions for other fresh exhausted accounts still apply. Ranking is not guaranteed during this fallback.
- After a reset timestamp passes, request a new observation; do not infer replenishment from a clock alone. Until refreshed, that snapshot is unknown (fail open to CPA).
- No artificial warmup or model request is sent to start a quota window. Only quota GETs are used by the polling worker.

## Build and checks

Linux amd64 with glibc 2.34+ is the currently runtime-verified platform. Install Go **1.26.10**, a C compiler, make, and Python **3.12+**. The pinned CPA SDK can trigger Go's normal toolchain download. Version **0.1.2** builds all six required platforms, and Darwin/Linux/Windows × amd64/arm64 passed matching native CPA 8.1.0 load/store/routing tests. Tested hosts and runtime limits are documented; oldest-OS runtime coverage is not implied. See [platform support and candidate verification](docs/platform-support.md) for pinned toolchains, macOS TLS isolation, commands and exact limits.

```sh
make GO=go test
make GO=go package cpa
make GO=go integration-test
make GO=go soak-test SOAK_SECONDS=120
```

Output: `dist/quota-first-router.so`. The integration test uses Docker and a pinned Go/Debian image, local TLS fixtures, synthetic credentials, and `--network none`. It mounts the supplied CPA binary read-only and does not touch the running CPA instance. Pull the image before offline execution:

```sh
docker pull golang:1.26-bookworm@sha256:a688600ca24f8a4d3ca77f95b0dd40704a9fc787c826660eb7ba0b641b8b175d
```

Set `GO=go` to use your installed Go. `make cpa` downloads CPA 8.1.0 and verifies both the published checksum and a pinned archive hash; `CPA_BINARY` can override its path. Builds resolve pinned public Go modules, not `references/`.

## Install and observe

**CPA 8.1.0: Linux, macOS and Windows, each amd64/arm64.** Version 0.1.2 provides all six native-tested packages. Linux requires glibc 2.34+; macOS binary deployment minimum is 13.0; Windows was tested on Server 2025 amd64 and Windows 11 ARM64. See [platform support](docs/platform-support.md) for precise tested/runtime limits. Older v0.1.0/v0.1.1 releases remain Linux-amd64 only.

v0.1.2 is published with all six native-tested packages. Platform issue **#1 is resolved**; the replacement registry-only store PR **#404 is awaiting maintainer review**. Earlier #402 stays closed. Official listing is not claimed before merge; details are in [release status](docs/release-status.md). To install from the author’s registry, add this custom registry source in CPA's configuration, keeping any existing sources:

```yaml
plugins:
  store-sources:
    - https://raw.githubusercontent.com/kazimshah39/quota-first-router/main/registry.json
```

Then refresh the plugin store, search **Quota First Router**, and install it. This source is the author's registry, not a claim of official-store approval. The official registry PR and its status are tracked in [release status](docs/release-status.md).

Alternatively download the ZIP matching your OS/architecture and checksums from [GitHub Releases](https://github.com/kazimshah39/quota-first-router/releases), verify SHA-256, and extract the library into CPA's plugin directory. The installer validates checksums automatically; manual extraction requires your own checksum check.

See [operations](docs/operations.md) before enabling the plugin. Start with:

```yaml
plugins:
  enabled: true
  dir: /path/to/plugins
  configs:
    quota-first-router:
      enabled: true
      priority: 100
      poll_interval: 1m
      max_age: 5m
      request_timeout: 10s
```

For a first installation, copy a regular platform library (`.so`, `.dylib`, `.dll`) into the plugin directory. Never overwrite a potentially mapped library during an upgrade; see [version upgrades](docs/operations.md#native-version-upgrades). CPA 8.1.0 supports loading/configuring it through its existing plugin management flow; verify registration, never assume copying alone succeeded. `priority` above orders scheduler **plugins**, not account tiers. CPA 8.1.0 invokes only its highest-priority active scheduler; another scheduler can supersede this one.

- **Enabled** (unpublished 0.2.0): chooses accounts and excludes fresh exhausted accounts.
- **Disabled**: CPA routes normally, and the plugin stops polling quota.

Before upgrading an older shadow installation, disable it and remove its legacy `mode` setting. Enable it only when you want it to route requests. Published v0.1.x builds still have the older mode setting.

### Configuration

| Key | Default | Allowed / meaning |
|---|---|---|
| `poll_interval` | `1m` | `1m`–`1h` |
| `max_age` | `5m` | At least poll interval, at most `1h` |
| `request_timeout` | `10s` | `1s`–`30s`, quota GET timeout |
| `proxy_url` | unset | Mirror CPA's `requests.proxy-url` when set; per-account proxy overrides it. `direct` explicitly disables environment proxies. |

The worker reconciles the roster and reset deadlines every 30 seconds and polls sequentially with bounded timeouts/backoff. Normal requests only read cached snapshots. 401/403 invalidates a snapshot; transient polling errors preserve the last success until `max_age`.

**Proxy limitation:** CPA's lifecycle configuration does not expose its global upstream proxy to this native plugin. Configure `proxy_url` explicitly when CPA uses `requests.proxy-url`. Per-account `proxy_url` is respected; otherwise Go's environment proxy policy applies. Custom account `base_url` is explicitly unsupported for quota polling and reported, never silently bypassed. Redirects are refused.

## Monitor candidate (unpublished 0.2.0)

The read-only **Quota First Router** plugin page shows health, cached quota, bounded scheduler choices and incidents. Public HTML is generic; all private diagnostics require management login. Choices are not actual served-account or request-success evidence. See [monitor guide](docs/monitor.md) for paths, auth, limits and lifecycle; [local evidence](docs/evidence/0.2.0/README.md) lists passed checks and pending native/release gates. The public v0.1.2 release is unchanged.

## Authenticated diagnostics

```text
GET   /v0/management/plugins/quota-first-router/status
PATCH /v0/management/plugins/quota-first-router/config
PATCH /v0/management/plugins/quota-first-router/enabled
```

Use existing CPA management authentication. The status response contains mode, snapshot age/reset information, worker health, last decision, each offered account's tier and skip reason, and counters. IDs can include email addresses, but OAuth tokens and raw provider error bodies are never returned. CPA 8.1.0's existing plugin endpoints use the `/v0/management` compatibility path; no CPA core changes are needed.

## Project and references

- [Reference repositories and attribution](docs/references.md)
- [Operation and rollback](docs/operations.md)
- [Verification evidence](docs/verification.md)
- [Comprehensive test matrix](docs/test-matrix.md)
- [Current release and deployment results](docs/release-status.md)
- [Six-platform candidate and native acceptance](docs/platform-support.md)

`references/` contains four independent, ignored Git clones for research (reset-router, CPA, 9router, and official OpenAI Codex). They are not submodules or build dependencies. MIT-licensed code was adapted from WebDevCaptain/quota-reset-router; required attribution is retained. Plugin metadata points to [this project](https://github.com/kazimshah39/quota-first-router); upstream code attribution remains in LICENSE and THIRD_PARTY_NOTICES.md.

Claude support is intentionally not implemented. Add a separate quota provider/normalizer later, then reuse the tier/reset policy with provider-specific tests.

Quota endpoints are provider implementation details and can change. Near-empty accounts can be exhausted between polling and inference; CPA handles the resulting failure and retries another eligible account. Cross-account switching may reduce prompt-cache reuse. Explicit pinning/host-retained connections remain under CPA control rather than being forcibly rerouted.
