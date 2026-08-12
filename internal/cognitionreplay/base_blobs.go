package cognitionreplay

import "fmt"

func validateBaseBlobClosure(
	manifest BaseManifest,
	sources []SourceRecord,
	events []Event,
	checkpoints []KnowledgeCheckpoint,
	chunked []ChunkedBlobBinding,
	blobs map[string]Blob,
) error {
	used := make(map[string]BlobRef, len(blobs))
	use := func(ref BlobRef) error {
		if err := ref.Validate(); err != nil {
			return err
		}
		blob, exists := blobs[ref.SHA256]
		if !exists || !ref.matches(blob) {
			return fmt.Errorf("replay blob %s is missing or changed", ref.SHA256)
		}
		if prior, exists := used[ref.SHA256]; exists && prior.MediaType != ref.MediaType {
			return fmt.Errorf("replay blob %s has conflicting media types", ref.SHA256)
		}
		used[ref.SHA256] = ref
		return nil
	}
	if manifest.ProjectionAuthority != nil || manifest.AblationProjectionAuthority != nil {
		for _, content := range manifestProjectionContentValues(manifest) {
			if content.Storage == ProjectionContentEmpty {
				continue
			}
			if err := use(projectionContentStorageRef(content)); err != nil {
				return err
			}
		}
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
	for _, checkpoint := range checkpoints {
		for _, entry := range checkpoint.State.Entries {
			if err := use(entry.Content); err != nil {
				return err
			}
		}
		if checkpoint.Delta != nil {
			for _, entry := range checkpoint.Delta.Upserts {
				if err := use(entry.Content); err != nil {
					return err
				}
			}
		}
	}
	for _, binding := range chunked {
		if _, cited := used[binding.Manifest.SHA256]; !cited {
			return fmt.Errorf("public chunked replay manifest is not cited by a public record")
		}
		_, chunkRefs, err := verifyChunkedBlobBinding(
			binding, blobs, ChunkedBlobPublicAgentKnowledge,
		)
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
	for _, content := range manifestProjectionContentValues(manifest) {
		if err := validateProjectionContentBinding(content, chunked, blobs); err != nil {
			return err
		}
	}
	if len(used) != len(blobs) {
		return fmt.Errorf("public replay contains an orphan content-addressed blob")
	}
	for digest, blob := range blobs {
		if digest != blob.SHA256 || blob.Validate() != nil {
			return fmt.Errorf("public replay blob set is invalid")
		}
	}
	return nil
}
