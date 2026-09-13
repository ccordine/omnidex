package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/projectroot"
)

// This installation identifier disambiguates local filesystem identities. It
// stores no workspace inventory, job state, source bytes, or execution history.
func loadClientInstallationIdentity(configRoot string) (string, error) {
	if !filepath.IsAbs(configRoot) || filepath.Clean(configRoot) != configRoot {
		return "", fmt.Errorf("client configuration requires one canonical absolute directory")
	}
	directory := filepath.Join(configRoot, "omnidex")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create client configuration directory: %w", err)
	}
	name := filepath.Join(directory, "client-id")
	file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return readClientInstallationIdentity(name)
	}
	if err != nil {
		return "", fmt.Errorf("create client installation identity: %w", err)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", errors.Join(err, file.Close())
	}
	identity := hex.EncodeToString(nonce[:])
	if _, err := file.WriteString(identity); err != nil {
		return "", errors.Join(fmt.Errorf("write client identity: %w", err), file.Close())
	}
	return identity, errors.Join(file.Sync(), file.Close())
}

func readClientInstallationIdentity(name string) (identity string, resultErr error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
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
	identity = string(data)
	if err := projectroot.ValidateClientIdentity(identity); err != nil {
		return "", fmt.Errorf("stored client identity is invalid: %w", err)
	}
	return identity, nil
}
