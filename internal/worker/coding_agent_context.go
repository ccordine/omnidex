package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/queue"
)

// Coding uses the same chunking and semantic minification as conversation
// context. Frozen ledger data remains unchanged; this is a prompt projection.
func (runtime *nativeRuntimeV3) minifyCodingContext(ctx context.Context, call codingagent.Assignment) (string, error) {
	raw, err := json.Marshal(codingagent.PromptContext(call))
	if err != nil {
		return "", err
	}
	redacted, identities, err := assemblyline.RedactContextArtifactJSON(raw)
	if err != nil {
		return "", err
	}
	projected, err := assemblyline.ProjectContextJSONText([]byte(redacted))
	if err != nil {
		return "", err
	}
	set, err := buildContextCandidateSet([]queue.ContextSearchRecord{{Namespace: "session_coding", SourceID: "current-assignment", Content: projected}}, nil)
	if err != nil {
		return "", err
	}
	instruction := "Keep the minimum context needed for this " + string(call.Role) + " assignment. Preserve all requirements, constraints, acceptance criteria, task boundaries, owner answers, relevant systems and files, and observed verification failures. Preserve exact item IDs and every proposed ledger change, question, permission request, error, finding and verification result in a response being parsed; do not resolve or reinterpret them. Remove duplication and unrelated history, never obligations."
	result, err := contextcompiler.Compile(ctx, contextcompiler.Request{ExactInstruction: instruction, ModelInstruction: instruction, KnownArtifactPaths: []string{}},
		sessionTaskContextProvider{set: set}, contextcompiler.Stations{Minification: portableObjectiveContextSieveStations{runtime: runtime}})
	if err != nil {
		return "", &codingagent.ParserFailure{Cause: fmt.Errorf("minify coding context: %w", err)}
	}
	parts := make([]string, len(result.Context.Capsules))
	for index, capsule := range result.Context.Capsules {
		parts[index] = capsule.Content
	}
	compact, err := assemblyline.RestoreContextArtifactJSONOrText(strings.Join(parts, "\n\n"), identities)
	if err != nil {
		return "", &codingagent.ParserFailure{Cause: fmt.Errorf("restore minified coding context: %w", err)}
	}
	return compact, nil
}
