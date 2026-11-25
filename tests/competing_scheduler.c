/* Synthetic scheduler to verify CPA 8.1.0's highest-plugin-priority rule. */
#include "../bridge.h"
#include <string.h>

static int call(char *method, uint8_t *request, size_t size, cliproxy_buffer *out) {
    (void)request; (void)size;
    const char *json = "{\"ok\":true,\"result\":{}}";
    if (!strcmp(method, "plugin.register") || !strcmp(method, "plugin.reconfigure")) {
        json = "{\"ok\":true,\"result\":{\"schema_version\":10,\"metadata\":{\"Name\":\"Synthetic competing scheduler\",\"Version\":\"0.0.0\",\"Author\":\"test\",\"GitHubRepository\":\"https://github.com/router-for-me/CLIProxyAPI\"},\"capabilities\":{\"scheduler\":true}}}";
    } else if (!strcmp(method, "scheduler.pick")) {
        json = "{\"ok\":true,\"result\":{\"Handled\":true,\"DelegateBuiltin\":\"fill-first\"}}";
    }
    out->len = strlen(json);
    out->ptr = malloc(out->len);
    memcpy(out->ptr, json, out->len);
    return 0;
}
static void release(void *ptr, size_t len) { (void)len; free(ptr); }
static void shutdown_plugin(void) {}
int cliproxy_plugin_init(cliproxy_host_api *host, cliproxy_plugin_api *api) {
    if (!host || host->abi_version != 1) return 1;
    api->abi_version = 1; api->call = call; api->free_buffer = release; api->shutdown = shutdown_plugin;
    return 0;
}
