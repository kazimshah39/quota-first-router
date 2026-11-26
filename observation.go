package main

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

const historyCapacity = 200
const candidateLimit = 100

// Observation records intentionally contain no request headers, metadata or bodies.
type decisionRecord struct {
	Sequence  uint64    `json:"sequence"`
	At        time.Time `json:"at"`
	Model     string    `json:"model"`
	Stream    bool      `json:"stream"`
	Mode      string    `json:"mode"`
	Decision  decision  `json:"decision"`
	Truncated int       `json:"truncated"`
}
type incidentRecord struct {
	Sequence uint64    `json:"sequence"`
	At       time.Time `json:"at"`
	LastAt   time.Time `json:"last_at"`
	Account  string    `json:"account,omitempty"`
	Kind     string    `json:"kind"`
	Code     string    `json:"code"`
	Count    uint64    `json:"count"`
}
type observation struct {
	mu                                                sync.Mutex
	started                                           time.Time
	epoch                                             string
	sequence                                          uint64
	decisions                                         [historyCapacity]decisionRecord
	decisionCount, decisionNext                       int
	incidents                                         [historyCapacity]incidentRecord
	incidentCount, incidentNext                       int
	decisionEvictions, incidentEvictions, truncations uint64
	active                                            map[string]int
}

var observationEpoch atomic.Uint64

func newObservation(now time.Time) *observation {
	return &observation{started: now, epoch: fmt.Sprintf("%d-%d", now.UnixNano(), observationEpoch.Add(1)), active: map[string]int{}}
}
func safeLabel(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > 128 {
		return string(runes[:128]) + "…"
	}
	return value
}
func boundedDecision(d decision) (decision, int) {
	out := d
	out.AuthID = safeLabel(d.AuthID)
	out.Reason = safeLabel(d.Reason)
	out.Excluded = nil
	out.Candidates = nil
	truncated := max(0, len(d.Candidates)-candidateLimit) + max(0, len(d.Excluded)-candidateLimit)
	for _, c := range d.Candidates[:min(len(d.Candidates), candidateLimit)] {
		c.ID = safeLabel(c.ID)
		c.Reason = safeLabel(c.Reason)
		out.Candidates = append(out.Candidates, c)
	}
	for _, id := range d.Excluded[:min(len(d.Excluded), candidateLimit)] {
		out.Excluded = append(out.Excluded, safeLabel(id))
	}
	return out, truncated
}
func (o *observation) recordDecision(d decision, model, mode string, stream bool, now time.Time) {
	bounded, truncated := boundedDecision(d)
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sequence++
	o.decisions[o.decisionNext] = decisionRecord{o.sequence, now, safeLabel(model), stream, mode, bounded, truncated}
	o.decisionNext = (o.decisionNext + 1) % historyCapacity
	if o.decisionCount < historyCapacity {
		o.decisionCount++
	} else {
		o.decisionEvictions++
	}
	o.truncations += uint64(truncated)
}
func (o *observation) incident(account, kind, code string, now time.Time) {
	// Only internal fixed codes may reach this method. Coalescing keys use original
	// account identifiers to avoid conflating labels that share a truncated prefix.
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(kind+"\x00"+account)))
	o.mu.Lock()
	defer o.mu.Unlock()
	if index, ok := o.active[key]; ok {
		r := &o.incidents[index]
		if r.Code == code {
			r.Count++
			r.LastAt = now
			return
		}
	}
	o.sequence++
	index := o.incidentNext
	// Drop keys whose slots are being evicted (map stays bounded with the ring).
	for k, i := range o.active {
		if i == index {
			delete(o.active, k)
		}
	}
	o.incidents[index] = incidentRecord{o.sequence, now, now, safeLabel(account), kind, code, 1}
	o.active[key] = index
	o.incidentNext = (index + 1) % historyCapacity
	if o.incidentCount < historyCapacity {
		o.incidentCount++
	} else {
		o.incidentEvictions++
	}
}
func (o *observation) snapshot() observationSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := observationSnapshot{Started: o.started, Epoch: o.epoch, DecisionEvictions: o.decisionEvictions, IncidentEvictions: o.incidentEvictions, Truncations: o.truncations, Decisions: []decisionRecord{}, Incidents: []incidentRecord{}}
	for i := 0; i < o.decisionCount; i++ {
		r := o.decisions[(o.decisionNext-1-i+historyCapacity)%historyCapacity]
		r.Decision.Candidates = append([]candidateNote(nil), r.Decision.Candidates...)
		r.Decision.Excluded = append([]string(nil), r.Decision.Excluded...)
		s.Decisions = append(s.Decisions, r)
	}
	for i := 0; i < o.incidentCount; i++ {
		s.Incidents = append(s.Incidents, o.incidents[(o.incidentNext-1-i+historyCapacity)%historyCapacity])
	}
	return s
}

type observationSnapshot struct {
	Started           time.Time        `json:"started_at"`
	Epoch             string           `json:"epoch"`
	Decisions         []decisionRecord `json:"decisions"`
	Incidents         []incidentRecord `json:"incidents"`
	DecisionEvictions uint64           `json:"decision_evictions"`
	IncidentEvictions uint64           `json:"incident_evictions"`
	Truncations       uint64           `json:"candidate_truncations"`
}

func (e *engine) markFailed() {
	if e.failed.CompareAndSwap(false, true) {
		e.observations.incident("", "worker", "worker_failed", e.now())
	}
}
