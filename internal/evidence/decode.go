package evidence

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

// UnmarshalJSON rejects unknown fields, including retired hash-only receipts.
// Evidence is actual typed data, not an extensible set of authority labels.
func (record *Record) UnmarshalJSON(raw []byte) error {
	type plainRecord Record
	var decoded plainRecord
	if err := exactjson.ValidateObject(raw, &decoded, "evidence record"); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("decode evidence record: %w", err)
	}
	value := Record(decoded)
	if err := value.Validate(); err != nil {
		return err
	}
	*record = value
	return nil
}
