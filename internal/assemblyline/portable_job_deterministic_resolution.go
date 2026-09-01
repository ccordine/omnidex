package assemblyline

// ResolvePortableJobWithoutInference consumes a closed choice when code can
// prove that exactly one option remains. The returned candidate is the
// call-local opaque ID understood by the station decoder; the code-owned value
// never becomes model output. Non-choice jobs and choices with two or more
// options remain unresolved.
func ResolvePortableJobWithoutInference(
	job PortableJob,
) (PortableResult, bool, error) {
	if err := job.Validate(); err != nil {
		return PortableResult{}, false, err
	}
	resolved, handled, err := resolvePortableDatabaseJobWithoutInference(job)
	if err != nil || !handled || !resolved {
		return PortableResult{}, false, err
	}
	result := PortableResult{JobID: job.ID, Candidate: opaqueModelChoiceID(0)}
	if err := result.ValidateFor(job); err != nil {
		return PortableResult{}, false, err
	}
	return result, true, nil
}

func resolvePortableDatabaseJobWithoutInference(
	job PortableJob,
) (resolved bool, handled bool, err error) {
	switch job.Kind {
	case WorkDatabaseQueryFromRelation:
		var input DatabaseQueryIntentLeafState
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return false, true, err
		}
		_, resolved, err = ResolveSoleDatabaseQueryFromRelationLeaf(input)
	case WorkDatabaseQueryProjectionField:
		var input DatabaseQueryProjectionLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return false, true, err
		}
		_, resolved, err = ResolveSoleDatabaseQueryProjectionFieldLeaf(input)
	case WorkDatabaseQueryFilterField:
		var input DatabaseQueryFilterLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return false, true, err
		}
		_, resolved, err = ResolveSoleDatabaseQueryFilterFieldLeaf(input)
	case WorkDatabaseQueryFilterOperator:
		var input DatabaseQueryFilterLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return false, true, err
		}
		_, resolved, err = ResolveSoleDatabaseQueryFilterOperatorLeaf(input)
	case WorkDatabaseQueryFilterValue:
		var input DatabaseQueryFilterLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return false, true, err
		}
		_, resolved, err = ResolveSoleDatabaseQueryFilterValueLeaf(input)
	case WorkDatabaseQueryWindowField:
		var input DatabaseQueryWindowLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return false, true, err
		}
		_, resolved, err = ResolveSoleDatabaseQueryWindowFieldLeaf(input)
	case WorkDatabaseQueryWindowUnit:
		var input DatabaseQueryWindowLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return false, true, err
		}
		_, resolved, err = ResolveSoleDatabaseQueryWindowUnitLeaf(input)
	case WorkDatabaseQueryExistenceRelation:
		var input DatabaseQueryExistenceLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return false, true, err
		}
		_, resolved, err = ResolveSoleDatabaseQueryExistenceRelationLeaf(input)
	case WorkDatabaseQueryHavingField:
		var input DatabaseQueryHavingLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return false, true, err
		}
		_, resolved, err = ResolveSoleDatabaseQueryHavingFieldLeaf(input)
	case WorkDatabaseQueryOrderProjection:
		var input DatabaseQueryOrderLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return false, true, err
		}
		_, resolved, err = ResolveSoleDatabaseQueryOrderProjectionLeaf(input)
	case WorkDatabaseJoinPathSelection:
		var input DatabaseJoinPathSelectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return false, true, err
		}
		_, resolved, err = ResolveSoleDatabaseJoinPathSelectionDecision(input)
	default:
		return false, false, nil
	}
	return resolved, true, err
}
