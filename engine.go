package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const reconcileInterval = 30 * time.Second

type authHost interface {
	List() ([]pluginapi.HostAuthFileEntry, error)
	Get(string) (json.RawMessage, error)
}

type accountState struct {
	Index       string         `json:"auth_index"`
	Provider    string         `json:"provider"`
	Stamp       string         `json:"-"`
	Email       string         `json:"-"`
	Priority    *int           `json:"-"`
	Snapshot    *quotaSnapshot `json:"quota,omitempty"`
	LastError   string         `json:"last_error,omitempty"`
	NextRefresh time.Time      `json:"next_refresh"`
	Failures    int            `json:"consecutive_failures"`
}

type engine struct {
	host         authHost
	fetcher      quotaFetcher
	cfg          config
	now          func() time.Time
	mu           sync.RWMutex
	states       map[string]accountState
	listErr      string
	last         decision
	picks        atomic.Uint64
	routed       atomic.Uint64
	failed       atomic.Bool
	refresh      sync.Mutex
	observations *observation
}

func newEngine(host authHost, fetcher quotaFetcher, cfg config) *engine {
	return &engine{host: host, fetcher: fetcher, cfg: cfg, now: time.Now, states: make(map[string]accountState), observations: newObservation(time.Now())}
}

func (e *engine) run(ctx context.Context) {
	e.refreshQuota(ctx)
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.refreshQuota(ctx)
		}
	}
}

