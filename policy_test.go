package main

import (
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)

func ptrFloat(n float64) *float64    { return &n }
func ptrTime(t time.Time) *time.Time { return &t }
func readyState(minutes int, used float64) accountState {
	return accountState{Provider: "codex", Snapshot: &quotaSnapshot{Provider: "codex", ReadAt: testNow, Identity: "test", FiveHour: &quotaWindow{Utilization: ptrFloat(used), ResetsAt: ptrTime(testNow.Add(time.Duration(minutes) * time.Minute))}, Weekly: quotaWindow{Utilization: ptrFloat(10), ResetsAt: ptrTime(testNow.Add(24 * time.Hour))}}}
}
func fixtureRequest() pluginapi.SchedulerPickRequest {
	return pluginapi.SchedulerPickRequest{Provider: "codex", Providers: []string{"codex"}, Model: "gpt-test", Candidates: []pluginapi.SchedulerAuthCandidate{{ID: "a", Provider: "codex", Priority: 20}, {ID: "b", Provider: "codex", Priority: 20}, {ID: "d", Provider: "codex", Priority: 10}}}
}
func TestPolicy(t *testing.T) {
	tests := []struct {
		name         string
		change       func(*pluginapi.SchedulerPickRequest, map[string]accountState)
		want, reason string
		excluded     int
	}{
		{"tier before reset", nil, "a", "earliest_five_hour_reset", 0},
		{"free skipped before tier and reset ranking", func(r *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			r.Candidates[0].Priority = 100
			s["a"].Snapshot.PlanType = " Free "
			s["b"].Snapshot.PlanType = "plus"
		}, "b", "earliest_five_hour_reset", 1},
		{"last confirmed plan retained with stale quota", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["a"].Snapshot.PlanType = "free"
			s["a"].Snapshot.ReadAt = testNow.Add(-time.Hour)
		}, "b", "earliest_five_hour_reset", 1},
		{"old Free snapshot not applied to replaced account", func(r *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["a"].Snapshot.PlanType = "free"
			r.Candidates[0].Metadata = map[string]any{"account_id": "different"}
		}, "", "tier_quota_unknown_delegate", 0},
		{"filename and candidate metadata cannot assert plan", func(r *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["codex-user-free.json"] = s["a"]
			r.Candidates[0].ID = "codex-user-free.json"
			r.Candidates[0].Metadata = map[string]any{"plan_type": "free"}
		}, "codex-user-free.json", "earliest_five_hour_reset", 0},
		{"delegation retains Free exclusions", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["a"].Snapshot.PlanType = "free"
			delete(s, "b")
		}, "", "tier_quota_unknown_delegate", 1},
		{"only Free or exhausted", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["d"].Snapshot.PlanType = "free"
			s["a"].Snapshot.FiveHour.Utilization = ptrFloat(100)
			s["b"].Snapshot.FiveHour.Utilization = ptrFloat(100)
		}, "", "no_usable_quota", 3},
		{"within tier", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) { s["b"] = readyState(5, 10) }, "b", "earliest_five_hour_reset", 0},
		{"exhausted top tier", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["a"] = readyState(20, 100)
			s["b"] = readyState(120, 100)
		}, "d", "earliest_five_hour_reset", 2},
		{"all exhausted", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			for k := range s {
				s[k] = readyState(20, 100)
			}
		}, "", "no_usable_quota", 3},
		{"weekly exhausted", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["a"].Snapshot.Weekly.Utilization = ptrFloat(100)
		}, "b", "earliest_five_hour_reset", 1},
		{"stable tie", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) { s["b"] = readyState(20, 10) }, "a", "earliest_five_hour_reset", 0},
		{"known reset before unknown reset", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["a"].Snapshot.FiveHour.ResetsAt = nil
		}, "b", "earliest_five_hour_reset", 0},
		{"all resets unknown", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["a"].Snapshot.FiveHour.ResetsAt = nil
			s["b"].Snapshot.FiveHour.ResetsAt = nil
		}, "a", "usable_unknown_reset", 0},
		{"unknown quota preserves tier", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) { delete(s, "a") }, "", "tier_quota_unknown_delegate", 0},
		{"stale snapshot", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["a"].Snapshot.ReadAt = testNow.Add(-10 * time.Minute)
		}, "", "tier_quota_unknown_delegate", 0},
		{"elapsed reset", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["a"].Snapshot.FiveHour.ResetsAt = ptrTime(testNow)
		}, "", "tier_quota_unknown_delegate", 0},
		{"elapsed plus other exhausted", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["a"].Snapshot.FiveHour.ResetsAt = ptrTime(testNow)
			s["a"].Snapshot.Weekly.Utilization = ptrFloat(100)
		}, "b", "earliest_five_hour_reset", 1},
		{"identity change", func(r *pluginapi.SchedulerPickRequest, _ map[string]accountState) {
			r.Candidates[0].Metadata = map[string]any{"account_id": "different"}
		}, "", "tier_quota_unknown_delegate", 0},
		{"mixed providers", func(r *pluginapi.SchedulerPickRequest, _ map[string]accountState) {
			r.Providers = append(r.Providers, "claude")
		}, "", "provider_not_exclusively_supported", 0},
		{"claude unhandled", func(r *pluginapi.SchedulerPickRequest, _ map[string]accountState) { r.Provider = "claude" }, "", "provider_not_exclusively_supported", 0},
		{"disabled", func(r *pluginapi.SchedulerPickRequest, _ map[string]accountState) {
			r.Candidates[0].Status = "disabled"
		}, "b", "earliest_five_hour_reset", 1},
		{"model quota", func(_ *pluginapi.SchedulerPickRequest, s map[string]accountState) {
			s["a"].Snapshot.Models = map[string][]quotaWindow{"gpt-test": {{Utilization: ptrFloat(100), ResetsAt: ptrTime(testNow.Add(time.Hour))}}}
		}, "b", "earliest_five_hour_reset", 1},
		{"no candidates", func(r *pluginapi.SchedulerPickRequest, _ map[string]accountState) { r.Candidates = nil }, "", "no_usable_quota", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := fixtureRequest()
			s := map[string]accountState{"a": readyState(20, 30), "b": readyState(120, 20), "d": readyState(10, 10)}
			if tt.change != nil {
				tt.change(&r, s)
			}
			d := choose(r, s, testNow, 10*time.Minute)
			if d.AuthID != tt.want || d.Reason != tt.reason || len(d.Excluded) != tt.excluded {
				t.Fatalf("%+v", d)
			}
		})
	}
}
func TestEnabledRouterSelectsAndFailedWorkerDelegates(t *testing.T) {
	cfg, _ := decodeConfig(nil)
	e := newEngine(nil, nil, cfg)
	e.now = func() time.Time { return testNow }
	e.states = map[string]accountState{"a": readyState(20, 100), "b": readyState(120, 100), "d": readyState(10, 10)}
	r := e.pick(fixtureRequest())
	if !r.Handled || r.AuthID != "d" {
		t.Fatalf("%+v", r)
	}
	e.failed.Store(true)
	if e.pick(fixtureRequest()).Handled {
		t.Fatal("failed worker still routing")
	}
}
