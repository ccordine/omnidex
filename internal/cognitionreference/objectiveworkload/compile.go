package objectiveworkload

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func Compile(
	ctx context.Context,
	exactAuthority string,
	station PartitionStation,
	limits CompileLimits,
) (CompileResult, error) {
	result := CompileResult{
		EvidenceClass: EvidencePrimitiveContaminatedNonAutonomy,
		Gaps:          []PartitionGapRecord{},
	}
	if ctx == nil {
		return result, fmt.Errorf("%w: context is required", ErrInvalidCompile)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := limits.validate(); err != nil {
		return result, err
	}
	authority, err := newAuthority(exactAuthority)
	if err != nil {
		return result, err
	}
	result.Authority = authority
	result.CompilationID = compilationIdentity(authority)
	if nilPartitionStation(station) {
		return result, fmt.Errorf("%w: requirement partition station is required", ErrInvalidCompile)
	}
	counter := &countingPartitionStation{delegate: station, calls: &result.StationCalls}
	sequence := 0
	decision, err := assemblyline.CompleteRequirementPartition(
		exactAuthority,
		func(input assemblyline.RequirementPartitionInput) (assemblyline.RequirementPartitionDecision, error) {
			if result.StationCalls >= limits.MaxStationCalls {
				return assemblyline.RequirementPartitionDecision{}, fmt.Errorf(
					"%w: exceeded %d station calls", ErrCompileBound, limits.MaxStationCalls,
				)
			}
			job, err := assemblyline.NewRequirementPartitionJob(input)
			if err != nil {
				return assemblyline.RequirementPartitionDecision{}, err
			}
			sequence++
			frozen := clonePortableJob(job)
			dispatched := clonePortableJob(frozen)
			gap := PartitionGapRecord{
				ID:            partitionGapIdentity(result.CompilationID, sequence, frozen),
				Kind:          GapRequirementPartition,
				CompilationID: result.CompilationID,
				JobID:         frozen.ID,
				Mode:          input.Mode,
				InputSHA256:   digestBytes(frozen.Payload),
				Status:        GapOpened,
			}
			result.Gaps = append(result.Gaps, gap)
			gapIndex := len(result.Gaps) - 1
			if err := ctx.Err(); err != nil {
				result.Gaps[gapIndex].Status = GapFailed
				return assemblyline.RequirementPartitionDecision{}, err
			}
			result.Gaps[gapIndex].Status = GapDispatched
			portableResult, callErr := counter.Generate(ctx, dispatched)
			var receiptErr error
			if callErr == nil || portableResult.JobID != "" || portableResult.Candidate != "" {
				receiptErr = bindReceivedResponse(&result.Gaps[gapIndex], frozen, portableResult)
			}
			if err := ctx.Err(); err != nil {
				result.Gaps[gapIndex].Status = GapFailed
				return assemblyline.RequirementPartitionDecision{}, err
			}
			if !samePortableJob(dispatched, frozen) {
				result.Gaps[gapIndex].Status = GapFailed
				return assemblyline.RequirementPartitionDecision{}, fmt.Errorf(
					"%w: station mutated its immutable portable job", ErrInvalidCompile,
				)
			}
			if callErr != nil {
				result.Gaps[gapIndex].Status = GapFailed
				return assemblyline.RequirementPartitionDecision{}, callErr
			}
			if receiptErr != nil {
				result.Gaps[gapIndex].Status = GapFailed
				return assemblyline.RequirementPartitionDecision{}, receiptErr
			}
			if err := portableResult.ValidateFor(frozen); err != nil {
				result.Gaps[gapIndex].Status = GapFailed
				return assemblyline.RequirementPartitionDecision{}, err
			}
			candidate, err := decodePartitionDecision(portableResult.Candidate)
			if err != nil {
				result.Gaps[gapIndex].Status = GapFailed
				return assemblyline.RequirementPartitionDecision{}, err
			}
			if err := candidate.ValidateFor(input); err != nil {
				result.Gaps[gapIndex].Status = GapFailed
				return assemblyline.RequirementPartitionDecision{}, err
			}
			result.Gaps[gapIndex].OutputSHA256 = digestBytes([]byte(portableResult.Candidate))
			result.Gaps[gapIndex].Status = GapResolved
			return candidate, nil
		},
	)
	if err != nil {
		return failedCompileResult(result), err
	}
	if err := ctx.Err(); err != nil {
		return failedCompileResult(result), err
	}
	workload, err := buildWorkload(authority, decision)
	if err != nil {
		return failedCompileResult(result), err
	}
	if err := ctx.Err(); err != nil {
		return failedCompileResult(result), err
	}
	for index := range result.Gaps {
		result.Gaps[index].FinalWorkloadID = workload.ID
	}
	result.Workload = cloneWorkload(workload)
	result.Compiled = true
	return cloneCompileResult(result), nil
}

func bindReceivedResponse(
	gap *PartitionGapRecord,
	expected assemblyline.PortableJob,
	result assemblyline.PortableResult,
) error {
	if gap == nil {
		return fmt.Errorf("%w: response receipt is uninitialized", ErrInvalidCompile)
	}
	gap.ResponseObserved = true
	gap.ResponseJobIDBytes = len(result.JobID)
	gap.ResponseCandidateBytes = len(result.Candidate)
	gap.ResponseJobIDMatches = result.JobID == expected.ID
	if len(result.JobID) > maxResponseJobIDBytes || len(result.Candidate) > maxResponseCandidateBytes {
		return fmt.Errorf(
			"%w: station response exceeds job ID (%d) or candidate (%d) byte bound",
			ErrInvalidCompile, maxResponseJobIDBytes, maxResponseCandidateBytes,
		)
	}
	gap.ResponseWithinBounds = true
	gap.ResponseJobIDSHA256 = digestBytes([]byte(result.JobID))
	gap.ResponseCandidateSHA256 = digestBytes([]byte(result.Candidate))
	gap.ResponseCandidateBytes = len(result.Candidate)
	gap.ResponseSHA256 = digestFields(
		result.JobID, result.Candidate,
	)
	return nil
}

type countingPartitionStation struct {
	delegate PartitionStation
	calls    *int
}

func (station countingPartitionStation) Generate(
	ctx context.Context,
	job assemblyline.PortableJob,
) (assemblyline.PortableResult, error) {
	*station.calls++
	return station.delegate.Generate(ctx, job)
}

func clonePortableJob(job assemblyline.PortableJob) assemblyline.PortableJob {
	job.Payload = append([]byte{}, job.Payload...)
	return job
}

func samePortableJob(left, right assemblyline.PortableJob) bool {
	return left.Schema == right.Schema && left.ID == right.ID && left.Kind == right.Kind &&
		bytes.Equal(left.Payload, right.Payload)
}

func nilPartitionStation(station PartitionStation) bool {
	return nilInterface(station)
}

func partitionGapIdentity(
	compilationID CompilationID,
	sequence int,
	job assemblyline.PortableJob,
) GapID {
	return GapID("G" + digestFields(string(compilationID), fmt.Sprintf("%d", sequence), job.ID))
}

func failedCompileResult(result CompileResult) CompileResult {
	result.Workload = Workload{}
	result.Compiled = false
	return cloneCompileResult(result)
}

func cloneCompileResult(result CompileResult) CompileResult {
	result.Gaps = cloneGaps(result.Gaps)
	result.Workload = cloneWorkload(result.Workload)
	return result
}
