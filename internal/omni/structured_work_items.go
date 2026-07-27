package omni

func refreshStructuredTypedWorkItems(
	prompt string,
	workingDirectory string,
	worksiteSurvey WorksiteSurvey,
	ledger *[]StructuredObjective,
	result *CommandDecisionResult,
	onEvent func(StructuredCommandEvent),
) {
	if len(*ledger) == 0 {
		if hasImplementationArchitectProgress(result.Observations) {
			result.WorkItems = architectObjectiveWorkItemsFromObservations(prompt, workingDirectory, worksiteSurvey, result.Observations)
			result.ChildJobs = BuildChildJobsFromObjectiveWorkItems(result.WorkItems)
		}
		return
	}

	result.WorkItems = ReconcileObjectiveWorkItemsFromObservations(
		BuildObjectiveWorkItemsFromLedger(prompt, *ledger, workingDirectory, worksiteSurvey),
		result.Observations,
	)
	reconciledLedger := reconcileStructuredObjectiveLedgerFromWorkItems(*ledger, result.WorkItems)
	if !structuredObjectiveLedgersEqual(*ledger, reconciledLedger) {
		*ledger = reconciledLedger
		result.ObjectiveLedger = reconciledLedger
		result.WorkItems = ReconcileObjectiveWorkItemsFromObservations(
			BuildObjectiveWorkItemsFromLedger(prompt, reconciledLedger, workingDirectory, worksiteSurvey),
			result.Observations,
		)
	}
	if hasImplementationArchitectProgress(result.Observations) && !ValidateObjectiveWorkForest(result.WorkItems).Passed {
		architectItems := architectObjectiveWorkItemsFromObservations(prompt, workingDirectory, worksiteSurvey, result.Observations)
		if ValidateObjectiveWorkForest(architectItems).Passed {
			result.WorkItems = architectItems
		}
	}

	var latest *StructuredCommandObservation
	if len(result.Observations) > 0 {
		latest = &result.Observations[len(result.Observations)-1]
	}
	childJobs := result.ChildJobs
	if len(childJobs) == 0 {
		childJobs = BuildChildJobsFromObjectiveWorkItems(result.WorkItems)
	}
	childJobs = SyncChildJobsWithObjectiveLedger(childJobs, *ledger)
	commandChildJobID, commandObjectiveID := reconciliationOwnerFromObservation(latest, childJobs, *ledger)
	reconciledSuccess := RunSuccessReconciliation(SuccessReconciliationInput{
		LatestObservation: latest,
		ChildJobID:        commandChildJobID,
		ObjectiveID:       commandObjectiveID,
		ObjectiveLedger:   *ledger,
		WorkQueue:         result.WorkItems,
		ChildJobs:         childJobs,
		WorkingDirectory:  workingDirectory,
		Observations:      result.Observations,
	})
	result.WorkItems = reconciledSuccess.WorkQueue
	result.ChildJobs = reconciledSuccess.ChildJobs
	if !structuredObjectiveLedgersEqual(*ledger, reconciledSuccess.ObjectiveLedger) {
		*ledger = reconciledSuccess.ObjectiveLedger
		result.ObjectiveLedger = reconciledSuccess.ObjectiveLedger
	}
	emitSuccessReconciliationEvents(onEvent, reconciledSuccess.Events)
}
