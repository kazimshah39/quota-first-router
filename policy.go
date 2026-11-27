package main

import (
	"sort"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type candidateNote struct {
	ID       string     `json:"auth_id"`
	Priority int        `json:"priority"`
	Reason   string     `json:"reason"`
	Reset    *time.Time `json:"five_hour_reset,omitempty"`
}

type decision struct {
	AuthID     string          `json:"auth_id,omitempty"`
	Priority   int             `json:"priority"`
	Reason     string          `json:"reason"`
	Reset      *time.Time      `json:"five_hour_reset,omitempty"`
	Excluded   []string        `json:"excluded,omitempty"`
	Candidates []candidateNote `json:"candidates,omitempty"`
}

// choose never assumes unknown data means zero quota. Unknown or stale data in
// the highest usable tier delegates to CPA, preserving that tier's precedence.
func choose(req pluginapi.SchedulerPickRequest, states map[string]accountState, now time.Time, maxAge time.Duration) decision {
	d := decision{Reason: "provider_not_exclusively_supported"}
	if !providerOnly(req, "codex") {
		return d
	}
	candidates := append([]pluginapi.SchedulerAuthCandidate(nil), req.Candidates...)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		return candidates[i].ID < candidates[j].ID
	})
	var tierSet, unknown bool
	var tier int
	var best string
	var bestReset *time.Time
	for _, c := range candidates {
		if c.ID == "" {
			continue
		}
		n := candidateNote{ID: c.ID, Priority: c.Priority}
		disabled, _ := c.Metadata["disabled"].(bool)
		if disabled || strings.EqualFold(c.Status, "disabled") {
			n.Reason = "disabled"
			d.Excluded = append(d.Excluded, c.ID)
			d.Candidates = append(d.Candidates, n)
			continue
		}
		state := states[c.ID]
		s := state.Snapshot
		n.Reason = "unknown"
		if s != nil && state.Provider == "codex" {
			identity := candidateIdentity(c)
			if identity != "" && identity != s.Identity {
				n.Reason = "identity_changed"
			} else if s.confirmedFree() {
				n.Reason = "free_plan_excluded"
			} else {
				n.Reason = s.availability(req.Model, now, maxAge)
			}
			if s.FiveHour != nil {
				n.Reset = s.FiveHour.ResetsAt
			}
		}
		d.Candidates = append(d.Candidates, n)
		if n.Reason == "exhausted" || n.Reason == "free_plan_excluded" {
			d.Excluded = append(d.Excluded, c.ID)
			continue
		}
		if !tierSet {
			tier = c.Priority
			tierSet = true
		}
		if c.Priority != tier {
			continue
		}
		if n.Reason != "ready" {
			unknown = true
			continue
		}
		if best == "" || (n.Reset != nil && (bestReset == nil || n.Reset.Before(*bestReset))) {
			best = c.ID
			bestReset = n.Reset
		}
	}
	d.Priority = tier
	if !tierSet {
		d.Reason = "no_usable_quota"
		return d
	}
	if unknown {
		d.Reason = "tier_quota_unknown_delegate"
		return d
	}
	if best == "" {
		d.Reason = "no_known_usable_quota"
		return d
	}
	d.AuthID = best
	d.Reset = bestReset
	d.Reason = "earliest_five_hour_reset"
	if bestReset == nil {
		d.Reason = "usable_unknown_reset"
	}
	return d
}
