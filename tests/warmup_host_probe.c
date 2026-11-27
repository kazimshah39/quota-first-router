/* Disposable integration probe. Never packaged with the plugin. */
#include "../bridge.h"
#include <pthread.h>
#include <stdio.h>
#include <string.h>

static cliproxy_host_api host_api;
static pthread_t worker;
static int running;

static void drain_stream(const char *opened);

static void rpc(const char *method, const char *input, const char *result) {
    cliproxy_buffer out = {0};
    bridge_host_call(&host_api, (char *)method, (uint8_t *)input, strlen(input), &out);
    FILE *file = fopen(result, "wb");
    if (file != NULL) {
        if (out.ptr != NULL && out.len > 0) fwrite(out.ptr, 1, out.len, file);
        fclose(file);
    }
    if (!strcmp(method, "host.model.execute_stream") && out.ptr != NULL && out.len < 65536) {
        char *opened = calloc(out.len + 1, 1);
        if (opened != NULL) {
            memcpy(opened, out.ptr, out.len);
            drain_stream(opened);
            free(opened);
        }
    }
    if (out.ptr != NULL) bridge_host_free(&host_api, &out);
}

static void drain_stream(const char *opened) {
    const char *key = "\"stream_id\":\"";
    const char *id = strstr(opened, key);
    if (!id) return;
    id += strlen(key);
    const char *end = strchr(id, '"');
    if (!end || end - id >= 128) return;
    char request[160];
    snprintf(request, sizeof(request), "{\"stream_id\":\"%.*s\"}", (int)(end - id), id);
    for (int i = 0; i < 32; i++) {
        cliproxy_buffer out = {0};
        bridge_host_call(&host_api, "host.model.stream_read", (uint8_t *)request, strlen(request), &out);
        char *msg = out.ptr != NULL && out.len < 65536 ? calloc(out.len + 1, 1) : NULL;
        if (msg) memcpy(msg, out.ptr, out.len);
        if (out.ptr != NULL) bridge_host_free(&host_api, &out);
        // Store the synthetic chunk for the Python driver to decode and check.
        if (!msg) break;
        FILE *file = fopen("probe-chunks.jsonl", "ab");
        if (file != NULL) { fprintf(file, "%s\n", msg); fclose(file); }
        int done = strstr(msg, "\"done\":true") != NULL || strstr(msg, "\"ok\":false") != NULL;
        free(msg);
        if (done) break;
    }
    rpc("host.model.stream_close", request, "probe-close.json");
}

static void *background(void *unused) {
    (void)unused;
    const char *method = getenv("QFR_TEST_MODEL_RPC");
    char input[8192];
    FILE *file = fopen("probe-request.json", "rb");
    if (file != NULL) {
        size_t n = fread(input, 1, sizeof(input) - 1, file);
        fclose(file);
        input[n] = '\0';
        rpc("host.auth.models", "{}", "probe-models.json");
        if (method != NULL) rpc(method, input, "probe-result.json");
    }
    return NULL;
}

static void join_worker(void) {
    if (running) {
        pthread_join(worker, NULL);
        running = 0;
    }
}

static int call(char *method, uint8_t *request, size_t size, cliproxy_buffer *out) {
    (void)request;
    (void)size;
    const char *json = "{\"ok\":true,\"result\":{}}";
    if (!strcmp(method, "plugin.register") || !strcmp(method, "plugin.reconfigure")) {
        if (!strcmp(method, "plugin.reconfigure")) {
            FILE *file = fopen("probe-reconfigure.entered", "wb");
            if (file != NULL) fclose(file);
        }
        join_worker();
        running = pthread_create(&worker, NULL, background, NULL) == 0;
        json = "{\"ok\":true,\"result\":{\"schema_version\":10,\"metadata\":{\"Name\":\"Synthetic warmup probe\",\"Version\":\"0.0.0\",\"Author\":\"test\",\"GitHubRepository\":\"https://github.com/router-for-me/CLIProxyAPI\"},\"capabilities\":{\"scheduler\":true}}}";
    } else if (!strcmp(method, "plugin.quiesce") || !strcmp(method, "plugin.shutdown")) {
        join_worker();
    }
    out->len = strlen(json);
    out->ptr = malloc(out->len);
    if (out->ptr == NULL) return 1;
    memcpy(out->ptr, json, out->len);
    return 0;
}

static void release(void *ptr, size_t len) { (void)len; free(ptr); }
int cliproxy_plugin_init(cliproxy_host_api *host, cliproxy_plugin_api *api) {
    if (!host || !api || host->abi_version != 1) return 1;
    host_api = *host;
    api->abi_version = 1;
    api->call = call;
    api->free_buffer = release;
    api->shutdown = join_worker;
    return 0;
}
