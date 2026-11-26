package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func quotaJSON(minutes int, used float64) string {
	return fmt.Sprintf(`{"rate_limit":{"allowed":true,"primary_window":{"used_percent":%g,"limit_window_seconds":18000,"reset_after_seconds":%d},"secondary_window":{"used_percent":10,"limit_window_seconds":604800,"reset_after_seconds":86400}}}`, used, minutes*60)
}
func TestCodexParsing(t *testing.T) {
	for _, tt := range []struct{ name, body, want string }{
		{"normal", quotaJSON(20, 20), "ready"},
		{"exhausted", quotaJSON(20, 100), "exhausted"},
		{"denied", strings.Replace(quotaJSON(20, 20), `"allowed":true`, `"allowed":false`, 1), "exhausted"},
		{"no five hour unknown availability", `{"rate_limit":{"secondary_window":{"used_percent":10,"limit_window_seconds":604800,"reset_after_seconds":86400}}}`, "unknown"},
		{"monthly-only allowed plan", `{"rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":1,"limit_window_seconds":2592000,"reset_after_seconds":1992997},"secondary_window":null}}`, "ready"},
		{"unknown reset", `{"rate_limit":{"primary_window":{"used_percent":10,"limit_window_seconds":18000}}}`, "ready"},
		{"model limit", strings.TrimSuffix(quotaJSON(20, 20), "}") + `,"additional_rate_limits":[{"limit_name":"gpt-test","rate_limit":{"allowed":false,"primary_window":{"used_percent":10,"reset_after_seconds":120}}}]}`, "exhausted"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := parseCodexQuota([]byte(tt.body), testNow)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.availability("gpt-test", testNow, time.Minute); got != tt.want {
				t.Fatal(got)
			}
		})
	}
	for _, body := range []string{`{`, `{}`, `{"rate_limit":{}}`, strings.Replace(quotaJSON(20, 20), `"used_percent":20`, `"used_percent":101`, 1)} {
		if _, err := parseCodexQuota([]byte(body), testNow); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	raw := `{"rateLimit":{"primaryWindow":{"usedPercent":3,"limitWindowSeconds":18000,"resetAt":1795566000}}}`
	if _, err := parseCodexQuota([]byte(raw), testNow); err != nil {
		t.Fatal(err)
	}
}
func TestHTTPQuota(t *testing.T) {
	calls := 0
	code := 200
	body := quotaJSON(20, 20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Chatgpt-Account-Id") != "acct" {
			t.Error("credential headers")
		}
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(code)
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	f := newHTTPFetcher()
	f.endpoint = server.URL
	c := credential{Type: "codex", AccessToken: "secret", AccountID: "acct"}
	s, err := f.Fetch(context.Background(), c)
	if err != nil || s.Identity != c.identity() {
		t.Fatal(err)
	}
	for _, status := range []int{401, 403, 429, 503} {
		code = status
		_, err := f.Fetch(context.Background(), c)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal(err)
		}
	}
	code = 200
	body = strings.Repeat("x", maxUsageBytes+1)
	if _, err := f.Fetch(context.Background(), c); err == nil {
		t.Fatal("large body")
	}
	before := calls
	c.ProxyURL = ":bad"
	if _, err := f.Fetch(context.Background(), c); err == nil || calls != before {
		t.Fatal("invalid proxy bypass")
	}
	c.ProxyURL = ""
	c.BaseURL = "https://custom.invalid"
	if _, err := f.Fetch(context.Background(), c); err == nil || calls != before {
		t.Fatal("custom endpoint bypass")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.BaseURL = ""
	if _, err := f.Fetch(ctx, c); err == nil {
		t.Fatal("cancel ignored")
	}
}
func TestQuotaTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	f := newHTTPFetcher()
	f.endpoint = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := f.Fetch(ctx, credential{Type: "codex", AccessToken: "secret"})
	if err == nil || err.Error() != "quota_transport_failed" {
		t.Fatal(err)
	}
}

func TestProxyAndRedirect(t *testing.T) {
	var calls int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Host != "quota.invalid" {
			t.Error(r.URL)
		}
		fmt.Fprint(w, quotaJSON(20, 10))
	}))
	defer proxy.Close()
	f := newHTTPFetcher()
	f.endpoint = "http://quota.invalid/usage"
	c := credential{Type: "codex", AccessToken: "secret", ProxyURL: proxy.URL}
	if _, err := f.Fetch(context.Background(), c); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	f.endpoint = redirect.URL
	c.ProxyURL = "direct"
	if _, err := f.Fetch(context.Background(), c); err == nil || targetCalls != 0 {
		t.Fatal("followed redirect")
	}
}

type fakeHost struct {
	entries []pluginapi.HostAuthFileEntry
	raw     json.RawMessage
	err     error
}

func (h *fakeHost) List() ([]pluginapi.HostAuthFileEntry, error) { return h.entries, h.err }
func (h *fakeHost) Get(string) (json.RawMessage, error)          { return h.raw, h.err }

type fakeFetcher struct {
	calls int
	err   error
}

func (f *fakeFetcher) Fetch(_ context.Context, c credential) (quotaSnapshot, error) {
	f.calls++
	s := readyState(20, 10).Snapshot
	s.Identity = c.identity()
	return *s, f.err
}
func TestRefreshLifecycle(t *testing.T) {
	h := &fakeHost{entries: []pluginapi.HostAuthFileEntry{{ID: "a", AuthIndex: "1", Provider: "codex"}}, raw: json.RawMessage(`{"type":"codex","access_token":"secret","account_id":"acct"}`)}
	f := &fakeFetcher{}
	cfg, _ := decodeConfig(nil)
	e := newEngine(h, f, cfg)
	e.now = func() time.Time { return testNow }
	e.refreshQuota(context.Background())
	if len(e.states) != 1 || f.calls != 1 {
		t.Fatal("refresh")
	}
	e.refreshQuota(context.Background())
	if f.calls != 1 {
		t.Fatal("poll not bounded")
	}
	req := pluginapi.SchedulerPickRequest{Provider: "codex", Candidates: []pluginapi.SchedulerAuthCandidate{{ID: "a", Provider: "codex"}}}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			for range 100 {
				e.pick(req)
				e.status()
			}
		})
	}
	wg.Wait()
	if f.calls != 1 {
		t.Fatal("request path fetch")
	}
	state := e.states["a"]
	state.NextRefresh = time.Time{}
	e.states["a"] = state
	f.err = &quotaError{code: "quota_http_401"}
	e.refreshQuota(context.Background())
	if e.states["a"].Snapshot != nil || e.states["a"].LastError != "quota_http_401" {
		t.Fatal("401 retained cache")
	}
	h.entries[0].Disabled = true
	e.refreshQuota(context.Background())
	if len(e.states) != 0 {
		t.Fatal("disabled retained")
	}
	h.entries[0].Disabled = false
	f.err = nil
	e.refreshQuota(context.Background())
	h.entries = nil
	e.refreshQuota(context.Background())
	if len(e.states) != 0 {
		t.Fatal("removed retained")
	}
}
func TestConfiguration(t *testing.T) {
	for _, raw := range []string{"mode: shadow", "mode: nope", "poll_interval: 0s", "max_age: 1s", "request_timeout: 31s", "mode: ["} {
		if _, err := decodeConfig([]byte(raw)); err == nil {
			t.Fatal(raw)
		}
	}
	cfg, err := decodeConfig([]byte("poll_interval: 1m\nmax_age: 2m\n"))
	if err != nil || cfg.MaxAge != 2*time.Minute {
		t.Fatal(err)
	}
}
