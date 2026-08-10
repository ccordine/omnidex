package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
)

func TestGenericArtifactWriterRejectsPromotedIntentAuthority(t *testing.T) {
	repository := &Repository{}
	err := repository.WriteArtifact(context.Background(), model.StepAttemptAuthority{
		JobID: 1, Generation: 1, StepID: 2, Attempt: 1, WorkerID: "worker-test",
	}, artifacts.Envelope{
		JobID: 1, StepID: 2, Kind: artifacts.KindIntent, Version: "1",
		Payload: []byte(`{"user_goal":"do not persist through the generic path"}`),
	})
	if !errors.Is(err, ErrIntentArtifactRequiresAcceptedWriter) {
		t.Fatalf("WriteArtifact() err=%v, want promoted-intent rejection", err)
	}
}
