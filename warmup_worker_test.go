//go:build !windows

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWarmupWorkerClaimsBeforeSendAndSkipsUncertainReplay(t *testing.T) {
	dir := provisionWarmupState(t)
	var sent atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case warmupAuthFiles:
			io.WriteString(w, `{"files":[{"name":"a.json","auth_index":"a","provider":"codex","account_type":"oauth","disabled":false,"unavailable":false,"runtime_only":false,"status":"active","cooldowns":[]},{"name":"copy.json","auth_index":"copy","provider":"codex","account_type":"oauth","disabled":false,"unavailable":false,"runtime_only":false,"status":"active","cooldowns":[]}]}`)
		case warmupAuthFiles + "/models":
			io.WriteString(w, `{"models":[{"id":"gpt-test"}]}`)
		case warmupVerify:
			if raw, err := os.ReadFile(filepath.Join(dir, warmupLedgerFile)); err != nil || string(raw) == "{}" {
				t.Error("request preceded durable claim")
			}
			sent.Add(1)
			w.WriteHeader(502)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	cfg := config{WarmupEnabled: true, WarmupEndpoint: server.URL, WarmupDirectory: dir, WarmupModel: "gpt-test", RequestTimeout: time.Second, MaxAge: 5 * time.Minute}
	now := time.Unix(2_000_000_000, 0)
	e := newEngine(nil, nil, cfg)
	e.now = func() time.Time { return now }
	snap := warmupSnapshot(now, now.Add(warmupWindow), 0)
	for _, id := range []string{"a", "copy"} {
		e.states[id+".json"] = accountState{Index: id, Stamp: "same", Snapshot: &snap, WarmupCycle: now.Add(-time.Minute)}
	}
	w, err := openWarmupWorker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	candidates := e.warmupCandidates()
	if len(candidates) != 2 || candidates[0].id != "a.json" {
		t.Fatalf("unexpected candidates: %v", candidates)
	}
	if taken, usable := w.process(context.Background(), e, candidates[0]); !taken || !usable || sent.Load() != 1 || e.states["a.json"].WarmupStatus != "unknown" {
		t.Fatal("did not record one uncertain request")
	}
	w.ledger.close()
	w.cpa.client.CloseIdleConnections()
	w, err = openWarmupWorker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer w.ledger.close()
	defer w.cpa.client.CloseIdleConnections()
	if taken, usable := w.process(context.Background(), e, candidates[1]); taken || !usable || sent.Load() != 1 || e.states["copy.json"].WarmupStatus != "already_attempted" {
		t.Fatal("replayed account after reload")
	}
	e.listErr = "credential_list_failed"
	if len(e.warmupCandidates()) != 0 || e.warmupStillReady(candidates[0]) {
		t.Fatal("used cached candidate after roster failure")
	}
}

func TestWarmupConfirmedFromStableResetEvenWhenPercentRoundsDown(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	cfg := config{MaxAge: 5 * time.Minute, WarmupModel: "gpt-test"}
	s := accountState{WarmupSentAt: now, WarmupStatus: "sent"}
	end := now.Add(warmupWindow)
	apply := func(after time.Duration, ends time.Time) {
		at := now.Add(after)
		s.observeWarmup(warmupSnapshot(at, ends, 0), at, cfg)
	}
	apply(time.Minute, end)
	apply(time.Minute+2*time.Second, end)
	if s.WarmupStatus != "sent" {
		t.Fatal("premature confirmation")
	}
	// CPA persists verification state in the auth file, which invalidates
	// cached routing quota without discarding same-account attribution.
	var refreshed accountState
	refreshed.carryWarmupAcrossCredentialWrite(s)
	s = refreshed
	apply(2*time.Minute, end)
	if s.WarmupStatus != "confirmed" || !s.WarmupWindow.activeEnds.Equal(end) {
		t.Fatal("did not confirm stable deadline")
	}
}
