package worker

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	directCodingVerificationPhaseIsolatedImplementation = queue.VerificationIsolatedImplementation
	directCodingVerificationPhaseIsolatedTask           = queue.VerificationIsolatedTask
	directCodingVerificationPhaseIsolatedFinal          = queue.VerificationIsolatedFinal
	defaultDirectCodingVerificationTimeout              = 2 * time.Minute
)

type directCodingVerificationCommand struct {
	Argv        []string
	Environment []string
	Stdin       []byte
	Timeout     time.Duration
}

func directCodingApplicationTaskStageCommands(
	program directCodingProgram,
	context assemblyline.ApplicationTaskContext,
) ([]directCodingVerificationCommand, error) {
	if context.WorkloadSHA256 != program.Workload.SHA256 {
		return nil, fmt.Errorf("task verification workload authority differs from the compiled program")
	}
	pair, err := directCodingTaskSinglePair(program.Coverage, context.Task.TaskID)
	if err != nil {
		return nil, err
	}
	return []directCodingVerificationCommand{
		directCodingNPMVerificationCommand("run", "typecheck"),
		directCodingNPMVerificationCommand("test", "--", pair.VerificationPath),
	}, nil
}

func directCodingImplementationStageCommands() []directCodingVerificationCommand {
	return []directCodingVerificationCommand{
		directCodingNPMVerificationCommand("run", "typecheck"),
	}
}

func directCodingFullTypeScriptStageCommands() []directCodingVerificationCommand {
	return []directCodingVerificationCommand{
		directCodingNPMVerificationCommand("run", "typecheck"),
		directCodingNPMVerificationCommand("test"),
		directCodingNPMVerificationCommand("run", "build"),
	}
}

func directCodingNPMVerificationCommand(arguments ...string) directCodingVerificationCommand {
	return directCodingVerificationCommand{
		Argv: append([]string{"npm"}, arguments...),
		Environment: []string{
			"CI=1",
			"NPM_CONFIG_AUDIT=false",
			"NPM_CONFIG_FUND=false",
			"NPM_CONFIG_UPDATE_NOTIFIER=false",
			"NPM_CONFIG_USERCONFIG=/dev/null",
		},
		Timeout: defaultDirectCodingVerificationTimeout,
	}
}

func directCodingNPMInstallCommand(cacheRoot string) (directCodingVerificationCommand, error) {
	if strings.TrimSpace(cacheRoot) == "" {
		return directCodingVerificationCommand{}, fmt.Errorf("npm installation requires one isolated cache root")
	}
	command := directCodingNPMVerificationCommand(
		"ci", "--ignore-scripts", "--no-audit", "--no-fund",
	)
	command.Environment = append(command.Environment, "NPM_CONFIG_CACHE="+cacheRoot)
	sort.Strings(command.Environment)
	command.Timeout = 3 * time.Minute
	return command, nil
}

func directCodingTypeScriptBlockIsTSX(
	blueprint assemblyline.SourceBlueprint,
	blockID string,
) bool {
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.ID == blockID {
				return directCodingTypeScriptDocumentIsTSX(document)
			}
		}
	}
	return false
}
