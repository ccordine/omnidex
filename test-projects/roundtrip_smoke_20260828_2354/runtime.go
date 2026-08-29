package main

type TaskInput struct {
	Arguments     []string
	StandardInput string
}

type TaskResult struct {
	Output   string
	Error    string
	ExitCode int
	State    map[string]string
}

type CapabilityResults map[string]TaskResult
