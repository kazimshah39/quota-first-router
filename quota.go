package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const codexUsageURL = "https://chatgpt.com/backend-api/wham/usage"
const maxUsageBytes = 128 * 1024

type credential struct {
	Type         string `json:"type"`
	AccessToken  string `json:"access_token"`
	AccountUUID  string `json:"account_uuid"`
	AccountID    string `json:"account_id"`
	Organization string `json:"organization_uuid"`
	Disabled     bool   `json:"disabled"`
	ProxyURL     string `json:"proxy_url"`
	BaseURL      string `json:"base_url"`
}

func (c credential) identity() string {
	value := strings.ToLower(strings.TrimSpace(c.Type)) + "\x00" + c.AccountUUID + "\x00" + c.AccountID + "\x00" + c.Organization
	if c.AccountUUID == "" && c.AccountID == "" {
		value += "\x00" + c.AccessToken
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type quotaWindow struct {
	Utilization *float64   `json:"utilization"`
	ResetsAt    *time.Time `json:"resets_at"`
}
type quotaSnapshot struct {
	Provider string       `json:"provider"`
	Weekly   quotaWindow  `json:"seven_day"`
	FiveHour *quotaWindow `json:"five_hour,omitempty"`
	// Some Codex plans expose only a long/monthly window, not a five-hour limit.
	FiveHourAbsent bool `json:"five_hour_absent,omitempty"`
	// Kept separately from reset ranking: all blocking windows matter.
	Windows  []quotaWindow            `json:"other_windows,omitempty"`
	Models   map[string][]quotaWindow `json:"model_windows,omitempty"`
	PlanType string                   `json:"plan_type,omitempty"`
	ReadAt   time.Time                `json:"read_at"`
	Identity string                   `json:"-"`
}

func parseCodexQuota(raw []byte, now time.Time) (quotaSnapshot, error) {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return quotaSnapshot{}, errors.New("invalid_quota_json")
	}
	rate, ok := mapValue(doc, "rate_limit", "rateLimit")
	if !ok {
		return quotaSnapshot{}, errors.New("invalid_codex_rate_limit")
	}
	s := quotaSnapshot{Provider: "codex", ReadAt: now, PlanType: stringValue(doc, "plan_type", "planType"), Models: map[string][]quotaWindow{}}
	var windows []quotaWindow
	for i, names := range [][2]string{{"primary_window", "primaryWindow"}, {"secondary_window", "secondaryWindow"}} {
		wraw, exists := mapValue(rate, names[0], names[1])
		if !exists {
			continue
		}
		w, kind, valid := parseCodexWindow(wraw, now, i)
		if !valid {
			return quotaSnapshot{}, errors.New("invalid_codex_window")
		}
		if kind == "five_hour" {
			s.FiveHour = &w
		} else {
			if s.Weekly.Utilization == nil {
				s.Weekly = w
			} else {
				s.Windows = append(s.Windows, w)
			}
		}
		windows = append(windows, w)
	}
	if len(windows) == 0 {
		return quotaSnapshot{}, errors.New("invalid_codex_windows")
	}
	// A global denial must not be lost if the response has no exhausted window.
	allowed, ha := boolValue(rate, "allowed")
	reached, hr := boolValue(rate, "limit_reached", "limitReached")
	s.FiveHourAbsent = s.FiveHour == nil && ha && allowed && s.Weekly.Utilization != nil
	if (ha && !allowed) || (hr && reached) {
		used := 100.0
		s.Windows = append(s.Windows, quotaWindow{Utilization: &used, ResetsAt: latestReset(windows)})
	}
	if additional, ok := doc["additional_rate_limits"].([]any); ok {
		for _, v := range additional {
			m, ok := v.(map[string]any)
			if !ok {
				return quotaSnapshot{}, errors.New("invalid_model_windows")
			}
			name := strings.ToLower(stringValue(m, "limit_name", "metered_feature", "id"))
			if name == "" {
				continue
			}
			r, exists := mapValue(m, "rate_limit", "rateLimit")
			if !exists {
				r = m
			}
			var list []quotaWindow
			for i, key := range []string{"primary_window", "secondary_window"} {
				wr, exists := mapValue(r, key)
				if !exists {
					continue
				}
				w, _, valid := parseCodexWindow(wr, now, i)
				if !valid {
					return quotaSnapshot{}, errors.New("invalid_model_windows")
				}
				list = append(list, w)
			}
			a, ha := boolValue(r, "allowed")
			l, hl := boolValue(r, "limit_reached")
			if (ha && !a) || (hl && l) {
				used := 100.0
				list = append(list, quotaWindow{Utilization: &used, ResetsAt: latestReset(list)})
			}
			s.Models[name] = list
		}
	}
	return s, nil
}
func latestReset(ws []quotaWindow) *time.Time {
	var latest *time.Time
	for _, w := range ws {
		if w.ResetsAt != nil && (latest == nil || w.ResetsAt.After(*latest)) {
			t := *w.ResetsAt
			latest = &t
		}
	}
	return latest
}
func parseCodexWindow(raw map[string]any, now time.Time, position int) (quotaWindow, string, bool) {
	used, ok := numberValue(raw, "used_percent", "usedPercent")
	if !ok || math.IsNaN(used) || math.IsInf(used, 0) || used < 0 || used > 100 {
		return quotaWindow{}, "", false
	}
	seconds, has := numberValue(raw, "limit_window_seconds", "limitWindowSeconds")
	kind := "long"
	if has && seconds == 18000 || !has && position == 0 {
		kind = "five_hour"
	} else if has && (seconds <= 0 || seconds > 2678400) {
		return quotaWindow{}, "", false
	}
	if a, has := boolValue(raw, "allowed"); has && !a {
		used = 100
	}
	if r, has := boolValue(raw, "limit_reached", "limitReached"); has && r {
		used = 100
	}
	w := quotaWindow{Utilization: &used}
	if reset, ok := codexResetAt(raw, now); ok {
		w.ResetsAt = &reset
	}
	return w, kind, true
}
func codexResetAt(raw map[string]any, now time.Time) (time.Time, bool) {
	if n, ok := numberValue(raw, "reset_at", "resetAt"); ok && n > 0 && n < 1e12 {
		return time.Unix(int64(n), 0).UTC(), true
	}
	if n, ok := numberValue(raw, "reset_after_seconds", "resetAfterSeconds"); ok && n >= 0 && n <= 2678400 {
		return now.Add(time.Duration(n * float64(time.Second))), true
	}
	return time.Time{}, false
}
func mapValue(m map[string]any, keys ...string) (map[string]any, bool) {
	for _, k := range keys {
		for a, v := range m {
			if strings.EqualFold(a, k) {
				r, ok := v.(map[string]any)
				return r, ok
			}
		}
	}
	return nil, false
}
func stringValue(m map[string]any, keys ...string) string {
	for _, k := range keys {
		for a, v := range m {
			if strings.EqualFold(a, k) {
				r, _ := v.(string)
				return r
			}
		}
	}
	return ""
}
func numberValue(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		for a, v := range m {
			if strings.EqualFold(a, k) {
				r, ok := v.(float64)
				return r, ok
			}
		}
	}
	return 0, false
}
func boolValue(m map[string]any, keys ...string) (bool, bool) {
	for _, k := range keys {
		for a, v := range m {
			if strings.EqualFold(a, k) {
				r, ok := v.(bool)
				return r, ok
			}
		}
	}
	return false, false
}
func (s quotaSnapshot) availability(model string, now time.Time, maxAge time.Duration) string {
	if s.ReadAt.IsZero() || now.Before(s.ReadAt) || now.Sub(s.ReadAt) >= maxAge {
		return "stale"
	}
	windows := append([]quotaWindow{}, s.Windows...)
	if s.Weekly.Utilization != nil {
		windows = append(windows, s.Weekly)
	}
	if s.FiveHour != nil {
		windows = append(windows, *s.FiveHour)
	}
	model = strings.ToLower(model)
	for name, ws := range s.Models {
		if model == name || strings.Contains(model, name) {
			windows = append(windows, ws...)
		}
	}
	pending := false
	for _, w := range windows {
		// A second exhausted window still blocks even if another window has elapsed.
		if w.ResetsAt != nil && !w.ResetsAt.After(now) {
			pending = true
			continue
		}
		if w.Utilization != nil && *w.Utilization >= 100 {
			return "exhausted"
		}
	}
	if pending {
		return "reset_pending_refresh"
	}
	if s.FiveHour == nil && !s.FiveHourAbsent {
		return "unknown"
	}
	return "ready"
}

