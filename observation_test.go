package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestObservationBoundsAndCoalescing(t *testing.T) {
	o := newObservation(testNow)
	d := decision{AuthID: "synthetic", Reason: "ready"}
	for i := 0; i < 125; i++ {
		d.Candidates = append(d.Candidates, candidateNote{ID: strings.Repeat("x", 1024), Reason: "ready"})
		d.Excluded = append(d.Excluded, "synthetic")
	}
	for i := 0; i < 450; i++ {
		o.recordDecision(d, "gpt-test\n", "active", false, testNow.Add(time.Duration(i)*time.Second))
		o.incident(fmt.Sprint(i), "quota", "quota_http_503", testNow)
	}
	o.incident("449", "quota", "quota_http_503", testNow.Add(time.Minute))
	s := o.snapshot()
	if len(s.Decisions) != 200 || len(s.Incidents) != 200 || s.DecisionEvictions != 250 || s.IncidentEvictions != 250 {
		t.Fatalf("bad bounds: %+v", s)
	}
	if s.Decisions[0].Sequence <= s.Decisions[1].Sequence || s.Incidents[0].Count != 2 || len(s.Decisions[0].Decision.Candidates) != 100 || s.CandidateTruncationsForTest() != 22500 {
		t.Fatal("ordering/coalescing/truncation")
	}
	if s.Decisions[0].Model != "gpt-test" || len([]rune(s.Decisions[0].Decision.Candidates[0].ID)) != 129 {
		t.Fatal("unsafe label")
	}
	if len(o.active) > 200 {
		t.Fatal("coalescing map grew unbounded")
	}
	o.incident("449", "quota", "recovered", testNow.Add(2*time.Minute))
	s = o.snapshot()
	if s.Incidents[0].Code != "recovered" || s.Incidents[0].Count != 1 {
		t.Fatal("recovery hidden")
	}
	// Records are detached from the original scheduler candidate slice.
	d.Candidates[0].ID = "mutated"
	if o.snapshot().Decisions[0].Decision.Candidates[0].ID == "mutated" {
		t.Fatal("aliased input")
	}
}
func (s observationSnapshot) CandidateTruncationsForTest() uint64 { return s.Truncations }

