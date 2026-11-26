package main

import (
	"errors"
	"net/url"
	"time"

	"gopkg.in/yaml.v3"
)

const pluginID = "quota-first-router"
const pluginVersion = "0.2.0"

type config struct {
	PollInterval   time.Duration `yaml:"poll_interval" json:"-"`
	MaxAge         time.Duration `yaml:"max_age" json:"-"`
	RequestTimeout time.Duration `yaml:"request_timeout" json:"-"`
	// Empty follows environment transport policy; set this to CPA requests.proxy-url
	// when the host uses a global proxy (the lifecycle API does not expose it).
	ProxyURL string `yaml:"proxy_url" json:"-"`
}

func decodeConfig(raw []byte) (config, error) {
	input := struct {
		config     `yaml:",inline"`
		LegacyMode string `yaml:"mode"`
	}{config: config{PollInterval: time.Minute, MaxAge: 5 * time.Minute, RequestTimeout: 10 * time.Second}}
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
	return cfg, nil
}