func (e *engine) refreshQuota(ctx context.Context) {
	e.refresh.Lock()
	defer e.refresh.Unlock()
	if ctx.Err() != nil {
		return
	}
	entries, err := e.host.List()
	if err != nil {
		e.mu.Lock()
		e.listErr = "credential_list_failed"
		e.mu.Unlock()
		e.observations.incident("", "roster", "credential_list_failed", e.now())
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	eligible := make(map[string]bool)
	e.mu.Lock()
	previousListError := e.listErr
	e.listErr = ""
	for _, entry := range entries {
		provider := entry.Provider
		if provider == "" {
			provider = entry.Type
		}
		provider = strings.ToLower(strings.TrimSpace(provider))
		if provider != "codex" || entry.Disabled || entry.Status == "disabled" || entry.RuntimeOnly || entry.ID == "" || entry.AuthIndex == "" {
			continue
		}
		eligible[entry.ID] = true
		state := e.states[entry.ID]
		stamp := fmt.Sprintf("%q:%q:%q:%d:%d", entry.Path, entry.Account, entry.Email, entry.Size, entry.ModTime.UnixNano())
		if state.Index != entry.AuthIndex || state.Provider != provider || state.Stamp != stamp {
			state = accountState{Index: entry.AuthIndex, Provider: provider, Stamp: stamp}
		}
		state.Email = monitorEmail(entry.Email)
		priority := entry.Priority
		state.Priority = &priority
		e.states[entry.ID] = state
	}
	for id := range e.states {
		if !eligible[id] {
			delete(e.states, id)
		}
	}
	e.mu.Unlock()
	if previousListError != "" {
		e.observations.incident("", "roster", "recovered", e.now())
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		if !eligible[entry.ID] {
			continue
		}
		provider := entry.Provider
		if provider == "" {
			provider = entry.Type
		}
		provider = strings.ToLower(strings.TrimSpace(provider))
		e.mu.RLock()
		state := e.states[entry.ID]
		e.mu.RUnlock()
		if e.now().Before(state.NextRefresh) {
			continue
		}
		previousError := state.LastError
		snap, err := e.readQuota(ctx, entry.AuthIndex, provider)
		if ctx.Err() != nil {
			return
		}
		now := e.now()
		if err != nil {
			state.Failures++
			state.LastError = "quota_refresh_failed"
			for _, code := range []string{"custom_base_url_unsupported", "invalid_proxy_url", "credential_read_failed", "ineligible_credential"} {
				if err.Error() == code {
					state.LastError = code
				}
			}
			var qerr *quotaError
			delay := time.Minute * time.Duration(1<<min(state.Failures-1, 4))
			if errors.As(err, &qerr) {
				state.LastError = qerr.code
				delay = max(delay, qerr.retryAfter)
				if qerr.code == "quota_http_401" || qerr.code == "quota_http_403" {
					state.Snapshot = nil
				}
			}
			state.NextRefresh = now.Add(delay)
			e.observations.incident(entry.ID, "quota", state.LastError, now)
		} else {
			state.Snapshot = &snap
			state.LastError = ""
			state.Failures = 0
			state.NextRefresh = nextRefresh(snap, now, e.cfg.PollInterval)
			if previousError != "" {
				e.observations.incident(entry.ID, "quota", "recovered", now)
			}
		}
		e.mu.Lock()
		e.states[entry.ID] = state
		e.mu.Unlock()
	}
}

func (e *engine) readQuota(ctx context.Context, index, provider string) (quotaSnapshot, error) {
	raw, err := e.host.Get(index)
	if err != nil {
		return quotaSnapshot{}, errors.New("credential_read_failed")
	}
	var cred credential
	if err := json.Unmarshal(raw, &cred); err != nil {
		return quotaSnapshot{}, errors.New("credential_decode_failed")
	}
	if strings.TrimSpace(cred.Type) == "" {
		cred.Type = provider
	}
	if cred.ProxyURL == "" {
		cred.ProxyURL = e.cfg.ProxyURL
	}
	ctx, cancel := context.WithTimeout(ctx, e.cfg.RequestTimeout)
	defer cancel()
	return e.fetcher.Fetch(ctx, cred)
}

func nextRefresh(s quotaSnapshot, now time.Time, interval time.Duration) time.Time {
	next := now.Add(interval)
	windows := []*quotaWindow{&s.Weekly, s.FiveHour}
	for i := range s.Windows {
		windows = append(windows, &s.Windows[i])
	}
	for _, ws := range s.Models {
		for i := range ws {
			windows = append(windows, &ws[i])
		}
	}
	for _, w := range windows {
		if w != nil && w.ResetsAt != nil && w.ResetsAt.Add(5*time.Second).Before(next) {
			next = w.ResetsAt.Add(5 * time.Second)
		}
	}
	return maxTime(next, now.Add(reconcileInterval))
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func providerOnly(req pluginapi.SchedulerPickRequest, provider string) bool {
	if req.Provider != provider && !(req.Provider == "" && len(req.Providers) == 1 && req.Providers[0] == provider) {
		return false
	}
	for _, p := range req.Providers {
		if p != provider {
			return false
		}
	}
	for _, c := range req.Candidates {
		if c.Provider != provider {
			return false
		}
	}
	return true
}

func candidateIdentity(c pluginapi.SchedulerAuthCandidate) string {
	accountID, _ := c.Metadata["account_id"].(string)
	account, _ := c.Metadata["account_uuid"].(string)
	org, _ := c.Metadata["organization_uuid"].(string)
	token, _ := c.Metadata["access_token"].(string)
	if accountID == "" && account == "" && token == "" {
		return ""
	}
	return (credential{Type: c.Provider, AccountUUID: account, AccountID: accountID, Organization: org, AccessToken: token}).identity()
}

func (e *engine) pick(req pluginapi.SchedulerPickRequest) pluginapi.SchedulerPickResponse {
	if e.failed.Load() {
		return pluginapi.SchedulerPickResponse{}
	}
	e.mu.RLock()
	d := choose(req, e.states, e.now(), e.cfg.MaxAge)
	e.mu.RUnlock()
	e.mu.Lock()
	e.last = d
	e.mu.Unlock()
	e.observations.recordDecision(d, req.Model, e.cfg.Mode, req.Stream, e.now())
	e.picks.Add(1)
	if e.cfg.Mode != "active" {
		return pluginapi.SchedulerPickResponse{}
	}
	if d.AuthID == "" {
		return pluginapi.SchedulerPickResponse{ExcludeCandidates: d.Excluded}
	}
	e.routed.Add(1)
	return pluginapi.SchedulerPickResponse{AuthID: d.AuthID, Handled: true}
}

func (e *engine) status() any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	accounts := make(map[string]accountState, len(e.states))
	for id, state := range e.states {
		accounts[id] = state
	}
	return struct {
		Version      string                  `json:"version"`
		Policy       string                  `json:"selection_policy"`
		Mode         string                  `json:"mode"`
		PollInterval string                  `json:"poll_interval"`
		MaxAge       string                  `json:"max_age"`
		Accounts     map[string]accountState `json:"accounts"`
		ListError    string                  `json:"list_error,omitempty"`
		LastDecision decision                `json:"last_decision"`
		Picks        uint64                  `json:"picks"`
		Routed       uint64                  `json:"routed"`
		WorkerFailed bool                    `json:"worker_failed"`
	}{pluginVersion, "tier_then_five_hour_reset", e.cfg.Mode, e.cfg.PollInterval.String(), e.cfg.MaxAge.String(), accounts, e.listErr, e.last, e.picks.Load(), e.routed.Load(), e.failed.Load()}
}
