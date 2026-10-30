package queue

import "github.com/gryph/omnidex/internal/codingagent"

func codingAgentTestConfiguration() codingagent.Configuration {
	route := codingagent.Route{Provider: "codex", Model: "fixture-model"}
	return codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route}
}
