package worker

import (
	"context"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

type directCodingVerificationCommandResult struct {
	Stdout []byte
	Stderr []byte
}

func directCodingVerificationPhaseUsesHostRoot(phase queue.VerificationCommandPhase) bool {
	return phase == queue.VerificationHostInstall || phase == queue.VerificationHostFinal
}

func directCodingVerificationEvidenceContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
}

func directCodingVerificationTimestamp(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}
