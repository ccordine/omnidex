package assemblyline

import "sort"

func applicationWorkloadTestInput() ApplicationWorkloadDraftInput {
	return ApplicationWorkloadDraftInput{
		Surface:      ApplicationSurfaceBrowser,
		ProductQuote: "browser operations console",
		Requirements: []Requirement{
			{ID: "requirement_001", SourceQuote: "group records by status"},
			{ID: "requirement_002", SourceQuote: "filter records quickly"},
			{ID: "requirement_003", SourceQuote: "export printable summaries"},
		},
	}
}

func applicationWorkloadTestDraft() ApplicationWorkloadDraft {
	return ApplicationWorkloadDraft{Schema: ApplicationWorkloadDraftSchemaV1, Tasks: []ApplicationWorkloadTaskDraft{
		{
			RequirementID: "requirement_001", Objective: "Implement status grouping.",
			RequiredBehaviors:  []string{"Place records in their selected status group."},
			AcceptanceCriteria: []string{"Changing status moves the record to its selected group."},
		},
		{
			RequirementID: "requirement_002", Objective: "Implement record filtering.",
			RequiredBehaviors:  []string{"Apply the selected filter to visible records."},
			AcceptanceCriteria: []string{"Visible records match the selected filter."},
			DependsOn:          []string{"requirement_001"},
		},
		{
			RequirementID: "requirement_003", Objective: "Implement printable export.",
			RequiredBehaviors:  []string{"Create a printable summary from visible records."},
			AcceptanceCriteria: []string{"A user can open a printable summary."},
			DependsOn:          []string{"requirement_001", "requirement_002"},
		},
	}}
}

func cloneApplicationWorkloadDraft(value ApplicationWorkloadDraft) ApplicationWorkloadDraft {
	copy := value
	copy.Tasks = append([]ApplicationWorkloadTaskDraft(nil), value.Tasks...)
	for index := range copy.Tasks {
		copy.Tasks[index].RequiredBehaviors = append([]string(nil), value.Tasks[index].RequiredBehaviors...)
		copy.Tasks[index].AcceptanceCriteria = append([]string(nil), value.Tasks[index].AcceptanceCriteria...)
		copy.Tasks[index].DependsOn = append([]string(nil), value.Tasks[index].DependsOn...)
	}
	return copy
}

func sortedJobSpecificationKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
