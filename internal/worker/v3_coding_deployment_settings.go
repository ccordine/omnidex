package worker

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const directCodingDeploymentKeyBytes = 32

var directCodingDeploymentHostPattern = regexp.MustCompile(
	`^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`,
)

func validateDirectCodingDeploymentSettings(settings DeploymentSettings) error {
	if settings.KeyFile == "" || !filepath.IsAbs(settings.KeyFile) ||
		filepath.Clean(settings.KeyFile) != settings.KeyFile {
		return fmt.Errorf("persistent deployment requires one normalized absolute key file")
	}
	bind, err := netip.ParseAddr(settings.BindAddress)
	if err != nil || !bind.IsValid() || !bind.Is4() || bind.IsMulticast() ||
		bind.String() != settings.BindAddress {
		return fmt.Errorf("persistent deployment bind address must be one canonical non-multicast IPv4 literal")
	}
	if settings.BindAddress != "127.0.0.1" && settings.BindAddress != "0.0.0.0" {
		return fmt.Errorf("persistent deployment bind address must be exactly loopback or all interfaces")
	}
	if err := validateDirectCodingDeploymentHost("advertised", settings.AdvertisedHost); err != nil {
		return err
	}
	if err := validateDirectCodingDeploymentHost("probe", settings.ProbeHost); err != nil {
		return err
	}
	return nil
}

func validateDirectCodingDeploymentHost(label, value string) error {
	if value == "" || value != strings.TrimSpace(value) ||
		strings.ContainsAny(value, "\x00\r\n/:[]") {
		return fmt.Errorf("persistent deployment %s host is invalid", label)
	}
	if address, err := netip.ParseAddr(value); err == nil {
		if !address.IsValid() || address.IsUnspecified() || address.IsMulticast() {
			return fmt.Errorf("persistent deployment %s host is invalid", label)
		}
		return nil
	}
	if len(value) > 253 || !directCodingDeploymentHostPattern.MatchString(value) ||
		strings.Contains(value, "..") {
		return fmt.Errorf("persistent deployment %s host is invalid", label)
	}
	return nil
}

func loadOrCreateDirectCodingDeploymentKey(path string) ([]byte, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("deployment key path must be normalized and absolute")
	}
	directory := filepath.Dir(path)
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("deployment key directory is unavailable: %s", directory)
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil || resolved != directory {
		return nil, fmt.Errorf("deployment key directory contains a symbolic-link boundary")
	}
	key, err := readDirectCodingDeploymentKey(path)
	if err == nil {
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	key = make([]byte, directCodingDeploymentKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate deployment key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return readDirectCodingDeploymentKey(path)
		}
		return nil, fmt.Errorf("create deployment key: %w", err)
	}
	writeErr := error(nil)
	if _, err := file.Write(key); err != nil {
		writeErr = fmt.Errorf("write deployment key: %w", err)
	} else if err := file.Sync(); err != nil {
		writeErr = fmt.Errorf("sync deployment key: %w", err)
	}
	if closeErr := file.Close(); writeErr == nil && closeErr != nil {
		writeErr = fmt.Errorf("close deployment key: %w", closeErr)
	}
	if writeErr != nil {
		return nil, writeErr
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return nil, fmt.Errorf("open deployment key directory: %w", err)
	}
	if err := directoryHandle.Sync(); err != nil {
		directoryHandle.Close()
		return nil, fmt.Errorf("sync deployment key directory: %w", err)
	}
	if err := directoryHandle.Close(); err != nil {
		return nil, fmt.Errorf("close deployment key directory: %w", err)
	}
	return append([]byte(nil), key...), nil
}

func readDirectCodingDeploymentKey(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("deployment key must be one regular non-symlink file with mode 0600")
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read deployment key: %w", err)
	}
	if len(key) != directCodingDeploymentKeyBytes {
		return nil, fmt.Errorf("deployment key must contain exactly %d bytes", directCodingDeploymentKeyBytes)
	}
	return key, nil
}

func deriveDirectCodingDeploymentSecrets(
	key []byte,
	projectID int64,
	secretGeneration int64,
) (map[string]string, error) {
	if len(key) != directCodingDeploymentKeyBytes || projectID <= 0 || secretGeneration <= 0 {
		return nil, fmt.Errorf("deployment secret derivation requires exact project authority")
	}
	derive := func(purpose string) ([]byte, error) {
		info := strings.Join([]string{
			"omnidex.generated-service-deployment.v2", strconv.FormatInt(projectID, 10),
			strconv.FormatInt(secretGeneration, 10), purpose,
		}, "\x00")
		return hkdf.Key(sha256.New, key, []byte("omnidex-deployment-key-v1"), info, 32)
	}
	applicationKey, err := derive("application-key")
	if err != nil {
		return nil, fmt.Errorf("derive application deployment key: %w", err)
	}
	databaseKey, err := derive("database-password")
	if err != nil {
		return nil, fmt.Errorf("derive database deployment password: %w", err)
	}
	return map[string]string{
		"APP_KEY":                   "base64:" + base64.StdEncoding.EncodeToString(applicationKey),
		"DATABASE_PASSWORD":         hex.EncodeToString(databaseKey),
		"SERVICE_STATE_DB_PASSWORD": hex.EncodeToString(databaseKey),
	}, nil
}

func directCodingDeploymentSecretSetSHA256(
	names []string,
	environment map[string]string,
) (string, error) {
	if len(names) > 16 {
		return "", fmt.Errorf("deployment secret set exceeds 16 names")
	}
	hash := sha256.New()
	hash.Write([]byte("omnidex.generated-service-secret-set.v1"))
	hash.Write([]byte{0})
	previous := ""
	for index, name := range names {
		if name == "" || index > 0 && name <= previous {
			return "", fmt.Errorf("deployment secret names must be normalized, sorted, and unique")
		}
		value := environment[name]
		if value == "" {
			return "", fmt.Errorf("deployment secret environment omits %s", name)
		}
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write([]byte(value))
		hash.Write([]byte{0})
		previous = name
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
