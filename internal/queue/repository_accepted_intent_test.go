package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
)

func TestGenericArtifactWriterRejectsPromotedIntentAuthority(t *testing.T) {
	repository := &Repository{}
	err := repository.WriteArtifact(context.Background(), artifacts.Envelope{
		JobID: 1, StepID: 2, Kind: artifacts.KindIntent, Version: "1",
		Payload: []byte(`{"user_goal":"do not persist through the generic path"}`),
	})
	if !errors.Is(err, ErrIntentArtifactRequiresAcceptedWriter) {
		t.Fatalf("WriteArtifact() err=%v, want promoted-intent rejection", err)
	}
}
