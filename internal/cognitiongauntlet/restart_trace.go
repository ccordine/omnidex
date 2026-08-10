package cognitiongauntlet

import "fmt"

const RestartTraceSchemaV1 = "omnidex.cognition-restart-trace.v1"

type RestartTrace struct {
	Schema          string `json:"schema"`
	ID              string `json:"id"`
	CompletedCycles uint32 `json:"completed_cycles"`
	BeforeSHA256    string `json:"before_sha256"`
	AfterSHA256     string `json:"after_sha256"`
	StateIdentical  bool   `json:"state_identical"`
}

func NewRestartTrace(cycles uint32, before, after string) (RestartTrace, error) {
	trace := RestartTrace{
		Schema: RestartTraceSchemaV1, CompletedCycles: cycles,
		BeforeSHA256: before, AfterSHA256: after, StateIdentical: before == after,
	}
	digest, err := digestJSON(struct {
		Cycles uint32 `json:"cycles"`
		Before string `json:"before"`
		After  string `json:"after"`
	}{cycles, before, after})
	if err != nil {
		return RestartTrace{}, err
	}
	trace.ID = "restart-" + digest
	return trace, trace.Validate()
}

func (trace RestartTrace) Validate() error {
	if trace.Schema != RestartTraceSchemaV1 ||
		requireExact(trace.ID, "restart trace ID", 512) != nil || trace.CompletedCycles == 0 ||
		!validDigest(trace.BeforeSHA256) || !validDigest(trace.AfterSHA256) ||
		!trace.StateIdentical || trace.BeforeSHA256 != trace.AfterSHA256 {
		return fmt.Errorf("cognition restart did not restore identical durable state")
	}
	return nil
}
