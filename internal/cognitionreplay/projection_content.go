package cognitionreplay

import (
	"bytes"
	"fmt"
)

const ProjectionContentAuthoritySchemaV1 = "omnidex.replay-content-authority.v1"

type ProjectionContentStorage string

const (
	ProjectionContentEmpty   ProjectionContentStorage = "empty"
	ProjectionContentDirect  ProjectionContentStorage = "direct"
	ProjectionContentChunked ProjectionContentStorage = "chunked"
)

func NewEmptyProjectionContent(mediaType string) (ProjectionContentAuthority, error) {
	return newEmptyProjectionContent(mediaType, ChunkedBlobPublicAgentKnowledge)
}

func NewPrivateEmptyProjectionContent(mediaType string) (ProjectionContentAuthority, error) {
	return newEmptyProjectionContent(mediaType, ChunkedBlobPrivateWorld)
}

func newEmptyProjectionContent(
	mediaType string,
	role ChunkedBlobRole,
) (ProjectionContentAuthority, error) {
	value := ProjectionContentAuthority{
		Schema: ProjectionContentAuthoritySchemaV1, MediaType: mediaType,
		ContentSHA256: digestBytes(nil), ByteCount: 0, Storage: ProjectionContentEmpty,
		Role: role,
	}
	return value, value.Validate()
}

// ProjectionContentAuthority separates logical content identity from its
// deterministic container storage. Exactly one direct blob or chunk manifest
// is authoritative.
type ProjectionContentAuthority struct {
	Schema        string                   `json:"schema"`
	MediaType     string                   `json:"media_type"`
	ContentSHA256 string                   `json:"content_sha256"`
	ByteCount     int64                    `json:"byte_count"`
	Role          ChunkedBlobRole          `json:"role"`
	Storage       ProjectionContentStorage `json:"storage"`
	Blob          *BlobRef                 `json:"blob,omitempty"`
	Manifest      *BlobRef                 `json:"manifest,omitempty"`
}

func NewPublicProjectionContent(
	id string,
	mediaType string,
	data []byte,
) (ProjectionContentAuthority, []ChunkedBlobBinding, []Blob, error) {
	return newProjectionContent(
		id, mediaType, data, ChunkedBlobPublicAgentKnowledge,
	)
}

func NewPrivateProjectionContent(
	id string,
	mediaType string,
	data []byte,
) (ProjectionContentAuthority, []ChunkedBlobBinding, []Blob, error) {
	return newProjectionContent(id, mediaType, data, ChunkedBlobPrivateWorld)
}

func newProjectionContent(
	id string,
	mediaType string,
	data []byte,
	role ChunkedBlobRole,
) (ProjectionContentAuthority, []ChunkedBlobBinding, []Blob, error) {
	if len(data) == 0 || len(data) > maxChunkedBlobBytes || !validMediaType(mediaType) {
		return ProjectionContentAuthority{}, nil, nil,
			fmt.Errorf("projection content byte authority is invalid")
	}
	value := ProjectionContentAuthority{
		Schema: ProjectionContentAuthoritySchemaV1, MediaType: mediaType,
		ContentSHA256: digestBytes(data), ByteCount: int64(len(data)),
		Role: role,
	}
	if len(data) <= maxBlobBytes {
		blob, err := NewBlob(mediaType, data)
		if err != nil {
			return ProjectionContentAuthority{}, nil, nil, err
		}
		ref := blob.Ref()
		value.Storage, value.Blob = ProjectionContentDirect, &ref
		return value, []ChunkedBlobBinding{}, []Blob{blob}, nil
	}
	binding, blobs, err := chunkProjectionBytes(id, mediaType, data, role)
	if err != nil {
		return ProjectionContentAuthority{}, nil, nil, err
	}
	ref := binding.Manifest
	value.Storage, value.Manifest = ProjectionContentChunked, &ref
	return value, []ChunkedBlobBinding{binding}, blobs, nil
}

func (value ProjectionContentAuthority) Validate() error {
	if value.Schema != ProjectionContentAuthoritySchemaV1 || !validMediaType(value.MediaType) ||
		!validDigest(value.ContentSHA256) || value.ByteCount < 0 ||
		value.ByteCount > maxChunkedBlobBytes || !validChunkedBlobRole(value.Role) {
		return fmt.Errorf("projection content logical authority is invalid")
	}
	switch value.Storage {
	case ProjectionContentEmpty:
		if value.ByteCount != 0 || value.ContentSHA256 != digestBytes(nil) ||
			value.MediaType != "application/octet-stream" ||
			value.Blob != nil || value.Manifest != nil {
			return fmt.Errorf("projection empty content authority is invalid")
		}
	case ProjectionContentDirect:
		if value.Blob == nil || value.Manifest != nil || value.Blob.Validate() != nil ||
			value.ByteCount <= 0 || value.ByteCount > maxBlobBytes || value.Blob.MediaType != value.MediaType ||
			value.Blob.SHA256 != value.ContentSHA256 || int64(value.Blob.ByteCount) != value.ByteCount {
			return fmt.Errorf("projection direct content authority is invalid")
		}
	case ProjectionContentChunked:
		if value.Blob != nil || value.Manifest == nil || value.Manifest.Validate() != nil ||
			value.Manifest.MediaType != "application/json" || value.ByteCount <= maxBlobBytes {
			return fmt.Errorf("projection chunked content authority is invalid")
		}
	default:
		return fmt.Errorf("projection content storage is not registered")
	}
	return nil
}

