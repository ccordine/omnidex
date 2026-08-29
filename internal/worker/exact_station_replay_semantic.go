package worker

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func replayExactStationSemanticArtifact(
	job assemblyline.PortableJob,
	raw string,
	artifact ExactStationReplayArtifact,
) (ExactStationReplayArtifact, bool, error) {
	for _, validate := range []func(assemblyline.PortableJob, string) (bool, error){
		replayApplicationSemanticLeaf,
		replayRepositorySemanticLeaf,
		replayConversationSemanticLeaf,
		replayDatabaseSemanticLeaf,
		replayWebSemanticLeaf,
		replayGeneralSemanticLeaf,
	} {
		handled, err := validate(job, raw)
		if !handled {
			continue
		}
		artifact.Kind = string(job.Kind)
		if err != nil {
			return artifact, true, fmt.Errorf("decode replay %s: %w", job.Kind, err)
		}
		return artifact, true, nil
	}
	return artifact, false, nil
}

func decodeReplaySemanticLeaf[Input, Output any](
	job assemblyline.PortableJob,
	raw string,
	decode func(Input, string) (Output, error),
) error {
	var input Input
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		return fmt.Errorf("decode authority: %w", err)
	}
	_, err := decode(input, raw)
	return err
}
