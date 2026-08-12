package cognitionreplay

import (
	"bytes"
	"fmt"
)

func ChunkPublicBytes(
	id string,
	mediaType string,
	data []byte,
) (ChunkedBlobBinding, []Blob, error) {
	return chunkProjectionBytes(id, mediaType, data, ChunkedBlobPublicAgentKnowledge)
}

func chunkProjectionBytes(
	id string,
	mediaType string,
	data []byte,
	role ChunkedBlobRole,
) (ChunkedBlobBinding, []Blob, error) {
	if requireExact(id, "chunked replay blob ID") != nil ||
		!validMediaType(mediaType) || !validChunkedBlobRole(role) || len(data) <= maxBlobBytes ||
		len(data) > maxChunkedBlobBytes {
		return ChunkedBlobBinding{}, nil,
			fmt.Errorf("chunked replay byte authority is invalid")
	}
	manifest, chunks, err := readChunkedBlob(
		bytes.NewReader(data), id, mediaType, role, int64(len(data)),
	)
	if err != nil {
		return ChunkedBlobBinding{}, nil, err
	}
	manifestRaw, err := marshalCanonical(manifest)
	if err != nil {
		return ChunkedBlobBinding{}, nil, err
	}
	manifestBlob, err := NewBlob("application/json", manifestRaw)
	if err != nil {
		return ChunkedBlobBinding{}, nil, err
	}
	blobs := []Blob{manifestBlob}
	seen := map[string]struct{}{manifestBlob.SHA256: {}}
	for _, chunk := range chunks {
		if _, duplicate := seen[chunk.SHA256]; duplicate {
			continue
		}
		seen[chunk.SHA256] = struct{}{}
		blobs = append(blobs, chunk)
	}
	binding := ChunkedBlobBinding{Manifest: manifestBlob.Ref()}
	if _, err := VerifyChunkedBlob(binding, blobs, role); err != nil {
		return ChunkedBlobBinding{}, nil, fmt.Errorf("verify chunked replay bytes: %w", err)
	}
	return binding, blobs, nil
}
