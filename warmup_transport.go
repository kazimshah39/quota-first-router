package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const warmupKeyFile = "cpa-management.key"
const warmupAuthFiles = "/v0/management/auth-files"
const warmupVerify = "/v8/management/credentials/verify"

const warmupHTTPMax = 8 * 1024 * 1024

var errWarmupTransport = errors.New("warmup_cpa_unavailable")

type warmupPinnedAccount struct{ name, index string }

type warmupCPA struct {
	client   *http.Client
	endpoint string
	key      string
}

func newWarmupCPA(endpoint string, root *os.Root) (*warmupCPA, error) {
	if !validWarmupEndpoint(endpoint) {
		return nil, errWarmupTransport
	}
	file, err := openWarmupRegular(root, warmupKeyFile)
	if err != nil {
		return nil, errWarmupTransport
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 1025))
	key := strings.TrimSpace(string(raw))
	if err != nil || len(raw) > 1024 || len(key) == 0 || strings.IndexFunc(key, func(r rune) bool { return r < 0x21 || r > 0x7e }) >= 0 {
		return nil, errWarmupTransport
	}
	tr := &http.Transport{MaxIdleConns: 1, MaxIdleConnsPerHost: 1, MaxConnsPerHost: 1, MaxResponseHeaderBytes: 64 * 1024}
	return &warmupCPA{endpoint: endpoint, key: key, client: &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *warmupCPA) call(ctx context.Context, path string, input, result any) (int, error) {
	method := http.MethodGet
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return 0, errWarmupTransport
		}
		body = bytes.NewReader(raw)
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return 0, errWarmupTransport
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, errWarmupTransport
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, errWarmupTransport
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, warmupHTTPMax+1))
	if err != nil || len(raw) > warmupHTTPMax || json.Unmarshal(raw, result) != nil {
		return 0, errWarmupTransport
	}
	return resp.StatusCode, nil
}

func (c *warmupCPA) eligible(ctx context.Context, a warmupPinnedAccount, model string) (string, error) {
	var files struct {
		Files []struct {
			Name        string             `json:"name"`
			Index       string             `json:"auth_index"`
			Provider    string             `json:"provider"`
			AccountType string             `json:"account_type"`
			Disabled    *bool              `json:"disabled"`
			Unavailable *bool              `json:"unavailable"`
			RuntimeOnly *bool              `json:"runtime_only"`
			Status      string             `json:"status"`
			Cooldowns   *[]json.RawMessage `json:"cooldowns"`
		} `json:"files"`
	}
	if _, err := c.call(ctx, warmupAuthFiles, nil, &files); err != nil {
		return "cpa_unavailable", err
	}
	var count int
	for _, r := range files.Files {
		if r.Name != a.name || r.Index != a.index {
			continue
		}
		if r.Provider != "codex" || r.AccountType != "oauth" || r.Status != "active" || r.Disabled == nil || *r.Disabled || r.Unavailable == nil || *r.Unavailable || r.RuntimeOnly == nil || *r.RuntimeOnly || r.Cooldowns == nil || len(*r.Cooldowns) != 0 {
			return "credential_unavailable", nil
		}
		count++
	}
	if count != 1 {
		return "credential_unavailable", nil
	}
	var supported struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if _, err := c.call(ctx, warmupAuthFiles+"/models?"+url.Values{"name": {a.name}}.Encode(), nil, &supported); err != nil {
		return "cpa_unavailable", err
	}
	for _, registered := range supported.Models {
		if registered.ID == model {
			return "", nil
		}
	}
	return "model_unavailable", nil
}

func (c *warmupCPA) probe(ctx context.Context, a warmupPinnedAccount, model string) string {
	var r struct {
		Status   string `json:"status"`
		Verified bool   `json:"verified"`
	}
	status, err := c.call(ctx, warmupVerify, map[string]string{"name": a.name, "auth_index": a.index, "model": model}, &r)
	if err == nil && r.Status == "ok" && r.Verified {
		return "sent"
	}
	if status == 400 || status == 401 || status == 403 || status == 404 || status == 409 || status == 429 {
		return "refused"
	}
	return "unknown"
}
