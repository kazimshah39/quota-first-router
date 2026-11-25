package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestAdditionalQuotaShapes(t *testing.T) {
	base := strings.TrimSuffix(quotaJSON(20, 10), "}")
	for _, tt := range []struct{ name, extra, model, want string }{
		{"object meter", `,"additional_rate_limits":{"gpt-special":{"primary_window":{"used_percent":100,"reset_after_seconds":120}}}}`, "gpt-special", "exhausted"},
		{"normal slug not display name", `,"additional_rate_limits":[{"limit_name":"Special Model","normal_model_slug":"gpt-special","metered_feature":"special_feature","rate_limit":{"primary_window":{"used_percent":100,"reset_after_seconds":120}}}]}`, "gpt-special", "exhausted"},
		{"camel windows", `,"additionalRateLimits":[{"limitName":"gpt-special","rateLimit":{"primaryWindow":{"usedPercent":100,"resetAfterSeconds":120}}}]}`, "gpt-special", "exhausted"},
		{"no substring false match", `,"additional_rate_limits":[{"limit_name":"gpt-special","rate_limit":{"primary_window":{"used_percent":100,"reset_after_seconds":120}}}]}`, "gpt-special-other", "ready"},
		{"duplicate meters retain exhaustion", `,"additional_rate_limits":[{"limit_name":"gpt-special","rate_limit":{"primary_window":{"used_percent":100,"reset_after_seconds":120}}},{"limit_name":"gpt-special","rate_limit":{"primary_window":{"used_percent":0,"reset_after_seconds":120}}}]}`, "gpt-special", "exhausted"},
		{"spend stop", `,"spend_control":{"reached":true}}`, "gpt-test", "exhausted"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := parseCodexQuota([]byte(base+tt.extra), testNow)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.availability(tt.model, testNow, time.Minute); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}

func TestMalformedQuotaNeverReady(t *testing.T) {
	for _, raw := range []string{
		`{"rate_limit":{"primary_window":{"used_percent":0,"limit_window_seconds":"18000"}}}`,
		`{"rate_limit":{"primary_window":{"used_percent":0,"reset_at":-1}}}`,
		`{"rate_limit":{"primary_window":{"used_percent":0,"reset_at":"bad"}}}`,
		`{"rate_limit":{"primary_window":{"used_percent":0,"reset_after_seconds":1e99}}}`,
		`{"rate_limit":{"primary_window":{"used_percent":0}},"additional_rate_limits":42}`,
		`{"rate_limit":{"primary_window":{"used_percent":0},"allowed":"yes"}}`,
	} {
		s, err := parseCodexQuota([]byte(raw), testNow)
		if err == nil && s.availability("gpt-test", testNow, time.Minute) == "ready" {
			t.Fatalf("malformed considered ready: %s", raw)
		}
	}
}

func TestPolicyPermutationAndTierProperties(t *testing.T) {
	random := rand.New(rand.NewPCG(11, 29))
	for trial := 0; trial < 1000; trial++ {
		req := pluginapi.SchedulerPickRequest{Provider: "codex", Model: "gpt-test"}
		states := map[string]accountState{}
		bestID := ""
		bestTier := -1000
		bestMinutes := 10000
		for i := 0; i < 20; i++ {
			id := fmt.Sprintf("%02d", i)
			tier := random.IntN(5) - 2
			minutes := random.IntN(240) + 1
			used := 10.0
			if random.IntN(4) == 0 {
				used = 100
			}
			req.Candidates = append(req.Candidates, pluginapi.SchedulerAuthCandidate{ID: id, Provider: "codex", Priority: tier})
			states[id] = readyState(minutes, used)
			if used < 100 && (bestID == "" || tier > bestTier || tier == bestTier && (minutes < bestMinutes || minutes == bestMinutes && id < bestID)) {
				bestID = id
				bestTier = tier
				bestMinutes = minutes
			}
		}
		random.Shuffle(len(req.Candidates), func(i, j int) { req.Candidates[i], req.Candidates[j] = req.Candidates[j], req.Candidates[i] })
		d := choose(req, states, testNow, time.Minute)
		if d.AuthID != bestID {
			t.Fatalf("trial %d got %+v want %s", trial, d, bestID)
		}
	}
}
func TestQuotaStaleBoundaryAndFuture(t *testing.T) {
	s := *readyState(20, 10).Snapshot
	for _, tt := range []struct {
		now  time.Time
		want string
	}{{testNow.Add(time.Minute - time.Nanosecond), "ready"}, {testNow.Add(time.Minute), "stale"}, {testNow.Add(-time.Nanosecond), "stale"}} {
		if got := s.availability("gpt-test", tt.now, time.Minute); got != tt.want {
			t.Fatal(got)
		}
	}
}
func TestInvalidProxyRejectedAtConfiguration(t *testing.T) {
	for _, raw := range []string{"proxy_url: ftp://host", "proxy_url: ':bad'", "proxy_url: http://"} {
		if _, err := decodeConfig([]byte(raw)); err == nil {
			t.Fatal(raw)
		}
	}
}
func TestReadErrorsAndRosterRecovery(t *testing.T) {
	h := &fakeHost{entries: []pluginapi.HostAuthFileEntry{{ID: "a", AuthIndex: "1", Provider: "codex"}}, raw: json.RawMessage(`{`)}
	f := &fakeFetcher{}
	cfg, _ := decodeConfig(nil)
	e := newEngine(h, f, cfg)
	now := testNow
	e.now = func() time.Time { return now }
	e.refreshQuota(context.Background())
	if f.calls != 0 || e.states["a"].Snapshot != nil {
		t.Fatal("invalid credential fetched")
	}
	h.raw = json.RawMessage(`{"type":"codex","access_token":"synthetic"}`)
	now = now.Add(2 * time.Minute)
	e.refreshQuota(context.Background())
	if f.calls != 1 {
		t.Fatal("recovery failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e.refreshQuota(ctx)
	if f.calls != 1 {
		t.Fatal("cancel ignored")
	}
}
func FuzzParseCodexQuota(f *testing.F) {
	for _, s := range []string{quotaJSON(20, 10), `{}`, `null`, `{"rate_limit":{"allowed":false}}`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > maxUsageBytes {
			return
		}
		s, err := parseCodexQuota(raw, testNow)
		if err == nil {
			for _, model := range []string{"gpt-test", "gpt-special"} {
				got := s.availability(model, testNow, time.Minute)
				if got == "" {
					t.Fatal("empty availability")
				}
			}
		}
	})
}
func BenchmarkChoose(b *testing.B) {
	r := fixtureRequest()
	s := map[string]accountState{"a": readyState(20, 20), "b": readyState(30, 20), "d": readyState(10, 20)}
	b.ReportAllocs()
	for b.Loop() {
		choose(r, s, testNow, time.Minute)
	}
}
