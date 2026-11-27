package main

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const (
	warmupWindow      = 5 * time.Hour
	warmupResetGrace  = 30 * time.Second
	warmupMinSample   = 45 * time.Second
	warmupResetJitter = 5 * time.Second
	warmupIdleSlack   = 90 * time.Second
	// Delay between idle samples is measured from the provider observations,
	// not the dashboard's rendering or scheduler traffic.
	warmupMaxLateness = 10 * time.Minute
)

// Only an explicitly identified provider account can acquire a warmup claim.
// Do not use credential file names, labels or rotating tokens as account keys.
func warmupAccountKey(accountID string) string {
	sum := sha256.Sum256([]byte("codex\x00" + accountID))
	return hex.EncodeToString(sum[:])
}

type warmupWindowState struct {
	key        string
	lastRead   time.Time
	activeEnds time.Time
	idleRead   time.Time
	idleEnds   time.Time
}

// observe returns the previous active boundary (the cycle key) only after a
// natural reset followed by two zero-use samples with a sliding reset horizon.
// It is deliberately independent of routing and cannot send a request. Callers
// still have to check account, model and billing eligibility immediately before
// sending and claim the returned cycle durably.
func (w *warmupWindowState) observe(s quotaSnapshot, now time.Time, maxAge time.Duration) time.Time {
	if s.warmupAccountKey == "" || s.warmupAccountKey != w.key {
		*w = warmupWindowState{key: s.warmupAccountKey}
	}
	if w.key == "" || s.Provider != "codex" || !s.ReadAt.After(w.lastRead) {
		return time.Time{}
	}
	if !warmupFresh(s, now, maxAge) {
		w.breakIdleEvidence()
		return time.Time{}
	}
	w.lastRead = s.ReadAt
	five := s.FiveHour
	if !s.warmupFiveHourReported || !s.warmupAbsoluteReset || five == nil || five.Utilization == nil || five.ResetsAt == nil {
		w.activeEnds = time.Time{}
		w.breakIdleEvidence()
		return time.Time{}
	}
	ends := *five.ResetsAt
	remaining := ends.Sub(s.ReadAt)
	if *five.Utilization > 0 && remaining > 0 && remaining <= warmupWindow+warmupResetJitter {
		// Preserve the original cycle key across small timestamp jitter, but
		// learn a new active boundary as soon as actual usage shows up.
		if w.activeEnds.IsZero() || !w.activeEnds.After(s.ReadAt) || !warmupClose(ends.Sub(w.activeEnds), warmupResetJitter) {
			w.activeEnds = ends
		}
		w.breakIdleEvidence()
		return time.Time{}
	}
	if w.activeEnds.IsZero() || s.ReadAt.Before(w.activeEnds.Add(warmupResetGrace)) || now.Sub(w.activeEnds) > warmupMaxLateness ||
		*five.Utilization != 0 || !warmupClose(remaining-warmupWindow, warmupIdleSlack) {
		w.breakIdleEvidence()
		return time.Time{}
	}
	if w.idleRead.IsZero() {
		w.idleRead, w.idleEnds = s.ReadAt, ends
		return time.Time{}
	}
	elapsed := s.ReadAt.Sub(w.idleRead)
	if elapsed < warmupMinSample {
		return time.Time{}
	}
	drift := ends.Sub(w.idleEnds)
	if elapsed >= maxAge || drift <= 0 || !warmupClose(drift-elapsed, warmupResetJitter) {
		w.idleRead, w.idleEnds = s.ReadAt, ends
		return time.Time{}
	}
	return w.activeEnds
}

// A failed read breaks the two-sample idle proof, not the known active cycle.
func (w *warmupWindowState) breakIdleEvidence() {
	w.idleRead, w.idleEnds = time.Time{}, time.Time{}
}

func warmupFresh(s quotaSnapshot, now time.Time, maxAge time.Duration) bool {
	return !s.ReadAt.IsZero() && !now.Before(s.ReadAt) && now.Sub(s.ReadAt) < maxAge
}

func warmupClose(delta, tolerance time.Duration) bool {
	return delta >= -tolerance && delta <= tolerance
}

func warmupQuotaReady(s quotaSnapshot, model string, now time.Time, maxAge time.Duration) bool {
	return s.Provider == "codex" && s.warmupAccountKey != "" && s.warmupFiveHourReported &&
		s.warmupAbsoluteReset && s.warmupSubscriptionOnly && s.warmupProviderAllows &&
		s.FiveHour != nil && s.FiveHour.Utilization != nil && *s.FiveHour.Utilization == 0 &&
		s.Weekly.Utilization != nil && s.Weekly.ResetsAt != nil && model != "" &&
		s.availability(model, now, maxAge) == "ready"
}
