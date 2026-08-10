package cognitionstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

const (
	EntryMappingSchemaV1  = "omnidex.cognition-state-entry-mapping.v1"
	maxMappedContentBytes = 64 * 1024
)

func mappingDigest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("%w: encode identity: %v", ErrInvalidMapping, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func mappingTextDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func validMappingDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validMappingIdentity(value string, maximum int) bool {
	if value == "" || len(value) > maximum || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return false
	}
	for _, character := range value {
		if unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func validMappedContent(value string) bool {
	return value != "" && len(value) <= maxMappedContentBytes && utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) && strings.TrimSpace(value) != ""
}

func evidenceLedgerRef(ref cognition.EvidenceRef) taskstate.Ref {
	return taskstate.Ref{
		URI:     "cognition:episode/" + string(ref.Revision.EpisodeID) + "/observation/" + string(ref.ObservationID),
		Version: strconv.FormatUint(ref.Revision.Number, 10), Hash: ref.SHA256,
		Relation: taskstate.RefEvidence,
	}
}

func revisionLedgerRef(revision cognition.WorldRevision) taskstate.Ref {
	return taskstate.Ref{
		URI:     "cognition:episode/" + string(revision.EpisodeID) + "/revision",
		Version: strconv.FormatUint(revision.Number, 10), Hash: revision.SHA256,
		Relation: taskstate.RefSource,
	}
}

func actionLedgerRef(episode cognition.EpisodeID, binding ActionBinding) (taskstate.Ref, error) {
	digest, err := mappingDigest(binding.Action)
	if err != nil {
		return taskstate.Ref{}, err
	}
	return taskstate.Ref{
		URI:     "cognition:episode/" + string(episode) + "/action/" + string(binding.Action.ID),
		Version: binding.Action.Schema.Version, Hash: digest, Relation: taskstate.RefSource,
	}, nil
}

func metadata(value any) (taskstate.JSONObject, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return taskstate.JSONObject{}, fmt.Errorf("%w: encode metadata: %v", ErrInvalidMapping, err)
	}
	result, err := taskstate.NewJSONObject(raw)
	if err != nil {
		return taskstate.JSONObject{}, fmt.Errorf("%w: metadata: %v", ErrInvalidMapping, err)
	}
	return result, nil
}
