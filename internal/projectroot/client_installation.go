package projectroot

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ReadClientInstallationIdentity binds a native environment to the existing
// local client. Reading never enrolls or replaces an installation.
func ReadClientInstallationIdentity(configRoot string) (string, error) {
	if !filepath.IsAbs(configRoot) || filepath.Clean(configRoot) != configRoot {
		return "", fmt.Errorf("client configuration requires a canonical absolute directory")
	}
	file, err := os.Open(filepath.Join(configRoot, "omnidex", "client-id"))
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("client identity is not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, 33))
	if err != nil {
		return "", err
	}
	identity := string(data)
	if err := ValidateClientIdentity(identity); err != nil {
		return "", fmt.Errorf("stored client identity is invalid: %w", err)
	}
	return identity, nil
}
