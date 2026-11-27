package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func warmupSnapshot(at, reset time.Time, used float64) quotaSnapshot {
	weeklyUsed := 10.0
	weeklyReset := at.Add(24 * time.Hour)
	return quotaSnapshot{
		Provider: "codex", ReadAt: at,
		FiveHour:               &quotaWindow{Utilization: &used, ResetsAt: &reset},
		Weekly:                 quotaWindow{Utilization: &weeklyUsed, ResetsAt: &weeklyReset},
		warmupAccountKey:       warmupAccountKey("synthetic-account"),
		warmupFiveHourReported: true,
		warmupAbsoluteReset:    true,
		warmupSubscriptionOnly: true,
		warmupProviderAllows:   true,
	}
}

func TestWarmupDefaultModel(t *testing.T) {
	const enabled = "warmup_after_reset: true\nwarmup_cpa_url: http://127.0.0.1:8317\n"
	// Use an absolute path on every supported test platform.
	base := enabled + fmt.Sprintf("warmup_state_dir: %q\n", t.TempDir())
	for _, test := range []struct{ suffix, want string }{
		{"", "gpt-6-luna"},
		{"warmup_model: \"\"\n", "gpt-6-luna"},
		{"warmup_model: null\n", "gpt-6-luna"},
		{"warmup_model: gpt-5.6-terra\n", "gpt-5.6-terra"},
	} {
		cfg, err := decodeConfig([]byte(base + test.suffix))
		if err != nil || !cfg.WarmupEnabled || cfg.WarmupModel != test.want {
			t.Fatalf("config %q selected %q, want %q (%v)", test.suffix, cfg.WarmupModel, test.want, err)
		}
	}
	if _, err := decodeConfig([]byte(base + "warmup_model: not-a-model\n")); err == nil {
		t.Fatal("accepted an invalid explicit model")
	}
}

