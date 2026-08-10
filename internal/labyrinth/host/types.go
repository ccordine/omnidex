package host

import (
	"context"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/jackc/pgx/v5"
)

const SchemaVersion = 1

// ScenarioResolver returns the sealed benchmark scenario named by a public
// reference. The private definition is never serialized by this package.
type ScenarioResolver func(context.Context, cognition.ScenarioRef) (labyrinth.Scenario, error)

// TransactionAttemptAuthorizer fences the exact active attempt inside the
// host mutation transaction. Implementations must not commit or mutate.
type TransactionAttemptAuthorizer func(context.Context, pgx.Tx, cognition.AttemptRef) error

// EpisodeReceipt is the bounded public projection of one durable host episode.
// It intentionally contains no definition, seed, witness, or oracle field.
type EpisodeReceipt struct {
	Episode  cognition.EpisodeRef    `json:"episode"`
	Scenario cognition.ScenarioRef   `json:"scenario"`
	Start    cognition.Transition    `json:"start"`
	Current  cognition.WorldRevision `json:"current"`
	Terminal bool                    `json:"terminal"`
}

// ActionReceipt is the exact public result bound to one action identity.
type ActionReceipt struct {
	Episode       cognition.EpisodeRef       `json:"episode"`
	Action        cognition.RegisteredAction `json:"action"`
	Expected      cognition.WorldRevision    `json:"expected"`
	Transition    *cognition.Transition      `json:"transition,omitempty"`
	Failure       *cognition.ActionFailure   `json:"failure,omitempty"`
	RequestSHA256 string                     `json:"request_sha256"`
}

func (receipt ActionReceipt) clone() ActionReceipt {
	receipt.Action = receipt.Action.Clone()
	if receipt.Transition != nil {
		transition := receipt.Transition.Clone()
		receipt.Transition = &transition
	}
	if receipt.Failure != nil {
		failure := receipt.Failure.Clone()
		receipt.Failure = &failure
	}
	return receipt
}

type storedEpisode struct {
	Episode      cognition.EpisodeRef
	Scenario     cognition.ScenarioRef
	Start        cognition.Transition
	Current      cognition.WorldRevision
	Terminal     bool
	ReceiptCount int64
}

type storedAction struct {
	Receipt      ActionReceipt
	ResultNumber *uint64
}
