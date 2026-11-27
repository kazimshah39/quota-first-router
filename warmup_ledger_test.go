//go:build !windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func provisionWarmupState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{warmupLedgerLock: "", warmupLedgerFile: "{}", warmupKeyFile: "synthetic-test-key\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestWarmupLedgerExclusiveAndDurable(t *testing.T) {
	dir := provisionWarmupState(t)
	l, err := openWarmupLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := openWarmupLedger(dir); !errors.Is(err, errWarmupLedger) {
		second.close()
		t.Fatalf("second owner: %v", err)
	}
	now := time.Unix(2_000_000_000, 0)
	key := warmupAccountKey("account")
	if ok, err := l.claim(key, now.Add(-time.Minute), now); err != nil || !ok {
		t.Fatalf("first claim = %v %v", ok, err)
	}
	l.close()
	l, err = openWarmupLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	for _, cycle := range []time.Time{now.Add(-time.Minute), now.Add(-time.Minute + 3*time.Second), now.Add(-5 * time.Hour)} {
		if ok, err := l.claim(key, cycle, now); err != nil || ok {
			t.Fatalf("duplicate claim = %v %v", ok, err)
		}
	}
	if ok, err := l.claim(key, now.Add(5*time.Hour-time.Minute), now.Add(5*time.Hour)); err != nil || !ok {
		t.Fatalf("next cycle claim = %v %v", ok, err)
	}
	var entries map[string]int64
	raw, err := os.ReadFile(filepath.Join(dir, warmupLedgerFile))
	if err != nil || json.Unmarshal(raw, &entries) != nil || len(entries) != 1 || entries[key] != now.Add(5*time.Hour-time.Minute).Unix() {
		t.Fatalf("invalid journal: %v, %v", entries, err)
	}
}

func TestWarmupLedgerFailsClosed(t *testing.T) {
	for _, name := range []string{"missing", "corrupt", "public", "symlink", "directory"} {
		t.Run(name, func(t *testing.T) {
			dir := provisionWarmupState(t)
			p := filepath.Join(dir, warmupLedgerFile)
			var err error
			switch name {
			case "missing":
				err = os.Remove(p)
			case "corrupt":
				err = os.WriteFile(p, []byte(`{"account":123}`), 0600)
			case "public":
				err = os.Chmod(p, 0644)
			case "symlink":
				if err = os.Rename(p, p+".real"); err == nil {
					err = os.Symlink(p+".real", p)
				}
			case "directory":
				err = os.Chmod(dir, 0755)
			}
			if err != nil {
				t.Fatal(err)
			}
			if l, err := openWarmupLedger(dir); !errors.Is(err, errWarmupLedger) {
				l.close()
				t.Fatalf("unsafe state accepted: %v", err)
			}
		})
	}
}
