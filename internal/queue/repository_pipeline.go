package queue

func codingSteps() []stepSeed {
	return []stepSeed{
		{action: "v3_coding_plan", sortIndex: 5},
		{action: "v3_coding", sortIndex: 10},
	}
}

func conversationObjectiveSteps() []stepSeed {
	return []stepSeed{
		{action: "objective_resolve", sortIndex: 5},
		{action: "v3_coding_plan", sortIndex: 10},
		{action: "v3_coding", sortIndex: 15},
	}
}