func TestWarmupNaturalReset(t *testing.T) {
	boundary := time.Unix(2_000_000_000, 0)
	var state warmupWindowState
	first := boundary.Add(30 * time.Second)
	second := first.Add(time.Minute)
	for _, tc := range []struct {
		name    string
		sample  quotaSnapshot
		wantDue bool
	}{
		{"active", warmupSnapshot(boundary.Add(-time.Minute), boundary, 80), false},
		{"minor boundary drift", warmupSnapshot(boundary.Add(-45*time.Second), boundary.Add(2*time.Second), 80), false},
		{"one idle observation", warmupSnapshot(first, first.Add(warmupWindow), 0), false},
		{"same observation repeated", warmupSnapshot(first, first.Add(warmupWindow), 0), false},
		{"idle horizon slides", warmupSnapshot(second, second.Add(warmupWindow), 0), true},
		{"still same cycle after slide", warmupSnapshot(second.Add(time.Minute), second.Add(warmupWindow+time.Minute), 0), true},
		{"real request activated next window", warmupSnapshot(second.Add(2*time.Minute), second.Add(warmupWindow), 1), false},
	} {
		got := state.observe(tc.sample, tc.sample.ReadAt, 5*time.Minute)
		if tc.wantDue && !got.Equal(boundary) || !tc.wantDue && !got.IsZero() {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestWarmupUncertainReset(t *testing.T) {
	boundary := time.Unix(2_000_000_000, 0)
	active := warmupSnapshot(boundary.Add(-time.Minute), boundary, 80)
	one := boundary.Add(time.Minute)
	two := one.Add(time.Minute)
	idle := warmupSnapshot(one, one.Add(warmupWindow), 0)
	later := warmupSnapshot(two, two.Add(warmupWindow), 0)
	for _, c := range []struct {
		name string
		use  []quotaSnapshot
		now  time.Time
	}{
		{"already idle on startup", []quotaSnapshot{idle, later}, two},
		{"read after eligibility window", []quotaSnapshot{active, idle, later}, boundary.Add(11 * time.Minute)},
		{"stale", []quotaSnapshot{active, idle, later}, two.Add(5 * time.Minute)},
		{"zero but fixed boundary", []quotaSnapshot{active, idle, warmupSnapshot(two, one.Add(warmupWindow), 0)}, two},
		{"no absolute reset", []quotaSnapshot{active, idle, func() quotaSnapshot { s := later; s.warmupAbsoluteReset = false; return s }()}, two},
		{"credential changed accounts", []quotaSnapshot{active, idle, func() quotaSnapshot { s := later; s.warmupAccountKey = warmupAccountKey("different"); return s }()}, two},
		{"real traffic starts between idle readings", []quotaSnapshot{active, idle, warmupSnapshot(two, two.Add(warmupWindow-5*time.Second), 1)}, two},
	} {
		t.Run(c.name, func(t *testing.T) {
			var state warmupWindowState
			for i, s := range c.use {
				now := s.ReadAt
				if i == len(c.use)-1 {
					now = c.now
				}
				if got := state.observe(s, now, 5*time.Minute); !got.IsZero() {
					t.Fatalf("uncertain observation triggered warmup for %v", got)
				}
			}
		})
	}
}

func TestWarmupFailedReadBreaksIdleProof(t *testing.T) {
	boundary := time.Unix(2_000_000_000, 0)
	var state warmupWindowState
	for _, at := range []time.Time{boundary.Add(-time.Minute), boundary.Add(time.Minute)} {
		used, reset := 0.0, at.Add(warmupWindow)
		if at.Before(boundary) {
			used, reset = 20, boundary
		}
		state.observe(warmupSnapshot(at, reset, used), at, 5*time.Minute)
	}
	state.breakIdleEvidence()
	second := boundary.Add(2 * time.Minute)
	if got := state.observe(warmupSnapshot(second, second.Add(warmupWindow), 0), second, 5*time.Minute); !got.IsZero() {
		t.Fatalf("counted idle evidence across a failed read: %v", got)
	}
	third := second.Add(time.Minute)
	if got := state.observe(warmupSnapshot(third, third.Add(warmupWindow), 0), third, 5*time.Minute); !got.Equal(boundary) {
		t.Fatalf("lost active boundary across a failed read: %v", got)
	}
}

func TestWarmupQuotaEvidence(t *testing.T) {
	at := time.Unix(2_000_000_000, 0)
	windows := `"primary_window":{"used_percent":0,"limit_window_seconds":18000,"reset_at":2000018000},"secondary_window":{"used_percent":10,"limit_window_seconds":604800,"reset_at":2000086400}`
	credits := `,"credits":{"has_credits":false,"unlimited":false,"balance":"0.00"}`
	for _, tc := range []struct {
		name  string
		main  string
		extra string
		ready bool
	}{
		{"explicit eligible", `"allowed":true,"limit_reached":false,` + windows, credits, true},
		{"no billing evidence", `"allowed":true,"limit_reached":false,` + windows, "", false},
		{"paid balance", `"allowed":true,"limit_reached":false,` + windows, strings.Replace(credits, `"0.00"`, `1`, 1), false},
		{"unlimited", `"allowed":true,"limit_reached":false,` + windows, strings.Replace(credits, `"unlimited":false`, `"unlimited":true`, 1), false},
		{"incomplete credits", `"allowed":true,"limit_reached":false,` + windows, `,"credits":{"balance":0}`, false},
		{"missing allowance confirmation", `"limit_reached":false,` + windows, credits, false},
		{"global block", `"allowed":false,"limit_reached":true,` + windows, credits, false},
		{"guessed duration", `"allowed":true,"limit_reached":false,` + strings.Replace(windows, `"limit_window_seconds":18000,`, ``, 1), credits, false},
		{"relative reset alone", `"allowed":true,"limit_reached":false,` + strings.Replace(windows, `"reset_at":2000018000`, `"reset_after_seconds":18000`, 1), credits, false},
		{"weekly exhausted", `"allowed":true,"limit_reached":false,` + strings.Replace(windows, `"used_percent":10`, `"used_percent":100`, 1), credits, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := parseCodexQuota([]byte(fmt.Sprintf(`{"rate_limit":{%s}%s}`, tc.main, tc.extra)), at)
			if err != nil {
				t.Fatal(err)
			}
			s.warmupAccountKey = warmupAccountKey("synthetic-account")
			if got := warmupQuotaReady(s, "gpt-test", at, 5*time.Minute); got != tc.ready {
				t.Fatalf("warmup ready = %v, want %v", got, tc.ready)
			}
			raw, err := json.Marshal(s)
			if err != nil || strings.Contains(string(raw), "warmup") || strings.Contains(string(raw), s.warmupAccountKey) {
				t.Fatalf("private warmup evidence leaked: %s (%v)", raw, err)
			}
		})
	}
}
