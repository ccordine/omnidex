package cognitionreplay

import "fmt"

func bindChunkedBlobMediaTypes(
	bindings []ChunkedBlobBinding,
	bodies map[string][]byte,
	expectedRole ChunkedBlobRole,
	bind func(BlobRef) error,
) error {
	previous := ""
	for index, binding := range bindings {
		if binding.Validate() != nil ||
			(previous != "" && binding.Manifest.SHA256 <= previous) {
			return fmt.Errorf("chunked replay blob binding %d is invalid, duplicated, or reordered", index+1)
		}
		if err := bind(binding.Manifest); err != nil {
			return err
		}
		raw, exists := bodies[binding.Manifest.SHA256]
		if !exists {
			return fmt.Errorf("chunked replay manifest blob is missing")
		}
		var manifest ChunkedBlobManifest
		if err := decodeCanonical(raw, &manifest, "chunked replay blob manifest"); err != nil {
			return err
		}
		if err := validateChunkedBlobManifest(manifest, expectedRole); err != nil {
			return err
		}
		if err := validateChunkedBlobManifestBinding(binding, manifest); err != nil {
			return err
		}
		for _, chunk := range manifest.Chunks {
			if err := bind(chunk.Payload); err != nil {
				return err
			}
		}
		previous = binding.Manifest.SHA256
	}
	return nil
}
