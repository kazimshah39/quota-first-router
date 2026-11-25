package main

import (
	"errors"
	"time"

	"gopkg.in/yaml.v3"
)

const pluginID = "quota-first-router"
const pluginVersion = "0.1.0"

type config struct {
	Mode           string        `yaml:"mode" json:"mode"`
	PollInterval   time.Duration `yaml:"poll_interval" json:"-"`
	MaxAge         time.Duration `yaml:"max_age" json:"-"`
	RequestTimeout time.Duration `yaml:"request_timeout" json:"-"`
	// Empty follows environment transport policy; set this to CPA requests.proxy-url
	// when the host uses a global proxy (the lifecycle API does not expose it).
	ProxyURL string `yaml:"proxy_url" json:"-"`
}

func decodeConfig(raw []byte) (config, error) {
	cfg := config{Mode: "shadow", PollInterval: time.Minute, MaxAge: 5 * time.Minute, RequestTimeout: 10 * time.Second}
	if len(raw) != 0 {
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return config{}, errors.New("invalid plugin configuration")
		}
	}
	if cfg.Mode != "shadow" && cfg.Mode != "active" {
		return config{}, errors.New("mode must be shadow or active")
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
	return cfg, nil
}
