//go:build musl

package proxy

import (
	"context"
	"fmt"

	"go.minekube.com/gate/pkg/edition/java/config"
)

type viaManagedRunner struct {
	cfg *config.Config
}

func newViaManagedRunner(cfg *config.Config) *viaManagedRunner {
	return &viaManagedRunner{cfg: cfg}
}

func (r *viaManagedRunner) enabled() bool {
	return r != nil && r.cfg != nil && r.cfg.Via.Enabled && !r.cfg.Lite.Enabled
}

func (r *viaManagedRunner) backendEnabled(string) bool {
	return false
}

func (r *viaManagedRunner) Start(context.Context) error {
	return fmt.Errorf("vialite is unavailable in this build")
}

func (r *viaManagedRunner) Stop() {}

func (r *viaManagedRunner) BackendDialAddress(string) (string, error) {
	return "", fmt.Errorf("vialite is unavailable in this build")
}

func (r *viaManagedRunner) AddBackend(context.Context, ServerInfo) (bool, error) {
	return false, nil
}

func (r *viaManagedRunner) RemoveBackend(context.Context, string) error { return nil }

type viaServerInfo struct {
	ServerInfo
}

func newViaServerInfo(info ServerInfo, _ *viaManagedRunner) ServerInfo {
	return info
}
