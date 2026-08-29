package main

import (
	"fmt"
	"io"
	"os"
)

func RunApplication(arguments []string, standardInput string) TaskResult {
	input := TaskInput{Arguments: arguments, StandardInput: standardInput}
	results := CapabilityResults{}
	combined := TaskResult{State: map[string]string{}}
	direct001 := CapabilityResults{
	}
	result001 := Feature001(input, direct001)
	results["capability_001"] = result001
	if result001.Output != "" {
		if combined.Output != "" { combined.Output += "\n" }
		combined.Output += result001.Output
	}
	for key, value := range result001.State { combined.State[key] = value }
	if result001.ExitCode != 0 { combined.ExitCode = result001.ExitCode }
	if result001.Error != "" {
		combined.Error = result001.Error
		if combined.ExitCode == 0 { combined.ExitCode = 1 }
		return combined
	}
	return combined
}

func main() {
	rawInput, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read standard input:", err)
		os.Exit(1)
	}
	result := RunApplication(os.Args[1:], string(rawInput))
	if result.Output != "" { fmt.Fprintln(os.Stdout, result.Output) }
	if result.Error != "" { fmt.Fprintln(os.Stderr, result.Error) }
	if result.ExitCode != 0 { os.Exit(result.ExitCode) }
}
