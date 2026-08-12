package cognitiongauntlet

import "fmt"

func appendAblationEvidenceTrace(
	recorder *EpisodeRecorder,
	authority AblationEvidenceAuthority,
) error {
	if err := authority.Validate(); err != nil {
		return err
	}
	payload, err := traceJSONObject(authority)
	if err != nil {
		return err
	}
	return recorder.Append(
		TraceAblationEvidence, authority.ID, nil, payload,
	)
}

func decodeAblationEvidenceTrace(
	entry TraceEntry,
) (AblationEvidenceAuthority, error) {
	var authority AblationEvidenceAuthority
	if entry.Kind != TraceAblationEvidence || entry.Revision != nil ||
		decodeTracePayload(entry.Payload, &authority, "ablation evidence trace") != nil ||
		authority.Validate() != nil || entry.ID != authority.ID {
		return AblationEvidenceAuthority{}, fmt.Errorf(
			"ablation evidence trace authority is invalid",
		)
	}
	return authority, nil
}

func ablationEvidenceAuthorityFromEpisode(
	sealed SealedEpisode,
) (AblationEvidenceAuthority, error) {
	var authority AblationEvidenceAuthority
	count := 0
	for _, entry := range sealed.Manifest.Trace {
		if entry.Kind != TraceAblationEvidence {
			continue
		}
		decoded, err := decodeAblationEvidenceTrace(entry)
		if err != nil {
			return AblationEvidenceAuthority{}, err
		}
		authority, count = decoded, count+1
	}
	if count != 1 {
		return AblationEvidenceAuthority{}, fmt.Errorf(
			"sealed ablation does not contain one evidence authority",
		)
	}
	return authority, nil
}
