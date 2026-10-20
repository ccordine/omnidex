package sessionintent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/userintent"
)

// Apply validates proposed data again at the repository boundary. Model-side
// validation is never authority, and stored JSON receives the same checks.
func Apply(c Context, patch []userintent.Patch) (Snapshot, error) {
	before := State(c)
	raw, err := json.Marshal(before)
	if err != nil {
		return Snapshot{}, err
	}
	after, err := userintent.Apply(raw, patch, Writable)
	if err != nil {
		return Snapshot{}, err
	}
	if err := Validate(raw, after); err != nil {
		return Snapshot{}, err
	}
	return userintent.DecodeState[Snapshot](after)
}

func Writable(path string) bool {
	switch path {
	case "/job_status", "/plan_state", "/message", "/native_reply", "/open", "/status_visible", "/approval":
		return true
	}
	parts := strings.Split(path, "/")
	return len(parts) == 4 && parts[1] == "items" && (parts[3] == "review_state" || parts[3] == "disposition")
}

func Validate(before, after json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(after, &fields); err != nil {
		return err
	}
	for _, key := range []string{"authority", "job_status", "plan_state", "items", "message", "native_reply", "open", "status_visible", "approval"} {
		value, ok := fields[key]
		if !ok {
			return fmt.Errorf("required state field removed: %s", key)
		}
		if string(value) == "null" && (key == "authority" || key == "job_status" || key == "plan_state" || key == "open" || key == "status_visible") {
			return fmt.Errorf("invalid null state field: %s", key)
		}
	}

	a, err := userintent.DecodeState[Snapshot](before)
	if err != nil {
		return err
	}
	b, err := userintent.DecodeState[Snapshot](after)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(a.Authority, b.Authority) || len(a.Items) != len(b.Items) {
		return fmt.Errorf("protected session authority changed")
	}
	c := a.Authority
	ledgerChanged := b.PlanState != a.PlanState
	needsDiscussion := false
	for i, item := range b.Items {
		old := a.Items[i]
		if item.ID != old.ID || item.Text != old.Text {
			return fmt.Errorf("protected ledger item changed")
		}
		if item.ReviewState != old.ReviewState || item.Disposition != old.Disposition {
			ledgerChanged = true
			if item.ReviewState != old.ReviewState && item.ReviewState != model.LedgerReviewAgreed && item.ReviewState != model.LedgerReviewNeedsDiscussion {
				return fmt.Errorf("invalid owner review state")
			}
			if item.Disposition != old.Disposition && (old.Disposition != model.LedgerDispositionActive || item.Disposition != model.LedgerDispositionTombstoned || item.ReviewState != old.ReviewState) {
				return fmt.Errorf("invalid item disposition delta")
			}
		}
		if item.Disposition == model.LedgerDispositionActive && item.ReviewState != model.LedgerReviewAgreed {
			needsDiscussion = true
		}
	}
	if ledgerChanged && (!c.Planning || c.PlanState != "proposed" || c.Revision < 1) {
		return fmt.Errorf("no displayed editable plan")
	}
	if b.PlanState != a.PlanState && (b.PlanState != "frozen" || needsDiscussion) {
		return fmt.Errorf("only an agreed plan can become frozen")
	}
	jobChanged := a.JobStatus != b.JobStatus
	if jobChanged {
		valid := c.JobID > 0 && ((b.JobStatus == model.JobStatusRunning && (c.Recoverable || c.Planning && b.PlanState == "frozen" && !needsDiscussion)) || (b.JobStatus == model.JobStatusCanceled && c.JobStatus != model.JobStatusFailed) || (b.JobStatus == model.JobStatusWaiting && (c.JobStatus == model.JobStatusRunning || c.JobStatus == model.JobStatusPending)))
		if !valid {
			return fmt.Errorf("unavailable job state transition")
		}
	}
	if b.Message != nil {
		if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, b.Message.Text); err != nil {
			return err
		}
		recipient := messageRecipient(c)
		if b.Message.Recipient != recipient {
			return fmt.Errorf("message recipient does not own the current workflow")
		}
	}
	if b.NativeReply != nil {
		if c.InputID == nil || *b.NativeReply == "" {
			return fmt.Errorf("no native question to answer")
		}
		if len(c.NativeChoices) > 0 {
			found := false
			for _, label := range c.NativeChoices {
				if label == *b.NativeReply {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("native answer is not a current choice")
			}
		}
	}
	if b.Approval != nil && c.Pending == nil {
		return fmt.Errorf("no pending proposal to confirm")
	}
	// These changes require different authoritative snapshots. A single message
	// cannot apply a pending proposal and independently mutate its base state.
	exclusive := 0
	for _, changed := range []bool{jobChanged, ledgerChanged, b.Message != nil, b.NativeReply != nil, b.Approval != nil} {
		if changed {
			exclusive++
		}
	}
	if jobChanged && ledgerChanged && b.PlanState == "frozen" && b.JobStatus == model.JobStatusRunning {
		exclusive--
	}
	if exclusive > 1 {
		return fmt.Errorf("incompatible state changes require clarification")
	}
	return nil
}

func NeedsApproval(c Context, after Snapshot) bool {
	if after.JobStatus != c.JobStatus && (after.JobStatus == model.JobStatusCanceled || after.JobStatus == model.JobStatusWaiting) {
		return true
	}
	for i, item := range after.Items {
		if item.Disposition != c.Items[i].Disposition || item.ReviewState != c.Items[i].ReviewState && item.ReviewState == model.LedgerReviewNeedsDiscussion {
			return true
		}
	}
	return false
}

// ConfirmationPitch describes the actual proposed delta, never a selectable
// catalog or another semantic decision.
func ConfirmationPitch(c Context, after Snapshot) string {
	lines := []string{}
	if c.JobStatus != after.JobStatus {
		lines = append(lines, fmt.Sprintf("Job %d: %s → %s.", c.JobID, c.JobStatus, after.JobStatus))
	}
	for i, item := range after.Items {
		if item.Disposition != c.Items[i].Disposition {
			lines = append(lines, "Remove plan item: "+item.Text)
		}
		if item.ReviewState != c.Items[i].ReviewState {
			lines = append(lines, "Plan item needs discussion: "+item.Text)
		}
	}
	return strings.Join(lines, "\n") + "\nApply these changes?"
}
