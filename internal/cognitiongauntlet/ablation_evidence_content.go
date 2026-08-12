package cognitiongauntlet

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

type ablationEvidenceContentStore struct {
	blobs        map[string]cognitionreplay.Blob
	bindings     map[string]cognitionreplay.ChunkedBlobBinding
	usedBlobs    map[string]struct{}
	usedBindings map[string]struct{}
}

func newAblationEvidenceContentStore(
	artifact ablationEvidenceArtifact,
) (*ablationEvidenceContentStore, error) {
	store := &ablationEvidenceContentStore{
		blobs: make(map[string]cognitionreplay.Blob, len(artifact.Blobs)),
		bindings: make(map[string]cognitionreplay.ChunkedBlobBinding,
			len(artifact.Root.ChunkedBlobs)),
		usedBlobs: make(map[string]struct{}), usedBindings: make(map[string]struct{}),
	}
	previous := ""
	for index, value := range artifact.Blobs {
		blob := cognitionreplay.Blob{
			SHA256: value.SHA256, MediaType: value.MediaType,
			Data: append([]byte(nil), value.Data...),
		}
		if blob.Validate() != nil || (index > 0 && value.SHA256 <= previous) {
			return nil, fmt.Errorf("ablation evidence blob order or authority is invalid")
		}
		store.blobs[value.SHA256] = blob
		previous = value.SHA256
	}
	previous = ""
	for index, binding := range artifact.Root.ChunkedBlobs {
		if binding.Validate() != nil ||
			(index > 0 && binding.Manifest.SHA256 <= previous) {
			return nil, fmt.Errorf("ablation evidence chunk binding order or authority is invalid")
		}
		store.bindings[binding.Manifest.SHA256] = binding
		previous = binding.Manifest.SHA256
	}
	return store, nil
}

func (store *ablationEvidenceContentStore) read(
	authority cognitionreplay.ProjectionContentAuthority,
	role cognitionreplay.ChunkedBlobRole,
) ([]byte, error) {
	if store == nil || authority.ValidateForRole(role) != nil {
		return nil, fmt.Errorf("ablation evidence content authority is invalid")
	}
	switch authority.Storage {
	case cognitionreplay.ProjectionContentEmpty:
		return []byte{}, nil
	case cognitionreplay.ProjectionContentDirect:
		blob, exists := store.blobs[authority.Blob.SHA256]
		if !exists || blob.Ref() != *authority.Blob {
			return nil, fmt.Errorf("ablation evidence direct content is missing or changed")
		}
		store.usedBlobs[blob.SHA256] = struct{}{}
		return append([]byte(nil), blob.Data...), nil
	case cognitionreplay.ProjectionContentChunked:
		return store.readChunked(authority, role)
	default:
		return nil, fmt.Errorf("ablation evidence content storage is not registered")
	}
}

func (store *ablationEvidenceContentStore) readChunked(
	authority cognitionreplay.ProjectionContentAuthority,
	role cognitionreplay.ChunkedBlobRole,
) ([]byte, error) {
	binding, exists := store.bindings[authority.Manifest.SHA256]
	manifestBlob, hasManifest := store.blobs[authority.Manifest.SHA256]
	if !exists || !hasManifest || binding.Manifest != *authority.Manifest ||
		manifestBlob.Ref() != binding.Manifest {
		return nil, fmt.Errorf("ablation evidence chunk manifest is missing or changed")
	}
	var manifest cognitionreplay.ChunkedBlobManifest
	if err := json.Unmarshal(manifestBlob.Data, &manifest); err != nil ||
		manifest.ChunkCount < 1 || manifest.ChunkCount > 128 ||
		len(manifest.Chunks) != manifest.ChunkCount {
		return nil, fmt.Errorf("ablation evidence chunk manifest is invalid: %v", err)
	}
	blobs := make([]cognitionreplay.Blob, 0, manifest.ChunkCount+1)
	blobs = append(blobs, manifestBlob)
	for _, chunk := range manifest.Chunks {
		blob, found := store.blobs[chunk.Payload.SHA256]
		if !found {
			return nil, fmt.Errorf("ablation evidence chunk is missing")
		}
		blobs = append(blobs, blob)
	}
	raw, err := cognitionreplay.VerifyChunkedBlob(binding, blobs, role)
	if err != nil || int64(len(raw)) != authority.ByteCount ||
		digestExactBytes(raw) != authority.ContentSHA256 ||
		manifest.MediaType != authority.MediaType {
		return nil, fmt.Errorf("ablation evidence chunks differ from logical content: %v", err)
	}
	store.usedBindings[binding.Manifest.SHA256] = struct{}{}
	store.usedBlobs[manifestBlob.SHA256] = struct{}{}
	for _, chunk := range manifest.Chunks {
		store.usedBlobs[chunk.Payload.SHA256] = struct{}{}
	}
	return bytes.Clone(raw), nil
}

func (store *ablationEvidenceContentStore) requireClosure() error {
	if store == nil || len(store.usedBlobs) != len(store.blobs) ||
		len(store.usedBindings) != len(store.bindings) {
		return fmt.Errorf("ablation evidence contains uncited content")
	}
	return nil
}
