package cognitiongauntlet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func (report CausalAcquisitionReport) Validate() error {
	if report.Schema != CausalAcquisitionReportSchemaV1 ||
		!validDigest(report.EpisodeSealSHA256) || !validDigest(report.OracleSHA256) ||
		!validDigest(report.EvidenceUseSHA256) || report.RequiredEvidence <= 0 ||
		report.AcquiredEvidence < 0 || report.AcquiredEvidence > report.RequiredEvidence ||
		report.AcquisitionTraceRefs == nil ||
		len(report.AcquisitionTraceRefs) > report.AcquiredEvidence ||
		(report.AcquiredEvidence == 0) != (len(report.AcquisitionTraceRefs) == 0) {
		return fmt.Errorf("causal acquisition report authority is invalid")
	}
	if err := requireExact(report.SurfaceVersion, "causal acquisition surface version", 256); err != nil {
		return err
	}
	previous := ""
	for index, ref := range report.AcquisitionTraceRefs {
		if err := requireExact(ref, "causal acquisition trace ref", 512); err != nil {
			return err
		}
		if index > 0 && ref <= previous {
			return fmt.Errorf("causal acquisition trace refs must be unique and sorted")
		}
		previous = ref
	}
	return nil
}

func decodeStrictJSON(raw []byte, target any, label string) error {
	if err := cognition.ValidateExactJSONObject(raw, target, label); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	return nil
}

func countEvidenceIdentities(
	raw json.RawMessage,
	expected labyrinth.EvidenceIdentity,
) (int, bool, error) {
	if err := cognition.ValidateUniqueJSONObject(raw, "bounded surface evidence result"); err != nil {
		return 0, false, fmt.Errorf("decode bounded surface evidence result: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return 0, false, fmt.Errorf("decode bounded surface evidence result: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return 0, false, fmt.Errorf("bounded surface evidence result has trailing content")
	}
	matches, conflicts := 0, false
	walkEvidenceResult(value, expected, &matches, &conflicts)
	return matches, conflicts, nil
}

func walkEvidenceResult(
	value any,
	expected labyrinth.EvidenceIdentity,
	matches *int,
	conflicts *bool,
) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			walkEvidenceResult(item, expected, matches, conflicts)
		}
	case map[string]any:
		if id, exists := typed["id"].(string); exists && id == expected.ID {
			hashes := make([]string, 0, 2)
			for _, field := range []string{"sha256", "content_sha256"} {
				if hash, exists := typed[field].(string); exists {
					hashes = append(hashes, hash)
				}
			}
			if len(hashes) == 0 {
				*conflicts = true
			} else {
				valid := true
				for _, hash := range hashes {
					if hash != expected.SHA256 {
						valid = false
					}
				}
				if valid {
					*matches++
				} else {
					*conflicts = true
				}
			}
		}
		for _, child := range typed {
			walkEvidenceResult(child, expected, matches, conflicts)
		}
	}
}
