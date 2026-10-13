package worker

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestActiveSessionQuestionFailurePreservesOriginalTaskAndIsTerminal(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	original := *claim
	setSessionAnswerStations(consumer)
	failure := errors.New("observed question provider failure")
	consumer.response = usageConversationFunc(func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		return assemblyline.ConversationResponseDecision{}, 1, failure
	})
	consumed, err := consumer.consume(context.Background())
	if err != nil || !consumed || store.pending != nil || !reflect.DeepEqual(original, *claim) ||
		len(store.failures) != 1 || len(store.resolutions) != 0 {
		t.Fatalf("question failure damaged or blocked accepted task: consumed=%t pending=%t failures=%v resolutions=%d err=%v", consumed, store.pending != nil, store.failures, len(store.resolutions), err)
	}
	if consumed, err := consumer.consume(context.Background()); err != nil || consumed || len(store.failures) != 1 {
		t.Fatalf("failed question was retried: consumed=%t failures=%v err=%v", consumed, store.failures, err)
	}
}

func TestActiveSessionFailureResolutionPublishesOnlyAfterTerminalCommit(t *testing.T) {
	for _, persistenceFails := range []bool{false, true} {
		consumer, store, _ := activeSessionConsumerTest(t)
		setSessionAnswerStations(consumer)
		consumer.response = usageConversationFunc(func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
			return assemblyline.ConversationResponseDecision{}, 1, errors.New("observed provider failure")
		})
		if persistenceFails {
			store.failureErr = errors.New("durable failure commit unavailable")
		}
		events := 0
		consumer.resolved = func(operation model.LifecycleOperationID) {
			events++
			if store.pending != nil || operation != "lifecycle_operation_active_message_test" {
				t.Fatalf("resolution event preceded exact durable result: pending=%t operation=%s", store.pending != nil, operation)
			}
		}
		_, err := consumer.consume(context.Background())
		if persistenceFails && (err == nil || events != 0) || !persistenceFails && (err != nil || events != 1) {
			t.Fatalf("post-commit failure publication: failed=%t events=%d err=%v", persistenceFails, events, err)
		}
	}
}

func TestActiveSessionFailureDiagnosticHonorsUTF8ByteLimit(t *testing.T) {
	consumer, store, _ := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	consumer.response = usageConversationFunc(func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		return assemblyline.ConversationResponseDecision{}, 1, errors.New(strings.Repeat("界", 2000))
	})
	if _, err := consumer.consume(context.Background()); err != nil || len(store.failures) != 1 {
		t.Fatalf("bounded diagnostic could not be published: failures=%d err=%v", len(store.failures), err)
	}
	if len(store.failures[0]) > assemblyline.MaxObjectiveControlFeedbackBytes || !utf8.ValidString(store.failures[0]) ||
		!strings.Contains(store.failures[0], "...[truncated]") {
		t.Fatalf("failure diagnostic exceeds exact byte boundary: bytes=%d validUTF8=%t", len(store.failures[0]), utf8.ValidString(store.failures[0]))
	}
}

func TestActiveSessionQuestionFailureCannotPublishAfterAuthorityLoss(t *testing.T) {
	consumer, store, _ := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	consumer.response = usageConversationFunc(func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		store.authorityErr = queue.ErrStaleStepAttempt
		return assemblyline.ConversationResponseDecision{}, 1, errors.New("question provider failed after authority changed")
	})
	if _, err := consumer.consume(context.Background()); !errors.Is(err, queue.ErrStaleStepAttempt) ||
		len(store.failures) != 0 || store.pending == nil {
		t.Fatalf("revoked authority published a failed answer: failures=%v pending=%t err=%v", store.failures, store.pending != nil, err)
	}
}

func TestActiveSessionFailurePersistenceErrorRemainsVisible(t *testing.T) {
	consumer, store, _ := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	providerFailure := errors.New("observed question provider failure")
	persistenceFailure := errors.New("cannot persist message failure")
	store.failureErr = persistenceFailure
	consumer.response = usageConversationFunc(func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		return assemblyline.ConversationResponseDecision{}, 1, providerFailure
	})
	if _, err := consumer.consume(context.Background()); !errors.Is(err, persistenceFailure) ||
		!errors.Is(err, providerFailure) || store.pending == nil {
		t.Fatalf("uncommitted failure was hidden: pending=%t err=%v", store.pending != nil, err)
	}
}

func TestNativeWaitContinuesOriginalTaskAfterFailedQuestion(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	consumer.response = usageConversationFunc(func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		return assemblyline.ConversationResponseDecision{}, 1, errors.New("observed question provider failure")
	})
	failedMessage := make(chan struct{})
	store.afterResolve = func() { close(failedMessage) }
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	runtime := &nativeRuntimeV3{ctx: ctx, claim: claim, sessionMessages: consumer}
	err := runtime.runWithSessionMessages(runtime.ctx, func(ctx context.Context) error {
		select {
		case <-failedMessage:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err != nil || len(store.failures) != 1 || claim.Job.Status != "running" {
		t.Fatalf("failed question stopped original task: failures=%v status=%s err=%v", store.failures, claim.Job.Status, err)
	}
}
