package cognitiongauntlet

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

type visibleFilesystemSearch struct {
	Query   string `json:"query"`
	Matches []struct {
		ID            string `json:"id"`
		Location      string `json:"location"`
		Line          int    `json:"line"`
		Content       string `json:"content"`
		ContentSHA256 string `json:"content_sha256"`
	} `json:"matches"`
	Truncated bool `json:"truncated"`
}

type visibleFilesystemRead struct {
	Records []struct {
		ID       string `json:"id"`
		Location string `json:"location"`
		Content  string `json:"content"`
		SHA256   string `json:"sha256"`
	} `json:"records"`
}

type visibleFilesystemObserve struct {
	Location string `json:"location"`
	Entries  []struct {
		ID     string `json:"id"`
		SHA256 string `json:"sha256"`
	} `json:"entries"`
	Truncated bool `json:"truncated"`
}

type visibleFilesystemNavigate struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type visibleFilesystemTake struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}

type visibleFilesystemUse struct {
	Item   string `json:"item"`
	Target string `json:"target"`
}

type visibleFilesystemWrite struct {
	ID             string `json:"id"`
	PreviousSHA256 string `json:"previous_sha256"`
	CurrentSHA256  string `json:"current_sha256"`
}

type visibleRecordSurfaceResult struct {
	Format              string              `json:"format"`
	Collection          string              `json:"collection"`
	PreviousCollection  string              `json:"previous_collection,omitempty"`
	Query               string              `json:"query,omitempty"`
	Records             []visibleRecordWire `json:"records"`
	TotalRecords        int                 `json:"total_records"`
	OmittedRecords      int                 `json:"omitted_records"`
	HeldRecord          string              `json:"held_record,omitempty"`
	UsedRecord          string              `json:"used_record,omitempty"`
	CurrentRecordSHA256 string              `json:"current_record_sha256"`
	PreviousSHA256      string              `json:"previous_sha256,omitempty"`
	CurrentSHA256       string              `json:"current_sha256,omitempty"`
}

func visibleFilesystemFactRecords(operation string, raw json.RawMessage) ([]visibleFactRecord, error) {
	switch cognition.ActionKind(operation) {
	case "search":
		payload := visibleFilesystemSearch{}
		if err := decodeStrictJSON(raw, &payload, "filesystem public search result"); err != nil {
			return nil, err
		}
		if requireExact(payload.Query, "filesystem search query", 512) != nil || payload.Matches == nil {
			return nil, fmt.Errorf("filesystem search result authority is invalid")
		}
		records := make([]visibleRecordWire, len(payload.Matches))
		for index, match := range payload.Matches {
			if match.Line < 1 {
				return nil, fmt.Errorf("filesystem search line authority is invalid")
			}
			records[index] = visibleRecordWire{
				ID: match.ID, Location: match.Location, Content: match.Content,
				ContentSHA256: match.ContentSHA256,
			}
		}
		return collectVisibleFactRecords(records)
	case "read":
		payload := visibleFilesystemRead{}
		if err := decodeStrictJSON(raw, &payload, "filesystem public read result"); err != nil {
			return nil, err
		}
		if payload.Records == nil {
			return nil, fmt.Errorf("filesystem read result authority is invalid")
		}
		records := make([]visibleRecordWire, len(payload.Records))
		for index, record := range payload.Records {
			records[index] = visibleRecordWire{
				ID: record.ID, Location: record.Location, Content: record.Content,
				ContentSHA256: record.SHA256,
			}
		}
		return collectVisibleFactRecords(records)
	case "observe":
		payload := visibleFilesystemObserve{}
		if err := decodeStrictJSON(raw, &payload, "filesystem public observe result"); err != nil {
			return nil, err
		}
		if requireExact(payload.Location, "filesystem location", 256) != nil || payload.Entries == nil {
			return nil, fmt.Errorf("filesystem observe result authority is invalid")
		}
		for _, entry := range payload.Entries {
			if requireExact(entry.ID, "filesystem entry ID", 256) != nil || !validDigest(entry.SHA256) {
				return nil, fmt.Errorf("filesystem observe entry authority is invalid")
			}
		}
	case "navigate":
		payload := visibleFilesystemNavigate{}
		if err := decodeStrictJSON(raw, &payload, "filesystem public navigate result"); err != nil {
			return nil, err
		}
		if requireExact(payload.From, "filesystem source", 256) != nil ||
			requireExact(payload.To, "filesystem target", 256) != nil {
			return nil, fmt.Errorf("filesystem navigate result authority is invalid")
		}
	case "take":
		payload := visibleFilesystemTake{}
		if err := decodeStrictJSON(raw, &payload, "filesystem public take result"); err != nil {
			return nil, err
		}
		if requireExact(payload.ID, "filesystem object", 256) != nil || !validDigest(payload.SHA256) {
			return nil, fmt.Errorf("filesystem take result authority is invalid")
		}
	case "use":
		payload := visibleFilesystemUse{}
		if err := decodeStrictJSON(raw, &payload, "filesystem public use result"); err != nil {
			return nil, err
		}
		if requireExact(payload.Item, "filesystem item", 256) != nil ||
			requireExact(payload.Target, "filesystem target", 256) != nil {
			return nil, fmt.Errorf("filesystem use result authority is invalid")
		}
	case "write":
		payload := visibleFilesystemWrite{}
		if err := decodeStrictJSON(raw, &payload, "filesystem public write result"); err != nil {
			return nil, err
		}
		if requireExact(payload.ID, "filesystem write target", 256) != nil ||
			!validDigest(payload.PreviousSHA256) || !validDigest(payload.CurrentSHA256) {
			return nil, fmt.Errorf("filesystem write result authority is invalid")
		}
	default:
		return nil, fmt.Errorf("filesystem public operation is not registered")
	}
	return []visibleFactRecord{}, nil
}

func visibleRecordSurfaceFactRecords(operation string, raw json.RawMessage) ([]visibleFactRecord, error) {
	payload := visibleRecordSurfaceResult{}
	if err := decodeStrictJSON(raw, &payload, "record surface public result"); err != nil {
		return nil, err
	}
	if payload.Format != "record-result.v1" || payload.Records == nil ||
		payload.TotalRecords < 0 || payload.OmittedRecords < 0 {
		return nil, fmt.Errorf("record surface public result authority is invalid")
	}
	switch cognition.ActionKind(operation) {
	case "search", "read":
		return collectVisibleFactRecords(payload.Records)
	case "observe", "navigate", "take", "use", "write":
		return []visibleFactRecord{}, nil
	default:
		return nil, fmt.Errorf("record surface public operation is not registered")
	}
}
