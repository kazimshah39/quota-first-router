package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const statusPath = "/v0/management/plugins/" + pluginID + "/status"

type pluginRuntime struct {
	host    authHost
	mu      sync.Mutex
	active  atomic.Pointer[engine]
	cancel  context.CancelFunc
	done    chan struct{}
	fetcher *httpQuotaFetcher
}

func success(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return failure("response_encode_failed")
	}
	out, _ := json.Marshal(pluginabi.Envelope{OK: true, Result: raw})
	return out
}

func failure(code string) []byte {
	raw, _ := json.Marshal(pluginabi.Envelope{Error: &pluginabi.Error{Code: code, Message: code}})
	return raw
}

func (p *pluginRuntime) handle(method string, raw []byte) []byte {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		var req struct {
			ConfigYAML []byte `json:"config_yaml"`
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &req); err != nil {
				return failure("invalid_lifecycle_request")
			}
		}
		cfg, err := decodeConfig(req.ConfigYAML)
		if err != nil {
			return failure("invalid_configuration")
		}
		if p.host == nil {
			return failure("host_unavailable")
		}
		p.configure(cfg)
		return success(registration())
	case pluginabi.MethodPluginQuiesce, pluginabi.MethodPluginShutdown:
		p.stop()
		return success(struct{}{})
	case pluginabi.MethodSchedulerPick:
		var req pluginapi.SchedulerPickRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return success(pluginapi.SchedulerPickResponse{})
		}
		if e := p.active.Load(); e != nil {
			return success(e.pick(req))
		}
		return success(pluginapi.SchedulerPickResponse{})
	case pluginabi.MethodManagementRegister:
		return success(pluginapi.ManagementRegistrationResponse{
			Routes:    []pluginapi.ManagementRoute{{Method: http.MethodGet, Path: statusPath}, {Method: http.MethodGet, Path: monitorPath}},
			Resources: []pluginapi.ResourceRoute{{Path: resourcePath, Menu: "Quota First Router", Description: "Account order, quota health and recent scheduler decisions"}, {Path: resourcePath + ".css"}, {Path: resourcePath + ".js"}},
		})
	case pluginabi.MethodManagementHandle:
		var req pluginapi.ManagementRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return success(pluginapi.ManagementResponse{StatusCode: http.StatusNotFound})
		}
		return success(p.management(req))
	default:
		return failure("unsupported_method")
	}
}

func (p *pluginRuntime) configure(cfg config) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
	p.fetcher = newHTTPFetcher()
	e := newEngine(p.host, p.fetcher, cfg)
	if cfg.WarmupEnabled {
		var err error
		e.warmup, err = openWarmupWorker(cfg)
		if err != nil {
			e.warmupError = err.Error() // fixed, non-secret error codes only
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	done := p.done
	p.active.Store(e)
	var warmupDone <-chan struct{}
	if e.warmup != nil {
		finished := make(chan struct{})
		warmupDone = finished
		go func() {
			defer close(finished)
			defer func() {
				if recover() != nil {
					e.mu.Lock()
					e.warmupError = "worker_failed"
					e.mu.Unlock()
				}
			}()
			e.warmup.run(ctx, e)
		}()
	}
	go func() {
		defer close(done)
		defer func() {
			cancel()
			if warmupDone != nil {
				<-warmupDone
			}
		}()
		defer func() {
			if recover() != nil {
				e.markFailed()
			}
		}()
		e.run(ctx)
	}()
}

func (p *pluginRuntime) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *pluginRuntime) stopLocked() {
	p.active.Store(nil)
	if p.cancel != nil {
		p.cancel()
		<-p.done
		p.cancel, p.done = nil, nil
	}
	if p.fetcher != nil {
		p.fetcher.client.CloseIdleConnections()
		p.fetcher = nil
	}
}

func registration() any {
	return struct {
		SchemaVersion uint32             `json:"schema_version"`
		Metadata      pluginapi.Metadata `json:"metadata"`
		Capabilities  map[string]bool    `json:"capabilities"`
	}{
		pluginabi.SchemaVersion,
		pluginapi.Metadata{Name: pluginID, Version: pluginVersion, Author: "kazimshah39", GitHubRepository: "https://github.com/kazimshah39/quota-first-router",
			ConfigFields: []pluginapi.ConfigField{
				// The main drawer only needs the routine control. Expert and setup
				// keys still decode normally and can be edited in JSON or YAML.
				{Name: "warmup_after_reset", Type: pluginapi.ConfigFieldTypeBoolean, Default: false, Description: "Start idle five-hour Codex windows with a small request. Default model: GPT-6 Luna. Uses a little quota."},
			}},
		map[string]bool{"scheduler": true, "scheduler_across_priorities": true, "management_api": true},
	}
}
