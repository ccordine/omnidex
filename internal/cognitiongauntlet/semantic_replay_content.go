package cognitiongauntlet

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func semanticReplayProjectionContent(
	id string,
	value any,
) (
	cognitionreplay.ProjectionContentAuthority,
	[]cognitionreplay.ChunkedBlobBinding,
	[]cognitionreplay.Blob,
	error,
) {
	raw, err := json.Marshal(value)
	if err != nil {
		return cognitionreplay.ProjectionContentAuthority{}, nil, nil,
			fmt.Errorf("encode semantic replay authority %q: %w", id, err)
	}
	raw = append(raw, '\n')
	return cognitionreplay.NewPublicProjectionContent(id, "application/json", raw)
}

func semanticReplayPolicyBodyContent(
	kind string,
	metadata semanticPolicyEvidence,
	raw []byte,
) (
	cognitionreplay.ProjectionContentAuthority,
	[]cognitionreplay.ChunkedBlobBinding,
	[]cognitionreplay.Blob,
	error,
) {
	if metadata.EvidenceKind != kind || len(raw) != metadata.Bytes ||
		digestExactBytes(raw) != metadata.ContentSHA256 {
		return cognitionreplay.ProjectionContentAuthority{}, nil, nil,
			fmt.Errorf("semantic policy body differs from its exact trace metadata")
	}
	if len(raw) == 0 {
		if kind != "provider_response_capture" {
			return cognitionreplay.ProjectionContentAuthority{}, nil, nil,
				fmt.Errorf("semantic %s body cannot be empty", kind)
		}
		empty, err := cognitionreplay.NewEmptyProjectionContent("application/octet-stream")
		return empty, []cognitionreplay.ChunkedBlobBinding{}, []cognitionreplay.Blob{}, err
	}
	return cognitionreplay.NewPublicProjectionContent(
		"policy-body-"+metadata.EvidenceID, "application/octet-stream", raw,
	)
}
