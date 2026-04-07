//go:build windows

package dns

import (
	"github.com/go-logr/logr"
)

// ResolvManager is a no-op on Windows.
type ResolvManager struct {
	logger logr.Logger
}

func NewResolvManager(logger logr.Logger) *ResolvManager {
	return &ResolvManager{logger: logger}
}

func NewResolvManagerWithPaths(_, _ string, logger logr.Logger) *ResolvManager {
	return &ResolvManager{logger: logger}
}

func (m *ResolvManager) SetInterface(name string) {}

func (m *ResolvManager) ParseUpstreamServers() ([]string, error) {
	return []string{"8.8.8.8:53"}, nil
}

func (m *ResolvManager) IsSymlink() bool { return false }

func (m *ResolvManager) RecoverFromCrash() error { return nil }

func (m *ResolvManager) ManageResolvConf(listenAddr string) error {
	m.logger.Info("resolv.conf management not supported on Windows")
	return nil
}

func (m *ResolvManager) RestoreResolvConf() error { return nil }

func (m *ResolvManager) AddDomain(domain, listenAddr string) error {
	m.logger.Info("Per-domain DNS not supported on Windows", "domain", domain)
	return nil
}

func (m *ResolvManager) RemoveDomain(domain string) error { return nil }

func (m *ResolvManager) RemoveAllDomains() error { return nil }

func (m *ResolvManager) ManagedDomains() []string { return nil }
