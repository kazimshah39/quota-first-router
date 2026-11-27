package main

import (
	"context"
	"sort"
	"time"
)

const warmupSendGap = 3 * time.Second
const warmupProbeTimeout = 12 * time.Second

// Warmup has its own worker and at most one in-flight request. No scheduler or
// management read waits for it. Wakeups coalesce; candidates are never queued.
type warmupWorker struct {
	ledger *warmupLedger
	cpa    *warmupCPA
	wake   chan struct{}
	next   time.Time
}

type warmupCandidate struct {
	id, index, stamp, key string
	cycle                 time.Time
	anchored              bool
}

func openWarmupWorker(cfg config) (*warmupWorker, error) {
	ledger, err := openWarmupLedger(cfg.WarmupDirectory)
	if err != nil {
		return nil, err
	}
	cpa, err := newWarmupCPA(cfg.WarmupEndpoint, ledger.root)
	if err != nil {
		ledger.close()
		return nil, err
	}
	return &warmupWorker{ledger: ledger, cpa: cpa, wake: make(chan struct{}, 1)}, nil
}

func (w *warmupWorker) run(ctx context.Context, e *engine) {
	defer w.ledger.close()
	defer w.cpa.client.CloseIdleConnections()
	ticker := time.NewTicker(warmupSendGap)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-ticker.C:
		}
		if ctx.Err() != nil || e.now().Before(w.next) {
			continue
		}
		candidates := e.warmupCandidates()
		var sent bool
		for _, c := range candidates {
			if ctx.Err() != nil {
				return
			}
			taken, usable := w.process(ctx, e, c)
			if !usable {
				return
			}
			if taken {
				sent = true
				break
			}
		}
		if sent || len(candidates) > 0 {
			w.next = e.now().Add(warmupSendGap)
		}
	}
}

func (w *warmupWorker) process(ctx context.Context, e *engine, c warmupCandidate) (sent, usable bool) {
	model := e.cfg.WarmupModel
	if !e.warmupStillReady(c) {
		return false, true
	}
	check, cancel := context.WithTimeout(ctx, e.cfg.RequestTimeout)
	reason, err := w.cpa.eligible(check, warmupPinnedAccount{c.id, c.index}, model)
	cancel()
	if ctx.Err() != nil {
		return false, true
	}
	if err != nil {
		e.deferWarmup(c, "cpa_unavailable")
		return true, true
	}
	if reason != "" {
		e.deferWarmup(c, reason)
		return true, true
	}
	if !e.warmupStillReady(c) {
		return false, true
	}
	claimed, err := w.ledger.claim(c.key, c.cycle, e.now(), c.anchored)
	if err != nil {
		e.mu.Lock()
		e.warmupError = "state_unavailable"
		e.mu.Unlock()
		e.setWarmupResult(c, "state_unavailable", false)
		return false, false
	}
	if !claimed {
		e.setWarmupResult(c, "already_attempted", false)
		return false, true
	}
	if ctx.Err() != nil || !e.warmupStillReady(c) {
		return true, true
	}
	probe, cancel := context.WithTimeout(ctx, warmupProbeTimeout)
	outcome := w.cpa.probe(probe, warmupPinnedAccount{c.id, c.index}, model)
	cancel()
	e.setWarmupResult(c, outcome, true)
	return true, true
}

func (e *engine) warmupCandidates() []warmupCandidate {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.listErr != "" || e.warmupError != "" {
		return nil
	}
	var result []warmupCandidate
	now := e.now()
	for id, s := range e.states {
		if warmupAccountReady(s, now, e.cfg) {
			result = append(result, warmupCandidate{id, s.Index, s.Stamp, s.Snapshot.warmupAccountKey, s.WarmupCycle, !s.WarmupWindow.activeEnds.IsZero()})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].cycle.Equal(result[j].cycle) {
			return result[i].id < result[j].id
		}
		return result[i].cycle.Before(result[j].cycle)
	})
	return result
}

func (e *engine) warmupStillReady(c warmupCandidate) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.states[c.id]
	return ok && e.listErr == "" && e.warmupError == "" && s.Index == c.index && s.Stamp == c.stamp &&
		warmupAccountReady(s, e.now(), e.cfg) && s.Snapshot.warmupAccountKey == c.key && s.WarmupCycle.Equal(c.cycle) && c.anchored == !s.WarmupWindow.activeEnds.IsZero()
}

