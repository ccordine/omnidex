package cognitiongauntlet

import "fmt"

func appendRuntimeProviderActivationTrace(
	recorder *EpisodeRecorder,
	authority RuntimeProviderActivationEvidenceAuthority,
) error {
	if err := authority.Validate(); err != nil {
		return err
	}
	payload, err := traceJSONObject(authority)
	if err != nil {
		return err
	}
	return recorder.Append(
		TraceProviderActivation,
		"runtime-provider-activation-"+authority.SHA256,
		nil,
		payload,
	)
}

func decodeRuntimeProviderActivationTrace(
	entry TraceEntry,
) (RuntimeProviderActivationEvidenceAuthority, error) {
	var authority RuntimeProviderActivationEvidenceAuthority
	if entry.Kind != TraceProviderActivation || entry.Revision != nil ||
		decodeTracePayload(entry.Payload, &authority, "runtime provider activation trace") != nil ||
		authority.Validate() != nil || entry.ID != "runtime-provider-activation-"+authority.SHA256 {
		return RuntimeProviderActivationEvidenceAuthority{},
			fmt.Errorf("runtime provider activation trace authority is invalid")
	}
	return authority, nil
}
