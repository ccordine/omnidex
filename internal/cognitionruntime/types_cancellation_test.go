package cognitionruntime

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestCancellationEvidenceBindsRegisteredCodeAndExactSource(t *testing.T) {
	evidence, err := NewCancellationEvidence(
		CancellationPolicyFailure,
		"The bounded cognition policy response was rejected.",
		errors.New("exact provider response validation error"),
	)
	if err != nil {
		t.Fatal(err)
	}
	binding := runtimeTestBinding(t)
	revision, err := cognition.NewWorldRevision(binding.Episode.ID, 1, runtimeTestDigest("a"))
	if err != nil {
		t.Fatal(err)
	}
	command := CancellationCommand{
		Binding: binding, ExpectedRevision: revision,
		Code: CancellationPolicyFailure, SourceEvidence: evidence,
	}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	changed := command
	changed.SourceEvidence.SourceErrorSHA256 = runtimeTestDigest("b")
	if err := changed.Validate(); err == nil {
		t.Fatal("changed source error identity was accepted")
	}
	changed = command
	changed.Code = "unknown"
	if err := changed.Validate(); err == nil {
		t.Fatal("unregistered cancellation code was accepted")
	}
}

func TestLifecycleCancellationCodesCannotUseWorkerAuthority(t *testing.T) {
	evidence, err := NewLifecycleCancellationEvidence(
		CancellationJobCanceled, "The owning job was canceled.", runtimeTestDigest("c"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCancellationEvidence(
		CancellationJobCanceled, "The owning job was canceled.", errors.New("not lifecycle authority"),
	); err == nil {
		t.Fatal("lifecycle cancellation code entered the worker evidence constructor")
	}
	binding := runtimeTestBinding(t)
	revision, err := cognition.NewWorldRevision(binding.Episode.ID, 1, runtimeTestDigest("d"))
	if err != nil {
		t.Fatal(err)
	}
	command := CancellationCommand{
		Binding: binding, ExpectedRevision: revision,
		Code: CancellationJobCanceled, SourceEvidence: evidence,
	}
	if err := command.Validate(); err == nil {
		t.Fatal("lifecycle cancellation code entered the worker cancellation API")
	}
}

func runtimeTestBinding(t *testing.T) Binding {
	t.Helper()
	episode, err := cognition.NewEpisodeRef("cancellation-episode")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := NewBinding(episode, cognition.AttemptRef{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker",
	})
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func runtimeTestDigest(character string) string {
	result := ""
	for len(result) < 64 {
		result += character
	}
	return result[:64]
}
