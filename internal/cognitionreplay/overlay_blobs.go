package cognitionreplay

import "fmt"

func validatePrivateBlobClosure(
	sources []PrivateSource,
	events []PrivateEvent,
	frames []PrivateFrame,
	chunked []ChunkedBlobBinding,
	blobs map[string]Blob,
) error {
	used := make(map[string]BlobRef, len(blobs))
	use := func(ref BlobRef) error {
		blob, exists := blobs[ref.SHA256]
		if ref.Validate() != nil || !exists || !ref.matches(blob) {
			return fmt.Errorf("private replay blob %s is missing or changed", ref.SHA256)
		}
		if prior, exists := used[ref.SHA256]; exists && prior.MediaType != ref.MediaType {
			return fmt.Errorf("private replay blob %s has conflicting media types", ref.SHA256)
		}
		used[ref.SHA256] = ref
		return nil
	}
	for _, source := range sources {
		if err := use(source.Payload); err != nil {
			return err
		}
	}
	for _, event := range events {
		if err := use(event.Payload); err != nil {
			return err
		}
	}
	for _, frame := range frames {
		if err := use(frame.Snapshot); err != nil {
			return err
		}
		if frame.Delta != nil {
			if err := use(*frame.Delta); err != nil {
				return err
			}
		}
	}
	for _, binding := range chunked {
		if _, cited := used[binding.Manifest.SHA256]; !cited {
			return fmt.Errorf("private chunked replay manifest is not cited by a private record")
		}
		_, chunkRefs, err := verifyChunkedBlobBinding(binding, blobs, ChunkedBlobPrivateWorld)
		if err != nil {
			return err
		}
		for digest, ref := range chunkRefs {
			if digest == binding.Manifest.SHA256 {
				continue
			}
			if err := use(ref); err != nil {
				return err
			}
		}
	}
	if len(used) != len(blobs) {
		return fmt.Errorf("private replay contains an orphan content-addressed blob")
	}
	for digest, blob := range blobs {
		if digest != blob.SHA256 || blob.Validate() != nil {
			return fmt.Errorf("private replay blob set is invalid")
		}
	}
	return nil
}

func bindPrivateBlobMediaTypes(
	sources []PrivateSource,
	events []PrivateEvent,
	frames []PrivateFrame,
	chunked []ChunkedBlobBinding,
	bodies map[string][]byte,
) (map[string]Blob, error) {
	media := make(map[string]string, len(bodies))
	bind := func(ref BlobRef) error {
		body, exists := bodies[ref.SHA256]
		if ref.Validate() != nil || !exists || len(body) != ref.ByteCount ||
			digestBytes(body) != ref.SHA256 {
			return fmt.Errorf("private replay blob %s is missing or changed", ref.SHA256)
		}
		if prior, exists := media[ref.SHA256]; exists && prior != ref.MediaType {
			return fmt.Errorf("private replay blob %s has conflicting media types", ref.SHA256)
		}
		media[ref.SHA256] = ref.MediaType
		return nil
	}
	for _, source := range sources {
		if err := bind(source.Payload); err != nil {
			return nil, err
		}
	}
	for _, event := range events {
		if err := bind(event.Payload); err != nil {
			return nil, err
		}
	}
	for _, frame := range frames {
		if err := bind(frame.Snapshot); err != nil {
			return nil, err
		}
		if frame.Delta != nil {
			if err := bind(*frame.Delta); err != nil {
				return nil, err
			}
		}
	}
	if err := bindChunkedBlobMediaTypes(
		chunked, bodies, ChunkedBlobPrivateWorld, bind,
	); err != nil {
		return nil, err
	}
	if len(media) != len(bodies) {
		return nil, fmt.Errorf("private replay contains an orphan content-addressed blob")
	}
	result := make(map[string]Blob, len(bodies))
	for digest, body := range bodies {
		blob := Blob{SHA256: digest, MediaType: media[digest], Data: append([]byte(nil), body...)}
		if err := blob.Validate(); err != nil {
			return nil, err
		}
		result[digest] = blob
	}
	return result, nil
}
