package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func (consumer *channelSessionMessageConsumer) projectTaskContext(
	ctx context.Context, authority turnAuthority, job model.Job,
) (turnAuthority, error) {
	if job.Instruction == "" {
		return authority, fmt.Errorf("current task request authority is unavailable")
	}
	content := "Original current task request:\n" + job.Instruction +
		"\n\nCurrent task status: " + consumer.claim.Job.Status +
		"\nCurrent step action: " + consumer.claim.Step.Action +
		"\nCurrent step status: " + consumer.claim.Step.Status
	authority, content, err := bindSessionTaskContextArtifacts(authority, content)
	if err != nil {
		return authority, err
	}
	set, err := buildContextCandidateSet([]queue.ContextSearchRecord{{
		Namespace: "session_current_task", SourceID: fmt.Sprintf("job:%d:generation:%d", job.ID, job.CurrentGeneration),
		Content: content,
	}}, nil)
	if err != nil {
		return authority, err
	}
	result, err := contextcompiler.Compile(ctx, contextcompiler.Request{
		ExactInstruction: authority.Instruction, ModelInstruction: authority.ModelInstruction,
		KnownArtifactPaths: append([]string{}, authority.ModelArtifactPaths...),
	}, sessionTaskContextProvider{set: set}, contextcompiler.Stations{Minification: consumer.minification})
	if err != nil {
		return authority, err
	}
	authority.Context = assemblyline.CloneObjectiveContext(result.Context)
	return authority, nil
}

// Task facts extend the same code-owned bindings without changing the exact
// current instruction or inventing another instruction for classification.
func bindSessionTaskContextArtifacts(authority turnAuthority, content string) (turnAuthority, string, error) {
	redacted, identities, err := assemblyline.RedactArtifactIdentities(content, assemblyline.ArtifactIdentityProvenance{})
	if err != nil {
		return authority, "", err
	}
	bindings := make(map[string]string, len(authority.ModelArtifactIdentities))
	for _, identity := range authority.ModelArtifactIdentities {
		bindings[identity.Value] = identity.Token
	}
	remapped := make([]assemblyline.ArtifactIdentity, 0, len(identities))
	for _, identity := range identities {
		token, exists := bindings[identity.Value]
		if !exists {
			token = fmt.Sprintf("ARTIFACT_%d", len(authority.ModelArtifactIdentities)+1)
			bindings[identity.Value] = token
			authority.ModelArtifactIdentities = append(authority.ModelArtifactIdentities,
				assemblyline.ArtifactIdentity{Token: token, Value: identity.Value})
		}
		remapped = append(remapped, assemblyline.ArtifactIdentity{Token: identity.Token, Value: token})
	}
	projected, err := assemblyline.RestoreArtifactIdentities(redacted, remapped)
	if err != nil {
		return authority, "", err
	}
	if err := validateObjectiveModelInput(authority, "active session task context", projected); err != nil {
		return authority, "", err
	}
	return authority, projected, nil
}

type sessionTaskContextProvider struct{ set contextcompiler.CandidateSet }

func (provider sessionTaskContextProvider) SearchAvailability(context.Context) (contextcompiler.SearchAvailability, error) {
	return contextcompiler.SearchUnavailable, nil
}

func (provider sessionTaskContextProvider) Retrieve(ctx context.Context, queries []string) (contextcompiler.CandidateSet, error) {
	if err := ctx.Err(); err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	if len(queries) != 0 {
		return contextcompiler.CandidateSet{}, fmt.Errorf("current task context does not admit historical search")
	}
	return provider.set, nil
}
