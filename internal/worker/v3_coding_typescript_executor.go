package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingTypeScriptSourceGenerator struct {
	session *directCodingSession
}

func newDirectCodingTypeScriptSourceGenerator(
	session *directCodingSession,
	_ directCodingProgram,
) (directCodingProjectSourceGenerator, error) {
	if session == nil {
		return nil, fmt.Errorf("TypeScript source generation requires one coding session")
	}
	return &directCodingTypeScriptSourceGenerator{session: session}, nil
}

func (executor *directCodingTypeScriptSourceGenerator) GenerateBlock(
	context assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, error) {
	if ref.Document.AdapterID != "typescript" && ref.Document.AdapterID != "typescript_react" {
		return "", fmt.Errorf(
			"TypeScript source generator cannot generate adapter %q block %s",
			ref.Document.AdapterID, ref.Block.ID,
		)
	}
	if ref.Block.Role != assemblyline.SourceBlockTaskImplementation &&
		ref.Block.Role != assemblyline.SourceBlockTaskRepresentation {
		return "", fmt.Errorf("TypeScript source generator cannot build task role %q", ref.Block.Role)
	}
	source, err := executor.session.generateDirectCodingApplicationTaskBlock(
		context, stage, ref, nil,
	)
	if err != nil {
		return "", err
	}
	return source, nil
}
