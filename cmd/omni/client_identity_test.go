package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClientInstallationIdentityPersistsAndRejectsInvalidState(t *testing.T) {
	root := t.TempDir()
	first, err := loadClientInstallationIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadClientInstallationIdentity(root)
	if err != nil || first != second {
		t.Fatalf("client identity changed: %q, %q: %v", first, second, err)
	}
	other, err := loadClientInstallationIdentity(t.TempDir())
	if err != nil || first == other {
		t.Fatalf("distinct installations share identity: %q: %v", other, err)
	}
	if err := os.WriteFile(filepath.Join(root, "omnidex", "client-id"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadClientInstallationIdentity(root); err == nil {
		t.Fatal("invalid client identity was replaced or accepted")
	}
}
