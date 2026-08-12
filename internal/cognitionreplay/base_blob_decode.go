package cognitionreplay

import "fmt"

func bindBlobMediaTypes(
	manifest BaseManifest,
	sources []SourceRecord,
	events []Event,
	checkpoints []KnowledgeCheckpoint,
	chunked []ChunkedBlobBinding,
	bodies map[string][]byte,
) (map[string]Blob, error) {
	media := make(map[string]string, len(bodies))
	bind := func(ref BlobRef) error {
		if err := ref.Validate(); err != nil {
			return err
		}
		body, exists := bodies[ref.SHA256]
		if !exists || len(body) != ref.ByteCount || digestBytes(body) != ref.SHA256 {
			return fmt.Errorf("replay blob %s is missing or changed", ref.SHA256)
		}
		if prior, exists := media[ref.SHA256]; exists && prior != ref.MediaType {
			return fmt.Errorf("replay blob %s has conflicting media types", ref.SHA256)
		}
		media[ref.SHA256] = ref.MediaType
		return nil
	}
	if manifest.ProjectionAuthority != nil || manifest.AblationProjectionAuthority != nil {
		for _, content := range manifestProjectionContentValues(manifest) {
			if content.Storage == ProjectionContentEmpty {
				continue
			}
			if err := bind(projectionContentStorageRef(content)); err != nil {
				return nil, err
			}
		}
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
	for _, checkpoint := range checkpoints {
		for _, entry := range checkpoint.State.Entries {
			if err := bind(entry.Content); err != nil {
				return nil, err
			}
		}
		if checkpoint.Delta != nil {
			for _, entry := range checkpoint.Delta.Upserts {
				if err := bind(entry.Content); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := bindChunkedBlobMediaTypes(
		chunked, bodies, ChunkedBlobPublicAgentKnowledge, bind,
	); err != nil {
		return nil, err
	}
	if len(media) != len(bodies) {
		return nil, fmt.Errorf("replay contains an orphan content-addressed blob")
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
