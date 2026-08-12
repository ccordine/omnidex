package cognitionreplay

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestChunkedBlobFileIsDeterministicAndRoleBound(t *testing.T) {
	data := bytes.Repeat([]byte("replay-evidence-"), maxBlobBytes/16+2)
	path := filepath.Join(t.TempDir(), "evidence.bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	firstBinding, firstBlobs, err := ChunkPublicFile(
		path, "provider-evidence", "application/octet-stream",
	)
	if err != nil {
		t.Fatal(err)
	}
	secondBinding, secondBlobs, err := ChunkPublicFile(
		path, "provider-evidence", "application/octet-stream",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstBinding, secondBinding) || !reflect.DeepEqual(firstBlobs, secondBlobs) {
		t.Fatal("identical exact files did not produce byte-identical chunk authority")
	}
	reassembled, err := VerifyChunkedBlob(
		firstBinding, firstBlobs, ChunkedBlobPublicAgentKnowledge,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reassembled, data) {
		t.Fatal("chunked replay blob did not reassemble exact source bytes")
	}
	if _, err := VerifyChunkedBlob(
		firstBinding, firstBlobs, ChunkedBlobPrivateWorld,
	); err == nil {
		t.Fatal("public chunk authority was accepted for private world data")
	}
	manifest := decodeChunkManifestForTest(t, firstBinding, firstBlobs)
	if manifest.ChunkCount != 2 || len(manifest.Chunks) != 2 ||
		manifest.Chunks[0].Offset != 0 || manifest.Chunks[0].Payload.ByteCount != maxBlobBytes ||
		manifest.Chunks[1].Offset != int64(maxBlobBytes) {
		t.Fatal("chunk manifest does not preserve fixed exact boundaries")
	}
}

func TestChunkedBlobRejectsMissingExtraDuplicateReorderedAndAlteredData(t *testing.T) {
	data := append(bytes.Repeat([]byte("a"), maxBlobBytes), []byte("tail")...)
	path := filepath.Join(t.TempDir(), "evidence.bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	binding, blobs, err := ChunkPublicFile(path, "evidence", "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("missing", func(t *testing.T) {
		if _, err := VerifyChunkedBlob(
			binding, append([]Blob(nil), blobs[:len(blobs)-1]...), ChunkedBlobPublicAgentKnowledge,
		); err == nil {
			t.Fatal("missing chunk was accepted")
		}
	})
	t.Run("extra", func(t *testing.T) {
		extra, err := NewBlob("application/octet-stream", []byte("extra"))
		if err != nil {
			t.Fatal(err)
		}
		values := append(append([]Blob(nil), blobs...), extra)
		if _, err := VerifyChunkedBlob(
			binding, values, ChunkedBlobPublicAgentKnowledge,
		); err == nil {
			t.Fatal("extra chunk blob was accepted")
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		values := append(append([]Blob(nil), blobs...), blobs[0])
		if _, err := VerifyChunkedBlob(
			binding, values, ChunkedBlobPublicAgentKnowledge,
		); err == nil {
			t.Fatal("duplicate chunk blob was accepted")
		}
	})
	t.Run("altered", func(t *testing.T) {
		values := cloneBlobs(blobs)
		values[len(values)-1].Data[0] ^= 0xff
		if _, err := VerifyChunkedBlob(
			binding, values, ChunkedBlobPublicAgentKnowledge,
		); err == nil {
			t.Fatal("altered chunk bytes were accepted")
		}
	})
	t.Run("reordered", func(t *testing.T) {
		manifest := decodeChunkManifestForTest(t, binding, blobs)
		manifest.Chunks[0].Payload, manifest.Chunks[1].Payload =
			manifest.Chunks[1].Payload, manifest.Chunks[0].Payload
		manifestRaw, err := marshalCanonical(manifest)
		if err != nil {
			t.Fatal(err)
		}
		manifestBlob, err := NewBlob("application/json", manifestRaw)
		if err != nil {
			t.Fatal(err)
		}
		values := replaceChunkManifestBlob(blobs, binding.Manifest.SHA256, manifestBlob)
		binding.Manifest = manifestBlob.Ref()
		if _, err := VerifyChunkedBlob(
			binding, values, ChunkedBlobPublicAgentKnowledge,
		); err == nil {
			t.Fatal("reordered chunks were accepted")
		}
	})
}

func TestChunkedBlobFileRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.bin")
	link := filepath.Join(directory, "link.bin")
	if err := os.WriteFile(target, []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ChunkPublicFile(
		link, "provider-evidence", "application/octet-stream",
	); err == nil {
		t.Fatal("symlinked chunk source was accepted")
	}
}

func TestChunkedBlobFileRejectsMutationBeforeOpenHandleVerification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.bin")
	data := bytes.Repeat([]byte("a"), 4096)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	initial, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = chunkExactFileWithPostRead(
		path, "provider-evidence", "application/octet-stream",
		ChunkedBlobPublicAgentKnowledge,
		func() error {
			if err := os.WriteFile(path, bytes.Repeat([]byte("b"), len(data)), 0o600); err != nil {
				return err
			}
			changed := initial.ModTime().Add(2 * time.Second)
			return os.Chtimes(path, changed, changed)
		},
	)
	if err == nil {
		t.Fatal("source mutation while its read handle was open was accepted")
	}
}

func TestChunkedBlobFileRejectsPathReplacementWhileHandleIsOpen(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "evidence.bin")
	moved := filepath.Join(directory, "moved.bin")
	data := bytes.Repeat([]byte("a"), 4096)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := chunkExactFileWithPostRead(
		path, "provider-evidence", "application/octet-stream",
		ChunkedBlobPublicAgentKnowledge,
		func() error {
			if err := os.Rename(path, moved); err != nil {
				return err
			}
			return os.WriteFile(path, data, 0o600)
		},
	)
	if err == nil {
		t.Fatal("replacement path was accepted as the file held open for chunking")
	}
}

func TestChunkedBlobManifestCannotCiteItsOwnManifestBlob(t *testing.T) {
	binding := ChunkedBlobBinding{Manifest: BlobRef{
		SHA256: testDigest("manifest"), ByteCount: 17, MediaType: "application/json",
	}}
	manifest := ChunkedBlobManifest{Chunks: []ChunkedBlobChunk{{
		Ordinal: 1, Offset: 0, Payload: BlobRef{
			SHA256: binding.Manifest.SHA256, ByteCount: 17, MediaType: "application/octet-stream",
		},
	}}}
	if err := validateChunkedBlobManifestBinding(binding, manifest); err == nil {
		t.Fatal("chunk manifest accepted itself as a content chunk")
	}
}

func decodeChunkManifestForTest(
	t *testing.T,
	binding ChunkedBlobBinding,
	blobs []Blob,
) ChunkedBlobManifest {
	t.Helper()
	for _, blob := range blobs {
		if blob.SHA256 != binding.Manifest.SHA256 {
			continue
		}
		var manifest ChunkedBlobManifest
		if err := decodeCanonical(blob.Data, &manifest, "test chunk manifest"); err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	t.Fatal("chunk manifest blob is missing")
	return ChunkedBlobManifest{}
}

func replaceChunkManifestBlob(
	values []Blob,
	want string,
	replacement Blob,
) []Blob {
	result := cloneBlobs(values)
	for index := range result {
		if result[index].SHA256 == want {
			result[index] = replacement
			return result
		}
	}
	return append(result, replacement)
}
