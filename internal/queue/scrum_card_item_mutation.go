package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type ScrumCardItemCollection string

const (
	ScrumCardChecklist    ScrumCardItemCollection = "checklist"
	ScrumCardTestCriteria ScrumCardItemCollection = "test_criteria"
)

type ScrumCardItemAction string

const (
	ScrumCardItemAdd    ScrumCardItemAction = "add"
	ScrumCardItemToggle ScrumCardItemAction = "toggle"
	ScrumCardItemRemove ScrumCardItemAction = "remove"
)

var ErrScrumCardItemNotFound = errors.New("Scrum card item was not found")

type ScrumCardItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type ScrumCardItemMutation struct {
	ProjectID         int64
	CardID            string
	ExpectedUpdatedAt time.Time
	Collection        ScrumCardItemCollection
	Action            ScrumCardItemAction
	ItemID            string
	Text              string
	Done              *bool
}

func (mutation *ScrumCardItemMutation) validate() error {
	if mutation == nil || mutation.ProjectID <= 0 || mutation.CardID == "" {
		return fmt.Errorf("Scrum card item mutation requires a project and card")
	}
	if mutation.CardID != strings.TrimSpace(mutation.CardID) {
		return fmt.Errorf("Scrum card item mutation card ID must be canonical")
	}
	if mutation.ItemID != strings.TrimSpace(mutation.ItemID) {
		return fmt.Errorf("Scrum card item mutation item ID must be canonical")
	}
	if mutation.Text != strings.TrimSpace(mutation.Text) {
		return fmt.Errorf("Scrum card item mutation text must be canonical")
	}
	if mutation.ExpectedUpdatedAt.IsZero() {
		return fmt.Errorf("Scrum card item mutation requires an expected card revision")
	}
	if mutation.Collection != ScrumCardChecklist && mutation.Collection != ScrumCardTestCriteria {
		return fmt.Errorf("unsupported Scrum card item collection %q", mutation.Collection)
	}
	if !utf8.ValidString(mutation.Text) || strings.ContainsRune(mutation.Text, '\x00') {
		return fmt.Errorf("Scrum card item text must be valid UTF-8 without NUL")
	}
	switch mutation.Action {
	case ScrumCardItemAdd:
		if mutation.Text == "" || mutation.ItemID != "" || mutation.Done != nil {
			return fmt.Errorf("add requires text and forbids item_id and done")
		}
	case ScrumCardItemToggle:
		if mutation.ItemID == "" || mutation.Text != "" || mutation.Done == nil {
			return fmt.Errorf("toggle requires item_id and done and forbids text")
		}
	case ScrumCardItemRemove:
		if mutation.ItemID == "" || mutation.Text != "" || mutation.Done != nil {
			return fmt.Errorf("remove requires item_id and forbids text and done")
		}
	default:
		return fmt.Errorf("unsupported Scrum card item action %q", mutation.Action)
	}
	return nil
}

func (mutation *ScrumCardItemMutation) ValidateForTransport() error {
	return mutation.validate()
}

func (r *Repository) MutateScrumCardItem(ctx context.Context, mutation ScrumCardItemMutation) (DBScrumCard, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return DBScrumCard{}, fmt.Errorf("PostgreSQL and context are required for Scrum card item mutation")
	}
	if err := mutation.validate(); err != nil {
		return DBScrumCard{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("begin Scrum card item mutation: %w", err)
	}
	defer rollbackTx(ctx, tx, "Scrum card item mutation")
	current, err := lockScrumCardTx(ctx, tx, mutation.ProjectID, mutation.CardID)
	if err != nil {
		return DBScrumCard{}, err
	}
	if !current.UpdatedAt.Equal(mutation.ExpectedUpdatedAt) {
		return DBScrumCard{}, fmt.Errorf("%w: Scrum card %q changed; reload server state and retry", ErrScrumCardVersionConflict, mutation.CardID)
	}
	raw := current.Checklist
	if mutation.Collection == ScrumCardTestCriteria {
		raw = current.TestCriteria
	}
	items, err := decodeCanonicalScrumCardItems(raw)
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("decode %s: %w", mutation.Collection, err)
	}
	items, err = applyScrumCardItemMutation(items, mutation)
	if err != nil {
		return DBScrumCard{}, err
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("encode %s: %w", mutation.Collection, err)
	}
	statement := `UPDATE scrum_cards SET checklist=$3::jsonb, updated_at=GREATEST(clock_timestamp(), updated_at + interval '1 microsecond') WHERE project_id=$1 AND id=$2`
	if mutation.Collection == ScrumCardTestCriteria {
		statement = `UPDATE scrum_cards SET test_criteria=$3::jsonb, updated_at=GREATEST(clock_timestamp(), updated_at + interval '1 microsecond') WHERE project_id=$1 AND id=$2`
	}
	tag, err := tx.Exec(ctx, statement, mutation.ProjectID, mutation.CardID, string(encoded))
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("persist %s: %w", mutation.Collection, err)
	}
	if tag.RowsAffected() != 1 {
		return DBScrumCard{}, fmt.Errorf("%w: Scrum card %q disappeared during mutation", ErrScrumCardNotFound, mutation.CardID)
	}
	if err := refreshScrumFlowMetricsTx(ctx, tx, mutation.ProjectID, mutation.CardID); err != nil {
		return DBScrumCard{}, fmt.Errorf("refresh Scrum item mutation flow metrics: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE projects SET last_seen_at=clock_timestamp(), updated_at=clock_timestamp() WHERE id=$1`, mutation.ProjectID); err != nil {
		return DBScrumCard{}, fmt.Errorf("touch Scrum card item project: %w", err)
	}
	updated, err := scanDBScrumCard(tx.QueryRow(ctx, scrumCardSelectSQL, mutation.ProjectID, mutation.CardID))
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("load mutated Scrum card item: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return DBScrumCard{}, fmt.Errorf("commit Scrum card item mutation: %w", err)
	}
	return updated, nil
}

func decodeCanonicalScrumCardItems(raw json.RawMessage) ([]ScrumCardItem, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`[]`)
	}
	var items []ScrumCardItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(items))
	for index := range items {
		if items[index].ID == "" || items[index].Text == "" {
			return nil, fmt.Errorf("item %d requires a non-blank id and text", index)
		}
		if items[index].ID != strings.TrimSpace(items[index].ID) ||
			items[index].Text != strings.TrimSpace(items[index].Text) {
			return nil, fmt.Errorf("item %d contains noncanonical id or text", index)
		}
		if _, duplicate := seen[items[index].ID]; duplicate {
			return nil, fmt.Errorf("duplicate item id %q", items[index].ID)
		}
		seen[items[index].ID] = struct{}{}
	}
	return items, nil
}

func applyScrumCardItemMutation(items []ScrumCardItem, mutation ScrumCardItemMutation) ([]ScrumCardItem, error) {
	switch mutation.Action {
	case ScrumCardItemAdd:
		idBytes := make([]byte, 12)
		if _, err := rand.Read(idBytes); err != nil {
			return nil, fmt.Errorf("generate Scrum card item identity: %w", err)
		}
		return append(items, ScrumCardItem{ID: "item_" + hex.EncodeToString(idBytes), Text: mutation.Text}), nil
	case ScrumCardItemToggle:
		for index := range items {
			if items[index].ID == mutation.ItemID {
				items[index].Done = *mutation.Done
				return items, nil
			}
		}
	case ScrumCardItemRemove:
		for index := range items {
			if items[index].ID == mutation.ItemID {
				return append(items[:index:index], items[index+1:]...), nil
			}
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrScrumCardItemNotFound, mutation.ItemID)
}
