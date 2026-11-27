package main

import (
	"errors"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const pluginID = "quota-first-router"
const pluginVersion = "0.2.0"
const defaultWarmupModel = "gpt-6-luna"

type config struct {
	PollInterval   time.Duration `yaml:"poll_interval" json:"-"`
	MaxAge         time.Duration `yaml:"max_age" json:"-"`
	RequestTimeout time.Duration `yaml:"request_timeout" json:"-"`
	// Empty follows environment transport policy; set this to CPA requests.proxy-url
	// when the host uses a global proxy (the lifecycle API does not expose it).
	ProxyURL        string `yaml:"proxy_url" json:"-"`
	WarmupEnabled   bool   `yaml:"warmup_after_reset" json:"-"`
	WarmupDirectory string `yaml:"warmup_state_dir" json:"-"`
	WarmupEndpoint  string `yaml:"warmup_cpa_url" json:"-"`
	WarmupModel     string `yaml:"warmup_model" json:"-"`
}

func decodeConfig(raw []byte) (config, error) {
	input := struct {
		config     `yaml:",inline"`
		LegacyMode string `yaml:"mode"`
	}{config: config{PollInterval: time.Minute, MaxAge: 5 * time.Minute, RequestTimeout: 10 * time.Second, WarmupModel: defaultWarmupModel}}
	if len(raw) != 0 {
		if err := yaml.Unmarshal(raw, &input); err != nil {
			return config{}, errors.New("invalid plugin configuration")
		}
	}
	// Do not silently start routing for installations explicitly using shadow.
	if input.LegacyMode != "" && input.LegacyMode != "active" {
		return config{}, errors.New("legacy mode must be removed before enabling the plugin")
	}
	cfg := input.config
	if strings.TrimSpace(cfg.WarmupModel) == "" {
		cfg.WarmupModel = defaultWarmupModel
	}
	if cfg.PollInterval < time.Minute || cfg.PollInterval > time.Hour {
		return config{}, errors.New("poll_interval must be between 1m and 1h")
	}
	if cfg.MaxAge < cfg.PollInterval || cfg.MaxAge > time.Hour {
		return config{}, errors.New("max_age must be between poll_interval and 1h")
	}
	if cfg.RequestTimeout < time.Second || cfg.RequestTimeout > 30*time.Second {
		return config{}, errors.New("request_timeout must be between 1s and 30s")
	}
	if cfg.ProxyURL != "" && cfg.ProxyURL != "direct" {
		u, err := url.Parse(cfg.ProxyURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
			return config{}, errors.New("invalid proxy_url")
		}
	}
	if cfg.WarmupEnabled && (!filepath.IsAbs(cfg.WarmupDirectory) || !validWarmupEndpoint(cfg.WarmupEndpoint) || !validWarmupModel(cfg.WarmupModel)) {
		return config{}, errors.New("warmup requires a private state directory, numeric loopback CPA URL and valid model")
	}
	return cfg, nil
}

func validWarmupEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(raw, "#\\") {
		return false
	}
	addr, err := netip.ParseAddr(u.Hostname())
	port, errPort := strconv.ParseUint(u.Port(), 10, 16)
	return err == nil && addr.IsLoopback() && !addr.Is4In6() && addr.Zone() == "" && errPort == nil && port != 0
}

func validWarmupModel(model string) bool {
	return len(model) <= 64 && strings.HasPrefix(model, "gpt-") &&
		strings.IndexFunc(model, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.') }) < 0
}
