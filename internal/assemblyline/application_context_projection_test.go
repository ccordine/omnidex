package assemblyline

import (
	"strings"
	"testing"
)

const sensitiveProjectionFactValue = "Lifecycle events are committed in the same transaction as the change."

func TestApplicationIntentEnvelopesKeepFullAuthorityOutsideModelProjection(t *testing.T) {
	t.Parallel()
	const request = "Summarize lifecycle events in the existing browser dashboard."
	context := sensitiveApplicationContextProjectionFixture(t, request)
	const productContext = "An existing browser dashboard for lifecycle events."
	const accepted = "The dashboard summarizes lifecycle events."

	productJob, err := NewApplicationProductContextJob(ApplicationProductContextInput{
		UserRequest: request,
		Context:     context,
	})
	if err != nil {
		t.Fatal(err)
	}
	coverageInput := ApplicationRequirementCoverageInput{
		UserRequest:          request,
		Context:              context,
		AcceptedRequirements: []string{accepted},
		ExcludedCandidates:   []string{},
	}
	coverageJob, err := NewApplicationRequirementCoverageJob(coverageInput)
	if err != nil {
		t.Fatal(err)
	}
	requirementJob, err := NewApplicationRequirementJob(
		applicationRequirementCandidateFixture(t, coverageInput),
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name               string
		job                PortableJob
		expectedLeafValues []string
	}{
		{name: "product context", job: productJob},
		{
			name: "requirement coverage", job: coverageJob,
			expectedLeafValues: []string{accepted},
		},
		{
			name: "requirement", job: requirementJob,
			expectedLeafValues: []string{accepted},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertFullApplicationContextAuthorityInPayload(t, test.job, context)
			prompt, err := RenderPortableJob(test.job)
			if err != nil {
				t.Fatal(err)
			}
			assertMinimumApplicationContextProjection(t, prompt, request, context)
			for _, value := range test.expectedLeafValues {
				if !strings.Contains(prompt, value) {
					t.Fatalf("model projection omitted accepted semantic leaf %q:\n%s", value, prompt)
				}
			}
			if test.job.Kind != WorkApplicationProductContext &&
				(strings.Contains(prompt, productContext) ||
					strings.Contains(prompt, "PRODUCT CONTEXT:")) {
				t.Fatalf("requirement station received redundant product context:\n%s", prompt)
			}
		})
	}
}

func TestRepositoryRequirementEnvelopesProjectOnlySemanticAuthority(t *testing.T) {
	t.Parallel()
	const request = "Add lifecycle event export to the existing service."
	context := sensitiveApplicationContextProjectionFixture(t, request)
	const accepted = "The service exports lifecycle events."
	input := RepositoryRequirementLeafInput{
		Authority: RepositoryRequirementInterpretationInput{
			UserRequest: request,
			Context:     context,
		},
		AcceptedRequirements: []string{accepted},
	}
	coverageJob, err := NewRepositoryRequirementCoverageJob(input)
	if err != nil {
		t.Fatal(err)
	}
	requirementJob, err := NewRepositoryRequirementJob(input)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		job  PortableJob
	}{
		{name: "coverage", job: coverageJob},
		{name: "requirement", job: requirementJob},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertFullApplicationContextAuthorityInPayload(t, test.job, context)
			prompt, err := RenderPortableJob(test.job)
			if err != nil {
				t.Fatal(err)
			}
			assertMinimumApplicationContextProjection(t, prompt, request, context)
			if !strings.Contains(prompt, accepted) {
				t.Fatalf("model projection omitted accepted requirement:\n%s", prompt)
			}
			if strings.Contains(prompt, "without paths") {
				t.Fatalf("repository requirement prompt retained unrelated wording:\n%s", prompt)
			}
		})
	}

	if _, err := DecodeRepositoryRequirementLeaf(input, "Change ../private/service.go."); err == nil {
		t.Fatal("repository requirement decoder accepted a mechanically forbidden path")
	}
}

