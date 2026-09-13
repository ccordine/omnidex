package workspacetransport

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/workspace"
)

func TestDisconnectDeliversAlreadyDecodedChangesBeforeFailure(t *testing.T) {
	pending := &pendingOperation{result: make(chan responseData, 1), finished: make(chan struct{})}
	changes := []workspace.Change{{Path: "first", Kind: workspace.ChangeCreate}, {Path: "second", Kind: workspace.ChangeCreate}, {Path: "third", Kind: workspace.ChangeDelete}}
	// The reader has filled the bounded queue and still owns another decoded
	// change when the connection ends. Returning early would lose that evidence
	// and leave the reader blocked on its final send.
	pending.result <- responseData{response: response{Change: &changes[0]}}
	go func() {
		for index := 1; index < len(changes); index++ {
			pending.result <- responseData{response: response{Change: &changes[index]}}
		}
		close(pending.finished)
	}()
	wantErr := errors.New("connection ended")
	var observed []workspace.Change
	err := drainPendingChanges(pending, func(change workspace.Change) error { observed = append(observed, change); return nil }, wantErr)
	if !errors.Is(err, wantErr) || !reflect.DeepEqual(observed, changes) {
		t.Fatalf("observed=%#v err=%v", observed, err)
	}
}
