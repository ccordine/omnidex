package worker

type ExactStationReplayArtifactError struct {
	Cause error
}

func (failure *ExactStationReplayArtifactError) Error() string {
	if failure == nil || failure.Cause == nil {
		return "station replay artifact is invalid"
	}
	return failure.Cause.Error()
}

func (failure *ExactStationReplayArtifactError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}
