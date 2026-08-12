package cognitionreplay

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxChunkedBlobBytes = 256 * 1024 * 1024

func ChunkPublicFile(
	path string,
	id string,
	mediaType string,
) (ChunkedBlobBinding, []Blob, error) {
	return chunkExactFile(path, id, mediaType, ChunkedBlobPublicAgentKnowledge)
}

func ChunkPrivateFile(
	path string,
	id string,
	mediaType string,
) (ChunkedBlobBinding, []Blob, error) {
	return chunkExactFile(path, id, mediaType, ChunkedBlobPrivateWorld)
}

func chunkExactFile(
	path string,
	id string,
	mediaType string,
	role ChunkedBlobRole,
) (ChunkedBlobBinding, []Blob, error) {
	return chunkExactFileWithPostRead(path, id, mediaType, role, nil)
}

func chunkExactFileWithPostRead(
	path string,
	id string,
	mediaType string,
	role ChunkedBlobRole,
	postRead func() error,
) (ChunkedBlobBinding, []Blob, error) {
	if path == "" || filepath.Clean(path) != path || requireExact(id, "chunked replay blob ID") != nil ||
		!validMediaType(mediaType) || !validChunkedBlobRole(role) {
		return ChunkedBlobBinding{}, nil, fmt.Errorf("chunked replay file authority is invalid")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return ChunkedBlobBinding{}, nil, fmt.Errorf("inspect chunked replay file: %w", err)
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Size() <= 0 || pathInfo.Size() > maxChunkedBlobBytes {
		return ChunkedBlobBinding{}, nil, fmt.Errorf("chunked replay source is not one bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return ChunkedBlobBinding{}, nil, fmt.Errorf("open chunked replay file: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) ||
		openedInfo.Size() != pathInfo.Size() {
		return ChunkedBlobBinding{}, nil, fmt.Errorf("chunked replay source file changed before reading")
	}
	manifest, chunks, err := readChunkedBlob(file, id, mediaType, role, pathInfo.Size())
	if err != nil {
		return ChunkedBlobBinding{}, nil, err
	}
	if postRead != nil {
		if err := postRead(); err != nil {
			return ChunkedBlobBinding{}, nil, fmt.Errorf("exercise chunked replay read boundary: %w", err)
		}
	}
	finalInfo, err := file.Stat()
	finalPathInfo, pathErr := os.Lstat(path)
	if err != nil || pathErr != nil || !finalPathInfo.Mode().IsRegular() ||
		!os.SameFile(pathInfo, finalInfo) || !os.SameFile(pathInfo, finalPathInfo) ||
		finalInfo.Size() != pathInfo.Size() || finalPathInfo.Size() != pathInfo.Size() ||
		!finalInfo.ModTime().Equal(pathInfo.ModTime()) ||
		!finalPathInfo.ModTime().Equal(pathInfo.ModTime()) {
		return ChunkedBlobBinding{}, nil, fmt.Errorf("chunked replay source file changed while reading")
	}
	manifestRaw, err := marshalCanonical(manifest)
	if err != nil {
		return ChunkedBlobBinding{}, nil, err
	}
	manifestBlob, err := NewBlob("application/json", manifestRaw)
	if err != nil {
		return ChunkedBlobBinding{}, nil, err
	}
	blobs := make([]Blob, 0, len(chunks)+1)
	blobs = append(blobs, manifestBlob)
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
		return ChunkedBlobBinding{}, nil, fmt.Errorf("verify prepared chunked replay blob: %w", err)
	}
	return binding, blobs, nil
}

func readChunkedBlob(
	reader io.Reader,
	id string,
	mediaType string,
	role ChunkedBlobRole,
	byteCount int64,
) (ChunkedBlobManifest, []Blob, error) {
	hash := sha256.New()
	remaining := byteCount
	offset := int64(0)
	chunks := make([]Blob, 0, (byteCount+maxBlobBytes-1)/maxBlobBytes)
	refs := make([]ChunkedBlobChunk, 0, cap(chunks))
	for remaining > 0 {
		size := int64(maxBlobBytes)
		if remaining < size {
			size = remaining
		}
		raw := make([]byte, int(size))
		if _, err := io.ReadFull(reader, raw); err != nil {
			return ChunkedBlobManifest{}, nil, fmt.Errorf("read chunked replay source: %w", err)
		}
		_, _ = hash.Write(raw)
		blob, err := NewBlob("application/octet-stream", raw)
		if err != nil {
			return ChunkedBlobManifest{}, nil, err
		}
		chunks = append(chunks, blob)
		refs = append(refs, ChunkedBlobChunk{
			Ordinal: uint64(len(refs) + 1), Offset: offset, Payload: blob.Ref(),
		})
		offset += size
		remaining -= size
	}
	var trailing [1]byte
	if count, err := reader.Read(trailing[:]); count != 0 || err != io.EOF {
		return ChunkedBlobManifest{}, nil, fmt.Errorf("chunked replay source byte count changed")
	}
	return ChunkedBlobManifest{
		Schema: ChunkedBlobManifestSchemaV1, Role: role, ID: id, MediaType: mediaType,
		ContentSHA256: hex.EncodeToString(hash.Sum(nil)), ByteCount: byteCount,
		ChunkCount: len(refs), Chunks: refs,
	}, chunks, nil
}
