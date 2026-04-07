//go:build darwin

package dns

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func TestResolvManager_AddAndRemoveDomain(t *testing.T) {
	dir := t.TempDir()
	m := NewResolvManagerWithPaths(dir, "", logr.Discard())

	if err := m.AddDomain("example.com", "127.0.0.2:53"); err != nil {
		t.Fatalf("AddDomain: %v", err)
	}

	filePath := filepath.Join(dir, "example.com")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("resolver file not created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "nameserver 127.0.0.2") {
		t.Fatalf("expected nameserver 127.0.0.2 in file, got: %s", content)
	}
	if !strings.Contains(content, "port 53") {
		t.Fatalf("expected port 53 in file, got: %s", content)
	}
	if !strings.Contains(content, managedMarker) {
		t.Fatalf("expected managed marker in file, got: %s", content)
	}

	if err := m.RemoveDomain("example.com"); err != nil {
		t.Fatalf("RemoveDomain: %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatal("resolver file should be removed after RemoveDomain")
	}
}

func TestResolvManager_RecoverFromCrash(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "example.com")
	if err := os.WriteFile(filePath, []byte(managedMarker+"\nnameserver 127.0.0.1\nport 53\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewResolvManagerWithPaths(dir, "", logr.Discard())
	if err := m.RecoverFromCrash(); err != nil {
		t.Fatalf("RecoverFromCrash: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatal("managed resolver file should be removed after crash recovery")
	}
}

func TestResolvManager_RecoverSkipsUnmanagedFiles(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "other.com")
	if err := os.WriteFile(filePath, []byte("nameserver 8.8.8.8\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewResolvManagerWithPaths(dir, "", logr.Discard())
	if err := m.RecoverFromCrash(); err != nil {
		t.Fatalf("RecoverFromCrash: %v", err)
	}

	if _, err := os.Stat(filePath); err != nil {
		t.Fatal("unmanaged resolver file should not be removed")
	}
}

func TestResolvManager_ManagedDomains(t *testing.T) {
	dir := t.TempDir()
	m := NewResolvManagerWithPaths(dir, "", logr.Discard())

	m.AddDomain("a.example.com", "127.0.0.1:53")
	m.AddDomain("b.example.com", "127.0.0.1:53")

	domains := m.ManagedDomains()
	sort.Strings(domains)
	if len(domains) != 2 {
		t.Fatalf("expected 2 managed domains, got %d", len(domains))
	}
	if domains[0] != "a.example.com" || domains[1] != "b.example.com" {
		t.Fatalf("unexpected domains: %v", domains)
	}
}

func TestResolvManager_RemoveAllDomains(t *testing.T) {
	dir := t.TempDir()
	m := NewResolvManagerWithPaths(dir, "", logr.Discard())

	m.AddDomain("a.example.com", "127.0.0.1:53")
	m.AddDomain("b.example.com", "127.0.0.1:53")

	if err := m.RemoveAllDomains(); err != nil {
		t.Fatalf("RemoveAllDomains: %v", err)
	}

	for _, domain := range []string{"a.example.com", "b.example.com"} {
		if _, err := os.Stat(filepath.Join(dir, domain)); !os.IsNotExist(err) {
			t.Fatalf("resolver file for %s should be removed", domain)
		}
	}

	if len(m.ManagedDomains()) != 0 {
		t.Fatal("ManagedDomains should return empty after RemoveAllDomains")
	}
}
