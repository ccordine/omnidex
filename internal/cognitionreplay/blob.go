package cognitionreplay

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type BlobRef struct {
	SHA256    string `json:"sha256"`
	ByteCount int    `json:"byte_count"`
	MediaType string `json:"media_type"`
}

type Blob struct {
	SHA256    string `json:"sha256"`
	MediaType string `json:"media_type"`
	Data      []byte `json:"-"`
}

func NewBlob(mediaType string, data []byte) (Blob, error) {
	value := Blob{SHA256: digestBytes(data), MediaType: mediaType, Data: append([]byte(nil), data...)}
	return value, value.Validate()
}

func (blob Blob) Ref() BlobRef {
	return BlobRef{SHA256: blob.SHA256, ByteCount: len(blob.Data), MediaType: blob.MediaType}
}

func (blob Blob) Validate() error {
	if !validDigest(blob.SHA256) || blob.SHA256 != digestBytes(blob.Data) ||
		len(blob.Data) == 0 || len(blob.Data) > maxBlobBytes || !validMediaType(blob.MediaType) {
		return fmt.Errorf("replay blob authority is invalid")
	}
	if blob.MediaType == "application/json" && !json.Valid(blob.Data) {
		return fmt.Errorf("replay JSON blob is invalid")
	}
	return nil
}

func (ref BlobRef) Validate() error {
	if !validDigest(ref.SHA256) || ref.ByteCount <= 0 || ref.ByteCount > maxBlobBytes ||
		!validMediaType(ref.MediaType) {
		return fmt.Errorf("replay blob reference is invalid")
	}
	return nil
}

func (ref BlobRef) matches(blob Blob) bool {
	return ref.SHA256 == blob.SHA256 && ref.ByteCount == len(blob.Data) && ref.MediaType == blob.MediaType
}

func validMediaType(value string) bool {
	switch value {
	case "application/json", "application/octet-stream", "text/plain; charset=utf-8":
		return true
	default:
		return false
	}
}

func cloneBlobs(values []Blob) []Blob {
	result := make([]Blob, len(values))
	for index, value := range values {
		value.Data = bytes.Clone(value.Data)
		result[index] = value
	}
	return result
}
