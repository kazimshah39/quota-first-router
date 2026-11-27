package main

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

//go:embed web/monitor.html web/monitor.css web/monitor.js
var monitorFiles embed.FS

const monitorPath = "/v0/management/plugins/" + pluginID + "/monitor"
const resourcePath = "/v0/resource/plugins/" + pluginID + "/monitor"
const monitorAccountLimit = 1000

type monitorAccount struct {
	ID                    string         `json:"id"`
	Email                 string         `json:"email,omitempty"`
	Priority              *int           `json:"priority,omitempty"`
	Index                 string         `json:"auth_index"`
	Error                 string         `json:"last_error,omitempty"`
	Failures              int            `json:"consecutive_failures"`
	NextRefresh           time.Time      `json:"next_refresh"`
	Quota                 *quotaSnapshot `json:"quota,omitempty"`
	QuotaDetailsTruncated int            `json:"quota_details_truncated,omitempty"`
	Availability          string         `json:"availability"`
	WarmupStatus          string         `json:"warmup_status,omitempty"`
}
type monitorSnapshot struct {
	Version           string              `json:"version"`
	ServerTime        time.Time           `json:"server_time"`
	Mode              string              `json:"mode"`
	State             string              `json:"state"`
	MaxAgeSeconds     float64             `json:"max_age_seconds"`
	PollInterval      string              `json:"poll_interval"`
	ListError         string              `json:"list_error,omitempty"`
	WorkerFailed      bool                `json:"worker_failed"`
	WarmupError       string              `json:"warmup_error,omitempty"`
	Accounts          []monitorAccount    `json:"accounts"`
	TotalAccounts     int                 `json:"total_accounts"`
	AccountsTruncated int                 `json:"accounts_truncated"`
	FreshAccounts     int                 `json:"fresh_accounts"`
	Picks             uint64              `json:"picks"`
	Routed            uint64              `json:"routed"`
	History           observationSnapshot `json:"history"`
}

