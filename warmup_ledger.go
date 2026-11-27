package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const warmupLedgerLimit = 4096
const warmupLedgerBytes = 512 * 1024
const warmupLedgerFile = "warmup-attempts.json"
const warmupLedgerLock = "warmup-attempts.lock"

var errWarmupLedger = errors.New("warmup_state_unavailable")

type warmupLedger struct {
	root *os.Root
	lock *flock.Flock
}

// State is provisioned explicitly, so deleting the journal cannot silently reset
// deduplication. Acquire an OS lock across instances; a crash releases the lock.
func openWarmupLedger(dir string) (*warmupLedger, error) {
	if !filepath.IsAbs(dir) {
		return nil, errWarmupLedger
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || !warmupPrivateMode(info.Mode(), true) {
		return nil, errWarmupLedger
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, errWarmupLedger
	}
	fail := func() (*warmupLedger, error) { root.Close(); return nil, errWarmupLedger }
	name := filepath.Join(dir, warmupLedgerLock)
	file, err := openWarmupRegular(root, warmupLedgerLock)
	if err != nil {
		return fail()
	}
	file.Close()
	lock := flock.New(name, flock.SetFlag(os.O_RDWR), flock.SetPermissions(0600))
	locked, err := lock.TryLock()
	if !locked || err != nil {
		return fail()
	}
	l := &warmupLedger{root: root, lock: lock}
	if _, err = l.read(); err != nil {
		l.close()
		return nil, errWarmupLedger
	}
	return l, nil
}

func openWarmupRegular(root *os.Root, name string) (*os.File, error) {
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || !warmupPrivateMode(info.Mode(), false) {
		return nil, errWarmupLedger
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, errWarmupLedger
	}
	now, err := f.Stat()
	if err != nil || !os.SameFile(info, now) {
		f.Close()
		return nil, errWarmupLedger
	}
	return f, nil
}

func (l *warmupLedger) close() { l.lock.Unlock(); l.root.Close() }

func (l *warmupLedger) read() (map[string]int64, error) {
	f, err := openWarmupRegular(l.root, warmupLedgerFile)
	if err != nil {
		return nil, errWarmupLedger
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, warmupLedgerBytes+1))
	if err != nil || len(data) > warmupLedgerBytes {
		return nil, errWarmupLedger
	}
	var entries map[string]int64
	if json.Unmarshal(data, &entries) != nil || entries == nil || len(entries) > warmupLedgerLimit {
		return nil, errWarmupLedger
	}
	for key, cycle := range entries {
		if decoded, err := hex.DecodeString(key); err != nil || len(decoded) != 32 || cycle <= 0 || cycle > 253402300799 {
			return nil, errWarmupLedger
		}
	}
	return entries, nil
}

// Max claimed active boundary per account prevents replay across small reset
// timestamp changes, failures, restarts, and duplicate credential files.
func (l *warmupLedger) claim(key string, cycle, now time.Time) (bool, error) {
	if decoded, err := hex.DecodeString(key); err != nil || len(decoded) != 32 || cycle.Unix() <= 0 || cycle.After(now) {
		return false, errWarmupLedger
	}
	entries, err := l.read()
	if err != nil {
		return false, errWarmupLedger
	}
	last, exists := entries[key]
	if exists && cycle.Unix() <= last+int64(warmupResetJitter/time.Second) {
		return false, nil
	}
	// No still-eligible cycle can be older than this. Bound the ledger without
	// discarding a claim that could be reused in a current warmup window.
	for id, boundary := range entries {
		if now.Sub(time.Unix(boundary, 0)) > 24*time.Hour {
			delete(entries, id)
		}
	}
	if _, exists = entries[key]; !exists && len(entries) >= warmupLedgerLimit {
		return false, errWarmupLedger
	}
	entries[key] = cycle.Unix()
	raw, err := json.Marshal(entries)
	if err != nil {
		return false, errWarmupLedger
	}
	// A stable temp name is safe under the lock. Remove only the name owned by
	// this ledger; never follow it if it was replaced by a symlink.
	const tmp = "warmup-attempts.tmp"
	if err = l.root.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, errWarmupLedger
	}
	f, err := l.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return false, errWarmupLedger
	}
	defer l.root.Remove(tmp)
	_, writeErr := f.Write(raw)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return false, errWarmupLedger
	}
	if replaceWarmupLedger(l.root, tmp, warmupLedgerFile) != nil {
		return false, errWarmupLedger
	}
	return true, nil
}