func TestApplicationContextNeedEnvelopeDoesNotRepeatWorkspaceFact(t *testing.T) {
	t.Parallel()
	const request = "Clarify lifecycle event ownership in the existing service."
	context := sensitiveApplicationContextProjectionFixture(t, request)
	const accepted = "Which component owns lifecycle event persistence?"
	input := ApplicationContextNeedLeafInput{
		UserRequest:       request,
		Context:           context,
		AcceptedQuestions: []string{accepted},
	}
	coverageJob, err := NewApplicationContextNeedCoverageJob(input)
	if err != nil {
		t.Fatal(err)
	}
	questionJob, err := NewApplicationContextNeedQuestionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		job  PortableJob
	}{
		{name: "coverage", job: coverageJob},
		{name: "question", job: questionJob},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertFullApplicationContextAuthorityInPayload(t, test.job, context)
			prompt, err := RenderPortableJob(test.job)
			if err != nil {
				t.Fatal(err)
			}
			assertMinimumApplicationContextProjection(t, prompt, request, context)
			if !strings.Contains(prompt, accepted) {
				t.Fatalf("model projection omitted accepted question:\n%s", prompt)
			}
		})
	}
}

func sensitiveApplicationContextProjectionFixture(
	t *testing.T,
	request string,
) ApplicationContext {
	t.Helper()
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceExisting)
	if err != nil {
		t.Fatal(err)
	}
	context.Facts[0].SourceID = "private_bootstrap_source_identity"
	context.Facts = append(context.Facts, ApplicationContextFact{
		ID:           "fact_002",
		Kind:         ApplicationContextRepositoryFact,
		Authority:    ApplicationContextEvidenceAuthority,
		NeedID:       "private_evidence_need_identity",
		Value:        sensitiveProjectionFactValue,
		SourceID:     "private_repository_source_identity",
		SourceSHA256: ExactObjectiveContextSHA(sensitiveProjectionFactValue),
	})
	if err := context.Validate(); err != nil {
		t.Fatal(err)
	}
	return context
}

func assertFullApplicationContextAuthorityInPayload(
	t *testing.T,
	job PortableJob,
	context ApplicationContext,
) {
	t.Helper()
	payload := string(job.Payload)
	for _, value := range sensitiveApplicationContextMetadata(context) {
		if !strings.Contains(payload, value) {
			t.Fatalf("portable payload omitted code-owned context value %q:\n%s", value, payload)
		}
	}
}

func assertMinimumApplicationContextProjection(
	t *testing.T,
	prompt string,
	request string,
	context ApplicationContext,
) {
	t.Helper()
	for _, value := range []string{
		request,
		string(context.WorkspaceState),
		string(ApplicationContextRepositoryFact),
		sensitiveProjectionFactValue,
	} {
		if !strings.Contains(prompt, value) {
			t.Fatalf("model projection omitted semantic value %q:\n%s", value, prompt)
		}
	}
	if count := strings.Count(prompt, "WORKSPACE STATE:"); count != 1 {
		t.Fatalf("workspace state rendered %d times:\n%s", count, prompt)
	}
	if strings.Contains(prompt, string(ApplicationContextWorkspaceState)) {
		t.Fatalf("bootstrap workspace fact leaked into model projection:\n%s", prompt)
	}
	for _, value := range sensitiveApplicationContextMetadata(context) {
		if strings.Contains(prompt, value) {
			t.Fatalf("code-owned context metadata %q leaked into model projection:\n%s", value, prompt)
		}
	}
	for _, label := range []string{
		`"schema"`,
		`"request_sha256"`,
		`"id"`,
		`"need_id"`,
		`"source_id"`,
		`"source_sha256"`,
		`"authority"`,
	} {
		if strings.Contains(prompt, label) {
			t.Fatalf("code-owned context label %q leaked into model projection:\n%s", label, prompt)
		}
	}
}

func sensitiveApplicationContextMetadata(context ApplicationContext) []string {
	return []string{
		context.Schema,
		context.RequestSHA256,
		context.Facts[0].ID,
		string(context.Facts[0].Authority),
		context.Facts[0].SourceID,
		context.Facts[0].SourceSHA256,
		context.Facts[1].ID,
		string(context.Facts[1].Authority),
		context.Facts[1].NeedID,
		context.Facts[1].SourceID,
		context.Facts[1].SourceSHA256,
	}
}
