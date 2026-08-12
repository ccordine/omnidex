package cognitiongauntlet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func SealEpisode(path string, manifest EpisodeManifest) (SealedEpisode, error) {
	prepared, err := prepareEpisodeManifest(manifest)
	if err != nil {
		return SealedEpisode{}, err
	}
	seal := SealedEpisode{Schema: EpisodeSealSchemaV1, Manifest: prepared}
	seal.SealSHA256, err = digestJSON(seal.Manifest)
	if err != nil {
		return SealedEpisode{}, fmt.Errorf("hash cognition episode seal: %w", err)
	}
	if err := seal.Validate(); err != nil {
		return SealedEpisode{}, err
	}
	raw, err := json.Marshal(seal)
	if err != nil {
		return SealedEpisode{}, fmt.Errorf("encode cognition episode seal: %w", err)
	}
	if err := writeExclusiveAtomic(path, append(raw, '\n')); err != nil {
		return SealedEpisode{}, err
	}
	return seal, nil
}

func LoadSealedEpisode(path string) (SealedEpisode, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return SealedEpisode{}, fmt.Errorf("read cognition episode seal: %w", err)
	}
	var seal SealedEpisode
	if err := decodeStrictJSON(raw, &seal, "cognition episode seal"); err != nil {
		return SealedEpisode{}, err
	}
	if err := seal.Validate(); err != nil {
		return SealedEpisode{}, err
	}
	if err := verifySealedEpisodeRuntimeProviderIdentity(path, seal); err != nil {
		return SealedEpisode{}, err
	}
	return seal, nil
}

func prepareEpisodeManifest(manifest EpisodeManifest) (EpisodeManifest, error) {
	prepared := manifest
	prepared.Trace = append([]TraceEntry(nil), manifest.Trace...)
	for index := range prepared.Trace {
		entry := &prepared.Trace[index]
		if err := entry.Payload.Validate(); err != nil {
			return EpisodeManifest{}, fmt.Errorf("trace entry %d payload: %w", index+1, err)
		}
		digest, err := digestJSON(entry.Payload)
		if err != nil {
			return EpisodeManifest{}, fmt.Errorf("hash trace entry %d: %w", index+1, err)
		}
		entry.PayloadSHA256 = digest
	}
	traceDigest, err := digestJSON(prepared.Trace)
	if err != nil {
		return EpisodeManifest{}, fmt.Errorf("hash cognition episode trace: %w", err)
	}
	prepared.TraceSHA256 = traceDigest
	if err := prepared.validateSealed(); err != nil {
		return EpisodeManifest{}, err
	}
	return prepared, nil
}

func (seal SealedEpisode) Validate() error {
	if seal.Schema != EpisodeSealSchemaV1 || !validDigest(seal.SealSHA256) {
		return fmt.Errorf("cognition episode seal identity is invalid")
	}
	if err := seal.Manifest.validateSealed(); err != nil {
		return err
	}
	expected, err := digestJSON(seal.Manifest)
	if err != nil || expected != seal.SealSHA256 {
		return fmt.Errorf("cognition episode seal does not match its manifest")
	}
	return nil
}

func writeExclusiveAtomic(path string, raw []byte) error {
	if path == "" || filepath.Clean(path) != path {
		return fmt.Errorf("cognition episode seal path must be exact")
	}
	directory := filepath.Dir(path)
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("cognition episode seal directory is unavailable")
	}
	temporary, err := os.CreateTemp(directory, ".cognition-episode-*")
	if err != nil {
		return fmt.Errorf("create cognition episode seal: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() { _ = os.Remove(temporaryPath) }
	defer cleanup()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure cognition episode seal: %w", err)
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write cognition episode seal: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync cognition episode seal: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close cognition episode seal: %w", err)
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return fmt.Errorf("publish exclusive cognition episode seal: %w", err)
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open cognition episode seal directory: %w", err)
	}
	defer directoryHandle.Close()
	if err := directoryHandle.Sync(); err != nil {
		return fmt.Errorf("sync cognition episode seal directory: %w", err)
	}
	return nil
}