func TestMonitorHealthLifecycleAndRedaction(t *testing.T) {
	cfg, _ := decodeConfig(nil)
	h := &fakeHost{}
	e := newEngine(h, &fakeFetcher{}, cfg)
	e.now = func() time.Time { return testNow }
	if e.monitor().State != "no_accounts" {
		t.Fatal("no accounts state")
	}
	e.states["a"] = readyState(20, 10)
	if s := e.monitor(); s.State != "healthy" || s.FreshAccounts != 1 {
		t.Fatalf("healthy %+v", s)
	}
	e.now = func() time.Time { return testNow.Add(6 * time.Minute) }
	if e.monitor().State != "degraded" {
		t.Fatal("stale healthy")
	}
	e.markFailed()
	e.markFailed()
	if s := e.monitor(); s.State != "failed" || len(s.History.Incidents) != 1 {
		t.Fatal("worker failed hidden")
	}
	p := &pluginRuntime{host: h}
	defer p.stop()
	p.configure(cfg)
	first := p.active.Load().observations.epoch
	p.active.Load().observations.recordDecision(decision{}, "test", "shadow", false, testNow)
	p.configure(cfg)
	if p.active.Load().observations.epoch == first || len(p.active.Load().observations.snapshot().Decisions) != 0 {
		t.Fatal("epoch/history not reset")
	}
	p.stop()
	response := p.management(pluginapi.ManagementRequest{Method: "GET", Path: monitorPath})
	if !strings.Contains(string(response.Body), "stopped") {
		t.Fatal("stopped not reported")
	}
	// Arbitrary host fields and raw credentials never enter observation records.
	e = newEngine(h, &fakeFetcher{}, cfg)
	req := fixtureRequest()
	req.Options = pluginapi.SchedulerOptions{Headers: map[string][]string{"Authorization": {"secret-bearer"}}, Metadata: map[string]any{"prompt": "private-prompt"}}
	e.pick(req)
	raw, _ := json.Marshal(e.monitor())
	for _, secret := range []string{"secret-bearer", "private-prompt"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("leaked " + secret)
		}
	}
}
func TestMonitorConcurrentSnapshots(t *testing.T) {
	cfg, _ := decodeConfig(nil)
	e := newEngine(&fakeHost{}, &fakeFetcher{}, cfg)
	e.now = func() time.Time { return testNow }
	e.states["a"] = readyState(20, 10)
	var wg sync.WaitGroup
	for lane := 0; lane < 8; lane++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				e.pick(fixtureRequest())
				e.observations.incident("a", "quota", "quota_http_503", testNow)
				raw, err := json.Marshal(e.monitor())
				if err != nil || len(raw) > 8*1024*1024 {
					t.Error("snapshot encode/bound")
				}
			}
		}()
	}
	wg.Wait()
	s := e.monitor()
	if len(s.History.Decisions) != 200 || s.History.DecisionEvictions != 2200 {
		t.Fatal("lost records")
	}
}
func TestMonitorResourcesAndMethodGuards(t *testing.T) {
	p := &pluginRuntime{}
	for _, path := range []string{resourcePath, resourcePath + ".js", resourcePath + ".css"} {
		r := p.management(pluginapi.ManagementRequest{Method: "GET", Path: path})
		if r.StatusCode != 200 || len(r.Body) == 0 || r.Headers.Get("Content-Security-Policy") == "" || r.Headers.Get("Cache-Control") != "no-store" {
			t.Fatal(path, r)
		}
		if strings.Contains(string(r.Body), "synthetic-management-key") {
			t.Fatal("secret in shell")
		}
	}
	if p.management(pluginapi.ManagementRequest{Method: "POST", Path: resourcePath}).StatusCode != 405 || p.management(pluginapi.ManagementRequest{Method: "GET", Path: resourcePath + "/../status"}).StatusCode != 404 {
		t.Fatal("route guards")
	}
}
func TestIncidentRecoveryFromPollingAndRoster(t *testing.T) {
	h := &fakeHost{entries: []pluginapi.HostAuthFileEntry{{ID: "a", AuthIndex: "1", Provider: "codex"}}, raw: json.RawMessage(`{"type":"codex","access_token":"synthetic"}`)}
	f := &fakeFetcher{err: &quotaError{code: "quota_http_503"}}
	cfg, _ := decodeConfig(nil)
	e := newEngine(h, f, cfg)
	now := testNow
	e.now = func() time.Time { return now }
	e.refreshQuota(context.Background())
	now = now.Add(2 * time.Minute)
	f.err = nil
	e.refreshQuota(context.Background())
	s := e.monitor()
	if len(s.History.Incidents) != 2 || s.History.Incidents[0].Code != "recovered" || s.Accounts[0].Error != "" {
		t.Fatal("quota recovery")
	}
	h.err = fmt.Errorf("secret raw error")
	e.refreshQuota(context.Background())
	h.err = nil
	e.refreshQuota(context.Background())
	s = e.monitor()
	if s.ListError != "" || s.History.Incidents[0].Code != "recovered" {
		t.Fatal("roster recovery")
	}
}
func TestObservationDetachedEpochAndStarting(t *testing.T) {
	o := newObservation(testNow)
	if o.epoch == newObservation(testNow).epoch {
		t.Fatal("same-clock epoch collision")
	}
	o.recordDecision(decision{Candidates: []candidateNote{{ID: "a"}}, Excluded: []string{"b"}}, "test", "active", false, testNow)
	s := o.snapshot()
	s.Decisions[0].Decision.Candidates[0].ID = "mutated"
	s.Decisions[0].Decision.Excluded[0] = "mutated"
	if next := o.snapshot(); next.Decisions[0].Decision.Candidates[0].ID != "a" || next.Decisions[0].Decision.Excluded[0] != "b" {
		t.Fatal("snapshot aliases history")
	}
	cfg, _ := decodeConfig(nil)
	e := newEngine(&fakeHost{}, &fakeFetcher{}, cfg)
	e.now = func() time.Time { return testNow }
	e.states["a"] = accountState{Index: "1", Provider: "codex"}
	if e.monitor().State != "starting" {
		t.Fatal("unobserved startup reported degraded")
	}
	e.states["a"] = accountState{Index: "1", Provider: "codex", LastError: "quota_http_503"}
	if e.monitor().State != "degraded" {
		t.Fatal("failed startup hidden")
	}
}

