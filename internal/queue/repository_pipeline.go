package queue

func stepsForJob(metadataJSON []byte) ([]stepSeed, error) {
	metadata, err := decodeMetadataObject(metadataJSON)
	if err != nil {
		return nil, err
	}
	if err := ValidateJobMetadataAuthority(metadata); err != nil {
		return nil, err
	}
	return []stepSeed{{action: "v3_coding", sortIndex: 5}}, nil
}

func conversationObjectiveSteps() []stepSeed {
	return []stepSeed{{action: "objective_resolve", sortIndex: 5}}
}
