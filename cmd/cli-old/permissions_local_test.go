package main

import (
	"path/filepath"
	"testing"
)

func TestPermissionManagerSetListUnset(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "permissions.json")

	pm := &permissionManager{
		path: path,
		state: permissionRegistry{
			Version:     1,
			Permissions: map[string]permissionDecision{},
		},
	}

	if err := pm.Set(permissionKeyScreenCapture, true, "test"); err != nil {
		t.Fatalf("Set allow failed: %v", err)
	}
	if err := pm.Set(permissionKeyBrowserConsole, false, "test"); err != nil {
		t.Fatalf("Set deny failed: %v", err)
	}

	_, entries, err := pm.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if !entries[permissionKeyScreenCapture].Allowed {
		t.Fatalf("expected %s to be allowed", permissionKeyScreenCapture)
	}
	if entries[permissionKeyBrowserConsole].Allowed {
		t.Fatalf("expected %s to be denied", permissionKeyBrowserConsole)
	}

	if err := pm.Unset(permissionKeyBrowserConsole); err != nil {
		t.Fatalf("Unset failed: %v", err)
	}
	_, entries, err = pm.List()
	if err != nil {
		t.Fatalf("List after unset failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after unset, got %d", len(entries))
	}
	if _, ok := entries[permissionKeyBrowserConsole]; ok {
		t.Fatalf("expected %s to be removed", permissionKeyBrowserConsole)
	}
}

func TestPermissionManagerReset(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "permissions.json")

	pm := &permissionManager{
		path: path,
		state: permissionRegistry{
			Version:     1,
			Permissions: map[string]permissionDecision{},
		},
	}

	if err := pm.Set(permissionKeyMediaControl, true, "test"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := pm.Reset(); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	_, entries, err := pm.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty permissions after reset, got %d", len(entries))
	}
}
