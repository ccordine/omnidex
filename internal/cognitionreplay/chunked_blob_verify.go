package cognitionreplay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

func VerifyChunkedBlob(
	binding ChunkedBlobBinding,
	blobs []Blob,
	expectedRole ChunkedBlobRole,
) ([]byte, error) {
	byDigest := make(map[string]Blob, len(blobs))
	for _, blob := range blobs {
		if blob.Validate() != nil {
			return nil, fmt.Errorf("chunked replay blob set is invalid")
		}
		if _, duplicate := byDigest[blob.SHA256]; duplicate {
			return nil, fmt.Errorf("chunked replay blob set contains a duplicate")
		}
		byDigest[blob.SHA256] = blob
	}
	raw, used, err := verifyChunkedBlobBinding(binding, byDigest, expectedRole)
	if err != nil {
		return nil, err
	}
	if len(used) != len(byDigest) {
		return nil, fmt.Errorf("chunked replay blob set contains an extra blob")
	}
	return raw, nil
}

func verifyChunkedBlobBinding(
	binding ChunkedBlobBinding,
	blobs map[string]Blob,
	expectedRole ChunkedBlobRole,
) ([]byte, map[string]BlobRef, error) {
	if binding.Validate() != nil || !validChunkedBlobRole(expectedRole) {
		return nil, nil, fmt.Errorf("chunked replay blob authority is invalid")
	}
	manifestBlob, exists := blobs[binding.Manifest.SHA256]
	if !exists || !binding.Manifest.matches(manifestBlob) {
		return nil, nil, fmt.Errorf("chunked replay manifest blob is missing or changed")
	}
	var manifest ChunkedBlobManifest
	if err := decodeCanonical(manifestBlob.Data, &manifest, "chunked replay blob manifest"); err != nil {
		return nil, nil, err
	}
	if err := validateChunkedBlobManifest(manifest, expectedRole); err != nil {
		return nil, nil, err
	}
	if err := validateChunkedBlobManifestBinding(binding, manifest); err != nil {
		return nil, nil, err
	}
	used := map[string]BlobRef{binding.Manifest.SHA256: binding.Manifest}
	var assembled bytes.Buffer
	assembled.Grow(int(manifest.ByteCount))
	for _, chunk := range manifest.Chunks {
		blob, exists := blobs[chunk.Payload.SHA256]
		if !exists || !chunk.Payload.matches(blob) {
			return nil, nil, fmt.Errorf("chunked replay payload chunk %d is missing or changed", chunk.Ordinal)
		}
		used[chunk.Payload.SHA256] = chunk.Payload
		_, _ = assembled.Write(blob.Data)
	}
	raw := assembled.Bytes()
	if int64(len(raw)) != manifest.ByteCount || digestBytes(raw) != manifest.ContentSHA256 ||
		!validLogicalBlob(mediaTypeAndData{mediaType: manifest.MediaType, data: raw}) {
		return nil, nil, fmt.Errorf("chunked replay payload did not reassemble to its exact authority")
	}
	return bytes.Clone(raw), used, nil
}

func validateChunkedBlobManifest(
	manifest ChunkedBlobManifest,
	expectedRole ChunkedBlobRole,
) error {
	if manifest.Schema != ChunkedBlobManifestSchemaV1 || manifest.Role != expectedRole ||
		requireExact(manifest.ID, "chunked replay blob ID") != nil ||
		!validMediaType(manifest.MediaType) || !validDigest(manifest.ContentSHA256) ||
		manifest.ByteCount <= 0 || manifest.ByteCount > maxChunkedBlobBytes ||
		manifest.ChunkCount <= 0 || manifest.ChunkCount != len(manifest.Chunks) ||
		manifest.Chunks == nil {
		return fmt.Errorf("chunked replay blob manifest authority is invalid")
	}
	expectedOffset := int64(0)
	for index, chunk := range manifest.Chunks {
		if chunk.Ordinal != uint64(index+1) || chunk.Offset != expectedOffset ||
			chunk.Payload.Validate() != nil || chunk.Payload.MediaType != "application/octet-stream" ||
			(index < len(manifest.Chunks)-1 && chunk.Payload.ByteCount != maxBlobBytes) {
			return fmt.Errorf("chunked replay blob chunk %d authority is invalid", index+1)
		}
		expectedOffset += int64(chunk.Payload.ByteCount)
	}
	if expectedOffset != manifest.ByteCount {
		return fmt.Errorf("chunked replay blob chunk byte count changed")
	}
	return nil
}

func validateChunkedBlobManifestBinding(
	binding ChunkedBlobBinding,
	manifest ChunkedBlobManifest,
) error {
	for _, chunk := range manifest.Chunks {
		if chunk.Payload.SHA256 == binding.Manifest.SHA256 {
			return fmt.Errorf("chunked replay manifest cannot cite itself as content")
		}
	}
	return nil
}

type mediaTypeAndData struct {
	mediaType string
	data      []byte
}

func validLogicalBlob(value mediaTypeAndData) bool {
	switch value.mediaType {
	case "application/json":
		return json.Valid(value.data)
	case "text/plain; charset=utf-8":
		return utf8.Valid(value.data)
	case "application/octet-stream":
		return true
	default:
		return false
	}
}
