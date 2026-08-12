package cognitionreplay

import "fmt"

const ChunkedBlobManifestSchemaV1 = "omnidex.replay-chunked-blob.v1"

type ChunkedBlobRole string

const (
	ChunkedBlobPublicAgentKnowledge ChunkedBlobRole = "public_agent_knowledge"
	ChunkedBlobPrivateWorld         ChunkedBlobRole = "private_world"
)

type ChunkedBlobChunk struct {
	Ordinal uint64  `json:"ordinal"`
	Offset  int64   `json:"offset"`
	Payload BlobRef `json:"payload"`
}

type ChunkedBlobManifest struct {
	Schema        string             `json:"schema"`
	Role          ChunkedBlobRole    `json:"role"`
	ID            string             `json:"id"`
	MediaType     string             `json:"media_type"`
	ContentSHA256 string             `json:"content_sha256"`
	ByteCount     int64              `json:"byte_count"`
	ChunkCount    int                `json:"chunk_count"`
	Chunks        []ChunkedBlobChunk `json:"chunks"`
}

// ChunkedBlobBinding registers one canonical manifest blob with a replay
// container. A public base and private overlay require different immutable
// role values, so the same manifest cannot cross the privacy boundary.
type ChunkedBlobBinding struct {
	Manifest BlobRef `json:"manifest"`
}

func (binding ChunkedBlobBinding) Validate() error {
	if binding.Manifest.Validate() != nil || binding.Manifest.MediaType != "application/json" {
		return fmt.Errorf("chunked replay blob binding is invalid")
	}
	return nil
}

func validChunkedBlobRole(role ChunkedBlobRole) bool {
	return role == ChunkedBlobPublicAgentKnowledge || role == ChunkedBlobPrivateWorld
}

func cloneChunkedBlobBindings(values []ChunkedBlobBinding) []ChunkedBlobBinding {
	result := make([]ChunkedBlobBinding, len(values))
	copy(result, values)
	return result
}