func (e *engine) monitor() monitorSnapshot {
	now := e.now()
	s := monitorSnapshot{Version: pluginVersion, ServerTime: now, Mode: "active", MaxAgeSeconds: e.cfg.MaxAge.Seconds(), PollInterval: e.cfg.PollInterval.String(), Accounts: []monitorAccount{}, WorkerFailed: e.failed.Load(), Picks: e.picks.Load(), Routed: e.routed.Load()}
	e.mu.RLock()
	s.ListError = e.listErr
	s.WarmupError = e.warmupError
	s.TotalAccounts = len(e.states)
	ids := make([]string, 0, len(e.states))
	for id := range e.states {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids[:min(len(ids), monitorAccountLimit)] {
		a := e.states[id]
		s.Accounts = append(s.Accounts, monitorAccount{ID: id, Email: a.Email, Priority: a.Priority, Index: a.Index, Error: a.LastError, Failures: a.Failures, NextRefresh: a.NextRefresh, Quota: a.Snapshot, Availability: "unknown", WarmupStatus: a.WarmupStatus})
	}
	e.mu.RUnlock()
	// Quota snapshots are immutable after publication by refreshQuota. Copy and
	// sanitize monitor-only data outside the scheduler lock, never alter policy.
	for i := range s.Accounts {
		a := &s.Accounts[i]
		a.ID, a.Index = safeLabel(a.ID), safeLabel(a.Index)
		if a.Quota != nil {
			a.Availability = a.Quota.availability("", now, e.cfg.MaxAge)
			if !now.Before(a.Quota.ReadAt) && now.Sub(a.Quota.ReadAt) < e.cfg.MaxAge {
				s.FreshAccounts++
			}
		}
		a.Quota, a.QuotaDetailsTruncated = monitorQuota(a.Quota)
	}
	s.AccountsTruncated = max(0, s.TotalAccounts-len(s.Accounts))
	s.History = e.observations.snapshot()
	s.State = "healthy"
	if s.TotalAccounts == 0 {
		s.State = "no_accounts"
	} else if s.FreshAccounts == 0 {
		s.State = "starting"
	}
	for _, a := range s.Accounts {
		if a.Error != "" || a.Availability == "stale" || a.Availability == "reset_pending_refresh" || a.Availability == "unknown" && a.Quota != nil {
			s.State = "degraded"
		}
	}
	if s.AccountsTruncated > 0 || s.ListError != "" {
		s.State = "degraded"
	}
	if s.WorkerFailed {
		s.State = "failed"
	}
	return s
}

// Only accept a bounded, plain mailbox from CPA's roster. Never expose the
// credential's other metadata or attempt to infer a mailbox from its tokens.
func monitorEmail(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 512 || safeLabel(value) != value {
		return ""
	}
	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Name != "" || addr.Address != value {
		return ""
	}
	return value
}

func monitorPlan(plan string) string {
	switch plan = strings.ToLower(strings.TrimSpace(plan)); plan {
	case "free", "go", "plus", "pro", "team", "business", "enterprise", "edu":
		return plan
	default:
		return ""
	}
}

// Bound provider-supplied model labels/windows independently of routing data.
// Only known subscription codes are exposed; identity and unknown labels are omitted.
func monitorQuota(source *quotaSnapshot) (*quotaSnapshot, int) {
	if source == nil {
		return nil, 0
	}
	q := *source
	q.PlanType, q.Identity = monitorPlan(source.PlanType), ""
	q.Provider = "codex"
	q.Windows = append([]quotaWindow(nil), source.Windows[:min(len(source.Windows), candidateLimit)]...)
	omitted := len(source.Windows) - len(q.Windows)
	q.Models = map[string][]quotaWindow{}
	names := make([]string, 0, len(source.Models))
	for name := range source.Models {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		windows := source.Models[name]
		label := safeLabel(name)
		if len(q.Models) >= candidateLimit || len(windows) == 0 {
			omitted += len(windows)
			continue
		}
		if _, exists := q.Models[label]; exists {
			omitted += len(windows)
			continue
		}
		q.Models[label] = append([]quotaWindow(nil), windows[:min(len(windows), candidateLimit)]...)
		omitted += len(windows) - len(q.Models[label])
	}
	return &q, omitted
}

func (p *pluginRuntime) management(req pluginapi.ManagementRequest) pluginapi.ManagementResponse {
	headers := http.Header{"Cache-Control": {"no-store"}, "X-Content-Type-Options": {"nosniff"}, "Referrer-Policy": {"no-referrer"}}
	if req.Method != http.MethodGet {
		return pluginapi.ManagementResponse{StatusCode: 405, Headers: headers}
	}
	for path, file := range map[string]string{resourcePath: "web/monitor.html", resourcePath + ".css": "web/monitor.css", resourcePath + ".js": "web/monitor.js"} {
		if req.Path == path {
			raw, _ := monitorFiles.ReadFile(file)
			contentType := "text/html; charset=utf-8"
			if file == "web/monitor.css" {
				contentType = "text/css; charset=utf-8"
			}
			if file == "web/monitor.js" {
				contentType = "text/javascript; charset=utf-8"
			}
			headers.Set("Content-Type", contentType)
			headers.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
			return pluginapi.ManagementResponse{StatusCode: 200, Headers: headers, Body: raw}
		}
	}
	if req.Path != statusPath && req.Path != monitorPath {
		return pluginapi.ManagementResponse{StatusCode: 404, Headers: headers}
	}
	var result any = map[string]string{"state": "stopped"}
	if e := p.active.Load(); e != nil {
		if req.Path == monitorPath {
			result = e.monitor()
		} else {
			result = e.status()
		}
	}
	raw, err := json.Marshal(result)
	if err != nil || (req.Path == monitorPath && len(raw) > 8*1024*1024) {
		return pluginapi.ManagementResponse{StatusCode: 503, Headers: headers, Body: []byte(`{"error":"monitor_snapshot_unavailable"}`)}
	}
	headers.Set("Content-Type", "application/json")
	return pluginapi.ManagementResponse{StatusCode: 200, Headers: headers, Body: raw}
}
