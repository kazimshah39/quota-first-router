# References and attribution

The following independent repositories are cloned into `references/`, which is ignored by the parent Git project. They retain their own full Git histories. Do not edit them as part of this project, commit them, or make them required build inputs.

| Directory | Source | Recorded revision | License / use |
|---|---|---|---|
| `references/quota-reset-router` | https://github.com/WebDevCaptain/quota-reset-router | `62b73e7c66b40bcef7b3eb2879f99d727290a740` | MIT; adapted native ABI bridge, runtime lifecycle, polling skeleton, credential identity, configuration validation |
| `references/CLIProxyAPI` | https://github.com/router-for-me/CLIProxyAPI | `6960763455c4ac3d4828fb1736aa5de2183ccecf` (`v8.1.0`) | MIT; exact scheduler/ABI contract, model/pinning filters, host callbacks and tests; SDK dependency pinned to `v8.1.0` |
| `references/9router` | https://github.com/decolua/9router | `5ae63d22847e07042e235b40e1d926b65e509013` | MIT; inspected Codex quota shapes, additional model quota handling, proxy-aware fetching; no runtime dependency |
| `references/codex` | https://github.com/openai/codex | `3f65e9f13165d85d1b1fbe20ce06c55758f927b9` | Apache-2.0; official adaptive quota polling and usage endpoint reference; no code copied or runtime dependency |

Recreate for research:

```sh
mkdir -p references
git clone https://github.com/WebDevCaptain/quota-reset-router references/quota-reset-router
git clone https://github.com/router-for-me/CLIProxyAPI references/CLIProxyAPI
git -C references/CLIProxyAPI checkout v8.1.0
git clone https://github.com/decolua/9router references/9router
git clone https://github.com/openai/codex references/codex
git check-ignore references/quota-reset-router references/CLIProxyAPI references/9router references/codex
```

Before implementation we inspected the native ABI exports and ownership, registration requirements, lifecycle, quota endpoints, scheduler exclusion semantics, all-priority capability, and original tests. CPA ABI is **1**, schema **10** at the compatibility target. `SchedulerAcrossPriorities` lets this plugin examine all host-eligible tiers but does not change our priority-first policy. `ExcludeCandidates` allows exhaustion observed by the plugin to remove an otherwise host-ready account before built-in fallback.

CPA 8.1.0 requires a nonempty metadata repository URL. Plugin metadata identifies https://github.com/kazimshah39/quota-first-router. Upstream reset-router attribution and original MIT notices remain intact.

The root LICENSE retains Shreyash's MIT notice. `THIRD_PARTY_NOTICES.md` covers SDK, YAML, Go and build dependencies; all source reuse remains under its original notices. The new five-hour policy and tests are separate from upstream's weekly/monthly ranking.

## Official Codex quota refresh cadence

Inspected official Codex at the revision above. This describes its interactive TUI, not a promise about every Codex client/version:

- `codex-rs/tui/src/chatwidget/rate_limits.rs:217–245`: use the maximum finite used percentage across ordinary Codex and selected-model primary/secondary windows. Poll every 60 seconds below 75% used, 30 seconds at 75–<90%, 15 seconds at 90–<99%, and 5 seconds at >=99%.
- The same file at `should_prefetch_rate_limits` requires OpenAI authentication and a ChatGPT account; it is not unconditional API-key polling.
- `codex-rs/tui/src/app/rate_limit_refresh.rs`: no overlapping periodic/recovery read; completion (including failure/timeout) starts the next wait.
- `codex-rs/tui/src/app/background_requests.rs`: background `account/rateLimits/read` excludes the separate reset-credit detail lookup. Startup, /status, usage-menu and recovery/reset actions can also trigger reads outside the periodic cadence.
- `codex-rs/backend-client/src/client/rate_limit_resets.rs`: usage URL is `/wham/usage` for ChatGPT-style routing (or `/api/codex/usage` for Codex API-style routing).

Our plugin deliberately uses a fixed 1-minute default rather than adaptive polling. It manages multiple accounts, so frequent reads would multiply across the pool. The 30-second worker sweep checks deadlines; a future 5/15-second adaptive policy would need a corresponding sweep change.

Secondary documentation was checked with Context7 `/router-for-me/cliproxyapi`; exact-version source and actual installed-binary tests take precedence over latest-version examples.
