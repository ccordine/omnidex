package assemblyline

import (
	"strings"
	"testing"
)

func TestRoleplayContextScopeIsPortableButNeverModelVisible(t *testing.T) {
	t.Parallel()
	candidate := contextCandidateFixture(
		t, "simulation_event", "CTX_8", "The bridge remains raised.",
	)
	tests := []struct {
		name string
		job  func() (PortableJob, error)
	}{
		{
			name: "relevance",
			job: func() (PortableJob, error) {
				return NewContextRelevanceRelationJob(ContextRelevanceRelationInput{
					ExactInstruction:   "Continue.",
					KnownArtifactPaths: []string{},
					Candidate:          candidate,
					Scope:              ContextScopeRoleplaySimulation,
				})
			},
		},
		{
			name: "minification",
			job: func() (PortableJob, error) {
				return NewContextMinificationJob(ContextMinificationInput{
					ExactInstruction:    "Continue.",
					KnownArtifactPaths:  []string{},
					SelectedAuthorities: []ContextCandidateAuthority{candidate},
					Scope:               ContextScopeRoleplaySimulation,
				})
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			job, err := test.job()
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(job.Payload), `"scope":"roleplay_simulation"`) {
				t.Fatalf("portable job lost typed scope: %s", job.Payload)
			}
			if strings.Contains(prompt, string(ContextScopeRoleplaySimulation)) ||
				strings.Contains(prompt, `"scope"`) {
				t.Fatalf("model prompt exposed technical scope: %s", prompt)
			}
		})
	}
}