func (value ProjectionContentAuthority) ValidateForRole(role ChunkedBlobRole) error {
	if !validChunkedBlobRole(role) || value.Validate() != nil || value.Role != role {
		return fmt.Errorf("projection content authority has the wrong privacy role")
	}
	return nil
}

func cloneProjectionContent(value ProjectionContentAuthority) ProjectionContentAuthority {
	if value.Blob != nil {
		copy := *value.Blob
		value.Blob = &copy
	}
	if value.Manifest != nil {
		copy := *value.Manifest
		value.Manifest = &copy
	}
	return value
}

func cloneProjectionSidecars(values []ProjectionSidecarAuthority) []ProjectionSidecarAuthority {
	result := make([]ProjectionSidecarAuthority, len(values))
	for index, value := range values {
		value.Content = cloneProjectionContent(value.Content)
		result[index] = value
	}
	return result
}

func (verified VerifiedBase) ProjectionContent(value ProjectionContentAuthority) ([]byte, error) {
	if value.Validate() != nil {
		return nil, fmt.Errorf("projection content authority is invalid")
	}
	if value.Storage == ProjectionContentEmpty {
		return []byte{}, nil
	}
	if value.Storage == ProjectionContentDirect {
		blob, exists := verified.blobs[value.Blob.SHA256]
		if !exists || !value.Blob.matches(blob) {
			return nil, fmt.Errorf("projection direct content is missing or changed")
		}
		return bytes.Clone(blob.Data), nil
	}
	binding, exists := projectionChunkBinding(verified.manifest.ChunkedBlobs, *value.Manifest)
	if !exists {
		return nil, fmt.Errorf("projection chunked content manifest is unregistered")
	}
	raw, _, err := verifyChunkedBlobBinding(
		binding, verified.blobs, value.Role,
	)
	if err != nil || int64(len(raw)) != value.ByteCount || digestBytes(raw) != value.ContentSHA256 {
		return nil, fmt.Errorf("projection chunked content differs from logical authority: %v", err)
	}
	var manifest ChunkedBlobManifest
	manifestBlob := verified.blobs[value.Manifest.SHA256]
	if decodeCanonical(manifestBlob.Data, &manifest, "projection chunk manifest") != nil ||
		manifest.MediaType != value.MediaType {
		return nil, fmt.Errorf("projection chunked content media type changed")
	}
	return raw, nil
}

func projectionContentValues(value *ProjectionAuthority) []ProjectionContentAuthority {
	if value == nil {
		return nil
	}
	result := []ProjectionContentAuthority{
		value.PublicBundle, value.SealedEpisode, value.ProductionTrace,
	}
	for _, sidecar := range value.Sidecars {
		result = append(result, sidecar.Content)
	}
	return result
}

func manifestProjectionContentValues(manifest BaseManifest) []ProjectionContentAuthority {
	result := projectionContentValues(manifest.ProjectionAuthority)
	return append(result, ablationProjectionContentValues(manifest.AblationProjectionAuthority)...)
}

func projectionContentStorageRef(value ProjectionContentAuthority) BlobRef {
	if value.Storage == ProjectionContentEmpty {
		return BlobRef{}
	}
	if value.Storage == ProjectionContentDirect {
		return *value.Blob
	}
	return *value.Manifest
}

func validateProjectionContentBinding(
	value ProjectionContentAuthority,
	bindings []ChunkedBlobBinding,
	blobs map[string]Blob,
) error {
	if value.Validate() != nil {
		return fmt.Errorf("projection content authority is invalid")
	}
	if value.Storage == ProjectionContentEmpty {
		return nil
	}
	if value.Storage == ProjectionContentDirect {
		blob, exists := blobs[value.Blob.SHA256]
		if !exists || !value.Blob.matches(blob) {
			return fmt.Errorf("projection direct content is missing or changed")
		}
		return nil
	}
	binding, exists := projectionChunkBinding(bindings, *value.Manifest)
	if !exists {
		return fmt.Errorf("projection chunked content manifest is unregistered")
	}
	raw, _, err := verifyChunkedBlobBinding(binding, blobs, value.Role)
	if err != nil || int64(len(raw)) != value.ByteCount || digestBytes(raw) != value.ContentSHA256 {
		return fmt.Errorf("projection chunked content differs from logical authority: %v", err)
	}
	var manifest ChunkedBlobManifest
	manifestBlob := blobs[value.Manifest.SHA256]
	if decodeCanonical(manifestBlob.Data, &manifest, "projection chunk manifest") != nil ||
		manifest.MediaType != value.MediaType {
		return fmt.Errorf("projection chunked content media type changed")
	}
	return nil
}

func projectionChunkBinding(
	values []ChunkedBlobBinding,
	ref BlobRef,
) (ChunkedBlobBinding, bool) {
	for _, value := range values {
		if value.Manifest == ref {
			return value, true
		}
	}
	return ChunkedBlobBinding{}, false
}
