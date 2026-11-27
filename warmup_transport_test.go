//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWarmupConfigRejectsUnsafeAddresses(t *testing.T) {
	for _, bad := range []string{"http://localhost:8317", "http://127.0.0.1:8317/", "http://127.0.0.1:8317?q=1", "https://127.0.0.1:8317", "http://example.org:8317", "http://user@127.0.0.1:8317", "http://127.0.0.1:0", "http://127.0.0.1:8317#", "http://[::1%25eth0]:8317", "http://127.0.0.1:8317\\", "http://[::ffff:127.0.0.1]:8317"} {
		if validWarmupEndpoint(bad) {
			t.Errorf("accepted %s", bad)
		}
	}
	for _, ok := range []string{"http://127.0.0.1:8317", "http://[::1]:8317"} {
		if !validWarmupEndpoint(ok) {
			t.Errorf("rejected %s", ok)
		}
	}
	cfg, err := decodeConfig(nil)
	if err != nil || cfg.WarmupEnabled {
		t.Fatalf("default enabled: %v %v", cfg.WarmupEnabled, err)
	}
	if _, err = decodeConfig([]byte("warmup_after_reset: true")); err == nil {
		t.Fatal("enabled without settings")
	}
}

func TestWarmupTransportExactPinAndCancellation(t *testing.T) {
	var requests, mode atomic.Int32
	disconnected := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-test-key" {
			t.Error("auth missing")
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case warmupAuthFiles:
			io.WriteString(w, `{"files":[{"name":"test account.json","auth_index":"abc","provider":"codex","account_type":"oauth","disabled":false,"unavailable":false,"runtime_only":false,"status":"active","cooldowns":[]}]}`)
		case warmupAuthFiles + "/models":
			if r.URL.Query().Get("name") != "test account.json" {
				t.Error("wrong model target")
			}
			io.WriteString(w, `{"models":[{"id":"gpt-test"}]}`)
		case warmupVerify:
			requests.Add(1)
			var body map[string]string
			if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 3 || body["name"] != "test account.json" || body["auth_index"] != "abc" || body["model"] != "gpt-test" {
				t.Error("wrong pinned payload")
			}
			switch mode.Load() {
			case 1:
				http.Redirect(w, r, "/other", 307)
			case 2:
				<-r.Context().Done()
				close(disconnected)
			default:
				io.WriteString(w, `{"status":"ok","verified":true}`)
			}
		default:
			t.Error("unexpected path", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	root, err := os.OpenRoot(provisionWarmupState(t))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	c, err := newWarmupCPA(srv.URL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer c.client.CloseIdleConnections()
	a := warmupPinnedAccount{"test account.json", "abc"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if why, err := c.eligible(ctx, a, "gpt-test"); err != nil || why != "" {
		t.Fatalf("eligible: %s %v", why, err)
	}
	if why, _ := c.eligible(ctx, a, "gpt-wrong"); why != "model_unavailable" {
		t.Fatalf("wrong model result: %s", why)
	}
	if why, _ := c.eligible(ctx, warmupPinnedAccount{a.name, "bad"}, "gpt-test"); why != "credential_unavailable" {
		t.Fatalf("wrong index result: %s", why)
	}
	if got := c.probe(ctx, a, "gpt-test"); got != "sent" {
		t.Fatalf("probe result: %s", got)
	}
	mode.Store(1)
	if got := c.probe(ctx, a, "gpt-test"); got != "unknown" {
		t.Fatalf("redirect result: %s", got)
	}
	mode.Store(2)
	deadline, stop := context.WithTimeout(ctx, 25*time.Millisecond)
	defer stop()
	if got := c.probe(deadline, a, "gpt-test"); got != "unknown" {
		t.Fatalf("timeout result: %s", got)
	}
	select {
	case <-disconnected:
	case <-ctx.Done():
		t.Fatal("request did not cancel")
	}
	if requests.Load() != 3 {
		t.Fatal("unexpected request count", requests.Load())
	}
	if strings.Contains(errWarmupTransport.Error(), "synthetic-test-key") {
		t.Fatal("secret in error")
	}
}
