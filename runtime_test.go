package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestReleaseDefaults(t *testing.T) {
	cfg, err := decodeConfig(nil)
	if err != nil || cfg.Mode != "shadow" || cfg.PollInterval != time.Minute || cfg.MaxAge != 5*time.Minute || cfg.RequestTimeout != 10*time.Second {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	for _, raw := range []string{"enabled: true", "mode: active"} {
		cfg, err = decodeConfig([]byte(raw))
		if err != nil || cfg.PollInterval != time.Minute || cfg.MaxAge != 5*time.Minute {
			t.Fatalf("partial defaults: %+v %v", cfg, err)
		}
	}
}

func TestRuntimeProtocol(t *testing.T) {
	h := &fakeHost{}
	p := &pluginRuntime{host: h}
	defer p.stop()
	raw := p.handle(pluginabi.MethodPluginRegister, []byte(`{}`))
	var env pluginabi.Envelope
	if err := json.Unmarshal(raw, &env); err != nil || !env.OK {
		t.Fatal(string(raw), err)
	}
	if !strings.Contains(string(env.Result), `"scheduler_across_priorities":true`) {
		t.Fatal(string(raw))
	}
	for _, method := range []string{pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure} {
		if !strings.Contains(string(p.handle(method, []byte(`{"config_yaml":"bW9kZTogYWN0aXZlCg=="}`))), `"ok":true`) {
			t.Fatal(method)
		}
	}
	if !strings.Contains(string(p.handle(pluginabi.MethodManagementRegister, nil)), statusPath) {
		t.Fatal("missing status route")
	}
	req, _ := json.Marshal(pluginapi.ManagementRequest{Method: "GET", Path: statusPath})
	raw = p.handle(pluginabi.MethodManagementHandle, req)
	if strings.Contains(string(raw), "secret") || !strings.Contains(string(raw), `"ok":true`) {
		t.Fatal(string(raw))
	}
	if !strings.Contains(string(p.handle("unknown", nil)), "unsupported_method") {
		t.Fatal("unsupported")
	}
	if !strings.Contains(string(p.handle(pluginabi.MethodPluginReconfigure, []byte(`{`))), "invalid_lifecycle_request") {
		t.Fatal("bad json")
	}
	p.stop()
	raw = p.handle(pluginabi.MethodSchedulerPick, []byte(`{}`))
	if !strings.Contains(string(raw), `"Handled":false`) {
		t.Fatal(string(raw))
	}
}
func TestRefreshFailuresAndInvalidation(t *testing.T) {
	h := &fakeHost{entries: []pluginapi.HostAuthFileEntry{{ID: "a", AuthIndex: "1", Provider: "codex", Email: "first"}}, raw: json.RawMessage(`{"type":"codex","access_token":"secret","account_id":"acct"}`)}
	f := &fakeFetcher{}
	cfg, _ := decodeConfig(nil)
	e := newEngine(h, f, cfg)
	e.now = func() time.Time { return testNow }
	e.refreshQuota(context.Background())
	for _, code := range []string{"quota_http_503", "quota_http_403"} {
		state := e.states["a"]
		state.NextRefresh = time.Time{}
		e.states["a"] = state
		f.err = &quotaError{code: code}
		e.refreshQuota(context.Background())
		if e.states["a"].LastError != code {
			t.Fatal(code)
		}
		if code == "quota_http_503" && e.states["a"].Snapshot == nil {
			t.Fatal("transient error erased good data")
		}
		if code == "quota_http_403" && e.states["a"].Snapshot != nil {
			t.Fatal("forbidden data retained")
		}
	}
	f.err = nil
	h.entries[0].Email = "changed"
	e.refreshQuota(context.Background())
	if e.states["a"].Snapshot == nil {
		t.Fatal("credential change not refreshed")
	}
	h.err = errors.New("secret raw failure")
	e.refreshQuota(context.Background())
	b, _ := json.Marshal(e.status())
	if strings.Contains(string(b), "secret") {
		t.Fatal("error leaked")
	}
	h.err = nil
	h.entries[0].Provider = "claude"
	e.refreshQuota(context.Background())
	if len(e.states) != 0 {
		t.Fatal("Claude polled")
	}
}
func TestRefreshScheduling(t *testing.T) {
	s := *readyState(1, 10).Snapshot
	if got := nextRefresh(s, testNow, 5*time.Minute); !got.Equal(testNow.Add(65 * time.Second)) {
		t.Fatal(got)
	}
	s.FiveHour.ResetsAt = ptrTime(testNow.Add(-time.Minute))
	if got := nextRefresh(s, testNow, 5*time.Minute); !got.Equal(testNow.Add(30 * time.Second)) {
		t.Fatal(got)
	}
	if retryAfter("999999", testNow) != time.Hour || retryAfter("-1", testNow) != 0 {
		t.Fatal("retry bound")
	}
}
