# Codex reset warmup: feasibility results

The native model callbacks below were rejected for this feature. The implemented worker instead uses CPA's existing credential-pinned verification endpoint on numeric localhost, with a cancelable HTTP request. This is the same endpoint Oh My CPA uses for warmup. Warmup is off by default and does not change scheduler ordering.

## Native host test

`tests/host_integration.py --warmup-probe` compiles the synthetic `tests/warmup_host_probe.c` adapter, loads it in a disposable CPA container without a network, and simulates a Codex server on loopback. It confirms model discovery, exact-account pinning, a completed response, and no fallback when the pinned account is missing or disabled or its model is unavailable. It holds a subsequent upstream response before headers, then asks CPA to disable or reconfigure the test plugin. The test only releases the synthetic response after checking whether the background call ended on its own.

| CPA | RPC | Change | Stalled model call ended before the fixture released it? |
| --- | --- | --- | --- |
| 8.1.3 (`8cd6817b`) | `host.model.execute` | Disable | No |
| 8.1.3 | `host.model.execute_stream` | Disable | Yes |
| 8.1.3 | `host.model.execute_stream` | Reconfigure | No |
| 8.1.0 (`69607634`) | `host.model.execute_stream` | Reconfigure | No |

Run the reconfigure check with a disposable copy of the target CPA binary (currently expected to fail the cancellation assertion):

```sh
make build
python3 tests/host_integration.py --cpa /absolute/path/to/cli-proxy-api \
  --library dist/quota-first-router.so \
  --warmup-probe execute_stream --probe-lifecycle reconfigure
```

The native ABI starts a background call with no host-issued request context. The [stream call](https://github.com/router-for-me/CLIProxyAPI/blob/8cd6817b/internal/pluginhost/host_model_stream_callbacks.go#L44-L73) does not return a stream ID until model execution starts; `stream_close` therefore cannot cancel a request stalled before that point. CPA closes streams when it unloads the plugin, which explains why streaming passed the disable case but not reconfigure. A Go timeout without a way to cancel and join the native call would only stop waiting on our side. The HTTP operation cancellation API does not cancel the model API.

## Verified alternative

`--warmup-probe verify` runs the exact CPA verification endpoint against the fake Codex upstream. On disposable CPA 8.1.3, the endpoint confirmed the target credential's completed response. In the stalled case, closing the HTTP request disconnected the provider connection before headers, without switching accounts. The plugin gives that call a deadline and cancels it on disable/reconfigure. It neither issues direct OAuth inference nor stores the management key in visible configuration; [operator provisioning](operations.md#optional-codex-warmup) keeps it in a protected file.

`--warmup-probe feature` exercises an active → reset → idle → warmup → confirmed five-hour transition. Explicit no-credit provider evidence is an eligibility guard, not a guarantee about future billing or an unknown provider change. Missing, contradictory, or positive credit evidence never authorizes a send.