type quotaFetcher interface {
	Fetch(context.Context, credential) (quotaSnapshot, error)
}
type httpQuotaFetcher struct {
	client   *http.Client
	endpoint string
}

func newHTTPFetcher() *httpQuotaFetcher {
	return &httpQuotaFetcher{client: &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, endpoint: codexUsageURL}
}

type quotaError struct {
	code       string
	retryAfter time.Duration
}

func (e *quotaError) Error() string { return e.code }
func (f *httpQuotaFetcher) Fetch(ctx context.Context, c credential) (quotaSnapshot, error) {
	if c.Type != "codex" || c.Disabled || strings.TrimSpace(c.AccessToken) == "" {
		return quotaSnapshot{}, errors.New("ineligible_credential")
	}
	if c.BaseURL != "" {
		return quotaSnapshot{}, errors.New("custom_base_url_unsupported")
	}
	client := f.client
	if c.ProxyURL != "" {
		tr := f.client.Transport.(*http.Transport).Clone()
		if c.ProxyURL == "direct" {
			tr.Proxy = nil
		} else {
			u, err := url.Parse(c.ProxyURL)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
				return quotaSnapshot{}, errors.New("invalid_proxy_url")
			}
			tr.Proxy = http.ProxyURL(u)
		}
		defer tr.CloseIdleConnections()
		copyClient := *f.client
		copyClient.Transport = tr
		client = &copyClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.endpoint, nil)
	if err != nil {
		return quotaSnapshot{}, errors.New("quota_request_failed")
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codex_cli_rs/0.76.0")
	if c.AccountID != "" {
		req.Header.Set("Chatgpt-Account-Id", c.AccountID)
	}
	resp, err := client.Do(req)
	if err != nil {
		return quotaSnapshot{}, errors.New("quota_transport_failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return quotaSnapshot{}, &quotaError{code: fmt.Sprintf("quota_http_%d", resp.StatusCode), retryAfter: retryAfter(resp.Header.Get("Retry-After"), time.Now())}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUsageBytes+1))
	if err != nil || len(body) > maxUsageBytes {
		return quotaSnapshot{}, errors.New("invalid_quota_body")
	}
	s, err := parseCodexQuota(body, time.Now().UTC())
	if err != nil {
		return quotaSnapshot{}, err
	}
	s.Identity = c.identity()
	return s, nil
}
func retryAfter(raw string, now time.Time) time.Duration {
	var delay time.Duration
	if n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil {
		if n > 3600 {
			return time.Hour
		}
		if n > 0 {
			delay = time.Duration(n) * time.Second
		}
	} else if t, err := http.ParseTime(raw); err == nil {
		delay = t.Sub(now)
	}
	return min(time.Hour, max(0, delay))
}
