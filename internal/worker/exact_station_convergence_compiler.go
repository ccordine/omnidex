package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const exactTypeScriptReplayBlockID = "replay.fragment"

type exactTypeScriptReplayCompiler struct {
	workspace *directCodingTypeScriptStageWorkspace
	program   directCodingProgram
	contract  assemblyline.TypeScriptFunctionContract
}

func newExactTypeScriptReplayCompiler(
	ctx context.Context,
	input assemblyline.FragmentCorrectionInput,
) (*exactTypeScriptReplayCompiler, error) {
	contract := assemblyline.TypeScriptFunctionContract{
		Signature: input.Signature,
		TSX:       true,
	}
	if _, err := assemblyline.ParseTypeScriptFunction(contract, input.CurrentDeclaration); err != nil {
		return nil, fmt.Errorf("validate frozen TypeScript replay declaration: %w", err)
	}
	program, err := exactTypeScriptReplayProgram(input, input.CurrentDeclaration)
	if err != nil {
		return nil, err
	}
	workspace, err := newDirectCodingTypeScriptStageWorkspace(ctx, program)
	if err != nil {
		return nil, err
	}
	return &exactTypeScriptReplayCompiler{
		workspace: workspace,
		program:   program,
		contract:  contract,
	}, nil
}

func (compiler *exactTypeScriptReplayCompiler) Close() {
	if compiler != nil && compiler.workspace != nil {
		compiler.workspace.Close()
	}
}

func (compiler *exactTypeScriptReplayCompiler) Verify(
	ctx context.Context,
	source string,
) (*ExactTypeScriptReplayDiagnostic, error) {
	if compiler == nil || compiler.workspace == nil {
		return nil, fmt.Errorf("TypeScript replay compiler is uninitialized")
	}
	if _, err := assemblyline.ParseTypeScriptFunction(compiler.contract, source); err != nil {
		return exactTypeScriptReplayRejectedCandidateDiagnostic(err), nil
	}
	compiler.program.Generated[exactTypeScriptReplayBlockID] = source
	root := compiler.workspace.Root()
	if err := resetDirectCodingTypeScriptStage(root); err != nil {
		return nil, err
	}
	if err := writeDirectCodingTypeScriptStage(root, compiler.program); err != nil {
		return nil, err
	}
	diagnostic, err := verifyDirectCodingTypeScriptStageCommands(
		ctx, root, compiler.program, [][]string{{"run", "typecheck"}},
	)
	if err != nil || diagnostic == nil {
		return nil, err
	}
	if diagnostic.BlockID != exactTypeScriptReplayBlockID {
		return nil, fmt.Errorf("TypeScript replay compiler mapped failure to %s", diagnostic.BlockID)
	}
	feedback := directCodingTypeScriptModelFailure(diagnostic.Output)
	compilerDiagnostics := exactTypeScriptReplayDiagnosticLines(diagnostic.Output)
	return &ExactTypeScriptReplayDiagnostic{
		ModelFeedback: feedback, ModelFeedbackSHA256: replaySHA256(feedback),
		CompilerOutputSHA256: replaySHA256(diagnostic.Output),
		CompilerDiagnostics:  compilerDiagnostics,
		Count:                len(compilerDiagnostics),
	}, nil
}

func exactTypeScriptReplayRejectedCandidateDiagnostic(err error) *ExactTypeScriptReplayDiagnostic {
	feedback := strings.TrimSpace(err.Error())
	return &ExactTypeScriptReplayDiagnostic{
		ModelFeedback:        feedback,
		ModelFeedbackSHA256:  replaySHA256(feedback),
		CompilerOutputSHA256: replaySHA256(feedback),
		CompilerDiagnostics:  []string{feedback},
		Count:                1,
	}
}

func exactTypeScriptReplayDiagnosticCount(output string) int {
	return len(exactTypeScriptReplayDiagnosticLines(output))
}

func exactTypeScriptReplayDiagnosticLines(output string) []string {
	seen := make(map[string]struct{})
	lines := make([]string, 0)
	for _, rawLine := range strings.Split(
		directCodingANSISequencePattern.ReplaceAllString(strings.ReplaceAll(output, "\r", ""), ""), "\n",
	) {
		line := strings.TrimSpace(rawLine)
		if !strings.Contains(line, "error TS") {
			continue
		}
		line = directCodingTypeScriptIdentityPattern.ReplaceAllString(line, "[source]")
		if _, duplicate := seen[line]; !duplicate {
			seen[line] = struct{}{}
			lines = append(lines, line)
		}
	}
	return lines
}

func exactTypeScriptReplayProgram(
	input assemblyline.FragmentCorrectionInput,
	source string,
) (directCodingProgram, error) {
	job, err := assemblyline.NewFragmentCorrectionJob(input)
	if err != nil {
		return directCodingProgram{}, err
	}
	if job.Kind != assemblyline.WorkFragmentCorrection || input.Language != "typescript" || input.RepairRegion != nil {
		return directCodingProgram{}, fmt.Errorf("compiler replay requires one whole TypeScript declaration")
	}
	header, err := exactTypeScriptReplayHeader(input)
	if err != nil {
		return directCodingProgram{}, err
	}
	document := assemblyline.TypeScriptDocument{
		ID: "station_replay", Path: "src/replay.tsx", Header: header,
		Blocks: []assemblyline.TypeScriptBlock{{
			ID: exactTypeScriptReplayBlockID, Signature: input.Signature,
			Contract: "Compile the exact untrusted correction candidate.", API: input.Signature,
		}},
	}
	static := make([]directCodingFileTask, 0, 3)
	for _, file := range typeScriptBrowserStaticFiles("station-replay", "Station replay", "") {
		if file.Path == "package.json" || file.Path == "tsconfig.json" || file.Path == "vite.config.ts" {
			static = append(static, file)
		}
	}
	program := directCodingProgram{
		Adapter: "exact_typescript_station_replay", PackageName: "station-replay",
		TypeScript:  assemblyline.TypeScriptBlueprint{Documents: []assemblyline.TypeScriptDocument{document}},
		StaticFiles: static, Generated: map[string]string{exactTypeScriptReplayBlockID: source},
	}
	if err := program.TypeScript.Validate(); err != nil {
		return directCodingProgram{}, err
	}
	return program, nil
}