func warmupAccountReady(s accountState, now time.Time, cfg config) bool {
	return s.Snapshot != nil && s.LastError == "" && !s.WarmupCycle.IsZero() && !now.Before(s.WarmupCycle) &&
		(s.WarmupWindow.activeEnds.IsZero() || now.Sub(s.WarmupCycle) <= warmupMaxLateness) &&
		(s.WarmupHandled.IsZero() || s.WarmupHandled.Before(s.WarmupCycle)) && !now.Before(s.WarmupRetryAt) &&
		warmupQuotaReady(*s.Snapshot, cfg.WarmupModel, now, min(cfg.MaxAge, 2*time.Minute))
}

func (e *engine) deferWarmup(c warmupCandidate, status string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.states[c.id]
	if !ok || s.Index != c.index || s.Stamp != c.stamp || !s.WarmupCycle.Equal(c.cycle) {
		return
	}
	s.WarmupStatus = status
	s.WarmupRetryAt = e.now().Add(time.Minute)
	e.states[c.id] = s
}

func (e *engine) setWarmupResult(c warmupCandidate, status string, attempted bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.states[c.id]
	if !ok || s.Index != c.index || (s.Snapshot != nil && s.Snapshot.warmupAccountKey != c.key) || (s.Snapshot == nil && s.WarmupWindow.key != c.key) {
		return
	}
	s.WarmupHandled = c.cycle
	s.WarmupStatus = status
	if attempted {
		s.WarmupSentAt = e.now()
		s.WarmupConfirmEnd, s.WarmupConfirmRead = time.Time{}, time.Time{}
		// The next scheduled quota read confirms whether a new window started.
	}
	e.states[c.id] = s
}

func (s *accountState) carryWarmupAcrossCredentialWrite(previous accountState) {
	// The cache and reset detector still reset; keep only attribution for a
	// completed/uncertain send. A fresh quota read must match this account key.
	s.WarmupWindow.key = previous.WarmupWindow.key
	s.WarmupHandled = previous.WarmupHandled
	s.WarmupSentAt = previous.WarmupSentAt
	s.WarmupConfirmEnd = previous.WarmupConfirmEnd
	s.WarmupConfirmRead = previous.WarmupConfirmRead
	s.WarmupStatus = previous.WarmupStatus
}

func (s *accountState) observeWarmup(snap quotaSnapshot, now time.Time, cfg config) {
	if s.WarmupWindow.key != "" && s.WarmupWindow.key != snap.warmupAccountKey {
		s.WarmupHandled, s.WarmupSentAt, s.WarmupConfirmEnd, s.WarmupConfirmRead, s.WarmupRetryAt = time.Time{}, time.Time{}, time.Time{}, time.Time{}, time.Time{}
		s.WarmupStatus = ""
	}
	s.WarmupCycle = s.WarmupWindow.observe(snap, now, min(cfg.MaxAge, 2*time.Minute))
	if snap.confirmedFree() {
		s.WarmupCycle = time.Time{}
		s.WarmupStatus = ""
		return
	}
	if !s.WarmupCycle.IsZero() && s.WarmupHandled.Before(s.WarmupCycle) && !now.Before(s.WarmupRetryAt) {
		if !snap.warmupSubscriptionOnly {
			s.WarmupStatus = "billing_unverified"
		} else if !warmupQuotaReady(snap, cfg.WarmupModel, now, min(cfg.MaxAge, 2*time.Minute)) {
			s.WarmupStatus = "quota_blocked"
		} else {
			s.WarmupStatus = "pending"
		}
	}
	if s.WarmupSentAt.IsZero() || s.WarmupStatus == "confirmed" || s.WarmupStatus == "refused" {
		return
	}
	if now.Sub(s.WarmupSentAt) > warmupMaxLateness {
		s.WarmupSentAt, s.WarmupConfirmEnd, s.WarmupConfirmRead = time.Time{}, time.Time{}, time.Time{}
		return
	}
	five := snap.FiveHour
	if !snap.ReadAt.After(s.WarmupSentAt) || !snap.warmupFiveHourReported || !snap.warmupAbsoluteReset || five == nil || five.Utilization == nil || five.ResetsAt == nil {
		return
	}
	end := *five.ResetsAt
	if end.Before(s.WarmupSentAt.Add(warmupWindow-time.Minute)) || end.After(s.WarmupSentAt.Add(warmupWindow+time.Minute)) {
		return
	}
	if !s.WarmupConfirmEnd.IsZero() && warmupClose(end.Sub(s.WarmupConfirmEnd), warmupResetJitter) {
		if snap.ReadAt.Sub(s.WarmupConfirmRead) < warmupMinSample {
			return
		}
		// A tiny probe can round to zero utilization. Two stable absolute
		// observations after dispatch distinguish it from a sliding idle timer.
		s.WarmupWindow.activeEnds = end
		s.WarmupStatus = "confirmed"
	} else {
		s.WarmupConfirmEnd, s.WarmupConfirmRead = end, snap.ReadAt
	}
}
