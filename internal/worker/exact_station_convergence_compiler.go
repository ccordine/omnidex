package worker

import (
	"context"
	"fmt"
	"regexp"
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
		return exactTypeScriptReplayRejectedCandidateDiagnostic(source, err)
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
	if !diagnostic.CompilerIssue {
		return nil, fmt.Errorf("TypeScript replay compiler returned a non-compiler diagnostic")
	}
	feedback, err := directCodingTypeScriptStageModelFeedback(diagnostic)
	if err != nil {
		return nil, err
	}
	scope, err := inspectDirectCodingTypeScriptScope(ctx, root, *diagnostic)
	if err != nil {
		return nil, fmt.Errorf("derive TypeScript replay compiler scope: %w", err)
	}
	repairRegion, err := assemblyline.NewTypeScriptCompilerRepairRegion(
		source, compiler.contract.TSX, diagnostic.DeclarationLine, diagnostic.DeclarationColumn,
		scope.Bindings, scope.UnavailableBindings,
	)
	if err != nil {
		return nil, fmt.Errorf("localize TypeScript replay compiler failure: %w", err)
	}
	compilerDiagnostics := exactTypeScriptReplayDiagnosticLines(diagnostic.Output)
	return &ExactTypeScriptReplayDiagnostic{
		Stage:         ExactTypeScriptVerificationTypecheck,
		ModelFeedback: feedback, ModelFeedbackSHA256: replaySHA256(feedback),
		CompilerOutputSHA256: replaySHA256(diagnostic.Output),
		CompilerDiagnostics:  compilerDiagnostics,
		Count:                len(compilerDiagnostics),
		RepairRegion:         &repairRegion,
	}, nil
}

func exactTypeScriptReplayRejectedCandidateDiagnostic(
	source string,
	err error,
) (*ExactTypeScriptReplayDiagnostic, error) {
	feedback := strings.TrimSpace(err.Error())
	diagnostic := &ExactTypeScriptReplayDiagnostic{
		Stage:                ExactTypeScriptVerificationSyntax,
		ModelFeedback:        feedback,
		ModelFeedbackSHA256:  replaySHA256(feedback),
		CompilerOutputSHA256: replaySHA256(feedback),
		CompilerDiagnostics:  []string{feedback},
		Count:                1,
	}
	failure, localized := assemblyline.TypeScriptSyntaxFailureFromError(err)
	if !localized {
		return diagnostic, nil
	}
	region, regionErr := assemblyline.NewTypeScriptFragmentRepairRegion(source, failure, 2)
	if regionErr != nil {
		return nil, fmt.Errorf("localize TypeScript replay syntax failure: %w", regionErr)
	}
	diagnostic.RepairRegion = &region
	return diagnostic, nil
}

func exactTypeScriptReplayDiagnosticCount(output string) int {
	return len(exactTypeScriptReplayDiagnosticLines(output))
}

func exactTypeScriptReplayDiagnosticLines(output string) []string {
	seen := make(map[string]struct{})
	lines := make([]string, 0)
	for _, issue := range directCodingTypeScriptCompilerIssues(output) {
		if !strings.Contains(issue.message, "error TS") {
			continue
		}
		message := directCodingTypeScriptIdentityPattern.ReplaceAllString(issue.message, "[source]")
		line := fmt.Sprintf("[source]:%d:%d: %s", issue.line, issue.column, message)
		if _, duplicate := seen[line]; !duplicate {
			seen[line] = struct{}{}
			lines = append(lines, line)
		}
	}
	return lines
}

var exactTypeScriptReplaySourceLocationPattern = regexp.MustCompile(`^\[source\]:[0-9]+:[0-9]+:\s*`)

func exactTypeScriptReplayHistoricalDiagnostic(line string) string {
	line = strings.TrimSpace(line)
	if exactTypeScriptReplaySourceLocationPattern.MatchString(line) {
		return "[source]: " + exactTypeScriptReplaySourceLocationPattern.ReplaceAllString(line, "")
	}
	return line
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
