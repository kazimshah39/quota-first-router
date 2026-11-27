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
	sampleRead time.Time
	sampleEnds time.Time
	idleSince  time.Time
}

// observe returns a candidate only after two zero-use observations prove the
// reset horizon is sliding. A known active boundary identifies a natural
// reset. Without one, idleSince is a local marker, not a provider cycle ID;
// the ledger must only allow that catch-up when no prior claim exists.
// Window evidence never affects routing eligibility or ranking.
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
	if *five.Utilization != 0 || remaining <= 0 || remaining > warmupWindow+warmupIdleSlack {
		w.breakIdleEvidence()
		return time.Time{}
	}
	elapsed, drift := s.ReadAt.Sub(w.sampleRead), ends.Sub(w.sampleEnds)
	if w.sampleRead.IsZero() || elapsed >= maxAge {
		w.breakIdleEvidence()
		w.sampleRead, w.sampleEnds = s.ReadAt, ends
		return time.Time{}
	}
	looksIdle := warmupClose(remaining-warmupWindow, warmupIdleSlack) && drift > 0 && warmupClose(drift-elapsed, warmupResetJitter)
	if !looksIdle {
		w.idleSince = time.Time{}
	}
	if elapsed >= warmupMinSample {
		if looksIdle && w.idleSince.IsZero() {
			w.idleSince = w.sampleRead
		}
		if !looksIdle && warmupClose(drift, warmupResetJitter) {
			// A window can be active even when its usage rounds to zero.
			w.activeEnds = ends
		}
		w.sampleRead, w.sampleEnds = s.ReadAt, ends
	}
	if w.idleSince.IsZero() {
		return time.Time{}
	}
	if w.activeEnds.IsZero() {
		return w.idleSince
	}
	if s.ReadAt.Before(w.activeEnds.Add(warmupResetGrace)) || now.Sub(w.activeEnds) > warmupMaxLateness {
		return time.Time{}
	}
	return w.activeEnds
}

// A failed read breaks the two-sample idle proof, not the known active cycle.
func (w *warmupWindowState) breakIdleEvidence() {
	w.sampleRead, w.sampleEnds, w.idleSince = time.Time{}, time.Time{}, time.Time{}
}

func warmupFresh(s quotaSnapshot, now time.Time, maxAge time.Duration) bool {
	return !s.ReadAt.IsZero() && !now.Before(s.ReadAt) && now.Sub(s.ReadAt) < maxAge
}

func warmupClose(delta, tolerance time.Duration) bool {
	return delta >= -tolerance && delta <= tolerance
}

func warmupQuotaReady(s quotaSnapshot, model string, now time.Time, maxAge time.Duration) bool {
	return s.Provider == "codex" && !s.confirmedFree() && s.warmupAccountKey != "" && s.warmupFiveHourReported &&
		s.warmupAbsoluteReset && s.warmupSubscriptionOnly && s.warmupProviderAllows &&
		s.FiveHour != nil && s.FiveHour.Utilization != nil && *s.FiveHour.Utilization == 0 &&
		s.Weekly.Utilization != nil && s.Weekly.ResetsAt != nil && model != "" &&
		s.availability(model, now, maxAge) == "ready"
}