func TestMonitorAccountQuotaAndResponseBounds(t *testing.T) {
	cfg, _ := decodeConfig(nil)
	e := newEngine(&fakeHost{}, &fakeFetcher{}, cfg)
	e.now = func() time.Time { return testNow }
	for i := 0; i < 1005; i++ {
		e.states[fmt.Sprintf("%04d", i)] = readyState(20, 10)
	}
	q := e.states["0000"].Snapshot
	q.PlanType = "private-provider-plan"
	q.Models = map[string][]quotaWindow{}
	for i := 0; i < 102; i++ {
		q.Models[fmt.Sprintf("%03d", i)+strings.Repeat("x", 200)] = make([]quotaWindow, 103)
	}
	s := e.monitor()
	if len(s.Accounts) != 1000 || s.AccountsTruncated != 5 || s.State != "degraded" {
		t.Fatal("account bounds")
	}
	if len(s.Accounts[0].Quota.Models) != 100 || s.Accounts[0].QuotaDetailsTruncated != 506 {
		t.Fatal("quota detail bounds", s.Accounts[0].QuotaDetailsTruncated)
	}
	if q.PlanType != "private-provider-plan" || len(q.Models) != 102 {
		t.Fatal("monitor mutated policy cache")
	}
	raw, _ := json.Marshal(s)
	if strings.Contains(string(raw), "private-provider-plan") {
		t.Fatal("provider plan leaked")
	}
	// Even extreme internally supplied snapshots cannot return oversized JSON.
	q.Models = map[string][]quotaWindow{}
	for i := 0; i < 30; i++ {
		q.Models[fmt.Sprint(i)] = make([]quotaWindow, 10)
	}
	for id, state := range e.states {
		state.Snapshot = q
		e.states[id] = state
	}
	p := &pluginRuntime{}
	p.active.Store(e)
	r := p.management(pluginapi.ManagementRequest{Method: "GET", Path: monitorPath})
	if len(r.Body) > 8*1024*1024 || r.StatusCode != 503 {
		t.Fatal("response cap", len(r.Body), r.StatusCode)
	}
	for _, now := range []time.Time{testNow.Add(-time.Second), testNow.Add(cfg.MaxAge)} {
		e.now = func() time.Time { return now }
		if e.monitor().FreshAccounts != 0 {
			t.Fatal("server-clock freshness boundary")
		}
	}
}

func TestMonitorUsesRosterEmailAndPriority(t *testing.T) {
	cfg, _ := decodeConfig(nil)
	h := &fakeHost{entries: []pluginapi.HostAuthFileEntry{
		{ID: "codex-1234abcd-old@example.test-plus.json", AuthIndex: "1", Provider: "codex", Email: "new+tag@example.test", Priority: 0},
		{ID: "bad-label.json", AuthIndex: "2", Provider: "codex", Email: "Name <other@example.test>", Priority: -5},
	}}
	e := newEngine(h, &fakeFetcher{}, cfg)
	e.refreshQuota(context.Background())
	snapshot := e.monitor()
	bad, good := snapshot.Accounts[0], snapshot.Accounts[1]
	if good.Email != "new+tag@example.test" || good.Priority == nil || *good.Priority != 0 || bad.Email != "" || bad.Priority == nil || *bad.Priority != -5 || len(snapshot.History.Decisions) != 0 {
		t.Fatal("monitor must report validated roster metadata before traffic")
	}
	raw, err := json.Marshal(e.status())
	var legacy struct {
		Accounts map[string]map[string]json.RawMessage
	}
	if err != nil || json.Unmarshal(raw, &legacy) != nil || legacy.Accounts[good.ID]["email"] != nil || legacy.Accounts[good.ID]["priority"] != nil {
		t.Fatal("status API must not gain roster metadata", err)
	}
}

func BenchmarkRecordedPick(b *testing.B) {
	cfg, _ := decodeConfig([]byte("mode: active"))
	e := newEngine(&fakeHost{}, &fakeFetcher{}, cfg)
	e.now = func() time.Time { return testNow }
	e.states = map[string]accountState{"a": readyState(20, 20), "b": readyState(30, 20), "d": readyState(10, 20)}
	req := fixtureRequest()
	b.ReportAllocs()
	for b.Loop() {
		e.pick(req)
	}
}
