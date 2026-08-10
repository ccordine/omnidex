package cognitiontransport

import (
	"errors"
	"net/http"

	"github.com/gryph/omnidex/internal/cognition"
)

func (server *server) evaluate(writer http.ResponseWriter, request *http.Request) {
	var input evaluateRequest
	if err := decodeRequest(request, &input); err != nil || input.Protocol != ProtocolVersionV1 ||
		validateCompletionRequest(input.Request) != nil {
		writeWireError(writer, http.StatusBadRequest, "invalid_completion_request", "Completion request is invalid.")
		return
	}
	result, err := server.completion.Evaluate(request.Context(), input.Request)
	if err != nil {
		writeCompletionError(writer, err)
		return
	}
	if err := result.ValidateFor(
		input.Request.Obligation, input.Request.Revision, input.Request.EvidenceRefs,
	); err != nil {
		writeWireError(writer, http.StatusInternalServerError, "invalid_completion_response", "Completion evaluator returned invalid evidence.")
		return
	}
	writeWire(writer, http.StatusOK, wireResponse{
		Protocol: ProtocolVersionV1, Completion: &result,
	})
}

func writeCompletionError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cognition.ErrAuthorityDenied):
		writeWireError(writer, http.StatusConflict, "stale_authority", "Completion authority is stale.")
	case errors.Is(err, cognition.ErrInvalidRevision):
		writeWireError(writer, http.StatusConflict, "stale_revision", "Completion revision is stale.")
	case errors.Is(err, cognition.ErrInvalidEvidence):
		writeWireError(writer, http.StatusConflict, "invalid_evidence", "Completion evidence is unavailable.")
	case errors.Is(err, cognition.ErrInvalidCompletionCheck),
		errors.Is(err, cognition.ErrUnsupportedCompletionPredicate):
		writeWireError(writer, http.StatusConflict, "unsupported_completion", "Completion evaluator is not registered.")
	default:
		writeWireError(writer, http.StatusInternalServerError, "completion_failed", "Completion evaluation failed.")
	}
}
